package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/Kirizu-Official/KiriVers/internal/delta"
	"github.com/Kirizu-Official/KiriVers/internal/model"
	"github.com/Kirizu-Official/KiriVers/internal/platform"
	"github.com/Kirizu-Official/KiriVers/internal/repository"
	"github.com/Kirizu-Official/KiriVers/internal/storage"
)

// 本文件实现单文件二进制差量的生成链路（docs/app-init.md §7.2 / §5.9）：
//
//   - 管理 API 触发：CreateDeltaJob（校验后入队 delta_generate Job，C10-7，202）；
//   - Job worker：ExecuteDeltaJob / RegisterDeltaJobHandler —— 从 storage 读取
//     源/目标全量包 → delta.Engine.Diff → 计算哈希 → §5.9 稳定文件名上传 →
//     写 kind=delta Artifact 行（含 DeltaAlgo / DeltaSourceSHA256 / DeltaTargetSHA256）；
//   - 幂等：同 (project, 目标线, algo, srcsha, dstsha) 已存在 → 跳过重新生成直接成功
//     （C10-6 逆用：字节变化 → 哈希变化 → 幂等判定不命中 → 必然生成新对象）。
//
// 24 小时请求级幂等键复用既有 Job 体系（jobs.GetByIdempotencyKey）。

// deltaGenerateJobType 是差量生成任务类型（父设计 job-type 清单内）。
const deltaGenerateJobType = "delta_generate"

// 差量生成领域错误（HTTP 映射见 controller/admin）。
var (
	// ErrDeltaAlgoUnsupported 未知差量算法 → 400 DELTA_ALGO_UNSUPPORTED。
	ErrDeltaAlgoUnsupported = errors.New("delta algo unsupported")
	// ErrDeltaSameVersion 源与目标是同一版本 → 400（无意义差量）。
	ErrDeltaSameVersion = errors.New("source and target version are identical")
	// ErrDeltaBaselineMissing 源或目标线缺少 kind=full 产物 → 404。
	ErrDeltaBaselineMissing = errors.New("source or target line has no full artifact")
)

// CreateDeltaJobInput 是管理 API 触发差量生成的输入。
// 目标版本 = 路径中的 version；源版本由 SourceVersion 指定。
type CreateDeltaJobInput struct {
	// SourceVersion 源版本引用（十进制整数或 SemVer），必填。
	SourceVersion string
	// OS / Arch 目标平台（必填；写入前规范化）。
	OS   string
	Arch string
	// Algo 差量算法；为空时取平台矩阵 delta_algo（仍为空 → hdiffpatch，§7.2 默认）。
	Algo string
	// HwRev 可选硬件代号变体（空 = 默认变体）。
	HwRev *string
	// IdempotencyKey 可选 24h 请求级幂等键（复用 Job 体系）。
	IdempotencyKey string
}

// DeltaJobPayload 是 delta_generate 任务的载荷：全部决策在入队前完成，
// worker 只按 ID 重新定位实体（实体可能在该期间变化，worker 端再次校验）。
type DeltaJobPayload struct {
	ProjectID       uuid.UUID `json:"project_id"`
	TargetVersionID uuid.UUID `json:"target_version_id"`
	SourceVersionID uuid.UUID `json:"source_version_id"`
	OS              string    `json:"os"`
	Arch            string    `json:"arch"`
	HwRev           *string   `json:"hw_rev,omitempty"`
	Algo            string    `json:"algo"`
}

// DeltaJobResult 记录差量生成任务结果（job.result）。
type DeltaJobResult struct {
	Success    bool   `json:"success"`
	Skipped    bool   `json:"skipped"` // 幂等命中：差量对象已存在，未重新生成
	ArtifactID string `json:"artifact_id,omitempty"`
	FileName   string `json:"file_name,omitempty"`
	Size       int64  `json:"size,omitempty"`
	SHA256     string `json:"sha256,omitempty"`
	Error      string `json:"error,omitempty"`
}

