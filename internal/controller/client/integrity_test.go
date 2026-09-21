package client

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
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
)

// setupIntegrityTest 构造 gin 引擎 + 内存 Catalog/LineDetailSource 的集成测试环境
// （integrity/diff 需要 LineDetailSource 读取 Manifest）。
func setupIntegrityTest(t *testing.T) (*gin.Engine, *service.ProjectService, repository.ProjectStore, context.Context) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	store := repository.NewMemoryProjectStore()
	backend, err := storage.NewLocalFS(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	projSvc := service.NewProjectService(store, backend)
	r := gin.New()
	Register(r.Group("/api/v1"), projSvc,
		update.NewService(repository.NewMemoryUpdateCatalog(store), update.WithLineDetails(store)), nil, nil, nil, nil)
	return r, projSvc, store, context.Background()
}

// integritySHA 是合法 64 位十六进制占位哈希。
const integritySHA = "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"

// mkZipPayload 构造合法 zip 字节流（上传闸门校验扩展名与内容）。
func mkZipPayload(t *testing.T, size int) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, err := zw.Create("placeholder.bin")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write(bytes.Repeat([]byte("z"), size)); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// publishMultiFile 走完整夹具流程：矩阵行（multi_file）→ 建版本 → 上传全量 zip →
// 设置 Manifest → 就绪 → 发布。
func publishMultiFile(t *testing.T, projSvc *service.ProjectService, ctx context.Context, projectID uuid.UUID, versionRef, channel string, entries []service.ManifestEntryInput, mutators ...func(*service.VersionWriteInput)) *model.Version {
	t.Helper()
	in := service.VersionWriteInput{Channel: channel}
	for _, m := range mutators {
		m(&in)
	}
	if _, _, err := projSvc.PutVersion(ctx, projectID, versionRef, in); err != nil {
		t.Fatalf("put version %s: %v", versionRef, err)
	}
	payload := mkZipPayload(t, 1024)
	sha, _ := hashutil.SHA256Hex(bytes.NewReader(payload))
	fname := fmt.Sprintf("demo-%s-windows-x86_64.zip", versionRef)
	if _, err := projSvc.UploadArtifact(ctx, fmt.Sprint(projectID), versionRef, "windows", "x86_64",
		service.UploadArtifactInput{Filename: fname, ExpectedSHA256: sha, Size: int64(len(payload))},
		bytes.NewReader(payload)); err != nil {
		t.Fatalf("upload artifact %s: %v", versionRef, err)
	}
	if _, err := projSvc.SetManifest(ctx, fmt.Sprint(projectID), versionRef, "windows", "x86_64", entries); err != nil {
		t.Fatalf("set manifest %s: %v", versionRef, err)
	}
	if _, err := projSvc.ReadyVersionLine(ctx, projectID, versionRef, "windows", "x86_64"); err != nil {
		t.Fatalf("ready line %s: %v", versionRef, err)
	}
	v, err := projSvc.PublishVersion(ctx, projectID, versionRef)
	if err != nil {
		t.Fatalf("publish %s: %v", versionRef, err)
	}
	return v
}

// integrityMD5 是合法 32 位十六进制占位 MD5（SetManifest 复核格式）。
const integrityMD5 = "d41d8cd98f00b204e9800998ecf8427e"

// mkMFEntries 构造 n 条 Manifest 输入（路径 m/00..）。
func mkMFEntries(n, offset int) []service.ManifestEntryInput {
	entries := make([]service.ManifestEntryInput, 0, n)
	for i := offset; i < offset+n; i++ {
		entries = append(entries, service.ManifestEntryInput{
			Path:          fmt.Sprintf("m/%02d.txt", i),
			Size:          int64(10 + i),
			SHA256:        integritySHA,
			MD5:           integrityMD5,
			InstallPolicy: "OVERWRITE",
		})
	}
	return entries
}

// setupIntegrityProject 建项目 + multi_file 矩阵 + 1.0.0/1.1.0 两个已发布版本。
func setupIntegrityProject(t *testing.T, projSvc *service.ProjectService, ctx context.Context) *model.Project {
	t.Helper()
	slug := "integrity-e2e"
	p, _, err := projSvc.Create(ctx, service.CreateProjectInput{DefaultLocale: ptr("en"), Slug:          &slug,
		CompareEngine: ptr(model.CompareEngineSemver),
	})
	if err != nil {
		t.Fatal(err)
	}
	osStr, archStr := "windows", "x86_64"
	pkgType := model.PackageTypeMultiFile
	if _, err := projSvc.CreateMatrix(ctx, p.ID, service.MatrixWrite{
		OS: &osStr, Arch: &archStr, PackageType: &pkgType,
	}); err != nil {
		t.Fatal(err)
	}
	publishMultiFile(t, projSvc, ctx, p.ID, "1.0.0", "stable", mkMFEntries(3, 0))
	publishMultiFile(t, projSvc, ctx, p.ID, "1.1.0", "stable", mkMFEntries(3, 1))
	return p
}

