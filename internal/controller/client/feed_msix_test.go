package client

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Kirizu-Official/KiriVers/internal/model"
	"github.com/Kirizu-Official/KiriVers/internal/service"
	"github.com/Kirizu-Official/KiriVers/pkg/hashutil"
)

// ---------- MSIX .appinstaller feed HTTP 集成测试（§9 / §9.2，C23-1..C23-6） ----------

// enableMSIXPatch 返回启用 msix 协议的 Patch 输入。
// testClientAppInstallerXML 是测试中验证 App Installer XML 结构的最小模型。
type testClientAppInstallerXML struct {
	XMLName        xml.Name `xml:"http://schemas.microsoft.com/appx/appinstaller/2018 AppInstaller"`
	Version        string   `xml:"Version,attr"`
	Uri            string   `xml:"Uri,attr"`
	MainPackage    struct {
		Name                  string `xml:"Name,attr"`
		Publisher             string `xml:"Publisher,attr"`
		Version               string `xml:"Version,attr"`
		ProcessorArchitecture string `xml:"ProcessorArchitecture,attr"`
		Uri                   string `xml:"Uri,attr"`
	} `xml:"MainPackage"`
	UpdateSettings struct {
		OnLaunch struct {
			HoursBetweenUpdateChecks string `xml:"HoursBetweenUpdateChecks,attr"`
		} `xml:"OnLaunch"`
	} `xml:"UpdateSettings"`
}

// parseClientAppInstallerXML 解析 App Installer XML 清单响应。
func parseClientAppInstallerXML(t *testing.T, body []byte) *testClientAppInstallerXML {
	t.Helper()
	var doc testClientAppInstallerXML
	if err := xml.Unmarshal(body, &doc); err != nil {
		t.Fatalf("xml unmarshal failed: %v\nbody:\n%s", err, body)
	}
	return &doc
}

