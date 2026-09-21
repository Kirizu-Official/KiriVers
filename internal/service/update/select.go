package update

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strconv"
	"strings"

	"github.com/google/uuid"

	"github.com/Kirizu-Official/KiriVers/internal/model"
)

// 选目标结果 reason 枚举（task design §3）。
const (
	ReasonNormal                  = "normal"
	ReasonMinimumSupportedVersion = "minimum_supported_version"
	ReasonCriticalVersion         = "critical_version"
	ReasonRevoked                 = "revoked"
	ReasonYanked                  = "yanked"
	ReasonIntermediate            = "intermediate"
)

// maxRelayHops 中继链最大跳数（§5.5.1 建议默认 8，超过视为配置错误）。
const maxRelayHops = 8

var (
	// ErrNoSafeTarget 吊销/yank 分支找不到安全目标 → HTTP 409 NO_SAFE_TARGET
	//（响应禁止 public 缓存）。
	ErrNoSafeTarget = errors.New("no safe update target")
	// ErrIntermediateUnavailable 中继版本缺失、未发布、未就绪、成环或超过跳数
	// → HTTP 409 INTERMEDIATE_UNAVAILABLE。
	ErrIntermediateUnavailable = errors.New("intermediate version unavailable")
	// ErrMinOSNotMet 请求带 os_version 且当前版本自身在本平台的 min_os 未满足
	// → HTTP 409 MIN_OS_NOT_MET。候选因 min_os 被排除则是静默跳过。
	ErrMinOSNotMet = errors.New("client os version below the minimum of current line")
)

// Request 是一次选目标的请求侧输入；OS/Arch/HwRev 均为已规范化值。
type Request struct {
	// Current 是已解析的当前版本状态（service 层保证非空）。
	Current *VersionState
	// OS / Arch 是已规范化的请求平台。
	OS, Arch string
	// HwRev 为空串表示默认变体（C08-10：只匹配 HwRev=nil 的产物）。
	HwRev string
	// OSVersion 可选；仅当传入时才做 min_os / min_api_level 比较。
	OSVersion string
	// ChannelQuery 是 check 的 channel query（存在性已在 Check 校验）；
	// 用于允许升入不可见渠道。
	ChannelQuery string
	// ChannelToken 是 X-Channel-Token 明文；永不进入 ETag。
	ChannelToken string
	// DeviceID 是明文原始设备号；空串=匿名。仅参与灰度判定，
	// 不进入 ETag、日志或任何落库字段。
	DeviceID string
	// DeviceHash 是 HTTP 层在身份解析处按 service.HashDeviceID 语义计算一次
	// 的设备标识（hashed 策略 = HMAC hex；raw 策略下白名单比较改用原样
	// DeviceID，见 grayHitFor/allowlistKey）。空 = 匿名或 none 策略。
	DeviceHash string
}

// Result 是选目标结果。Target 为 nil 表示无更新（HTTP 204）。
type Result struct {
	Target      *VersionState
	Line        *LineState
	Package     *ArtifactInfo // 目标线匹配 hw_rev 变体的 full 包
	IsMandatory bool
	IsDowngrade bool
	Reason      string // normal | minimum_supported_version | critical_version | revoked | yanked | intermediate
	// DeltaAllowed 表示该结果允许差量（非降级分支）；最终 delta_available
	// 还需 check 层按客户端能力收敛。
	DeltaAllowed bool
	RelayHops    int
	// UsedFallbackArch 表示目标是通过矩阵 fallback_arch 在
	// (os, fallback_arch) 平台上选出的；此时 FallbackMatrix 提供该平台的
	// package_type / delta_algo。
	UsedFallbackArch bool
	FallbackMatrix   *MatrixInfo
}