// versionDisplayRef 取版本展示引用（与 §5.9 全量稳定文件名一致：优先规范 SemVer，否则整数）。
func versionDisplayRef(v *model.Version) string {
	if v.VersionSemverCanonical != nil && *v.VersionSemverCanonical != "" {
		return *v.VersionSemverCanonical
	}
	if v.VersionInteger != nil {
		return fmt.Sprintf("%d", *v.VersionInteger)
	}
	return "0"
}

// DeltaExtensionForAlgo 返回差量产物扩展名（§5.9：按算法 .hdiff/.bsdiff/.vcdiff）。
func DeltaExtensionForAlgo(algo string) string {
	switch algo {
	case delta.AlgoHDiffPatch:
		return ".hdiff"
	case delta.AlgoBsdiff:
		return ".bsdiff"
	case delta.AlgoXdelta3:
		return ".vcdiff"
	default:
		return ".delta"
	}
}

// BuildDeltaStableFilename 按 §5.9 构造单文件二进制差量的稳定文件名（C10-4）：
//
//	{project_slug}-{target_ver}-from-{source_ver}-{os}-{arch}[-{hw_rev}]-{delta_algo}-{source_sha256}-{target_sha256}{.ext}
//
// 源/目标 SHA-256 必须是完整 64 位小写 hex（PRD 明确 64 位）：字节变化 → 哈希变化
// → 文件名变化 → 新对象新路径（C10-6），且同算法下不同字节对象天然分离（C10-2）。
func BuildDeltaStableFilename(projectSlug, targetRef, sourceRef, osSlug, archSlug string, hwRev *string, algo, sourceSHA, targetSHA string) string {
	hwPart := ""
	if hwRev != nil && strings.TrimSpace(*hwRev) != "" {
		hwPart = "-" + strings.TrimSpace(*hwRev)
	}
	return fmt.Sprintf("%s-%s-from-%s-%s-%s%s-%s-%s-%s%s",
		projectSlug, targetRef, sourceRef, osSlug, archSlug, hwPart, algo,
		strings.ToLower(sourceSHA), strings.ToLower(targetSHA),
		DeltaExtensionForAlgo(algo))
}

// resolveReadyFullArtifact 解析版本引用并要求 Published、指定平台线 Ready 且存在
// kind=full 产物；返回（版本, 线, 全量产物）。差量生成与 diff 下发的基线约定一致。
func (s *ProjectService) resolveReadyFullArtifact(ctx context.Context, projectID uuid.UUID, versionRef, osSlug, archSlug string, hwRev *string) (*model.Version, *model.VersionLine, *model.Artifact, error) {
	v, err := s.ResolveVersion(ctx, projectID, versionRef)
	if err != nil {
		return nil, nil, nil, err
	}
	if v.Status != model.VersionStatusPublished {
		return nil, nil, nil, ErrVersionNotFound
	}
	line, err := s.store.GetVersionLine(ctx, v.ID, osSlug, archSlug)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil, nil, ErrVersionLineNotFound
		}
		return nil, nil, nil, err
	}
	if line.Status != model.VersionLineStatusReady {
		return nil, nil, nil, ErrVersionLineNotFound
	}
	full, err := pickFullArtifactForHw(ctx, s.store, line.ID, hwRev)
	if err != nil {
		return nil, nil, nil, err
	}
	return v, line, full, nil
}

// pickFullArtifactForHw 在线的 kind=full 产物中按硬件变体挑选：
// 请求无 hw_rev → 默认变体（HwRev 为空）的产物；有 hw_rev → 精确匹配。
func pickFullArtifactForHw(ctx context.Context, store repository.ProjectStore, lineID uuid.UUID, hwRev *string) (*model.Artifact, error) {
	arts, err := store.ListArtifactsByLineAndKind(ctx, lineID, model.ArtifactKindFull)
	if err != nil {
		return nil, err
	}
	wantHw := ""
	if hwRev != nil {
		wantHw = strings.TrimSpace(*hwRev)
	}
	for i := range arts {
		artHw := ""
		if arts[i].HwRev != nil {
			artHw = strings.TrimSpace(*arts[i].HwRev)
		}
		if strings.EqualFold(artHw, wantHw) {
			return &arts[i], nil
		}
	}
	return nil, ErrDeltaBaselineMissing
}