// TestFeedMSIX_EndToEnd 验收项 1：.appinstaller 全链路：XML 符合 App Installer 必填元素 fixture，
// Content-Type、ETag 与 304 条件请求。
func TestFeedMSIX_EndToEnd(t *testing.T) {
	r, projSvc, _, ctx := setupCheckTest(t)

	slug := "msixdemo"
	p, _, err := projSvc.Create(ctx, service.CreateProjectInput{DefaultLocale: ptr("en"), Slug: &slug})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := projSvc.CreateMatrix(ctx, p.ID, service.MatrixWrite{
		OS: ptr("windows"), Arch: ptr("x86_64"), PackageType: ptr(model.PackageTypeSingleFile),
	}); err != nil {
		t.Fatal(err)
	}
	enableListing(t, projSvc, ctx, p.ID, "msix", "default", nil)

	payload1 := []byte("msix-payload-1.0.0")
	sha256_1, _ := hashutil.SHA256Hex(bytes.NewReader(payload1))

	if _, _, err := projSvc.PutVersion(ctx, p.ID, "1.0.0", service.VersionWriteInput{
		Channel:        "stable",
		Changelog:      ptr("msixdemo 1.0.0"),
		VersionInteger: ptr(int64(10)),
	}); err != nil {
		t.Fatalf("put version 1.0.0: %v", err)
	}
	if _, err := projSvc.UploadArtifact(ctx, fmt.Sprint(p.ID), "1.0.0", "windows", "x86_64", service.UploadArtifactInput{
		Filename:       "msixdemo-1.0.0-x64.msix",
		ExpectedSHA256: sha256_1,
		Size:           int64(len(payload1)),
		ContentType:    "application/x-msix",
	}, bytes.NewReader(payload1)); err != nil {
		t.Fatalf("upload 1.0.0: %v", err)
	}
	if _, err := projSvc.ReadyVersionLine(ctx, p.ID, "1.0.0", "windows", "x86_64"); err != nil {
		t.Fatalf("ready 1.0.0: %v", err)
	}
	if _, err := projSvc.PublishVersion(ctx, p.ID, "1.0.0"); err != nil {
		t.Fatalf("publish 1.0.0: %v", err)
	}

	payload2 := []byte("msix-payload-1.1.0")
	sha256_2, _ := hashutil.SHA256Hex(bytes.NewReader(payload2))

	if _, _, err := projSvc.PutVersion(ctx, p.ID, "1.1.0", service.VersionWriteInput{
		Channel:        "stable",
		Changelog:      ptr("msixdemo 1.1.0"),
		VersionInteger: ptr(int64(11)),
	}); err != nil {
		t.Fatalf("put version 1.1.0: %v", err)
	}
	if _, err := projSvc.UploadArtifact(ctx, fmt.Sprint(p.ID), "1.1.0", "windows", "x86_64", service.UploadArtifactInput{
		Filename:       "msixdemo-1.1.0-x64.msix",
		ExpectedSHA256: sha256_2,
		Size:           int64(len(payload2)),
		ContentType:    "application/x-msix",
	}, bytes.NewReader(payload2)); err != nil {
		t.Fatalf("upload 1.1.0: %v", err)
	}
	if _, err := projSvc.ReadyVersionLine(ctx, p.ID, "1.1.0", "windows", "x86_64"); err != nil {
		t.Fatalf("ready 1.1.0: %v", err)
	}
	if _, err := projSvc.PublishVersion(ctx, p.ID, "1.1.0"); err != nil {
		t.Fatalf("publish 1.1.0: %v", err)
	}

	// GET .appinstaller feed
	url := "/api/v1/projects/msixdemo/store/msix/default/app.appinstaller"
	w := perform(r, http.MethodGet, url)
	if w.Code != http.StatusOK {
		t.Fatalf("GET status=%d want 200, body=%s", w.Code, w.Body.String())
	}
	if ct := w.Header().Get("Content-Type"); ct != "application/appinstaller+xml; charset=utf-8" {
		t.Fatalf("Content-Type = %q, want application/appinstaller+xml; charset=utf-8", ct)
	}

	etag := w.Header().Get("ETag")
	if etag == "" {
		t.Fatal("ETag must be set on 200 response")
	}

	doc := parseClientAppInstallerXML(t, w.Body.Bytes())
	// 最新发布是 1.1.0，4 段式版本应为 1.1.0.11
	if doc.Version != "1.1.0.11" {
		t.Fatalf("root Version = %q, want 1.1.0.11", doc.Version)
	}
	if doc.MainPackage.Version != "1.1.0.11" {
		t.Fatalf("MainPackage Version = %q, want 1.1.0.11", doc.MainPackage.Version)
	}
	if doc.MainPackage.ProcessorArchitecture != "x64" {
		t.Fatalf("MainPackage ProcessorArchitecture = %q, want x64", doc.MainPackage.ProcessorArchitecture)
	}
	if !strings.Contains(doc.MainPackage.Uri, "/packages/") {
		t.Fatalf("MainPackage Uri = %q, want hash package URL", doc.MainPackage.Uri)
	}
	if doc.UpdateSettings.OnLaunch.HoursBetweenUpdateChecks != "0" {
		t.Fatalf("HoursBetweenUpdateChecks = %q, want 0", doc.UpdateSettings.OnLaunch.HoursBetweenUpdateChecks)
	}

	// If-None-Match 条件请求 → 304 Not Modified
	req304 := httptest.NewRequest(http.MethodGet, url, nil)
	req304.Header.Set("If-None-Match", etag)
	rec304 := httptest.NewRecorder()
	r.ServeHTTP(rec304, req304)
	if rec304.Code != http.StatusNotModified {
		t.Fatalf("If-None-Match status=%d want 304", rec304.Code)
	}
}

