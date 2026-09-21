package admin

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/Kirizu-Official/KiriVers/internal/controller/client"
	"github.com/Kirizu-Official/KiriVers/internal/middleware"
	"github.com/Kirizu-Official/KiriVers/internal/model"
	"github.com/Kirizu-Official/KiriVers/internal/repository"
	"github.com/Kirizu-Official/KiriVers/internal/service"
	"github.com/Kirizu-Official/KiriVers/internal/service/update"
	"github.com/Kirizu-Official/KiriVers/internal/storage"
)

// setupTelemetryAdmin 构造带遥测服务的管理端 + 客户端引擎，返回管理员会话 Token。
func setupTelemetryAdmin(t *testing.T, limiter *middleware.Limiter) (*gin.Engine, *service.ProjectService, *repository.MemoryProjectStore, *repository.MemoryTelemetryStore, *service.TelemetryService, string) {
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
	telStore := repository.NewMemoryTelemetryStore()
	telSvc := service.NewTelemetryService(telStore)
	// 隐私删除补全（C15-4）：按哈希删除同时清除灰度白名单行。
	telSvc.SetAllowlistDeleter(store)

	r := gin.New()
	Register(r.Group("/api/v1/admin"), adminSvc, projSvc, telSvc, limiter, nil, nil, nil)
	client.Register(r.Group("/api/v1"), projSvc,
		update.NewService(repository.NewMemoryUpdateCatalog(store), update.WithLineDetails(store), update.WithDowngradeSource(telSvc)),
		telSvc, limiter, nil, nil)
	return r, projSvc, store, telStore, telSvc, login.Token
}

// TestTelemetryDeviceDeletion 按哈希删除遥测事件（§13.11 / C15-4）：200 带计数、
// 只删该哈希、未知项目 404、幂等。
func TestTelemetryDeviceDeletion(t *testing.T) {
	r, projSvc, _, telStore, telSvc, adminTok := setupTelemetryAdmin(t, nil)
	ctx := context.Background()

	slug := "del-demo"
	p, _, err := projSvc.Create(ctx, service.CreateProjectInput{DefaultLocale: ptr("en"), Slug: &slug})
	if err != nil {
		t.Fatal(err)
	}
	hash := telSvc.HashDeviceID(p, "device-x")
	// 两台设备各一条（另一条属其他项目场景，此处校验项目内隔离）。
	_ = telStore.Insert(ctx, &model.TelemetryEvent{ProjectID: p.ID, DeviceHash: hash, OS: "windows", Arch: "x86_64", Status: model.TelemetryStatusFailed})
	_ = telStore.Insert(ctx, &model.TelemetryEvent{ProjectID: p.ID, DeviceHash: "other-hash", OS: "windows", Arch: "x86_64", Status: model.TelemetryStatusFailed})
	_ = telStore.Insert(ctx, &model.TelemetryEvent{ProjectID: p.ID, DeviceHash: "third-hash", OS: "linux", Arch: "x86_64", Status: model.TelemetryStatusFailed})

	// 无凭证 → 401。
	if w := doJSON(r, http.MethodDelete, "/api/v1/admin/projects/del-demo/telemetry/devices/"+hash, "", nil); w.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated: %d", w.Code)
	}
	// 未知项目 → 404。
	if w := doJSON(r, http.MethodDelete, "/api/v1/admin/projects/nope/telemetry/devices/"+hash, adminTok, nil); w.Code != http.StatusNotFound {
		t.Fatalf("unknown project: %d", w.Code)
	}
	// 按哈希删除 → 200 幂等成功，响应体附删除计数（C15-4；204 无法携带响应体）。
	w := doJSON(r, http.MethodDelete, "/api/v1/admin/projects/del-demo/telemetry/devices/"+hash, adminTok, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("delete: %d %s", w.Code, w.Body.String())
	}
	var counts struct {
		TelemetryDeleted int64 `json:"telemetry_deleted"`
		AllowlistDeleted int64 `json:"allowlist_deleted"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &counts); err != nil {
		t.Fatal(err)
	}
	if counts.TelemetryDeleted != 1 {
		t.Fatalf("telemetry_deleted = %d, want 1", counts.TelemetryDeleted)
	}
	// 只有该哈希的事件被删除；重复删除幂等 204。
	remaining := telStore.Snapshot()
	if len(remaining) != 2 {
		t.Fatalf("expected 2 remaining events, got %d", len(remaining))
	}
	for _, e := range remaining {
		if e.DeviceHash == hash {
			t.Fatal("deleted hash must not remain")
		}
	}
	if w := doJSON(r, http.MethodDelete, "/api/v1/admin/projects/del-demo/telemetry/devices/"+hash, adminTok, nil); w.Code != http.StatusOK {
		t.Fatalf("idempotent delete: %d", w.Code)
	}
}

// TestCIRateLimitSeparateFromIP 验收：CI Token 限流键与匿名 IP 限流键不同。
// CI 维度打满后匿名 check 不受影响（同 IP、同 Limiter 实例）。
func TestCIRateLimitSeparateFromIP(t *testing.T) {
	limits := model.JSONObject{
		model.RateLimitKeyCIToken:        1,
		model.RateLimitKeyCheckPerIP:     1000,
		model.RateLimitKeyCheckPerDevice: 1000,
	}
	r, projSvc, _, _, _, adminTok := setupTelemetryAdmin(t, middleware.NewLimiter())
	ctx := context.Background()

	slug := "ci-demo"
	p, _, err := projSvc.Create(ctx, service.CreateProjectInput{DefaultLocale: ptr("en"), Slug: &slug, RateLimit: &limits})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := projSvc.CreateMatrix(ctx, p.ID, service.MatrixWrite{
		OS: ptr("windows"), Arch: ptr("x86_64"), PackageType: ptr(model.PackageTypeSingleFile),
	}); err != nil {
		t.Fatal(err)
	}

	// 发放 CI Token（artifact:write）。
	w := doJSON(r, http.MethodPost, "/api/v1/admin/projects/ci-demo/ci-tokens", adminTok, map[string]any{
		"name": "agent", "scopes": []string{model.ScopeArtifactWrite},
	})
	if w.Code != http.StatusCreated {
		t.Fatalf("create ci token: %d %s", w.Code, w.Body.String())
	}
	var issued struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &issued); err != nil || issued.Token == "" {
		t.Fatalf("ci token body=%s err=%v", w.Body.String(), err)
	}

	// 第 1 次 CI 发版请求（缺合法版本数据 → 400，但计入 CI 配额）。
	w1 := doJSON(r, http.MethodPost, "/api/v1/admin/projects/ci-demo/ci/releases", issued.Token, map[string]any{})
	if w1.Code == http.StatusTooManyRequests {
		t.Fatalf("first ci request must not be rate limited: %d", w1.Code)
	}
	// 第 2 次 → CI 维度 429。
	w2 := doJSON(r, http.MethodPost, "/api/v1/admin/projects/ci-demo/ci/releases", issued.Token, map[string]any{})
	if w2.Code != http.StatusTooManyRequests {
		t.Fatalf("second ci request: %d %s, want 429", w2.Code, w2.Body.String())
	}

	// 匿名 check（同 IP、同 Limiter 实例）不受 CI 配额影响。
	cw := doJSON(r, http.MethodPost, "/api/v1/projects/ci-demo/update/check", "", map[string]any{
		"os": "windows", "arch": "x86_64", "current_version": "0.0.1",
	})
	if cw.Code == http.StatusTooManyRequests {
		t.Fatal("anonymous check must not be affected by the exhausted CI key")
	}
}
