package update

import (
	"context"
	"crypto/hmac"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/Kirizu-Official/KiriVers/internal/model"
	"github.com/Kirizu-Official/KiriVers/pkg/signature"
)

var (
	// ErrInvalidQuery 请求参数非法（缺失必填、未知渠道等）
	// → HTTP 400 INVALID_QUERY_PARAM。
	ErrInvalidQuery = errors.New("invalid query parameter")
	// ErrEngineMismatch current_version 无法解析 → HTTP 400 ENGINE_MISMATCH。
	ErrEngineMismatch = errors.New("engine mismatch or invalid version format")
	// ErrVersionNotFound 未知 current_version → HTTP 404 VERSION_NOT_FOUND
	//（永不视为 0，C08-2）。
	ErrVersionNotFound = errors.New("version not found")
)

// Cache-Control 常量（§10.1 / task design §6）。
const (
	cacheControlPrivateDevice = "private, max-age=0, must-revalidate"
	cacheControlNoStore       = "private, no-store"
	cacheControlSWR           = "stale-while-revalidate=30"
)

// cacheControlPublic 组装 changelog / integrity 等 GET 的共享缓存头。
// POST /update/check 不再使用 public s-maxage。
func cacheControlPublic(sMaxage int) string {
	if sMaxage <= 0 {
		sMaxage = model.DefaultCacheSMaxageSeconds
	}
	return fmt.Sprintf("public, s-maxage=%d, %s", sMaxage, cacheControlSWR)
}

// buildVary 组装 Vary：恒含 Accept-Encoding；开启客户端 Token 追加 Authorization。
func buildVary(project ProjectInfo) []string {
	vary := []string{"Accept-Encoding"}
	if project.RequireClientToken {
		vary = append(vary, "Authorization")
	}
	return vary
}

// CheckInput 是 POST update/check 的请求参数（已由 HTTP 层完成原始绑定）。
type CheckInput struct {
	// OS / Arch 是已规范化的请求平台（HTTP 层经 platform.CanonicalOS/Arch 规范化），
	// 与目录加载的平台一致。
	OS, Arch       string
	CurrentVersion string
	// Channel 仅做存在性校验；选目标锚定当前 Version 的渠道（§4.4）。
	// 不可见渠道允许在 query 与当前渠道 slug 匹配时作为升级目标。
	Channel string
	// ChannelToken 是 X-Channel-Token 明文；永不进入 ETag / 日志 / 缓存键。
	ChannelToken string
	HwRev        string
	// OSVersion 可选；传入时才做 min_os / min_api_level 比较。
	OSVersion string
	// DeviceID 仅用于灰度，不进入 ETag / 日志 / 缓存键。
	DeviceID string
	// DeviceHash 是按项目策略（HMAC/明文）处理后的设备标识，由 HTTP 层经
	// 遥测服务计算后传入；用于限流键、连续失败降级查询与灰度白名单匹配
	//（C12：raw 策略下白名单比较改用原样 DeviceID，见 grayHitFor）。
	// 空 = 匿名或 none 策略。
	DeviceHash string
	// DeviceDowngradeActive 由 Service 编排层根据 DeviceHash 查询降级读模型
	// 后置位（C11-8）；纯函数直接消费：置位时 delta_available 强制 false。
	// 该状态与灰度一样排除在 ETag / Cache-Control 之外（不进快照）。
	DeviceDowngradeActive bool
	// Capabilities JSON 字符串数组；未知能力忽略。
	Capabilities []string
	// AcceptedDeltaAlgos 非空（含至少一个非空值）自动授予 binary_delta（§10.0）。
	AcceptedDeltaAlgos []string

	// signing 由 Service 包装层按项目可见性注入（私有项目 = 签名器 + 项目级
	// TTL，§13.7 / C15-1）；HTTP 层与测试直调纯函数时为零值，URL 原样拼装。
	signing urlSigning
}

