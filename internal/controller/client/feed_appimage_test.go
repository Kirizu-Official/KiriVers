package client

import (
	"bytes"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"golang.org/x/crypto/md4"

	"github.com/Kirizu-Official/KiriVers/internal/model"
	"github.com/Kirizu-Official/KiriVers/internal/service"
	"github.com/Kirizu-Official/KiriVers/pkg/hashutil"
)

// ---------- AppImageUpdate .zsync feed HTTP 集成测试（§9 / §9.2，C21-1..C21-6） ----------
//
// 复用 check_test.go 的 setupCheckTest / publishSingleFile / perform / ptr。

// enableAppImagePatch 返回启用 appimage 协议的 Patch 输入。
// validateHTTPZSyncBody 校验 HTTP 响应的 .zsync 控制文件头与各块校验和。
func validateHTTPZSyncBody(t *testing.T, body, originalBytes []byte) (filename string, length int64, sha1Hex string) {
	t.Helper()

	sepIdx := bytes.Index(body, []byte("\n\n"))
	if sepIdx == -1 {
		t.Fatalf("missing header separator \\n\\n in .zsync response")
	}

	headerText := string(body[:sepIdx])
	blockSums := body[sepIdx+2:]

	headers := make(map[string]string)
	for _, line := range strings.Split(headerText, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		k, v, ok := strings.Cut(line, ":")
		if !ok {
			t.Fatalf("malformed header: %q", line)
		}
		headers[strings.TrimSpace(k)] = strings.TrimSpace(v)
	}

	if headers["zsync"] != "0.6.2" {
		t.Fatalf("zsync version = %q, want 0.6.2", headers["zsync"])
	}
	filename = headers["Filename"]
	if filename == "" {
		t.Fatalf("header Filename is empty")
	}

	bs, _ := strconv.Atoi(headers["Blocksize"])
	if bs <= 0 || (bs&(bs-1)) != 0 {
		t.Fatalf("invalid Blocksize: %d", bs)
	}

	lenVal, _ := strconv.ParseInt(headers["Length"], 10, 64)
	if lenVal != int64(len(originalBytes)) {
		t.Fatalf("Length = %d, want %d", lenVal, len(originalBytes))
	}

	hl := strings.Split(headers["Hash-Lengths"], ",")
	if len(hl) != 3 {
		t.Fatalf("invalid Hash-Lengths: %v", headers["Hash-Lengths"])
	}
	rsumL, _ := strconv.Atoi(hl[1])
	checkL, _ := strconv.Atoi(hl[2])

	sha1Hex = headers["SHA-1"]
	expectedSHA1 := sha1.Sum(originalBytes)
	if strings.ToLower(sha1Hex) != hex.EncodeToString(expectedSHA1[:]) {
		t.Fatalf("SHA-1 = %q, want %x", sha1Hex, expectedSHA1)
	}

	numBlocks := int((lenVal + int64(bs) - 1) / int64(bs))
	perBlock := rsumL + checkL
	if len(blockSums) != numBlocks*perBlock {
		t.Fatalf("blockSums len = %d, want %d", len(blockSums), numBlocks*perBlock)
	}

	blockBuf := make([]byte, bs)
	for b := 0; b < numBlocks; b++ {
		start := int64(b * bs)
		end := start + int64(bs)
		if end > lenVal {
			end = lenVal
		}
		clear(blockBuf)
		copy(blockBuf, originalBytes[start:end])

		// 滚动校验和
		var a, bSum uint16
		n := len(blockBuf)
		for i, c := range blockBuf {
			uc := uint16(c)
			a += uc
			bSum += uint16(n-i) * uc
		}
		rBuf := [4]byte{byte(a >> 8), byte(a), byte(bSum >> 8), byte(bSum)}
		wantRSum := rBuf[4-rsumL : 4]

		offset := b * perBlock
		gotRSum := blockSums[offset : offset+rsumL]
		if !bytes.Equal(gotRSum, wantRSum) {
			t.Fatalf("block %d rsum mismatch: got %x, want %x", b, gotRSum, wantRSum)
		}

		// MD4 校验和
		m := md4.New()
		m.Write(blockBuf)
		wantMD4 := m.Sum(nil)[:checkL]
		gotMD4 := blockSums[offset+rsumL : offset+perBlock]
		if !bytes.Equal(gotMD4, wantMD4) {
			t.Fatalf("block %d md4 mismatch: got %x, want %x", b, gotMD4, wantMD4)
		}
	}

	return filename, lenVal, sha1Hex
}

