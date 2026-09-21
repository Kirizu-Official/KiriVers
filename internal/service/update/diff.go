package update

import (
	"context"
	"sort"
	"strconv"
	"strings"

	"github.com/google/uuid"

	"github.com/Kirizu-Official/KiriVers/internal/model"
	"github.com/Kirizu-Official/KiriVers/pkg/signature"
)

// diff 具名阈值常量（docs/app-init.md §7.4 / §7.5）。
// 任何调整都必须同步产品文档；禁止在逻辑中出现裸数字。
const (
	// PatchPackageMaxSizeRatio D7：needed Manifest size 合计 ≥ 全量 Manifest
	// 未压缩合计的 70% → 改全量（永不读 kind=full zip Size）。
	PatchPackageMaxSizeRatio = 0.70
)

// diff_mode 取值（§7.5 / §10.3）。
const (
	// DiffModeFullPackage 目标线已有全量归档（默认兜底，唯一会产生写请求体的形态）。
	DiffModeFullPackage = "full_package"
	// DiffModePatchPackage 版本对预生成差量归档（多文件；任务 13 生成，本任务只消费）。
	DiffModePatchPackage = "patch_package"
	// DiffModeBinaryDelta 单文件二进制差量（任务 13 生成，本任务只消费）。
	DiffModeBinaryDelta = "binary_delta"
	// DiffModeFileList file_list 例外：逐文件 URL 数组（仅显式声明且不超过生效上限）。
	DiffModeFileList = "file_list"
)

// delta 产物消费形态（DeltaArtifactInfo.Kind）。
const (
	DeltaKindPatchPackage = "patch_package"
	DeltaKindBinaryDelta  = "binary_delta"
)

// DiffInput 是 POST update/diff 的请求参数（原始 JSON 已由 HTTP 层绑定）。
type DiffInput struct {
	// SourceVersion / TargetVersion / OS / Arch 必填（OS/Arch 已规范化）。
	SourceVersion string
	TargetVersion string
	OS            string
	Arch          string
	// Channel 可选；传入则必须等于目标 Version 渠道（与 integrity 同规则）。
	Channel string
	// DeviceID 仅参与灰度判定（C09-11），不进日志、不建会话缓存。
	DeviceID string
	// DeviceHash 是按项目策略处理后的设备标识（HTTP 层经遥测服务计算）；
	// 用于限流键、连续失败降级查询与灰度白名单匹配（C12，见 grayHitFor）。
	// 空 = 匿名或 none 策略。
	DeviceHash string
	// DeviceDowngradeActive 由 Service 编排层查询降级读模型后置位（C11-8）：
	// 生效时在灰度闸之后、差量选择之前短路为 full_package。
	DeviceDowngradeActive bool
	// HwRev 可选硬件代号。
	HwRev string
	// LocalSHA256 单文件本地基线哈希（binary_delta 前提：必须等于 source 官方哈希）。
	LocalSHA256 string
	// AcceptedDeltaAlgos 客户端可接受的差量算法（取交集，优先矩阵默认算法）。
	AcceptedDeltaAlgos []string
	// PreferFull 客户端显式要求全量（§7.1 回退条件之一）。
	PreferFull bool
	// Capabilities 显式能力声明（patch_package / binary_delta / file_list）。
	Capabilities []string

	// signing 由 Service 包装层按项目可见性注入（私有项目签名，§13.7 / C15-1）：
	// 覆盖 package_url 与 file_list 逐文件 URL。
	signing urlSigning
}

// DiffFile 是 file_list 例外下的逐文件条目（§7.5.4）；仅显式声明 file_list
// 能力且待下载数不超过目录生效上限时出现在响应中。
type DiffFile struct {
	Path   string `json:"path"`
	Size   int64  `json:"size"`
	SHA256 string `json:"sha256,omitempty"`
	MD5    string `json:"md5,omitempty"`
	URL    string `json:"url"`
}

