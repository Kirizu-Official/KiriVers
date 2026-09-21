package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
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
)

// setupCheckTest 构造 gin 引擎 + 内存 CatalogLoader 的集成测试环境。
func setupCheckTest(t *testing.T) (*gin.Engine, *service.ProjectService, repository.ProjectStore, context.Context) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	store := repository.NewMemoryProjectStore()
	backend, err := storage.NewLocalFS(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	projSvc := service.NewProjectService(store, backend)
	r := gin.New()
	Register(r.Group("/api/v1"), projSvc, update.NewService(repository.NewMemoryUpdateCatalog(store), update.WithLineDetails(store)), nil, nil, nil, nil)
	return r, projSvc, store, context.Background()
}

func enableListing(t *testing.T, svc *service.ProjectService, ctx context.Context, projectID uuid.UUID, protocol, slug string, ids map[string]string) *model.StoreListing {
	t.Helper()
	if slug == "" {
		slug = "default"
	}
	src := model.PackageSourceLineFull
	enabled := true
	in := service.StoreListingWrite{
		Protocol:      &protocol,
		Slug:          &slug,
		Enabled:       &enabled,
		PackageSource: &src,
	}
	if ids != nil {
		m := model.IdentifierMap{}
		for k, v := range ids {
			m[k] = v
		}
		in.Identifiers = &m
	}
	row, err := svc.CreateStoreListing(ctx, projectID, in)
	if err != nil {
		t.Fatalf("enable listing %s/%s: %v", protocol, slug, err)
	}
	return row
}

// forceGrayIncomplete 名册为空时发布会按 D6 立即转全量；测试夹具把它改回进行中。
func forceGrayIncomplete(t *testing.T, store repository.ProjectStore, svc *service.ProjectService, projectID uuid.UUID, versionRef string) {
	t.Helper()
	ctx := t.Context()
	v, err := svc.ResolveVersion(ctx, projectID, versionRef)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	v.GrayCompletedAt = nil
	v.GrayStartedAt = &now
	if v.GrayStartPercent >= 100 {
		v.GrayStartPercent = 50
	}
	if err := store.SaveVersion(ctx, v); err != nil {
		t.Fatal(err)
	}
}

func markProjectPrivate(t *testing.T, store repository.ProjectStore, ctx context.Context, p *model.Project) {
	t.Helper()
	p.StorageVisibility = model.StorageVisibilityPrivate
	if err := store.Save(ctx, p); err != nil {
		t.Fatal(err)
	}
}

func seedClients(t *testing.T, svc *service.ProjectService, p *model.Project, n int) {
	t.Helper()
	for i := 0; i < n; i++ {
		if _, err := svc.LoginClient(t.Context(), p, service.ClientLoginInput{
			DeviceID: fmt.Sprintf("seed-%d", i), Version: "0.9.0", OS: "windows", Arch: "x86_64",
		}); err != nil {
			t.Fatal(err)
		}
	}
}

// publishSingleFile 走完整夹具流程：建版本 → 上传产物 → 就绪 → 发布。
func publishSingleFile(t *testing.T, projSvc *service.ProjectService, ctx context.Context, projectID uuid.UUID, versionRef, channel, os, arch string, size int64, fname string, mutators ...func(*service.VersionWriteInput)) *model.Version {
	t.Helper()
	in := service.VersionWriteInput{Channel: channel, GrayStartPercent: ptr(100)}
	for _, m := range mutators {
		m(&in)
	}
	if _, _, err := projSvc.PutVersion(ctx, projectID, versionRef, in); err != nil {
		t.Fatalf("put version %s: %v", versionRef, err)
	}
	payload := bytes.Repeat([]byte("x"), int(size))
	sha, _ := hashutil.SHA256Hex(bytes.NewReader(payload))
	if _, err := projSvc.UploadArtifact(ctx, fmt.Sprint(projectID), versionRef, os, arch, service.UploadArtifactInput{
		Filename:       fname,
		ExpectedSHA256: sha,
		Size:           int64(len(payload)),
	}, bytes.NewReader(payload)); err != nil {
		t.Fatalf("upload artifact %s: %v", versionRef, err)
	}
	if _, err := projSvc.ReadyVersionLine(ctx, projectID, versionRef, os, arch); err != nil {
		t.Fatalf("ready line %s: %v", versionRef, err)
	}
	v, err := projSvc.PublishVersion(ctx, projectID, versionRef)
	if err != nil {
		t.Fatalf("publish %s: %v", versionRef, err)
	}
	return v
}

// perform 执行一次请求并返回 recorder。
func perform(r *gin.Engine, method, path string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

// splitCommaList 拆分逗号分隔的 query 值并去空白；仅测试辅助把旧 GET query 转成 POST 数组。
func splitCommaList(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if t := strings.TrimSpace(p); t != "" {
			out = append(out, t)
		}
	}
	return out
}

// performCheck 把旧 GET /update/check?query 转成 POST JSON body。
func performCheck(r http.Handler, path string) *httptest.ResponseRecorder {
	return performCheckHdr(r, path, nil)
}

