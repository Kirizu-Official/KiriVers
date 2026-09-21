package store

import (
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/Kirizu-Official/KiriVers/internal/model"
	"github.com/Kirizu-Official/KiriVers/internal/service/update"
	"github.com/Kirizu-Official/KiriVers/pkg/signature"
)

// Sparkle appcast 投影（docs/app-init.md §9 Sparkle 行 / §9.2，C16-1..C16-8）。
//
// 产出 RSS 2.0 + sparkle 命名空间的 appcast.xml，WinSparkle / NetSparkle 共用：
//   - 双号映射（已拍板，不跟 compare_engine 走）：sparkle:version =
//     version_integer（构建号，空则省略元素）；sparkle:shortVersionString =
//     version_semver（营销号，空则省略，C16-2）；
//   - item 集合 = 该渠道 + os + arch 的匿名可见版本（AnonymousVisible），
//     比较键新→旧；
//   - enclosure 指向该线默认 hw 变体 kind=full 产物的稳定文件名 URL
//     （§5.9；私有项目经短时签名，§13.7），type = 产物登记的 Content-Type
//     （C16-3）；Sparkle 单 enclosure 语义 → 非默认 hw 变体不生成额外
//     enclosure（C16-8，design 裁决：不并列多 URL）；
//   - description = 该 Version changelog 按项目默认口径聚合的一段文本
//     （item 级 target_only，语言回退链同 check，C16-6）；
//   - sparkle:edSignature = Ed25519 对 enclosure 文件字节的签名（Sparkle
//     语义：签文件而非元数据，§12.1——不复用原生 check signature 字节格式，
//     不进任何 URL）；结果按产物 SHA-256 做 LRU 缓存（见 SignatureCache）。
//     RSA 项目不发 sparkle:dsaSignature：本服务不支持 DSA 密钥，Sparkle 2
//     主推 EdDSA，DSA 项省略即未签名。
const (
	// SparkleNamespace 是 Sparkle 扩展元素的官方命名空间。
	SparkleNamespace = "http://www.andymatuschak.org/xml-namespaces/sparkle"
	// DCNamespace 是 Dublin Core 命名空间（appcast 惯例声明）。
	DCNamespace = "http://purl.org/dc/elements/1.1/"

	// sparkleContentType 是 appcast 文档的 MIME。
	sparkleContentType = "application/xml; charset=utf-8"

	// rfC822Layout 是 pubDate 的时间格式（RFC 822/1123Z，Sparkle 惯例）。
	rfC822Layout = time.RFC1123Z

	// stableChannel 不标注 sparkle:channel（Sparkle 约定：无该元素即默认渠道）。
	stableChannel = "stable"
)

// SparkleAdapter 是 Sparkle appcast 协议的 Adapter 实现。
type SparkleAdapter struct {
	// sigCache 缓存「产物字节 → Ed25519 签名」，避免每次请求读取全量文件。
	sigCache *SignatureCache
}

// compile-time 校验：storage.Backend / update 包产物快照满足依赖合同。
var (
	_ Adapter = (*SparkleAdapter)(nil)
)

// NewSparkleAdapter 构造 Sparkle Adapter（默认容量签名缓存）。
func NewSparkleAdapter() *SparkleAdapter {
	return &SparkleAdapter{sigCache: NewSignatureCache(0)}
}

// NewSparkleAdapterWithCache 以给定签名缓存构造（测试可注入）。
func NewSparkleAdapterWithCache(cache *SignatureCache) *SparkleAdapter {
	return &SparkleAdapter{sigCache: cache}
}

// Protocol 实现 Adapter。
func (a *SparkleAdapter) Protocol() string { return ProtocolSparkle }

// Enabled 实现 Adapter：StoreProtocols["sparkle"].Enabled（C16-4）。
func (a *SparkleAdapter) Enabled(p *model.Project) bool {
	return enabledStoreProtocol(p, ProtocolSparkle)
}