// CreateDeltaJob 校验并创建 delta_generate 异步任务（C10-7）：
// 目标与源均 Published 且对应线就绪、双方 kind=full 产物存在、算法白名单
// （缺省取矩阵 delta_algo）、source==target 拒绝。成功返回 202 所需 job_id。
func (s *ProjectService) CreateDeltaJob(ctx context.Context, projectRef, targetVersionRef string, in CreateDeltaJobInput) (uuid.UUID, bool, error) {
	if s.jobs == nil {
		return uuid.Nil, false, fmt.Errorf("job store is not configured")
	}
	if s.storage == nil {
		return uuid.Nil, false, ErrStorageUnavailable
	}

	p, err := s.Resolve(ctx, projectRef)
	if err != nil {
		return uuid.Nil, false, err
	}

	// 平台规范化：与全量上传同一套 OS/Arch 别名规则；os/arch 必填。
	osSlug := platform.CanonicalOSWrite(strings.TrimSpace(in.OS))
	archSlug := platform.CanonicalArch(strings.TrimSpace(in.Arch))
	if osSlug == "" || archSlug == "" {
		return uuid.Nil, false, fmt.Errorf("%w: os and arch are required", ErrInvalidProjectSettings)
	}

	// 目标与源：Published + 线就绪 + kind=full 产物存在（design §4 校验清单）。
	targetVer, _, targetFull, err := s.resolveReadyFullArtifact(ctx, p.ID, targetVersionRef, osSlug, archSlug, in.HwRev)
	if err != nil {
		return uuid.Nil, false, err
	}
	sourceRef := strings.TrimSpace(in.SourceVersion)
	if sourceRef == "" {
		return uuid.Nil, false, fmt.Errorf("%w: source_version is required", ErrInvalidProjectSettings)
	}
	sourceVer, _, sourceFull, err := s.resolveReadyFullArtifact(ctx, p.ID, sourceRef, osSlug, archSlug, in.HwRev)
	if err != nil {
		return uuid.Nil, false, err
	}
	if targetVer.ID == sourceVer.ID {
		return uuid.Nil, false, ErrDeltaSameVersion
	}

	// 算法：请求显式指定 > 矩阵 delta_algo > hdiffpatch（§7.2 默认，不另造默认）。
	algo := strings.ToLower(strings.TrimSpace(in.Algo))
	if algo == "" {
		if row, _ := s.store.GetMatrix(ctx, p.ID, osSlug, archSlug); row != nil {
			algo = strings.ToLower(strings.TrimSpace(row.DeltaAlgo))
		}
		if algo == "" {
			algo = model.DeltaAlgoHDiffPatch
		}
	}
	if _, err := delta.Get(algo); err != nil {
		return uuid.Nil, false, ErrDeltaAlgoUnsupported
	}

	// hw_rev 必须已登记（与产物上传同一校验）。
	if err := s.validateHwRevs(ctx, p.ID, in.HwRev, nil, nil, nil); err != nil {
		return uuid.Nil, false, err
	}

	// 24h 请求级幂等（复用 Job 体系）。
	key := strings.TrimSpace(in.IdempotencyKey)
	if key != "" {
		if existing, err := s.jobs.GetByIdempotencyKey(ctx, p.ID, key); err == nil && existing != nil {
			return existing.ID, false, nil
		}
	}

	payload, err := json.Marshal(DeltaJobPayload{
		ProjectID:       p.ID,
		TargetVersionID: targetVer.ID,
		SourceVersionID: sourceVer.ID,
		OS:              osSlug,
		Arch:            archSlug,
		HwRev:           in.HwRev,
		Algo:            algo,
	})
	if err != nil {
		return uuid.Nil, false, err
	}

	job := &model.Job{
		ID:        uuid.New(),
		Type:      deltaGenerateJobType,
		Status:    model.JobStatusQueued,
		Payload:   payload,
		ProjectID: &p.ID,
		OwnerNodeID: s.ownerNodePtr(),
	}
	if key != "" {
		job.IdempotencyKey = &key
	}
	if err := s.jobs.Create(ctx, job); err != nil {
		return uuid.Nil, false, err
	}

	// 引用校验目标/源全量产物已取得（targetFull/sourceFull 仅作前置校验，
	// worker 按 ID 重新定位，期间字节不可变由发布不可变规则保证）。
	_ = targetFull
	_ = sourceFull
	return job.ID, true, nil
}