func performCheckHdr(r http.Handler, path string, extra http.Header) *httptest.ResponseRecorder {
	u, err := url.Parse(path)
	if err != nil {
		panic(err)
	}
	q := u.Query()
	body := map[string]any{}
	put := func(key string) {
		if v := strings.TrimSpace(q.Get(key)); v != "" {
			body[key] = v
		}
	}
	put("current_version")
	put("os")
	put("arch")
	put("channel")
	put("hw_rev")
	put("os_version")
	put("device_id")
	if v := strings.TrimSpace(q.Get("capabilities")); v != "" {
		body["capabilities"] = splitCommaList(v)
	}
	if v := strings.TrimSpace(q.Get("accepted_delta_algos")); v != "" {
		body["accepted_delta_algos"] = splitCommaList(v)
	}
	raw, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, u.Path, bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	for k, vs := range extra {
		for _, v := range vs {
			req.Header.Add(k, v)
		}
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func changelogURL(slug, channel, os, arch string) string {
	return fmt.Sprintf("/api/v1/projects/%s/changelog/%s/%s/%s", slug, channel, os, arch)
}

// TestCheckEndpointFullSnapshot 端到端 200：响应字段快照断言（§8 全字段、
// 无 display_version、无逐文件 URL、package_url 指向 /packages/{sha256}）。
func TestCheckEndpointFullSnapshot(t *testing.T) {
	r, projSvc, _, ctx := setupCheckTest(t)

	slug := "demo"
	p, _, err := projSvc.Create(ctx, service.CreateProjectInput{DefaultLocale: ptr("en"), Slug: &slug})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := projSvc.CreateMatrix(ctx, p.ID, service.MatrixWrite{
		OS: ptr("windows"), Arch: ptr("x86_64"), PackageType: ptr(model.PackageTypeSingleFile),
	}); err != nil {
		t.Fatal(err)
	}
	setInt := func(n int64) func(*service.VersionWriteInput) {
		return func(in *service.VersionWriteInput) { in.VersionInteger = ptr(n) }
	}
	_ = publishSingleFile(t, projSvc, ctx, p.ID, "1.0.0", "stable", "windows", "x86_64", 100, "a-1.0.0.zip", setInt(10))
	_ = publishSingleFile(t, projSvc, ctx, p.ID, "1.1.0", "stable", "windows", "x86_64", 200, "a-1.1.0.zip", setInt(11))
	w := performCheck(r, "/api/v1/projects/demo/update/check?current_version=1.0.0&os=windows&arch=x86_64")
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	// 必备响应头。
	if w.Header().Get("ETag") == "" || !strings.HasPrefix(w.Header().Get("ETag"), `"`) {
		t.Fatalf("missing strong ETag: %q", w.Header().Get("ETag"))
	}
	if w.Header().Get("Cache-Control") != "private, max-age=0, must-revalidate" {
		t.Fatalf("cache-control = %q", w.Header().Get("Cache-Control"))
	}
	if !strings.Contains(w.Header().Get("Vary"), "Accept-Encoding") {
		t.Fatalf("vary = %q", w.Header().Get("Vary"))
	}
	if w.Header().Get("X-KiriVers-Protocol") != "" {
		t.Fatalf("protocol header must be absent: %q", w.Header().Get("X-KiriVers-Protocol"))
	}

	var body map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if _, ok := body["server_protocol"]; ok {
		t.Fatalf("body must not contain server_protocol")
	}
	// 必备字段。
	for _, key := range []string{
		"has_update", "is_mandatory", "is_downgrade", "reason", "compare_engine",
		"version_integer", "version_semver", "target_channel", "target_hw_rev", "package_type",
		"root_hash", "package_url", "size", "sha256", "delta_available",
	} {
		if _, ok := body[key]; !ok {
			t.Fatalf("missing field %q in body: %v", key, body)
		}
	}
	// 不存在的字段：display_version 与逐文件 URL 数组。
	if _, ok := body["display_version"]; ok {
		t.Fatalf("body must not contain display_version")
	}
	if _, ok := body["files"]; ok {
		t.Fatalf("multi-file check must not return per-file url array")
	}
	if _, ok := body["changelog"]; ok {
		t.Fatalf("check must not contain changelog")
	}
	if _, ok := body["changelog_versions"]; ok {
		t.Fatalf("check must not contain changelog_versions")
	}
	// 双号与 reason。
	if body["version_integer"].(float64) != 11 || body["version_semver"] != "1.1.0" {
		t.Fatalf("dual numbers wrong: %v %v", body["version_integer"], body["version_semver"])
	}
	if body["reason"] != "normal" || body["target_channel"] != "stable" || body["target_hw_rev"] != nil {
		t.Fatalf("target fields wrong: %v", body)
	}
	if body["package_type"] != model.PackageTypeSingleFile {
		t.Fatalf("package_type = %v", body["package_type"])
	}
	// package_url 指向 /packages/{sha256} 且真实可下载。
	pkgURL, _ := body["package_url"].(string)
	if !strings.HasPrefix(pkgURL, "/api/v1/projects/demo/packages/") {
		t.Fatalf("package_url = %q", pkgURL)
	}
	sha, _ := body["sha256"].(string)
	if sha == "" || !strings.HasSuffix(pkgURL, sha) {
		t.Fatalf("package_url must end with sha256: url=%q sha=%q", pkgURL, sha)
	}
	fn, _ := body["file_name"].(string)
	if fn == "" {
		t.Fatal("file_name must be present")
	}
	wPkg := perform(r, http.MethodGet, pkgURL)
	if wPkg.Code != http.StatusOK {
		t.Fatalf("package_url not downloadable: %d", wPkg.Code)
	}
	if body["size"].(float64) != 200 {
		t.Fatalf("size = %v", body["size"])
	}
	if _, ok := body["hash_tree_url"]; ok {
		t.Fatalf("check must not contain hash_tree_url")
	}
	if _, ok := body["diff_url"]; ok {
		t.Fatalf("check must not contain diff_url")
	}
}

// 204 无更新 + If-None-Match 304（200 与 204 两种底态）。
func TestCheckEndpoint204And304(t *testing.T) {
	r, projSvc, _, ctx := setupCheckTest(t)
	slug := "demo-204"
	p, _, err := projSvc.Create(ctx, service.CreateProjectInput{DefaultLocale: ptr("en"), Slug: &slug})
	if err != nil {
		t.Fatal(err)
	}
	publishSingleFile(t, projSvc, ctx, p.ID, "1.0.0", "stable", "windows", "x86_64", 100, "a.zip")
	publishSingleFile(t, projSvc, ctx, p.ID, "1.1.0", "stable", "windows", "x86_64", 110, "a-1.1.0.zip")

	// 200 底态拿 ETag。
	w200 := performCheck(r, "/api/v1/projects/demo-204/update/check?current_version=1.0.0&os=windows&arch=x86_64")
	if w200.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w200.Code)
	}
	etag := w200.Header().Get("ETag")

	// 当前已是最新 → 204，仍带 ETag 与 Cache-Control。
	w204 := performCheck(r, "/api/v1/projects/demo-204/update/check?current_version=1.1.0&os=windows&arch=x86_64")
	if w204.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d: %s", w204.Code, w204.Body.String())
	}
	if w204.Header().Get("ETag") == "" || w204.Header().Get("Cache-Control") == "" {
		t.Fatalf("204 must carry ETag and Cache-Control")
	}

	// 200 底态 If-None-Match → 304。
	wr := performCheckHdr(r, "/api/v1/projects/demo-204/update/check?current_version=1.0.0&os=windows&arch=x86_64", http.Header{"If-None-Match": []string{etag}})
	if wr.Code != http.StatusNotModified {
		t.Fatalf("expected 304 on 200-state, got %d", wr.Code)
	}
	if wr.Header().Get("ETag") != etag {
		t.Fatalf("304 must return same ETag")
	}

	// 204 底态 If-None-Match → 同样 304。
	wr2 := performCheckHdr(r, "/api/v1/projects/demo-204/update/check?current_version=1.1.0&os=windows&arch=x86_64", http.Header{"If-None-Match": []string{w204.Header().Get("ETag")}})
	if wr2.Code != http.StatusNotModified {
		t.Fatalf("expected 304 on 204-state, got %d", wr2.Code)
	}

	// ETag 不匹配 → 200。
	wr3 := performCheckHdr(r, "/api/v1/projects/demo-204/update/check?current_version=1.0.0&os=windows&arch=x86_64", http.Header{"If-None-Match": []string{`"stale-etag"`}})
	if wr3.Code != http.StatusOK {
		t.Fatalf("stale If-None-Match must return 200, got %d", wr3.Code)
	}
}

