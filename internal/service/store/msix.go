// Package store 实现商店协议 feed 适配。本文件实现 Windows App Installer
// （.appinstaller）XML feed 适配器（docs/app-init.md §9 / §9.2）。
package store

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/Kirizu-Official/KiriVers/internal/model"
	"github.com/Kirizu-Official/KiriVers/internal/platform"
	semverpkg "github.com/Masterminds/semver/v3"
)

// ProtocolMSIX 是 Windows MSIX App Installer 的协议名
// （对应 Project.StoreProtocols 的 jsonb 键）。
const ProtocolMSIX = "msix"

// msixDefaultArch 是 App Installer 缺省架构。
const msixDefaultArch = "x86_64"

// msixContentType 是 App Installer XML 文档的标准 MIME 类型。
const msixContentType = "application/appinstaller+xml; charset=utf-8"

// MSIXAdapter 实现 Windows App Installer (.appinstaller) feed 适配。
type MSIXAdapter struct{}

// NewMSIXAdapter 构造 MSIXAdapter。
func NewMSIXAdapter() *MSIXAdapter {
	return &MSIXAdapter{}
}

// Protocol 返回协议唯一标识符 "msix"。
func (a *MSIXAdapter) Protocol() string {
	return ProtocolMSIX
}

// Enabled 返回项目是否开启 msix 协议。
func (a *MSIXAdapter) Enabled(p *model.Project) bool {
	return enabledStoreProtocol(p, ProtocolMSIX)
}

// Render 投影当前渠道及平台在 Windows 下的最新匿名可见 MSIX 产物为 .appinstaller XML。
func (a *MSIXAdapter) Render(ctx context.Context, deps *Deps, req Request) (*Response, error) {
	if deps == nil || deps.Updates == nil {
		return nil, fmt.Errorf("feed: update service unavailable")
	}
	if req.Project == nil {
		return nil, fmt.Errorf("%w: project is required", ErrMissingParam)
	}

	// 校验路径后缀：必须以 .appinstaller 结尾（大小写不敏感）
	path := strings.Trim(strings.ToLower(req.Path), "/")
	if !strings.HasSuffix(path, ".appinstaller") {
		return nil, fmt.Errorf("%w: %q (must end with .appinstaller)", ErrUnknownPath, req.Path)
	}

	// App Installer 面向 Windows 平台；若未指定 arch 则默认为 x86_64
	const osName = "windows"
	arch := req.Arch
	if arch == "" {
		arch = msixDefaultArch
	}
	arch = platform.CanonicalArch(arch)

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

	// C23-4 / AC2: 产物不是 MSIX 时该 feed 应对该 Line 关闭或 404，不冒充。
	// 仅选择产物文件名为 .msix 结尾的条目。
	var msixItems []VisibleItem
	for _, it := range items {
		if it.Package != nil && strings.HasSuffix(strings.ToLower(it.Package.FileName), ".msix") {
			msixItems = append(msixItems, it)
		}
	}
	if len(msixItems) == 0 {
		return nil, ErrNoRelease
	}

	// 投影当前可见最新 MSIX 版本
	chosen := &msixItems[0]
	v := &chosen.Version.Version
	pkg, ok := resolveStorePackage(ctx, deps, req, chosen.Line, chosen.Package)
	if !ok || pkg == nil {
		return nil, ErrNoRelease
	}

	signing := urlSigningFromDeps(cat, deps)
	downloadURL := enclosureURL(signing, cat.Project.Slug, pkg.SHA256, pkg.StorageKey)

	// 版本 4 段式映射（C23-2）
	ver4 := msixVersion(v.VersionSemverCanonical, v.VersionInteger)

	// 架构映射
	procArch := msixProcessorArch(arch)

	// 包标识与配置项读取
	identifiers := listingIdentifiers(req)
	name := cat.Project.Slug
	if n := identifiers["name"]; n != "" {
		name = n
	} else if n := identifiers["package_name"]; n != "" {
		name = n
	} else if n := identifiers["main_package_name"]; n != "" {
		name = n
	}

	publisher := "CN=" + cat.Project.Slug
	if p := identifiers["publisher"]; p != "" {
		publisher = p
	}

	uri := fmt.Sprintf("/api/v1/projects/%s/store/msix/%s/%s", cat.Project.Slug, listingSlug(req), filepath.Base(req.Path))
	if u := identifiers["appinstaller_uri"]; u != "" {
		uri = u
	} else if u := identifiers["uri"]; u != "" {
		uri = u
	}

	hoursBetweenUpdateChecks := "0"
	if h := identifiers["hours_between_update_checks"]; h != "" {
		hoursBetweenUpdateChecks = h
	}

	automaticBackgroundTask := true
	if b, ok := identifiers["automatic_background_task"]; ok && (b == "false" || b == "0") {
		automaticBackgroundTask = false
	}

	forceUpdateFromAnyVersion := true
	if f, ok := identifiers["force_update_from_any_version"]; ok && (f == "false" || f == "0") {
		forceUpdateFromAnyVersion = false
	}

	showPrompt := false
	if s, ok := identifiers["show_prompt"]; ok && (s == "true" || s == "1") {
		showPrompt = true
	}

	updateBlocksActivation := false
	if u, ok := identifiers["update_blocks_activation"]; ok && (u == "true" || u == "1") {
		updateBlocksActivation = true
	}

	xmlBody := renderAppInstallerXML(msixXMLData{
		Version:                   ver4,
		Uri:                       uri,
		Name:                      name,
		Publisher:                 publisher,
		ProcessorArchitecture:     procArch,
		MainPackageURI:            downloadURL,
		HoursBetweenUpdateChecks:  hoursBetweenUpdateChecks,
		AutomaticBackgroundTask:   automaticBackgroundTask,
		ForceUpdateFromAnyVersion: forceUpdateFromAnyVersion,
		ShowPrompt:                showPrompt,
		UpdateBlocksActivation:    updateBlocksActivation,
	})

	return &Response{
		Body:        xmlBody,
		ContentType: msixContentType,
		Status:      200,
	}, nil
}

