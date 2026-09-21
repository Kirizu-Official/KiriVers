package store

import (
	"context"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"path/filepath"
	"strings"

	semverpkg "github.com/Masterminds/semver/v3"

	"github.com/Kirizu-Official/KiriVers/internal/model"
	"github.com/Kirizu-Official/KiriVers/internal/platform"
	"github.com/Kirizu-Official/KiriVers/internal/service"
)

// ClickOnce .application 部署清单投影（docs/app-init.md §9 ClickOnce 行 / §9.2，C20-1..C20-6）。
//
// 产出 ClickOnce 可直接消费的 XML 部署清单：
//   - 端点：GET /store/clickonce/default/{path...}?channel=&arch=；
//   - 仅运行于 Windows；arch 缺省回退至 x86_64；
//   - 文档路径：接受以 .application 结尾的文件名（如 app.application 或 {slug}.application），
//     非 .application 结尾返回 ErrUnknownPath（404）；
//   - MIME：application/x-ms-application; charset=utf-8；
//   - 版本映射（C20-2）：ClickOnce assemblyIdentity 要求 4 段式整数字符串 Major.Minor.Build.Revision：
//     有 version_semver 时取 SemVer 的 Major.Minor.Patch，第 4 位优先使用 0..65535 的 version_integer（空或超限则为 0）；
//     无 version_semver 仅有 version_integer 时映射为 1.0.0.{version_integer % 65536}；
//   - processorArchitecture 映射：x86_64→amd64、x86/i386/386→x86、arm64→arm64、其余→msil；
//   - 依赖与摘要：dependentAssembly 指向最新匿名可见版本的默认 hw 变体全量包（§5.9 稳定文件名 URL，
//     私有项目经短时签名），携带 SHA-256 DigestValue（base64）与文件大小；
//   - 可见性与硬件变体（C20-3, C20-6）：按 AnonymousVisible 仅投影默认 hw 变体（非默认 hw 或 MCU 变体绝不进入清单）；
//     灰度未 100% 且非关键版本不可见；可见集为空返回 ErrNoRelease（404，不发空文件或通用 zip）；
//   - 开关与包标识（C20-2, C20-4）：StoreProtocols["clickonce"].Enabled 控制；
//     StoreProtocols["clickonce"].Identifiers 可配置 application_name、publisher、product、
//     public_key_token、deployment_provider 等字段，未配置时自动使用安全缺省值。
const (
	// ProtocolClickOnce 是 ClickOnce 协议名（Project.StoreProtocols 的 jsonb 键）。
	ProtocolClickOnce = "clickonce"

	// clickOnceContentType 是 ClickOnce 部署清单文档的 MIME。
	clickOnceContentType = "application/x-ms-application; charset=utf-8"

	// clickOnceDefaultArch 是未提供 arch query 时的默认架构。
	clickOnceDefaultArch = "x86_64"

	// clickOnceDefaultToken 是未指定公钥指纹时的默认全 0 token。
	clickOnceDefaultToken = "0000000000000000"
)

// ClickOnceAdapter 是 ClickOnce .application 协议的 Adapter 实现。
type ClickOnceAdapter struct{}

// compile-time 校验：满足 Adapter 合同。
var _ Adapter = (*ClickOnceAdapter)(nil)

// NewClickOnceAdapter 构造 ClickOnce Adapter。
func NewClickOnceAdapter() *ClickOnceAdapter {
	return &ClickOnceAdapter{}
}

// Protocol 实现 Adapter。
func (a *ClickOnceAdapter) Protocol() string { return ProtocolClickOnce }

// Enabled 实现 Adapter：StoreProtocols["clickonce"].Enabled（C20-4）。
func (a *ClickOnceAdapter) Enabled(p *model.Project) bool {
	return enabledStoreProtocol(p, ProtocolClickOnce)
}