// TestFeedAppImageEndToEnd .zsync 全链路：全量包哈希、滚动/MD4 校验和、
// Content-Type、ETag 与 304 条件请求。
func TestFeedAppImageEndToEnd(t *testing.T) {
	r, projSvc, _, ctx := setupCheckTest(t)

	slug := "aidemo"
	p, _, err := projSvc.Create(ctx, service.CreateProjectInput{DefaultLocale: ptr("en"), Slug: &slug})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := projSvc.CreateMatrix(ctx, p.ID, service.MatrixWrite{
		OS: ptr("linux"), Arch: ptr("x86_64"), PackageType: ptr(model.PackageTypeSingleFile),
	}); err != nil {
		t.Fatal(err)
	}
	enableListing(t, projSvc, ctx, p.ID, "appimage", "default", nil)

	payload1 := make([]byte, 5000)
	for i := range payload1 {
		payload1[i] = byte((i * 13) % 256)
	}
	sha256_1, _ := hashutil.SHA256Hex(bytes.NewReader(payload1))

	if _, _, err := projSvc.PutVersion(ctx, p.ID, "1.0.0", service.VersionWriteInput{
		Channel:   "stable",
		Changelog: ptr("aidemo 1.0.0"),
	}); err != nil {
		t.Fatalf("put version 1.0.0: %v", err)
	}
	if _, err := projSvc.UploadArtifact(ctx, fmt.Sprint(p.ID), "1.0.0", "linux", "x86_64", service.UploadArtifactInput{
		Filename:       "aidemo-1.0.0-x86_64.AppImage",
		ExpectedSHA256: sha256_1,
		Size:           int64(len(payload1)),
		ContentType:    "application/x-executable",
	}, bytes.NewReader(payload1)); err != nil {
		t.Fatalf("upload 1.0.0: %v", err)
	}
	if _, err := projSvc.ReadyVersionLine(ctx, p.ID, "1.0.0", "linux", "x86_64"); err != nil {
		t.Fatal(err)
	}
	if _, err := projSvc.PublishVersion(ctx, p.ID, "1.0.0"); err != nil {
		t.Fatal(err)
	}

	payload2 := make([]byte, 7000)
	for i := range payload2 {
		payload2[i] = byte((i * 29) % 256)
	}
	sha256_2, _ := hashutil.SHA256Hex(bytes.NewReader(payload2))

	if _, _, err := projSvc.PutVersion(ctx, p.ID, "1.1.0", service.VersionWriteInput{
		Channel:   "stable",
		Changelog: ptr("aidemo 1.1.0"),
	}); err != nil {
		t.Fatalf("put version 1.1.0: %v", err)
	}
	if _, err := projSvc.UploadArtifact(ctx, fmt.Sprint(p.ID), "1.1.0", "linux", "x86_64", service.UploadArtifactInput{
		Filename:       "aidemo-1.1.0-x86_64.AppImage",
		ExpectedSHA256: sha256_2,
		Size:           int64(len(payload2)),
		ContentType:    "application/x-executable",
	}, bytes.NewReader(payload2)); err != nil {
		t.Fatalf("upload 1.1.0: %v", err)
	}
	if _, err := projSvc.ReadyVersionLine(ctx, p.ID, "1.1.0", "linux", "x86_64"); err != nil {
		t.Fatal(err)
	}
	if _, err := projSvc.PublishVersion(ctx, p.ID, "1.1.0"); err != nil {
		t.Fatal(err)
	}

	url := "/api/v1/projects/aidemo/store/appimage/default/aidemo.AppImage.zsync"
	w := perform(r, http.MethodGet, url)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	if ct := w.Header().Get("Content-Type"); !strings.Contains(ct, "application/x-zsync") {
		t.Fatalf("content-type=%q, want application/x-zsync", ct)
	}
	etag := w.Header().Get("ETag")
	if etag == "" {
		t.Fatal("ETag must be present")
	}
	if cc := w.Header().Get("Cache-Control"); !strings.Contains(cc, "s-maxage=60") {
		t.Fatalf("cache-control=%q, want s-maxage=60", cc)
	}

	// 验证最新发布版本为 1.1.0，长度为 7000，各块校验和正确
	fn, l, _ := validateHTTPZSyncBody(t, w.Body.Bytes(), payload2)
	if fn != "aidemo-1.1.0-linux-x86_64.AppImage" {
		t.Fatalf("filename = %q, want aidemo-1.1.0-linux-x86_64.AppImage", fn)
	}
	if l != 7000 {
		t.Fatalf("length = %d, want 7000", l)
	}

	// ETag 匹配 -> 304 Not Modified
	req304 := httptest.NewRequest(http.MethodGet, url, nil)
	req304.Header.Set("If-None-Match", etag)
	w304 := httptest.NewRecorder()
	r.ServeHTTP(w304, req304)
	if w304.Code != http.StatusNotModified {
		t.Fatalf("status=%d, want 304", w304.Code)
	}
}

