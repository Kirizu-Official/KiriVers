package store

import (
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/Kirizu-Official/KiriVers/internal/model"
	"github.com/Kirizu-Official/KiriVers/internal/platform"
	"github.com/Kirizu-Official/KiriVers/internal/service/update"
	"github.com/Kirizu-Official/KiriVers/pkg/signature"
)

// Tauri updater v2 投影（docs/app-init.md §9 Tauri 行，C18-1..C18-7）。
//
// Tauri 官方支持两种端点形态，本适配器同时实现（C18-1）：
//
//  1. 静态文档：GET /store/tauri/default/latest.json?os=&arch=&channel= —— Tauri v2
//     多平台 JSON：{version, notes, pub_date, platforms: {"darwin-x86_64":
//     {signature, url}, ...}}。os / arch query 可省略（真实 Tauri 客户端
//     不携带任何 query）：省略时 platforms 覆盖该项目矩阵中全部 Tauri 可用
//     os（macos/windows/linux）× 候选 arch（x86_64/arm64）组合。
//     顶层 version / notes / pub_date 取比较键最高的匿名可见版本（§4.4）；
//     platforms 只收录该同一最新版本在各平台的默认 hw 变体产物（design
//     裁决：静态 latest.json 只出现最新可见版本，与 Tauri updater 拿顶层
//     version 比对、按平台键取下载地址的行为一致——顶层版本与平台产物
//     必须是同一版本，否则客户端会被误导下载旧包）。
//  2. 动态端点：GET /store/tauri/default/{target}/{arch}/{current_version} —— 完整
//     走 update.SelectTarget（匿名 Request：无 device_id、hw_rev 强制空串
//     即默认变体，C18-7；Tauri 协议无法表达 hw_rev，query 里的 hw_rev 不
//     参与）。有更新 → 200 单平台对象 {version, notes, pub_date, signature,
//     url}；无更新 → 204 无 body（C18-3）。target ∈ darwin|windows|linux
//     （接受 macos 别名）映射回 macos/windows/linux；arch 接受 x86_64 /
//     arm64（含 aarch64 别名）；未知 target / arch → ErrUnknownPath（404）。
//
// 字段语义：
//
//   - version = version_semver，为空则 version_integer 十进制字符串
//     （C18-2；JSON 字符串语义，整数构建号不会被解析成数字）；
//
//   - url = 默认 hw 变体 kind=full 产物的 §5.9 稳定文件名下载 URL（私有
//     项目经短时签名，§13.7；复用 update.PackageDownloadURL，与原生 check
//     同源）；
//
//   - signature = minisign 容器格式的 Ed25519 签名（签安装器文件字节，
//     C18-4；§12.1 三种字节格式隔离：minisign 容器 ≠ 原生 base64 裸签名
//     ≠ sparkle:edSignature）：
//
//     untrusted comment: signature from KiriVers ed25519 key
//     <base64( "Ed" + key_id(8 字节全 0) + Ed25519 签名(64 字节) )>
//
//     RSA 项目 / 未配置私钥 / 无存储依赖 → 省略 signature 字段（不冒充；
//     客户端公钥由发布者登记在 StoreProtocols["tauri"].Identifiers
//     ["public_key"]，本服务不参与客户端验签）。结果按产物 SHA-256 走
//     LRU + 单飞缓存（复用 SignatureCache，独立实例）。
//
// HTTP 层语义（internal/controller/client/store 统一处理）：开关关闭 → 404
// （C18-5）；require_client_token / store_token → 401 UNAUTHORIZED（C18-6）；
// ETag / Cache-Control / Vary 统一计算；204 无 body、无 ETag。
const (
	// ProtocolTauri 是 Tauri updater 的协议名（Project.StoreProtocols 的
	// jsonb 键）。
	ProtocolTauri = "tauri"

	// tauriContentType 是 Tauri JSON 文档的 MIME。
	tauriContentType = "application/json; charset=utf-8"

	// tauriStaticPath 是静态文档路径（C18-1）。
	tauriStaticPath = "latest.json"

	// tauriMinisignComment 是 minisign 容器的 untrusted comment 行。客户端
	// 不校验该行内容（minisign 语义），但行必须存在且容器整体可被
	// minisign 解析。
	tauriMinisignComment = "untrusted comment: signature from KiriVers ed25519 key"

	// tauriNoUpdateStatus 是动态端点「无更新」的响应语义（C18-3）：Adapter
	// 返回 Status=204、无 body；HTTP 层据此跳过 ETag / body 写出。
	tauriNoUpdateStatus = 204

	// tauriStaticDefaultArches 是静态 latest.json 未指定 arch query 时的
	// 候选架构（Tauri 官方目标的两个主流架构；目录加载对不存在的组合
	// 返回空可见集，安全跳过）。
	tauriStaticDefaultArches = "x86_64,arm64"
)