// CheckResponse 是 200 响应体（§10.1 / task design §8，无 display_version、
// 无逐文件 URL 数组）。
type CheckResponse struct {
	HasUpdate      bool    `json:"has_update"`
	IsMandatory    bool    `json:"is_mandatory"`
	IsDowngrade    bool    `json:"is_downgrade"`
	Reason         string  `json:"reason"`
	CompareEngine  string  `json:"compare_engine"`
	VersionInteger *int64  `json:"version_integer"`
	VersionSemver  *string `json:"version_semver"`
	TargetChannel  string  `json:"target_channel"`
	TargetHwRev    *string `json:"target_hw_rev"`
	PackageType    string  `json:"package_type"`
	PublishTime    *string `json:"publish_time,omitempty"`

	// PlatformNotes 目标线平台说明；受项目 changelog_include_platform_notes 开关。
	PlatformNotes *string `json:"platform_notes,omitempty"`

	RootHash   string `json:"root_hash"`
	PackageURL string `json:"package_url"`
	FileName   string `json:"file_name"`
	Size       int64  `json:"size"`
	SHA256     string `json:"sha256"`
	// Signature 是项目私钥对关键字段的签名（未配置时省略）；
	// ArtifactSignature 是开发者随包载荷，与 Signature 完全独立、原样透传。
	Signature         string `json:"signature,omitempty"`
	ArtifactSignature string `json:"artifact_signature,omitempty"`

	DeltaAvailable bool   `json:"delta_available"`
	DeltaAlgo      string `json:"delta_algo,omitempty"`
}

// CheckResult 是 check 编排结果：HTTP 层据此写状态码、ETag、缓存头与 body。
type CheckResult struct {
	// Status 为 200（有更新）或 204（无更新）。
	Status int
	// Body 仅在 200 时非 nil。
	Body *CheckResponse
	// ETag 基于目录快照（不含 device_id/query），200/204/304 共用。
	ETag string
	// CacheControl 按请求是否带 device_id 与目录灰度状态决定；
	// 私有项目恒 private, no-store（签名 URL 不进共享 CDN，§13.7）。
	CacheControl string
	// Vary 是有序的 Vary 列表。
	Vary []string
	// Private 标记项目存储可见性为 private：HTTP 层据此跳过 If-None-Match
	// 304 短路（私有响应必须每次带新签名 URL，恒回新鲜 200）。
	Private bool
}

// Service 组合 CatalogLoader 与纯函数编排，是 HTTP 层的唯一入口。
//
// details 为可选的 Line 明细读取源（Manifest / delta / file 产物），
// 仅 integrity 与 diff 依赖；未注入时二者返回 ErrLineDetailsUnavailable，
// check 路径不受影响。
type Service struct {
	loader  CatalogLoader
	details LineDetailSource
	// downgrade 是可选的连续失败降级读取源（C11-8）；nil 时不查询。
	downgrade DowngradeSource
	// signer 是可选的私有存储短时签名器（§13.7 / C15-1）；nil 或公开项目
	// 时不签名（C15-2 公开直链不变）。
	signer URLSigner
	// pack 是可选的动态打包运行时（ProjectService）；nil 时 pack API 走 full_package。
	pack PackRuntime
	// dynamicPackMaxBytes 是 D6 未压缩硬顶；0 视为 512MiB。
	dynamicPackMaxBytes int64
	// fileListMaxFiles 是原生 file_list 平台天花板；<1 视为 16。
	fileListMaxFiles int
	// changelogDefaultEntries / changelogMaxEntries 是实例 changelog 窗口；<1 视为 5/50。
	changelogDefaultEntries int
	changelogMaxEntries     int
	objectURL               func(slug, sha256, storageKey string) string
	replicaReady            func(storageKey string) bool
	localProxy              bool
}

