package client

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
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

// ---------- ClickOnce .application feed HTTP 集成测试（§9 / §9.2，C20-1..C20-6） ----------
//
// 复用 check_test.go 的 setupCheckTest / publishSingleFile / perform / ptr。

// enableClickOncePatch 返回启用 clickonce 协议的 Patch 输入。
// testClickOnceXML 是测试中验证 ClickOnce XML 结构的最小模型。
type testClickOnceXML struct {
	XMLName          xml.Name `xml:"urn:schemas-microsoft-com:asm.v1 assembly"`
	AssemblyIdentity struct {
		Name                  string `xml:"name,attr"`
		Version               string `xml:"version,attr"`
		ProcessorArchitecture string `xml:"processorArchitecture,attr"`
	} `xml:"assemblyIdentity"`
	Description struct {
		Publisher string `xml:"publisher,attr"`
		Product   string `xml:"product,attr"`
	} `xml:"description"`
	Dependency struct {
		DependentAssembly struct {
			Codebase string `xml:"codebase,attr"`
			Size     int64  `xml:"size,attr"`
			Hash     struct {
				DigestValue string `xml:"DigestValue"`
			} `xml:"hash"`
		} `xml:"dependentAssembly"`
	} `xml:"dependency"`
}

// parseClientClickOnceXML 解析 ClickOnce XML 清单响应。
func parseClientClickOnceXML(t *testing.T, body []byte) *testClickOnceXML {
	t.Helper()
	var doc testClickOnceXML
	if err := xml.Unmarshal(body, &doc); err != nil {
		t.Fatalf("xml unmarshal failed: %v\nbody:\n%s", err, body)
	}
	return &doc
}