// DiffResponse 是 200 响应体（§10.3 / C09-3）。
type DiffResponse struct {
	DiffMode string `json:"diff_mode"`
	// PackageURL / Size / SHA256 单条包三元组：full_package 取目标线 kind=full
	// 产物（与 check 200 完全同源，一致性由同一 artifact 行保证）；patch/binary_delta
	// 取匹配的差量产物；file_list 模式下缺省。
	PackageURL string `json:"package_url,omitempty"`
	FileName   string `json:"file_name,omitempty"`
	Size       int64  `json:"size,omitempty"`
	SHA256     string `json:"sha256,omitempty"`
	// DeltaAlgo 仅 binary_delta 模式输出。
	DeltaAlgo string `json:"delta_algo,omitempty"`
	// DeletedPaths 是 source Manifest 有而 target Manifest 无的路径（仅多文件；
	// 单文件恒空；脏路径场景按设计不查 Manifest diff，恒空）。
	DeletedPaths []string `json:"deleted_paths,omitempty"`
	// Files 仅 file_list 例外模式输出；默认恒缺省（验收项）。
	Files []DiffFile `json:"files,omitempty"`
	// InvalidPaths 是归一化失败被忽略的原始路径（不 500，C09-5）。
	InvalidPaths []string `json:"invalid_paths,omitempty"`

	RootHash       string  `json:"root_hash"`
	VersionInteger *int64  `json:"version_integer"`
	VersionSemver  *string `json:"version_semver"`
	Channel        string  `json:"channel"`
	CompareEngine  string  `json:"compare_engine"`
	// Signature 与 check 同一套（§12.1）：双号 \n root_hash \n package_url \n size \n sha256。
	Signature string `json:"signature,omitempty"`
}

// DiffResult 是 diff 编排结果。
type DiffResult struct {
	Body *DiffResponse
}

// diffCapabilities 是 diff 裁决使用的客户端能力集合。
// 与 check 的 parseCapabilities 不同点：patch_package / file_list 参与分支，
// accepted_delta_algos 的具体取值需要保留做交集。
type diffCapabilities struct {
	binaryDelta   bool
	patchPackage  bool
	fileList      bool
	acceptedAlgos []string
}

// parseDiffCapabilities 合并 capabilities 与 accepted_delta_algos（§10.0）：
// 非空 accepted_delta_algos 自动授予 binary_delta；未知能力忽略。
func parseDiffCapabilities(caps, acceptedDeltaAlgos []string) diffCapabilities {
	var c diffCapabilities
	for _, a := range acceptedDeltaAlgos {
		if t := strings.TrimSpace(a); t != "" {
			c.acceptedAlgos = append(c.acceptedAlgos, t)
			c.binaryDelta = true
		}
	}
	for _, cap := range caps {
		switch strings.TrimSpace(cap) {
		case "binary_delta":
			c.binaryDelta = true
		case "patch_package":
			c.patchPackage = true
		case "file_list":
			c.fileList = true
		}
	}
	return c
}

// Diff 执行更新计划裁决（HTTP 层便捷入口）。
// 注入了 DowngradeSource 且请求带 DeviceHash 时，先查询连续失败降级状态
// （C11-8）：生效则置位 DeviceDowngradeActive，裁决在灰度闸后直接短路全量。
// 查询失败按「无降级」处理——遥测读模型绝不阻塞更新协议。
func (s *Service) Diff(ctx context.Context, projectID uuid.UUID, os, arch string, in DiffInput) (*DiffResult, error) {
	cat, err := s.loadCatalog(ctx, projectID, os, arch)
	if err != nil {
		return nil, err
	}
	if s.downgrade != nil && in.DeviceHash != "" {
		if active, derr := s.downgrade.DowngradeActive(ctx, projectID, os, arch, in.DeviceHash); derr == nil && active {
			in.DeviceDowngradeActive = true
		}
	}
	// 私有项目注入短时签名装配（§13.7 / C15-1）；公开项目零值原样返回。
	in.signing = s.signingFor(cat)
	return Diff(ctx, cat, s.details, in)
}