// 错误路径：404 VERSION_NOT_FOUND / 409 NO_SAFE_TARGET / 409 MIN_OS_NOT_MET /
// 400 INVALID_QUERY_PARAM（含禁入参数）。剩余 protocol_version 忽略，不 426。
func TestCheckEndpointErrors(t *testing.T) {
	r, projSvc, _, ctx := setupCheckTest(t)
	slug := "demo-err"
	p, _, err := projSvc.Create(ctx, service.CreateProjectInput{DefaultLocale: ptr("en"), Slug: &slug})
	if err != nil {
		t.Fatal(err)
	}
	publishSingleFile(t, projSvc, ctx, p.ID, "1.0.0", "stable", "windows", "x86_64", 100, "a.zip")

	checkURL := func(extra string) string {
		return "/api/v1/projects/demo-err/update/check?current_version=1.0.0&os=windows&arch=x86_64" + extra
	}

	// 当前已是最新 → 204（基线）。
	w := performCheck(r, checkURL(""))
	if w.Code != http.StatusNoContent {
		t.Fatalf("baseline check failed: %d", w.Code)
	}
	// 剩余 protocol_version=0：忽略，不 426；无协议头。
	w = performCheck(r, checkURL("&protocol_version=0"))
	if w.Code != http.StatusNoContent {
		t.Fatalf("leftover protocol_version must be ignored, got %d %s", w.Code, w.Body.String())
	}
	if w.Header().Get("X-KiriVers-Protocol") != "" {
		t.Fatalf("protocol header must be absent on leftover query: %q", w.Header().Get("X-KiriVers-Protocol"))
	}
	w = performCheck(r, strings.Replace(checkURL(""), "current_version=1.0.0", "current_version=99", 1))
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
	assertErrorCode(t, w, "VERSION_NOT_FOUND")

	// 缺必填参数 → 400。
	w = performCheck(r, "/api/v1/projects/demo-err/update/check?os=windows&arch=x86_64")
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for missing current_version, got %d", w.Code)
	}
	assertErrorCode(t, w, "INVALID_QUERY_PARAM")

	// 旧 GET 路径 404（无 301）。
	w = perform(r, http.MethodGet, checkURL("&local_sha256=abc"))
	if w.Code != http.StatusNotFound {
		t.Fatalf("GET check must 404, got %d %s", w.Code, w.Body.String())
	}

	// 409 NO_SAFE_TARGET：吊销全部版本。
	if _, err := projSvc.RevokeVersion(ctx, p.ID, "1.0.0"); err != nil {
		t.Fatal(err)
	}
	w = performCheck(r, checkURL(""))
	if w.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d", w.Code)
	}
	assertErrorCode(t, w, "NO_SAFE_TARGET")
	// 409 必须 private no-store。
	if w.Header().Get("Cache-Control") != "private, no-store" {
		t.Fatalf("409 cache-control = %q", w.Header().Get("Cache-Control"))
	}

	// 409 MIN_OS_NOT_MET：当前版本自身 min_os 未满足且带 os_version。
	slugMin := "demo-minos"
	pMin, _, err := projSvc.Create(ctx, service.CreateProjectInput{DefaultLocale: ptr("en"), Slug: &slugMin})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := projSvc.CreateMatrix(ctx, pMin.ID, service.MatrixWrite{
		OS: ptr("windows"), Arch: ptr("x86_64"), PackageType: ptr(model.PackageTypeSingleFile),
	}); err != nil {
		t.Fatal(err)
	}
	publishSingleFile(t, projSvc, ctx, pMin.ID, "2.0.0", "stable", "windows", "x86_64", 100, "c.zip")
	minOS := "10.0"
	if _, err := projSvc.PatchVersionLine(ctx, pMin.ID, "2.0.0", "windows", "x86_64", service.VersionLinePatchInput{
		MinOSSet: true, MinOS: &minOS,
	}); err != nil {
		t.Fatal(err)
	}
	w = performCheck(r, "/api/v1/projects/demo-minos/update/check?current_version=2.0.0&os=windows&arch=x86_64&os_version=6.1")
	if w.Code != http.StatusConflict {
		t.Fatalf("expected 409 MIN_OS_NOT_MET, got %d", w.Code)
	}
	assertErrorCode(t, w, "MIN_OS_NOT_MET")
}