// sparkleItem 是单个 appcast item 的已解析字段（nil 指针 = 省略元素）。
type sparkleItem struct {
	Title            string
	PubDate          string // RFC 1123Z；空串省略
	VersionInteger   *int64 // sparkle:version（空则省略）
	VersionSemver    *string
	OS               string
	Channel          string // stable 时不输出
	MinimumSystemVer string // Line min_os；空串省略
	Description      string
	EnclosureURL     string
	EnclosureLength  int64
	EnclosureType    string
	EdSignature      string // 空 = 不输出（未配置 ed25519 或跳过）
}

// Render 实现 Adapter：加载目录 → 匿名可见集 → 组装 appcast.xml。
func (a *SparkleAdapter) Render(ctx context.Context, deps *Deps, req Request) (*Response, error) {
	if deps == nil || deps.Updates == nil {
		return nil, fmt.Errorf("feed: update service unavailable")
	}
	if req.Project == nil {
		return nil, fmt.Errorf("%w: project is required", ErrMissingParam)
	}
	if req.OS == "" || req.Arch == "" {
		return nil, fmt.Errorf("%w: os and arch are required", ErrMissingParam)
	}

	cat, err := deps.Updates.LoadCatalog(ctx, req.Project.ID, req.OS, req.Arch)
	if err != nil {
		return nil, err
	}
	var items []VisibleItem
	if req.Listing != nil && listingPin(req.Listing, "channel") == "" {
		items = AnonymousVisibleAllPublic(cat, cat.Project.CompareEngine, req.OS, req.Arch)
	} else {
		if _, found := cat.Channel(req.Channel); !found {
			return nil, fmt.Errorf("%w: %q", ErrUnknownChannel, req.Channel)
		}
		items = AnonymousVisible(cat, cat.Project.CompareEngine, req.Channel, req.OS, req.Arch)
	}
	// 私有项目下载 URL 签名装配（§13.7 / C15-1）：公开项目零值原样返回。
	signing := urlSigningFromDeps(cat, deps)

	localeChain := update.BuildLocaleChain(req.ChangelogLocale, req.Locale, req.AcceptLanguage, cat.Project.DefaultLocale)

	rendered := make([]sparkleItem, 0, len(items))
	for _, it := range items {
		// description：该 Version changelog 按 target_only 聚合一段文本
		//（C16-6；语言回退链与 check 相同：changelog_locale → locale →
		// Accept-Language 首段 → 项目 default_locale）。
		desc, _, err := update.BuildChangelog(cat.Versions, it.Version, it.Version, it.Version, update.ChangelogQuery{
			Engine:      cat.Project.CompareEngine,
			Scope:       model.ChangelogScopeTargetOnly,
			Layout:      model.ChangelogLayoutAggregated,
			LocaleChain: localeChain,
			OS:          req.OS,
			Arch:        req.Arch,
		})
		if err != nil {
			return nil, err
		}

		// item 至少要有一种版本表达（双号全空的 Published 版本不投影）。
		if it.Version.Version.VersionInteger == nil && it.Version.Version.VersionSemverCanonical == nil {
			continue
		}

		pkg, ok := resolveStorePackage(ctx, deps, req, it.Line, it.Package)
		if !ok || pkg == nil {
			continue
		}
		encType := pkg.ContentType
		if encType == "" {
			encType = "application/octet-stream"
		}
		encURL := enclosureURL(signing, cat.Project.Slug, pkg.SHA256, pkg.StorageKey)

		item := sparkleItem{
			Title:            versionDisplay(it.Version.Version),
			OS:               req.OS,
			Channel:          it.Version.Version.ChannelSlug,
			MinimumSystemVer: derefString(it.Line.MinOS),
			Description:      desc,
			EnclosureURL:     encURL,
			EnclosureLength:  pkg.Size,
			EnclosureType:    encType,
		}
		item.VersionInteger = it.Version.Version.VersionInteger
		item.VersionSemver = it.Version.Version.VersionSemverCanonical
		if it.Version.Version.PublishTime != nil {
			item.PubDate = it.Version.Version.PublishTime.UTC().Format(rfC822Layout)
		}
		if item.Channel == stableChannel {
			item.Channel = ""
		}

		sig, err := a.edSignature(ctx, deps, cat, pkg)
		if err != nil {
			return nil, err
		}
		item.EdSignature = sig

		rendered = append(rendered, item)
	}
	if len(rendered) == 0 {
		return nil, ErrNoRelease
	}

	body := renderSparkleXML(cat, req, rendered)
	return &Response{Body: body, ContentType: sparkleContentType, Status: 200}, nil
}

