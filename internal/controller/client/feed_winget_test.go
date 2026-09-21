package client

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Kirizu-Official/KiriVers/internal/model"
	"github.com/Kirizu-Official/KiriVers/internal/service"
)

// ---------- WinGet REST source feed HTTP 集成测试（§9 / §9.2，C22-1..C22-7） ----------
//
// 复用 check_test.go 的 setupCheckTest / publishSingleFile / perform / ptr。

// testWinGetInformationData 测试用 information 返回模型。
type testWinGetInformationData struct {
	SourceIdentifier              string   `json:"SourceIdentifier"`
	ServerSupportedVersions       []string `json:"ServerSupportedVersions"`
	UnsupportedPackageMatchFields []string `json:"UnsupportedPackageMatchFields"`
	RequiredPackageMatchFields    []string `json:"RequiredPackageMatchFields"`
	UnsupportedQueryParameters    []string `json:"UnsupportedQueryParameters"`
	RequiredQueryParameters       []string `json:"RequiredQueryParameters"`
}

type testWinGetInformationResp struct {
	Data testWinGetInformationData `json:"Data"`
}

// testWinGetManifestData 测试用 packageManifests 返回模型。
type testWinGetManifestResp struct {
	Data struct {
		PackageIdentifier string `json:"PackageIdentifier"`
		Versions          []struct {
			PackageVersion string `json:"PackageVersion"`
			Channel        string `json:"Channel"`
			DefaultLocale  struct {
				PackageLocale    string `json:"PackageLocale"`
				Publisher        string `json:"Publisher"`
				PackageName      string `json:"PackageName"`
				License          string `json:"License"`
				ShortDescription string `json:"ShortDescription"`
			} `json:"DefaultLocale"`
			Installers []struct {
				Architecture    string `json:"Architecture"`
				InstallerType   string `json:"InstallerType"`
				InstallerUrl    string `json:"InstallerUrl"`
				InstallerSha256 string `json:"InstallerSha256"`
				InstallerLocale string `json:"InstallerLocale"`
			} `json:"Installers"`
		} `json:"Versions"`
	} `json:"Data"`
}

// testWinGetSearchResp 测试用 manifestSearch 返回模型。
type testWinGetSearchResp struct {
	Data []struct {
		PackageIdentifier string `json:"PackageIdentifier"`
		PackageName       string `json:"PackageName"`
		Publisher         string `json:"Publisher"`
		Versions          []struct {
			PackageVersion string `json:"PackageVersion"`
			Channel        string `json:"Channel"`
		} `json:"Versions"`
	} `json:"Data"`
}

