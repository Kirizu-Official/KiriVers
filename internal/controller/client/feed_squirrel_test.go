package client

import (
	"bytes"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/Kirizu-Official/KiriVers/internal/model"
	"github.com/Kirizu-Official/KiriVers/internal/service"
	"github.com/Kirizu-Official/KiriVers/pkg/hashutil"
)

// ---------- Squirrel RELEASES feed HTTP 集成测试（§9 / §9.2，C19-1..C19-6） ----------
//
// 复用 check_test.go 的 setupCheckTest / publishSingleFile / perform / ptr。

// enableSquirrelPatch 返回启用 squirrel 协议的 Patch 输入。
// squirrelEntry 是 RELEASES 解析结果。
type testSquirrelEntry struct {
	SHA1     string
	FileName string
	Size     int64
}

var squirrelLineRegex = regexp.MustCompile(`^([0-9a-fA-F]{40})\s+(\S+)\s+(\d+)$`)

// parseTestRELEASES 行式解析 Squirrel RELEASES 文件。
func parseTestRELEASES(t *testing.T, body []byte) []testSquirrelEntry {
	t.Helper()
	lines := strings.Split(string(body), "\n")
	var entries []testSquirrelEntry
	for i, line := range lines {
		if line == "" {
			if i != len(lines)-1 {
				t.Fatalf("unexpected empty line at index %d:\n%s", i, body)
			}
			continue
		}
		m := squirrelLineRegex.FindStringSubmatch(line)
		if m == nil {
			t.Fatalf("line %d %q does not match squirrel regex", i, line)
		}
		sz, err := strconv.ParseInt(m[3], 10, 64)
		if err != nil {
			t.Fatalf("line %d invalid size %q: %v", i, m[3], err)
		}
		entries = append(entries, testSquirrelEntry{
			SHA1:     m[1],
			FileName: m[2],
			Size:     sz,
		})
	}
	return entries
}

