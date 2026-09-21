// Package store 实现商店协议 feed 适配。本文件实现 Android F-Droid
// 仓库索引（index-v1 / index-v2 / entry.json）feed 适配器（docs/app-init.md §9 / §9.2）。
package store

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/Kirizu-Official/KiriVers/internal/model"
	"github.com/Kirizu-Official/KiriVers/internal/platform"
	"github.com/Kirizu-Official/KiriVers/internal/service/update"
	semverpkg "github.com/Masterminds/semver/v3"
)

// ProtocolFDroid 是 Android F-Droid 仓库协议名
// （对应 Project.StoreProtocols 的 jsonb 键）。
const ProtocolFDroid = "fdroid"

// fdroidContentType 是 F-Droid 仓库索引的标准 MIME 类型。
const fdroidContentType = "application/json; charset=utf-8"

// FDroidAdapter 实现 Android F-Droid 仓库 feed 适配。
type FDroidAdapter struct{}

// NewFDroidAdapter 构造 FDroidAdapter。
func NewFDroidAdapter() *FDroidAdapter {
	return &FDroidAdapter{}
}

// Protocol 返回协议唯一标识符 "fdroid"。
func (a *FDroidAdapter) Protocol() string {
	return ProtocolFDroid
}

// Enabled 返回项目是否开启 fdroid 协议。
func (a *FDroidAdapter) Enabled(p *model.Project) bool {
	return enabledStoreProtocol(p, ProtocolFDroid)
}

// Render 投影 Android 单文件 APK 产物为 F-Droid 格式 JSON 索引（index-v1 / index-v2 / entry.json）。
func (a *FDroidAdapter) Render(ctx context.Context, deps *Deps, req Request) (*Response, error) {
	if deps == nil || deps.Updates == nil {
		return nil, fmt.Errorf("feed: update service unavailable")
	}
	if req.Project == nil {
		return nil, fmt.Errorf("%w: project is required", ErrMissingParam)
	}

	cleanPath := strings.Trim(strings.ToLower(req.Path), "/")
	switch cleanPath {
	case "index-v1.json", "index-v1":
		return a.renderIndexV1(ctx, deps, req)
	case "index-v2.json", "index-v2":
		return a.renderIndexV2(ctx, deps, req)
	case "entry.json":
		return a.renderEntry(ctx, deps, req)
	default:
		return nil, fmt.Errorf("%w: %q (expected index-v1.json, index-v2.json or entry.json)", ErrUnknownPath, req.Path)
	}
}

// fdroidIdentifiers 存储从 StoreProtocols["fdroid"].Identifiers 读取的配置。
type fdroidIdentifiers struct {
	PackageName     string
	Name            string
	Summary         string
	Description     string
	Author          string
	License         string
	RepoName        string
	RepoDescription string
	RepoAddress     string
	Icon            string
}

// resolveIdentifiers 从项目配置提取 F-Droid 元数据，或缺省回退。
func (a *FDroidAdapter) resolveIdentifiers(p *model.Project, ids map[string]string) fdroidIdentifiers {
	m := ids

	get := func(def string, keys ...string) string {
		if m == nil {
			return def
		}
		for _, k := range keys {
			if v, ok := m[k]; ok && strings.TrimSpace(v) != "" {
				return strings.TrimSpace(v)
			}
		}
		return def
	}

	slug := ""
	if p != nil {
		slug = p.Slug
	}

	return fdroidIdentifiers{
		PackageName:     get(slug, "package_name", "application_id"),
		Name:            get(slug, "name", "app_name"),
		Summary:         get(slug+" Android application", "summary", "short_description"),
		Description:     get(slug+" Android package repository", "description"),
		Author:          get(slug, "author", "publisher"),
		License:         get("Unknown", "license"),
		RepoName:        get(slug+" Repository", "repo_name"),
		RepoDescription: get(slug+" Android packages", "repo_description"),
		RepoAddress:     get("/api/v1/projects/"+slug+"/store/fdroid", "repo_address", "address"),
		Icon:            get("", "icon"),
	}
}

// fdroidApkItem 表示一个解析就绪的 Android APK 产物项。
type fdroidApkItem struct {
	version   model.Version
	arch      string
	pkg       *update.ArtifactInfo
	cat       *update.Catalog
	signedURL string
}