// msixVersion 将项目的版本双号转换为 App Installer 所需的 Major.Minor.Build.Revision 四段式格式（C23-2）。
func msixVersion(semverCanonical *string, versionInt *int64) string {
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

// msixProcessorArch 映射规范化架构至 App Installer ProcessorArchitecture 属性。
func msixProcessorArch(arch string) string {
	switch platform.CanonicalArch(arch) {
	case "x86_64":
		return "x64"
	case "x86", "i386", "386":
		return "x86"
	case "arm64", "aarch64":
		return "arm64"
	case "arm":
		return "arm"
	default:
		return "neutral"
	}
}

// msixXMLData 组装 App Installer .appinstaller 清单所需字段。
type msixXMLData struct {
	Version                   string
	Uri                       string
	Name                      string
	Publisher                 string
	ProcessorArchitecture     string
	MainPackageURI            string
	HoursBetweenUpdateChecks  string
	AutomaticBackgroundTask   bool
	ForceUpdateFromAnyVersion bool
	ShowPrompt                bool
	UpdateBlocksActivation    bool
}

// renderAppInstallerXML 渲染 App Installer .appinstaller XML 文档。
func renderAppInstallerXML(d msixXMLData) []byte {
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="utf-8"?>` + "\n")
	b.WriteString(fmt.Sprintf(`<AppInstaller xmlns="http://schemas.microsoft.com/appx/appinstaller/2018" Version="%s" Uri="%s">`+"\n",
		xmlEscape(d.Version), xmlEscape(d.Uri)))
	b.WriteString(fmt.Sprintf(`  <MainPackage Name="%s" Publisher="%s" Version="%s" ProcessorArchitecture="%s" Uri="%s" />`+"\n",
		xmlEscape(d.Name), xmlEscape(d.Publisher), xmlEscape(d.Version), xmlEscape(d.ProcessorArchitecture), xmlEscape(d.MainPackageURI)))
	b.WriteString(`  <UpdateSettings>` + "\n")
	b.WriteString(fmt.Sprintf(`    <OnLaunch HoursBetweenUpdateChecks="%s" />`+"\n", xmlEscape(d.HoursBetweenUpdateChecks)))
	if d.AutomaticBackgroundTask {
		b.WriteString(`    <AutomaticBackgroundTask />` + "\n")
	}
	if d.ForceUpdateFromAnyVersion {
		b.WriteString(`    <ForceUpdateFromAnyVersion>true</ForceUpdateFromAnyVersion>` + "\n")
	}
	if d.ShowPrompt {
		b.WriteString(`    <ShowPrompt>true</ShowPrompt>` + "\n")
	}
	if d.UpdateBlocksActivation {
		b.WriteString(`    <UpdateBlocksActivation>true</UpdateBlocksActivation>` + "\n")
	}
	b.WriteString(`  </UpdateSettings>` + "\n")
	b.WriteString(`</AppInstaller>` + "\n")
	return []byte(b.String())
}
