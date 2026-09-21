package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/Kirizu-Official/KiriVers/internal/model"
	"github.com/Kirizu-Official/KiriVers/internal/service/update"
	"github.com/Kirizu-Official/KiriVers/pkg/semver"
)

// 本文件实现发布后自动差量与多文件增量 zip（docs/app-init.md §7.2 / §7.5 / §5.9，
// 任务 09-12-auto-delta-patch C13-1..C13-7）：
//
//   - 触发点：PublishVersion 成功后 + 已 Published Version 的新 Line 就绪转换后
//     （补平台场景），均只做毫秒级入队（notifyLineReady），生成全部异步；
//   - Job：单一 auto_delta 任务编排目标 Version 的全部就绪线（os/arch 在执行时
//     枚举，一条 Job 处理全部线），复用既有 JobRunner / 24h Idempotency-Key 体系；
//   - 合格源（C13-1 / C13-5，执行时选择）：同项目、同 (os,arch)、比较键低于目标、
//     status=published（天然排除吊销）、该线未 yank/disabled、线就绪且 kind=full
//     产物存在；按 matrix.DeltaSourceCount（默认 3）取比较键最高的 N 个；
//   - 单文件线（C13-1）：复用手动 delta_generate 的执行路径 generateDelta，
//     幂等身份 (algo, srcsha, dstsha) 已存在则跳过（C13-4）；
//   - 多文件线（C13-2..C13-4）：对每个合格源生成仅含 Manifest 新增/替换完整文件
//     （KEEP 不进包）的增量 zip，从目标线预生成全量 zip 提取条目字节；
//     入队前 D6/D7（未压缩 Manifest size，非 zip Size）超限则丢弃不落产物行；
//   - 失败语义（C13-6）：job 失败只记 error_message，不影响 Version 已 Published；
//     各线失败隔离，错误进 job result。
//
// 24 小时请求级幂等键：确定性键 "auto-delta:{version_id}:{就绪线签名}"。
// 同一就绪状态重复入队 → 返回既有 job；新 Line 就绪 → 签名变化 → 新 Job
// （补平台场景必须重新生成，否则新线永远拿不到差量）。

// autoDeltaJobType 是发布后自动差量任务类型（父设计 job 清单内
// delta_generate / patch_zip 语义由本任务统一编排）。
const autoDeltaJobType = "auto_delta"

// PatchMaxFullRatio 与 update.PatchPackageMaxSizeRatio 同源（D7：70% 未压缩 Manifest）。
const PatchMaxFullRatio = update.PatchPackageMaxSizeRatio

// autoDeltaJobPayload 是 auto_delta 任务的载荷：只锚定项目与目标版本，
// 线与合格源全部在执行时枚举（实体可能在该期间变化，worker 端再次校验）。
type autoDeltaJobPayload struct {
	ProjectID uuid.UUID `json:"project_id"`
	VersionID uuid.UUID `json:"version_id"`
}

// autoDeltaSourceStatus 是单条源产物的生成结果状态。
const (
	// autoDeltaSourceCreated 已生成并落库新产物。
	autoDeltaSourceCreated = "created"
	// autoDeltaSourceSkipped 幂等命中：同身份产物已存在，未重新生成（C13-4）。
	autoDeltaSourceSkipped = "skipped"
	// autoDeltaSourceDiscarded 增量包 ≥ 全量 70%，按 §7.5 丢弃（C13-3）。
	autoDeltaSourceDiscarded = "discarded"
	// autoDeltaSourceError 本源生成失败（不阻断同线其它源与其它线）。
	autoDeltaSourceError = "error"
)

// AutoDeltaSourceResult 记录单个 (源版本, hw 变体) 的差量生成结果（job.result）。
type AutoDeltaSourceResult struct {
	// SourceVersionRef 源版本展示引用（优先规范 SemVer，否则整数）。
	SourceVersionRef string `json:"source_version_ref,omitempty"`
	// HwRev 硬件变体（空 = 默认变体）。
	HwRev string `json:"hw_rev,omitempty"`
	// Status created | skipped | discarded | error。
	Status string `json:"status"`
	// ArtifactKind 产物种类（model.ArtifactKindDelta / model.ArtifactKindPatch）。
	ArtifactKind string `json:"artifact_kind,omitempty"`
	ArtifactID   string `json:"artifact_id,omitempty"`
	FileName     string `json:"file_name,omitempty"`
	Size         int64  `json:"size,omitempty"`
	// Error 失败原因（Status=error 时非空）。
	Error string `json:"error,omitempty"`
}