// Diff 是纯函数裁决（design §3 裁决顺序，任一命中即短路）：
//
//  1. 解析 source/target（未知 → VERSION_NOT_FOUND；target Draft → VERSION_NOT_VISIBLE；
//     target Revoked → VERSION_REVOKED。source 状态宽松：异常一律走全量回退，不报错）；
//  2. 灰度闸（C09-11）：target 非强制（复用 select 的强制判定）且灰度未命中 → 412；
//     吊销/yank 离开路径忽略本条；
//  3. 平台守恒：target (os,arch) 线必须就绪且 packs_ready_at 已盖戳，否则
//     VERSION_LINE_NOT_FOUND / VERSION_NOT_VISIBLE；
//     请求只有一对 os/arch，禁止任何隐式跨平台换算；
//  4. 多文件原生路径是 POST /update/pack（本函数不入队、不处理 needed_paths）；
//  5. 干净升级：patch_package（预生成对象、能力、非降级）→
//     binary_delta（单文件、能力、算法交集、基线哈希吻合、非降级）→
//     file_list 例外（显式声明且新增文件 ≤16）→ full_package；
//     降级目标禁止 binary_delta/patch（C09-8）；
//  6. 响应组装与签名注入（与 check 同套算法）。
func Diff(ctx context.Context, cat *Catalog, details LineDetailSource, in DiffInput) (*DiffResult, error) {
	if cat == nil {
		return nil, ErrInvalidQuery
	}
	if details == nil {
		return nil, ErrLineDetailsUnavailable
	}

	// ---- 1. 解析与状态闸 ----
	srcRef, ok := parseVersionRef(in.SourceVersion)
	if !ok {
		return nil, ErrVersionNotFound
	}
	tgtRef, ok := parseVersionRef(in.TargetVersion)
	if !ok {
		return nil, ErrVersionNotFound
	}
	source := findVersionState(cat, srcRef)
	target := findVersionState(cat, tgtRef)
	if source == nil || target == nil {
		return nil, ErrVersionNotFound
	}
	switch target.Version.Status {
	case model.VersionStatusDraft:
		return nil, ErrVersionNotVisible
	case model.VersionStatusRevoked:
		return nil, ErrVersionRevoked
	}
	// channel 可省略（以 Version 为准）；传入则必须一致（C09-10 同规则）。
	if ch := strings.TrimSpace(in.Channel); ch != "" && ch != target.Version.ChannelSlug {
		return nil, ErrChannelConflict
	}

	// ---- 2. 灰度闸（C09-11）----
	srcLine := source.Line(in.OS, in.Arch)
	// 目标线切片：per-line 百分比覆盖与 per-line 白名单按目标线判定（C12）。
	tgtLine := target.Line(in.OS, in.Arch)
	// 吊销/yank 离开路径忽略灰度（强制离开不设闸）。
	escape := source.Version.Status == model.VersionStatusRevoked ||
		(srcLine != nil && srcLine.Status == model.VersionLineStatusYanked)
	if !escape && !diffGrayHit(cat, source, tgtLine, target, in) {
		return nil, ErrPreconditionFailed
	}

	// ---- 3. 平台守恒：target 线必须就绪 ----
	if tgtLine == nil || tgtLine.Status != model.VersionLineStatusReady {
		return nil, ErrVersionLineNotFound
	}
	if tgtLine.PacksReadyAt == nil {
		return nil, ErrVersionNotVisible
	}
	tgtPkg := matchHwVariant(cat, cat.Matrix, tgtLine, strings.TrimSpace(in.HwRev))
	if tgtPkg == nil {
		return nil, ErrVersionLineNotFound
	}
	// fullURL：私有项目附加短时签名（§13.7 / C15-1）。
	fullURL := in.signing.artifactURL(cat.Project.Slug, tgtPkg.SHA256, tgtPkg.StorageKey)

	// ---- 4. （已删除 dirty_paths：多文件走 pack；diff 仅单文件 binary_delta）----

	// ---- 5.5 连续失败强制全量（C11-8 / §15.2）----
	// 设备在 24h 窗口内 failed≥3 且未被 installed 恢复：跳过全部差量分支，
	// 在灰度闸之后、差量选择之前直接短路 full_package。
	if in.DeviceDowngradeActive {
		return fullDiffResult(cat, target, tgtLine, tgtPkg, fullURL, nil)
	}

	// ---- 5. 干净升级裁决 ----
	engine := cat.Project.CompareEngine
	srcKey, srcKeyOK := versionKey(engine, &source.Version)
	tgtKey, tgtKeyOK := versionKey(engine, &target.Version)
	isDowngrade := srcKeyOK && tgtKeyOK && func() bool {
		c, comparable := compareKeys(tgtKey, srcKey)
		return comparable && c < 0
	}()
	caps := parseDiffCapabilities(in.Capabilities, in.AcceptedDeltaAlgos)
	matrix := matrixOrEmpty(cat.Matrix)
	singleFile := matrix.PackageType == model.PackageTypeSingleFile

	// source 健康性：未知行/Draft/Revoked/yank/未就绪/无兼容变体 → 全量回退（C09-7）。
	srcHealthy := false
	var srcPkg *ArtifactInfo
	if srcLine != nil && srcLine.Status == model.VersionLineStatusReady && srcKeyOK {
		if p := matchHwVariant(cat, cat.Matrix, srcLine, strings.TrimSpace(in.HwRev)); p != nil {
			srcPkg = p
			srcHealthy = true
		}
	}

	var tgtDetail, srcDetail *LineDetail
	getTargetDetails := func() (*LineDetail, error) {
		if tgtDetail == nil {
			d, err := details.LineDetails(ctx, tgtLine.ID)
			if err != nil {
				return nil, err
			}
			tgtDetail = d
		}
		return tgtDetail, nil
	}
	getSourceDetails := func() (*LineDetail, error) {
		if srcDetail == nil {
			d, err := details.LineDetails(ctx, srcLine.ID)
			if err != nil {
				return nil, err
			}
			srcDetail = d
		}
		return srcDetail, nil
	}

	mode := DiffModeFullPackage
	var delta *DeltaArtifactInfo

	// 6a. patch_package：仅非降级、source 线健康、显式能力、预生成对象以
	// 源/目标线全量包 SHA-256 身份命中（C13-4，与 binary_delta 同一身份规则）
	// 且体积 <70% 全量（§7.5.2）。
	if !in.PreferFull && !isDowngrade && caps.patchPackage && srcHealthy {
		td, err := getTargetDetails()
		if err != nil {
			return nil, err
		}
		if d := matchPatchPackage(td.Patches, srcPkg.SHA256, tgtPkg.SHA256, strings.TrimSpace(in.HwRev)); d != nil {
			mode = DiffModePatchPackage
			delta = d
		}
	}

	// 6b. binary_delta：仅单文件、非降级、能力 + 算法交集 + 基线哈希吻合（§7.2）。
	// C10-3：基线哈希必须等于 source 官方哈希，否则禁止套差量。
	// 命中条件再叠加差量对象自带的源/目标官方哈希（C10-6：任一端字节变化
	// → 哈希变化 → 旧差量不再命中）。
	if !in.PreferFull && !isDowngrade && mode == DiffModeFullPackage &&
		singleFile && caps.binaryDelta && len(caps.acceptedAlgos) > 0 &&
		in.LocalSHA256 != "" && srcHealthy && strings.EqualFold(in.LocalSHA256, srcPkg.SHA256) {
		td, err := getTargetDetails()
		if err != nil {
			return nil, err
		}
		if d := matchBinaryDelta(td.Deltas, srcPkg.SHA256, tgtPkg.SHA256, caps.acceptedAlgos, strings.TrimSpace(in.HwRev), matrix.DeltaAlgo); d != nil {
			mode = DiffModeBinaryDelta
			delta = d
		}
	}

	// deleted_paths：仅多文件且 source 健康时计算（source Manifest 有而 target 无）。
	var deletedPaths []string
	var fileListFiles []DiffFile
	if !singleFile && srcHealthy {
		sd, err := getSourceDetails()
		if err != nil {
			return nil, err
		}
		td, err := getTargetDetails()
		if err != nil {
			return nil, err
		}
		deletedPaths = diffDeletedPaths(sd.Manifest, td.Manifest)

		// 6c. file_list 例外（C09-6 / §7.5.4）：显式声明 + 待下载（新增）文件数不超过生效上限
		// 且每个新增路径都有 kind=file 产物可下发；任一不满足 → 维持归档模式。
		if !in.PreferFull && !isDowngrade && mode == DiffModeFullPackage && caps.fileList {
			if files, ok := buildFileList(td, sd, cat.Project.Slug, in.signing, fileListMax(cat)); ok {
				mode = DiffModeFileList
				fileListFiles = files
			}
		}
	}

	// ---- 6. 响应组装 ----
	resp := &DiffResponse{
		DiffMode:       mode,
		RootHash:       tgtLine.RootHash,
		VersionInteger: target.Version.VersionInteger,
		VersionSemver:  target.Version.VersionSemverCanonical,
		Channel:        target.Version.ChannelSlug,
		CompareEngine:  cat.Project.CompareEngine,
	}
	switch mode {
	case DiffModeFullPackage:
		// 与 check 200 完全同源：同一 matchHwVariant 结果行（验收项）。
		resp.PackageURL = fullURL
		resp.FileName = tgtPkg.FileName
		resp.Size = tgtPkg.Size
		resp.SHA256 = tgtPkg.SHA256
	case DiffModePatchPackage, DiffModeBinaryDelta:
		resp.PackageURL = in.signing.artifactURL(cat.Project.Slug, delta.SHA256, delta.StorageKey)
		resp.FileName = delta.FileName
		resp.Size = delta.Size
		resp.SHA256 = delta.SHA256
		if mode == DiffModeBinaryDelta {
			resp.DeltaAlgo = delta.Algo
		}
	case DiffModeFileList:
		resp.Files = fileListFiles
	}
	resp.DeletedPaths = deletedPaths
	resp.InvalidPaths = nil

	// 签名（§12.1，与 check 同套载荷）：双号\nroot_hash\npackage_url\nsize\nsha256。
	if cat.Project.SigningPrivateKey != "" {
		payload := signature.BuildCheckPayload(
			int64OrEmpty(target.Version.VersionInteger),
			stringOrEmpty(target.Version.VersionSemverCanonical),
			tgtLine.RootHash,
			resp.PackageURL,
			strconv.FormatInt(resp.Size, 10),
			resp.SHA256,
		)
		sig, err := signature.SignPayload(cat.Project.SigningAlgo, cat.Project.SigningPrivateKey, payload)
		if err != nil {
			return nil, err
		}
		resp.Signature = sig
	}

	return &DiffResult{Body: resp}, nil
}

