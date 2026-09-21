package admin

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/Kirizu-Official/KiriVers/internal/model"
	"github.com/Kirizu-Official/KiriVers/internal/repository"
	"github.com/Kirizu-Official/KiriVers/internal/service"
	"github.com/Kirizu-Official/KiriVers/internal/storage"
)

// setupWebhookHTTP 在 setupProjectHTTP 基础上为 ProjectService 附加 webhook
// 投递仓储与任务仓储，供 promote / reuse / deliveries 端点测试使用。
func setupWebhookHTTP(t *testing.T, hookHandler http.HandlerFunc) (*gin.Engine, *service.ProjectService, *repository.MemoryProjectStore, *repository.MemoryWebhookStore, string, *httptest.Server) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	adminSvc := service.NewTestAdminService(repository.NewMemoryAdminStore())
	if _, err := adminSvc.Create(t.Context(), "root", "password123"); err != nil {
		t.Fatal(err)
	}
	login, err := adminSvc.CompleteLogin(t.Context(), "root", "password123", "")
	if err != nil {
		t.Fatal(err)
	}
	store := repository.NewMemoryProjectStore()
	backend, err := storage.NewLocalFS(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	projSvc := service.NewProjectService(store, backend)
	projSvc.SetJobStore(repository.NewMemoryJobRepo())
	webhooks := repository.NewMemoryWebhookStore()
	projSvc.SetWebhookStore(webhooks)
	r := gin.New()
	Register(r.Group("/api/v1/admin"), adminSvc, projSvc, nil, nil, nil, nil, nil)

	var ts *httptest.Server
	if hookHandler != nil {
		ts = httptest.NewServer(hookHandler)
		t.Cleanup(ts.Close)
	}
	return r, projSvc, store, webhooks, login.Token, ts
}