// TestFeedMSIX_NonMSIXArtifactExcluded 验收项 2：非 msix 扩展名的 Line 不会出现在 appinstaller 的 MainPackage。
func TestFeedMSIX_NonMSIXArtifactExcluded(t *testing.T) {
	r, projSvc, _, ctx := setupCheckTest(t)

	slug := "msixfilter"
	p, _, err := projSvc.Create(ctx, service.CreateProjectInput{DefaultLocale: ptr("en"), Slug: &slug})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := projSvc.CreateMatrix(ctx, p.ID, service.MatrixWrite{
		OS: ptr("windows"), Arch: ptr("x86_64"), PackageType: ptr(model.PackageTypeSingleFile),
	}); err != nil {
		t.Fatal(err)
	}
	enableListing(t, projSvc, ctx, p.ID, "msix", "default", nil)

	// 发布 1.0.0，产物为 .msix
	publishSingleFile(t, projSvc, ctx, p.ID, "1.0.0", "stable", "windows", "x86_64", 10, "msixfilter-1.0.0.msix")

	// 发布 2.0.0，产物为 .zip（非 msix！）
	publishSingleFile(t, projSvc, ctx, p.ID, "2.0.0", "stable", "windows", "x86_64", 20, "msixfilter-2.0.0.zip")

	w := perform(r, http.MethodGet, "/api/v1/projects/msixfilter/store/msix/default/package.appinstaller")
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d want 200, body=%s", w.Code, w.Body.String())
	}

	doc := parseClientAppInstallerXML(t, w.Body.Bytes())
	// MainPackage 必须指向 1.0.0 的 msix，而不是 2.0.0.zip
	if !strings.HasPrefix(doc.MainPackage.Version, "1.0.0") {
		t.Fatalf("MainPackage Version = %q, want 1.0.0.x (should exclude 2.0.0.zip)", doc.MainPackage.Version)
	}
	if !strings.Contains(doc.MainPackage.Uri, "/packages/") {
		t.Fatalf("MainPackage Uri = %q, want hash package URL", doc.MainPackage.Uri)
	}
}

// TestFeedMSIX_AllNonMSIXReturns404 C23-4：当所有版本产物均非 msix 时返回 404，不冒充 zip。
func TestFeedMSIX_AllNonMSIXReturns404(t *testing.T) {
	r, projSvc, _, ctx := setupCheckTest(t)

	slug := "allnonmsix"
	p, _, err := projSvc.Create(ctx, service.CreateProjectInput{DefaultLocale: ptr("en"), Slug: &slug})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := projSvc.CreateMatrix(ctx, p.ID, service.MatrixWrite{
		OS: ptr("windows"), Arch: ptr("x86_64"), PackageType: ptr(model.PackageTypeSingleFile),
	}); err != nil {
		t.Fatal(err)
	}
	enableListing(t, projSvc, ctx, p.ID, "msix", "default", nil)

	// 产物为 .exe 与 .zip
	publishSingleFile(t, projSvc, ctx, p.ID, "1.0.0", "stable", "windows", "x86_64", 10, "app-1.0.0.exe")
	publishSingleFile(t, projSvc, ctx, p.ID, "2.0.0", "stable", "windows", "x86_64", 20, "app-2.0.0.zip")

	w := perform(r, http.MethodGet, "/api/v1/projects/allnonmsix/store/msix/default/app.appinstaller")
	if w.Code != http.StatusNotFound {
		t.Fatalf("status=%d want 404", w.Code)
	}
	if strings.HasPrefix(w.Body.String(), "PK") {
		t.Fatal("404 body must not be a zip archive")
	}
	if strings.Contains(w.Body.String(), "<AppInstaller") {
		t.Fatal("404 body must not be an AppInstaller XML")
	}
}