// tauriOSes 是 Tauri updater 可用的 os 集合（§9 os/arch 映射：macos→darwin、
// windows→windows、linux→linux）。静态枚举与动态 target 校验共用。
var tauriOSes = map[string]bool{"macos": true, "windows": true, "linux": true}

// tauriArches 是动态端点接受的 arch 集合（x86_64 / arm64；aarch64、amd64
// 等别名先经 CanonicalArch 规范化）。未知 → 404。
var tauriArches = map[string]bool{"x86_64": true, "arm64": true}

// TauriAdapter 是 Tauri updater 协议的 Adapter 实现。
type TauriAdapter struct {
	// sigCache 缓存「产物字节 → minisign 容器签名字符串」，避免每次请求
	// 读取全量安装器文件（键 = 产物 SHA-256，与 Sparkle 签名缓存同一模式、
	// 独立实例——两协议容器格式不同，绝不共享条目）。
	sigCache *SignatureCache
}

// compile-time 校验：满足 Adapter 合同。
var _ Adapter = (*TauriAdapter)(nil)

// NewTauriAdapter 构造 Tauri Adapter（默认容量 minisign 签名缓存）。
func NewTauriAdapter() *TauriAdapter {
	return &TauriAdapter{sigCache: NewSignatureCache(0)}
}

// NewTauriAdapterWithCache 以给定缓存构造（测试可注入）。
func NewTauriAdapterWithCache(cache *SignatureCache) *TauriAdapter {
	return &TauriAdapter{sigCache: cache}
}

// Protocol 实现 Adapter。
func (a *TauriAdapter) Protocol() string { return ProtocolTauri }

// Enabled 实现 Adapter：StoreProtocols["tauri"].Enabled（C18-5）。
func (a *TauriAdapter) Enabled(p *model.Project) bool {
	return enabledStoreProtocol(p, ProtocolTauri)
}

// Render 实现 Adapter：按文档路径分流（latest.json → 静态多平台 JSON；
// {target}/{arch}/{current_version} → 动态选目标）。
func (a *TauriAdapter) Render(ctx context.Context, deps *Deps, req Request) (*Response, error) {
	if deps == nil || deps.Updates == nil {
		return nil, fmt.Errorf("feed: update service unavailable")
	}
	if req.Project == nil {
		return nil, fmt.Errorf("%w: project is required", ErrMissingParam)
	}

	path := strings.Trim(req.Path, "/")
	if path == tauriStaticPath {
		return a.renderStatic(ctx, deps, req)
	}
	segs := strings.Split(path, "/")
	switch len(segs) {
	case 3:
		// 官方形态：{{target}}/{{arch}}/{{current_version}}（C18-1）。
		return a.renderDynamic(ctx, deps, req, segs[0], segs[1], segs[2])
	case 2:
		// 兼容形态：{{target}}/{{arch}} + current_version query（HTTP 层
		// 已绑定 Request.CurrentVersion；OpenAPI 同步登记）。
		if req.CurrentVersion != "" {
			return a.renderDynamic(ctx, deps, req, segs[0], segs[1], req.CurrentVersion)
		}
	}
	return nil, fmt.Errorf("%w: %q", ErrUnknownPath, req.Path)
}

