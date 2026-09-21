package client

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Kirizu-Official/KiriVers/internal/model"
	"github.com/Kirizu-Official/KiriVers/internal/service"
	"github.com/Kirizu-Official/KiriVers/pkg/hashutil"
)

// ---------- F-Droid feed HTTP 集成测试（§9 / §9.2，C24-1..C24-7） ----------

// enableFDroidPatch 返回启用 fdroid 协议的 Patch 输入。
// testFDroidV1JSON 是测试验证 index-v1.json 响应的结构。
type testFDroidV1JSON struct {
	Repo struct {
		Name      string `json:"name"`
		Address   string `json:"address"`
		Timestamp int64  `json:"timestamp"`
	} `json:"repo"`
	Apps []struct {
		PackageName string `json:"packageName"`
		Name        string `json:"name"`
	} `json:"apps"`
	Packages map[string][]struct {
		VersionName string   `json:"versionName"`
		VersionCode int64    `json:"versionCode"`
		Size        int64    `json:"size"`
		Hash        string   `json:"hash"`
		ApkName     string   `json:"apkName"`
		NativeCode  []string `json:"nativecode"`
	} `json:"packages"`
}

// testFDroidV2JSON 是测试验证 index-v2.json 响应的结构。
type testFDroidV2JSON struct {
	Repo struct {
		Name map[string]string `json:"name"`
	} `json:"repo"`
	Packages map[string]struct {
		Metadata struct {
			Name map[string]string `json:"name"`
		} `json:"metadata"`
		Versions map[string]struct {
			File struct {
				Name   string `json:"name"`
				SHA256 string `json:"sha256"`
				Size   int64  `json:"size"`
			} `json:"file"`
			Manifest struct {
				VersionName string `json:"versionName"`
				VersionCode int64  `json:"versionCode"`
			} `json:"manifest"`
		} `json:"versions"`
	} `json:"packages"`
}

// testFDroidEntryJSON 是测试验证 entry.json 响应的结构。
type testFDroidEntryJSON struct {
	Timestamp int64 `json:"timestamp"`
	Index     struct {
		Name   string `json:"name"`
		SHA256 string `json:"sha256"`
		Size   int64  `json:"size"`
	} `json:"index"`
}

