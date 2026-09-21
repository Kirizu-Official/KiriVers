// Package store 提供商店协议 feed 的投影适配实现。
//
// 本文件实现 Microsoft WinGet REST 源协议（docs/app-init.md §9 / §9.2，C22-1..C22-7）。
package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/google/uuid"

	"github.com/Kirizu-Official/KiriVers/internal/model"
	"github.com/Kirizu-Official/KiriVers/internal/platform"
	"github.com/Kirizu-Official/KiriVers/internal/service/update"
)

// wingetContentType 是 WinGet REST 源的标准 JSON 响应 MIME。
const wingetContentType = "application/json; charset=utf-8"

// wingetServerSupportedVersions 是本实例支持的官方 REST 契约版本列表。
var wingetServerSupportedVersions = []string{"1.1.0", "1.4.0"}

// WinGetAdapter 是 Microsoft WinGet REST 源协议的 Adapter 实现。
type WinGetAdapter struct{}

// compile-time 校验：满足 Adapter 合同。
var _ Adapter = (*WinGetAdapter)(nil)

// NewWinGetAdapter 构造 WinGetAdapter。
func NewWinGetAdapter() *WinGetAdapter {
	return &WinGetAdapter{}
}

// Protocol 返回协议唯一标识符（对应 Project.StoreProtocols 的 jsonb 键）。
func (a *WinGetAdapter) Protocol() string {
	return ProtocolWinGet
}

// Enabled 读取项目开关：StoreProtocols["winget"].Enabled。
func (a *WinGetAdapter) Enabled(p *model.Project) bool {
	return enabledStoreProtocol(p, ProtocolWinGet)
}

// Render 将项目的 Windows 匿名可见版本集投影为 WinGet REST 源格式响应。
//
// 依据路径支持三个标准端点：
//   - GET information: 返回源元数据与支持的契约版本清单；
//   - POST / GET manifestSearch: 搜索或列出包概要；
//   - GET packageManifests / packageManifests/{PackageIdentifier}:
//     返回包含安装器（Architecture、InstallerType、InstallerUrl、InstallerSha256）的完整清单。
//
// 任意其它路径报 ErrUnknownPath（→ 纯文本 404，绝不回退 zip）。
func (a *WinGetAdapter) Render(ctx context.Context, deps *Deps, req Request) (*Response, error) {
	if deps == nil || deps.Updates == nil {
		return nil, errors.New("feed: updates service unavailable")
	}

	cleanPath := strings.Trim(req.Path, "/")
	ident := a.resolveIdentifiers(req.Project, listingIdentifiers(req))

	switch {
	case cleanPath == "information":
		return a.renderInformation(ident.PackageIdentifier)

	case cleanPath == "manifestSearch":
		return a.renderManifestSearch(ctx, deps, req, ident)

	case cleanPath == "packageManifests" || strings.HasPrefix(cleanPath, "packageManifests/"):
		targetID := ""
		if strings.HasPrefix(cleanPath, "packageManifests/") {
			targetID = strings.TrimPrefix(cleanPath, "packageManifests/")
		}
		return a.renderPackageManifests(ctx, deps, req, ident, targetID)

	default:
		return nil, ErrUnknownPath
	}
}

// wingetIdentifiers 聚合 WinGet 协议所需的元数据配置。
type wingetIdentifiers struct {
	PackageIdentifier string
	Publisher         string
	PackageName       string
	License           string
	ShortDescription  string
	InstallerType     string
}

// resolveIdentifiers 从项目的 store_protocols["winget"].identifiers 提取或兜底默认值。
func (a *WinGetAdapter) resolveIdentifiers(p *model.Project, ids map[string]string) wingetIdentifiers {
	m := ids

	get := func(keys ...string) string {
		if m == nil {
			return ""
		}
		for _, k := range keys {
			if v, ok := m[k]; ok && strings.TrimSpace(v) != "" {
				return strings.TrimSpace(v)
			}
		}
		return ""
	}

	pub := get("publisher", "Publisher")
	if pub == "" {
		pub = "KiriVers"
	}

	pkgName := get("package_name", "PackageName", "name", "Name")
	if pkgName == "" {
		if p != nil {
			pkgName = p.Slug
		} else {
			pkgName = "App"
		}
	}

	pkgID := get("package_identifier", "PackageIdentifier", "id", "Id")
	if pkgID == "" {
		slugPart := ""
		if p != nil {
			slugPart = strings.ReplaceAll(strings.Title(strings.ReplaceAll(p.Slug, "-", " ")), " ", "")
		} else {
			slugPart = "App"
		}
		if pub != "" && pub != "KiriVers" {
			pkgID = pub + "." + slugPart
		} else {
			pkgID = "KiriVers." + slugPart
		}
	}

	lic := get("license", "License")
	if lic == "" {
		lic = "Proprietary"
	}

	desc := get("short_description", "ShortDescription", "description", "Description")
	if desc == "" {
		desc = pkgName + " application"
	}

	instType := get("installer_type", "InstallerType")

	return wingetIdentifiers{
		PackageIdentifier: pkgID,
		Publisher:         pub,
		PackageName:       pkgName,
		License:           lic,
		ShortDescription:  desc,
		InstallerType:     instType,
	}
}

