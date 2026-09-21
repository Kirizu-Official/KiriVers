package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/Kirizu-Official/KiriVers/internal/model"
	"github.com/Kirizu-Official/KiriVers/internal/repository"
	"github.com/Kirizu-Official/KiriVers/internal/service"
	"github.com/Kirizu-Official/KiriVers/internal/service/update"
	"github.com/Kirizu-Official/KiriVers/internal/storage"
	"github.com/Kirizu-Official/KiriVers/pkg/hashutil"
)

func ptr[T any](v T) *T {
	return &v
}

func setupClientTest(t *testing.T) (*gin.Engine, *service.ProjectService, repository.ProjectStore) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	store := repository.NewMemoryProjectStore()
	backend, err := storage.NewLocalFS(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	projSvc := service.NewProjectService(store, backend)
	r := gin.New()
	Register(r.Group("/api/v1"), projSvc, update.NewService(repository.NewMemoryUpdateCatalog(store)), nil, nil, nil, nil)
	return r, projSvc, store
}

func TestClientArtifact_DownloadAndRange(t *testing.T) {
	r, projSvc, store := setupClientTest(t)
	ctx := context.Background()

	slug := "dl-proj"
	p, _, err := projSvc.Create(ctx, service.CreateProjectInput{DefaultLocale: ptr("en"), Slug: &slug,
		CompareEngine: ptr(model.CompareEngineSemver),
	})
	if err != nil {
		t.Fatal(err)
	}

	_, _, err = projSvc.PutVersion(ctx, p.ID, "1.0.0", service.VersionWriteInput{Channel: "stable"})
	if err != nil {
		t.Fatal(err)
	}

	payload := []byte("0123456789abcdefghijklmnopqrstuvwxyz") // 36 bytes
	sha, _ := hashutil.SHA256Hex(bytes.NewReader(payload))

	art, err := projSvc.UploadArtifact(ctx, p.Slug, "1.0.0", "windows", "x86_64", service.UploadArtifactInput{
		Filename:       "myprog.exe",
		ExpectedSHA256: sha,
		Size:           int64(len(payload)),
	}, bytes.NewReader(payload))
	if err != nil {
		t.Fatal(err)
	}

	// 1. 全量下载 -> 200 OK（哈希路径）
	dlPath := fmt.Sprintf("/api/v1/projects/%s/packages/%s", p.Slug, sha)
	reqFull := httptest.NewRequest(http.MethodGet, dlPath, nil)
	wFull := httptest.NewRecorder()
	r.ServeHTTP(wFull, reqFull)

	if wFull.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", wFull.Code)
	}
	if wFull.Header().Get("Accept-Ranges") != "bytes" {
		t.Fatalf("expected Accept-Ranges: bytes, got %s", wFull.Header().Get("Accept-Ranges"))
	}
	if wFull.Header().Get("Content-Length") != "36" {
		t.Fatalf("expected Content-Length: 36, got %s", wFull.Header().Get("Content-Length"))
	}
	if !bytes.Equal(wFull.Body.Bytes(), payload) {
		t.Fatalf("payload mismatch")
	}

	// 2. 旧 FileName 路径必须 404
	pkgPath := fmt.Sprintf("/api/v1/projects/%s/packages/%s", p.Slug, art.FileName)
	reqPkg := httptest.NewRequest(http.MethodGet, pkgPath, nil)
	wPkg := httptest.NewRecorder()
	r.ServeHTTP(wPkg, reqPkg)
	if wPkg.Code != http.StatusNotFound {
		t.Fatalf("filename lookup must 404, got %d", wPkg.Code)
	}

	// UUID 旧路由不注册
	oldUUID := fmt.Sprintf("/api/v1/projects/%s/artifacts/%s/%s", p.Slug, art.ID, art.FileName)
	wUUID := httptest.NewRecorder()
	r.ServeHTTP(wUUID, httptest.NewRequest(http.MethodGet, oldUUID, nil))
	if wUUID.Code != http.StatusNotFound {
		t.Fatalf("uuid artifact route must 404, got %d", wUUID.Code)
	}

	// 3. Range 读前 10 字节 (bytes=0-9) -> 206 Partial Content
	reqRange1 := httptest.NewRequest(http.MethodGet, dlPath, nil)
	reqRange1.Header.Set("Range", "bytes=0-9")
	wRange1 := httptest.NewRecorder()
	r.ServeHTTP(wRange1, reqRange1)

	if wRange1.Code != http.StatusPartialContent {
		t.Fatalf("expected 206 Partial Content, got %d", wRange1.Code)
	}
	if wRange1.Header().Get("Content-Range") != "bytes 0-9/36" {
		t.Fatalf("expected Content-Range: bytes 0-9/36, got %s", wRange1.Header().Get("Content-Range"))
	}
	if wRange1.Header().Get("Content-Length") != "10" {
		t.Fatalf("expected Content-Length: 10, got %s", wRange1.Header().Get("Content-Length"))
	}
	if wRange1.Body.String() != "0123456789" {
		t.Fatalf("expected '0123456789', got %q", wRange1.Body.String())
	}

	// 4. Range 读中间字节 (bytes=10-15) -> 206
	reqRange2 := httptest.NewRequest(http.MethodGet, dlPath, nil)
	reqRange2.Header.Set("Range", "bytes=10-15")
	wRange2 := httptest.NewRecorder()
	r.ServeHTTP(wRange2, reqRange2)

	if wRange2.Code != http.StatusPartialContent {
		t.Fatalf("expected 206 Partial Content, got %d", wRange2.Code)
	}
	if wRange2.Body.String() != "abcdef" {
		t.Fatalf("expected 'abcdef', got %q", wRange2.Body.String())
	}

	// 5. 非法 Range (start >= size) -> 416 Range Not Satisfiable
	reqBadRange := httptest.NewRequest(http.MethodGet, dlPath, nil)
	reqBadRange.Header.Set("Range", "bytes=100-200")
	wBadRange := httptest.NewRecorder()
	r.ServeHTTP(wBadRange, reqBadRange)

	if wBadRange.Code != http.StatusRequestedRangeNotSatisfiable {
		t.Fatalf("expected 416, got %d", wBadRange.Code)
	}
	if wBadRange.Header().Get("Content-Range") != "bytes */36" {
		t.Fatalf("expected Content-Range: bytes */36, got %s", wBadRange.Header().Get("Content-Range"))
	}

	// 6. HEAD 请求 -> 200 OK，无 body
	reqHead := httptest.NewRequest(http.MethodHead, dlPath, nil)
	wHead := httptest.NewRecorder()
	r.ServeHTTP(wHead, reqHead)

	if wHead.Code != http.StatusOK || wHead.Body.Len() != 0 {
		t.Fatalf("expected 200 OK with empty body for HEAD, got %d, len=%d", wHead.Code, wHead.Body.Len())
	}
	if wHead.Header().Get("Content-Length") != "36" {
		t.Fatalf("expected Content-Length: 36 on HEAD, got %s", wHead.Header().Get("Content-Length"))
	}

	// D7：装饰后缀仍按前导 64 hex 查找。
	reqDec := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/v1/projects/%s/packages/%s.exe", p.Slug, sha), nil)
	wDec := httptest.NewRecorder()
	r.ServeHTTP(wDec, reqDec)
	if wDec.Code != http.StatusOK || !bytes.Equal(wDec.Body.Bytes(), payload) {
		t.Fatalf("decorated hash GET: status=%d", wDec.Code)
	}

	unknown := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	reqUnknown := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/v1/projects/%s/packages/%s", p.Slug, unknown), nil)
	wUnknown := httptest.NewRecorder()
	r.ServeHTTP(wUnknown, reqUnknown)
	if wUnknown.Code != http.StatusNotFound {
		t.Fatalf("unknown hash status=%d", wUnknown.Code)
	}
	var env struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(wUnknown.Body.Bytes(), &env); err != nil {
		t.Fatalf("unknown hash must be JSON envelope: %s", wUnknown.Body.String())
	}
	if env.Error.Code != "NOT_FOUND" {
		t.Fatalf("unknown hash code=%q body=%s", env.Error.Code, wUnknown.Body.String())
	}

	_ = store
}