// AC11：错误/缺失 X-Channel-Token 不得 403；跳过 Token 渠道后仍可回到公开渠道。
func TestCheckEndpointTokenFallbackNo403(t *testing.T) {
	r, projSvc, _, ctx := setupCheckTest(t)
	slug := "tok-check"
	p, _, err := projSvc.Create(ctx, service.CreateProjectInput{DefaultLocale: ptr("en"), Slug: &slug})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := projSvc.CreateMatrix(ctx, p.ID, service.MatrixWrite{
		OS: ptr("windows"), Arch: ptr("x86_64"), PackageType: ptr(model.PackageTypeSingleFile),
	}); err != nil {
		t.Fatal(err)
	}
	name := "Insider"
	token := "s3cret"
	rank := 30
	if _, err := projSvc.CreateChannel(ctx, p.ID, service.ChannelWrite{
		Name: &name, Slug: ptr("insider"), StabilityRank: &rank, Token: &token,
	}); err != nil {
		t.Fatal(err)
	}
	setInt := func(n int64) func(*service.VersionWriteInput) {
		return func(in *service.VersionWriteInput) { in.VersionInteger = ptr(n) }
	}
	_ = publishSingleFile(t, projSvc, ctx, p.ID, "1.0.0", "stable", "windows", "x86_64", 100, "t-1.0.0.zip", setInt(10))
	_ = publishSingleFile(t, projSvc, ctx, p.ID, "2.0.0", "stable", "windows", "x86_64", 200, "t-2.0.0.zip", setInt(20))
	_ = publishSingleFile(t, projSvc, ctx, p.ID, "3.0.0", "insider", "windows", "x86_64", 300, "t-3.0.0.zip", setInt(30))

	url := "/api/v1/projects/tok-check/update/check?current_version=1.0.0&os=windows&arch=x86_64"
	w := performCheck(r, url)
	if w.Code == http.StatusForbidden {
		t.Fatalf("missing channel token must not 403: %s", w.Body.String())
	}
	if w.Code != http.StatusOK {
		t.Fatalf("missing token expected public 2.0.0, got %d %s", w.Code, w.Body.String())
	}
	assertCheckSemver(t, w, "2.0.0")

	w = performCheckHdr(r, url, http.Header{"X-Channel-Token": []string{"wrong"}})
	if w.Code == http.StatusForbidden {
		t.Fatalf("wrong channel token must not 403: %s", w.Body.String())
	}
	if w.Code != http.StatusOK {
		t.Fatalf("wrong token expected public 2.0.0, got %d %s", w.Code, w.Body.String())
	}
	assertCheckSemver(t, w, "2.0.0")

	w = performCheckHdr(r, url, http.Header{"X-Channel-Token": []string{"s3cret"}})
	if w.Code != http.StatusOK {
		t.Fatalf("matching token expected 3.0.0, got %d %s", w.Code, w.Body.String())
	}
	assertCheckSemver(t, w, "3.0.0")
}

