package client

import (
	"bytes"
	"context"
	"crypto/sha512"
	"encoding/base64"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/Kirizu-Official/KiriVers/internal/model"
	"github.com/Kirizu-Official/KiriVers/internal/repository"
	"github.com/Kirizu-Official/KiriVers/internal/service"
	"github.com/Kirizu-Official/KiriVers/internal/service/update"
	"github.com/Kirizu-Official/KiriVers/internal/storage"
	"github.com/Kirizu-Official/KiriVers/pkg/hashutil"
	"github.com/Kirizu-Official/KiriVers/pkg/urlsign"
)

// ---------- electron-updater generic feed HTTP 集成测试（§9 / §7.2，C17-1..C17-7） ----------
//
// 复用 check_test.go 的 setupCheckTest / publishSingleFile / perform / ptr。

// enableElectronPatch 返回启用 electron 协议的 Patch 输入。
// feedElectronDoc 是 latest*.yml 的最小解析目标（electron-updater generic
// 文档字段集的验证用子集）。
type feedElectronDoc struct {
	Version string
	Path    string
	SHA512  string
	Size    string
	FileURL string
}

// parseFeedElectronYML 行式提取 latest*.yml 的关键字段（渲染排版固定）。
func parseFeedElectronYML(t *testing.T, body []byte) *feedElectronDoc {
	t.Helper()
	doc := &feedElectronDoc{}
	for _, line := range strings.Split(string(body), "\n") {
		switch {
		case strings.HasPrefix(line, "version: "):
			doc.Version = strings.Trim(strings.TrimPrefix(line, "version: "), `"`)
		case strings.HasPrefix(line, "path: "):
			doc.Path = strings.Trim(strings.TrimPrefix(line, "path: "), `"`)
		case strings.HasPrefix(line, "sha512: "):
			doc.SHA512 = strings.Trim(strings.TrimPrefix(line, "sha512: "), `"`)
		case strings.HasPrefix(line, "size: "):
			doc.Size = strings.TrimPrefix(line, "size: ")
		case strings.HasPrefix(line, "  - url: "):
			doc.FileURL = strings.Trim(strings.TrimPrefix(line, "  - url: "), `"`)
		}
	}
	return doc
}