// collectAndroidVersions 收集项目在 Android 平台上可见的单文件 APK 产物。
// 遵循 C24-2 / AC2：仅投影 android 单文件 APK Line；其它 os（如 windows）绝对不进入索引。
func (a *FDroidAdapter) collectAndroidVersions(ctx context.Context, deps *Deps, req Request) ([]fdroidApkItem, *update.Catalog, error) {
	channel := req.Channel
	if channel == "" {
		channel = "stable"
	}

	targetArches := []string{"arm64", "arm", "armv7", "x86_64", "x86", "universal"}
	if req.Arch != "" {
		targetArches = []string{platform.CanonicalArch(req.Arch)}
	}

	var items []fdroidApkItem
	var mainCat *update.Catalog
	seen := make(map[string]bool)

	for _, arch := range targetArches {
		cat, err := deps.Updates.LoadCatalog(ctx, req.Project.ID, "android", arch)
		if err != nil {
			continue
		}
		if mainCat == nil {
			mainCat = cat
		}
		ch, ok := cat.Channel(channel)
		if !ok || !ch.Enabled {
			continue
		}

		vis := AnonymousVisible(cat, cat.Project.CompareEngine, channel, "android", arch)
		for _, it := range vis {
			pkg, ok := resolveStorePackage(ctx, deps, req, it.Line, it.Package)
			if !ok || pkg == nil {
				continue
			}
			if !strings.HasSuffix(strings.ToLower(pkg.FileName), ".apk") {
				continue
			}

			key := fmt.Sprintf("%s:%s", it.Version.Version.ID, pkg.SHA256)
			if seen[key] {
				continue
			}
			seen[key] = true

			signing := urlSigningFromDeps(cat, deps)
			downloadURL := enclosureURL(signing, cat.Project.Slug, pkg.SHA256, pkg.StorageKey)

			items = append(items, fdroidApkItem{
				version:   it.Version.Version,
				arch:      arch,
				pkg:       pkg,
				cat:       cat,
				signedURL: downloadURL,
			})
		}
	}

	if len(items) == 0 {
		return nil, nil, ErrNoRelease
	}
	return items, mainCat, nil
}

// fdroidVersionCode 映射双号至 versionCode（C24-3）。
func fdroidVersionCode(v *model.Version) int64 {
	if v.VersionInteger != nil && *v.VersionInteger > 0 {
		return *v.VersionInteger
	}
	if v.VersionSemverCanonical != nil {
		sv, err := semverpkg.NewVersion(*v.VersionSemverCanonical)
		if err == nil {
			return int64(sv.Major()*10000 + sv.Minor()*100 + sv.Patch())
		}
	}
	return 1
}

// fdroidVersionName 映射双号至 versionName（C24-3）。
func fdroidVersionName(v *model.Version) string {
	if v.VersionSemverCanonical != nil && *v.VersionSemverCanonical != "" {
		return *v.VersionSemverCanonical
	}
	if v.VersionSemver != nil && *v.VersionSemver != "" {
		return *v.VersionSemver
	}
	if v.VersionInteger != nil {
		return fmt.Sprint(*v.VersionInteger)
	}
	return "1.0.0"
}

// fdroidNativeCode 映射架构为 Android ABI 字符串数组。
func fdroidNativeCode(arch string) []string {
	switch platform.CanonicalArch(arch) {
	case "arm64", "aarch64":
		return []string{"arm64-v8a"}
	case "arm", "armv7":
		return []string{"armeabi-v7a"}
	case "x86_64":
		return []string{"x86_64"}
	case "x86", "386":
		return []string{"x86"}
	default:
		return []string{"arm64-v8a", "armeabi-v7a", "x86_64", "x86"}
	}
}

// fdroidTimestamp 转换为毫秒时间戳。
func fdroidTimestamp(v *model.Version) int64 {
	if v != nil && v.PublishTime != nil {
		return v.PublishTime.UnixNano() / 1e6
	}
	return 0
}

// ---------------------------------------------------------------------------
// 1. F-Droid index-v1.json
// ---------------------------------------------------------------------------

type fdroidV1Response struct {
	Repo     fdroidV1Repo                 `json:"repo"`
	Requests fdroidV1Requests             `json:"requests"`
	Apps     []fdroidV1App                `json:"apps"`
	Packages map[string][]fdroidV1Package `json:"packages"`
}