// AutoDeltaLineResult 记录单条就绪线的差量生成结果（失败隔离单元）。
type AutoDeltaLineResult struct {
	OS      string                  `json:"os"`
	Arch    string                  `json:"arch"`
	Sources []AutoDeltaSourceResult `json:"sources,omitempty"`
	// Error 整线处理失败的原因（如目标全量包不可读）；单源失败记在 Sources[].Error。
	Error string `json:"error,omitempty"`
}

// AutoDeltaJobResult 记录 auto_delta 任务整体结果（job.result）。
type AutoDeltaJobResult struct {
	Success bool                  `json:"success"`
	Lines   []AutoDeltaLineResult `json:"lines,omitempty"`
	Error   string                `json:"error,omitempty"`
}

// notifyLineReady 在「某 Version 的某条线进入 ready」或「版本刚 Publish」后
// 最佳努力触发自动差量（C13-1 / C13-6）：入队是毫秒级且绝不影响发布主流程，
// 任何错误静默忽略（差量是增强，不是闸门）。
func (s *ProjectService) notifyLineReady(ctx context.Context, projectID, versionID uuid.UUID) {
	if s.jobs == nil {
		s.stampReadyLinesNow(ctx, versionID)
		return
	}
	_, _, _ = s.EnqueueAutoDelta(ctx, projectID, versionID)
}

// stampReadyLinesNow 无 Job 仓储时立即盖 packs_ready_at（HTTP 单测无 worker）。
func (s *ProjectService) stampReadyLinesNow(ctx context.Context, versionID uuid.UUID) {
	lines, err := s.store.ListVersionLines(ctx, versionID)
	if err != nil {
		return
	}
	now := time.Now().UTC()
	for i := range lines {
		l := &lines[i]
		if l.Status != model.VersionLineStatusReady || l.PacksReadyAt != nil {
			continue
		}
		t := now
		l.PacksReadyAt = &t
		_ = s.store.SaveVersionLine(ctx, l)
		if l.ProjectID != uuid.Nil {
			s.invalidateProject(ctx, l.ProjectID)
		}
		s.kickLineReplica(ctx, l)
	}
}

// stampPacksReadyAt 系统预热成功后盖戳；已有戳不覆盖。客户端动态打包不得调用。
func (s *ProjectService) stampPacksReadyAt(ctx context.Context, line *model.VersionLine) {
	if line == nil || line.PacksReadyAt != nil {
		return
	}
	now := time.Now().UTC()
	line.PacksReadyAt = &now
	if err := s.store.SaveVersionLine(ctx, line); err != nil {
		return
	}
	s.invalidateProject(ctx, line.ProjectID)
	s.kickLineReplica(ctx, line)
}

func (s *ProjectService) kickLineReplica(ctx context.Context, line *model.VersionLine) {
	if s == nil || line == nil || !s.clusterActive {
		return
	}
	p, err := s.store.GetByID(ctx, line.ProjectID)
	if err != nil || p == nil {
		return
	}
	arts, err := s.store.ListArtifactsByLineAndKind(ctx, line.ID, model.ArtifactKindFull)
	if err != nil {
		return
	}
	if extras, err := s.store.ListArtifactsByLineAndKind(ctx, line.ID, model.ArtifactKindStoreFull); err == nil {
		arts = append(arts, extras...)
	}
	keys := make([]string, 0, len(arts))
	sizes := make([]int64, 0, len(arts))
	for i := range arts {
		if arts[i].StorageKey == "" {
			continue
		}
		keys = append(keys, arts[i].StorageKey)
		sizes = append(sizes, arts[i].Size)
	}
	s.syncLineAfterReady(ctx, p, line.VersionID, line.ID, keys, sizes)
}

// SyncMissingReplicas 在本机代拉模式下补齐尚未落到本地的就绪线。
func (s *ProjectService) SyncMissingReplicas(ctx context.Context) {
	if s == nil || !s.keepLocalReplica() {
		return
	}
	projects, err := s.store.List(ctx)
	if err != nil {
		return
	}
	for i := range projects {
		p := &projects[i]
		vers, err := s.store.ListVersions(ctx, p.ID)
		if err != nil {
			continue
		}
		for j := range vers {
			lines, err := s.store.ListVersionLines(ctx, vers[j].ID)
			if err != nil {
				continue
			}
			for k := range lines {
				line := &lines[k]
				if line.PacksReadyAt == nil {
					continue
				}
				s.kickLineReplica(ctx, line)
			}
		}
	}
}