// RegisterDeltaJobHandler 为 JobWorker 注册 delta_generate 处理器。
// 返回非 nil 错误时由 JobWorker 统一 MarkFailed（错误信息进 error_message，
// design §4：引擎失败 → job failed，发布闸门无涉）。
func (s *ProjectService) RegisterDeltaJobHandler(w *JobWorker) {
	if w == nil {
		return
	}
	w.RegisterHandler(deltaGenerateJobType, func(ctx context.Context, jobType string, payload []byte) error {
		var p DeltaJobPayload
		if err := json.Unmarshal(payload, &p); err != nil {
			return err
		}
		_, err := s.generateDelta(ctx, p)
		return err
	})
}

// ExecuteDeltaJob 直接执行指定 delta_generate 任务（供测试或手动补偿调用）；
// 与 worker 路径不同，这里把结果 JSON 与错误一并落库。
func (s *ProjectService) ExecuteDeltaJob(ctx context.Context, jobID uuid.UUID) error {
	if s.jobs == nil {
		return fmt.Errorf("job store is not configured")
	}
	job, err := s.jobs.GetByID(ctx, jobID)
	if err != nil {
		return err
	}
	if job.Type != deltaGenerateJobType {
		return fmt.Errorf("unexpected job type: %s", job.Type)
	}
	var payload DeltaJobPayload
	if err := json.Unmarshal(job.Payload, &payload); err != nil {
		_ = s.jobs.MarkFailed(ctx, job.ID, "invalid payload json: "+err.Error())
		return err
	}
	result, genErr := s.generateDelta(ctx, payload)
	resultJSON, _ := json.Marshal(result)
	if genErr != nil {
		_ = s.jobs.MarkFailedWithResult(ctx, job.ID, genErr.Error(), resultJSON)
		return genErr
	}
	_ = s.jobs.MarkSucceededWithResult(ctx, job.ID, resultJSON)
	return nil
}