// SelectTarget 是选目标唯一实现（§4.4 / §4.4.1 / §5.3–5.6）。
//
// 步骤：
//  1. 当前 Version revoked 或本平台 Line yanked → 降级分支（允许比较键更低、
//     忽略灰度、不受 min_source 阻挡）；
//  2. 正常升级：候选 = Published + 渠道 rank ≥ 当前版本渠道（锚定 Version 记录，
//     非 channel 参数）+ 本平台 Line ready + hw 变体匹配 +（带 os_version 时）
//     min_os/min_api_level 不挡；
//  3. 丢掉比较键 ≤ current 的候选；
//  4. 逐候选做强制/灰度判定后取比较键最高者（同键取 stability_rank 高者）；
//  5. 中继截断（§5.5.1，≤8 跳，成环报 ErrIntermediateUnavailable）；
//  6. 正常分支无更新且矩阵声明 fallback_arch 时，以 fallback 架构重跑一次；
//  7. 仍无候选 → Target=nil（204）。
func SelectTarget(cat *Catalog, req Request) (Result, error) {
	curLine := req.Current.Line(req.OS, req.Arch)

	// 步骤 1：降级分支判定。
	if req.Current.Version.Status == model.VersionStatusRevoked ||
		(curLine != nil && curLine.Status == model.VersionLineStatusYanked) {
		return selectDowngrade(cat, req)
	}

	// MIN_OS_NOT_MET：仅当当前版本自身在本平台的 Line 就绪且其 min_os 未满足
	// 时发出；候选级 min_os 失败在候选集内静默跳过（task design §11 裁决）。
	if req.OSVersion != "" && curLine != nil && curLine.Status == model.VersionLineStatusReady {
		if !lineMeetsOS(curLine, req.OSVersion) {
			return Result{}, ErrMinOSNotMet
		}
	}

	// 步骤 2–5：本平台正常升级。
	res, err := selectUpgrade(cat, req, req.Arch, cat.Matrix)
	if err != nil {
		return Result{}, err
	}
	if res.Target != nil {
		return res, nil
	}

	// 步骤 6：fallback_arch（C08-10）。仅当本 arch 无更新、矩阵声明了
	// fallback 且与其不同、且 (os, fallback) 矩阵行存在时重跑候选集。
	if m := cat.Matrix; m != nil && m.FallbackArch != "" && m.FallbackArch != req.Arch && cat.FallbackMatrix != nil {
		res, err := selectUpgrade(cat, req, m.FallbackArch, cat.FallbackMatrix)
		if err != nil {
			return Result{}, err
		}
		if res.Target != nil {
			res.UsedFallbackArch = true
			res.FallbackMatrix = cat.FallbackMatrix
			return res, nil
		}
	}

	return Result{}, nil
}

// upgradeCandidate 是正常升级分支中的单个可行候选。
type upgradeCandidate struct {
	vs        *VersionState
	line      *LineState
	pkg       *ArtifactInfo
	key       cmpKey
	rank      int
	mandatory bool
	reason    string
}