// edSignature 计算 enclosure 文件字节的 Ed25519 签名（Sparkle EdDSA 语义）。
//
//   - 项目 SigningAlgo != ed25519 或未配置私钥 → 返回空串（不输出元素；
//     RSA 项目不支持 DSA/EdDSA 文件签名，见类型级注释）；
//   - 私钥已配置但无存储依赖（测试装配）→ 返回空串（文档降级为未签名，
//     不因签名缺失拒绝整个 feed）；
//   - 读取 / 签名失败 → 返回 error（宁可 500 也不输出不可验签的 appcast）。
//
// 签名按产物 SHA-256（字节身份）走 LRU 缓存：首请求读全量文件签名一次，
// 后续请求 O(1)。
func (a *SparkleAdapter) edSignature(ctx context.Context, deps *Deps, cat *update.Catalog, pkg *update.ArtifactInfo) (string, error) {
	if cat.Project.SigningAlgo != signature.AlgoEd25519 || cat.Project.SigningPrivateKey == "" {
		return "", nil
	}
	if deps.Storage == nil || pkg.StorageKey == "" {
		return "", nil
	}
	return a.sigCache.GetOrCompute(pkg.SHA256, func() (string, error) {
		rc, err := deps.Storage.Get(ctx, pkg.StorageKey)
		if err != nil {
			return "", fmt.Errorf("feed: read artifact for edSignature: %w", err)
		}
		defer rc.Close()
		data, err := io.ReadAll(rc)
		if err != nil {
			return "", fmt.Errorf("feed: read artifact bytes for edSignature: %w", err)
		}
		// Ed25519 对文件原始字节签名；base64(std) 输出与 Sparkle 2 EdDSA 一致。
		return signature.SignPayload(signature.AlgoEd25519, cat.Project.SigningPrivateKey, string(data))
	})
}

// renderSparkleXML 组装 appcast.xml 文本。手写 XML（encoding/xml 的命名空间
// 前缀控制能力不足）；所有动态文本经 xmlEscape 转义。
func renderSparkleXML(cat *update.Catalog, req Request, items []sparkleItem) []byte {
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="utf-8"?>` + "\n")
	b.WriteString(`<rss version="2.0" xmlns:sparkle="` + SparkleNamespace + `" xmlns:dc="` + DCNamespace + `">` + "\n")
	b.WriteString("  <channel>\n")
	// channel 元信息：title 用项目 slug（Project 无独立显示名）。
	b.WriteString("    <title>" + xmlEscape(cat.Project.Slug) + "</title>\n")
	b.WriteString("    <link>" + xmlEscape(sparkleChannelLink(cat, req)) + "</link>\n")
	b.WriteString("    <description>" + xmlEscape("Update feed for project "+cat.Project.Slug) + "</description>\n")
	for i := range items {
		it := &items[i]
		b.WriteString("    <item>\n")
		writeElement(&b, "      ", "title", it.Title)
		writeElement(&b, "      ", "pubDate", it.PubDate)
		if it.VersionInteger != nil {
			writeElement(&b, "      ", "sparkle:version", fmtInt(*it.VersionInteger))
		}
		if it.VersionSemver != nil && *it.VersionSemver != "" {
			writeElement(&b, "      ", "sparkle:shortVersionString", *it.VersionSemver)
		}
		writeElement(&b, "      ", "sparkle:os", it.OS)
		writeElement(&b, "      ", "sparkle:channel", it.Channel)
		writeElement(&b, "      ", "sparkle:minimumSystemVersion", it.MinimumSystemVer)
		writeElement(&b, "      ", "description", it.Description)
		// enclosure 与文件签名成对出现：有 EdDSA 签名时元素紧随 enclosure
		//（Sparkle 2 约定位置，WinSparkle/NetSparkle 对顺序宽容）。
		// 属性值同样转义（私有签名 URL 含 &，必须写成 &amp;）。
		b.WriteString(`      <enclosure url="` + xmlEscape(it.EnclosureURL) +
			`" length="` + fmtInt(it.EnclosureLength) +
			`" type="` + xmlEscape(it.EnclosureType) + `"/>` + "\n")
		writeElement(&b, "      ", "sparkle:edSignature", it.EdSignature)
		b.WriteString("    </item>\n")
	}
	b.WriteString("  </channel>\n")
	b.WriteString("</rss>\n")
	return []byte(b.String())
}