// Render 实现 Adapter：校验文档路径 → 加载 windows 目录 → 匿名可见集取最新 → 组装 .application XML。
func (a *ClickOnceAdapter) Render(ctx context.Context, deps *Deps, req Request) (*Response, error) {
	if deps == nil || deps.Updates == nil {
		return nil, fmt.Errorf("feed: update service unavailable")
	}
	if req.Project == nil {
		return nil, fmt.Errorf("%w: project is required", ErrMissingParam)
	}

	path := strings.Trim(req.Path, "/")
	if !strings.HasSuffix(strings.ToLower(path), ".application") {
		return nil, fmt.Errorf("%w: %q (must end with .application)", ErrUnknownPath, req.Path)
	}

	arch := req.Arch
	if arch == "" {
		arch = clickOnceDefaultArch
	}
	arch = platform.CanonicalArch(arch)

	const osName = "windows"
	cat, err := deps.Updates.LoadCatalog(ctx, req.Project.ID, osName, arch)
	if err != nil {
		return nil, err
	}
	if _, found := cat.Channel(req.Channel); !found {
		return nil, fmt.Errorf("%w: %q", ErrUnknownChannel, req.Channel)
	}

	items := AnonymousVisible(cat, cat.Project.CompareEngine, req.Channel, osName, arch)
	if len(items) == 0 {
		return nil, ErrNoRelease
	}

	// ClickOnce deployment manifest 投影当前可见最新版本
	chosen := &items[0]
	v := &chosen.Version.Version
	pkg, ok := resolveStorePackage(ctx, deps, req, chosen.Line, chosen.Package)
	if !ok || pkg == nil {
		return nil, ErrNoRelease
	}

	rawSHA, err := hex.DecodeString(pkg.SHA256)
	if err != nil {
		return nil, fmt.Errorf("feed: invalid artifact sha256 hex: %w", err)
	}
	sha256B64 := base64.StdEncoding.EncodeToString(rawSHA)

	signing := urlSigningFromDeps(cat, deps)
	downloadURL := enclosureURL(signing, cat.Project.Slug, pkg.SHA256, pkg.StorageKey)

	// 版本 4 段式映射（C20-2）
	ver4 := clickOnceVersion(v.VersionSemverCanonical, v.VersionInteger)

	// 架构映射
	procArch := clickOnceArch(arch)

	// 包标识与配置项读取
	identifiers := listingIdentifiers(req)
	appName := filepath.Base(path)
	if idName := identifiers["application_name"]; idName != "" {
		appName = idName
	}
	publisher := cat.Project.Slug
	if p := identifiers["publisher"]; p != "" {
		publisher = p
	}
	product := cat.Project.Slug
	if pr := identifiers["product"]; pr != "" {
		product = pr
	}
	token := clickOnceDefaultToken
	if tk := identifiers["public_key_token"]; tk != "" {
		token = tk
	}
	deploymentProvider := fmt.Sprintf("/api/v1/projects/%s/store/clickonce/%s/%s", cat.Project.Slug, listingSlug(req), appName)
	if dp := identifiers["deployment_provider"]; dp != "" {
		deploymentProvider = dp
	}

	xmlBody := renderClickOnceXML(clickOnceXMLData{
		ApplicationName:       appName,
		Version:               ver4,
		PublicKeyToken:        token,
		ProcessorArchitecture: procArch,
		Publisher:             publisher,
		Product:               product,
		DeploymentProvider:    deploymentProvider,
		PackageURL:            downloadURL,
		PackageName:           service.DecoratedPackageName(pkg.SHA256, pkg.FileName),
		PackageSize:           pkg.Size,
		SHA256Base64:          sha256B64,
	})

	return &Response{
		Body:        xmlBody,
		ContentType: clickOnceContentType,
		Status:      200,
	}, nil
}

// clickOnceVersion 将项目的版本双号转换为 ClickOnce 所需的 Major.Minor.Build.Revision 格式（C20-2）。
func clickOnceVersion(semverCanonical *string, versionInt *int64) string {
	if semverCanonical != nil && *semverCanonical != "" {
		sv, err := semverpkg.NewVersion(*semverCanonical)
		if err == nil {
			var rev int64
			if versionInt != nil && *versionInt >= 0 && *versionInt <= 65535 {
				rev = *versionInt
			}
			return fmt.Sprintf("%d.%d.%d.%d", sv.Major(), sv.Minor(), sv.Patch(), rev)
		}
	}
	if versionInt != nil && *versionInt >= 0 {
		return fmt.Sprintf("1.0.0.%d", *versionInt%65536)
	}
	return "1.0.0.0"
}