// noUpdate 返回动态端点的「无更新」响应（C18-3）：204、无 body、无
// Content-Type（HTTP 层不写 body）。
func noUpdate() *Response {
	return &Response{Status: tauriNoUpdateStatus}
}

// renderDynamic 实现动态端点：{target}/{arch}/{current_version} → 完整
// update.SelectTarget（匿名口径）→ 200 单平台对象 / 204（C18-3）。
//
// 与原生 check 的口径对齐：
//   - current_version 经 update.ResolveCurrent 解析（同一解析规则，C08-2）；
//     引用非法或目录中不存在 → 无更新语义（204）。Tauri 动态端点的 404 由
//     约定保留给开关 / 路径错误（task design 风险表：204=无更新，404 仅
//     开关/路径错误），未知当前版本对 feed 消费者而言就是「没有可下发的
//     更新」；
//   - hw_rev 强制空串 = 只匹配默认变体（C18-7：Tauri 无法表达 hw_rev，
//     绝不把非默认 hw 变体当成该平台的包下发；无默认变体 → 无候选 → 204）；
//   - 无 device_id / DeviceHash → 灰度只命中 100% 或强制版本（§4.4 匿名
//     口径，与 AnonymousVisible 同源）。
func (a *TauriAdapter) renderDynamic(ctx context.Context, deps *Deps, req Request, target, arch, currentVersion string) (*Response, error) {
	// os/arch 双向映射（C18-1）：darwin→macos（经平台别名表），arch 先经
	// CanonicalArch（aarch64→arm64）。未知 target / arch → 404。
	osName := platform.CanonicalOS(nil, target)
	if !tauriOSes[osName] {
		return nil, fmt.Errorf("%w: unknown tauri target %q", ErrUnknownPath, target)
	}
	archName := platform.CanonicalArch(arch)
	if !tauriArches[archName] {
		return nil, fmt.Errorf("%w: unknown tauri arch %q", ErrUnknownPath, arch)
	}

	cat, err := deps.Updates.LoadCatalog(ctx, req.Project.ID, osName, archName)
	if err != nil {
		return nil, err
	}
	// 渠道校验（§4.3）：未知渠道 400；渠道是分发权威，feed 只投影该渠道。
	if _, found := cat.Channel(req.Channel); !found {
		return nil, fmt.Errorf("%w: %q", ErrUnknownChannel, req.Channel)
	}

	// 当前版本解析（与原生 check 同一规则；未知/非法 → 无更新 204，见上）。
	current, err := update.ResolveCurrent(cat, strings.TrimSpace(currentVersion))
	if err != nil {
		return noUpdate(), nil
	}

	res, err := update.SelectTarget(cat, update.Request{
		Current: current,
		OS:      osName,
		Arch:    archName,
		// C18-7：Tauri 协议无 hw_rev 维度，强制默认变体口径。
		HwRev: "",
	})
	if err != nil {
		// 降级分支找不到安全目标（当前版本已吊销 / 本平台 yanked 且无
		// 更稳渠道）：对 feed 消费者等同「无可下发更新」→ 204。
		if errors.Is(err, update.ErrNoSafeTarget) {
			return noUpdate(), nil
		}
		return nil, err
	}
	if res.Target == nil {
		return noUpdate(), nil
	}

	// notes：目标 Version changelog 聚合（target_only，locale 回退链同
	// check，design §3）。
	notes, err := tauriNotes(cat, res.Target, osName, archName, req)
	if err != nil {
		return nil, err
	}

	// url：默认变体 kind=full 稳定文件名 URL（私有项目经短时签名，§13.7）。
	signing := urlSigningFromDeps(cat, deps)
	pkg, ok := resolveStorePackage(ctx, deps, req, res.Line, res.Package)
	if !ok || pkg == nil {
		return noUpdate(), nil
	}
	downloadURL := enclosureURL(signing, cat.Project.Slug, pkg.SHA256, pkg.StorageKey)

	sig, err := a.minisignSignature(ctx, deps, cat, pkg)
	if err != nil {
		return nil, err
	}

	body, err := json.Marshal(tauriUpdateDoc{
		Version:   versionDisplay(res.Target.Version),
		Notes:     notes,
		PubDate:   tauriPubDate(res.Target.Version),
		Signature: sig,
		URL:       downloadURL,
	})
	if err != nil {
		return nil, fmt.Errorf("feed: render tauri update doc: %w", err)
	}
	return &Response{Body: body, ContentType: tauriContentType, Status: 200}, nil
}