func linePreheatSucceeded(res *AutoDeltaLineResult) bool {
	if res == nil || res.Error != "" {
		return false
	}
	if len(res.Sources) == 0 {
		return true
	}
	for _, sr := range res.Sources {
		if sr.Status != autoDeltaSourceError {
			return true
		}
	}
	return false
}

// EnqueueAutoDelta 为指定版本幂等入队 auto_delta 任务（design §1）：
//
//   - 版本未 Published / 无就绪线 → 不入队，返回 (uuid.Nil, false, nil)；
//   - 幂等：以 "auto-delta:{version_id}:{就绪线签名}" 为 24h Idempotency-Key，
//     同一就绪状态重复调用 → 返回既有 job（created=false）；
//   - 新 Line 就绪 → 签名变化 → 新 Job（补平台场景必须重新枚举生成）。
func (s *ProjectService) EnqueueAutoDelta(ctx context.Context, projectID, versionID uuid.UUID) (uuid.UUID, bool, error) {
	if s.jobs == nil {
		return uuid.Nil, false, fmt.Errorf("job store is not configured")
	}
	v, err := s.store.GetVersionByID(ctx, versionID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return uuid.Nil, false, nil
		}
		return uuid.Nil, false, fmt.Errorf("resolve version for auto delta: %w", err)
	}
	// 只对已发布版本生成差量（§7.2：Published 后可自动相对最近 N 个已发布版本生成）。
	if v.Status != model.VersionStatusPublished {
		return uuid.Nil, false, nil
	}
	lines, err := s.store.ListVersionLines(ctx, v.ID)
	if err != nil {
		return uuid.Nil, false, fmt.Errorf("list version lines for auto delta: %w", err)
	}
	// 就绪线签名：os/arch 升序拼接；既做幂等键也做补平台再触发的区分度。
	parts := make([]string, 0, len(lines))
	for i := range lines {
		if lines[i].Status == model.VersionLineStatusReady {
			parts = append(parts, lines[i].OS+"/"+lines[i].Arch)
		}
	}
	if len(parts) == 0 {
		return uuid.Nil, false, nil
	}
	sort.Strings(parts)
	key := "auto-delta:" + v.ID.String() + ":" + strings.Join(parts, ",")

	if existing, err := s.jobs.GetByIdempotencyKey(ctx, projectID, key); err == nil && existing != nil {
		return existing.ID, false, nil
	}

	payload, err := json.Marshal(autoDeltaJobPayload{ProjectID: projectID, VersionID: v.ID})
	if err != nil {
		return uuid.Nil, false, err
	}
	job := &model.Job{
		ID:             uuid.New(),
		Type:           autoDeltaJobType,
		Status:         model.JobStatusQueued,
		Payload:        payload,
		ProjectID:      &projectID,
		OwnerNodeID:    s.ownerNodePtr(),
		IdempotencyKey: &key,
	}
	if err := s.jobs.Create(ctx, job); err != nil {
		return uuid.Nil, false, err
	}
	return job.ID, true, nil
}

// RegisterAutoDeltaJobHandler 为 JobWorker 注册 auto_delta 处理器。
// 返回非 nil 错误时由 JobWorker 统一 MarkFailed（C13-6：仅任务失败，
// 不影响 Version 已 Published 状态）。
func (s *ProjectService) RegisterAutoDeltaJobHandler(w *JobWorker) {
	if w == nil {
		return
	}
	w.RegisterHandler(autoDeltaJobType, func(ctx context.Context, jobType string, payload []byte) error {
		var p autoDeltaJobPayload
		if err := json.Unmarshal(payload, &p); err != nil {
			return err
		}
		result, runErr := s.runAutoDelta(ctx, p)
		if runErr != nil {
			return runErr
		}
		// 全部线均无产物且记录了错误时 runAutoDelta 返回错误；此处仅透传成功结果。
		_ = result
		return nil
	})
}