func assertCheckSemver(t *testing.T, w *httptest.ResponseRecorder, want string) {
	t.Helper()
	var body map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("body: %s", w.Body.String())
	}
	if body["version_semver"] != want {
		t.Fatalf("version_semver=%v want %s body=%s", body["version_semver"], want, w.Body.String())
	}
}

func assertErrorCode(t *testing.T, w *httptest.ResponseRecorder, code string) {
	t.Helper()
	var body struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("invalid error body: %s", w.Body.String())
	}
	if body.Error.Code != code {
		t.Fatalf("error code=%q want %q body=%s", body.Error.Code, code, w.Body.String())
	}
}

// 缓存头分支（验收：灰度未 100% 带 device_id 的 Cache-Control 不含 s-maxage）。
func TestCheckEndpointCacheHeadersWithGray(t *testing.T) {
	r, projSvc, _, ctx := setupCheckTest(t)
	slug := "demo-gray"
	p, _, err := projSvc.Create(ctx, service.CreateProjectInput{DefaultLocale: ptr("en"), Slug: &slug})
	if err != nil {
		t.Fatal(err)
	}
	seedClients(t, projSvc, p, 2)
	publishSingleFile(t, projSvc, ctx, p.ID, "1.0.0", "stable", "windows", "x86_64", 100, "a.zip")

	gray := 50
	if _, _, err := projSvc.PutVersion(ctx, p.ID, "1.1.0", service.VersionWriteInput{Channel: "stable", GrayStartPercent: &gray}); err != nil {
		t.Fatal(err)
	}
	payload := bytes.Repeat([]byte("y"), 120)
	sha, _ := hashutil.SHA256Hex(bytes.NewReader(payload))
	if _, err := projSvc.UploadArtifact(ctx, "demo-gray", "1.1.0", "windows", "x86_64", service.UploadArtifactInput{
		Filename: "b.zip", ExpectedSHA256: sha, Size: int64(len(payload)),
	}, bytes.NewReader(payload)); err != nil {
		t.Fatal(err)
	}
	if _, err := projSvc.ReadyVersionLine(ctx, p.ID, "1.1.0", "windows", "x86_64"); err != nil {
		t.Fatal(err)
	}
	if _, err := projSvc.PublishVersion(ctx, p.ID, "1.1.0"); err != nil {
		t.Fatal(err)
	}

	w := performCheck(r, "/api/v1/projects/demo-gray/update/check?current_version=1.0.0&os=windows&arch=x86_64")
	if w.Header().Get("Cache-Control") != "private, max-age=0, must-revalidate" {
		t.Fatalf("anonymous incomplete gray cache-control = %q", w.Header().Get("Cache-Control"))
	}

	w = performCheck(r, "/api/v1/projects/demo-gray/update/check?current_version=1.0.0&os=windows&arch=x86_64&device_id=dev-1")
	if w.Header().Get("Cache-Control") != "private, max-age=0, must-revalidate" {
		t.Fatalf("device cache-control = %q", w.Header().Get("Cache-Control"))
	}
	if strings.Contains(w.Header().Get("Cache-Control"), "s-maxage") {
		t.Fatalf("s-maxage must not appear for gray device responses")
	}
}

// windows-only Version 不成 android 目标（集成层回归）。
func TestCheckEndpointPlatformIsolation(t *testing.T) {
	r, projSvc, _, ctx := setupCheckTest(t)
	slug := "demo-multi"
	p, _, err := projSvc.Create(ctx, service.CreateProjectInput{DefaultLocale: ptr("en"), Slug: &slug})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := projSvc.CreateMatrix(ctx, p.ID, service.MatrixWrite{
		OS: ptr("android"), Arch: ptr("arm64"), PackageType: ptr(model.PackageTypeSingleFile),
	}); err != nil {
		t.Fatal(err)
	}
	// android 1.0.0（当前）；windows 1.1.0（对 android 不可见）。
	publishSingleFile(t, projSvc, ctx, p.ID, "1.0.0", "stable", "android", "arm64", 100, "app-1.0.0.apk")
	publishSingleFile(t, projSvc, ctx, p.ID, "1.1.0", "stable", "windows", "x86_64", 100, "a-1.1.0.zip")
	publishSingleFile(t, projSvc, ctx, p.ID, "1.2.0", "stable", "android", "arm64", 100, "app-1.2.0.apk")

	w := performCheck(r, "/api/v1/projects/demo-multi/update/check?current_version=1.0.0&os=android&arch=arm64")
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var body map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &body)
	if body["version_semver"] != "1.2.0" {
		t.Fatalf("android must skip windows-only 1.1.0, got %v", body["version_semver"])
	}
}