// TestFeedMSIX_Token401 验收项 3：require_client_token / store_token 开启时缺 Token → 401 UNAUTHORIZED，带 Token → 200。
func TestFeedMSIX_Token401(t *testing.T) {
	r, projSvc, _, ctx := setupCheckTest(t)

	secret := "msix-secret-token"
	slug := "msixtok"
	p, _, err := projSvc.Create(ctx, service.CreateProjectInput{DefaultLocale: ptr("en"), Slug: &slug, StoreToken: &secret})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := projSvc.CreateMatrix(ctx, p.ID, service.MatrixWrite{
		OS: ptr("windows"), Arch: ptr("x86_64"), PackageType: ptr(model.PackageTypeSingleFile),
	}); err != nil {
		t.Fatal(err)
	}
	enableListing(t, projSvc, ctx, p.ID, "msix", "default", nil)

	publishSingleFile(t, projSvc, ctx, p.ID, "1.0.0", "stable", "windows", "x86_64", 10, "msixtok-1.0.0.msix")

	url := "/api/v1/projects/msixtok/store/msix/default/app.appinstaller"

	// 1. 无 Token → 401
	wNoToken := perform(r, http.MethodGet, url)
	if wNoToken.Code != http.StatusUnauthorized {
		t.Fatalf("no token status=%d want 401", wNoToken.Code)
	}

	// 2. 错误 Token → 401
	reqBad := httptest.NewRequest(http.MethodGet, url, nil)
	reqBad.Header.Set("X-Project-Token", "wrong-token")
	recBad := httptest.NewRecorder()
	r.ServeHTTP(recBad, reqBad)
	if recBad.Code != http.StatusUnauthorized {
		t.Fatalf("wrong token status=%d want 401", recBad.Code)
	}

	// 3. 正确 Token → 200
	reqGood := httptest.NewRequest(http.MethodGet, url, nil)
	reqGood.Header.Set("X-Project-Token", secret)
	recGood := httptest.NewRecorder()
	r.ServeHTTP(recGood, reqGood)
	if recGood.Code != http.StatusOK {
		t.Fatalf("good token status=%d want 200", recGood.Code)
	}
}

// TestFeedMSIX_DisabledProtocolReturns404NonZip 验证协议关闭时返回纯文本 404，不返回 zip。
func TestFeedMSIX_DisabledProtocolReturns404NonZip(t *testing.T) {
	r, projSvc, _, ctx := setupCheckTest(t)

	slug := "msixdisabled"
	p, _, err := projSvc.Create(ctx, service.CreateProjectInput{DefaultLocale: ptr("en"), Slug: &slug})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := projSvc.CreateMatrix(ctx, p.ID, service.MatrixWrite{
		OS: ptr("windows"), Arch: ptr("x86_64"), PackageType: ptr(model.PackageTypeSingleFile),
	}); err != nil {
		t.Fatal(err)
	}
	// 不开启 msix 协议
	publishSingleFile(t, projSvc, ctx, p.ID, "1.0.0", "stable", "windows", "x86_64", 10, "app-1.0.0.msix")

	w := perform(r, http.MethodGet, "/api/v1/projects/msixdisabled/store/msix/default/app.appinstaller")
	if w.Code != http.StatusNotFound {
		t.Fatalf("disabled protocol status=%d want 404", w.Code)
	}
	if strings.HasPrefix(w.Body.String(), "PK") {
		t.Fatal("404 body must not be a zip archive")
	}
}

// TestFeedMSIX_InvalidPath404 验证非 .appinstaller 路径返回 404。
func TestFeedMSIX_InvalidPath404(t *testing.T) {
	r, projSvc, _, ctx := setupCheckTest(t)

	slug := "msixbadpath"
	p, _, err := projSvc.Create(ctx, service.CreateProjectInput{DefaultLocale: ptr("en"), Slug: &slug})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := projSvc.CreateMatrix(ctx, p.ID, service.MatrixWrite{
		OS: ptr("windows"), Arch: ptr("x86_64"), PackageType: ptr(model.PackageTypeSingleFile),
	}); err != nil {
		t.Fatal(err)
	}
	enableListing(t, projSvc, ctx, p.ID, "msix", "default", nil)

	publishSingleFile(t, projSvc, ctx, p.ID, "1.0.0", "stable", "windows", "x86_64", 10, "app-1.0.0.msix")

	for _, badPath := range []string{"app.xml", "app.msix", "latest.json", "manifest"} {
		w := perform(r, http.MethodGet, fmt.Sprintf("/api/v1/projects/msixbadpath/store/msix/default/%s", badPath))
		if w.Code != http.StatusNotFound {
			t.Fatalf("path %q status=%d want 404", badPath, w.Code)
		}
	}
}