// ExecuteAutoDeltaJob 直接执行指定 auto_delta 任务（供测试或手动补偿调用）；
// 与 worker 路径不同，这里把结果 JSON 与错误一并落库。
func (s *ProjectService) ExecuteAutoDeltaJob(ctx context.Context, jobID uuid.UUID) error {
	if s.jobs == nil {
		return fmt.Errorf("job store is not configured")
	}
	job, err := s.jobs.GetByID(ctx, jobID)
	if err != nil {
		return err
	}
	if job.Type != autoDeltaJobType {
		return fmt.Errorf("unexpected job type: %s", job.Type)
	}
	var payload autoDeltaJobPayload
	if err := json.Unmarshal(job.Payload, &payload); err != nil {
		_ = s.jobs.MarkFailed(ctx, job.ID, "invalid payload json: "+err.Error())
		return err
	}
	result, runErr := s.runAutoDelta(ctx, payload)
	resultJSON, _ := json.Marshal(result)
	if runErr != nil {
		_ = s.jobs.MarkFailedWithResult(ctx, job.ID, runErr.Error(), resultJSON)
		return runErr
	}
	_ = s.jobs.MarkSucceededWithResult(ctx, job.ID, resultJSON)
	return nil
}

// autoDeltaVersionKey 提取 Version 在项目比较引擎下的比较键形态
// （与 update 包 versionKey 同语义；update 包实现不导出，此处独立声明）。
// 返回 (是否整数键, 整数值, 规范 SemVer, 是否可比较)。
func autoDeltaVersionKey(engine string, v *model.Version) (isInteger bool, intVal int64, semverVal string, ok bool) {
	if engine == model.CompareEngineInteger {
		if v.VersionInteger == nil {
			return false, 0, "", false
		}
		return true, *v.VersionInteger, "", true
	}
	if v.VersionSemverCanonical == nil || *v.VersionSemverCanonical == "" {
		return false, 0, "", false
	}
	return false, 0, *v.VersionSemverCanonical, true
}

// autoDeltaCompare 比较两个同形态比较键，返回 -1/0/1；形态不同返回 0 且 ok=false。
func autoDeltaCompare(aInt bool, aIntVal int64, aSem string, bInt bool, bIntVal int64, bSem string) (int, bool) {
	if aInt != bInt {
		return 0, false
	}
	if aInt {
		switch {
		case aIntVal < bIntVal:
			return -1, true
		case aIntVal > bIntVal:
			return 1, true
		}
		return 0, true
	}
	c, err := semver.Compare(aSem, bSem)
	if err != nil {
		// 库内规范 SemVer 均已在写入时校验，此处兜底不参与比较。
		return 0, false
	}
	return c, true
}

// autoDeltaSource 是一个合格差量基线源（design §2）。
type autoDeltaSource struct {
	version *model.Version
	line    *model.VersionLine
	full    *model.Artifact // 该源在 (os,arch[,hw]) 上的 kind=full 产物
}

// selectAutoDeltaSources 为目标线 (os,arch[,hw]) 选择合格差量源（C13-1 / C13-5）：
//
//   - 同项目、同 (os,arch)、status=published（排除吊销/Draft/弃用）、
//     比较键严格低于目标（非降级方向）、该线 ready（排除 yank/disabled，
//     yank 源禁作差量基线，C13-5）、线就绪且 hw 匹配的 kind=full 产物存在；
//   - 按比较键降序取前 n 个（n = matrix.DeltaSourceCount，默认 3，§7.2）。
func (s *ProjectService) selectAutoDeltaSources(ctx context.Context, proj *model.Project, targetVer *model.Version, targetLine *model.VersionLine, hwRev *string, n int) ([]autoDeltaSource, error) {
	versions, err := s.store.ListVersions(ctx, proj.ID)
	if err != nil {
		return nil, fmt.Errorf("list versions for auto delta: %w", err)
	}

	tgtInt, tgtIntVal, tgtSem, tgtOK := autoDeltaVersionKey(proj.CompareEngine, targetVer)
	if !tgtOK {
		return nil, nil
	}

	type candidate struct {
		src     autoDeltaSource
		isInt   bool
		intVal  int64
		sem     string
		created int64
	}
	var candidates []candidate
	for i := range versions {
		v := &versions[i]
		if v.ID == targetVer.ID || v.Status != model.VersionStatusPublished {
			continue
		}
		cInt, cIntVal, cSem, ok := autoDeltaVersionKey(proj.CompareEngine, v)
		if !ok {
			continue
		}
		// 比较键必须严格低于目标（只生成升级方向差量）。
		if c, comparable := autoDeltaCompare(cInt, cIntVal, cSem, tgtInt, tgtIntVal, tgtSem); !comparable || c >= 0 {
			continue
		}
		line, err := s.store.GetVersionLine(ctx, v.ID, targetLine.OS, targetLine.Arch)
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				continue // 该源版本没有此平台线
			}
			return nil, err
		}
		// 线必须 ready：pending/failed 未就绪，disabled/yanked 禁作基线（C13-5）。
		if line.Status != model.VersionLineStatusReady {
			continue
		}
		full, err := pickFullArtifactForHw(ctx, s.store, line.ID, hwRev)
		if err != nil {
			continue // 无 hw 匹配的全量产物 → 不可作基线
		}
		candidates = append(candidates, candidate{
			src:   autoDeltaSource{version: v, line: line, full: full},
			isInt: cInt, intVal: cIntVal, sem: cSem,
			created: v.CreatedAt.UnixNano(),
		})
	}

	// 比较键降序；同键按创建时间新者优先。按渠道各取前 n 个（D3：渠道 × N）。
	sort.SliceStable(candidates, func(a, b int) bool {
		ca, cb := candidates[a], candidates[b]
		if c, ok := autoDeltaCompare(ca.isInt, ca.intVal, ca.sem, cb.isInt, cb.intVal, cb.sem); ok && c != 0 {
			return c > 0
		}
		return ca.created > cb.created
	})
	byCh := make(map[string][]candidate)
	chOrder := make([]string, 0)
	for _, c := range candidates {
		slug := c.src.version.ChannelSlug
		if _, ok := byCh[slug]; !ok {
			chOrder = append(chOrder, slug)
		}
		byCh[slug] = append(byCh[slug], c)
	}
	sort.Strings(chOrder)
	out := make([]autoDeltaSource, 0, n*len(chOrder))
	for _, slug := range chOrder {
		group := byCh[slug]
		if len(group) > n {
			group = group[:n]
		}
		for _, c := range group {
			out = append(out, c.src)
		}
	}
	return out, nil
}