// TestFeedWinGet_MinimalGetSequence 验收项 1：对照官方 REST 源契约的最小 GET 序列通过契约测试。
// 顺序：GET /information -> GET /packageManifests/{PackageIdentifier} -> POST /manifestSearch。
func TestFeedWinGet_MinimalGetSequence(t *testing.T) {
	r, projSvc, _, ctx := setupCheckTest(t)

	slug := "wingetdemo"
	p, _, err := projSvc.Create(ctx, service.CreateProjectInput{DefaultLocale: ptr("en"), Slug: &slug})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := projSvc.CreateMatrix(ctx, p.ID, service.MatrixWrite{
		OS: ptr("windows"), Arch: ptr("x86_64"), PackageType: ptr(model.PackageTypeSingleFile),
	}); err != nil {
		t.Fatal(err)
	}

	pkgID := "Contoso.AiDemo"
	enableListing(t, projSvc, ctx, p.ID, "winget", "default", map[string]string{
		"package_identifier": pkgID,
		"publisher":          "Contoso",
		"package_name":       "AiDemo",
		"license":            "MIT",
	})

	publishSingleFile(t, projSvc, ctx, p.ID, "1.0.0", "stable", "windows", "x86_64", 10, "aidemo-1.0.0.exe",
		func(in *service.VersionWriteInput) { in.Changelog = ptr("first release") })

	// 1. GET /information
	infoURL := "/api/v1/projects/" + slug + "/store/winget/default/information"
	wInfo := perform(r, http.MethodGet, infoURL)
	if wInfo.Code != http.StatusOK {
		t.Fatalf("information status=%d body=%s", wInfo.Code, wInfo.Body.String())
	}
	if ct := wInfo.Header().Get("Content-Type"); !strings.Contains(ct, "application/json") {
		t.Fatalf("information content-type=%q, want application/json", ct)
	}
	var infoResp testWinGetInformationResp
	if err := json.Unmarshal(wInfo.Body.Bytes(), &infoResp); err != nil {
		t.Fatalf("unmarshal info: %v", err)
	}
	if infoResp.Data.SourceIdentifier != pkgID {
		t.Fatalf("SourceIdentifier = %q, want %q", infoResp.Data.SourceIdentifier, pkgID)
	}
	has110 := false
	for _, sv := range infoResp.Data.ServerSupportedVersions {
		if sv == "1.1.0" {
			has110 = true
		}
	}
	if !has110 {
		t.Fatalf("ServerSupportedVersions %v must include 1.1.0", infoResp.Data.ServerSupportedVersions)
	}

	// 2. GET /packageManifests/{PackageIdentifier}
	manifestURL := "/api/v1/projects/" + slug + "/store/winget/default/packageManifests/" + pkgID
	wManifest := perform(r, http.MethodGet, manifestURL)
	if wManifest.Code != http.StatusOK {
		t.Fatalf("packageManifests status=%d body=%s", wManifest.Code, wManifest.Body.String())
	}
	if ct := wManifest.Header().Get("Content-Type"); !strings.Contains(ct, "application/json") {
		t.Fatalf("packageManifests content-type=%q, want application/json", ct)
	}
	etag := wManifest.Header().Get("ETag")
	if etag == "" {
		t.Fatal("manifest ETag must be present")
	}

	var mResp testWinGetManifestResp
	if err := json.Unmarshal(wManifest.Body.Bytes(), &mResp); err != nil {
		t.Fatalf("unmarshal manifest: %v", err)
	}
	if mResp.Data.PackageIdentifier != pkgID {
		t.Fatalf("manifest PackageIdentifier = %q, want %q", mResp.Data.PackageIdentifier, pkgID)
	}
	if len(mResp.Data.Versions) != 1 {
		t.Fatalf("manifest Versions len = %d, want 1", len(mResp.Data.Versions))
	}
	verItem := mResp.Data.Versions[0]
	if verItem.PackageVersion != "1.0.0" {
		t.Fatalf("PackageVersion = %q, want 1.0.0", verItem.PackageVersion)
	}
	if len(verItem.Installers) != 1 {
		t.Fatalf("Installers len = %d, want 1", len(verItem.Installers))
	}
	inst := verItem.Installers[0]
	if inst.Architecture != "x64" {
		t.Fatalf("Architecture = %q, want x64", inst.Architecture)
	}
	if inst.InstallerType != "exe" {
		t.Fatalf("InstallerType = %q, want exe", inst.InstallerType)
	}
	if inst.InstallerSha256 == "" || inst.InstallerSha256 != strings.ToUpper(inst.InstallerSha256) {
		t.Fatalf("InstallerSha256 = %q, want uppercase hex", inst.InstallerSha256)
	}

	// ETag 条件请求 -> 304 Not Modified
	req304 := httptest.NewRequest(http.MethodGet, manifestURL, nil)
	req304.Header.Set("If-None-Match", etag)
	w304 := httptest.NewRecorder()
	r.ServeHTTP(w304, req304)
	if w304.Code != http.StatusNotModified {
		t.Fatalf("status=%d, want 304", w304.Code)
	}

	// 3. POST /manifestSearch (以及 GET /manifestSearch)
	searchURL := "/api/v1/projects/" + slug + "/store/winget/default/manifestSearch"
	searchBody := []byte(`{"Query":{"KeyWord":"AiDemo"}}`)
	reqSearch := httptest.NewRequest(http.MethodPost, searchURL, bytes.NewReader(searchBody))
	reqSearch.Header.Set("Content-Type", "application/json")
	wSearch := httptest.NewRecorder()
	r.ServeHTTP(wSearch, reqSearch)
	if wSearch.Code != http.StatusOK {
		t.Fatalf("manifestSearch POST status=%d body=%s", wSearch.Code, wSearch.Body.String())
	}
	var sResp testWinGetSearchResp
	if err := json.Unmarshal(wSearch.Body.Bytes(), &sResp); err != nil {
		t.Fatalf("unmarshal search: %v", err)
	}
	if len(sResp.Data) != 1 {
		t.Fatalf("search results len = %d, want 1", len(sResp.Data))
	}
	if sResp.Data[0].PackageIdentifier != pkgID {
		t.Fatalf("search PackageIdentifier = %q, want %q", sResp.Data[0].PackageIdentifier, pkgID)
	}

	// GET /manifestSearch 也应成功返回列表
	wSearchGet := perform(r, http.MethodGet, searchURL)
	if wSearchGet.Code != http.StatusOK {
		t.Fatalf("manifestSearch GET status=%d body=%s", wSearchGet.Code, wSearchGet.Body.String())
	}
}