// selectUpgrade 在 (os, arch) 平台上执行正常升级选择（步骤 2–5）。
// Target 为 nil 表示该平台无更新；中继链配置错误返回 ErrIntermediateUnavailable。
func selectUpgrade(cat *Catalog, req Request, arch string, matrix *MatrixInfo) (Result, error) {
	engine := cat.Project.CompareEngine
	curKey, ok := versionKey(engine, &req.Current.Version)
	if !ok {
		// 当前版本缺引擎要求的比较键：无法安全比较，视为无更新。
		return Result{}, nil
	}
	curRank := cat.channelRank(req.Current.Version.ChannelSlug)

	var candidates []upgradeCandidate
	for i := range cat.Versions {
		vs := &cat.Versions[i]
		v := &vs.Version
		if v.Status != model.VersionStatusPublished || v.ID == req.Current.Version.ID {
			continue
		}
		ch, found := cat.Channel(v.ChannelSlug)
		if !found || !ch.Enabled {
			continue
		}
		if !channelAllowedForUpgrade(cat, req, v.ChannelSlug) {
			continue
		}
		// 禁止向下跨渠道：候选渠道 stability_rank 不得低于当前版本的渠道。
		if ch.StabilityRank < curRank {
			continue
		}
		line := vs.Line(req.OS, arch)
		if !LinePacksReady(line) {
			continue
		}
		pkg := matchHwVariant(cat, matrix, line, req.HwRev)
		if pkg == nil {
			continue
		}
		key, keyOK := versionKey(engine, v)
		if !keyOK || lessOrEqualKey(key, curKey) {
			// 步骤 3：丢掉比较键不高于当前的候选
			//（无本平台就绪产物的更高 Version 不会进入候选）。
			continue
		}
		if req.OSVersion != "" && !lineMeetsOS(line, req.OSVersion) {
			continue
		}
		mandatory, reason := mandatoryFor(cat, req, vs, matrix, curKey)
		// 非强制候选套用灰度命中判定（§5.4 / C12）：强制路径已在
		// mandatoryFor 命中并短路，天然忽略灰度与白名单（验收项 4）。
		if !mandatory && !grayHitFor(cat, vs, line, grayDevice{DeviceHash: req.DeviceHash, RawDevice: req.DeviceID}) {
			continue
		}
		candidates = append(candidates, upgradeCandidate{
			vs: vs, line: line, pkg: pkg, key: key,
			rank: ch.StabilityRank, mandatory: mandatory, reason: reason,
		})
	}
	if len(candidates) == 0 {
		return Result{}, nil
	}

	// 步骤 4 + 5：取比较键最高者。被 min_source 阻挡时沿该门槛中继，
	// 不得跳过被挡的最新版去选次高候选。
	sortCandidatesDesc(candidates)
	winner := candidates[0]
	if blocked, _ := candidateBlockedByMinSource(engine, &winner, curKey); !blocked {
		return candidateResult(winner, 0, false), nil
	}
	return hopViaMinSource(cat, req, arch, matrix, winner, curKey)
}

// candidateBlockedByMinSource 判断候选的 min_source_version 是否阻挡当前
// 客户端直升。第二返回值表示该配置是否有效（解析失败或与引擎形态不齐
// 视为未配置 → 不阻挡）。
func candidateBlockedByMinSource(engine string, cand *upgradeCandidate, curKey cmpKey) (blocked, configured bool) {
	raw := deref(cand.vs.Version.MinSourceVersion)
	if strings.TrimSpace(raw) == "" {
		return false, false
	}
	ref, ok := parseVersionRef(raw)
	if !ok {
		return false, false
	}
	floor, ok := refKey(engine, ref)
	if !ok {
		return false, false
	}
	c, comparable := compareKeys(curKey, floor)
	if !comparable {
		return false, false
	}
	return c < 0, true // current < min_source → 阻挡
}

// hopViaMinSource 沿 min_source_version 把目标改写为客户端可取的第一跳
// （D3）：中继必须 Published、本平台就绪、hw 兼容、未被 D7 跳过；
// 中继自身仍被 min_source 阻挡则继续跳其 min_source，最多 8 跳。
func hopViaMinSource(cat *Catalog, req Request, arch string, matrix *MatrixInfo, winner upgradeCandidate, curKey cmpKey) (Result, error) {
	engine := cat.Project.CompareEngine
	visited := map[uuid.UUID]bool{req.Current.Version.ID: true, winner.vs.Version.ID: true}
	hops := 0
	target := winner
	for {
		ref, ok := parseVersionRef(deref(target.vs.Version.MinSourceVersion))
		if !ok {
			return Result{}, ErrIntermediateUnavailable
		}
		hops++
		if hops > maxRelayHops {
			return Result{}, ErrIntermediateUnavailable
		}
		inter := findVersionState(cat, ref)
		if inter == nil || inter.Version.Status != model.VersionStatusPublished || visited[inter.Version.ID] {
			return Result{}, ErrIntermediateUnavailable
		}
		if !channelAllowedForUpgrade(cat, req, inter.Version.ChannelSlug) {
			return Result{}, ErrIntermediateUnavailable
		}
		line := inter.Line(req.OS, arch)
		if !LinePacksReady(line) {
			return Result{}, ErrIntermediateUnavailable
		}
		if req.OSVersion != "" && !lineMeetsOS(line, req.OSVersion) {
			return Result{}, ErrIntermediateUnavailable
		}
		pkg := matchHwVariant(cat, matrix, line, req.HwRev)
		if pkg == nil {
			return Result{}, ErrIntermediateUnavailable
		}
		visited[inter.Version.ID] = true
		key, keyOK := versionKey(engine, &inter.Version)
		if !keyOK {
			return Result{}, ErrIntermediateUnavailable
		}
		next := upgradeCandidate{vs: inter, line: line, pkg: pkg, key: key, rank: cat.channelRank(inter.Version.ChannelSlug)}
		blocked, _ := candidateBlockedByMinSource(engine, &next, curKey)
		target = next
		if !blocked {
			res := candidateResult(target, hops, true)
			if c, comparable := compareKeys(target.key, curKey); comparable {
				res.IsDowngrade = c < 0
			}
			return res, nil
		}
	}
}