// TestFeedClickOnceEndToEnd .application 全链路：4 段式版本号、SHA-256 DigestValue、
// 依赖全量包、Content-Type、ETag 与 304 条件请求。
func TestFeedClickOnceEndToEnd(t *testing.T) {
	r, projSvc, _, ctx := setupCheckTest(t)

	slug := "cdemo"
	p, _, err := projSvc.Create(ctx, service.CreateProjectInput{DefaultLocale: ptr("en"), Slug: &slug})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := projSvc.CreateMatrix(ctx, p.ID, service.MatrixWrite{
		OS: ptr("windows"), Arch: ptr("x86_64"), PackageType: ptr(model.PackageTypeSingleFile),
	}); err != nil {
		t.Fatal(err)
	}
	enableListing(t, projSvc, ctx, p.ID, "clickonce", "default", nil)

	payload1 := []byte("payload-1.0.0")
	sha256_1, _ := hashutil.SHA256Hex(bytes.NewReader(payload1))

	if _, _, err := projSvc.PutVersion(ctx, p.ID, "1.0.0", service.VersionWriteInput{
		Channel:        "stable",
		Changelog:      ptr("cdemo 1.0.0"),
		VersionInteger: ptr(int64(10)),
	}); err != nil {
		t.Fatalf("put version 1.0.0: %v", err)
	}
	if _, err := projSvc.UploadArtifact(ctx, fmt.Sprint(p.ID), "1.0.0", "windows", "x86_64", service.UploadArtifactInput{
		Filename:       "cdemo-1.0.0.exe",
		ExpectedSHA256: sha256_1,
		Size:           int64(len(payload1)),
		ContentType:    "application/x-msdownload",
	}, bytes.NewReader(payload1)); err != nil {
		t.Fatalf("upload 1.0.0: %v", err)
	}
	if _, err := projSvc.ReadyVersionLine(ctx, p.ID, "1.0.0", "windows", "x86_64"); err != nil {
		t.Fatal(err)
	}
	if _, err := projSvc.PublishVersion(ctx, p.ID, "1.0.0"); err != nil {
		t.Fatal(err)
	}

	payload2 := []byte("payload-1.1.0-newer")
	rawSHA2 := sha256.Sum256(payload2)
	expectedB64_2 := base64.StdEncoding.EncodeToString(rawSHA2[:])
	sha256_2, _ := hashutil.SHA256Hex(bytes.NewReader(payload2))

	if _, _, err := projSvc.PutVersion(ctx, p.ID, "1.1.0", service.VersionWriteInput{
		Channel:        "stable",
		Changelog:      ptr("cdemo 1.1.0"),
		VersionInteger: ptr(int64(11)),
	}); err != nil {
		t.Fatalf("put version 1.1.0: %v", err)
	}
	if _, err := projSvc.UploadArtifact(ctx, fmt.Sprint(p.ID), "1.1.0", "windows", "x86_64", service.UploadArtifactInput{
		Filename:       "cdemo-1.1.0.exe",
		ExpectedSHA256: sha256_2,
		Size:           int64(len(payload2)),
		ContentType:    "application/x-msdownload",
	}, bytes.NewReader(payload2)); err != nil {
		t.Fatalf("upload 1.1.0: %v", err)
	}
	if _, err := projSvc.ReadyVersionLine(ctx, p.ID, "1.1.0", "windows", "x86_64"); err != nil {
		t.Fatal(err)
	}
	if _, err := projSvc.PublishVersion(ctx, p.ID, "1.1.0"); err != nil {
		t.Fatal(err)
	}

	url := "/api/v1/projects/cdemo/store/clickonce/default/cdemo.application"
	w := perform(r, http.MethodGet, url)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	if ct := w.Header().Get("Content-Type"); !strings.Contains(ct, "application/x-ms-application") {
		t.Fatalf("content-type=%q, want application/x-ms-application", ct)
	}
	etag := w.Header().Get("ETag")
	if etag == "" {
		t.Fatal("ETag must be present")
	}
	if cc := w.Header().Get("Cache-Control"); !strings.Contains(cc, "s-maxage=60") {
		t.Fatalf("cache-control=%q, want s-maxage=60", cc)
	}

	doc := parseClientClickOnceXML(t, w.Body.Bytes())
	// 最新发布是 1.1.0，4 段式版本应为 1.1.0.11
	if doc.AssemblyIdentity.Version != "1.1.0.11" {
		t.Fatalf("version = %q, want 1.1.0.11", doc.AssemblyIdentity.Version)
	}
	if doc.AssemblyIdentity.ProcessorArchitecture != "amd64" {
		t.Fatalf("processorArchitecture = %q, want amd64", doc.AssemblyIdentity.ProcessorArchitecture)
	}
	if doc.Dependency.DependentAssembly.Size != int64(len(payload2)) {
		t.Fatalf("dep size = %d, want %d", doc.Dependency.DependentAssembly.Size, len(payload2))
	}
	if doc.Dependency.DependentAssembly.Hash.DigestValue != expectedB64_2 {
		t.Fatalf("digestValue = %q, want %q", doc.Dependency.DependentAssembly.Hash.DigestValue, expectedB64_2)
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

// TestFeedClickOnceDisabled404NonZip 验收项 2：开关关闭时纯文本 404，不返回假清单，更不回退通用 zip。
func TestFeedClickOnceDisabled404NonZip(t *testing.T) {
	r, projSvc, _, ctx := setupCheckTest(t)

	slug := "cclosed"
	p, _, err := projSvc.Create(ctx, service.CreateProjectInput{DefaultLocale: ptr("en"), Slug: &slug})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := projSvc.CreateMatrix(ctx, p.ID, service.MatrixWrite{
		OS: ptr("windows"), Arch: ptr("x86_64"), PackageType: ptr(model.PackageTypeSingleFile),
	}); err != nil {
		t.Fatal(err)
	}
	publishSingleFile(t, projSvc, ctx, p.ID, "1.0.0", "stable", "windows", "x86_64", 10, "cclosed-1.0.0.exe",
		func(in *service.VersionWriteInput) { in.Changelog = ptr("closed release") })

	// 未开启 clickonce 协议
	w := perform(r, http.MethodGet, "/api/v1/projects/cclosed/store/clickonce/default/cclosed.application")
	if w.Code != http.StatusNotFound {
		t.Fatalf("disabled protocol status=%d want 404", w.Code)
	}
	if strings.HasPrefix(w.Body.String(), "PK") {
		t.Fatal("404 body must not be a zip archive")
	}
	if strings.Contains(w.Body.String(), "assembly") {
		t.Fatal("404 body must not contain XML manifest")
	}
}

// TestFeedClickOnceNoRelease404 可见集为空（如灰度未 100% 且非关键）→ 纯文本 404（C20-3）。
func TestFeedClickOnceNoRelease404(t *testing.T) {
	r, projSvc, _, ctx := setupCheckTest(t)

	slug := "cempty"
	p, _, err := projSvc.Create(ctx, service.CreateProjectInput{DefaultLocale: ptr("en"), Slug: &slug})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := projSvc.CreateMatrix(ctx, p.ID, service.MatrixWrite{
		OS: ptr("windows"), Arch: ptr("x86_64"), PackageType: ptr(model.PackageTypeSingleFile),
	}); err != nil {
		t.Fatal(err)
	}
	enableListing(t, projSvc, ctx, p.ID, "clickonce", "default", nil)
	seedClients(t, projSvc, p, 2)
	publishSingleFile(t, projSvc, ctx, p.ID, "1.0.0", "stable", "windows", "x86_64", 10, "cempty-1.0.0.exe",
		func(in *service.VersionWriteInput) {
			in.GrayStartPercent = ptr(0)
			in.IsCritical = ptr(false)
		})

	w := perform(r, http.MethodGet, "/api/v1/projects/cempty/store/clickonce/default/cempty.application")
	if w.Code != http.StatusNotFound {
		t.Fatalf("empty visible release status=%d want 404", w.Code)
	}
	if strings.TrimSpace(w.Body.String()) == "" {
		t.Fatal("empty visible release must not return empty 200 or empty body")
	}
}

// TestFeedClickOnceToken401 验收项 3：require_client_token / store_token 开启时缺 Token → 401 UNAUTHORIZED，带 Token → 200。
func TestFeedClickOnceToken401(t *testing.T) {
	r, projSvc, _, ctx := setupCheckTest(t)

	secret := "clickonce-secret-token"
	slug := "ctok"
	p, _, err := projSvc.Create(ctx, service.CreateProjectInput{DefaultLocale: ptr("en"), Slug: &slug, StoreToken: &secret})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := projSvc.CreateMatrix(ctx, p.ID, service.MatrixWrite{
		OS: ptr("windows"), Arch: ptr("x86_64"), PackageType: ptr(model.PackageTypeSingleFile),
	}); err != nil {
		t.Fatal(err)
	}
	enableListing(t, projSvc, ctx, p.ID, "clickonce", "default", nil)
	publishSingleFile(t, projSvc, ctx, p.ID, "1.0.0", "stable", "windows", "x86_64", 10, "ctok-1.0.0.exe",
		func(in *service.VersionWriteInput) { in.Changelog = ptr("ctok release") })

	url := "/api/v1/projects/ctok/store/clickonce/default/ctok.application"

	// 1. 无 Token → 401 UNAUTHORIZED
	w := perform(r, http.MethodGet, url)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("no token status=%d want 401", w.Code)
	}
	if !strings.Contains(w.Body.String(), "UNAUTHORIZED") {
		t.Fatalf("401 body=%s", w.Body.String())
	}

	// 2. 带有效 Token → 200 OK
	req := httptest.NewRequest(http.MethodGet, url, nil)
	req.Header.Set("X-Project-Token", secret)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("valid token status=%d body=%s", rec.Code, rec.Body.String())
	}
}