// NewService 构造 check 服务；可变参注入 Option（如 WithLineDetails）。
func NewService(loader CatalogLoader, opts ...Option) *Service {
	s := &Service{loader: loader}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

// LoadCatalog 加载项目在 (os, arch) 平台上的目录快照，并写入已钳制的 file_list 上限。
func (s *Service) LoadCatalog(ctx context.Context, projectID uuid.UUID, os, arch string) (*Catalog, error) {
	return s.loadCatalog(ctx, projectID, os, arch)
}

func (s *Service) loadCatalog(ctx context.Context, projectID uuid.UUID, os, arch string) (*Catalog, error) {
	cat, err := s.loader.LoadCatalog(ctx, projectID, os, arch)
	if err != nil {
		return nil, err
	}
	applyFileListCeiling(cat, s.fileListMaxFiles)
	applyChangelogCeiling(cat, s.changelogDefaultEntries, s.changelogMaxEntries)
	return s.applyReplica(cat), nil
}

func applyFileListCeiling(cat *Catalog, platform int) {
	if cat == nil {
		return
	}
	cat.Project.FileListMaxFiles = model.EffectiveFileListMax(platform, cat.Project.FileListMaxFiles)
}

func applyChangelogCeiling(cat *Catalog, instDefault, instMax int) {
	if cat == nil {
		return
	}
	effMax := model.EffectiveChangelogMax(instMax, cat.Project.ChangelogMaxEntries)
	cat.Project.ChangelogMaxEntries = effMax
	cat.Project.ChangelogDefaultEntries = model.EffectiveChangelogDefault(instDefault, cat.Project.ChangelogDefaultEntries, effMax)
}

func fileListMax(cat *Catalog) int {
	n := 0
	if cat != nil {
		n = cat.Project.FileListMaxFiles
	}
	if n < 1 {
		return model.DefaultFileListMaxFiles
	}
	return n
}

// Check 加载目录并执行一次更新检查（HTTP 层便捷入口）。
// 注入了 DowngradeSource 且请求带 DeviceHash 时，先查询连续失败降级状态
// （C11-8）：生效则置位 DeviceDowngradeActive，最终 delta_available 强制 false；
// 查询失败按「无降级」处理——遥测读模型绝不阻塞更新协议。降级状态不参与
// ETag / Cache-Control（与灰度同样排除在目录快照之外）。
func (s *Service) Check(ctx context.Context, projectID uuid.UUID, os, arch string, in CheckInput) (*CheckResult, error) {
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
	return Check(cat, in)
}

func (s *Service) applyReplica(cat *Catalog) *Catalog {
	if s == nil || s.replicaReady == nil || cat == nil {
		return cat
	}
	cp := *cat
	cp.Versions = append([]VersionState(nil), cat.Versions...)
	for i := range cp.Versions {
		lines := append([]LineState(nil), cp.Versions[i].Lines...)
		for j := range lines {
			if !lineReplicaReady(&lines[j], s.replicaReady) {
				lines[j].PacksReadyAt = nil
			}
		}
		cp.Versions[i].Lines = lines
	}
	return &cp
}

func lineReplicaReady(line *LineState, fn func(string) bool) bool {
	if line == nil || fn == nil {
		return true
	}
	for _, p := range line.FullPkgs {
		if p.StorageKey == "" {
			continue
		}
		if !fn(p.StorageKey) {
			return false
		}
	}
	return true
}

// Check 是纯函数编排：参数校验 → 当前版本解析 → SelectTarget →
// 组装 200/204 结果（platform_notes、签名注入、ETag 与缓存头）。
// changelog 只走独立 GET /changelog。不触库、不依赖 gin。
func Check(cat *Catalog, in CheckInput) (*CheckResult, error) {
	if cat == nil {
		return nil, ErrInvalidQuery
	}

	// 1. channel 参数仅做存在性校验。
	if channel := strings.TrimSpace(in.Channel); channel != "" {
		if _, found := cat.Channel(channel); !found {
			return nil, fmt.Errorf("%w: unknown channel %q", ErrInvalidQuery, channel)
		}
	}

	// 2. 解析 current_version：非法 → ENGINE_MISMATCH；未知 → VERSION_NOT_FOUND。
	ref, ok := parseVersionRef(in.CurrentVersion)
	if !ok {
		return nil, ErrEngineMismatch
	}
	current := findVersionState(cat, ref)
	if current == nil {
		return nil, ErrVersionNotFound
	}

	// 3. 选目标（唯一实现）。
	res, err := SelectTarget(cat, Request{
		Current:      current,
		OS:           in.OS,
		Arch:         in.Arch,
		HwRev:        strings.TrimSpace(in.HwRev),
		OSVersion:    strings.TrimSpace(in.OSVersion),
		ChannelQuery: strings.TrimSpace(in.Channel),
		ChannelToken: in.ChannelToken,
		DeviceID:     in.DeviceID,
		DeviceHash:   in.DeviceHash,
	})
	if err != nil {
		return nil, err
	}

	// 4. ETag 与缓存头（§10.1）。ETag 输入不变：URL/签名永不进入目录快照。
	etag := CatalogETag(cat)
	// 私有项目（C15-1）：恒 private, no-store（签名 URL 不得进共享 CDN），
	// 且 HTTP 层跳过 304 短路（Private 标记）——每次回新鲜 200 带新签名。
	private := cat.Project.StorageVisibility == model.StorageVisibilityPrivate || in.signing.localProxy
	cacheControl := cacheControlPrivateDevice
	if private {
		cacheControl = cacheControlNoStore
	}
	vary := buildVary(cat.Project)
	if cat.catalogHasTokenProtected() {
		vary = append(vary, "X-Channel-Token")
	}
	result := &CheckResult{
		ETag:         etag,
		CacheControl: cacheControl,
		Vary:         vary,
		Private:      private,
	}

	// 5. 无更新 → 204（无 body，仍带 ETag / Cache-Control / Vary）。
	if res.Target == nil {
		result.Status = 204 // http.StatusNoContent
		return result, nil
	}

	// 6. 组装 200 响应体。
	resp := &CheckResponse{
		HasUpdate:      true,
		IsMandatory:    res.IsMandatory,
		IsDowngrade:    res.IsDowngrade,
		Reason:         res.Reason,
		CompareEngine:  cat.Project.CompareEngine,
		VersionInteger: res.Target.Version.VersionInteger,
		VersionSemver:  res.Target.Version.VersionSemverCanonical,
		TargetChannel:  res.Target.Version.ChannelSlug,
		TargetHwRev:    res.Package.HwRev,
		PackageType:    matrixOrEmpty(matrixFor(cat, res)).PackageType,
		RootHash:       res.Line.RootHash,
		PackageURL:     in.signing.artifactURL(cat.Project.Slug, res.Package.SHA256, res.Package.StorageKey),
		FileName:       res.Package.FileName,
		Size:           res.Package.Size,
		SHA256:         res.Package.SHA256,
	}
	if res.Target.Version.PublishTime != nil {
		s := res.Target.Version.PublishTime.UTC().Format(time.RFC3339)
		resp.PublishTime = &s
	}

	if cat.Project.ChangelogIncludeNotes {
		notes := res.Line.PlatformNotes
		resp.PlatformNotes = &notes
	}

	// delta 能力与降级维度（§10.0）：binary_delta 能力 && 非降级 &&
	// 设备未进入连续失败强制全量状态（C11-8：failed≥3 未恢复 → delta_available=false）。
	caps := parseCapabilities(in.Capabilities, in.AcceptedDeltaAlgos)
	resp.DeltaAvailable = caps.binaryDelta && res.DeltaAllowed && !in.DeviceDowngradeActive
	if resp.DeltaAvailable {
		if algo := matrixOrEmpty(matrixFor(cat, res)).DeltaAlgo; algo != "" {
			resp.DeltaAlgo = algo
		}
	}

	// 签名注入（§12.1）：项目配置私钥才输出。
	if cat.Project.SigningPrivateKey != "" {
		payload := signature.BuildCheckPayload(
			int64OrEmpty(res.Target.Version.VersionInteger),
			stringOrEmpty(res.Target.Version.VersionSemverCanonical),
			res.Line.RootHash,
			resp.PackageURL,
			strconv.FormatInt(res.Package.Size, 10),
			res.Package.SHA256,
		)
		sig, err := signature.SignPayload(cat.Project.SigningAlgo, cat.Project.SigningPrivateKey, payload)
		if err != nil {
			return nil, err
		}
		resp.Signature = sig
	}
	resp.ArtifactSignature = res.Package.ArtifactSignature

	result.Status = 200 // http.StatusOK
	result.Body = resp
	return result, nil
}

// matrixFor 返回目标平台对应的矩阵行（fallback_arch 命中时用回退行）。
func matrixFor(cat *Catalog, res Result) *MatrixInfo {
	if res.UsedFallbackArch && res.FallbackMatrix != nil {
		return res.FallbackMatrix
	}
	return cat.Matrix
}

// matrixOrEmpty 保证矩阵行访问不 panic（loader 未登记矩阵行时为 nil）。
func matrixOrEmpty(m *MatrixInfo) MatrixInfo {
	if m == nil {
		return MatrixInfo{}
	}
	return *m
}

// ChangelogInput 是独立 GET changelog/:channel/:os/:arch 的请求参数（§5.7）。
type ChangelogInput struct {
	// Channel 路径渠道 slug（必填）；输出只含该渠道。
	Channel string
	// ChannelToken 是 X-Channel-Token 明文；Token 保护渠道不匹配 → ErrNotFound。
	ChannelToken string
	// OS / Arch 来自路径：range_platform 过滤与 had_artifact_for_request_platform。
	OS, Arch    string
	FromVersion string
	ToVersion   string
	Scope       string
	Layout      string
	// IncludeRevoked / IncludeNotes 为 nil 时用项目默认。
	IncludeRevoked  *bool
	IncludeNotes    *bool
	ChangelogLocale string
	Locale          string
}

// ChangelogResponse 是独立 changelog 端点的响应体。
type ChangelogResponse struct {
	Changelog         string           `json:"changelog,omitempty"`
	ChangelogVersions []ChangelogEntry `json:"changelog_versions,omitempty"`
}

// ChangelogResult 是独立 changelog 编排结果。
type ChangelogResult struct {
	Body         ChangelogResponse
	ETag         string
	CacheControl string
	Vary         []string
}

// ServiceChangelog 执行独立 changelog 查询：路径渠道必填；from_version 可选。
func (s *Service) Changelog(ctx context.Context, projectID uuid.UUID, os, arch string, in ChangelogInput) (*ChangelogResult, error) {
	cat, err := s.loadCatalog(ctx, projectID, os, arch)
	if err != nil {
		return nil, err
	}
	return Changelog(cat, in)
}

// Changelog 是纯函数版独立 changelog 查询。
func Changelog(cat *Catalog, in ChangelogInput) (*ChangelogResult, error) {
	if cat == nil {
		return nil, ErrInvalidQuery
	}

	channel := strings.TrimSpace(in.Channel)
	if channel == "" {
		return nil, fmt.Errorf("%w: channel is required", ErrInvalidQuery)
	}
	ch, found := cat.Channel(channel)
	if !found {
		return nil, fmt.Errorf("%w: unknown channel %q", ErrInvalidQuery, channel)
	}
	if ch.TokenProtected {
		got := sha256Hex(strings.TrimSpace(in.ChannelToken))
		if !hmac.Equal([]byte(ch.TokenHash), []byte(got)) {
			return nil, ErrNotFound
		}
	}

	haveFrom := strings.TrimSpace(in.FromVersion) != ""
	var from *VersionState
	if haveFrom {
		fromRef, ok := parseVersionRef(in.FromVersion)
		if !ok {
			return nil, fmt.Errorf("%w: from_version must be a valid version", ErrInvalidChangelogQuery)
		}
		from = findVersionState(cat, fromRef)
		if from == nil {
			return nil, ErrVersionNotFound
		}
	}

	var to *VersionState
	if s := strings.TrimSpace(in.ToVersion); s != "" {
		toRef, ok := parseVersionRef(s)
		if !ok {
			return nil, fmt.Errorf("%w: to_version must be a valid version", ErrInvalidChangelogQuery)
		}
		to = findVersionState(cat, toRef)
		if to == nil {
			return nil, ErrVersionNotFound
		}
	}

	scope := cat.Project.ChangelogScope
	layout := cat.Project.ChangelogLayout
	if cat.Project.ChangelogClientOverride {
		if s := strings.TrimSpace(in.Scope); s != "" {
			scope = s
		}
		if l := strings.TrimSpace(in.Layout); l != "" {
			layout = l
		}
	}
	includeRevoked := cat.Project.ChangelogIncludeRevoked
	if in.IncludeRevoked != nil {
		includeRevoked = *in.IncludeRevoked
	}
	includeNotes := cat.Project.ChangelogIncludeNotes
	if in.IncludeNotes != nil {
		includeNotes = *in.IncludeNotes
	}

	query := ChangelogQuery{
		Engine:         cat.Project.CompareEngine,
		Scope:          scope,
		Layout:         layout,
		IncludeRevoked: includeRevoked,
		IncludeNotes:   includeNotes,
		LocaleChain:    BuildLocaleChain(in.ChangelogLocale, in.Locale, "", cat.Project.DefaultLocale),
		OS:             in.OS,
		Arch:           in.Arch,
	}

	target := to
	if target == nil {
		target = topPublishedVersionOnChannel(cat, channel)
	}
	if haveFrom && to == nil {
		to = target
	}

	agg, entries, err := BuildChangelog(cat.Versions, from, to, target, query)
	if err != nil {
		return nil, err
	}
	entries = filterChangelogChannel(entries, channel)

	limit := cat.Project.ChangelogMaxEntries
	if !haveFrom {
		limit = cat.Project.ChangelogDefaultEntries
		if scope == model.ChangelogScopeTargetOnly {
			limit = 1
		}
	}
	if limit < 1 {
		if haveFrom {
			limit = model.DefaultChangelogMaxEntries
		} else if scope == model.ChangelogScopeTargetOnly {
			limit = 1
		} else {
			limit = model.DefaultChangelogDefaultEntries
		}
	}
	entries = truncateChangelog(entries, limit)
	if layout == model.ChangelogLayoutAggregated || layout == model.ChangelogLayoutBoth {
		agg = renderAggregated(entries)
	} else {
		agg = ""
	}

	etagItems := make([]changelogETagItem, 0, len(entries))
	for _, e := range entries {
		etagItems = append(etagItems, changelogETagItem{
			ID: e.ID, Status: e.Status, Title: e.Title,
			Changelog: e.Changelog, PlatformNotes: e.PlatformNotes,
		})
	}

	body := ChangelogResponse{}
	if layout == model.ChangelogLayoutAggregated || layout == model.ChangelogLayoutBoth {
		body.Changelog = agg
	}
	if layout == model.ChangelogLayoutStructured || layout == model.ChangelogLayoutBoth {
		body.ChangelogVersions = entries
	}

	cacheControl := cacheControlPublic(cat.Project.CacheSMaxageSeconds)
	if ch.TokenProtected {
		cacheControl = cacheControlPrivateDevice
	}
	vary := buildVary(cat.Project)
	if ch.TokenProtected {
		vary = append(vary, "X-Channel-Token")
	}

	return &ChangelogResult{
		Body:         body,
		ETag:         ChangelogETag(etagItems),
		CacheControl: cacheControl,
		Vary:         vary,
	}, nil
}

func filterChangelogChannel(entries []ChangelogEntry, channel string) []ChangelogEntry {
	out := make([]ChangelogEntry, 0, len(entries))
	for _, e := range entries {
		if e.Channel == channel {
			out = append(out, e)
		}
	}
	return out
}

func truncateChangelog(entries []ChangelogEntry, n int) []ChangelogEntry {
	if n < 1 || len(entries) <= n {
		return entries
	}
	return entries[:n]
}

// topPublishedVersionOnChannel 返回该渠道比较键最高的 Published/Deprecated 版本。
func topPublishedVersionOnChannel(cat *Catalog, channel string) *VersionState {
	return topPublishedVersion(cat, channel)
}

// topPublishedVersion 返回比较键最高的 Published 非 revoked 版本（to 缺省）。
// channel 非空时限定该渠道。
func topPublishedVersion(cat *Catalog, channel ...string) *VersionState {
	want := ""
	if len(channel) > 0 {
		want = strings.TrimSpace(channel[0])
	}
	match := func(v *VersionState) bool {
		return want == "" || v.Version.ChannelSlug == want
	}
	var best *VersionState
	var bestKey cmpKey
	for i := range cat.Versions {
		v := &cat.Versions[i]
		if !match(v) {
			continue
		}
		if v.Version.Status != model.VersionStatusPublished && v.Version.Status != model.VersionStatusDeprecated {
			continue
		}
		key, ok := versionKey(cat.Project.CompareEngine, &v.Version)
		if !ok {
			continue
		}
		if best == nil {
			best, bestKey = v, key
			continue
		}
		if c, comparable := compareKeys(key, bestKey); comparable && c > 0 {
			best, bestKey = v, key
		}
	}
	// 仅 revoked/deprecated 时回退为比较键最高的非 revoked 版本。
	if best == nil {
		for i := range cat.Versions {
			v := &cat.Versions[i]
			if !match(v) {
				continue
			}
			if v.Version.Status == model.VersionStatusRevoked || v.Version.Status == model.VersionStatusDraft {
				continue
			}
			key, ok := versionKey(cat.Project.CompareEngine, &v.Version)
			if !ok {
				continue
			}
			if best == nil {
				best, bestKey = v, key
				continue
			}
			if c, comparable := compareKeys(key, bestKey); comparable && c > 0 {
				best, bestKey = v, key
			}
		}
	}
	return best
}

// capabilities 是客户端能力集合的判定结果。
type capabilities struct {
	binaryDelta bool
}

// parseCapabilities 合并 capabilities 与 accepted_delta_algos：
// 默认集为 full_package + 两种 changelog（无需记录，仅二进制差量与
// 文件列表参与分支）；binary_delta 默认无；非空 accepted_delta_algos
// 自动授予 binary_delta（§10.0）；未知能力忽略。
func parseCapabilities(caps []string, acceptedDeltaAlgos []string) capabilities {
	var c capabilities
	for _, cap := range caps {
		if strings.TrimSpace(cap) == "binary_delta" {
			c.binaryDelta = true
		}
	}
	if !c.binaryDelta {
		for _, a := range acceptedDeltaAlgos {
			if strings.TrimSpace(a) != "" {
				c.binaryDelta = true
				break
			}
		}
	}
	return c
}

// ---------- 静态 URL 拼装（端点本体属 integrity/diff 子任务） ----------

// PackageDownloadURL 返回哈希下载路径 /packages/{sha256}（小写 64 hex）。
// 导出供商店协议 feed 层复用：所有协议的下载 URL 必须与原生 check 同源，
// 不得各写一套路径拼装。装饰扩展名不属于 URL（D7 仅在 GET :ref 上解析）。
func PackageDownloadURL(projectSlug, sha256 string) string {
	return packageURL(projectSlug, sha256)
}

// packageURL 指向哈希下载路由 /packages/{sha256}。
func packageURL(projectSlug, sha256 string) string {
	return "/api/v1/projects/" + projectSlug + "/packages/" + strings.ToLower(strings.TrimSpace(sha256))
}

// versionRefString 返回可用于 URL 寻址的版本引用（SemVer 优先）。
func versionRefString(v *VersionState) string {
	if v.Version.VersionSemverCanonical != nil && *v.Version.VersionSemverCanonical != "" {
		return *v.Version.VersionSemverCanonical
	}
	if v.Version.VersionInteger != nil {
		return strconv.FormatInt(*v.Version.VersionInteger, 10)
	}
	return ""
}

func int64OrEmpty(p *int64) string {
	if p == nil {
		return ""
	}
	return strconv.FormatInt(*p, 10)
}

func stringOrEmpty(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}