// tauriCombo 是静态 latest.json 枚举中的一个 (os, arch) 组合及其该渠道下
// 匿名可见的最新版本（含所属目录快照，供签名 / URL 装配）。
type tauriCombo struct {
	os   string
	arch string
	cat  *update.Catalog
	item VisibleItem
}

// renderStatic 实现静态 latest.json（C18-1）：枚举 Tauri 可用平台组合 →
// 各组合匿名可见最新版本 → 全局最新版本 → 顶层字段 + 该版本在各平台默认
// 变体产物的 platforms 表。可见集全空 → ErrNoRelease（404，不发空文档）。
func (a *TauriAdapter) renderStatic(ctx context.Context, deps *Deps, req Request) (*Response, error) {
	// 初始装载一次：req.OS / req.Arch 可能为空（真实 Tauri 客户端不带
	// query）。目录装载器对空平台返回无切片快照，但 EnabledOS / 渠道表
	// 来自全部矩阵行，恰好用于 os 枚举与渠道校验。
	firstArch := req.Arch
	if firstArch == "" {
		firstArch = "x86_64"
	}
	cat0, err := deps.Updates.LoadCatalog(ctx, req.Project.ID, req.OS, firstArch)
	if err != nil {
		return nil, err
	}
	// 渠道校验（§4.3）：未知渠道 400；渠道表全组合共享，校验一次即可。
	if _, found := cat0.Channel(req.Channel); !found {
		return nil, fmt.Errorf("%w: %q", ErrUnknownChannel, req.Channel)
	}

	// os 集合：显式 query 优先；否则取项目矩阵中 Tauri 可用的 os 全集。
	osSet := []string{req.OS}
	if req.OS == "" {
		osSet = osSet[:0]
		for _, osName := range cat0.EnabledOS {
			if tauriOSes[osName] {
				osSet = append(osSet, osName)
			}
		}
	}
	// arch 候选：显式 query 优先；否则两个主流 Tauri 架构（不存在的组合
	// 自然为空可见集，安全跳过）。
	archSet := []string{req.Arch}
	if req.Arch == "" {
		archSet = strings.Split(tauriStaticDefaultArches, ",")
	}

	// 枚举组合，各取匿名可见集（§4.4）中比较键最高者。
	combos := make([]tauriCombo, 0, len(osSet)*len(archSet))
	for _, osName := range osSet {
		for _, archName := range archSet {
			// 复用初始装载（同一组合），其余组合各自装载。
			var cat *update.Catalog
			if osName == req.OS && archName == firstArch {
				cat = cat0
			} else {
				cat, err = deps.Updates.LoadCatalog(ctx, req.Project.ID, osName, archName)
				if err != nil {
					return nil, err
				}
			}
			items := AnonymousVisible(cat, cat.Project.CompareEngine, req.Channel, osName, archName)
			if len(items) == 0 {
				continue
			}
			combos = append(combos, tauriCombo{os: osName, arch: archName, cat: cat, item: items[0]})
		}
	}
	if len(combos) == 0 {
		return nil, ErrNoRelease
	}

	// 全局最新：比较键最高（同键渠道 rank 高者）——顶层 version/notes/
	// pub_date 由此版本承担，platforms 只收录该版本可见的平台（design
	// 裁决：顶层版本与平台产物必须是同一版本）。
	latest := combos[0]
	for i := 1; i < len(combos); i++ {
		c, ok := update.CompareVersions(cat0.Project.CompareEngine,
			&combos[i].item.Version.Version, &latest.item.Version.Version)
		if ok && c > 0 {
			latest = combos[i]
			continue
		}
		if (!ok || c == 0) && combos[i].item.Rank > latest.item.Rank {
			latest = combos[i]
		}
	}

	// platforms：最新版本在各组合的默认 hw 变体产物（C18-7）。encoding/json
	// 对 map 键按字典序输出，文档字节确定 → ETag 稳定。
	platforms := make(map[string]tauriPlatform, len(combos))
	notesOS, notesArch := latest.os, latest.arch
	for _, combo := range combos {
		if combo.item.Version.Version.ID != latest.item.Version.Version.ID {
			continue
		}
		key, ok := tauriPlatformKey(combo.os, combo.arch)
		if !ok {
			continue
		}
		signing := urlSigningFromDeps(combo.cat, deps)
		pkg, ok := resolveStorePackage(ctx, deps, req, combo.item.Line, combo.item.Package)
		if !ok || pkg == nil {
			continue
		}
		sig, err := a.minisignSignature(ctx, deps, combo.cat, pkg)
		if err != nil {
			return nil, err
		}
		platforms[key] = tauriPlatform{
			Signature: sig,
			URL:       enclosureURL(signing, combo.cat.Project.Slug, pkg.SHA256, pkg.StorageKey),
		}
	}

	// notes：全局最新版本的 changelog 聚合（按其首个可见平台过滤
	// platform_notes 维度；聚合文本本身与平台无关）。
	notes, err := tauriNotes(latest.cat, latest.item.Version, notesOS, notesArch, req)
	if err != nil {
		return nil, err
	}

	body, err := json.Marshal(tauriStaticDoc{
		Version:   versionDisplay(latest.item.Version.Version),
		Notes:     notes,
		PubDate:   tauriPubDate(latest.item.Version.Version),
		Platforms: platforms,
	})
	if err != nil {
		return nil, fmt.Errorf("feed: render tauri static doc: %w", err)
	}
	return &Response{Body: body, ContentType: tauriContentType, Status: 200}, nil
}