// TestFeedFDroid_EndToEnd_IndexV1 验收项 1：.apk 全链路投影至 index-v1.json，
// Content-Type、ETag 与 304 条件请求。
func TestFeedFDroid_EndToEnd_IndexV1(t *testing.T) {
	r, projSvc, _, ctx := setupCheckTest(t)

	slug := "fdroiddemo"
	p, _, err := projSvc.Create(ctx, service.CreateProjectInput{DefaultLocale: ptr("en"), Slug: &slug})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := projSvc.CreateMatrix(ctx, p.ID, service.MatrixWrite{
		OS: ptr("android"), Arch: ptr("arm64"), PackageType: ptr(model.PackageTypeSingleFile),
	}); err != nil {
		t.Fatal(err)
	}
	enableListing(t, projSvc, ctx, p.ID, "fdroid", "default", nil)

	payload1 := []byte("apk-payload-1.0.0")
	sha256_1, _ := hashutil.SHA256Hex(bytes.NewReader(payload1))

	if _, _, err := projSvc.PutVersion(ctx, p.ID, "1.0.0", service.VersionWriteInput{
		Channel:        "stable",
		Changelog:      ptr("fdroiddemo 1.0.0"),
		VersionInteger: ptr(int64(10)),
	}); err != nil {
		t.Fatalf("put version 1.0.0: %v", err)
	}
	if _, err := projSvc.UploadArtifact(ctx, fmt.Sprint(p.ID), "1.0.0", "android", "arm64", service.UploadArtifactInput{
		Filename:       "fdroiddemo-1.0.0.apk",
		ExpectedSHA256: sha256_1,
		Size:           int64(len(payload1)),
		ContentType:    "application/vnd.android.package-archive",
	}, bytes.NewReader(payload1)); err != nil {
		t.Fatalf("upload 1.0.0: %v", err)
	}
	if _, err := projSvc.ReadyVersionLine(ctx, p.ID, "1.0.0", "android", "arm64"); err != nil {
		t.Fatalf("ready 1.0.0: %v", err)
	}
	if _, err := projSvc.PublishVersion(ctx, p.ID, "1.0.0"); err != nil {
		t.Fatalf("publish 1.0.0: %v", err)
	}

	payload2 := []byte("apk-payload-1.1.0")
	sha256_2, _ := hashutil.SHA256Hex(bytes.NewReader(payload2))

	if _, _, err := projSvc.PutVersion(ctx, p.ID, "1.1.0", service.VersionWriteInput{
		Channel:        "stable",
		Changelog:      ptr("fdroiddemo 1.1.0"),
		VersionInteger: ptr(int64(11)),
	}); err != nil {
		t.Fatalf("put version 1.1.0: %v", err)
	}
	if _, err := projSvc.UploadArtifact(ctx, fmt.Sprint(p.ID), "1.1.0", "android", "arm64", service.UploadArtifactInput{
		Filename:       "fdroiddemo-1.1.0.apk",
		ExpectedSHA256: sha256_2,
		Size:           int64(len(payload2)),
		ContentType:    "application/vnd.android.package-archive",
	}, bytes.NewReader(payload2)); err != nil {
		t.Fatalf("upload 1.1.0: %v", err)
	}
	if _, err := projSvc.ReadyVersionLine(ctx, p.ID, "1.1.0", "android", "arm64"); err != nil {
		t.Fatalf("ready 1.1.0: %v", err)
	}
	if _, err := projSvc.PublishVersion(ctx, p.ID, "1.1.0"); err != nil {
		t.Fatalf("publish 1.1.0: %v", err)
	}

	url := "/api/v1/projects/fdroiddemo/store/fdroid/default/index-v1.json"
	w := perform(r, http.MethodGet, url)
	if w.Code != http.StatusOK {
		t.Fatalf("GET status=%d want 200, body=%s", w.Code, w.Body.String())
	}
	if ct := w.Header().Get("Content-Type"); ct != "application/json; charset=utf-8" {
		t.Fatalf("Content-Type = %q, want application/json; charset=utf-8", ct)
	}

	etag := w.Header().Get("ETag")
	if etag == "" {
		t.Fatal("ETag must be set on 200 response")
	}

	var doc testFDroidV1JSON
	if err := json.Unmarshal(w.Body.Bytes(), &doc); err != nil {
		t.Fatalf("unmarshal index-v1.json: %v", err)
	}

	if doc.Repo.Name != "fdroiddemo Repository" {
		t.Fatalf("repo.name = %q, want fdroiddemo Repository", doc.Repo.Name)
	}

	pkgs := doc.Packages["fdroiddemo"]
	if len(pkgs) != 2 {
		t.Fatalf("len(pkgs) = %d, want 2", len(pkgs))
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

// TestFeedFDroid_EndToEnd_IndexV2 验收项 1：.apk 全链路投影至 index-v2.json。
func TestFeedFDroid_EndToEnd_IndexV2(t *testing.T) {
	r, projSvc, _, ctx := setupCheckTest(t)

	slug := "fdroidv2"
	p, _, err := projSvc.Create(ctx, service.CreateProjectInput{DefaultLocale: ptr("en"), Slug: &slug})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := projSvc.CreateMatrix(ctx, p.ID, service.MatrixWrite{
		OS: ptr("android"), Arch: ptr("arm64"), PackageType: ptr(model.PackageTypeSingleFile),
	}); err != nil {
		t.Fatal(err)
	}
	enableListing(t, projSvc, ctx, p.ID, "fdroid", "default", nil)

	publishSingleFile(t, projSvc, ctx, p.ID, "1.0.0", "stable", "android", "arm64", 10, "app-1.0.0.apk")

	w := perform(r, http.MethodGet, "/api/v1/projects/fdroidv2/store/fdroid/default/index-v2.json")
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d want 200, body=%s", w.Code, w.Body.String())
	}

	var doc testFDroidV2JSON
	if err := json.Unmarshal(w.Body.Bytes(), &doc); err != nil {
		t.Fatalf("unmarshal index-v2.json: %v", err)
	}

	pkgData, ok := doc.Packages["fdroidv2"]
	if !ok {
		t.Fatalf("package fdroidv2 not found in packages: %v", doc.Packages)
	}
	if len(pkgData.Versions) != 1 {
		t.Fatalf("len(versions) = %d, want 1", len(pkgData.Versions))
	}
}

// TestFeedFDroid_EndToEnd_EntryJSON 验证 entry.json 响应。
func TestFeedFDroid_EndToEnd_EntryJSON(t *testing.T) {
	r, projSvc, _, ctx := setupCheckTest(t)

	slug := "fdroidentry"
	p, _, err := projSvc.Create(ctx, service.CreateProjectInput{DefaultLocale: ptr("en"), Slug: &slug})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := projSvc.CreateMatrix(ctx, p.ID, service.MatrixWrite{
		OS: ptr("android"), Arch: ptr("arm64"), PackageType: ptr(model.PackageTypeSingleFile),
	}); err != nil {
		t.Fatal(err)
	}
	enableListing(t, projSvc, ctx, p.ID, "fdroid", "default", nil)

	publishSingleFile(t, projSvc, ctx, p.ID, "1.0.0", "stable", "android", "arm64", 10, "app-1.0.0.apk")

	w := perform(r, http.MethodGet, "/api/v1/projects/fdroidentry/store/fdroid/default/entry.json")
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d want 200, body=%s", w.Code, w.Body.String())
	}

	var doc testFDroidEntryJSON
	if err := json.Unmarshal(w.Body.Bytes(), &doc); err != nil {
		t.Fatalf("unmarshal entry.json: %v", err)
	}

	if doc.Index.Name != "/index-v2.json" {
		t.Fatalf("index.name = %q, want /index-v2.json", doc.Index.Name)
	}
	if doc.Index.SHA256 == "" || len(doc.Index.SHA256) != 64 {
		t.Fatalf("invalid index.sha256: %q", doc.Index.SHA256)
	}
	if doc.Index.Size <= 0 {
		t.Fatalf("invalid index.size: %d", doc.Index.Size)
	}
}

