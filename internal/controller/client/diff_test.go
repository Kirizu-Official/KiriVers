package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"go/parser"
	"go/token"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/Kirizu-Official/KiriVers/internal/model"
	"github.com/Kirizu-Official/KiriVers/internal/service"
)

// setupDiffProject 建项目 + multi_file 矩阵 + 1.0.0（3 条 Manifest，含一条删除位）/
// 1.1.0（3 条新增位）两个已发布版本。返回项目与（供灰度用例的）版本输入入口。
func setupDiffProject(t *testing.T, projSvc *service.ProjectService, ctx context.Context) *model.Project {
	t.Helper()
	return setupIntegrityProject(t, projSvc, ctx)
}

// diffPOST 发起 diff POST 请求（body 为 JSON 对象）。
func diffPOST(r *gin.Engine, project string, body map[string]any) *httptest.ResponseRecorder {
	raw, err := json.Marshal(body)
	if err != nil {
		panic(err)
	}
	req := httptest.NewRequest(http.MethodPost,
		fmt.Sprintf("/api/v1/projects/%s/update/diff", project), bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

// diffBody 构造 1.0.0 → 1.1.0 的请求体。
func diffBody(extra map[string]any) map[string]any {
	b := map[string]any{
		"source_version": "1.0.0",
		"target_version": "1.1.0",
		"os":             "windows",
		"arch":           "x86_64",
	}
	for k, v := range extra {
		b[k] = v
	}
	return b
}

// 端到端干净升级：diff_mode=full_package，全量三元组与 check 200 同源一致，
// deleted_paths 出现在响应中；恒 private no-store、无 ETag（C09-3 / §13.2）。
func TestDiffEndpointFullPackage(t *testing.T) {
	r, projSvc, _, ctx := setupIntegrityTest(t)
	p := setupDiffProject(t, projSvc, ctx)

	// check 200 基线。
	cw := performCheck(r, fmt.Sprintf("/api/v1/projects/%s/update/check?os=windows&arch=x86_64&current_version=1.0.0", p.Slug))
	if cw.Code != http.StatusOK {
		t.Fatalf("check expected 200, got %d %s", cw.Code, cw.Body.String())
	}
	var chk struct {
		PackageURL string `json:"package_url"`
		Size       int64  `json:"size"`
		SHA256     string `json:"sha256"`
	}
	if err := json.Unmarshal(cw.Body.Bytes(), &chk); err != nil {
		t.Fatal(err)
	}

	w := diffPOST(r, p.Slug, diffBody(map[string]any{"protocol_version": 0}))
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d %s", w.Code, w.Body.String())
	}
	if cc := w.Header().Get("Cache-Control"); cc != "private, no-store" {
		t.Fatalf("diff must always be private, no-store; got %q", cc)
	}
	if etag := w.Header().Get("ETag"); etag != "" {
		t.Fatalf("diff must never carry an ETag, got %q", etag)
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
		DiffMode       string   `json:"diff_mode"`
		PackageURL     string   `json:"package_url"`
		Size           int64    `json:"size"`
		SHA256         string   `json:"sha256"`
		RootHash       string   `json:"root_hash"`
		VersionInteger int64    `json:"version_integer"`
		VersionSemver  string   `json:"version_semver"`
		CompareEngine  string   `json:"compare_engine"`
		DeletedPaths   []string `json:"deleted_paths"`
		Files          []any    `json:"files"`
		InvalidPaths   []string `json:"invalid_paths"`
		Signature      string   `json:"signature"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &res); err != nil {
		t.Fatal(err)
	}
	if res.DiffMode != "full_package" {
		t.Fatalf("diff_mode wrong: %+v", res)
	}
	// 验收项：check 与 diff 对同一全量包三元组一致。
	if res.PackageURL != chk.PackageURL || res.Size != chk.Size || res.SHA256 != chk.SHA256 {
		t.Fatalf("diff triple must match check: %+v vs %+v", res, chk)
	}
	if res.RootHash == "" || res.CompareEngine != "semver" || res.VersionSemver != "1.1.0" {
		t.Fatalf("target meta wrong: %+v", res)
	}
	// 1.0.0 独有路径 m/00.txt 在 1.1.0 中不存在 → deleted_paths。
	if len(res.DeletedPaths) == 0 {
		t.Fatalf("expected deleted_paths, got %+v", res)
	}
	// 验收项：未声明 file_list 的多文件 diff 响应无逐文件 URL 数组。
	if res.Files != nil {
		t.Fatalf("undeclared file_list must never produce files[]: %+v", res.Files)
	}
	if res.InvalidPaths != nil {
		t.Fatalf("clean upgrade must not report invalid_paths: %+v", res.InvalidPaths)
	}
}

// 验收项（C09-11）：非强制且灰度未命中时，对指定 target 的 diff 返回
// 412 PRECONDITION_FAILED（匿名分支），不得下发任何包。
func TestDiffEndpointGrayPrecondition(t *testing.T) {
	r, projSvc, store, ctx := setupIntegrityTest(t)
	p := setupDiffProject(t, projSvc, ctx)

	v, err := projSvc.ResolveVersion(ctx, p.ID, "1.1.0")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	v.GrayStartPercent = 0
	v.GrayStartedAt = &now
	v.GrayCompletedAt = nil
	if err := store.SaveVersion(ctx, v); err != nil {
		t.Fatal(err)
	}

	w := diffPOST(r, p.Slug, diffBody(nil))
	if w.Code != http.StatusPreconditionFailed || !strings.Contains(w.Body.String(), "PRECONDITION_FAILED") {
		t.Fatalf("gray miss must be 412 PRECONDITION_FAILED, got %d %s", w.Code, w.Body.String())
	}
	if strings.Contains(w.Body.String(), "packages/") {
		t.Fatalf("412 must not serve any package url: %s", w.Body.String())
	}

	if _, err := projSvc.CompleteGray(ctx, p.ID, "1.1.0"); err != nil {
		t.Fatal(err)
	}
	w2 := diffPOST(r, p.Slug, diffBody(nil))
	if w2.Code != http.StatusOK {
		t.Fatalf("complete gray must pass, got %d %s", w2.Code, w2.Body.String())
	}
}

// 端到端错误闸：未知 target / Draft target / 渠道冲突 / 缺必填 / 非法 JSON。
func TestDiffEndpointGates(t *testing.T) {
	r, projSvc, _, ctx := setupIntegrityTest(t)
	p := setupDiffProject(t, projSvc, ctx)

	// 未知 target。
	w := diffPOST(r, p.Slug, diffBody(map[string]any{"target_version": "9.9.9"}))
	if w.Code != http.StatusNotFound || !strings.Contains(w.Body.String(), "VERSION_NOT_FOUND") {
		t.Fatalf("unknown target must be 404 VERSION_NOT_FOUND, got %d %s", w.Code, w.Body.String())
	}

	// Draft target。
	if _, _, err := projSvc.PutVersion(ctx, p.ID, "2.0.0", service.VersionWriteInput{Channel: "stable"}); err != nil {
		t.Fatal(err)
	}
	w = diffPOST(r, p.Slug, diffBody(map[string]any{"target_version": "2.0.0"}))
	if w.Code != http.StatusNotFound || !strings.Contains(w.Body.String(), "VERSION_NOT_VISIBLE") {
		t.Fatalf("draft target must be 404 VERSION_NOT_VISIBLE, got %d %s", w.Code, w.Body.String())
	}

	// 渠道冲突。
	w = diffPOST(r, p.Slug, diffBody(map[string]any{"channel": "beta"}))
	if w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), "CHANNEL_CONFLICT") {
		t.Fatalf("channel conflict must be 400, got %d %s", w.Code, w.Body.String())
	}

	// 缺必填。
	w = diffPOST(r, p.Slug, map[string]any{"source_version": "1.0.0"})
	if w.Code != http.StatusBadRequest {
		t.Fatalf("missing fields must be 400, got %d", w.Code)
	}

	// 非法 JSON。
	w = diffPOST(r, p.Slug, nil)
	req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/v1/projects/%s/update/diff", p.Slug), strings.NewReader("{not-json"))
	req.Header.Set("Content-Type", "application/json")
	w2 := httptest.NewRecorder()
	r.ServeHTTP(w2, req)
	if w2.Code != http.StatusBadRequest {
		t.Fatalf("bad JSON must be 400, got %d", w2.Code)
	}
}

// 验收项（结构断言）：diff / pack / integrity HTTP 与 update 服务源码不得导入任何
// 归档/打包库。zip 只允许出现在 internal/service 的 Job 路径。
func TestDiffPathHasNoPackingImports(t *testing.T) {
	dirs := []string{".", filepath.Join("..", "..", "service", "update")}
	for _, dir := range dirs {
		entries, err := os.ReadDir(dir)
		if err != nil {
			t.Fatalf("read %s: %v", dir, err)
		}
		for _, e := range entries {
			name := e.Name()
			// 只检查生产代码；_test.go 允许构造 zip 夹具（如上传闸门测试）。
			if e.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
				continue
			}
			fset := token.NewFileSet()
			f, err := parser.ParseFile(fset, filepath.Join(dir, name), nil, parser.ImportsOnly)
			if err != nil {
				t.Fatalf("parse %s: %v", name, err)
			}
			for _, imp := range f.Imports {
				path, _ := strconv.Unquote(imp.Path.Value)
				if isPackingImport(path) {
					t.Errorf("%s imports packing library %q (forbidden on the diff path)", name, path)
				}
			}
		}
	}
}

// isPackingImport 判断导入是否为归档/压缩打包库。
// archive/zip、archive/tar 及任何名称含 zip/tar/gzip 的压缩包库均禁止；
// 单纯哈希（hashutil）与 JSON 编码不在其列。
func isPackingImport(path string) bool {
	if path == "archive/zip" || path == "archive/tar" {
		return true
	}
	base := path[strings.LastIndex(path, "/")+1:]
	return base == "zip" || base == "tar" || base == "gzip" || base == "zstd"
}