type fdroidV1Repo struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Icon        string `json:"icon,omitempty"`
	Address     string `json:"address"`
	Timestamp   int64  `json:"timestamp"`
	Version     int    `json:"version"`
	MaxAge      int    `json:"maxage"`
}

type fdroidV1Requests struct {
	Install   []string `json:"install"`
	Uninstall []string `json:"uninstall"`
}

type fdroidV1App struct {
	PackageName string `json:"packageName"`
	Name        string `json:"name"`
	Summary     string `json:"summary"`
	Description string `json:"description"`
	License     string `json:"license"`
	AuthorName  string `json:"authorName"`
	Added       int64  `json:"added"`
	LastUpdated int64  `json:"lastUpdated"`
	Icon        string `json:"icon,omitempty"`
}

type fdroidV1Package struct {
	VersionName string   `json:"versionName"`
	VersionCode int64    `json:"versionCode"`
	Size        int64    `json:"size"`
	Hash        string   `json:"hash"`
	HashType    string   `json:"hashType"`
	ApkName     string   `json:"apkName"`
	Added       int64    `json:"added"`
	NativeCode  []string `json:"nativecode,omitempty"`
}

func (a *FDroidAdapter) renderIndexV1(ctx context.Context, deps *Deps, req Request) (*Response, error) {
	items, cat, err := a.collectAndroidVersions(ctx, deps, req)
	if err != nil {
		return nil, err
	}

	ident := a.resolveIdentifiers(req.Project, listingIdentifiers(req))

	var maxTimestamp int64
	var minTimestamp int64
	pkgs := make([]fdroidV1Package, 0, len(items))

	for _, it := range items {
		ts := fdroidTimestamp(&it.version)
		if ts > maxTimestamp {
			maxTimestamp = ts
		}
		if minTimestamp == 0 || ts < minTimestamp {
			minTimestamp = ts
		}

		pkgs = append(pkgs, fdroidV1Package{
			VersionName: fdroidVersionName(&it.version),
			VersionCode: fdroidVersionCode(&it.version),
			Size:        it.pkg.Size,
			Hash:        it.pkg.SHA256,
			HashType:    "sha256",
			ApkName:     it.signedURL,
			Added:       ts,
			NativeCode:  fdroidNativeCode(it.arch),
		})
	}

	app := fdroidV1App{
		PackageName: ident.PackageName,
		Name:        ident.Name,
		Summary:     ident.Summary,
		Description: ident.Description,
		License:     ident.License,
		AuthorName:  ident.Author,
		Added:       minTimestamp,
		LastUpdated: maxTimestamp,
		Icon:        ident.Icon,
	}

	resp := fdroidV1Response{
		Repo: fdroidV1Repo{
			Name:        ident.RepoName,
			Description: ident.RepoDescription,
			Icon:        ident.Icon,
			Address:     ident.RepoAddress,
			Timestamp:   maxTimestamp,
			Version:     20002,
			MaxAge:      14,
		},
		Requests: fdroidV1Requests{
			Install:   []string{},
			Uninstall: []string{},
		},
		Apps: []fdroidV1App{app},
		Packages: map[string][]fdroidV1Package{
			ident.PackageName: pkgs,
		},
	}

	_ = cat
	body, err := json.Marshal(resp)
	if err != nil {
		return nil, fmt.Errorf("feed: marshal index-v1: %w", err)
	}

	return &Response{
		Body:        body,
		ContentType: fdroidContentType,
		Status:      http.StatusOK,
	}, nil
}

// ---------------------------------------------------------------------------
// 2. F-Droid index-v2.json
// ---------------------------------------------------------------------------

type fdroidV2Response struct {
	Repo     fdroidV2Repo                   `json:"repo"`
	Packages map[string]fdroidV2PackageData `json:"packages"`
}

type fdroidV2Repo struct {
	Name        map[string]string `json:"name"`
	Description map[string]string `json:"description"`
	Address     string            `json:"address"`
	Timestamp   int64             `json:"timestamp"`
}

type fdroidV2PackageData struct {
	Metadata fdroidV2Metadata                  `json:"metadata"`
	Versions map[string]fdroidV2VersionDetails `json:"versions"`
}

type fdroidV2Metadata struct {
	Name        map[string]string `json:"name"`
	Summary     map[string]string `json:"summary"`
	Description map[string]string `json:"description"`
	AuthorName  string            `json:"authorName"`
	License     string            `json:"license"`
}

