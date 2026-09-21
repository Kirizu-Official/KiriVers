package client

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Kirizu-Official/KiriVers/internal/model"
	"github.com/Kirizu-Official/KiriVers/internal/service"
	"github.com/Kirizu-Official/KiriVers/pkg/hashutil"
	"github.com/Kirizu-Official/KiriVers/pkg/signature"
)

// ---------- Tauri updater feed HTTP 集成测试（§9，C18-1..C18-7） ----------
//
// 复用 check_test.go 的 setupCheckTest / publishSingleFile / perform / ptr。

// enableTauriPatch 返回启用 tauri 协议的 Patch 输入。
// feedTauriUpdateDoc 是动态端点单平台对象的解析目标（官方字段集子集）。
type feedTauriUpdateDoc struct {
	Version   string `json:"version"`
	Notes     string `json:"notes"`
	PubDate   string `json:"pub_date"`
	Signature string `json:"signature"`
	URL       string `json:"url"`
}

// feedTauriStaticDoc 是静态 latest.json 的解析目标（官方字段集子集）。
type feedTauriStaticDoc struct {
	Version   string `json:"version"`
	Notes     string `json:"notes"`
	PubDate   string `json:"pub_date"`
	Platforms map[string]struct {
		Signature string `json:"signature"`
		URL       string `json:"url"`
	} `json:"platforms"`
}

// parseFeedTauriUpdateDoc 解析动态端点 JSON 并断言官方字段齐全（验收项 1：
// url/signature/version 等官方字段）。
func parseFeedTauriUpdateDoc(t *testing.T, body []byte) *feedTauriUpdateDoc {
	t.Helper()
	var doc feedTauriUpdateDoc
	if err := json.Unmarshal(body, &doc); err != nil {
		t.Fatalf("unmarshal tauri update doc: %v\n%s", err, body)
	}
	if doc.Version == "" || doc.URL == "" || doc.Signature == "" {
		t.Fatalf("incomplete tauri update doc: %+v\n%s", doc, body)
	}
	return &doc
}

// assertFeedMinisignContainer 断言 Tauri 签名是可解析的 minisign 容器
// （验收项 3：base64 解码、"Ed" 前缀、74 字节总长）并返回内嵌 64 字节签名。
func assertFeedMinisignContainer(t *testing.T, sig string) []byte {
	t.Helper()
	lines := strings.Split(sig, "\n")
	if len(lines) != 3 || lines[2] != "" || !strings.HasPrefix(lines[0], "untrusted comment:") {
		t.Fatalf("signature must be a minisign container (comment + base64 lines), got %q", sig)
	}
	block, err := base64.StdEncoding.DecodeString(lines[1])
	if err != nil {
		t.Fatalf("minisign payload not base64: %v", err)
	}
	if len(block) != 74 {
		t.Fatalf("minisign block = %d bytes, want 74", len(block))
	}
	if string(block[:2]) != "Ed" {
		t.Fatalf("minisign block prefix = %q, want \"Ed\"", block[:2])
	}
	return block[10:]
}