// TestFeedClickOnceUnknownPath404 路径非 .application 结尾 → 纯文本 404。
func TestFeedClickOnceUnknownPath404(t *testing.T) {
	r, projSvc, _, ctx := setupCheckTest(t)

	slug := "cpath"
	p, _, err := projSvc.Create(ctx, service.CreateProjectInput{DefaultLocale: ptr("en"), Slug: &slug})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := projSvc.CreateMatrix(ctx, p.ID, service.MatrixWrite{
		OS: ptr("windows"), Arch: ptr("x86_64"), PackageType: ptr(model.PackageTypeSingleFile),
	}); err != nil {
		t.Fatal(err)
	}
	enableListing(t, projSvc, ctx, p.ID, "clickonce", "default", nil)
	publishSingleFile(t, projSvc, ctx, p.ID, "1.0.0", "stable", "windows", "x86_64", 10, "cpath-1.0.0.exe",
		func(in *service.VersionWriteInput) { in.Changelog = ptr("cpath release") })

	for _, pth := range []string{"app.exe", "RELEASES", "latest.json", "manifest.xml"} {
		w := perform(r, http.MethodGet, "/api/v1/projects/cpath/store/clickonce/default/"+pth)
		if w.Code != http.StatusNotFound {
			t.Fatalf("path %q status=%d want 404", pth, w.Code)
		}
	}
}

// TestFeedClickOnceIdentifiersConfig 配置 StoreProtocols["clickonce"].Identifiers 反映在清单中。
func TestFeedClickOnceIdentifiersConfig(t *testing.T) {
	r, projSvc, _, ctx := setupCheckTest(t)

	slug := "cident"
	p, _, err := projSvc.Create(ctx, service.CreateProjectInput{DefaultLocale: ptr("en"), Slug: &slug})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := projSvc.CreateMatrix(ctx, p.ID, service.MatrixWrite{
		OS: ptr("windows"), Arch: ptr("x86_64"), PackageType: ptr(model.PackageTypeSingleFile),
	}); err != nil {
		t.Fatal(err)
	}
	enableListing(t, projSvc, ctx, p.ID, "clickonce", "default", map[string]string{
		"publisher":        "Custom Publisher",
		"product":          "Custom Product",
		"public_key_token": "1234567890abcdef",
	})
	publishSingleFile(t, projSvc, ctx, p.ID, "1.0.0", "stable", "windows", "x86_64", 10, "cident-1.0.0.exe",
		func(in *service.VersionWriteInput) { in.Changelog = ptr("ident test") })

	w := perform(r, http.MethodGet, "/api/v1/projects/cident/store/clickonce/default/cident.application")
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	doc := parseClientClickOnceXML(t, w.Body.Bytes())
	if doc.Description.Publisher != "Custom Publisher" {
		t.Fatalf("publisher = %q, want Custom Publisher", doc.Description.Publisher)
	}
	if doc.Description.Product != "Custom Product" {
		t.Fatalf("product = %q, want Custom Product", doc.Description.Product)
	}
}