// 独立 changelog 端点：参数校验、分条、ETag/304、公开缓存头。
func TestChangelogEndpoint(t *testing.T) {
	r, projSvc, _, ctx := setupCheckTest(t)
	slug := "demo-log"
	p, _, err := projSvc.Create(ctx, service.CreateProjectInput{DefaultLocale: ptr("en"), Slug: &slug})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := projSvc.CreateMatrix(ctx, p.ID, service.MatrixWrite{
		OS: ptr("windows"), Arch: ptr("x86_64"), PackageType: ptr(model.PackageTypeSingleFile),
	}); err != nil {
		t.Fatal(err)
	}
	// 1.0.0 / 1.1.0(windows) / 1.2.0(macos only)。
	publishSingleFile(t, projSvc, ctx, p.ID, "1.0.0", "stable", "windows", "x86_64", 100, "a.zip")
	publishSingleFile(t, projSvc, ctx, p.ID, "1.1.0", "stable", "windows", "x86_64", 100, "b.zip")
	publishSingleFile(t, projSvc, ctx, p.ID, "1.2.0", "stable", "macos", "x86_64", 100, "c.zip")

	base := changelogURL("demo-log", "stable", "windows", "x86_64")

	// 无路径 changelog 404。
	w := perform(r, http.MethodGet, "/api/v1/projects/demo-log/changelog")
	if w.Code != http.StatusNotFound {
		t.Fatalf("pathless changelog must 404, got %d: %s", w.Code, w.Body.String())
	}

	// from_version 缺失 → 200 默认条（本夹具最新 1.1.0，windows 线）。
	w = perform(r, http.MethodGet, base+"?changelog_layout=structured")
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 without from_version, got %d: %s", w.Code, w.Body.String())
	}

	// 非法 scope → 400。
	w = perform(r, http.MethodGet, base+"?from_version=1.0.0&changelog_scope=bogus")
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for bad scope, got %d", w.Code)
	}
	assertErrorCode(t, w, "CHANGELOG_QUERY_INVALID")

	// 正常查询：range_platform 过滤 macos-only 版本。
	w = perform(r, http.MethodGet, base+"?from_version=1.0.0&changelog_scope=range_platform&changelog_layout=structured")
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if w.Header().Get("Cache-Control") != "public, s-maxage=60, stale-while-revalidate=30" {
		t.Fatalf("changelog cache-control = %q", w.Header().Get("Cache-Control"))
	}
	if w.Header().Get("X-KiriVers-Protocol") != "" {
		t.Fatalf("changelog protocol header must be absent: %q", w.Header().Get("X-KiriVers-Protocol"))
	}
	var body struct {
		ChangelogVersions []struct {
			VersionInteger                *int64 `json:"version_integer"`
			VersionSemver                 string `json:"version_semver"`
			Channel                       string `json:"channel"`
			Status                        string `json:"status"`
			HadArtifactForRequestPlatform bool   `json:"had_artifact_for_request_platform"`
		} `json:"changelog_versions"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if len(body.ChangelogVersions) != 1 || body.ChangelogVersions[0].VersionSemver != "1.1.0" {
		t.Fatalf("range_platform must contain only windows-ready 1.1.0: %+v", body.ChangelogVersions)
	}
	if !body.ChangelogVersions[0].HadArtifactForRequestPlatform || body.ChangelogVersions[0].Channel != "stable" || body.ChangelogVersions[0].Status != "published" {
		t.Fatalf("entry fields wrong: %+v", body.ChangelogVersions[0])
	}

	// ETag → 304。
	etag := w.Header().Get("ETag")
	if etag == "" {
		t.Fatalf("changelog must return ETag")
	}
	req304 := httptest.NewRequest(http.MethodGet, base+"?from_version=1.0.0&changelog_scope=range_platform&changelog_layout=structured", nil)
	req304.Header.Set("If-None-Match", etag)
	wr := httptest.NewRecorder()
	r.ServeHTTP(wr, req304)
	if wr.Code != http.StatusNotModified {
		t.Fatalf("expected 304, got %d", wr.Code)
	}

	wNight := perform(r, http.MethodGet, changelogURL("demo-log", "nightly", "windows", "x86_64")+"?changelog_layout=structured")
	if wNight.Code != http.StatusBadRequest {
		t.Fatalf("unknown channel must 400, got %d %s", wNight.Code, wNight.Body.String())
	}
	assertErrorCode(t, wNight, "INVALID_QUERY_PARAM")
}

// 项目级缓存配置 cache_s_maxage_seconds 生效（管理 PATCH → check 头）。
func TestCheckEndpointCustomSMaxage(t *testing.T) {
	r, projSvc, _, ctx := setupCheckTest(t)
	slug := "demo-cache"
	p, _, err := projSvc.Create(ctx, service.CreateProjectInput{DefaultLocale: ptr("en"), Slug: &slug})
	if err != nil {
		t.Fatal(err)
	}
	publishSingleFile(t, projSvc, ctx, p.ID, "1.0.0", "stable", "windows", "x86_64", 100, "a.zip")

	w := performCheck(r, "/api/v1/projects/demo-cache/update/check?current_version=1.0.0&os=windows&arch=x86_64")
	if w.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", w.Code)
	}
	if w.Header().Get("Cache-Control") != "private, max-age=0, must-revalidate" {
		t.Fatalf("default check cache-control: %q", w.Header().Get("Cache-Control"))
	}

	custom := 300
	if _, _, err := projSvc.Patch(ctx, "demo-cache", service.PatchProjectInput{CacheSMaxageSeconds: &custom}); err != nil {
		t.Fatal(err)
	}
	w = performCheck(r, "/api/v1/projects/demo-cache/update/check?current_version=1.0.0&os=windows&arch=x86_64")
	if w.Header().Get("Cache-Control") != "private, max-age=0, must-revalidate" {
		t.Fatalf("check must not use public s-maxage: %q", w.Header().Get("Cache-Control"))
	}
	clog := perform(r, http.MethodGet, changelogURL("demo-cache", "stable", "windows", "x86_64")+"?changelog_layout=structured")
	if clog.Header().Get("Cache-Control") != "public, s-maxage=300, stale-while-revalidate=30" {
		t.Fatalf("changelog custom s-maxage ignored: %q", clog.Header().Get("Cache-Control"))
	}
}

func TestCheckAndChangelogExpandSiteURL(t *testing.T) {
	r, projSvc, _, ctx := setupCheckTest(t)
	slug := "demo-md"
	p, _, err := projSvc.Create(ctx, service.CreateProjectInput{DefaultLocale: ptr("en"), Slug: &slug})
	if err != nil {
		t.Fatal(err)
	}
	md := "shot ${site_url}/api/v1/projects/demo-md/media/" + uuid.Must(uuid.NewRandom()).String()
	_ = publishSingleFile(t, projSvc, ctx, p.ID, "1.0.0", "stable", "windows", "x86_64", 100, "a.zip")
	_ = publishSingleFile(t, projSvc, ctx, p.ID, "1.1.0", "stable", "windows", "x86_64", 120, "b.zip", func(in *service.VersionWriteInput) {
		in.ChangelogI18n = model.ChangelogMap{"en": {Markdown: md}}
	})

	w := performCheckHdr(r, "/api/v1/projects/demo-md/update/check?current_version=1.0.0&os=windows&arch=x86_64", http.Header{"Referer": []string{"https://app.example/path"}})
	if w.Code != http.StatusOK {
		t.Fatalf("check=%d %s", w.Code, w.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if _, ok := body["changelog"]; ok {
		t.Fatalf("check 200 must omit changelog: %s", w.Body.String())
	}
	if _, ok := body["changelog_versions"]; ok {
		t.Fatalf("check 200 must omit changelog_versions: %s", w.Body.String())
	}
	if strings.Contains(w.Header().Get("Vary"), "Referer") {
		t.Fatalf("check vary must not include Referer: %q", w.Header().Get("Vary"))
	}
	etag := w.Header().Get("ETag")

	wScope := perform(r, http.MethodGet, "/api/v1/projects/demo-md/update/check?current_version=1.0.0&os=windows&arch=x86_64&changelog_scope=range_all")
	if wScope.Code != http.StatusNotFound {
		t.Fatalf("GET check must 404: %d %s", wScope.Code, wScope.Body.String())
	}
	wLoc := perform(r, http.MethodGet, "/api/v1/projects/demo-md/update/check?current_version=1.0.0&os=windows&arch=x86_64&changelog_locale=en")
	if wLoc.Code != http.StatusNotFound {
		t.Fatalf("GET check must 404: %d %s", wLoc.Code, wLoc.Body.String())
	}
	wLeftover := performCheck(r, "/api/v1/projects/demo-md/update/check?current_version=1.0.0&os=windows&arch=x86_64&locale=en")
	if wLeftover.Code != http.StatusOK {
		t.Fatalf("leftover locale must be ignored: %d %s", wLeftover.Code, wLeftover.Body.String())
	}

	req2 := httptest.NewRequest(http.MethodGet, changelogURL("demo-md", "stable", "windows", "x86_64")+"?from_version=1.0.0&changelog_locale=en", nil)
	req2.Header.Set("Referer", "https://app.example/x")
	cw := httptest.NewRecorder()
	r.ServeHTTP(cw, req2)
	if cw.Code != http.StatusOK || !strings.Contains(cw.Body.String(), "http://") {
		t.Fatalf("changelog=%d %s", cw.Code, cw.Body.String())
	}
	if strings.Contains(cw.Header().Get("Vary"), "Referer") {
		t.Fatalf("changelog vary must not include Referer: %q", cw.Header().Get("Vary"))
	}
	if strings.Contains(cw.Body.String(), "https://app.example/api/v1/projects/") {
		t.Fatalf("changelog must expand from RequestOrigin not Referer: %s", cw.Body.String())
	}

	w3 := performCheckHdr(r, "/api/v1/projects/demo-md/update/check?current_version=1.0.0&os=windows&arch=x86_64", http.Header{"Referer": []string{"https://other.example/"}})
	if w3.Header().Get("ETag") != etag {
		t.Fatalf("etag flipped by referer")
	}

	req5 := httptest.NewRequest(http.MethodGet, "http://api.test"+changelogURL("demo-md", "stable", "windows", "x86_64")+"?from_version=1.0.0&changelog_locale=en", nil)
	w5 := httptest.NewRecorder()
	r.ServeHTTP(w5, req5)
	if w5.Code != http.StatusOK || !strings.Contains(w5.Body.String(), "http://api.test/api/v1/projects/") {
		t.Fatalf("changelog origin expansion=%d %s", w5.Code, w5.Body.String())
	}
}

func changelogVersions(t *testing.T, w *httptest.ResponseRecorder) []string {
	t.Helper()
	var body struct {
		ChangelogVersions []struct {
			VersionSemver string `json:"version_semver"`
		} `json:"changelog_versions"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("changelog body: %s", w.Body.String())
	}
	out := make([]string, 0, len(body.ChangelogVersions))
	for _, e := range body.ChangelogVersions {
		out = append(out, e.VersionSemver)
	}
	return out
}