// runAutoDelta 执行 auto_delta 任务主体：枚举目标版本全部就绪线，对每条线
// 选择合格源并生成差量/增量包（design §1）。各线失败隔离（C13-6 / design §B3）：
// 一条线的错误记录进结果并继续处理其它线；全部线都失败时任务标记失败。
func (s *ProjectService) runAutoDelta(ctx context.Context, p autoDeltaJobPayload) (*AutoDeltaJobResult, error) {
	if s.storage == nil {
		return nil, ErrStorageUnavailable
	}
	proj, err := s.store.GetByID(ctx, p.ProjectID)
	if err != nil {
		return nil, fmt.Errorf("resolve project: %w", err)
	}
	targetVer, err := s.store.GetVersionByID(ctx, p.VersionID)
	if err != nil {
		return nil, fmt.Errorf("resolve target version: %w", err)
	}
	lines, err := s.store.ListVersionLines(ctx, targetVer.ID)
	if err != nil {
		return nil, fmt.Errorf("list target lines: %w", err)
	}

	result := &AutoDeltaJobResult{Success: true}
	failedLines := 0
	processed := false
	for i := range lines {
		line := &lines[i]
		if line.Status != model.VersionLineStatusReady {
			continue
		}
		processed = true
		lineRes := s.processAutoDeltaLine(ctx, proj, targetVer, line)
		if lineRes.Error != "" {
			failedLines++
		} else if linePreheatSucceeded(lineRes) {
			s.stampPacksReadyAt(ctx, line)
		}
		result.Lines = append(result.Lines, *lineRes)
	}

	if processed && failedLines == len(result.Lines) {
		// 全部就绪线均失败 → 任务失败（结果仍落库供排查）；部分失败 → 任务成功
		//（各线独立，失败线在 diff 消费侧自然回退全量，C13-6）。
		result.Success = false
		result.Error = "all ready lines failed"
		return result, fmt.Errorf("%s (%d lines)", result.Error, failedLines)
	}
	return result, nil
}