// TestFeedFDroid_WindowsLineExcluded 验收项 2：windows 多文件 Line 不出现在 F-Droid 索引。
func TestFeedFDroid_WindowsLineExcluded(t *testing.T) {
	r, projSvc, _, ctx := setupCheckTest(t)

	slug := "multiplatform"
	p, _, err := projSvc.Create(ctx, service.CreateProjectInput{DefaultLocale: ptr("en"), Slug: &slug})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := projSvc.CreateMatrix(ctx, p.ID, service.MatrixWrite{
		OS: ptr("android"), Arch: ptr("arm64"), PackageType: ptr(model.PackageTypeSingleFile),
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := projSvc.CreateMatrix(ctx, p.ID, service.MatrixWrite{
		OS: ptr("windows"), Arch: ptr("x86_64"), PackageType: ptr(model.PackageTypeSingleFile),
	}); err != nil {
		t.Fatal(err)
	}
	enableListing(t, projSvc, ctx, p.ID, "fdroid", "default", nil)

	// 发布 Android 1.0.0 apk
	publishSingleFile(t, projSvc, ctx, p.ID, "1.0.0", "stable", "android", "arm64", 10, "app-1.0.0.apk")

	// 发布 Windows 2.0.0 zip
	publishSingleFile(t, projSvc, ctx, p.ID, "2.0.0", "stable", "windows", "x86_64", 20, "app-2.0.0.zip")

	w := perform(r, http.MethodGet, "/api/v1/projects/multiplatform/store/fdroid/default/index-v1.json")
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d want 200, body=%s", w.Code, w.Body.String())
	}

	var doc testFDroidV1JSON
	if err := json.Unmarshal(w.Body.Bytes(), &doc); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	pkgs := doc.Packages["multiplatform"]
	if len(pkgs) != 1 {
		t.Fatalf("len(pkgs) = %d, want 1 (windows 2.0.0 must be excluded)", len(pkgs))
	}
	if pkgs[0].VersionName != "1.0.0" {
		t.Fatalf("pkgs[0].VersionName = %q, want 1.0.0", pkgs[0].VersionName)
	}
	if !strings.Contains(pkgs[0].ApkName, "/packages/") {
		t.Fatalf("pkgs[0].ApkName = %q, want hash package URL", pkgs[0].ApkName)
	}
}

// TestFeedFDroid_DisabledProtocolReturns404 验收项 3：开关关闭时不返回空仓库冒充已适配，返回 404。
func TestFeedFDroid_DisabledProtocolReturns404(t *testing.T) {
	r, projSvc, _, ctx := setupCheckTest(t)

	slug := "fdroiddisabled"
	p, _, err := projSvc.Create(ctx, service.CreateProjectInput{DefaultLocale: ptr("en"), Slug: &slug})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := projSvc.CreateMatrix(ctx, p.ID, service.MatrixWrite{
		OS: ptr("android"), Arch: ptr("arm64"), PackageType: ptr(model.PackageTypeSingleFile),
	}); err != nil {
		t.Fatal(err)
	}
	// 不开启 fdroid 开关
	publishSingleFile(t, projSvc, ctx, p.ID, "1.0.0", "stable", "android", "arm64", 10, "app-1.0.0.apk")

	w := perform(r, http.MethodGet, "/api/v1/projects/fdroiddisabled/store/fdroid/default/index-v1.json")
	if w.Code != http.StatusNotFound {
		t.Fatalf("disabled protocol status=%d want 404", w.Code)
	}
	if strings.Contains(w.Body.String(), "packages") {
		t.Fatal("404 response must not return empty or fake repository JSON")
	}
}

// TestFeedFDroid_TokenAuth 验收项 4：Token 开关打开时无 Token → 401，有 Token → 200。
func TestFeedFDroid_TokenAuth(t *testing.T) {
	r, projSvc, _, ctx := setupCheckTest(t)

	secret := "fdroid-secret-token"
	slug := "fdroidtok"
	p, _, err := projSvc.Create(ctx, service.CreateProjectInput{DefaultLocale: ptr("en"), Slug: &slug, StoreToken: &secret})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := projSvc.CreateMatrix(ctx, p.ID, service.MatrixWrite{
		OS: ptr("android"), Arch: ptr("arm64"), PackageType: ptr(model.PackageTypeSingleFile),
	}); err != nil {
		t.Fatal(err)
	}
	enableListing(t, projSvc, ctx, p.ID, "fdroid", "default", nil)

	publishSingleFile(t, projSvc, ctx, p.ID, "1.0.0", "stable", "android", "arm64", 10, "app-1.0.0.apk")

	url := "/api/v1/projects/fdroidtok/store/fdroid/default/index-v1.json"

	// 1. 无 Token → 401
	wNoToken := perform(r, http.MethodGet, url)
	if wNoToken.Code != http.StatusUnauthorized {
		t.Fatalf("no token status=%d want 401", wNoToken.Code)
	}

	// 2. 错误 Token → 401
	reqBad := httptest.NewRequest(http.MethodGet, url, nil)
	reqBad.Header.Set("X-Project-Token", "invalid-token")
	recBad := httptest.NewRecorder()
	r.ServeHTTP(recBad, reqBad)
	if recBad.Code != http.StatusUnauthorized {
		t.Fatalf("bad token status=%d want 401", recBad.Code)
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

// TestFeedFDroid_CustomIdentifiers 验证自定义 identifiers 配置。
func TestFeedFDroid_CustomIdentifiers(t *testing.T) {
	r, projSvc, _, ctx := setupCheckTest(t)

	slug := "customfdroid"
	p, _, err := projSvc.Create(ctx, service.CreateProjectInput{DefaultLocale: ptr("en"), Slug: &slug})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := projSvc.CreateMatrix(ctx, p.ID, service.MatrixWrite{
		OS: ptr("android"), Arch: ptr("arm64"), PackageType: ptr(model.PackageTypeSingleFile),
	}); err != nil {
		t.Fatal(err)
	}
	enableListing(t, projSvc, ctx, p.ID, "fdroid", "default", map[string]string{
		"package_name": "org.kiri.customapp",
		"name":         "Kiri Custom App",
		"repo_name":    "Kiri Repo",
	})

	publishSingleFile(t, projSvc, ctx, p.ID, "1.0.0", "stable", "android", "arm64", 10, "app-1.0.0.apk")

	w := perform(r, http.MethodGet, "/api/v1/projects/customfdroid/store/fdroid/default/index-v1.json")
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d want 200", w.Code)
	}

	var doc testFDroidV1JSON
	if err := json.Unmarshal(w.Body.Bytes(), &doc); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if doc.Repo.Name != "Kiri Repo" {
		t.Fatalf("repo.name = %q, want Kiri Repo", doc.Repo.Name)
	}
	if len(doc.Apps) != 1 || doc.Apps[0].PackageName != "org.kiri.customapp" {
		t.Fatalf("app package name = %v, want org.kiri.customapp", doc.Apps)
	}
	if _, ok := doc.Packages["org.kiri.customapp"]; !ok {
		t.Fatalf("package map key org.kiri.customapp missing: %v", doc.Packages)
	}
}