// TestFeedSquirrelEndToEnd RELEASES 全链路：两版本新→旧行序、SHA-1 独立验证、
// 文件名与 size、Content-Type、ETag 与 304 条件请求。
func TestFeedSquirrelEndToEnd(t *testing.T) {
	r, projSvc, _, ctx := setupCheckTest(t)

	slug := "sdemo"
	p, _, err := projSvc.Create(ctx, service.CreateProjectInput{DefaultLocale: ptr("en"), Slug: &slug})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := projSvc.CreateMatrix(ctx, p.ID, service.MatrixWrite{
		OS: ptr("windows"), Arch: ptr("x86_64"), PackageType: ptr(model.PackageTypeSingleFile),
	}); err != nil {
		t.Fatal(err)
	}
	enableListing(t, projSvc, ctx, p.ID, "squirrel", "default", nil)

	payload1 := []byte("nupkg-payload-1.0.0")
	sha1Val1 := sha1.Sum(payload1)
	sha1Hex1 := hex.EncodeToString(sha1Val1[:])
	sha256_1, _ := hashutil.SHA256Hex(bytes.NewReader(payload1))

	if _, _, err := projSvc.PutVersion(ctx, p.ID, "1.0.0", service.VersionWriteInput{
		Channel:        "stable",
		Changelog:      ptr("sdemo 1.0.0"),
		VersionInteger: ptr(int64(10)),
	}); err != nil {
		t.Fatalf("put version 1.0.0: %v", err)
	}
	if _, err := projSvc.UploadArtifact(ctx, fmt.Sprint(p.ID), "1.0.0", "windows", "x86_64", service.UploadArtifactInput{
		Filename:       "sdemo-1.0.0-full.nupkg",
		ExpectedSHA256: sha256_1,
		Size:           int64(len(payload1)),
		ContentType:    "application/octet-stream",
	}, bytes.NewReader(payload1)); err != nil {
		t.Fatalf("upload 1.0.0: %v", err)
	}
	if _, err := projSvc.ReadyVersionLine(ctx, p.ID, "1.0.0", "windows", "x86_64"); err != nil {
		t.Fatal(err)
	}
	if _, err := projSvc.PublishVersion(ctx, p.ID, "1.0.0"); err != nil {
		t.Fatal(err)
	}

	payload2 := []byte("nupkg-payload-1.1.0-newer")
	sha1Val2 := sha1.Sum(payload2)
	sha1Hex2 := hex.EncodeToString(sha1Val2[:])
	sha256_2, _ := hashutil.SHA256Hex(bytes.NewReader(payload2))

	if _, _, err := projSvc.PutVersion(ctx, p.ID, "1.1.0", service.VersionWriteInput{
		Channel:        "stable",
		Changelog:      ptr("sdemo 1.1.0"),
		VersionInteger: ptr(int64(11)),
	}); err != nil {
		t.Fatalf("put version 1.1.0: %v", err)
	}
	if _, err := projSvc.UploadArtifact(ctx, fmt.Sprint(p.ID), "1.1.0", "windows", "x86_64", service.UploadArtifactInput{
		Filename:       "sdemo-1.1.0-full.nupkg",
		ExpectedSHA256: sha256_2,
		Size:           int64(len(payload2)),
		ContentType:    "application/octet-stream",
	}, bytes.NewReader(payload2)); err != nil {
		t.Fatalf("upload 1.1.0: %v", err)
	}
	if _, err := projSvc.ReadyVersionLine(ctx, p.ID, "1.1.0", "windows", "x86_64"); err != nil {
		t.Fatal(err)
	}
	if _, err := projSvc.PublishVersion(ctx, p.ID, "1.1.0"); err != nil {
		t.Fatal(err)
	}

	url := "/api/v1/projects/sdemo/store/squirrel/default/RELEASES"
	w := perform(r, http.MethodGet, url)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	if ct := w.Header().Get("Content-Type"); ct != "text/plain; charset=utf-8" {
		t.Fatalf("content-type=%q, want text/plain; charset=utf-8", ct)
	}
	etag := w.Header().Get("ETag")
	if etag == "" {
		t.Fatal("ETag must be present")
	}
	if cc := w.Header().Get("Cache-Control"); !strings.Contains(cc, "s-maxage=60") {
		t.Fatalf("cache-control=%q, want s-maxage=60", cc)
	}

	entries := parseTestRELEASES(t, w.Body.Bytes())
	if len(entries) != 2 {
		t.Fatalf("entries count = %d, want 2", len(entries))
	}

	// 新→旧行序：1.1.0 第一行，1.0.0 第二行（RELEASES 名为 {sha256}.nupkg）
	if !strings.HasSuffix(entries[0].FileName, ".nupkg") || entries[0].SHA1 != sha1Hex2 || entries[0].Size != int64(len(payload2)) {
		t.Fatalf("entry 0 mismatch: %+v, want sha1=%s", entries[0], sha1Hex2)
	}
	if !strings.HasSuffix(entries[1].FileName, ".nupkg") || entries[1].SHA1 != sha1Hex1 || entries[1].Size != int64(len(payload1)) {
		t.Fatalf("entry 1 mismatch: %+v, want sha1=%s", entries[1], sha1Hex1)
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

// TestFeedSquirrelDisabled404NonZip 验收项 2：开关关闭时纯文本 404，不返回假 RELEASES，更不回退通用 zip。
func TestFeedSquirrelDisabled404NonZip(t *testing.T) {
	r, projSvc, _, ctx := setupCheckTest(t)

	slug := "sclosed"
	p, _, err := projSvc.Create(ctx, service.CreateProjectInput{DefaultLocale: ptr("en"), Slug: &slug})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := projSvc.CreateMatrix(ctx, p.ID, service.MatrixWrite{
		OS: ptr("windows"), Arch: ptr("x86_64"), PackageType: ptr(model.PackageTypeSingleFile),
	}); err != nil {
		t.Fatal(err)
	}
	publishSingleFile(t, projSvc, ctx, p.ID, "1.0.0", "stable", "windows", "x86_64", 10, "sclosed-1.0.0-full.nupkg",
		func(in *service.VersionWriteInput) { in.Changelog = ptr("closed release") })

	// 未开启 squirrel 协议
	w := perform(r, http.MethodGet, "/api/v1/projects/sclosed/store/squirrel/default/RELEASES")
	if w.Code != http.StatusNotFound {
		t.Fatalf("disabled protocol status=%d want 404", w.Code)
	}
	if strings.HasPrefix(w.Body.String(), "PK") {
		t.Fatal("404 body must not be a zip archive")
	}
	if strings.Contains(w.Body.String(), "sclosed-1.0.0-full.nupkg") {
		t.Fatal("404 body must not contain release filenames")
	}
}

// TestFeedSquirrelNoRelease404 可见集为空（如全为灰度中）→ 纯文本 404，不返回空文件冒充（C19-4）。
func TestFeedSquirrelNoRelease404(t *testing.T) {
	r, projSvc, _, ctx := setupCheckTest(t)

	slug := "sempty"
	p, _, err := projSvc.Create(ctx, service.CreateProjectInput{DefaultLocale: ptr("en"), Slug: &slug})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := projSvc.CreateMatrix(ctx, p.ID, service.MatrixWrite{
		OS: ptr("windows"), Arch: ptr("x86_64"), PackageType: ptr(model.PackageTypeSingleFile),
	}); err != nil {
		t.Fatal(err)
	}
	enableListing(t, projSvc, ctx, p.ID, "squirrel", "default", nil)
	seedClients(t, projSvc, p, 2)
	publishSingleFile(t, projSvc, ctx, p.ID, "1.0.0", "stable", "windows", "x86_64", 10, "sempty-1.0.0-full.nupkg",
		func(in *service.VersionWriteInput) {
			in.GrayStartPercent = ptr(0)
			in.IsCritical = ptr(false)
		})

	w := perform(r, http.MethodGet, "/api/v1/projects/sempty/store/squirrel/default/RELEASES")
	if w.Code != http.StatusNotFound {
		t.Fatalf("empty visible release status=%d want 404", w.Code)
	}
	if strings.TrimSpace(w.Body.String()) == "" {
		t.Fatal("empty visible release must not return empty 200 or empty body")
	}
}

// TestFeedSquirrelToken401 验收项 3：require_client_token / store_token 开启时缺 Token → 401 UNAUTHORIZED，带 Token → 200。
func TestFeedSquirrelToken401(t *testing.T) {
	r, projSvc, _, ctx := setupCheckTest(t)

	secret := "squirrel-secret-token"
	slug := "stok"
	p, _, err := projSvc.Create(ctx, service.CreateProjectInput{DefaultLocale: ptr("en"), Slug: &slug, StoreToken: &secret})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := projSvc.CreateMatrix(ctx, p.ID, service.MatrixWrite{
		OS: ptr("windows"), Arch: ptr("x86_64"), PackageType: ptr(model.PackageTypeSingleFile),
	}); err != nil {
		t.Fatal(err)
	}
	enableListing(t, projSvc, ctx, p.ID, "squirrel", "default", nil)
	publishSingleFile(t, projSvc, ctx, p.ID, "1.0.0", "stable", "windows", "x86_64", 10, "stok-1.0.0-full.nupkg",
		func(in *service.VersionWriteInput) { in.Changelog = ptr("stok release") })

	url := "/api/v1/projects/stok/store/squirrel/default/RELEASES"

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

// TestFeedSquirrelUnknownPath404 未知路径（非 RELEASES）→ 纯文本 404。
func TestFeedSquirrelUnknownPath404(t *testing.T) {
	r, projSvc, _, ctx := setupCheckTest(t)

	slug := "spath"
	p, _, err := projSvc.Create(ctx, service.CreateProjectInput{DefaultLocale: ptr("en"), Slug: &slug})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := projSvc.CreateMatrix(ctx, p.ID, service.MatrixWrite{
		OS: ptr("windows"), Arch: ptr("x86_64"), PackageType: ptr(model.PackageTypeSingleFile),
	}); err != nil {
		t.Fatal(err)
	}
	enableListing(t, projSvc, ctx, p.ID, "squirrel", "default", nil)
	publishSingleFile(t, projSvc, ctx, p.ID, "1.0.0", "stable", "windows", "x86_64", 10, "spath-1.0.0-full.nupkg",
		func(in *service.VersionWriteInput) { in.Changelog = ptr("path test") })

	for _, pth := range []string{"releases", "RELEASES.txt", "latest.json", "other"} {
		w := perform(r, http.MethodGet, "/api/v1/projects/spath/store/squirrel/default/"+pth)
		if w.Code != http.StatusNotFound {
			t.Fatalf("path %q status=%d want 404", pth, w.Code)
		}
	}
}

// TestFeedSquirrelIdentifiersConfig 项目配置 StoreProtocols["squirrel"].Identifiers（C19-2）不阻断正常服务。
func TestFeedSquirrelIdentifiersConfig(t *testing.T) {
	r, projSvc, _, ctx := setupCheckTest(t)

	slug := "sident"
	p, _, err := projSvc.Create(ctx, service.CreateProjectInput{DefaultLocale: ptr("en"), Slug: &slug})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := projSvc.CreateMatrix(ctx, p.ID, service.MatrixWrite{
		OS: ptr("windows"), Arch: ptr("x86_64"), PackageType: ptr(model.PackageTypeSingleFile),
	}); err != nil {
		t.Fatal(err)
	}
	enableListing(t, projSvc, ctx, p.ID, "squirrel", "default", map[string]string{
		"releases_name": "RELEASES",
	})
	publishSingleFile(t, projSvc, ctx, p.ID, "1.0.0", "stable", "windows", "x86_64", 10, "sident-1.0.0-full.nupkg",
		func(in *service.VersionWriteInput) { in.Changelog = ptr("ident test") })

	w := perform(r, http.MethodGet, "/api/v1/projects/sident/store/squirrel/default/RELEASES")
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	entries := parseTestRELEASES(t, w.Body.Bytes())
	if len(entries) != 1 {
		t.Fatalf("entries count=%d, want 1", len(entries))
	}
}