// TestFeedTauriEndToEnd 全链路：动态端点官方字段（验收 1）、minisign 容器
// 可解析且对安装器文件字节可验、与原生 check signature 字节不等（验收 3）、
// 已是最新 → 204（验收 2）、静态 latest.json 与动态一致（C18-1）、
// darwin→macos 别名映射。
func TestFeedTauriEndToEnd(t *testing.T) {
	r, projSvc, _, ctx := setupCheckTest(t)

	privPEM, pubPEM := genFeedEd25519PEM(t)
	slug := "tdemo"
	p, _, err := projSvc.Create(ctx, service.CreateProjectInput{DefaultLocale: ptr("en"), Slug:              &slug,
		SigningAlgo:       ptr(model.SigningAlgoEd25519),
		SigningPrivateKey: &privPEM,
		SigningPublicKey:  &pubPEM,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := projSvc.CreateMatrix(ctx, p.ID, service.MatrixWrite{
		OS: ptr("macos"), Arch: ptr("x86_64"), PackageType: ptr(model.PackageTypeSingleFile),
	}); err != nil {
		t.Fatal(err)
	}
	enableListing(t, projSvc, ctx, p.ID, "tauri", "default", nil)
	// publishSingleFile 的产物字节是确定性的（"x" × size），可直接重建验证。
	payload := bytes.Repeat([]byte("x"), 64)
	publishSingleFile(t, projSvc, ctx, p.ID, "1.0.0", "stable", "macos", "x86_64", 32, "tdemo-1.0.0-macos-x86_64.dmg",
		func(in *service.VersionWriteInput) { in.Changelog = ptr("t 1.0.0") })
	publishSingleFile(t, projSvc, ctx, p.ID, "1.1.0", "stable", "macos", "x86_64", 64, "tdemo-1.1.0-macos-x86_64.dmg",
		func(in *service.VersionWriteInput) { in.Changelog = ptr("t 1.1.0") })

	// 动态端点（darwin 别名 → macos，C18-1）。
	url := "/api/v1/projects/tdemo/store/tauri/default/darwin/x86_64/1.0.0"
	w := perform(r, http.MethodGet, url)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	if ct := w.Header().Get("Content-Type"); !strings.Contains(ct, "json") {
		t.Fatalf("content-type=%q", ct)
	}
	doc := parseFeedTauriUpdateDoc(t, w.Body.Bytes())
	if doc.Version != "1.1.0" {
		t.Fatalf("version=%q", doc.Version)
	}
	if !strings.Contains(doc.Notes, "t 1.1.0") {
		t.Fatalf("notes=%q", doc.Notes)
	}
	if !strings.HasPrefix(doc.URL, "/api/v1/projects/tdemo/packages/") || strings.Contains(doc.URL, "tdemo-1.1.0") {
		t.Fatalf("url=%q want hash package URL", doc.URL)
	}
	// minisign 容器可解析（验收项 3），内嵌签名对安装器文件字节可验。
	raw := assertFeedMinisignContainer(t, doc.Signature)
	if err := signature.VerifyPayload(signature.AlgoEd25519, pubPEM, string(payload), base64.StdEncoding.EncodeToString(raw)); err != nil {
		t.Fatalf("tauri signature must verify over installer file bytes: %v", err)
	}
	// url 真实可下载。
	dl := perform(r, http.MethodGet, doc.URL)
	if dl.Code != http.StatusOK {
		t.Fatalf("url download status=%d", dl.Code)
	}

	// 验收项 3：原生 check signature 字符串不得原样充当 Tauri signature。
	cw := performCheck(r, "/api/v1/projects/tdemo/update/check?current_version=1.0.0&os=macos&arch=x86_64")
	if cw.Code != http.StatusOK {
		t.Fatalf("native check status=%d body=%s", cw.Code, cw.Body.String())
	}
	var native struct {
		Signature string `json:"signature"`
	}
	if err := json.Unmarshal(cw.Body.Bytes(), &native); err != nil {
		t.Fatalf("unmarshal native check: %v", err)
	}
	if native.Signature == "" {
		t.Fatal("native check must emit signature for ed25519 project")
	}
	if native.Signature == doc.Signature {
		t.Fatal("tauri signature must never reuse the native check signature bytes")
	}

	// 验收项 2：已是最新 → 204 无 body。
	w204 := perform(r, http.MethodGet, "/api/v1/projects/tdemo/store/tauri/default/darwin/x86_64/1.1.0")
	if w204.Code != http.StatusNoContent {
		t.Fatalf("already-latest status=%d want 204", w204.Code)
	}
	if w204.Body.Len() != 0 {
		t.Fatalf("204 must have no body, got %q", w204.Body.String())
	}

	// 静态 latest.json：与动态同 os/arch 的 version/url 一致（C18-1）。
	sw := perform(r, http.MethodGet, "/api/v1/projects/tdemo/store/tauri/default/latest.json?os=macos&arch=x86_64")
	if sw.Code != http.StatusOK {
		t.Fatalf("static status=%d body=%s", sw.Code, sw.Body.String())
	}
	var sdoc feedTauriStaticDoc
	if err := json.Unmarshal(sw.Body.Bytes(), &sdoc); err != nil {
		t.Fatalf("unmarshal static doc: %v\n%s", err, sw.Body)
	}
	if sdoc.Version != "1.1.0" {
		t.Fatalf("static version=%q", sdoc.Version)
	}
	entry, ok := sdoc.Platforms["darwin-x86_64"]
	if !ok {
		t.Fatalf("platforms must contain darwin-x86_64: %v", sdoc.Platforms)
	}
	if entry.URL != doc.URL {
		t.Fatalf("static/dynamic url mismatch: %q vs %q", entry.URL, doc.URL)
	}
	if entry.Signature == "" {
		t.Fatal("static platform entry must carry signature for ed25519 project")
	}

	// 未知 target → 404 且不冒充 zip。
	w404 := perform(r, http.MethodGet, "/api/v1/projects/tdemo/store/tauri/default/freebsd/x86_64/1.0.0")
	if w404.Code != http.StatusNotFound {
		t.Fatalf("unknown target status=%d want 404", w404.Code)
	}
	if strings.HasPrefix(w404.Body.String(), "PK") {
		t.Fatal("404 body must not be a zip archive")
	}
}

// TestFeedTauriToken401 store_token 开启时缺 Token → 401 UNAUTHORIZED，
// 携带合法 X-Project-Token → 200（C18-6，验收项 4）。
func TestFeedTauriToken401(t *testing.T) {
	r, projSvc, _, ctx := setupCheckTest(t)

	secret := "tauri-feed-secret"
	slug := "ttok"
	p, _, err := projSvc.Create(ctx, service.CreateProjectInput{DefaultLocale: ptr("en"), Slug: &slug, StoreToken: &secret})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := projSvc.CreateMatrix(ctx, p.ID, service.MatrixWrite{
		OS: ptr("macos"), Arch: ptr("x86_64"), PackageType: ptr(model.PackageTypeSingleFile),
	}); err != nil {
		t.Fatal(err)
	}
	enableListing(t, projSvc, ctx, p.ID, "tauri", "default", nil)
	publishSingleFile(t, projSvc, ctx, p.ID, "1.0.0", "stable", "macos", "x86_64", 10, "ttok-1.0.0-macos-x86_64.dmg",
		func(in *service.VersionWriteInput) { in.Changelog = ptr("ttok release") })
	publishSingleFile(t, projSvc, ctx, p.ID, "1.1.0", "stable", "macos", "x86_64", 10, "ttok-1.1.0-macos-x86_64.dmg",
		func(in *service.VersionWriteInput) { in.Changelog = ptr("ttok 1.1.0") })

	url := "/api/v1/projects/ttok/store/tauri/default/darwin/x86_64/1.0.0"
	w := perform(r, http.MethodGet, url)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("no token status=%d want 401", w.Code)
	}
	if !strings.Contains(w.Body.String(), "UNAUTHORIZED") {
		t.Fatalf("401 body=%s", w.Body.String())
	}
	req := httptest.NewRequest(http.MethodGet, url, nil)
	req.Header.Set("X-Project-Token", secret)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("valid token status=%d body=%s", rec.Code, rec.Body.String())
	}
}