// ---------------------------------------------------------------------------
// 1. GET information
// ---------------------------------------------------------------------------

type wingetInformationResponse struct {
	Data wingetInformationData `json:"Data"`
}

type wingetInformationData struct {
	SourceIdentifier              string   `json:"SourceIdentifier"`
	ServerSupportedVersions       []string `json:"ServerSupportedVersions"`
	UnsupportedPackageMatchFields []string `json:"UnsupportedPackageMatchFields"`
	RequiredPackageMatchFields    []string `json:"RequiredPackageMatchFields"`
	UnsupportedQueryParameters    []string `json:"UnsupportedQueryParameters"`
	RequiredQueryParameters       []string `json:"RequiredQueryParameters"`
}

func (a *WinGetAdapter) renderInformation(sourceID string) (*Response, error) {
	resp := wingetInformationResponse{
		Data: wingetInformationData{
			SourceIdentifier:              sourceID,
			ServerSupportedVersions:       wingetServerSupportedVersions,
			UnsupportedPackageMatchFields: []string{},
			RequiredPackageMatchFields:    []string{},
			UnsupportedQueryParameters:    []string{},
			RequiredQueryParameters:       []string{},
		},
	}
	body, err := json.Marshal(resp)
	if err != nil {
		return nil, fmt.Errorf("feed: marshal information: %w", err)
	}
	return &Response{
		Body:        body,
		ContentType: wingetContentType,
		Status:      http.StatusOK,
	}, nil
}

// ---------------------------------------------------------------------------
// 2. POST / GET manifestSearch
// ---------------------------------------------------------------------------

type wingetSearchResponse struct {
	Data []wingetSearchPackage `json:"Data"`
}

type wingetSearchPackage struct {
	PackageIdentifier string                `json:"PackageIdentifier"`
	PackageName       string                `json:"PackageName"`
	Publisher         string                `json:"Publisher"`
	Versions          []wingetSearchVersion `json:"Versions"`
}

type wingetSearchVersion struct {
	PackageVersion     string   `json:"PackageVersion"`
	Channel            string   `json:"Channel"`
	PackageFamilyNames []string `json:"PackageFamilyNames"`
	ProductCodes       []string `json:"ProductCodes"`
}

func (a *WinGetAdapter) renderManifestSearch(ctx context.Context, deps *Deps, req Request, ident wingetIdentifiers) (*Response, error) {
	winVersions := a.collectWindowsVersions(ctx, deps, req, "")
	if len(winVersions) == 0 {
		emptyResp := wingetSearchResponse{Data: []wingetSearchPackage{}}
		body, _ := json.Marshal(emptyResp)
		return &Response{
			Body:        body,
			ContentType: wingetContentType,
			Status:      http.StatusOK,
		}, nil
	}

	searchVersions := make([]wingetSearchVersion, 0, len(winVersions))
	for _, wv := range winVersions {
		searchVersions = append(searchVersions, wingetSearchVersion{
			PackageVersion:     wv.verStr,
			Channel:            wv.channel,
			PackageFamilyNames: []string{},
			ProductCodes:       []string{},
		})
	}

	pkg := wingetSearchPackage{
		PackageIdentifier: ident.PackageIdentifier,
		PackageName:       ident.PackageName,
		Publisher:         ident.Publisher,
		Versions:          searchVersions,
	}

	body, err := json.Marshal(wingetSearchResponse{Data: []wingetSearchPackage{pkg}})
	if err != nil {
		return nil, fmt.Errorf("feed: marshal manifestSearch: %w", err)
	}
	return &Response{
		Body:        body,
		ContentType: wingetContentType,
		Status:      http.StatusOK,
	}, nil
}