// TestFeedWinGet_PackageIdentifierFromStoreProtocols 验收项 2：PackageIdentifier / Publisher 等取自商店协议配置。
func TestFeedWinGet_PackageIdentifierFromStoreProtocols(t *testing.T) {
	r, projSvc, _, ctx := setupCheckTest(t)

	slug := "customidents"
	p, _, err := projSvc.Create(ctx, service.CreateProjectInput{DefaultLocale: ptr("en"), Slug: &slug})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := projSvc.CreateMatrix(ctx, p.ID, service.MatrixWrite{
		OS: ptr("windows"), Arch: ptr("x86_64"), PackageType: ptr(model.PackageTypeSingleFile),
	}); err != nil {
		t.Fatal(err)
	}

	customID := "AcmeCorp.SuperTool"
	customPub := "Acme Corp"
	customName := "Super Tool Deluxe"
	customLicense := "Apache-2.0"
	customDesc := "Super Tool for Windows"

	enableListing(t, projSvc, ctx, p.ID, "winget", "default", map[string]string{
		"package_identifier": customID,
		"publisher":          customPub,
		"package_name":       customName,
		"license":            customLicense,
		"short_description":  customDesc,
	})

	publishSingleFile(t, projSvc, ctx, p.ID, "2.5.0", "stable", "windows", "x86_64", 25, "supertool-2.5.0.exe",
		func(in *service.VersionWriteInput) { in.Changelog = ptr("v2.5.0 update") })

	// 查询 packageManifests/{customID}
	manifestURL := "/api/v1/projects/" + slug + "/store/winget/default/packageManifests/" + customID
	w := perform(r, http.MethodGet, manifestURL)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}

	var mResp testWinGetManifestResp
	if err := json.Unmarshal(w.Body.Bytes(), &mResp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if mResp.Data.PackageIdentifier != customID {
		t.Fatalf("PackageIdentifier = %q, want %q", mResp.Data.PackageIdentifier, customID)
	}
	ver := mResp.Data.Versions[0]
	if ver.DefaultLocale.Publisher != customPub {
		t.Fatalf("Publisher = %q, want %q", ver.DefaultLocale.Publisher, customPub)
	}
	if ver.DefaultLocale.PackageName != customName {
		t.Fatalf("PackageName = %q, want %q", ver.DefaultLocale.PackageName, customName)
	}
	if ver.DefaultLocale.License != customLicense {
		t.Fatalf("License = %q, want %q", ver.DefaultLocale.License, customLicense)
	}
}

// TestFeedWinGet_TokenAuth 验收项 3：require_client_token / store_token 开启时无 Token -> 401 UNAUTHORIZED，带 Token -> 200。
func TestFeedWinGet_TokenAuth(t *testing.T) {
	r, projSvc, _, ctx := setupCheckTest(t)

	secret := "winget-secret-token"
	slug := "winget-tok"
	p, _, err := projSvc.Create(ctx, service.CreateProjectInput{DefaultLocale: ptr("en"), Slug: &slug, StoreToken: &secret})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := projSvc.CreateMatrix(ctx, p.ID, service.MatrixWrite{
		OS: ptr("windows"), Arch: ptr("x86_64"), PackageType: ptr(model.PackageTypeSingleFile),
	}); err != nil {
		t.Fatal(err)
	}
	enableListing(t, projSvc, ctx, p.ID, "winget", "default", nil)

	publishSingleFile(t, projSvc, ctx, p.ID, "1.0.0", "stable", "windows", "x86_64", 10, "app-1.0.0.exe",
		func(in *service.VersionWriteInput) { in.Changelog = ptr("release") })

	url := "/api/v1/projects/" + slug + "/store/winget/default/information"

	// 1. 无 Token -> 401
	wAnon := perform(r, http.MethodGet, url)
	if wAnon.Code != http.StatusUnauthorized {
		t.Fatalf("status=%d without token, want 401", wAnon.Code)
	}
	if !strings.Contains(wAnon.Body.String(), "UNAUTHORIZED") {
		t.Fatalf("401 body=%s", wAnon.Body.String())
	}

	// 2. 带有效 Token -> 200
	reqAuth := httptest.NewRequest(http.MethodGet, url, nil)
	reqAuth.Header.Set("X-Project-Token", secret)
	wAuth := httptest.NewRecorder()
	r.ServeHTTP(wAuth, reqAuth)
	if wAuth.Code != http.StatusOK {
		t.Fatalf("status=%d with token, want 200", wAuth.Code)
	}
}