type fdroidV2VersionDetails struct {
	Added    int64               `json:"added"`
	File     fdroidV2File        `json:"file"`
	Manifest fdroidV2ManifestData `json:"manifest"`
}

type fdroidV2File struct {
	Name   string `json:"name"`
	SHA256 string `json:"sha256"`
	Size   int64  `json:"size"`
}

type fdroidV2ManifestData struct {
	VersionName string   `json:"versionName"`
	VersionCode int64    `json:"versionCode"`
	NativeCode  []string `json:"nativecode,omitempty"`
}

func (a *FDroidAdapter) renderIndexV2(ctx context.Context, deps *Deps, req Request) (*Response, error) {
	body, _, err := a.buildIndexV2Bytes(ctx, deps, req)
	if err != nil {
		return nil, err
	}

	return &Response{
		Body:        body,
		ContentType: fdroidContentType,
		Status:      http.StatusOK,
	}, nil
}

func (a *FDroidAdapter) buildIndexV2Bytes(ctx context.Context, deps *Deps, req Request) ([]byte, int64, error) {
	items, _, err := a.collectAndroidVersions(ctx, deps, req)
	if err != nil {
		return nil, 0, err
	}

	ident := a.resolveIdentifiers(req.Project, listingIdentifiers(req))

	var maxTimestamp int64
	versionMap := make(map[string]fdroidV2VersionDetails)

	for _, it := range items {
		ts := fdroidTimestamp(&it.version)
		if ts > maxTimestamp {
			maxTimestamp = ts
		}

		vCode := fdroidVersionCode(&it.version)
		vName := fdroidVersionName(&it.version)

		versionMap[it.pkg.SHA256] = fdroidV2VersionDetails{
			Added: ts,
			File: fdroidV2File{
				Name:   it.signedURL,
				SHA256: it.pkg.SHA256,
				Size:   it.pkg.Size,
			},
			Manifest: fdroidV2ManifestData{
				VersionName: vName,
				VersionCode: vCode,
				NativeCode:  fdroidNativeCode(it.arch),
			},
		}
	}

	resp := fdroidV2Response{
		Repo: fdroidV2Repo{
			Name:        map[string]string{"en-US": ident.RepoName},
			Description: map[string]string{"en-US": ident.RepoDescription},
			Address:     ident.RepoAddress,
			Timestamp:   maxTimestamp,
		},
		Packages: map[string]fdroidV2PackageData{
			ident.PackageName: {
				Metadata: fdroidV2Metadata{
					Name:        map[string]string{"en-US": ident.Name},
					Summary:     map[string]string{"en-US": ident.Summary},
					Description: map[string]string{"en-US": ident.Description},
					AuthorName:  ident.Author,
					License:     ident.License,
				},
				Versions: versionMap,
			},
		},
	}

	body, err := json.Marshal(resp)
	if err != nil {
		return nil, 0, fmt.Errorf("feed: marshal index-v2: %w", err)
	}
	return body, maxTimestamp, nil
}

// ---------------------------------------------------------------------------
// 3. F-Droid entry.json
// ---------------------------------------------------------------------------

type fdroidEntryResponse struct {
	Timestamp int64            `json:"timestamp"`
	Version   int              `json:"version"`
	MaxAge    int              `json:"maxAge"`
	Index     fdroidEntryIndex `json:"index"`
}

type fdroidEntryIndex struct {
	Name   string `json:"name"`
	SHA256 string `json:"sha256"`
	Size   int64  `json:"size"`
}

func (a *FDroidAdapter) renderEntry(ctx context.Context, deps *Deps, req Request) (*Response, error) {
	v2Bytes, maxTs, err := a.buildIndexV2Bytes(ctx, deps, req)
	if err != nil {
		return nil, err
	}

	sum := sha256.Sum256(v2Bytes)
	sha256Hex := hex.EncodeToString(sum[:])

	resp := fdroidEntryResponse{
		Timestamp: maxTs,
		Version:   20002,
		MaxAge:    14,
		Index: fdroidEntryIndex{
			Name:   "/index-v2.json",
			SHA256: sha256Hex,
			Size:   int64(len(v2Bytes)),
		},
	}

	body, err := json.Marshal(resp)
	if err != nil {
		return nil, fmt.Errorf("feed: marshal entry.json: %w", err)
	}

	return &Response{
		Body:        body,
		ContentType: fdroidContentType,
		Status:      http.StatusOK,
	}, nil
}