// candidateResult 组装单个胜出候选的 Result。正常与中继结果均允许差量
// （非降级）；最终 delta_available 由 check 层按客户端能力收敛。
func candidateResult(cand upgradeCandidate, hops int, relayed bool) Result {
	res := Result{
		Target:       cand.vs,
		Line:         cand.line,
		Package:      cand.pkg,
		IsMandatory:  cand.mandatory,
		Reason:       cand.reason,
		DeltaAllowed: true,
		RelayHops:    hops,
	}
	if res.Reason == "" {
		res.Reason = ReasonNormal
	}
	if relayed {
		res.Reason = ReasonIntermediate
		res.IsMandatory = true
	}
	return res
}

// selectDowngrade 是吊销/yank 降级分支（§4.4.1 / §5.6）：
// 解除「必须高于当前」限制，允许比较键更低的安全目标；忽略灰度；
// 不受 min_source_version 阻挡。选目标顺序：同渠道最新安全版 → 当前 LTS →
// 更稳渠道。全空 → ErrNoSafeTarget。
func selectDowngrade(cat *Catalog, req Request) (Result, error) {
	engine := cat.Project.CompareEngine
	curKey, ok := versionKey(engine, &req.Current.Version)
	if !ok {
		return Result{}, ErrNoSafeTarget
	}
	curRank := cat.channelRank(req.Current.Version.ChannelSlug)

	// reason 取决于触发方式：Version revoked 优先，其次本平台 Line yanked。
	reason := ReasonRevoked
	if req.Current.Version.Status != model.VersionStatusRevoked {
		reason = ReasonYanked
	}

	// pickFrom 在满足 filter 的版本中取比较键最高者（同键取渠道 rank 高者）。
	pickFrom := func(filter func(vs *VersionState, ch ChannelInfo) bool) *upgradeCandidate {
		var best *upgradeCandidate
		for i := range cat.Versions {
			vs := &cat.Versions[i]
			v := &vs.Version
			if v.Status != model.VersionStatusPublished || v.ID == req.Current.Version.ID {
				continue
			}
			ch, found := cat.Channel(v.ChannelSlug)
			if !found || !ch.Enabled || !filter(vs, ch) {
				continue
			}
			if !channelAllowedForUpgrade(cat, req, v.ChannelSlug) {
				continue
			}
			line := vs.Line(req.OS, req.Arch)
			if !LinePacksReady(line) {
				continue
			}
			pkg := matchHwVariant(cat, cat.Matrix, line, req.HwRev)
			if pkg == nil {
				continue
			}
			key, keyOK := versionKey(engine, v)
			if !keyOK {
				continue
			}
			cand := &upgradeCandidate{vs: vs, line: line, pkg: pkg, key: key, rank: ch.StabilityRank}
			if best == nil || candidateGreater(*cand, *best) {
				best = cand
			}
		}
		return best
	}

	// 1) 同渠道最新安全版。
	curChannel := req.Current.Version.ChannelSlug
	best := pickFrom(func(vs *VersionState, ch ChannelInfo) bool { return ch.Slug == curChannel })
	// 2) 当前 LTS（跨渠道：已发布、未吊销、is_lts=true 中最高者）。
	if best == nil {
		best = pickFrom(func(vs *VersionState, ch ChannelInfo) bool { return vs.Version.IsLTS })
	}
	// 3) 更稳渠道（rank 高于当前渠道，按 §4.4 候选规则）。
	if best == nil {
		best = pickFrom(func(vs *VersionState, ch ChannelInfo) bool { return ch.StabilityRank > curRank })
	}
	if best == nil {
		return Result{}, ErrNoSafeTarget
	}

	res := candidateResult(*best, 0, false)
	res.IsMandatory = true
	res.Reason = reason
	res.DeltaAllowed = false // 降级强制全量，禁差量（不得以已吊销/已 yank 基线做差量）
	if c, comparable := compareKeys(best.key, curKey); comparable {
		res.IsDowngrade = c < 0
	}
	return res, nil
}