// TestFeedWinGet_DisabledProtocolReturns404NonZip 开关关闭时纯文本 404（绝不回退 zip）。
func TestFeedWinGet_DisabledProtocolReturns404NonZip(t *testing.T) {
	r, projSvc, _, ctx := setupCheckTest(t)

	slug := "winget-disabled"
	p, _, err := projSvc.Create(ctx, service.CreateProjectInput{DefaultLocale: ptr("en"), Slug: &slug})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := projSvc.CreateMatrix(ctx, p.ID, service.MatrixWrite{
		OS: ptr("windows"), Arch: ptr("x86_64"), PackageType: ptr(model.PackageTypeSingleFile),
	}); err != nil {
		t.Fatal(err)
	}
	// 不开启 winget 协议（默认关闭）
	publishSingleFile(t, projSvc, ctx, p.ID, "1.0.0", "stable", "windows", "x86_64", 10, "app-1.0.0.exe",
		func(in *service.VersionWriteInput) { in.Changelog = ptr("release") })

	url := "/api/v1/projects/" + slug + "/store/winget/default/information"
	w := perform(r, http.MethodGet, url)
	if w.Code != http.StatusNotFound {
		t.Fatalf("status=%d, want 404", w.Code)
	}
	if strings.Contains(w.Header().Get("Content-Type"), "zip") {
		t.Fatal("must not return zip when protocol disabled")
	}
}

// TestFeedWinGet_EmptyCatalogReturns404 可见集为空时 packageManifests 返回 404。
func TestFeedWinGet_EmptyCatalogReturns404(t *testing.T) {
	r, projSvc, _, ctx := setupCheckTest(t)

	slug := "winget-empty"
	p, _, err := projSvc.Create(ctx, service.CreateProjectInput{DefaultLocale: ptr("en"), Slug: &slug})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := projSvc.CreateMatrix(ctx, p.ID, service.MatrixWrite{
		OS: ptr("windows"), Arch: ptr("x86_64"), PackageType: ptr(model.PackageTypeSingleFile),
	}); err != nil {
		t.Fatal(err)
	}
	enableListing(t, projSvc, ctx, p.ID, "winget", "default", nil)

	// 未发布任何版本 -> packageManifests 返回 404
	url := "/api/v1/projects/" + slug + "/store/winget/default/packageManifests"
	w := perform(r, http.MethodGet, url)
	if w.Code != http.StatusNotFound {
		t.Fatalf("status=%d, want 404 on empty catalog", w.Code)
	}
}

// TestFeedWinGet_UnknownPathReturns404 非标准路径返回 404。
func TestFeedWinGet_UnknownPathReturns404(t *testing.T) {
	r, projSvc, _, ctx := setupCheckTest(t)

	slug := "winget-badpath"
	p, _, err := projSvc.Create(ctx, service.CreateProjectInput{DefaultLocale: ptr("en"), Slug: &slug})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := projSvc.CreateMatrix(ctx, p.ID, service.MatrixWrite{
		OS: ptr("windows"), Arch: ptr("x86_64"), PackageType: ptr(model.PackageTypeSingleFile),
	}); err != nil {
		t.Fatal(err)
	}
	enableListing(t, projSvc, ctx, p.ID, "winget", "default", nil)
	publishSingleFile(t, projSvc, ctx, p.ID, "1.0.0", "stable", "windows", "x86_64", 10, "app-1.0.0.exe",
		func(in *service.VersionWriteInput) { in.Changelog = ptr("release") })

	badPaths := []string{
		"latest.json",
		"packages",
		"RELEASES",
		"appcast.xml",
		"manifest.yml",
	}

	for _, pth := range badPaths {
		url := "/api/v1/projects/" + slug + "/store/winget/default/" + pth
		w := perform(r, http.MethodGet, url)
		if w.Code != http.StatusNotFound {
			t.Fatalf("path %q status=%d, want 404", pth, w.Code)
		}
	}
}