// tauriNotes 计算目标版本的聚合 changelog（target_only，layout aggregated，
// locale 回退链 changelog_locale → locale → Accept-Language → 项目默认，
// 与 check / Sparkle 同口径）。
func tauriNotes(cat *update.Catalog, target *update.VersionState, osName, archName string, req Request) (string, error) {
	localeChain := update.BuildLocaleChain(req.ChangelogLocale, req.Locale, req.AcceptLanguage, cat.Project.DefaultLocale)
	notes, _, err := update.BuildChangelog(cat.Versions, target, target, target, update.ChangelogQuery{
		Engine:      cat.Project.CompareEngine,
		Scope:       model.ChangelogScopeTargetOnly,
		Layout:      model.ChangelogLayoutAggregated,
		LocaleChain: localeChain,
		OS:          osName,
		Arch:        archName,
	})
	if err != nil {
		return "", err
	}
	return notes, nil
}

// tauriPubDate 返回 RFC 3339（UTC）发布时间；未登记发布时间为空串（字段
// 省略）。
func tauriPubDate(v model.Version) string {
	if v.PublishTime == nil {
		return ""
	}
	return v.PublishTime.UTC().Format(time.RFC3339)
}

// tauriPlatformKey 把内部 (os, arch) 映射为 Tauri 官方平台键
// （macos→darwin、windows→windows、linux→linux；x86_64 原样、arm64→
// aarch64）。非 Tauri 平台返回 false。
func tauriPlatformKey(osName, archName string) (string, bool) {
	var tauriOS string
	switch osName {
	case "macos":
		tauriOS = "darwin"
	case "windows", "linux":
		tauriOS = osName
	default:
		return "", false
	}
	arch := archName
	if archName == "arm64" {
		arch = "aarch64"
	}
	return tauriOS + "-" + arch, true
}