// mandatoryFor 计算「选中 candidate 后本次响应是否强制」（§5.4/§5.5）。
// 强制则忽略灰度，视为灰度命中：
//  1. 比较键 floor（矩阵行覆盖优先于项目 minimum_supported_version）高于当前；
//  2. (current, candidate] 路径存在本平台曾就绪的关键 Version。
func mandatoryFor(cat *Catalog, req Request, cand *VersionState, matrix *MatrixInfo, curKey cmpKey) (bool, string) {
	engine := cat.Project.CompareEngine

	// 1) 最低支持 floor：矩阵覆盖优先。
	floorRaw := ""
	if matrix != nil && matrix.MinimumSupportedVersion != nil && strings.TrimSpace(*matrix.MinimumSupportedVersion) != "" {
		floorRaw = *matrix.MinimumSupportedVersion
	} else if cat.Project.MinimumSupportedVersion != nil {
		floorRaw = *cat.Project.MinimumSupportedVersion
	}
	if strings.TrimSpace(floorRaw) != "" {
		if ref, ok := parseVersionRef(floorRaw); ok {
			if floor, ok := refKey(engine, ref); ok {
				if c, comparable := compareKeys(floor, curKey); comparable && c > 0 {
					return true, ReasonMinimumSupportedVersion
				}
			}
		}
	}

	// 2) 路径上的关键 Version（C08-13）：仅计入本平台曾就绪的
	//（仅对其他平台就绪过的关键 Version 不使本平台变强制）。
	candKey, ok := versionKey(engine, &cand.Version)
	if !ok {
		return false, ""
	}
	for i := range cat.Versions {
		v := &cat.Versions[i]
		if !v.Version.IsCritical || v.Version.Status == model.VersionStatusDraft {
			continue
		}
		key, keyOK := versionKey(engine, &v.Version)
		if !keyOK {
			continue
		}
		lo, okLo := compareKeys(curKey, key)
		hi, okHi := compareKeys(key, candKey)
		if okLo && okHi && lo < 0 && hi <= 0 && everReadyOnPlatform(v, req.OS, req.Arch) {
			return true, ReasonCriticalVersion
		}
	}
	return false, ""
}

// everReadyOnPlatform 判断版本在本平台是否「曾经就绪」：
// 存在 status ∈ {ready, yanked, disabled} 的 Line 记录即算
// （yanked/disabled 说明当时就绪过，后被撤下）。
func everReadyOnPlatform(vs *VersionState, os, arch string) bool {
	line := vs.Line(os, arch)
	if line == nil {
		return false
	}
	switch line.Status {
	case model.VersionLineStatusReady, model.VersionLineStatusYanked, model.VersionLineStatusDisabled:
		return true
	}
	return false
}

// sortCandidatesDesc 按比较键降序；同键渠道 stability_rank 高者在前。
func sortCandidatesDesc(candidates []upgradeCandidate) {
	for i := 1; i < len(candidates); i++ {
		for j := i; j > 0 && candidateGreater(candidates[j], candidates[j-1]); j-- {
			candidates[j], candidates[j-1] = candidates[j-1], candidates[j]
		}
	}
}