// diffGrayHit 灰度闸命中判定（C09-11）：复用 select 的强制判定
// （floor、关键路径——强制则视为命中，忽略灰度与白名单）与共享的
// grayHitFor（per-line 百分比覆盖、白名单、一致性哈希放量同口径，C12）。
func diffGrayHit(cat *Catalog, source *VersionState, tgtLine *LineState, target *VersionState, in DiffInput) bool {
	req := Request{
		Current:    source,
		OS:         in.OS,
		Arch:       in.Arch,
		HwRev:      strings.TrimSpace(in.HwRev),
		DeviceID:   in.DeviceID,
		DeviceHash: in.DeviceHash,
	}
	if key, ok := versionKey(cat.Project.CompareEngine, &source.Version); ok {
		if mandatory, _ := mandatoryFor(cat, req, target, cat.Matrix, key); mandatory {
			return true
		}
	}
	return grayHitFor(cat, target, tgtLine, grayDevice{DeviceHash: in.DeviceHash, RawDevice: in.DeviceID})
}

// matchPatchPackage 匹配预生成增量归档：源/目标全量 SHA-256 身份 + hw。
// 体积闸门已在生成/入队前用 Manifest 未压缩合计完成，此处只要求 Size>0。
func matchPatchPackage(patches []DeltaArtifactInfo, srcSHA, tgtSHA, hwRev string) *DeltaArtifactInfo {
	for i := range patches {
		d := &patches[i]
		if d.Kind != DeltaKindPatchPackage {
			continue
		}
		if d.SourceSHA256 == "" || d.TargetSHA256 == "" {
			continue
		}
		if !strings.EqualFold(d.SourceSHA256, srcSHA) || !strings.EqualFold(d.TargetSHA256, tgtSHA) {
			continue
		}
		if !deltaHwMatch(d.HwRev, hwRev) {
			continue
		}
		if d.Size <= 0 {
			continue
		}
		return d
	}
	return nil
}