// TestHTTPPromoteSuffixConflictThenReuseFlow 验收 2 的 HTTP 面：
// beta 1.2.3-beta.1 → promote stable 返回 400 CHANNEL_SUFFIX_MISMATCH（details 带
// 复用指引）；新建 1.2.3 stable + artifacts/reuse → 201 且引用同一对象，可发布。
func TestHTTPPromoteSuffixConflictThenReuseFlow(t *testing.T) {
	r, projSvc, _, _, token, _ := setupWebhookHTTP(t, nil)
	ctx := t.Context()

	p, _, err := projSvc.Create(ctx, service.CreateProjectInput{DefaultLocale: ptr("en"), Slug: ptr("flow-http")})
	if err != nil {
		t.Fatal(err)
	}

	// beta 1.2.3-beta.1 + 产物 + 发布（直接走 service 便于夹具）。
	if _, _, err := projSvc.PutVersion(ctx, p.ID, "1.2.3-beta.1", service.VersionWriteInput{Channel: "beta"}); err != nil {
		t.Fatal(err)
	}
	content := []byte("http-flow payload")
	src, err := projSvc.UploadArtifact(ctx, p.Slug, "1.2.3-beta.1", "windows", "x86_64", service.UploadArtifactInput{
		Filename: "app.exe",
		Size:     int64(len(content)),
	}, bytes.NewReader(content))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := projSvc.PublishVersion(ctx, p.ID, "1.2.3-beta.1"); err != nil {
		t.Fatal(err)
	}

	// promote → 400 CHANNEL_SUFFIX_MISMATCH，details 带 remedy 指引。
	w := doJSON(r, http.MethodPost, "/api/v1/admin/projects/"+p.Slug+"/versions/1.2.3-beta.1/promote", token, gin.H{
		"target_channel": "stable",
	})
	if w.Code != http.StatusBadRequest || decodeErr(t, w) != "CHANNEL_SUFFIX_MISMATCH" {
		t.Fatalf("expected 400 CHANNEL_SUFFIX_MISMATCH, got %d %s", w.Code, w.Body.String())
	}
	var env struct {
		Error struct {
			Details map[string]any `json:"details"`
		} `json:"error"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &env); err != nil {
		t.Fatal(err)
	}
	remedy, _ := env.Error.Details["remedy"].(string)
	if !strings.Contains(remedy, "artifacts/reuse") {
		t.Fatalf("mismatch details must hint the reuse path, got %q", remedy)
	}

	// 新建 1.2.3 stable → reuse → publish（HTTP）。
	wPut := doJSON(r, http.MethodPut, "/api/v1/admin/projects/"+p.Slug+"/versions/1.2.3", token, gin.H{"channel": "stable"})
	if wPut.Code != http.StatusCreated {
		t.Fatalf("put 1.2.3 failed: %d %s", wPut.Code, wPut.Body.String())
	}
	wReuse := doJSON(r, http.MethodPost, "/api/v1/admin/projects/"+p.Slug+"/versions/1.2.3/artifacts/reuse", token, gin.H{
		"os":   "windows",
		"arch": "x86_64",
		"source": gin.H{
			"artifact_id": src.ID.String(),
		},
	})
	if wReuse.Code != http.StatusCreated {
		t.Fatalf("reuse failed: %d %s", wReuse.Code, wReuse.Body.String())
	}
	var reuseRes struct {
		Artifacts []model.Artifact `json:"artifacts"`
	}
	if err := json.Unmarshal(wReuse.Body.Bytes(), &reuseRes); err != nil {
		t.Fatal(err)
	}
	if len(reuseRes.Artifacts) != 1 || reuseRes.Artifacts[0].SHA256 != src.SHA256 {
		t.Fatalf("reuse must reference the source object (same sha256): %s", wReuse.Body.String())
	}
	if reuseRes.Artifacts[0].FileName != "flow-http-1.2.3-windows-x86_64.exe" {
		t.Fatalf("unexpected recomputed file name: %s", reuseRes.Artifacts[0].FileName)
	}
	// storage_key 不经 HTTP 回显，从仓储确认零拷贝引用。
	reusedRow, err := projSvc.GetArtifactDownload(ctx, p.Slug, reuseRes.Artifacts[0].SHA256, "")
	if err != nil {
		t.Fatal(err)
	}
	if reusedRow.StorageKey != src.StorageKey {
		t.Fatalf("storage key must be referenced, not copied")
	}

	wPub := doJSON(r, http.MethodPost, "/api/v1/admin/projects/"+p.Slug+"/versions/1.2.3/publish", token, nil)
	if wPub.Code != http.StatusOK {
		t.Fatalf("publish failed: %d %s", wPub.Code, wPub.Body.String())
	}
}

// TestHTTPWebhookDeliveriesEndpoint deliveries 排障查询：签名密钥永不回显，
// 记录含 event/status/attempts。
func TestHTTPWebhookDeliveriesEndpoint(t *testing.T) {
	r, projSvc, _, webhooks, token, _ := setupWebhookHTTP(t, nil)
	ctx := t.Context()

	p, _, err := projSvc.Create(ctx, service.CreateProjectInput{DefaultLocale: ptr("en"), Slug: ptr("deliveries-http")})
	if err != nil {
		t.Fatal(err)
	}
	url := "http://127.0.0.1:1/hook" // 不会被真正调用（只查记录）
	if _, _, err := projSvc.Patch(ctx, p.Slug, service.PatchProjectInput{WebhookURL: &url}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := projSvc.PutVersion(ctx, p.ID, "1.0.0", service.VersionWriteInput{Channel: "stable"}); err != nil {
		t.Fatal(err)
	}
	body := []byte("payload")
	if _, err := projSvc.UploadArtifact(ctx, p.Slug, "1.0.0", "windows", "x86_64", service.UploadArtifactInput{
		Filename: "app.exe",
		Size:     int64(len(body)),
	}, bytes.NewReader(body)); err != nil {
		t.Fatal(err)
	}
	if _, err := projSvc.PublishVersion(ctx, p.ID, "1.0.0"); err != nil {
		t.Fatal(err)
	}

	w := doJSON(r, http.MethodGet, "/api/v1/admin/projects/"+p.Slug+"/webhook/deliveries", token, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d %s", w.Code, w.Body.String())
	}
	var res struct {
		Deliveries []map[string]any `json:"deliveries"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &res); err != nil {
		t.Fatal(err)
	}
	if len(res.Deliveries) != 1 {
		t.Fatalf("expected 1 delivery row, got %d", len(res.Deliveries))
	}
	d := res.Deliveries[0]
	if d["event"] != model.WebhookEventVersionPublished || d["status"] != model.WebhookDeliveryStatusPending {
		t.Fatalf("unexpected delivery row: %+v", d)
	}
	// 密钥永不回显。
	if raw, _ := json.Marshal(res); strings.Contains(string(raw), "webhook_secret") {
		t.Fatalf("webhook secret must never appear in deliveries response")
	}
	if webhooks == nil {
		t.Fatalf("webhook store required")
	}
}