// candidateGreater 判断 a 是否应排在 b 之前（键更高，或同键且 rank 更高）。
// 键形态一致才比较；不一致保持原序（同项目内 Published 版本恒同形态）。
func candidateGreater(a, b upgradeCandidate) bool {
	if c, ok := compareKeys(a.key, b.key); ok {
		if c != 0 {
			return c > 0
		}
		return a.rank > b.rank
	}
	return false
}

// lessOrEqualKey 判断 key 是否 ≤ cur（同形态才比较；形态不齐按不高于处理，
// 保守丢弃候选避免越权升级）。
func lessOrEqualKey(key, cur cmpKey) bool {
	c, ok := compareKeys(key, cur)
	if !ok {
		return true
	}
	return c <= 0
}

// findVersionState 在目录中按解析后的引用定位版本（整数或规范 SemVer）。
func findVersionState(cat *Catalog, ref versionRef) *VersionState {
	for i := range cat.Versions {
		v := &cat.Versions[i].Version
		if ref.isInteger {
			if v.VersionInteger != nil && *v.VersionInteger == ref.integer {
				return &cat.Versions[i]
			}
		} else if v.VersionSemverCanonical != nil && *v.VersionSemverCanonical == ref.semver {
			return &cat.Versions[i]
		}
	}
	return nil
}

// ResolveCurrent 按原生 check 完全相同的规则解析 current_version 引用
// （整数构建号或规范 SemVer，见 parseVersionRef）并在目录中定位对应版本。
//
// 导出供商店协议 feed 适配器复用（Tauri 动态端点 {{target}}/{{arch}}/
// {{current_version}} 的当前版本解析），保证 feed 与原生 check 的版本解析
// 永不漂移（C08-2：未知版本绝不视为 0）。
//
//   - 引用格式非法 → ErrEngineMismatch（原生 check → 400 ENGINE_MISMATCH）；
//   - 引用合法但目录中不存在 → ErrVersionNotFound（原生 check → 404）。
func ResolveCurrent(cat *Catalog, currentVersion string) (*VersionState, error) {
	ref, ok := parseVersionRef(currentVersion)
	if !ok {
		return nil, ErrEngineMismatch
	}
	current := findVersionState(cat, ref)
	if current == nil {
		return nil, ErrVersionNotFound
	}
	return current, nil
}

// deref 安全取字符串指针内容。
func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// lineMeetsOS 判定请求 os_version 是否满足线的最低系统要求：
// 只使用线的 MinOS 与 MinAPILevel（二者都设时 AND）。无矩阵回退。
func lineMeetsOS(line *LineState, osVersion string) bool {
	if line == nil {
		return true
	}
	if line.MinOS != nil && strings.TrimSpace(*line.MinOS) != "" {
		if !osVersionAtLeast(osVersion, *line.MinOS) {
			return false
		}
	}
	if line.MinAPILevel != nil && *line.MinAPILevel > 0 {
		api, err := strconv.Atoi(strings.TrimSpace(osVersion))
		if err != nil {
			return true
		}
		if api < *line.MinAPILevel {
			return false
		}
	}
	return true
}

// channelAllowedForUpgrade 判断渠道是否可作为本次升级目标（D7）。
// 关停渠道已在候选循环排除；此处处理不可见与 Token。
func channelAllowedForUpgrade(cat *Catalog, req Request, slug string) bool {
	ch, found := cat.Channel(slug)
	if !found || !ch.Enabled {
		return false
	}
	if ch.Unlisted {
		curSlug := ""
		if req.Current != nil {
			curSlug = req.Current.Version.ChannelSlug
		}
		if curSlug != slug && strings.TrimSpace(req.ChannelQuery) != slug {
			return false
		}
	}
	if ch.TokenProtected {
		got := sha256Hex(strings.TrimSpace(req.ChannelToken))
		if !hmac.Equal([]byte(ch.TokenHash), []byte(got)) {
			return false
		}
	}
	return true
}

