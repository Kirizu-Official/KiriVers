package store

import (
	"context"
	"crypto/sha512"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/Kirizu-Official/KiriVers/internal/model"
	"github.com/Kirizu-Official/KiriVers/internal/service"
	"github.com/Kirizu-Official/KiriVers/internal/service/update"
)

// electron-updater generic provider 投影（docs/app-init.md §9 electron 行 /
// §7.2 / §5.8，C17-1..C17-7）。
//
// 产出 electron-updater 可直接消费的 latest*.yml（generic provider）：
//   - 路由即平台：latest.yml→windows、latest-mac.yml→macos、
//     latest-linux.yml→linux（C17-1）；未知文件名 → ErrUnknownPath（404）；
//   - latest 型 feed：该渠道 + os + arch 匿名可见集（AnonymousVisible）中
//     比较键最高者；可见集为空 → ErrNoRelease（404，不发空 yml）；
//   - version = version_semver，为空则 version_integer 的十进制字符串
//     （C17-2）；
//   - path = 默认 hw 变体 kind=full 产物的 §5.9 稳定文件名（C17-3 / C17-7：
//     yml 无 hw_rev 维度，只投影默认变体）；
//   - sha512 = base64(原始 512-bit 摘要)（electron-updater 校验语义，§5.8）。
//     产物已登记 SHA512（hex）→ 解码转 base64；为空（旧行）→ 首请求时从
//     存储流式计算并按产物 SHA-256 走单飞 + LRU 缓存（复用 SignatureCache，
//     独立实例）。刻意不回填数据库：feed 读路径无写权限语义，且 SHA512 可
//     在下一次产物管理操作时自然补全，回填会让读路径依赖写库可用性；
//   - files 数组（electron-updater ≥6 读取）：url 指向 /packages/ 稳定下载
//     URL（私有项目经短时签名，§13.7），sha512/size 与顶层字段一致；
//   - blockmap（C17-4 / §7.2）：yml 中**不**输出任何字段——electron-updater
//     按 path + ".blockmap" URL 约定自动探测，经既有 /packages/{sha256}
//     路由原样下载（字节不变）。blockmap 产物为 kind=file，天然不进入
//     diff/binary_delta 选择（后者只消费 kind=delta/patch，见 update.Diff）。
const electronContentType = "application/yaml; charset=utf-8"

// electronFileNames 是 electron feed 文档路径 → os 的固定映射（C17-1）。
// 键即 electron-updater generic provider 的约定文件名。
var electronFileNames = map[string]string{
	"latest.yml":       "windows",
	"latest-mac.yml":   "macos",
	"latest-linux.yml": "linux",
}

// ElectronAdapter 是 electron-updater generic provider 协议的 Adapter 实现。
type ElectronAdapter struct {
	// sha512Cache 缓存「产物字节 → base64(SHA-512 摘要)」，避免每次请求
	// 读取全量文件（键 = 产物 SHA-256，与 Sparkle 签名缓存同一模式、独立实例）。
	sha512Cache *SignatureCache
}

// compile-time 校验：满足 Adapter 合同。
var _ Adapter = (*ElectronAdapter)(nil)

// NewElectronAdapter 构造 Electron Adapter（默认容量 SHA-512 缓存）。
func NewElectronAdapter() *ElectronAdapter {
	return &ElectronAdapter{sha512Cache: NewSignatureCache(0)}
}

// NewElectronAdapterWithCache 以给定缓存构造（测试可注入）。
func NewElectronAdapterWithCache(cache *SignatureCache) *ElectronAdapter {
	return &ElectronAdapter{sha512Cache: cache}
}

// Protocol 实现 Adapter。
func (a *ElectronAdapter) Protocol() string { return ProtocolElectron }

// Enabled 实现 Adapter：StoreProtocols["electron"].Enabled（C17-5）。
func (a *ElectronAdapter) Enabled(p *model.Project) bool {
	return enabledStoreProtocol(p, ProtocolElectron)
}