// ---------------------------------------------------------------------------
// 3. GET packageManifests / packageManifests/{PackageIdentifier}
// ---------------------------------------------------------------------------

type wingetSingleManifestResponse struct {
	Data wingetPackageManifest `json:"Data"`
}

type wingetMultipleManifestResponse struct {
	Data []wingetPackageManifest `json:"Data"`
}

type wingetPackageManifest struct {
	PackageIdentifier string                  `json:"PackageIdentifier"`
	Versions          []wingetManifestVersion `json:"Versions"`
}

type wingetManifestVersion struct {
	PackageVersion string              `json:"PackageVersion"`
	Channel        string              `json:"Channel"`
	DefaultLocale  wingetDefaultLocale `json:"DefaultLocale"`
	Installers     []wingetInstaller   `json:"Installers"`
}

type wingetDefaultLocale struct {
	PackageLocale    string `json:"PackageLocale"`
	Publisher        string `json:"Publisher"`
	PackageName      string `json:"PackageName"`
	License          string `json:"License"`
	ShortDescription string `json:"ShortDescription"`
}

type wingetInstaller struct {
	Architecture    string `json:"Architecture"`
	InstallerType   string `json:"InstallerType"`
	InstallerUrl    string `json:"InstallerUrl"`
	InstallerSha256 string `json:"InstallerSha256"`
	InstallerLocale string `json:"InstallerLocale"`
}

func (a *WinGetAdapter) renderPackageManifests(ctx context.Context, deps *Deps, req Request, ident wingetIdentifiers, targetID string) (*Response, error) {
	// 如果路径中指明了 PackageIdentifier，必须与本项目一致，否则 404
	if targetID != "" && !strings.EqualFold(targetID, ident.PackageIdentifier) {
		return nil, ErrNoRelease
	}

	// 检查是否有版本过滤 query (?Version=...)
	verFilter := req.CurrentVersion
	if verFilter == "" && req.QueryParams != nil {
		if v, ok := req.QueryParams["Version"]; ok {
			verFilter = strings.TrimSpace(v)
		} else if v, ok := req.QueryParams["version"]; ok {
			verFilter = strings.TrimSpace(v)
		}
	}

	winVersions := a.collectWindowsVersions(ctx, deps, req, verFilter)
	if len(winVersions) == 0 {
		return nil, ErrNoRelease
	}

	manifestVersions := make([]wingetManifestVersion, 0, len(winVersions))
	for _, wv := range winVersions {
		installers := make([]wingetInstaller, 0, len(wv.installers))
		for _, inst := range wv.installers {
			pkg, ok := resolveStorePackage(ctx, deps, req, inst.line, inst.pkg)
			if !ok || pkg == nil {
				continue
			}
			signing := urlSigningFromDeps(inst.cat, deps)
			downloadURL := enclosureURL(signing, inst.cat.Project.Slug, pkg.SHA256, pkg.StorageKey)
			iType := detectWinGetInstallerType(pkg.FileName, ident.InstallerType)
			installers = append(installers, wingetInstaller{
				Architecture:    toWinGetArch(inst.arch),
				InstallerType:   iType,
				InstallerUrl:    downloadURL,
				InstallerSha256: strings.ToUpper(pkg.SHA256),
				InstallerLocale: "en-US",
			})
		}
		if len(installers) == 0 {
			continue
		}

		desc := ident.ShortDescription
		if wv.changelog != "" {
			desc = wv.changelog
		}

		manifestVersions = append(manifestVersions, wingetManifestVersion{
			PackageVersion: wv.verStr,
			Channel:        wv.channel,
			DefaultLocale: wingetDefaultLocale{
				PackageLocale:    "en-US",
				Publisher:        ident.Publisher,
				PackageName:      ident.PackageName,
				License:          ident.License,
				ShortDescription: desc,
			},
			Installers: installers,
		})
	}

	if len(manifestVersions) == 0 {
		return nil, ErrNoRelease
	}

	pm := wingetPackageManifest{
		PackageIdentifier: ident.PackageIdentifier,
		Versions:          manifestVersions,
	}

	var body []byte
	var err error
	if targetID != "" {
		// 单包查询：Data 为单个对象
		body, err = json.Marshal(wingetSingleManifestResponse{Data: pm})
	} else {
		// 列表查询：Data 为对象数组
		body, err = json.Marshal(wingetMultipleManifestResponse{Data: []wingetPackageManifest{pm}})
	}
	if err != nil {
		return nil, fmt.Errorf("feed: marshal packageManifests: %w", err)
	}

	return &Response{
		Body:        body,
		ContentType: wingetContentType,
		Status:      http.StatusOK,
	}, nil
}