// TestFeedElectronEndToEnd latest.yml 全链路：字段集、默认变体 path（验收 4）、
// sha512 base64 语义（对上传字节独立验证，§5.8）、文件名→os 映射与未知文件名
// 404（C17-1）、ETag/304。
func TestFeedElectronEndToEnd(t *testing.T) {
	r, projSvc, _, ctx := setupCheckTest(t)

	slug := "edemo"
	p, _, err := projSvc.Create(ctx, service.CreateProjectInput{DefaultLocale: ptr("en"), Slug: &slug})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := projSvc.CreateMatrix(ctx, p.ID, service.MatrixWrite{
		OS: ptr("windows"), Arch: ptr("x86_64"), PackageType: ptr(model.PackageTypeSingleFile),
	}); err != nil {
		t.Fatal(err)
	}
	enableListing(t, projSvc, ctx, p.ID, "electron", "default", nil)

	payload := bytes.Repeat([]byte("e"), 123)
	sha, _ := hashutil.SHA256Hex(bytes.NewReader(payload))
	if _, _, err := projSvc.PutVersion(ctx, p.ID, "1.1.0", service.VersionWriteInput{
		Channel:        "stable",
		Changelog:      ptr("edemo release"),
		VersionInteger: ptr(int64(11)),
	}); err != nil {
		t.Fatalf("put version: %v", err)
	}
	if _, err := projSvc.UploadArtifact(ctx, fmt.Sprint(p.ID), "1.1.0", "windows", "x86_64", service.UploadArtifactInput{
		Filename:       "edemo-1.1.0-windows-x86_64.exe",
		ExpectedSHA256: sha,
		Size:           int64(len(payload)),
		ContentType:    "application/x-msdownload",
	}, bytes.NewReader(payload)); err != nil {
		t.Fatalf("upload: %v", err)
	}
	if _, err := projSvc.ReadyVersionLine(ctx, p.ID, "1.1.0", "windows", "x86_64"); err != nil {
		t.Fatal(err)
	}
	if _, err := projSvc.PublishVersion(ctx, p.ID, "1.1.0"); err != nil {
		t.Fatal(err)
	}

	url := "/api/v1/projects/edemo/store/electron/default/latest.yml"
	w := perform(r, http.MethodGet, url)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	if ct := w.Header().Get("Content-Type"); !strings.Contains(ct, "yaml") {
		t.Fatalf("content-type=%q", ct)
	}
	etag := w.Header().Get("ETag")
	if etag == "" {
		t.Fatal("missing ETag")
	}

	doc := parseFeedElectronYML(t, w.Body.Bytes())
	if doc.Version != "1.1.0" {
		t.Fatalf("version=%q", doc.Version)
	}
	if doc.Path != sha+".exe" {
		t.Fatalf("path=%q want %s.exe", doc.Path, sha)
	}
	if doc.Size != "123" {
		t.Fatalf("size=%q", doc.Size)
	}
	// sha512 语义（§5.8）：base64(原始 512-bit 摘要)，对上传字节独立验证。
	digest := sha512.Sum512(payload)
	if want := base64.StdEncoding.EncodeToString(digest[:]); doc.SHA512 != want {
		t.Fatalf("sha512=%q want %q", doc.SHA512, want)
	}
	// files[].url 真实可下载且字节一致。
	if doc.FileURL == "" {
		t.Fatal("files[].url missing")
	}
	dl := perform(r, http.MethodGet, doc.FileURL)
	if dl.Code != http.StatusOK {
		t.Fatalf("files url download status=%d", dl.Code)
	}
	if !bytes.Equal(dl.Body.Bytes(), payload) {
		t.Fatal("files url download bytes must match the uploaded artifact")
	}

	// If-None-Match → 304（框架层统一）。
	req := httptest.NewRequest(http.MethodGet, url, nil)
	req.Header.Set("If-None-Match", etag)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotModified {
		t.Fatalf("If-None-Match status=%d want 304", rec.Code)
	}

	// 文件名→os 映射（C17-1）：windows-only 项目上 latest-mac.yml 无可见集 → 404。
	w2 := perform(r, http.MethodGet, "/api/v1/projects/edemo/store/electron/default/latest-mac.yml")
	if w2.Code != http.StatusNotFound {
		t.Fatalf("latest-mac.yml on windows-only project status=%d want 404", w2.Code)
	}
	// 未知文件名 → 404，且不冒充 zip。
	w3 := perform(r, http.MethodGet, "/api/v1/projects/edemo/store/electron/default/latest-beta.yml")
	if w3.Code != http.StatusNotFound {
		t.Fatalf("unknown filename status=%d want 404", w3.Code)
	}
	if strings.HasPrefix(w3.Body.String(), "PK") {
		t.Fatal("404 body must not be a zip archive")
	}
}

// TestFeedElectronBlockmapDownloadByteIdentical blockmap 产物（kind=file，
// 文件名 = 全量包 + ".blockmap"）经既有 /packages/{sha256} 路由原样下载、
// 字节不变（验收项 3，C17-4 / §7.2）；yml 中不出现 blockmap 字段。
func TestFeedElectronBlockmapDownloadByteIdentical(t *testing.T) {
	r, projSvc, store, ctx := setupCheckTest(t)

	slug := "ebm"
	p, _, err := projSvc.Create(ctx, service.CreateProjectInput{DefaultLocale: ptr("en"), Slug: &slug})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := projSvc.CreateMatrix(ctx, p.ID, service.MatrixWrite{
		OS: ptr("windows"), Arch: ptr("x86_64"), PackageType: ptr(model.PackageTypeSingleFile),
	}); err != nil {
		t.Fatal(err)
	}
	enableListing(t, projSvc, ctx, p.ID, "electron", "default", nil)
	v := publishSingleFile(t, projSvc, ctx, p.ID, "1.0.0", "stable", "windows", "x86_64", 64, "ebm-1.0.0-windows-x86_64.exe",
		func(in *service.VersionWriteInput) { in.Changelog = ptr("ebm release") })

	fullName := "ebm-1.0.0-windows-x86_64.exe"
	blockmapName := fullName + ".blockmap"
	blockmapBytes := []byte(`{"blockMap":[{"offset":0,"size":64}]}`)

	// 直插 blockmap 产物行（kind=file；上传 API 的稳定文件名规则由多文件
	// 任务处理，本测试关注 feed/下载语义）。
	full, err := store.GetArtifactByFileName(ctx, p.ID, fullName)
	if err != nil || full == nil {
		t.Fatalf("load full artifact: %v", err)
	}
	bmID := uuid.New()
	bmSHA, _ := hashutil.SHA256Hex(bytes.NewReader(blockmapBytes))
	bmKey := storage.ArtifactObjectKey(p.Slug, bmSHA)
	if err := projSvc.Storage().Put(ctx, bmKey, bytes.NewReader(blockmapBytes), int64(len(blockmapBytes)), "application/octet-stream"); err != nil {
		t.Fatalf("put blockmap bytes: %v", err)
	}
	if err := store.CreateArtifact(ctx, &model.Artifact{
		ID: bmID, ProjectID: p.ID, VersionID: v.ID, VersionLineID: full.VersionLineID,
		Kind: model.ArtifactKindFile, FileName: blockmapName, StorageKey: bmKey,
		Size: int64(len(blockmapBytes)), SHA256: bmSHA, ContentType: "application/octet-stream",
	}); err != nil {
		t.Fatalf("create blockmap artifact: %v", err)
	}

	// yml 本身不携带任何 blockmap 字段（electron-updater 按 path+".blockmap"
	// 约定自动探测，C17-4）。
	w := perform(r, http.MethodGet, "/api/v1/projects/ebm/store/electron/default/latest.yml")
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	if strings.Contains(w.Body.String(), "blockmap") {
		t.Fatalf("yml must not contain blockmap fields:\n%s", w.Body.String())
	}
	// /packages/ 原样下载，字节不变（§7.2）。
	dl := perform(r, http.MethodGet, "/api/v1/projects/ebm/packages/"+bmSHA)
	if dl.Code != http.StatusOK {
		t.Fatalf("blockmap download status=%d body=%s", dl.Code, dl.Body.String())
	}
	if !bytes.Equal(dl.Body.Bytes(), blockmapBytes) {
		t.Fatal("blockmap download bytes must be identical to the hosted object")
	}
}