// processAutoDeltaLine 处理单条就绪线：按平台矩阵选择差量算法与源数量，
// 枚举目标线全部 hw 变体的全量产物，逐 (源, 变体) 生成单文件差量或多文件
// 增量 zip。返回的 LineResult 携带逐源结果；整线级错误写入 Error 字段。
func (s *ProjectService) processAutoDeltaLine(ctx context.Context, proj *model.Project, targetVer *model.Version, targetLine *model.VersionLine) *AutoDeltaLineResult {
	res := &AutoDeltaLineResult{OS: targetLine.OS, Arch: targetLine.Arch}

	// 平台矩阵：差量算法（§7.2 默认 hdiffpatch）与源数量（默认 3，§7.2）。
	matrixRow, err := s.store.GetMatrix(ctx, proj.ID, targetLine.OS, targetLine.Arch)
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		res.Error = fmt.Sprintf("load platform matrix: %v", err)
		return res
	}
	algo := model.DeltaAlgoHDiffPatch
	sourceCount := model.DefaultDeltaSourceCount
	multiFile := false
	if matrixRow != nil {
		if a := strings.ToLower(strings.TrimSpace(matrixRow.DeltaAlgo)); a != "" {
			algo = a
		}
		if matrixRow.DeltaSourceCount > 0 {
			sourceCount = matrixRow.DeltaSourceCount
		}
		multiFile = matrixRow.PackageType == model.PackageTypeMultiFile
	} else if targetLine.RootHash != "" {
		// 无矩阵行时按 RootHash 兜底判定多文件（多文件线就绪必有 RootHash）。
		multiFile = true
	}

	// 目标线全部 hw 变体的 kind=full 产物（默认变体 + 各 hw 变体）。
	targetFulls, err := s.store.ListArtifactsByLineAndKind(ctx, targetLine.ID, model.ArtifactKindFull)
	if err != nil {
		res.Error = fmt.Sprintf("list target full artifacts: %v", err)
		return res
	}
	if len(targetFulls) == 0 {
		res.Error = "target line has no full artifact"
		return res
	}

	for i := range targetFulls {
		hw := targetFulls[i].HwRev
		sources, err := s.selectAutoDeltaSources(ctx, proj, targetVer, targetLine, hw, sourceCount)
		if err != nil {
			res.Sources = append(res.Sources, AutoDeltaSourceResult{
				HwRev:  artifactHwLabel(hw),
				Status: autoDeltaSourceError,
				Error:  fmt.Sprintf("select sources: %v", err),
			})
			continue
		}
		for _, src := range sources {
			var sr AutoDeltaSourceResult
			if multiFile {
				sr = s.generateAutoPatchZip(ctx, proj, targetVer, targetLine, &targetFulls[i], src)
			} else {
				sr = s.generateAutoBinaryDelta(ctx, proj, targetVer, targetLine, &targetFulls[i], src, algo)
			}
			res.Sources = append(res.Sources, sr)
		}
	}
	return res
}

// generateAutoBinaryDelta 单文件线自动二进制差量（C13-1 / design §3）：
// 复用手动 delta_generate 的执行路径 generateDelta（同一幂等身份
// (algo, srcsha, dstsha)、同一 §5.9 文件名与产物行结构）。
func (s *ProjectService) generateAutoBinaryDelta(ctx context.Context, proj *model.Project, targetVer *model.Version, targetLine *model.VersionLine, targetFull *model.Artifact, src autoDeltaSource, algo string) AutoDeltaSourceResult {
	sr := AutoDeltaSourceResult{
		SourceVersionRef: versionDisplayRef(src.version),
		HwRev:            artifactHwLabel(targetFull.HwRev),
		ArtifactKind:     model.ArtifactKindDelta,
	}
	payload := DeltaJobPayload{
		ProjectID:       proj.ID,
		TargetVersionID: targetVer.ID,
		SourceVersionID: src.version.ID,
		OS:              targetLine.OS,
		Arch:            targetLine.Arch,
		HwRev:           targetFull.HwRev,
		Algo:            algo,
	}
	result, err := s.generateDelta(ctx, payload)
	if err != nil {
		sr.Status = autoDeltaSourceError
		sr.Error = err.Error()
		return sr
	}
	sr.Status = autoDeltaSourceCreated
	if result.Skipped {
		sr.Status = autoDeltaSourceSkipped
	}
	sr.ArtifactID = result.ArtifactID
	sr.FileName = result.FileName
	sr.Size = result.Size
	return sr
}