// generateDelta 执行差量生成主体：定位双端全量包 → 幂等判定 → Engine.Diff →
// 上传稳定文件名对象 → 写 kind=delta Artifact 行。
func (s *ProjectService) generateDelta(ctx context.Context, p DeltaJobPayload) (*DeltaJobResult, error) {
	if s.storage == nil {
		return nil, ErrStorageUnavailable
	}
	project, err := s.store.GetByID(ctx, p.ProjectID)
	if err != nil {
		return nil, fmt.Errorf("resolve project: %w", err)
	}
	targetVer, err := s.store.GetVersionByID(ctx, p.TargetVersionID)
	if err != nil {
		return nil, fmt.Errorf("resolve target version: %w", err)
	}
	sourceVer, err := s.store.GetVersionByID(ctx, p.SourceVersionID)
	if err != nil {
		return nil, fmt.Errorf("resolve source version: %w", err)
	}
	if targetVer.Status != model.VersionStatusPublished || sourceVer.Status != model.VersionStatusPublished {
		return nil, fmt.Errorf("%w: both versions must stay published", ErrInvalidVersionTransition)
	}

	targetLine, err := s.store.GetVersionLine(ctx, targetVer.ID, p.OS, p.Arch)
	if err != nil {
		return nil, fmt.Errorf("resolve target line: %w", err)
	}
	if targetLine.Status != model.VersionLineStatusReady {
		return nil, fmt.Errorf("target line not ready: %s", p.OS+"/"+p.Arch)
	}

	// 双端 kind=full 产物按硬件变体定位（与创建请求时的选择规则一致）。
	targetFull, err := pickFullArtifactForHw(ctx, s.store, targetLine.ID, p.HwRev)
	if err != nil {
		return nil, fmt.Errorf("locate target full artifact: %w", err)
	}
	sourceLine, err := s.store.GetVersionLine(ctx, sourceVer.ID, p.OS, p.Arch)
	if err != nil {
		return nil, fmt.Errorf("resolve source line: %w", err)
	}
	sourceFull, err := pickFullArtifactForHw(ctx, s.store, sourceLine.ID, p.HwRev)
	if err != nil {
		return nil, fmt.Errorf("locate source full artifact: %w", err)
	}

	// 幂等（C10-6 逆用）：同 (目标线, algo, srcsha, dstsha) 已存在 → 跳过生成。
	existing, err := s.store.ListArtifactsByLineAndKind(ctx, targetLine.ID, model.ArtifactKindDelta)
	if err != nil {
		return nil, err
	}
	wantHw := ""
	if p.HwRev != nil {
		wantHw = strings.TrimSpace(*p.HwRev)
	}
	for i := range existing {
		e := &existing[i]
		eHw := ""
		if e.HwRev != nil {
			eHw = strings.TrimSpace(*e.HwRev)
		}
		if !strings.EqualFold(eHw, wantHw) {
			continue
		}
		if strings.EqualFold(e.DeltaAlgo, p.Algo) &&
			strings.EqualFold(e.DeltaSourceSHA256, sourceFull.SHA256) &&
			strings.EqualFold(e.DeltaTargetSHA256, targetFull.SHA256) {
			return &DeltaJobResult{
				Success:    true,
				Skipped:    true,
				ArtifactID: e.ID.String(),
				FileName:   e.FileName,
				Size:       e.Size,
				SHA256:     e.SHA256,
			}, nil
		}
	}

	// 读取双端全量字节：单文件差量基线即全量文件（design §4）。
	oldData, err := readStorageObject(ctx, s.storage, sourceFull.StorageKey)
	if err != nil {
		return nil, fmt.Errorf("read source full artifact: %w", err)
	}
	newData, err := readStorageObject(ctx, s.storage, targetFull.StorageKey)
	if err != nil {
		return nil, fmt.Errorf("read target full artifact: %w", err)
	}

	engine, err := delta.Get(p.Algo)
	if err != nil {
		return nil, ErrDeltaAlgoUnsupported
	}
	deltaBytes, err := engine.Diff(oldData, newData)
	if err != nil {
		return nil, fmt.Errorf("engine %s diff: %w", p.Algo, err)
	}

	// §5.9 稳定文件名：完整 64 位源/目标 SHA-256（C10-4）。
	fileName := BuildDeltaStableFilename(
		project.Slug,
		versionDisplayRef(targetVer),
		versionDisplayRef(sourceVer),
		p.OS, p.Arch, p.HwRev, p.Algo,
		sourceFull.SHA256, targetFull.SHA256,
	)

	artifactID := uuid.New()
	key, _, hasher, err := s.putCanonicalBytes(ctx, project.Slug, "application/octet-stream", deltaBytes)
	if err != nil {
		return nil, fmt.Errorf("upload delta object: %w", err)
	}

	art := &model.Artifact{
		ID:                artifactID,
		ProjectID:         p.ProjectID,
		VersionID:         targetVer.ID,
		VersionLineID:     targetLine.ID,
		Kind:              model.ArtifactKindDelta,
		FileName:          fileName,
		StorageKey:        key,
		Size:              int64(len(deltaBytes)),
		SHA256:            hasher.SHA256(),
		MD5:               hasher.MD5(),
		ContentType:       "application/octet-stream",
		HwRev:             p.HwRev,
		DeltaAlgo:         p.Algo,
		DeltaSourceSHA256: sourceFull.SHA256,
		DeltaTargetSHA256: targetFull.SHA256,
	}
	if err := s.store.CreateArtifact(ctx, art); err != nil {
		_ = s.storage.Delete(ctx, key)
		return nil, fmt.Errorf("persist delta artifact: %w", err)
	}
	s.invalidateProject(ctx, p.ProjectID)

	return &DeltaJobResult{
		Success:    true,
		ArtifactID: art.ID.String(),
		FileName:   art.FileName,
		Size:       art.Size,
		SHA256:     art.SHA256,
	}, nil
}

// readStorageObject 从 storage 后端读取整个对象（差量生成直接面向全量文件字节）。
func readStorageObject(ctx context.Context, b storage.Backend, key string) ([]byte, error) {
	rc, err := b.Get(ctx, key)
	if err != nil {
		return nil, err
	}
	defer rc.Close()
	return io.ReadAll(rc)
}