// TestFeedTauriDisabled404NonZip tauri 协议未开启 → 纯文本 404，不回退
// 通用 zip（C18-5 / §9.2）。
func TestFeedTauriDisabled404NonZip(t *testing.T) {
	r, projSvc, _, ctx := setupCheckTest(t)

	slug := "tclosed"
	p, _, err := projSvc.Create(ctx, service.CreateProjectInput{DefaultLocale: ptr("en"), Slug: &slug})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := projSvc.CreateMatrix(ctx, p.ID, service.MatrixWrite{
		OS: ptr("macos"), Arch: ptr("x86_64"), PackageType: ptr(model.PackageTypeSingleFile),
	}); err != nil {
		t.Fatal(err)
	}
	publishSingleFile(t, projSvc, ctx, p.ID, "1.0.0", "stable", "macos", "x86_64", 10, "tclosed-1.0.0-macos-x86_64.dmg",
		func(in *service.VersionWriteInput) { in.Changelog = ptr("closed release") })

	w := perform(r, http.MethodGet, "/api/v1/projects/tclosed/store/tauri/default/darwin/x86_64/1.0.0")
	if w.Code != http.StatusNotFound {
		t.Fatalf("disabled protocol status=%d want 404", w.Code)
	}
	if strings.HasPrefix(w.Body.String(), "PK") {
		t.Fatal("404 body must not be a zip archive")
	}
}