// TestFeedAppImageDisabledProtocolReturns404NonZip 开关关闭时纯文本 404（绝不回退 zip）。
func TestFeedAppImageDisabledProtocolReturns404NonZip(t *testing.T) {
	r, projSvc, _, ctx := setupCheckTest(t)

	slug := "aidemo-disabled"
	p, _, err := projSvc.Create(ctx, service.CreateProjectInput{DefaultLocale: ptr("en"), Slug: &slug})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := projSvc.CreateMatrix(ctx, p.ID, service.MatrixWrite{
		OS: ptr("linux"), Arch: ptr("x86_64"), PackageType: ptr(model.PackageTypeSingleFile),
	}); err != nil {
		t.Fatal(err)
	}
	// 不开启 appimage（默认关闭）
	publishSingleFile(t, projSvc, ctx, p.ID, "1.0.0", "stable", "linux", "x86_64", 10, "aidemo-1.0.0-x86_64.AppImage",
		func(in *service.VersionWriteInput) { in.Changelog = ptr("aidemo release") })

	url := "/api/v1/projects/" + slug + "/store/appimage/default/latest.zsync"
	w := perform(r, http.MethodGet, url)
	if w.Code != http.StatusNotFound {
		t.Fatalf("status=%d, want 404", w.Code)
	}
	if strings.Contains(w.Header().Get("Content-Type"), "zip") {
		t.Fatal("must not return zip when protocol disabled")
	}
}

// TestFeedAppImageEmptyCatalogReturns404 可见集为空时返回 404。
func TestFeedAppImageEmptyCatalogReturns404(t *testing.T) {
	r, projSvc, _, ctx := setupCheckTest(t)

	slug := "aidemo-empty"
	p, _, err := projSvc.Create(ctx, service.CreateProjectInput{DefaultLocale: ptr("en"), Slug: &slug})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := projSvc.CreateMatrix(ctx, p.ID, service.MatrixWrite{
		OS: ptr("linux"), Arch: ptr("x86_64"), PackageType: ptr(model.PackageTypeSingleFile),
	}); err != nil {
		t.Fatal(err)
	}
	enableListing(t, projSvc, ctx, p.ID, "appimage", "default", nil)

	// 未发布任何版本 -> 可见集为空
	url := "/api/v1/projects/" + slug + "/store/appimage/default/latest.zsync"
	w := perform(r, http.MethodGet, url)
	if w.Code != http.StatusNotFound {
		t.Fatalf("status=%d, want 404 on empty visible catalog", w.Code)
	}
}