// matchBinaryDelta 匹配单文件二进制差量（§7.2 / C10-5）：
//   - 对象的源/目标官方哈希必须与请求解析出的全量包哈希一致（C10-6：字节变化
//     → 哈希变化 → 旧差量永不复用；未声明哈希的行不参与匹配）；
//   - 算法必须在 accepted ∩ 可用交集内；
//   - 交集内优先矩阵默认算法 matrix.delta_algo；无默认（或默认未命中）时按
//     算法名字典序取第一个，再按文件名字典序决胜负 —— 确定性回退（design §5）。
func matchBinaryDelta(deltas []DeltaArtifactInfo, srcSHA, tgtSHA string, accepted []string, hwRev, defaultAlgo string) *DeltaArtifactInfo {
	var best *DeltaArtifactInfo
	for i := range deltas {
		d := &deltas[i]
		if d.Kind != DeltaKindBinaryDelta {
			continue
		}
		// 差量身份锁定：源/目标全量包哈希必须双双吻合（C10-6）。
		if d.SourceSHA256 == "" || d.TargetSHA256 == "" {
			continue
		}
		if !strings.EqualFold(d.SourceSHA256, srcSHA) || !strings.EqualFold(d.TargetSHA256, tgtSHA) {
			continue
		}
		if !deltaHwMatch(d.HwRev, hwRev) {
			continue
		}
		if !algoAccepted(accepted, d.Algo) {
			continue
		}
		if best == nil || deltaCandidateLess(d, best, defaultAlgo) {
			best = d
		}
	}
	return best
}