// TestFeedTauriGrayProjection 灰度 <100% 的非强制版本对匿名动态端点无更新
// （204），静态回退到下一个可见版本（C18-5，§4.4 匿名口径）。
func TestFeedTauriGrayProjection(t *testing.T) {
	r, projSvc, _, ctx := setupCheckTest(t)

	slug := "tgray"
	p, _, err := projSvc.Create(ctx, service.CreateProjectInput{DefaultLocale: ptr("en"), Slug: &slug})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := projSvc.CreateMatrix(ctx, p.ID, service.MatrixWrite{
		OS: ptr("macos"), Arch: ptr("x86_64"), PackageType: ptr(model.PackageTypeSingleFile),
	}); err != nil {
		t.Fatal(err)
	}
	enableListing(t, projSvc, ctx, p.ID, "tauri", "default", nil)
	seedClients(t, projSvc, p, 2)
	publishSingleFile(t, projSvc, ctx, p.ID, "1.0.0", "stable", "macos", "x86_64", 10, "tgray-1.0.0-macos-x86_64.dmg",
		func(in *service.VersionWriteInput) { in.Changelog = ptr("g 1.0.0") })
	publishSingleFile(t, projSvc, ctx, p.ID, "1.1.0", "stable", "macos", "x86_64", 10, "tgray-1.1.0-macos-x86_64.dmg",
		func(in *service.VersionWriteInput) { in.GrayStartPercent = ptr(50); in.Changelog = ptr("g 1.1.0") })

	if w := perform(r, http.MethodGet, "/api/v1/projects/tgray/store/tauri/default/darwin/x86_64/1.0.0"); w.Code != http.StatusNoContent {
		t.Fatalf("gray target must be invisible, status=%d body=%s", w.Code, w.Body.String())
	}
	w := perform(r, http.MethodGet, "/api/v1/projects/tgray/store/tauri/default/latest.json?os=macos&arch=x86_64")
	if w.Code != http.StatusOK {
		t.Fatalf("static status=%d", w.Code)
	}
	var sdoc feedTauriStaticDoc
	if err := json.Unmarshal(w.Body.Bytes(), &sdoc); err != nil {
		t.Fatal(err)
	}
	if sdoc.Version != "1.0.0" {
		t.Fatalf("static must fall through to 1.0.0, got %q", sdoc.Version)
	}
}