// 无 from_version 取最新 default_entries（产品缺省 5）；有 from 截到 max 仍 200。
func TestChangelogDefaultEntriesAndTruncate(t *testing.T) {
	r, projSvc, store, ctx := setupCheckTest(t)
	slug := "demo-log-n"
	p, _, err := projSvc.Create(ctx, service.CreateProjectInput{DefaultLocale: ptr("en"), Slug: &slug})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := projSvc.CreateMatrix(ctx, p.ID, service.MatrixWrite{
		OS: ptr("windows"), Arch: ptr("x86_64"), PackageType: ptr(model.PackageTypeSingleFile),
	}); err != nil {
		t.Fatal(err)
	}
	for i := 0; i <= 6; i++ {
		ver := fmt.Sprintf("1.%d.0", i)
		publishSingleFile(t, projSvc, ctx, p.ID, ver, "stable", "windows", "x86_64", 50, ver+".zip")
	}

	base := changelogURL(slug, "stable", "windows", "x86_64")
	w := perform(r, http.MethodGet, base+"?changelog_layout=structured")
	if w.Code != http.StatusOK {
		t.Fatalf("no-from=%d %s", w.Code, w.Body.String())
	}
	got := changelogVersions(t, w)
	want := []string{"1.6.0", "1.5.0", "1.4.0", "1.3.0", "1.2.0"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("no-from versions=%v want %v", got, want)
	}

	p.ChangelogMaxEntries = 2
	if err := store.Save(ctx, p); err != nil {
		t.Fatal(err)
	}
	w = perform(r, http.MethodGet, base+"?from_version=1.0.0&changelog_layout=structured")
	if w.Code != http.StatusOK {
		t.Fatalf("truncate must stay 200, got %d %s", w.Code, w.Body.String())
	}
	got = changelogVersions(t, w)
	if strings.Join(got, ",") != "1.6.0,1.5.0" {
		t.Fatalf("truncated versions=%v", got)
	}
}