// sparkleChannelLink 返回 channel/link：优先协议标识袋中的 homepage，
// 缺省回落本实例的 appcast 路径（RSS 必备 link 元素）。
func sparkleChannelLink(cat *update.Catalog, req Request) string {
	ids := listingIdentifiers(req)
	if hp := strings.TrimSpace(ids["homepage"]); hp != "" {
		return hp
	}
	return "/api/v1/projects/" + cat.Project.Slug + "/store/" + ProtocolSparkle + "/" + listingSlug(req) + "/appcast.xml"
}

// writeElement 写出 <name>escaped</name>；text 为空串时整元素省略
// （可选元素的双号/签名等语义由此统一）。
func writeElement(b *strings.Builder, indent, name, text string) {
	if text == "" {
		return
	}
	b.WriteString(indent + "<" + name + ">" + xmlEscape(text) + "</" + name + ">\n")
}

// urlSigningFor 按目录快照的项目可见性返回生效的签名装配（与 update 包
// signingFor 同口径）：私有项目 = {Signer, 项目级 TTL}；公开项目或未注入
// 签名器为零值（sign 恒原样返回）。
type urlSigning struct {
	signer     update.URLSigner
	ttl        int
	objectURL  func(slug, sha256, storageKey string) string
	localProxy bool
}

func urlSigningFor(cat *update.Catalog, signer update.URLSigner) urlSigning {
	u := urlSigning{}
	if signer == nil || cat.Project.StorageVisibility != model.StorageVisibilityPrivate {
		return u
	}
	u.signer = signer
	u.ttl = cat.Project.SignedURLTTLSeconds
	return u
}

func urlSigningFromDeps(cat *update.Catalog, deps *Deps) urlSigning {
	u := urlSigning{}
	if deps != nil {
		u.objectURL = deps.ObjectURL
		u.localProxy = deps.LocalProxy
	}
	if deps == nil || deps.Signer == nil || cat.Project.StorageVisibility != model.StorageVisibilityPrivate {
		return u
	}
	u.signer = deps.Signer
	u.ttl = cat.Project.SignedURLTTLSeconds
	return u
}

// sign 对下载路径签名；绝对 http(s) URL 原样返回。
func (u urlSigning) sign(path string) string {
	if strings.HasPrefix(path, "http://") || strings.HasPrefix(path, "https://") {
		return path
	}
	if u.signer == nil {
		return path
	}
	return u.signer.SignDownload(path, u.ttl)
}

func (u urlSigning) artifactURL(slug, sha256, storageKey string) string {
	raw := update.PackageDownloadURL(slug, sha256)
	if u.objectURL != nil {
		if alt := u.objectURL(slug, sha256, storageKey); alt != "" {
			raw = alt
		}
	}
	return u.sign(raw)
}

// xmlEscape 用 encoding/xml 的文本转义规则处理动态内容（&、<、>、引号等）。
func xmlEscape(s string) string {
	var b strings.Builder
	_ = xml.EscapeText(&b, []byte(s))
	return b.String()
}

// versionDisplay 返回 item 标题用版本串：SemVer 优先，否则整数构建号。
func versionDisplay(v model.Version) string {
	if v.VersionSemverCanonical != nil && *v.VersionSemverCanonical != "" {
		return *v.VersionSemverCanonical
	}
	if v.VersionInteger != nil {
		return fmtInt(*v.VersionInteger)
	}
	return ""
}

// derefString 安全取字符串指针内容。
func derefString(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// fmtInt 整数转十进制字符串。
func fmtInt(n int64) string {
	return fmt.Sprintf("%d", n)
}