// deltaCandidateLess 在两个已通过硬性闸的 binary_delta 候选间决出优先者：
// 默认算法 > 算名字典序 > 文件名字典序（§7.2 优先默认；design §5 确定性回退）。
func deltaCandidateLess(a, b *DeltaArtifactInfo, defaultAlgo string) bool {
	aDefault := defaultAlgo != "" && strings.EqualFold(a.Algo, defaultAlgo)
	bDefault := defaultAlgo != "" && strings.EqualFold(b.Algo, defaultAlgo)
	if aDefault != bDefault {
		return aDefault
	}
	if !strings.EqualFold(a.Algo, b.Algo) {
		return strings.ToLower(a.Algo) < strings.ToLower(b.Algo)
	}
	return a.FileName < b.FileName
}

// deltaHwMatch 差量对象的 hw 变体兼容性：默认变体（nil/空）对所有客户端可用，
// 非默认变体要求请求 hw_rev 精确一致（§7.2 基线含 hw 变体维度）。
func deltaHwMatch(dHw *string, hwRev string) bool {
	if isDefaultHwRev(dHw) {
		return true
	}
	return hwRev != "" && strings.TrimSpace(*dHw) == hwRev
}

// algoAccepted 判断差量算法是否在客户端接受列表内（大小写不敏感）。
func algoAccepted(accepted []string, algo string) bool {
	if algo == "" {
		return false
	}
	for _, a := range accepted {
		if strings.EqualFold(strings.TrimSpace(a), algo) {
			return true
		}
	}
	return false
}