func TestChangelogTokenMismatch404(t *testing.T) {
	r, projSvc, _, ctx := setupCheckTest(t)
	slug := "tok-log"
	p, _, err := projSvc.Create(ctx, service.CreateProjectInput{DefaultLocale: ptr("en"), Slug: &slug})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := projSvc.CreateMatrix(ctx, p.ID, service.MatrixWrite{
		OS: ptr("windows"), Arch: ptr("x86_64"), PackageType: ptr(model.PackageTypeSingleFile),
	}); err != nil {
		t.Fatal(err)
	}
	token := "s3cret"
	rank := 30
	if _, err := projSvc.CreateChannel(ctx, p.ID, service.ChannelWrite{
		Name: ptr("Insider"), Slug: ptr("insider"), StabilityRank: &rank, Token: &token,
	}); err != nil {
		t.Fatal(err)
	}
	publishSingleFile(t, projSvc, ctx, p.ID, "1.0.0", "insider", "windows", "x86_64", 100, "i.zip")

	path := changelogURL(slug, "insider", "windows", "x86_64") + "?changelog_layout=structured"
	w := perform(r, http.MethodGet, path)
	if w.Code != http.StatusNotFound {
		t.Fatalf("missing token must 404 not %d: %s", w.Code, w.Body.String())
	}
	assertErrorCode(t, w, "NOT_FOUND")

	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Header.Set("X-Channel-Token", "wrong")
	wr := httptest.NewRecorder()
	r.ServeHTTP(wr, req)
	if wr.Code != http.StatusForbidden && wr.Code != http.StatusNotFound {
		t.Fatalf("wrong token unexpected %d %s", wr.Code, wr.Body.String())
	}
	if wr.Code == http.StatusForbidden {
		t.Fatalf("token mismatch must not 403: %s", wr.Body.String())
	}
	assertErrorCode(t, wr, "NOT_FOUND")

	reqOK := httptest.NewRequest(http.MethodGet, path, nil)
	reqOK.Header.Set("X-Channel-Token", "s3cret")
	wok := httptest.NewRecorder()
	r.ServeHTTP(wok, reqOK)
	if wok.Code != http.StatusOK {
		t.Fatalf("matching token=%d %s", wok.Code, wok.Body.String())
	}
}