func TestClientArtifact_HardwareRevisionSecurity(t *testing.T) {
	r, projSvc, store := setupClientTest(t)
	ctx := context.Background()

	slug := "hw-sec-proj"
	p, _, _ := projSvc.Create(ctx, service.CreateProjectInput{DefaultLocale: ptr("en"), Slug: &slug,
		CompareEngine: ptr(model.CompareEngineSemver),
	})

	_ = store.CreateHwRev(ctx, &model.HwRev{ProjectID: p.ID, Slug: "v1", Rank: 1})
	_ = store.CreateHwRev(ctx, &model.HwRev{ProjectID: p.ID, Slug: "v2", Rank: 2})

	_ = store.CreateMatrix(ctx, &model.PlatformMatrix{
		ProjectID:       p.ID,
		OS:              "linux",
		Arch:            "arm64",
		PackageType:     "single_file",
		HwVariantPolicy: model.HwVariantIndependent,
	})

	_, _, _ = projSvc.PutVersion(ctx, p.ID, "1.0.0", service.VersionWriteInput{Channel: "stable"})

	rev2 := "v2"
	art, err := projSvc.UploadArtifact(ctx, p.Slug, "1.0.0", "linux", "arm64", service.UploadArtifactInput{
		Filename: "fw.bin",
		HwRev:    &rev2,
		Size:     4,
	}, bytes.NewReader([]byte("firmware")))
	if err != nil {
		t.Fatal(err)
	}

	dlPath := fmt.Sprintf("/api/v1/projects/%s/packages/%s", p.Slug, art.SHA256)

	// 哈希 GET 跳过 HW_REV_INCOMPATIBLE：未传 hw_rev 仍 200
	reqNoHw := httptest.NewRequest(http.MethodGet, dlPath, nil)
	wNoHw := httptest.NewRecorder()
	r.ServeHTTP(wNoHw, reqNoHw)
	if wNoHw.Code != http.StatusOK {
		t.Fatalf("hash GET must skip hw_rev gate, got %d", wNoHw.Code)
	}

	oldUUID := fmt.Sprintf("/api/v1/projects/%s/artifacts/%s/%s", p.Slug, art.ID, art.FileName)
	wOld := httptest.NewRecorder()
	r.ServeHTTP(wOld, httptest.NewRequest(http.MethodGet, oldUUID, nil))
	if wOld.Code != http.StatusNotFound {
		t.Fatalf("uuid artifact route must 404, got %d", wOld.Code)
	}
}