// ---------------------------------------------------------------------------
// 内部聚合辅助：Windows 版本与架构聚合
// ---------------------------------------------------------------------------

type winInstallerItem struct {
	arch string
	pkg  *update.ArtifactInfo
	line *update.LineState
	cat  *update.Catalog
}

type winVersionItem struct {
	id         uuid.UUID
	verStr     string
	channel    string
	changelog  string
	installers []winInstallerItem
}

// collectWindowsVersions 聚合符合条件的 Windows 版本及其各架构的安装器列表。
func (a *WinGetAdapter) collectWindowsVersions(ctx context.Context, deps *Deps, req Request, verFilter string) []winVersionItem {
	if deps == nil || deps.Updates == nil || req.Project == nil {
		return nil
	}

	chName := req.Channel
	if chName == "" {
		chName = "stable"
	}

	targetArches := []string{"x86_64", "arm64", "x86"}
	if req.Arch != "" {
		targetArches = []string{platform.CanonicalArch(req.Arch)}
	}

	var results []winVersionItem

	for _, arch := range targetArches {
		cat, err := deps.Updates.LoadCatalog(ctx, req.Project.ID, "windows", arch)
		if err != nil {
			continue
		}
		ch, ok := cat.Channel(chName)
		if !ok || !ch.Enabled {
			continue
		}

		items := AnonymousVisible(cat, cat.Project.CompareEngine, chName, "windows", arch)
		for _, it := range items {
			v := &it.Version.Version
			verStr := wingetVersionString(v)
			if verFilter != "" && verStr != verFilter {
				continue
			}

			idx := -1
			for i, r := range results {
				if r.id == v.ID {
					idx = i
					break
				}
			}

			chLog := ""
			if v.Changelog != nil {
				for _, entry := range v.Changelog {
					if entry.Markdown != "" {
						chLog = entry.Markdown
						break
					} else if entry.Title != "" {
						chLog = entry.Title
						break
					}
				}
			}

			installer := winInstallerItem{
				arch: arch,
				pkg:  it.Package,
				line: it.Line,
				cat:  cat,
			}

			if idx == -1 {
				results = append(results, winVersionItem{
					id:         v.ID,
					verStr:     verStr,
					channel:    v.ChannelSlug,
					changelog:  chLog,
					installers: []winInstallerItem{installer},
				})
			} else {
				already := false
				for _, inst := range results[idx].installers {
					if inst.arch == arch {
						already = true
						break
					}
				}
				if !already {
					results[idx].installers = append(results[idx].installers, installer)
				}
			}
		}
	}

	return results
}

// toWinGetArch 将系统架构转译为 WinGet 规范架构字符串。
func toWinGetArch(arch string) string {
	switch strings.ToLower(strings.TrimSpace(arch)) {
	case "x86_64", "amd64", "x64":
		return "x64"
	case "x86", "386", "i386":
		return "x86"
	case "arm64", "aarch64":
		return "arm64"
	case "arm":
		return "arm"
	default:
		return arch
	}
}

// detectWinGetInstallerType 根据产物文件名扩展名或用户配置推断 WinGet 安装器类型。
func detectWinGetInstallerType(filename string, configured string) string {
	if configured != "" {
		return strings.ToLower(strings.TrimSpace(configured))
	}
	ext := strings.ToLower(filepath.Ext(filename))
	switch ext {
	case ".msi":
		return "msi"
	case ".msix", ".msixbundle", ".appx", ".appxbundle":
		return "msix"
	case ".zip":
		return "zip"
	case ".exe":
		return "exe"
	default:
		return "exe"
	}
}

// wingetVersionString 计算 WinGet 协议要求的 PackageVersion 字段。
// 严格遵守 C22-3：写死 semver 规范字串，兜底 integer 字串。
func wingetVersionString(v *model.Version) string {
	if v == nil {
		return "0.0.0"
	}
	if v.VersionSemverCanonical != nil && *v.VersionSemverCanonical != "" {
		return *v.VersionSemverCanonical
	}
	if v.VersionSemver != nil && *v.VersionSemver != "" {
		return *v.VersionSemver
	}
	if v.VersionInteger != nil {
		return strconv.FormatInt(*v.VersionInteger, 10)
	}
	return "0.0.0"
}