// integrityGET 发起 integrity GET 请求。
func integrityGET(r *gin.Engine, project, query string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet,
		fmt.Sprintf("/api/v1/projects/%s/versions/1.0.0/integrity?os=windows&arch=x86_64&%s", project, query), nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

// 端到端 200：字段快照（root_hash、full_package_url、files、install_policy、
// integrity_check、channel、package_type）。
func TestIntegrityEndpoint200(t *testing.T) {
	r, projSvc, _, ctx := setupIntegrityTest(t)
	p := setupIntegrityProject(t, projSvc, ctx)

	w := integrityGET(r, p.Slug, "")
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d %s", w.Code, w.Body.String())
	}
	if w.Header().Get("X-KiriVers-Protocol") != "" {
		t.Fatalf("protocol header must be absent: %q", w.Header().Get("X-KiriVers-Protocol"))
	}
	var raw map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &raw); err != nil {
		t.Fatal(err)
	}
	if _, ok := raw["server_protocol"]; ok {
		t.Fatalf("body must not contain server_protocol")
	}
	var res struct {
		VersionInteger int64  `json:"version_integer"`
		VersionSemver  string `json:"version_semver"`
		Channel        string `json:"channel"`
		PackageType    string `json:"package_type"`
		RootHash       string `json:"root_hash"`
		FullPackageURL string `json:"full_package_url"`
		Size           int64  `json:"size"`
		SHA256         string `json:"sha256"`
		Files          []struct {
			Path           string `json:"path"`
			Size           int64  `json:"size"`
			SHA256         string `json:"sha256"`
			MD5            string `json:"md5"`
			InstallPolicy  string `json:"install_policy"`
			IntegrityCheck bool   `json:"integrity_check"`
			URL            string `json:"url"`
		} `json:"files"`
		Signature string `json:"signature"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &res); err != nil {
		t.Fatal(err)
	}
	if res.VersionSemver != "1.0.0" || res.Channel != "stable" {
		t.Fatalf("header fields wrong: %+v", res)
	}
	if res.PackageType != model.PackageTypeMultiFile {
		t.Fatalf("package_type = %s", res.PackageType)
	}
	if res.RootHash == "" {
		t.Fatalf("root_hash must be present")
	}
	if res.FullPackageURL != fmt.Sprintf("/api/v1/projects/%s/packages/%s", p.Slug, res.SHA256) ||
		res.Size <= 0 || res.SHA256 == "" {
		t.Fatalf("full package triple wrong: %+v", res)
	}
	if len(res.Files) != 3 || res.Files[0].Path != "m/00.txt" || res.Files[2].Path != "m/02.txt" {
		t.Fatalf("files wrong: %+v", res.Files)
	}
	for _, f := range res.Files {
		if f.InstallPolicy != "OVERWRITE" || !f.IntegrityCheck || f.SHA256 != integritySHA || f.URL != "" {
			t.Fatalf("file entry wrong: %+v", f)
		}
	}
	if _, ok := raw["next_cursor"]; ok {
		t.Fatalf("integrity must omit next_cursor")
	}
	_ = res.VersionInteger
}

// 端到端 304：If-None-Match 命中（ETag = 线 root_hash）→ 304 无 body，带缓存头。
func TestIntegrityEndpoint304(t *testing.T) {
	r, projSvc, _, ctx := setupIntegrityTest(t)
	p := setupIntegrityProject(t, projSvc, ctx)

	w := integrityGET(r, p.Slug, "")
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	etag := w.Header().Get("ETag")
	if etag == "" {
		t.Fatalf("200 must carry ETag")
	}
	if w.Header().Get("Cache-Control") == "" {
		t.Fatalf("200 must carry Cache-Control")
	}

	req := httptest.NewRequest(http.MethodGet,
		fmt.Sprintf("/api/v1/projects/%s/versions/1.0.0/integrity?os=windows&arch=x86_64", p.Slug), nil)
	req.Header.Set("If-None-Match", etag)
	w2 := httptest.NewRecorder()
	r.ServeHTTP(w2, req)
	if w2.Code != http.StatusNotModified {
		t.Fatalf("expected 304, got %d %s", w2.Code, w2.Body.String())
	}
	if w2.Header().Get("ETag") != etag {
		t.Fatalf("304 must echo ETag")
	}
	if w2.Header().Get("Cache-Control") == "" {
		t.Fatalf("304 must carry Cache-Control")
	}
}

// 端到端错误闸：Draft 404 VERSION_NOT_VISIBLE；Revoked 409 无任何下载 URL；
// 无线 404 VERSION_LINE_NOT_FOUND；channel 冲突 400；缺参 400。
func TestIntegrityEndpointGates(t *testing.T) {
	r, projSvc, _, ctx := setupIntegrityTest(t)
	p := setupIntegrityProject(t, projSvc, ctx)

	// Draft：建了但未发布。
	if _, _, err := projSvc.PutVersion(ctx, p.ID, "9.9.9", service.VersionWriteInput{Channel: "stable"}); err != nil {
		t.Fatal(err)
	}
	w := perform(r, http.MethodGet, fmt.Sprintf("/api/v1/projects/%s/versions/9.9.9/integrity?os=windows&arch=x86_64", p.Slug))
	if w.Code != http.StatusNotFound || !strings.Contains(w.Body.String(), "VERSION_NOT_VISIBLE") {
		t.Fatalf("draft must be 404 VERSION_NOT_VISIBLE, got %d %s", w.Code, w.Body.String())
	}

	// 无此平台线。
	w = perform(r, http.MethodGet, fmt.Sprintf("/api/v1/projects/%s/versions/1.1.0/integrity?os=linux&arch=x86_64", p.Slug))
	if w.Code != http.StatusNotFound || !strings.Contains(w.Body.String(), "VERSION_LINE_NOT_FOUND") {
		t.Fatalf("missing line must be 404 VERSION_LINE_NOT_FOUND, got %d %s", w.Code, w.Body.String())
	}

	// channel 冲突（C09-10）：不得假装另一渠道的版本。
	w = perform(r, http.MethodGet, fmt.Sprintf("/api/v1/projects/%s/versions/1.1.0/integrity?os=windows&arch=x86_64&channel=beta", p.Slug))
	if w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), "CHANNEL_CONFLICT") {
		t.Fatalf("channel conflict must be 400 CHANNEL_CONFLICT, got %d %s", w.Code, w.Body.String())
	}

	// Revoked：409 且响应无任何下载 URL（验收项）。
	if _, err := projSvc.RevokeVersion(ctx, p.ID, "1.0.0"); err != nil {
		t.Fatal(err)
	}
	w = integrityGET(r, p.Slug, "")
	if w.Code != http.StatusConflict || !strings.Contains(w.Body.String(), "VERSION_REVOKED") {
		t.Fatalf("revoked must be 409 VERSION_REVOKED, got %d %s", w.Code, w.Body.String())
	}
	if strings.Contains(w.Body.String(), "packages/") || strings.Contains(w.Body.String(), "package_url") ||
		strings.Contains(w.Body.String(), "full_package_url") {
		t.Fatalf("revoked response must not contain any download url: %s", w.Body.String())
	}

	// 缺 os/arch。
	w = perform(r, http.MethodGet, fmt.Sprintf("/api/v1/projects/%s/versions/1.1.0/integrity", p.Slug))
	if w.Code != http.StatusBadRequest {
		t.Fatalf("missing os/arch must be 400, got %d", w.Code)
	}
}

// 端到端：一次返回全部文件；遗留 limit/cursor 忽略；compact / hash_algo。
func TestIntegrityEndpointNoPaging(t *testing.T) {
	r, projSvc, _, ctx := setupIntegrityTest(t)
	p := setupIntegrityProject(t, projSvc, ctx)

	w := integrityGET(r, p.Slug, "limit=2")
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var all struct {
		Files      []struct{ Path string } `json:"files"`
		NextCursor string                  `json:"next_cursor"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &all); err != nil {
		t.Fatal(err)
	}
	if len(all.Files) != 3 || all.NextCursor != "" {
		t.Fatalf("must return all files without next_cursor: %+v", all)
	}

	w2 := integrityGET(r, p.Slug, "cursor=m/01.txt")
	if w2.Code != http.StatusOK {
		t.Fatalf("leftover cursor must be ignored, got %d", w2.Code)
	}

	w4 := integrityGET(r, p.Slug, "hash_algo=both&compact=true")
	var comp struct {
		Files []struct {
			MD5    string `json:"md5"`
			SHA256 string `json:"sha256"`
		} `json:"files"`
	}
	if err := json.Unmarshal(w4.Body.Bytes(), &comp); err != nil {
		t.Fatal(err)
	}
	if len(comp.Files) == 0 || comp.Files[0].MD5 != "" || comp.Files[0].SHA256 == "" {
		t.Fatalf("compact must omit md5: %+v", comp.Files)
	}

	w5 := integrityGET(r, p.Slug, "hash_algo=crc32")
	if w5.Code != http.StatusBadRequest {
		t.Fatalf("invalid hash_algo must be 400, got %d", w5.Code)
	}
}