// TestFeedTauriDefaultHWVariant C18-7：目标线只有非默认 hw 变体时动态端点
// 视为该平台无包 → 204，绝不把非默认 hw 包当该平台的更新下发。
func TestFeedTauriDefaultHWVariant(t *testing.T) {
	r, projSvc, _, ctx := setupCheckTest(t)

	slug := "thw"
	p, _, err := projSvc.Create(ctx, service.CreateProjectInput{DefaultLocale: ptr("en"), Slug: &slug})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := projSvc.CreateMatrix(ctx, p.ID, service.MatrixWrite{
		OS: ptr("macos"), Arch: ptr("x86_64"), PackageType: ptr(model.PackageTypeSingleFile),
	}); err != nil {
		t.Fatal(err)
	}
	enableListing(t, projSvc, ctx, p.ID, "tauri", "default", nil)
	publishSingleFile(t, projSvc, ctx, p.ID, "1.0.0", "stable", "macos", "x86_64", 10, "thw-1.0.0-macos-x86_64.dmg",
		func(in *service.VersionWriteInput) { in.Changelog = ptr("thw 1.0.0") })

	// 1.1.0：只上传 revb 非默认变体（发布前装配，ARTIFACT_IMMUTABLE）。
	if _, _, err := projSvc.PutVersion(ctx, p.ID, "1.1.0", service.VersionWriteInput{
		Channel:   "stable",
		Changelog: ptr("thw 1.1.0"),
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := projSvc.CreateHwRev(ctx, p.ID, service.HwRevWrite{Slug: ptr("revb"), Rank: ptr(10)}); err != nil {
		t.Fatal(err)
	}
	payload := bytes.Repeat([]byte("x"), 20)
	sha, _ := hashutil.SHA256Hex(bytes.NewReader(payload))
	if _, err := projSvc.UploadArtifact(ctx, fmt.Sprint(p.ID), "1.1.0", "macos", "x86_64", service.UploadArtifactInput{
		Filename:       "thw-1.1.0-macos-x86_64-revb.dmg",
		ExpectedSHA256: sha,
		Size:           int64(len(payload)),
		HwRev:          ptr("revb"),
		ContentType:    "application/x-apple-diskimage",
	}, bytes.NewReader(payload)); err != nil {
		t.Fatalf("upload hw variant: %v", err)
	}
	if _, err := projSvc.ReadyVersionLine(ctx, p.ID, "1.1.0", "macos", "x86_64"); err != nil {
		t.Fatal(err)
	}
	if _, err := projSvc.PublishVersion(ctx, p.ID, "1.1.0"); err != nil {
		t.Fatal(err)
	}

	w := perform(r, http.MethodGet, "/api/v1/projects/thw/store/tauri/default/darwin/x86_64/1.0.0")
	if w.Code != http.StatusNoContent {
		t.Fatalf("non-default-only target must be 204, got %d body=%s", w.Code, w.Body.String())
	}
}

// TestFeedTauriStaticNoQuery 无 os/arch query 的静态 latest.json（真实
// Tauri 客户端形态）：platforms 覆盖矩阵中的平台键。
func TestFeedTauriStaticNoQuery(t *testing.T) {
	r, projSvc, _, ctx := setupCheckTest(t)

	slug := "tnoq"
	p, _, err := projSvc.Create(ctx, service.CreateProjectInput{DefaultLocale: ptr("en"), Slug: &slug})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := projSvc.CreateMatrix(ctx, p.ID, service.MatrixWrite{
		OS: ptr("macos"), Arch: ptr("x86_64"), PackageType: ptr(model.PackageTypeSingleFile),
	}); err != nil {
		t.Fatal(err)
	}
	enableListing(t, projSvc, ctx, p.ID, "tauri", "default", nil)
	publishSingleFile(t, projSvc, ctx, p.ID, "1.0.0", "stable", "macos", "x86_64", 10, "tnoq-1.0.0-macos-x86_64.dmg",
		func(in *service.VersionWriteInput) { in.Changelog = ptr("noq release") })

	w := perform(r, http.MethodGet, "/api/v1/projects/tnoq/store/tauri/default/latest.json")
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	var sdoc feedTauriStaticDoc
	if err := json.Unmarshal(w.Body.Bytes(), &sdoc); err != nil {
		t.Fatal(err)
	}
	if sdoc.Version != "1.0.0" {
		t.Fatalf("version=%q", sdoc.Version)
	}
	if e, ok := sdoc.Platforms["darwin-x86_64"]; !ok || e.URL == "" {
		t.Fatalf("platforms must contain darwin-x86_64: %v", sdoc.Platforms)
	}
}