// generateAutoPatchZip 多文件线自动增量归档（C13-2..C13-4 / design §4）。
func (s *ProjectService) generateAutoPatchZip(ctx context.Context, proj *model.Project, targetVer *model.Version, targetLine *model.VersionLine, targetFull *model.Artifact, src autoDeltaSource) AutoDeltaSourceResult {
	sr := AutoDeltaSourceResult{
		SourceVersionRef: versionDisplayRef(src.version),
		HwRev:            artifactHwLabel(targetFull.HwRev),
		ArtifactKind:     model.ArtifactKindPatch,
	}
	srcSHA := strings.ToLower(src.full.SHA256)
	tgtSHA := strings.ToLower(targetFull.SHA256)
	hw := targetFull.HwRev

	// 幂等（C13-4）：同 (hw, srcsha, dstsha) 的 kind=patch 产物已存在 → 跳过。
	existing, err := s.store.ListArtifactsByLineAndKind(ctx, targetLine.ID, model.ArtifactKindPatch)
	if err != nil {
		sr.Status = autoDeltaSourceError
		sr.Error = fmt.Sprintf("list existing patch artifacts: %v", err)
		return sr
	}
	for i := range existing {
		e := &existing[i]
		eHw := ""
		if e.HwRev != nil {
			eHw = strings.TrimSpace(*e.HwRev)
		}
		if strings.EqualFold(eHw, sr.HwRev) &&
			strings.EqualFold(e.DeltaSourceSHA256, srcSHA) &&
			strings.EqualFold(e.DeltaTargetSHA256, tgtSHA) {
			sr.Status = autoDeltaSourceSkipped
			sr.ArtifactID = e.ID.String()
			sr.FileName = e.FileName
			sr.Size = e.Size
			return sr
		}
	}

	// Manifest diff（C13-2）：新增（源无目标有）∪ 替换（双方都有但 SHA-256 不同）；
	// KEEP 条目不进包（PRD 明文）；删除文件不进包（diff 响应 deleted_paths 送达）。
	needed, err := s.patchNeededEntries(ctx, targetLine.ID, src.line.ID)
	if err != nil {
		sr.Status = autoDeltaSourceError
		sr.Error = err.Error()
		return sr
	}
	if len(needed) == 0 {
		sr.Status = autoDeltaSourceSkipped
		sr.Error = "no added/replaced non-KEEP entries"
		return sr
	}

	tgtEntries, err := s.store.ListManifestEntries(ctx, targetLine.ID)
	if err != nil {
		sr.Status = autoDeltaSourceError
		sr.Error = fmt.Sprintf("list target manifest: %v", err)
		return sr
	}
	var neededSum, fullSum int64
	for _, e := range needed {
		neededSum += e.Size
	}
	for _, e := range tgtEntries {
		fullSum += e.Size
	}
	maxBytes := s.dynamicPackMaxBytes
	if maxBytes <= 0 {
		maxBytes = 512 * 1024 * 1024
	}
	if exceedsUncompressedPackGates(neededSum, fullSum, maxBytes) {
		sr.Status = autoDeltaSourceDiscarded
		sr.Error = fmt.Sprintf("needed uncompressed %d exceeds gates (max_bytes=%d full_manifest=%d)", neededSum, maxBytes, fullSum)
		return sr
	}

	filesetSHA := update.CanonicalFilesetSHA256(update.FilesetEntriesFromManifest(needed))

	patchBytes, err := s.buildPatchZipFromTarget(ctx, targetFull, needed)
	if err != nil {
		sr.Status = autoDeltaSourceError
		sr.Error = err.Error()
		return sr
	}

	fileName := BuildPatchStableFilename(
		proj.Slug,
		versionDisplayRef(targetVer),
		versionDisplayRef(src.version),
		targetLine.OS, targetLine.Arch, hw,
		srcSHA, tgtSHA,
	)
	artifactID := uuid.New()
	key, _, hasher, err := s.putCanonicalBytes(ctx, proj.Slug, "application/zip", patchBytes)
	if err != nil {
		sr.Status = autoDeltaSourceError
		sr.Error = fmt.Sprintf("upload patch object: %v", err)
		return sr
	}

	art := &model.Artifact{
		ID:                artifactID,
		ProjectID:         proj.ID,
		VersionID:         targetVer.ID,
		VersionLineID:     targetLine.ID,
		Kind:              model.ArtifactKindPatch,
		FileName:          fileName,
		StorageKey:        key,
		Size:              int64(len(patchBytes)),
		SHA256:            hasher.SHA256(),
		MD5:               hasher.MD5(),
		ContentType:       "application/zip",
		Compression:       model.ArtifactCompressionZip,
		FilesetSHA256:     filesetSHA,
		HwRev:             hw,
		DeltaSourceSHA256: srcSHA,
		DeltaTargetSHA256: tgtSHA,
	}
	if err := s.store.CreateArtifact(ctx, art); err != nil {
		_ = s.storage.Delete(ctx, key)
		sr.Status = autoDeltaSourceError
		sr.Error = fmt.Sprintf("persist patch artifact: %v", err)
		return sr
	}
	s.invalidateProject(ctx, proj.ID)

	sr.Status = autoDeltaSourceCreated
	sr.ArtifactID = art.ID.String()
	sr.FileName = art.FileName
	sr.Size = art.Size
	return sr
}