// TestFeedAppImageTokenAuth 验收项 3：require_client_token / store_token 开启时无 Token -> 401 UNAUTHORIZED，带 Token -> 200。
func TestFeedAppImageTokenAuth(t *testing.T) {
	r, projSvc, _, ctx := setupCheckTest(t)

	secret := "appimage-secret-token"
	slug := "aidemo-token"
	p, _, err := projSvc.Create(ctx, service.CreateProjectInput{DefaultLocale: ptr("en"), Slug: &slug, StoreToken: &secret})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := projSvc.CreateMatrix(ctx, p.ID, service.MatrixWrite{
		OS: ptr("linux"), Arch: ptr("x86_64"), PackageType: ptr(model.PackageTypeSingleFile),
	}); err != nil {
		t.Fatal(err)
	}
	enableListing(t, projSvc, ctx, p.ID, "appimage", "default", nil)

	publishSingleFile(t, projSvc, ctx, p.ID, "1.0.0", "stable", "linux", "x86_64", 10, "aidemo-1.0.0-x86_64.AppImage",
		func(in *service.VersionWriteInput) { in.Changelog = ptr("aidemo release") })

	url := "/api/v1/projects/" + slug + "/store/appimage/default/latest.zsync"

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

// TestFeedAppImageUnknownPathReturns404 非 .zsync 结尾的路径返回 404。
func TestFeedAppImageUnknownPathReturns404(t *testing.T) {
	r, projSvc, _, ctx := setupCheckTest(t)

	slug := "aidemo-badpath"
	p, _, err := projSvc.Create(ctx, service.CreateProjectInput{DefaultLocale: ptr("en"), Slug: &slug})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := projSvc.CreateMatrix(ctx, p.ID, service.MatrixWrite{
		OS: ptr("linux"), Arch: ptr("x86_64"), PackageType: ptr(model.PackageTypeSingleFile),
	}); err != nil {
		t.Fatal(err)
	}
	enableListing(t, projSvc, ctx, p.ID, "appimage", "default", nil)
	publishSingleFile(t, projSvc, ctx, p.ID, "1.0.0", "stable", "linux", "x86_64", 10, "aidemo-1.0.0-x86_64.AppImage",
		func(in *service.VersionWriteInput) { in.Changelog = ptr("aidemo release") })

	badPaths := []string{
		"latest.json",
		"appimage",
		"RELEASES",
		"manifest.xml",
		"foo.zip",
	}

	for _, pth := range badPaths {
		url := "/api/v1/projects/" + slug + "/store/appimage/default/" + pth
		w := perform(r, http.MethodGet, url)
		if w.Code != http.StatusNotFound {
			t.Fatalf("path %q status=%d, want 404", pth, w.Code)
		}
	}
}

// TestFeedAppImageIdentifiersConfiguration 验证配置 identifiers 时不阻断 feed 正常服务。
func TestFeedAppImageIdentifiersConfiguration(t *testing.T) {
	r, projSvc, _, ctx := setupCheckTest(t)

	slug := "aidemo-ident"
	p, _, err := projSvc.Create(ctx, service.CreateProjectInput{DefaultLocale: ptr("en"), Slug: &slug})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := projSvc.CreateMatrix(ctx, p.ID, service.MatrixWrite{
		OS: ptr("linux"), Arch: ptr("x86_64"), PackageType: ptr(model.PackageTypeSingleFile),
	}); err != nil {
		t.Fatal(err)
	}
	enableListing(t, projSvc, ctx, p.ID, "appimage", "default", map[string]string{
		"app_name": "MyLinuxApp",
	})
	publishSingleFile(t, projSvc, ctx, p.ID, "1.0.0", "stable", "linux", "x86_64", 10, "aidemo-1.0.0-x86_64.AppImage",
		func(in *service.VersionWriteInput) { in.Changelog = ptr("aidemo release") })

	url := "/api/v1/projects/" + slug + "/store/appimage/default/latest.zsync"
	w := perform(r, http.MethodGet, url)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d, want 200 with identifiers", w.Code)
	}
}