// diffDeletedPaths 计算 source Manifest 有而 target Manifest 无的路径
// （§7.1 第 6 步的 DELETE 列表；输出按路径升序保证确定性）。
func diffDeletedPaths(src, tgt []model.ManifestEntry) []string {
	tgtSet := make(map[string]struct{}, len(tgt))
	for _, e := range tgt {
		tgtSet[e.Path] = struct{}{}
	}
	var deleted []string
	for _, e := range src {
		if _, ok := tgtSet[e.Path]; !ok {
			deleted = append(deleted, e.Path)
		}
	}
	sort.Strings(deleted)
	return deleted
}

// buildFileList 组装 file_list 例外的逐文件条目：target Manifest 有而 source 无
// 的每个路径都必须有 kind=file 产物（含 URL 映射），否则返回 ok=false 回退归档。
// URL 经签名装配（私有项目带 ?exp=&sig=，C15-1）。
func buildFileList(td, sd *LineDetail, projectSlug string, signing urlSigning, maxFiles int) ([]DiffFile, bool) {
	srcSet := make(map[string]struct{}, len(sd.Manifest))
	for _, e := range sd.Manifest {
		srcSet[e.Path] = struct{}{}
	}
	fileByName := make(map[string]FileArtifactInfo, len(td.Files))
	for _, f := range td.Files {
		if f.Path != "" {
			fileByName[f.Path] = f
		}
	}
	var files []DiffFile
	for _, e := range td.Manifest {
		if _, ok := srcSet[e.Path]; ok {
			continue
		}
		art, ok := fileByName[e.Path]
		if !ok {
			return nil, false
		}
		files = append(files, DiffFile{
			Path:   e.Path,
			Size:   e.Size,
			SHA256: e.SHA256,
			MD5:    e.MD5,
			URL:    signing.artifactURL(projectSlug, art.SHA256, art.StorageKey),
		})
	}
	if maxFiles < 1 {
		maxFiles = model.DefaultFileListMaxFiles
	}
	if len(files) > maxFiles {
		return nil, false
	}
	return files, true
}

// fullDiffResult 组装 full_package 响应（脏路径与全量兜底共用）。
// 全量包三元组与 check 200 同源：同一 matchHwVariant 结果行。
func fullDiffResult(cat *Catalog, target *VersionState, tgtLine *LineState, tgtPkg *ArtifactInfo, fullURL string, invalidPaths []string) (*DiffResult, error) {
	resp := &DiffResponse{
		DiffMode:       DiffModeFullPackage,
		PackageURL:     fullURL,
		FileName:       tgtPkg.FileName,
		Size:           tgtPkg.Size,
		SHA256:         tgtPkg.SHA256,
		RootHash:       tgtLine.RootHash,
		VersionInteger: target.Version.VersionInteger,
		VersionSemver:  target.Version.VersionSemverCanonical,
		Channel:        target.Version.ChannelSlug,
		CompareEngine:  cat.Project.CompareEngine,
		InvalidPaths:   invalidPaths,
	}
	if cat.Project.SigningPrivateKey != "" {
		payload := signature.BuildCheckPayload(
			int64OrEmpty(target.Version.VersionInteger),
			stringOrEmpty(target.Version.VersionSemverCanonical),
			tgtLine.RootHash,
			fullURL,
			strconv.FormatInt(tgtPkg.Size, 10),
			tgtPkg.SHA256,
		)
		sig, err := signature.SignPayload(cat.Project.SigningAlgo, cat.Project.SigningPrivateKey, payload)
		if err != nil {
			return nil, err
		}
		resp.Signature = sig
	}
	return &DiffResult{Body: resp}, nil
}