func TestClientManifestUnregistered(t *testing.T) {
	r, projSvc, _ := setupClientTest(t)
	slug := "client-mf"
	if _, _, err := projSvc.Create(context.Background(), service.CreateProjectInput{DefaultLocale: ptr("en"), Slug: &slug}); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/projects/client-mf/versions/1.0.0/lines/windows/x86_64/manifest", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("client manifest must 404, got %d %s", w.Code, w.Body.String())
	}
}

func TestClientArtifact_DeltaAndPatchKinds(t *testing.T) {
	r, projSvc, store := setupClientTest(t)
	ctx := context.Background()
	slug := "kind-dl"
	p, _, err := projSvc.Create(ctx, service.CreateProjectInput{DefaultLocale: ptr("en"), Slug: &slug,
		CompareEngine: ptr(model.CompareEngineSemver),
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := projSvc.PutVersion(ctx, p.ID, "1.0.0", service.VersionWriteInput{Channel: "stable"}); err != nil {
		t.Fatal(err)
	}

	deltaPayload := []byte("delta-kind-bytes")
	deltaSHA, _ := hashutil.SHA256Hex(bytes.NewReader(deltaPayload))
	deltaArt, err := projSvc.UploadArtifact(ctx, p.Slug, "1.0.0", "windows", "x86_64", service.UploadArtifactInput{
		Filename: "delta.bin", Size: int64(len(deltaPayload)), ExpectedSHA256: deltaSHA,
	}, bytes.NewReader(deltaPayload))
	if err != nil {
		t.Fatal(err)
	}
	deltaArt.Kind = model.ArtifactKindDelta
	if err := store.SaveArtifact(ctx, deltaArt); err != nil {
		t.Fatal(err)
	}
	wDelta := httptest.NewRecorder()
	r.ServeHTTP(wDelta, httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/v1/projects/%s/packages/%s", p.Slug, deltaSHA), nil))
	if wDelta.Code != http.StatusOK || !bytes.Equal(wDelta.Body.Bytes(), deltaPayload) {
		t.Fatalf("kind=delta GET: status=%d body=%q", wDelta.Code, wDelta.Body.String())
	}
	wDeltaHead := httptest.NewRecorder()
	r.ServeHTTP(wDeltaHead, httptest.NewRequest(http.MethodHead, fmt.Sprintf("/api/v1/projects/%s/packages/%s", p.Slug, deltaSHA), nil))
	if wDeltaHead.Code != http.StatusOK || wDeltaHead.Body.Len() != 0 {
		t.Fatalf("kind=delta HEAD: status=%d len=%d", wDeltaHead.Code, wDeltaHead.Body.Len())
	}

	if _, _, err := projSvc.PutVersion(ctx, p.ID, "1.1.0", service.VersionWriteInput{Channel: "stable"}); err != nil {
		t.Fatal(err)
	}
	patchPayload := []byte("patch-kind-bytes")
	patchSHA, _ := hashutil.SHA256Hex(bytes.NewReader(patchPayload))
	patchArt, err := projSvc.UploadArtifact(ctx, p.Slug, "1.1.0", "linux", "x86_64", service.UploadArtifactInput{
		Filename: "patch.bin", Size: int64(len(patchPayload)), ExpectedSHA256: patchSHA,
	}, bytes.NewReader(patchPayload))
	if err != nil {
		t.Fatal(err)
	}
	patchArt.Kind = model.ArtifactKindPatch
	if err := store.SaveArtifact(ctx, patchArt); err != nil {
		t.Fatal(err)
	}
	wPatch := httptest.NewRecorder()
	r.ServeHTTP(wPatch, httptest.NewRequest(http.MethodHead, fmt.Sprintf("/api/v1/projects/%s/packages/%s", p.Slug, patchSHA), nil))
	if wPatch.Code != http.StatusOK || wPatch.Body.Len() != 0 {
		t.Fatalf("kind=patch HEAD: status=%d len=%d", wPatch.Code, wPatch.Body.Len())
	}
	wPatchGet := httptest.NewRecorder()
	r.ServeHTTP(wPatchGet, httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/v1/projects/%s/packages/%s", p.Slug, patchSHA), nil))
	if wPatchGet.Code != http.StatusOK || !bytes.Equal(wPatchGet.Body.Bytes(), patchPayload) {
		t.Fatalf("kind=patch GET: status=%d body=%q", wPatchGet.Code, wPatchGet.Body.String())
	}
}