// clickOnceArch 映射规范化架构至 ClickOnce processorArchitecture 属性。
func clickOnceArch(arch string) string {
	switch platform.CanonicalArch(arch) {
	case "x86_64":
		return "amd64"
	case "x86", "i386", "386":
		return "x86"
	case "arm64":
		return "arm64"
	default:
		return "msil"
	}
}

// listingIdentifiers 已覆盖协议包名袋读取。

// clickOnceXMLData 组装 ClickOnce 部署清单所需字段。
type clickOnceXMLData struct {
	ApplicationName       string
	Version               string
	PublicKeyToken        string
	ProcessorArchitecture string
	Publisher             string
	Product               string
	DeploymentProvider    string
	PackageURL            string
	PackageName           string
	PackageSize           int64
	SHA256Base64          string
}

// renderClickOnceXML 渲染 ClickOnce .application XML 文档。
func renderClickOnceXML(d clickOnceXMLData) []byte {
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="utf-8"?>` + "\n")
	b.WriteString(`<assembly xmlns="urn:schemas-microsoft-com:asm.v1" manifestVersion="1.0" xmlns:asmv1="urn:schemas-microsoft-com:asm.v1" xmlns:asmv2="urn:schemas-microsoft-com:asm.v2" xmlns:dsig="http://www.w3.org/2000/09/xmldsig#">` + "\n")
	b.WriteString(fmt.Sprintf(`  <assemblyIdentity name="%s" version="%s" publicKeyToken="%s" processorArchitecture="%s" />`+"\n",
		xmlEscape(d.ApplicationName), xmlEscape(d.Version), xmlEscape(d.PublicKeyToken), xmlEscape(d.ProcessorArchitecture)))
	b.WriteString(fmt.Sprintf(`  <description asmv2:publisher="%s" asmv2:product="%s" xmlns="urn:schemas-microsoft-com:asm.v2" />`+"\n",
		xmlEscape(d.Publisher), xmlEscape(d.Product)))
	b.WriteString(`  <deployment install="true" mapFileExtensions="true" xmlns="urn:schemas-microsoft-com:asm.v2">` + "\n")
	b.WriteString(`    <subscription>` + "\n")
	b.WriteString(`      <update>` + "\n")
	b.WriteString(`        <beforeApplicationStartup />` + "\n")
	b.WriteString(`      </update>` + "\n")
	b.WriteString(`    </subscription>` + "\n")
	if d.DeploymentProvider != "" {
		b.WriteString(fmt.Sprintf(`    <deploymentProvider codebase="%s" />`+"\n", xmlEscape(d.DeploymentProvider)))
	}
	b.WriteString(`  </deployment>` + "\n")
	b.WriteString(`  <dependency>` + "\n")
	b.WriteString(fmt.Sprintf(`    <dependentAssembly dependencyType="install" codebase="%s" size="%d">`+"\n",
		xmlEscape(d.PackageURL), d.PackageSize))
	b.WriteString(fmt.Sprintf(`      <assemblyIdentity name="%s" version="%s" publicKeyToken="%s" processorArchitecture="%s" type="win32" />`+"\n",
		xmlEscape(d.PackageName), xmlEscape(d.Version), xmlEscape(d.PublicKeyToken), xmlEscape(d.ProcessorArchitecture)))
	b.WriteString(`      <hash>` + "\n")
	b.WriteString(`        <dsig:Transforms>` + "\n")
	b.WriteString(`          <dsig:Transform Algorithm="urn:schemas-microsoft-com:HashTransforms.Identity" />` + "\n")
	b.WriteString(`        </dsig:Transforms>` + "\n")
	b.WriteString(`        <dsig:DigestMethod Algorithm="http://www.w3.org/2000/09/xmldsig#sha256" />` + "\n")
	b.WriteString(fmt.Sprintf(`        <dsig:DigestValue>%s</dsig:DigestValue>`+"\n", xmlEscape(d.SHA256Base64)))
	b.WriteString(`      </hash>` + "\n")
	b.WriteString(`    </dependentAssembly>` + "\n")
	b.WriteString(`  </dependency>` + "\n")
	b.WriteString(`</assembly>` + "\n")
	return []byte(b.String())
}