// Render 实现 Adapter：解析文档路径 → 加载目录 → 匿名可见集取最新 → 组装
// latest*.yml。os 由文件名决定（请求 os query 不参与，C17-1）；arch 取请求
// query（缺省 x86_64——yml 本身无 arch 维度，arch 由发布者拼入 feed URL）。
func (a *ElectronAdapter) Render(ctx context.Context, deps *Deps, req Request) (*Response, error) {
	if deps == nil || deps.Updates == nil {
		return nil, fmt.Errorf("feed: update service unavailable")
	}
	if req.Project == nil {
		return nil, fmt.Errorf("%w: project is required", ErrMissingParam)
	}
	osName, ok := electronFileNames[req.Path]
	if !ok {
		return nil, fmt.Errorf("%w: %q", ErrUnknownPath, req.Path)
	}
	if pin := listingPin(req.Listing, "os"); pin != "" {
		osName = pin
	}
	arch := req.Arch
	if arch == "" {
		arch = "x86_64"
	}

	cat, err := deps.Updates.LoadCatalog(ctx, req.Project.ID, osName, arch)
	if err != nil {
		return nil, err
	}
	// 渠道校验（§4.3）：未知渠道 400；渠道是分发权威，feed 只投影该渠道。
	if _, found := cat.Channel(req.Channel); !found {
		return nil, fmt.Errorf("%w: %q", ErrUnknownChannel, req.Channel)
	}

	// latest 选择：匿名可见集（比较键新→旧）中第一个有版本表达者。
	items := AnonymousVisible(cat, cat.Project.CompareEngine, req.Channel, osName, arch)
	var chosen *VisibleItem
	for i := range items {
		v := &items[i].Version.Version
		if v.VersionInteger != nil || (v.VersionSemverCanonical != nil && *v.VersionSemverCanonical != "") {
			chosen = &items[i]
			break
		}
	}
	if chosen == nil {
		return nil, ErrNoRelease
	}

	// 私有项目下载 URL 签名装配（§13.7 / C15-1）：公开项目零值原样返回。
	signing := urlSigningFromDeps(cat, deps)

	v := chosen.Version.Version
	pkg, ok := resolveStorePackage(ctx, deps, req, chosen.Line, chosen.Package)
	if !ok || pkg == nil {
		return nil, ErrNoRelease
	}
	sha512B64, err := a.sha512For(ctx, deps, pkg)
	if err != nil {
		return nil, err
	}
	downloadURL := enclosureURL(signing, cat.Project.Slug, pkg.SHA256, pkg.StorageKey)

	releaseDate := ""
	if v.PublishTime != nil {
		releaseDate = v.PublishTime.UTC().Format(time.RFC3339)
	}

	body := renderElectronYAML(electronYML{
		Version:     versionDisplay(v),
		Path:        service.DecoratedPackageName(pkg.SHA256, pkg.FileName),
		SHA512:      sha512B64,
		Size:        pkg.Size,
		ReleaseDate: releaseDate,
		FileURL:     downloadURL,
	})
	return &Response{Body: body, ContentType: electronContentType, Status: 200}, nil
}