// patchNeededEntries 计算 patch zip 应包含的目标 Manifest 条目：
// 新增（源 Manifest 无该路径）∪ 替换（双方都有但 SHA-256 不同），
// 排除 install_policy=KEEP 条目（§7.5.2：KEEP 不进包），排除路径升序输出。
func (s *ProjectService) patchNeededEntries(ctx context.Context, targetLineID, sourceLineID uuid.UUID) ([]model.ManifestEntry, error) {
	tgtEntries, err := s.store.ListManifestEntries(ctx, targetLineID)
	if err != nil {
		return nil, fmt.Errorf("list target manifest: %w", err)
	}
	srcEntries, err := s.store.ListManifestEntries(ctx, sourceLineID)
	if err != nil {
		return nil, fmt.Errorf("list source manifest: %w", err)
	}
	srcByPath := make(map[string]model.ManifestEntry, len(srcEntries))
	for _, e := range srcEntries {
		srcByPath[e.Path] = e
	}
	needed := make([]model.ManifestEntry, 0, len(tgtEntries))
	for _, t := range tgtEntries {
		// KEEP 不进包：客户端本地已存在则保留，不存在时走全量包兜底。
		if strings.EqualFold(strings.TrimSpace(t.InstallPolicy), model.InstallPolicyKeepIfExists) {
			continue
		}
		if s, ok := srcByPath[t.Path]; ok {
			if strings.EqualFold(s.SHA256, t.SHA256) {
				continue // 未变化
			}
			// 替换：SHA-256 不同（C13-2）
		}
		needed = append(needed, t)
	}
	sort.Slice(needed, func(a, b int) bool { return needed[a].Path < needed[b].Path })
	return needed, nil
}

func exceedsUncompressedPackGates(neededSum, fullManifestSum, maxBytes int64) bool {
	return update.ExceedsUncompressedGates(neededSum, fullManifestSum, maxBytes)
}

// buildPatchZipFromTarget 从目标线哈希根目录 full zip 按文件 SHA-256 提取成员，
// 写出同样以 hex 为成员名的增量 zip。旧路径 zip 回退按 Manifest 路径提取。
func (s *ProjectService) buildPatchZipFromTarget(ctx context.Context, targetFull *model.Artifact, needed []model.ManifestEntry) ([]byte, error) {
	fullBytes, err := readStorageObject(ctx, s.storage, targetFull.StorageKey)
	if err != nil {
		return nil, fmt.Errorf("read target full artifact: %w", err)
	}
	members := make([]ArchiveMember, 0, len(needed))
	for _, e := range needed {
		body, err := ExtractHashRootMember(fullBytes, e.SHA256)
		if err != nil {
			body, err = ExtractZipMember(fullBytes, e.Path)
			if err != nil {
				return nil, fmt.Errorf("target full zip missing entry %q / %s: %w", e.Path, e.SHA256, err)
			}
		}
		members = append(members, ArchiveMember{Path: e.Path, SHA256: e.SHA256, Body: body})
	}
	return BuildHashRootZip(members)
}

// artifactHwLabel 取产物 hw 变体展示标签（nil/空白 = 默认变体，空串）。
func artifactHwLabel(hw *string) string {
	if hw == nil {
		return ""
	}
	return strings.TrimSpace(*hw)
}

// BuildPatchStableFilename 按 §5.9 构造多文件版本对增量归档的稳定文件名（C13-4）：
//
//	{project_slug}-{target_ver}-from-{source_ver}-{os}-{arch}[-{hw_rev}]-{source_sha256}-{target_sha256}.zip
//
// 源/目标 SHA-256 为双方线全量归档的完整 64 位小写 hex（不是 Root Hash）：
// 任一端字节变化 → 哈希变化 → 文件名变化 → 新对象新路径。
func BuildPatchStableFilename(projectSlug, targetRef, sourceRef, osSlug, archSlug string, hwRev *string, sourceSHA, targetSHA string) string {
	hwPart := ""
	if hwRev != nil && strings.TrimSpace(*hwRev) != "" {
		hwPart = "-" + strings.TrimSpace(*hwRev)
	}
	return fmt.Sprintf("%s-%s-from-%s-%s-%s%s-%s-%s.zip",
		projectSlug, targetRef, sourceRef, osSlug, archSlug, hwPart,
		strings.ToLower(sourceSHA), strings.ToLower(targetSHA))
}