func sha256Hex(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

// matchHwVariant 从线的 full 产物中挑选与请求 hw_rev 兼容的变体（§2.3）。
//
// 匹配规则：
//   - 请求 hw_rev 为空（客户端不传）：只匹配默认变体（HwRev=nil）；
//   - 请求 hw_rev=X：优先精确变体（HwRev=X）；higher_compatible_with_lower
//     策略下允许 rank(X) ≤ rank(产物) 的覆盖（收窄字段以 rank 比较）；
//     最后回落默认变体（与下载端点口径一致：默认变体对所有客户端可用）；
//   - 无任何兼容变体 → 该 Version 对该设备不存在。
func matchHwVariant(cat *Catalog, matrix *MatrixInfo, line *LineState, hwRev string) *ArtifactInfo {
	if line == nil || len(line.FullPkgs) == 0 {
		return nil
	}
	hwRev = strings.TrimSpace(hwRev)

	// 客户端不传 hw_rev：只匹配默认变体（C08-10）。
	if hwRev == "" {
		for i := range line.FullPkgs {
			if isDefaultHwRev(line.FullPkgs[i].HwRev) {
				return &line.FullPkgs[i]
			}
		}
		return nil
	}

	// 1) 精确变体。
	for i := range line.FullPkgs {
		p := &line.FullPkgs[i]
		if p.HwRev != nil && strings.TrimSpace(*p.HwRev) == hwRev && hwNarrowingOK(cat, p, hwRev) {
			return p
		}
	}

	// 2) higher_compatible_with_lower：高 rank 产物覆盖低 rank 设备。
	if matrix != nil && matrix.HwVariantPolicy == model.HwVariantHigherCompatible {
		if deviceRank, hasRank := cat.HwRevRanks[hwRev]; hasRank {
			var best *ArtifactInfo
			for i := range line.FullPkgs {
				p := &line.FullPkgs[i]
				if isDefaultHwRev(p.HwRev) {
					continue
				}
				artRank, ok := cat.HwRevRanks[strings.TrimSpace(*p.HwRev)]
				if !ok || artRank < deviceRank {
					continue
				}
				if !hwNarrowingOK(cat, p, hwRev) {
					continue
				}
				if best == nil || artRank > cat.HwRevRanks[strings.TrimSpace(*best.HwRev)] {
					best = p
				}
			}
			if best != nil {
				return best
			}
		}
	}

	// 3) 默认变体回落（与下载端点一致：HwRev=nil 的产物对所有客户端可用）。
	for i := range line.FullPkgs {
		if isDefaultHwRev(line.FullPkgs[i].HwRev) {
			return &line.FullPkgs[i]
		}
	}
	return nil
}

// isDefaultHwRev 判断产物是否为默认（无硬件代号）变体。
func isDefaultHwRev(hw *string) bool {
	return hw == nil || strings.TrimSpace(*hw) == ""
}

// hwNarrowingOK 校验产物上的 min_hw_rev / max_hw_rev / compatible_hw_revs
// 收窄字段（以 hw_revs.rank 比较）。设备 hw_rev 未登记时，任何收窄字段
// 均视为不匹配（不得凭猜测拿到兼容范围外的变体，§13.9）。
func hwNarrowingOK(cat *Catalog, art *ArtifactInfo, deviceHw string) bool {
	if isDefaultHwRev(art.HwRev) {
		// 默认变体不参与收窄（与下载端点口径一致）。
		return true
	}
	deviceRank, hasRank := cat.HwRevRanks[deviceHw]

	if len(art.CompatibleHwRevs) > 0 {
		found := false
		for _, c := range art.CompatibleHwRevs {
			if strings.TrimSpace(c) == deviceHw {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	if art.MinHwRev != nil && strings.TrimSpace(*art.MinHwRev) != "" {
		minRank, ok := cat.HwRevRanks[strings.TrimSpace(*art.MinHwRev)]
		if !ok || !hasRank || deviceRank < minRank {
			return false
		}
	}
	if art.MaxHwRev != nil && strings.TrimSpace(*art.MaxHwRev) != "" {
		maxRank, ok := cat.HwRevRanks[strings.TrimSpace(*art.MaxHwRev)]
		if !ok || !hasRank || deviceRank > maxRank {
			return false
		}
	}
	return true
}