// TestFeedElectronToken401 store_token 开启时缺 Token → 401 UNAUTHORIZED，
// 携带合法 X-Project-Token → 200（验收项 4，C17-6）。
func TestFeedElectronToken401(t *testing.T) {
	r, projSvc, _, ctx := setupCheckTest(t)

	secret := "electron-feed-secret"
	slug := "etok"
	p, _, err := projSvc.Create(ctx, service.CreateProjectInput{DefaultLocale: ptr("en"), Slug: &slug, StoreToken: &secret})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := projSvc.CreateMatrix(ctx, p.ID, service.MatrixWrite{
		OS: ptr("windows"), Arch: ptr("x86_64"), PackageType: ptr(model.PackageTypeSingleFile),
	}); err != nil {
		t.Fatal(err)
	}
	enableListing(t, projSvc, ctx, p.ID, "electron", "default", nil)
	publishSingleFile(t, projSvc, ctx, p.ID, "1.0.0", "stable", "windows", "x86_64", 10, "etok-1.0.0-windows-x86_64.exe",
		func(in *service.VersionWriteInput) { in.Changelog = ptr("etok release") })

	url := "/api/v1/projects/etok/store/electron/default/latest.yml"
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

// TestFeedElectronDisabled404NonZip electron 协议未开启 → 纯文本 404，不回退
// 通用 zip（C17-5 / §9.2）。
func TestFeedElectronDisabled404NonZip(t *testing.T) {
	r, projSvc, _, ctx := setupCheckTest(t)

	slug := "eclosed"
	p, _, err := projSvc.Create(ctx, service.CreateProjectInput{DefaultLocale: ptr("en"), Slug: &slug})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := projSvc.CreateMatrix(ctx, p.ID, service.MatrixWrite{
		OS: ptr("windows"), Arch: ptr("x86_64"), PackageType: ptr(model.PackageTypeSingleFile),
	}); err != nil {
		t.Fatal(err)
	}
	publishSingleFile(t, projSvc, ctx, p.ID, "1.0.0", "stable", "windows", "x86_64", 10, "eclosed-1.0.0-windows-x86_64.exe",
		func(in *service.VersionWriteInput) { in.Changelog = ptr("closed release") })

	w := perform(r, http.MethodGet, "/api/v1/projects/eclosed/store/electron/default/latest.yml")
	if w.Code != http.StatusNotFound {
		t.Fatalf("disabled protocol status=%d want 404", w.Code)
	}
	if strings.HasPrefix(w.Body.String(), "PK") {
		t.Fatal("404 body must not be a zip archive")
	}
}

// TestFeedElectronPrivateSignedURLDownload 私有存储项目：files[].url 带真实
// 签名器（urlsign.NewSigner）产生的 ?exp=&sig=，且签名 URL 可真实下载
// （§13.7 / C17-3 / 验收项 4）。私有项目缓存头恒 private, no-store，且 304
// 短路被跳过（签名 URL 不被 304 钉死）。
func TestFeedElectronPrivateSignedURLDownload(t *testing.T) {
	gin.SetMode(gin.TestMode)
	store := repository.NewMemoryProjectStore()
	backend, err := storage.NewLocalFS(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	projSvc := service.NewProjectService(store, backend)
	signer := urlsign.NewSigner("electron-test-secret", 0)
	r := gin.New()
	Register(r.Group("/api/v1"), projSvc,
		update.NewService(repository.NewMemoryUpdateCatalog(store)), nil, nil, signer, nil)
	ctx := context.Background()

	slug := "epriv"
	p, _, err := projSvc.Create(ctx, service.CreateProjectInput{DefaultLocale: ptr("en"), Slug: &slug})
	if err != nil {
		t.Fatal(err)
	}
	markProjectPrivate(t, store, ctx, p)
	if _, err := projSvc.CreateMatrix(ctx, p.ID, service.MatrixWrite{
		OS: ptr("windows"), Arch: ptr("x86_64"), PackageType: ptr(model.PackageTypeSingleFile),
	}); err != nil {
		t.Fatal(err)
	}
	enableListing(t, projSvc, ctx, p.ID, "electron", "default", nil)
	publishSingleFile(t, projSvc, ctx, p.ID, "1.0.0", "stable", "windows", "x86_64", 48, "epriv-1.0.0-windows-x86_64.exe",
		func(in *service.VersionWriteInput) { in.Changelog = ptr("epriv release") })

	url := "/api/v1/projects/epriv/store/electron/default/latest.yml"
	w := perform(r, http.MethodGet, url)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	if cc := w.Header().Get("Cache-Control"); cc != "private, no-store" {
		t.Fatalf("private project cache-control=%q", cc)
	}

	doc := parseFeedElectronYML(t, w.Body.Bytes())
	if !strings.Contains(doc.FileURL, "?exp=") || !strings.Contains(doc.FileURL, "&sig=") {
		t.Fatalf("private files url must be signed: %q", doc.FileURL)
	}
	// 签名 URL 真实可下载（私有 /packages/ 有签名闸，无签名 → 403）。
	dl := perform(r, http.MethodGet, doc.FileURL)
	if dl.Code != http.StatusOK {
		t.Fatalf("signed files url download status=%d body=%s", dl.Code, dl.Body.String())
	}
	// 无签名直链必须被拒（证明下载成功来自签名而非直链放行）。
	bare := perform(r, http.MethodGet, "/api/v1/projects/epriv/packages/"+doc.Path)
	if bare.Code != http.StatusForbidden {
		t.Fatalf("unsigned private download status=%d want 403", bare.Code)
	}

	// 私有存储跳过 304 短路（§13.7）：If-None-Match 命中也恒回新鲜 200。
	req := httptest.NewRequest(http.MethodGet, url, nil)
	req.Header.Set("If-None-Match", w.Header().Get("ETag"))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("private project must skip 304 short-circuit, got %d", rec.Code)
	}
}

// TestFeedElectronGrayFallthrough 灰度 <100% 的非强制版本不作为 latest 投影，
// 回退到下一个可见版本（C17-5 / §4.4）。
func TestFeedElectronGrayFallthrough(t *testing.T) {
	r, projSvc, _, ctx := setupCheckTest(t)

	slug := "egray"
	p, _, err := projSvc.Create(ctx, service.CreateProjectInput{DefaultLocale: ptr("en"), Slug: &slug})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := projSvc.CreateMatrix(ctx, p.ID, service.MatrixWrite{
		OS: ptr("windows"), Arch: ptr("x86_64"), PackageType: ptr(model.PackageTypeSingleFile),
	}); err != nil {
		t.Fatal(err)
	}
	enableListing(t, projSvc, ctx, p.ID, "electron", "default", nil)
	seedClients(t, projSvc, p, 2)
	publishSingleFile(t, projSvc, ctx, p.ID, "1.0.0", "stable", "windows", "x86_64", 10, "egray-1.0.0-windows-x86_64.exe",
		func(in *service.VersionWriteInput) { in.Changelog = ptr("g 1.0.0") })
	publishSingleFile(t, projSvc, ctx, p.ID, "1.1.0", "stable", "windows", "x86_64", 10, "egray-1.1.0-windows-x86_64.exe",
		func(in *service.VersionWriteInput) { in.GrayStartPercent = ptr(50); in.Changelog = ptr("g 1.1.0") })

	w := perform(r, http.MethodGet, "/api/v1/projects/egray/store/electron/default/latest.yml")
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	doc := parseFeedElectronYML(t, w.Body.Bytes())
	if doc.Version != "1.0.0" {
		t.Fatalf("latest must fall through to 1.0.0, got %q", doc.Version)
	}
}