// sha512For 返回 electron-updater 语义的 sha512 值：base64(原始 512-bit 摘要)。
//
//   - 产物已登记 SHA512（hex，上传时多重哈希写入）→ hex 解码转 base64；
//   - 为空（旧行 / 未登记）→ 从存储流式计算（首请求一次，按产物 SHA-256
//     走单飞 + LRU）；不回填数据库（见类型级注释）；
//   - 无存储依赖（测试装配）→ 返回空串（yml 缺省该字段；electron-updater
//     会拒绝无 sha512 的更新，但不因元数据缺失拒绝整个 feed 的可用性）。
//
// 读取 / 哈希失败 → 返回 error（宁可 500 也不输出不可校验的更新地址）。
func (a *ElectronAdapter) sha512For(ctx context.Context, deps *Deps, pkg *update.ArtifactInfo) (string, error) {
	if pkg.SHA512 != "" {
		raw, err := hex.DecodeString(pkg.SHA512)
		if err != nil {
			return "", fmt.Errorf("feed: artifact sha512 not hex: %w", err)
		}
		return base64.StdEncoding.EncodeToString(raw), nil
	}
	if deps.Storage == nil || pkg.StorageKey == "" {
		return "", nil
	}
	return a.sha512Cache.GetOrCompute(pkg.SHA256, func() (string, error) {
		rc, err := deps.Storage.Get(ctx, pkg.StorageKey)
		if err != nil {
			return "", fmt.Errorf("feed: read artifact for sha512: %w", err)
		}
		defer rc.Close()
		// 流式哈希：不整读进内存（产物量级可达数百 MB）。
		h := sha512.New()
		if _, err := io.Copy(h, rc); err != nil {
			return "", fmt.Errorf("feed: hash artifact for sha512: %w", err)
		}
		return base64.StdEncoding.EncodeToString(h.Sum(nil)), nil
	})
}

// electronYML 是 latest*.yml 的投影字段集合（electron-updater generic
// provider 文档字段集；files 数组恒单条，与顶层三元组一致）。
type electronYML struct {
	Version     string
	Path        string
	SHA512      string
	Size        int64
	ReleaseDate string
	FileURL     string
}

// renderElectronYAML 手写渲染 latest*.yml。不引 YAML 库（go.mod 无直接依赖，
// 且手写可固定字段顺序与省略语义）；所有字符串值经 yamlScalar 保证可解析。
func renderElectronYAML(d electronYML) []byte {
	var b strings.Builder
	b.WriteString("version: " + yamlScalar(d.Version) + "\n")
	b.WriteString("path: " + yamlScalar(d.Path) + "\n")
	if d.SHA512 != "" {
		b.WriteString("sha512: " + yamlScalar(d.SHA512) + "\n")
	}
	b.WriteString("size: " + strconv.FormatInt(d.Size, 10) + "\n")
	if d.ReleaseDate != "" {
		b.WriteString("releaseDate: " + yamlScalar(d.ReleaseDate) + "\n")
	}
	b.WriteString("files:\n")
	b.WriteString("  - url: " + yamlScalar(d.FileURL) + "\n")
	if d.SHA512 != "" {
		b.WriteString("    sha512: " + yamlScalar(d.SHA512) + "\n")
	}
	b.WriteString("    size: " + strconv.FormatInt(d.Size, 10) + "\n")
	return []byte(b.String())
}

// yamlNeedsQuote 判断字符串是否必须加引号才能保持字符串语义：数值/布尔/null
// 形态会被 YAML 解析器强转类型（electron-updater 将 version 读作字符串，
// 整数构建号 "10" 若不加引号会被解析成数字）；含 YAML 指示符的值同理。
func yamlNeedsQuote(v string) bool {
	if v == "" {
		return true
	}
	if _, err := strconv.ParseFloat(v, 64); err == nil {
		return true
	}
	switch strings.ToLower(v) {
	case "true", "false", "yes", "no", "on", "off", "null", "~":
		return true
	}
	if strings.ContainsAny(v, ":#{}[],&*!|>'\"%@`") || strings.Contains(v, " #") {
		return true
	}
	// 指示符开头的 plain scalar 有特殊解读风险（- 序列、? 映射键等），一律加引号。
	switch v[0] {
	case '-', '?', ':':
		return true
	}
	return false
}

// yamlScalar 按需加双引号渲染 YAML 字符串标量（strconv.Quote 的转义产物是
// 合法 YAML 双引号标量；本文档值均为 ASCII）。
func yamlScalar(v string) string {
	if yamlNeedsQuote(v) {
		return strconv.Quote(v)
	}
	return v
}