// minisignSignature 计算安装器文件字节的 minisign 容器签名（C18-4）。
//
//   - 项目 SigningAlgo != ed25519 或未配置私钥 → 空串（省略 signature
//     字段；RSA 项目不冒充 Ed25519 签名）；
//   - 私钥已配置但无存储依赖（测试装配）→ 空串（文档降级为未签名）；
//   - 读取 / 签名失败 → error（宁可 500 也不输出不可验签的更新地址）。
//
// 容器字节格式（§12.1 隔离）：base64( "Ed" + key_id(8 字节全 0) +
// Ed25519 签名(64 字节) )，即 minisign 的 74 字节二进制签名块；与原生
// check 的 base64 裸签名（64 字节、签元数据载荷）和 sparkle:edSignature
// （base64 裸签名、签文件字节）是三种互不相同的字节格式。签名本体对
// 文件原始字节计算（与 Sparkle 同语义、不同容器）；按产物 SHA-256 走
// LRU 缓存，首请求读全量文件签名一次，后续 O(1)。
func (a *TauriAdapter) minisignSignature(ctx context.Context, deps *Deps, cat *update.Catalog, pkg *update.ArtifactInfo) (string, error) {
	if cat.Project.SigningAlgo != signature.AlgoEd25519 || cat.Project.SigningPrivateKey == "" {
		return "", nil
	}
	if deps.Storage == nil || pkg.StorageKey == "" {
		return "", nil
	}
	return a.sigCache.GetOrCompute(pkg.SHA256, func() (string, error) {
		rc, err := deps.Storage.Get(ctx, pkg.StorageKey)
		if err != nil {
			return "", fmt.Errorf("feed: read artifact for tauri signature: %w", err)
		}
		defer rc.Close()
		data, err := io.ReadAll(rc)
		if err != nil {
			return "", fmt.Errorf("feed: read artifact bytes for tauri signature: %w", err)
		}
		// Ed25519 对文件原始字节签名（base64 std 裸签名，64 字节）。
		sigB64, err := signature.SignPayload(signature.AlgoEd25519, cat.Project.SigningPrivateKey, string(data))
		if err != nil {
			return "", fmt.Errorf("feed: sign artifact for tauri signature: %w", err)
		}
		return renderMinisignContainer(sigB64)
	})
}

// renderMinisignContainer 把 base64 裸 Ed25519 签名包装为 minisign 容器
// 文本：untrusted comment 行 + base64(74 字节签名块) 行。
func renderMinisignContainer(sigB64 string) (string, error) {
	raw, err := base64.StdEncoding.DecodeString(sigB64)
	if err != nil {
		return "", fmt.Errorf("feed: decode ed25519 signature: %w", err)
	}
	if len(raw) != ed25519.SignatureSize {
		return "", fmt.Errorf("feed: ed25519 signature size %d, want %d", len(raw), ed25519.SignatureSize)
	}
	// minisign 二进制签名块：算法标识 "Ed" + 8 字节 key_id（本服务未启用
	// key id 语义，恒全 0）+ 64 字节 Ed25519 签名。
	block := make([]byte, 0, 2+8+len(raw))
	block = append(block, 'E', 'd')
	block = append(block, make([]byte, 8)...)
	block = append(block, raw...)
	var b strings.Builder
	b.WriteString(tauriMinisignComment + "\n")
	b.WriteString(base64.StdEncoding.EncodeToString(block) + "\n")
	return b.String(), nil
}

// tauriUpdateDoc 是动态端点的单平台更新对象（Tauri 官方字段集；struct
// 顺序 = JSON 输出顺序）。signature 为空时省略字段（未签名项目不冒充）。
type tauriUpdateDoc struct {
	Version   string `json:"version"`
	Notes     string `json:"notes,omitempty"`
	PubDate   string `json:"pub_date,omitempty"`
	Signature string `json:"signature,omitempty"`
	URL       string `json:"url"`
}

// tauriPlatform 是 platforms 表中单平台的下载描述（官方字段集）。
type tauriPlatform struct {
	Signature string `json:"signature,omitempty"`
	URL       string `json:"url"`
}

// tauriStaticDoc 是静态 latest.json 的多平台文档（Tauri v2 官方 schema）。
type tauriStaticDoc struct {
	Version   string                   `json:"version"`
	Notes     string                   `json:"notes,omitempty"`
	PubDate   string                   `json:"pub_date,omitempty"`
	Platforms map[string]tauriPlatform `json:"platforms"`
}
