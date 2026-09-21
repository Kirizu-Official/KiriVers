package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

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

// signedFixture 是签名验收测试的共享夹具：签名器时钟可控（TTL 边界）。
type signedFixture struct {
	r       *gin.Engine
	store   *repository.MemoryProjectStore
	projSvc *service.ProjectService
	signer  *urlsign.Signer
	clock   *time.Time
}

// setupSignedClientTest 构造带短时签名器（§13.7 / C15-1）的客户端引擎：
// update Service 注入 WithURLSigner，下载 handler 持同一签名器（同密钥）。
func setupSignedClientTest(t *testing.T) *signedFixture {
	t.Helper()
	gin.SetMode(gin.TestMode)
	store := repository.NewMemoryProjectStore()
	backend, err := storage.NewLocalFS(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	projSvc := service.NewProjectService(store, backend)
	now := time.Unix(1700000000, 0).UTC()
	f := &signedFixture{store: store, projSvc: projSvc, clock: &now}
	f.signer = urlsign.NewSigner("test-signing-secret", time.Hour)
	f.signer.SetClock(func() time.Time { return *f.clock })
	updates := update.NewService(repository.NewMemoryUpdateCatalog(store), update.WithURLSigner(f.signer))
	f.r = gin.New()
	Register(f.r.Group("/api/v1"), projSvc, updates, nil, nil, f.signer, nil)
	return f
}

// publishPrivate 发布单文件产物（复用 publishSingleFile 夹具流程）。
func (f *signedFixture) publishPrivate(t *testing.T, projectID uuid.UUID, versionRef, fname string) {
	t.Helper()
	payload := bytes.Repeat([]byte("y"), 64)
	sha, _ := hashutil.SHA256Hex(bytes.NewReader(payload))
	if _, _, err := f.projSvc.PutVersion(context.Background(), projectID, versionRef, service.VersionWriteInput{Channel: "stable", GrayStartPercent: ptr(100)}); err != nil {
		t.Fatalf("put version: %v", err)
	}
	if _, err := f.projSvc.UploadArtifact(context.Background(), fmt.Sprint(projectID), versionRef, "windows", "x86_64", service.UploadArtifactInput{
		Filename:       fname,
		ExpectedSHA256: sha,
		Size:           int64(len(payload)),
	}, bytes.NewReader(payload)); err != nil {
		t.Fatalf("upload artifact: %v", err)
	}
	if _, err := f.projSvc.ReadyVersionLine(context.Background(), projectID, versionRef, "windows", "x86_64"); err != nil {
		t.Fatalf("ready line: %v", err)
	}
	if _, err := f.projSvc.PublishVersion(context.Background(), projectID, versionRef); err != nil {
		t.Fatalf("publish: %v", err)
	}
}

// queryValue 从 URL query 提取单值。
func queryValue(raw, key string) string {
	q := raw
	if i := strings.Index(q, "?"); i >= 0 {
		q = q[i+1:]
	}
	for _, kv := range strings.Split(q, "&") {
		if strings.HasPrefix(kv, key+"=") {
			v, _ := url.QueryUnescape(strings.TrimPrefix(kv, key+"="))
			return v
		}
	}
	return ""
}

// TestPrivateProjectSignedDownload 验收 1/2（C15-1/C15-2）：私有项目下载必须
// 携带有效未过期签名（缺失/过期/篡改 → 403）；公开项目直链不要求签名。
func TestPrivateProjectSignedDownload(t *testing.T) {
	f := setupSignedClientTest(t)
	ctx := context.Background()

	// 私有项目 + 产物。
	slug := "priv"
	p, _, err := f.projSvc.Create(ctx, service.CreateProjectInput{DefaultLocale: ptr("en"), Slug: &slug})
	if err != nil {
		t.Fatal(err)
	}
	markProjectPrivate(t, f.store, ctx, p)
	if _, err := f.projSvc.CreateMatrix(ctx, p.ID, service.MatrixWrite{
		OS: ptr("windows"), Arch: ptr("x86_64"), PackageType: ptr(model.PackageTypeSingleFile),
	}); err != nil {
		t.Fatal(err)
	}
	f.publishPrivate(t, p.ID, "1.0.0", "app-1.0.0.zip")
	payload := bytes.Repeat([]byte("y"), 64)
	sha, _ := hashutil.SHA256Hex(bytes.NewReader(payload))
	pkgPath := "/api/v1/projects/priv/packages/" + sha

	// 1. 无签名 → 403 FORBIDDEN。
	w := perform(f.r, http.MethodGet, pkgPath)
	if w.Code != http.StatusForbidden {
		t.Fatalf("no signature: got %d, want 403", w.Code)
	}
	// 2. 签名器签发的地址 → 200。
	signed := f.signer.SignDownload(pkgPath, 0)
	if w = perform(f.r, http.MethodGet, signed); w.Code != http.StatusOK {
		t.Fatalf("signed download: got %d, want 200", w.Code)
	}
	// 3. 篡改 sig → 403。
	if w = perform(f.r, http.MethodGet, pkgPath+"?exp=1999999999&sig="+strings.Repeat("0", 64)); w.Code != http.StatusForbidden {
		t.Fatalf("tampered sig: got %d, want 403", w.Code)
	}
	// 4. 过期签名（TTL 边界：时钟拨过 exp）→ 403（验收 1：过期后 GET 非 200）。
	ttlSigned := f.signer.SignDownload(pkgPath, 60)
	*f.clock = f.clock.Add(61 * time.Second)
	if w = perform(f.r, http.MethodGet, ttlSigned); w.Code != http.StatusForbidden {
		t.Fatalf("expired signature: got %d, want 403", w.Code)
	}
	// TTL 边界内侧（59s 后）仍 200。
	*f.clock = f.clock.Add(-61 * time.Second).Add(59 * time.Second)
	signed60 := f.signer.SignDownload(pkgPath, 60)
	if w = perform(f.r, http.MethodGet, signed60); w.Code != http.StatusOK {
		t.Fatalf("within TTL: got %d, want 200", w.Code)
	}

	// 5. 公开项目（C15-2）：无签名直链 200。
	pub := "pubslug"
	pp, _, err := f.projSvc.Create(ctx, service.CreateProjectInput{DefaultLocale: ptr("en"), Slug: &pub})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.projSvc.CreateMatrix(ctx, pp.ID, service.MatrixWrite{
		OS: ptr("windows"), Arch: ptr("x86_64"), PackageType: ptr(model.PackageTypeSingleFile),
	}); err != nil {
		t.Fatal(err)
	}
	f.publishPrivate(t, pp.ID, "1.0.0", "pub-1.0.0.zip")
	pubPayload := bytes.Repeat([]byte("y"), 64)
	pubSHA, _ := hashutil.SHA256Hex(bytes.NewReader(pubPayload))
	if w = perform(f.r, http.MethodGet, "/api/v1/projects/pubslug/packages/"+pubSHA); w.Code != http.StatusOK {
		t.Fatalf("public direct download: got %d, want 200", w.Code)
	}
}

// TestPrivateProjectCheckSignedURLNo304 验收 1 + 304 跳过（C15-1）：
// 私有项目 check 200 的 package_url 含过期签名参数（?exp=&sig=）、
// Cache-Control 为 private, no-store；If-None-Match 命中仍回 200（不 304）；
// 过期后的 package_url 下载 403。公开项目 304 行为不变（回归）。
func TestPrivateProjectCheckSignedURLNo304(t *testing.T) {
	f := setupSignedClientTest(t)
	ctx := context.Background()

	slug := "privchk"
	p, _, err := f.projSvc.Create(ctx, service.CreateProjectInput{DefaultLocale: ptr("en"), Slug: &slug})
	if err != nil {
		t.Fatal(err)
	}
	markProjectPrivate(t, f.store, ctx, p)
	if _, err := f.projSvc.CreateMatrix(ctx, p.ID, service.MatrixWrite{
		OS: ptr("windows"), Arch: ptr("x86_64"), PackageType: ptr(model.PackageTypeSingleFile),
	}); err != nil {
		t.Fatal(err)
	}
	f.publishPrivate(t, p.ID, "0.9.0", "c-0.9.0.zip")
	f.publishPrivate(t, p.ID, "1.0.0", "c-1.0.0.zip")

	checkURL := "/api/v1/projects/privchk/update/check?current_version=0.9.0&os=windows&arch=x86_64"
	w := performCheck(f.r, checkURL)
	if w.Code != http.StatusOK {
		t.Fatalf("private check: got %d, want 200: %s", w.Code, w.Body.String())
	}
	if cc := w.Header().Get("Cache-Control"); cc != "private, no-store" {
		t.Fatalf("private Cache-Control = %q, want private, no-store", cc)
	}
	etag := w.Header().Get("ETag")
	if etag == "" {
		t.Fatal("private check must still send ETag for reference")
	}
	var body struct {
		PackageURL string `json:"package_url"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	// package_url 路径不变、签名只在 query（§13.7）。
	if !strings.HasPrefix(body.PackageURL, "/api/v1/projects/privchk/packages/") || queryValue(body.PackageURL, "sig") == "" || !strings.Contains(body.PackageURL, "?exp=") {
		t.Fatalf("package_url not signed with hash path: %q", body.PackageURL)
	}
	// 过期前的 package_url 可下载（时钟在 TTL 内）。
	if w = perform(f.r, http.MethodGet, body.PackageURL); w.Code != http.StatusOK {
		t.Fatalf("signed package download: got %d, want 200", w.Code)
	}

	// If-None-Match 命中 → 仍 200（私有项目跳过 304 短路）。
	rec := performCheckHdr(f.r, checkURL, http.Header{"If-None-Match": []string{etag}})
	if rec.Code != http.StatusOK {
		t.Fatalf("private check with If-None-Match: got %d, want 200 (304 must be skipped)", rec.Code)
	}

	// 时钟拨过 TTL → 同一 URL 过期，下载 403（验收 1）。
	*f.clock = f.clock.Add(time.Hour + time.Second)
	if w = perform(f.r, http.MethodGet, body.PackageURL); w.Code != http.StatusForbidden {
		t.Fatalf("expired signed package download: got %d, want 403", w.Code)
	}

	// 公开项目回归：ETag 输入与 304 语义不变。
	pub := "pubchk"
	pp, _, err := f.projSvc.Create(ctx, service.CreateProjectInput{DefaultLocale: ptr("en"), Slug: &pub})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.projSvc.CreateMatrix(ctx, pp.ID, service.MatrixWrite{
		OS: ptr("windows"), Arch: ptr("x86_64"), PackageType: ptr(model.PackageTypeSingleFile),
	}); err != nil {
		t.Fatal(err)
	}
	f.publishPrivate(t, pp.ID, "0.9.0", "p-0.9.0.zip")
	f.publishPrivate(t, pp.ID, "1.0.0", "p-1.0.0.zip")
	w = performCheck(f.r, "/api/v1/projects/pubchk/update/check?current_version=0.9.0&os=windows&arch=x86_64")
	if w.Code != http.StatusOK {
		t.Fatalf("public check: got %d", w.Code)
	}
	if cc := w.Header().Get("Cache-Control"); cc != "private, max-age=0, must-revalidate" {
		t.Fatalf("public Cache-Control = %q", cc)
	}
	pubEtag := w.Header().Get("ETag")
	rec = performCheckHdr(f.r, "/api/v1/projects/pubchk/update/check?current_version=0.9.0&os=windows&arch=x86_64", http.Header{"If-None-Match": []string{pubEtag}})
	if rec.Code != http.StatusNotModified {
		t.Fatalf("public check 304 regression: got %d, want 304", rec.Code)
	}
}
