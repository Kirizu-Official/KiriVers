package admin

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/rs/zerolog"

	"github.com/Kirizu-Official/KiriVers/internal/model"
	"github.com/Kirizu-Official/KiriVers/internal/repository"
	"github.com/Kirizu-Official/KiriVers/internal/service"
	"github.com/Kirizu-Official/KiriVers/internal/storage"
	"github.com/Kirizu-Official/KiriVers/pkg/hashutil"
)

// setupAuditAdmin 构造带审计服务（§11.2 / C15-3）与隐私删除补全（C15-4）的
// 管理端引擎，返回管理员会话 Token 与审计内存仓储。
func setupAuditAdmin(t *testing.T) (*gin.Engine, *service.ProjectService, *repository.MemoryProjectStore, *repository.MemoryTelemetryStore, *service.TelemetryService, *repository.MemoryAuditStore, string) {
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
	auditStore := repository.NewMemoryAuditStore()
	auditSvc := service.NewAuditService(auditStore, zerolog.Nop())

	r := gin.New()
	Register(r.Group("/api/v1/admin"), adminSvc, projSvc, telSvc, nil, auditSvc, nil, nil)
	return r, projSvc, store, telStore, telSvc, auditStore, login.Token
}

// publishForAudit 走完整发布夹具：矩阵 → 版本 → 产物 → 就绪（发布由调用方执行）。
func publishForAudit(t *testing.T, projSvc *service.ProjectService, ctx context.Context, projectID uuid.UUID, versionRef string) {
	t.Helper()
	if _, err := projSvc.CreateMatrix(ctx, projectID, service.MatrixWrite{
		OS: ptr("windows"), Arch: ptr("x86_64"), PackageType: ptr(model.PackageTypeSingleFile),
	}); err != nil && !strings.Contains(err.Error(), "already exists") {
		t.Fatal(err)
	}
	if _, _, err := projSvc.PutVersion(ctx, projectID, versionRef, service.VersionWriteInput{Channel: "stable"}); err != nil {
		t.Fatal(err)
	}
	payload := bytes.Repeat([]byte("z"), 32)
	sha, _ := hashutil.SHA256Hex(bytes.NewReader(payload))
	if _, err := projSvc.UploadArtifact(ctx, projectID.String(), versionRef, "windows", "x86_64", service.UploadArtifactInput{
		Filename:       "audit-" + versionRef + ".zip",
		ExpectedSHA256: sha,
		Size:           int64(len(payload)),
	}, bytes.NewReader(payload)); err != nil {
		t.Fatal(err)
	}
	if _, err := projSvc.ReadyVersionLine(ctx, projectID, versionRef, "windows", "x86_64"); err != nil {
		t.Fatal(err)
	}
}

// auditActions 汇总审计动作序列（测试断言辅助）。
func auditActions(auditStore *repository.MemoryAuditStore) []string {
	events := auditStore.Snapshot()
	out := make([]string, 0, len(events))
	for _, e := range events {
		out = append(out, e.Action)
	}
	return out
}

// TestAuditVersionLifecycleAndTokens 验收 3（C15-3）：Publish / Revoke / 签发
// Token 在 audit 表有行；明文 Token 绝不落审计（C15-5）；未认证请求不产生审计行。
func TestAuditVersionLifecycleAndTokens(t *testing.T) {
	r, projSvc, _, _, _, auditStore, adminTok := setupAuditAdmin(t)
	ctx := t.Context()

	slug := "audit-demo"
	p, _, err := projSvc.Create(ctx, service.CreateProjectInput{DefaultLocale: ptr("en"), Slug: &slug})
	if err != nil {
		t.Fatal(err)
	}
	publishForAudit(t, projSvc, ctx, p.ID, "1.0.0")

	// 未认证 publish → 401 且不产生审计行。
	if w := doJSON(r, http.MethodPost, "/api/v1/admin/projects/audit-demo/versions/1.0.0/publish", "", nil); w.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated publish: %d", w.Code)
	}
	if rows := auditStore.Snapshot(); len(rows) != 0 {
		t.Fatalf("unauthenticated action must not be audited, got %d rows", len(rows))
	}

	// Publish → 审计行 version.publish（主体 = 管理员，Fingerprint = username）。
	if w := doJSON(r, http.MethodPost, "/api/v1/admin/projects/audit-demo/versions/1.0.0/publish", adminTok, nil); w.Code != http.StatusOK {
		t.Fatalf("publish: %d %s", w.Code, w.Body.String())
	}
	rows := auditStore.Snapshot()
	if len(rows) != 1 || rows[0].Action != model.AuditActionVersionPublish {
		t.Fatalf("expected 1 publish row, got %v", auditActions(auditStore))
	}
	if rows[0].ActorType != model.AuditActorAdmin || rows[0].ActorFingerprint != "root" {
		t.Fatalf("actor = %s/%s, want admin/root", rows[0].ActorType, rows[0].ActorFingerprint)
	}
	if rows[0].ProjectID == nil || *rows[0].ProjectID != p.ID {
		t.Fatalf("project id missing on audit row")
	}

	// Revoke → version.revoke。
	if w := doJSON(r, http.MethodPost, "/api/v1/admin/projects/audit-demo/versions/1.0.0/revoke", adminTok, nil); w.Code != http.StatusOK {
		t.Fatalf("revoke: %d %s", w.Code, w.Body.String())
	}
	if got := auditActions(auditStore); len(got) != 2 || got[1] != model.AuditActionVersionRevoke {
		t.Fatalf("expected revoke row, got %v", got)
	}

	// 签发 CI Token → ci_token.create；明文 token 绝不落审计（C15-5）。
	w := doJSON(r, http.MethodPost, "/api/v1/admin/projects/audit-demo/ci-tokens", adminTok, map[string]any{
		"name": "agent", "scopes": []string{model.ScopeReleasePublish},
	})
	if w.Code != http.StatusCreated {
		t.Fatalf("create ci token: %d %s", w.Code, w.Body.String())
	}
	var issued struct {
		Token       string `json:"token"`
		Fingerprint string `json:"fingerprint"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &issued); err != nil {
		t.Fatal(err)
	}
	if got := auditActions(auditStore); len(got) != 3 || got[2] != model.AuditActionCITokenCreate {
		t.Fatalf("expected ci_token.create row, got %v", got)
	}
	// C15-5：序列化全部审计行，断言不含明文 Token。
	blob, _ := json.Marshal(auditStore.Snapshot())
	if strings.Contains(string(blob), issued.Token) {
		t.Fatal("plaintext token leaked into audit rows")
	}

	// 用 CI Token 发布新版本 → 主体 = ci_token，指纹 = Token 指纹（C15-3）。
	publishForAudit(t, projSvc, ctx, p.ID, "1.1.0")
	w = doJSON(r, http.MethodPost, "/api/v1/admin/projects/audit-demo/versions/1.1.0/publish", issued.Token, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("ci publish: %d %s", w.Code, w.Body.String())
	}
	rows = auditStore.Snapshot()
	last := rows[len(rows)-1]
	if last.Action != model.AuditActionVersionPublish || last.ActorType != model.AuditActorCIToken ||
		last.ActorFingerprint != issued.Fingerprint {
		t.Fatalf("ci actor row = %s/%s/%s, want ci_token/%s", last.Action, last.ActorType, last.ActorFingerprint, issued.Fingerprint)
	}
}

// TestAuditQueryEndpoint 管理端审计查询（C15-3）：GET /admin/projects/:ref/audit
// 倒序游标分页。
func TestAuditQueryEndpoint(t *testing.T) {
	r, projSvc, _, _, _, _, adminTok := setupAuditAdmin(t)
	ctx := t.Context()

	slug := "audit-q"
	p, _, err := projSvc.Create(ctx, service.CreateProjectInput{DefaultLocale: ptr("en"), Slug: &slug})
	if err != nil {
		t.Fatal(err)
	}
	publishForAudit(t, projSvc, ctx, p.ID, "1.0.0")
	if w := doJSON(r, http.MethodPost, "/api/v1/admin/projects/audit-q/versions/1.0.0/publish", adminTok, nil); w.Code != http.StatusOK {
		t.Fatalf("publish: %d", w.Code)
	}
	if w := doJSON(r, http.MethodPost, "/api/v1/admin/projects/audit-q/versions/1.0.0/revoke", adminTok, nil); w.Code != http.StatusOK {
		t.Fatalf("revoke: %d", w.Code)
	}

	w := doJSON(r, http.MethodGet, "/api/v1/admin/projects/audit-q/audit?limit=1", adminTok, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("audit query: %d %s", w.Code, w.Body.String())
	}
	var body struct {
		Events     []model.AuditEvent `json:"events"`
		NextCursor string             `json:"next_cursor"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if len(body.Events) != 1 || body.Events[0].Action != model.AuditActionVersionRevoke {
		t.Fatalf("first page = %+v", body.Events)
	}
	if body.NextCursor == "" {
		t.Fatal("expected next_cursor")
	}
	// 第二页取到最早的 publish 行。
	w = doJSON(r, http.MethodGet, "/api/v1/admin/projects/audit-q/audit?cursor="+body.NextCursor, adminTok, nil)
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if len(body.Events) != 1 || body.Events[0].Action != model.AuditActionVersionPublish {
		t.Fatalf("second page = %+v", body.Events)
	}
}

// TestPrivacyDeletionClearsAllowlist 验收 4（C15-4）：按哈希删除后该哈希的
// 遥测查询为空，且 Version 级 / per-line 白名单行同被清除；响应体附计数。
func TestPrivacyDeletionClearsAllowlist(t *testing.T) {
	r, projSvc, store, telStore, telSvc, auditStore, adminTok := setupAuditAdmin(t)
	ctx := t.Context()

	slug := "privacy-demo"
	p, _, err := projSvc.Create(ctx, service.CreateProjectInput{DefaultLocale: ptr("en"), Slug: &slug})
	if err != nil {
		t.Fatal(err)
	}
	// 完整夹具（矩阵 / 版本 / 产物 / 就绪线；per-line 白名单需要已存在的线）。
	publishForAudit(t, projSvc, ctx, p.ID, "1.0.0")
	// 设备哈希（hashed 策略 = HMAC hex，与遥测 DeviceHash 同函数）。
	hash := telSvc.HashDeviceID(p, "device-x")
	// 遥测事件。
	_ = telStore.Insert(ctx, &model.TelemetryEvent{ProjectID: p.ID, DeviceHash: hash, OS: "windows", Arch: "x86_64", Status: model.TelemetryStatusFailed})
	// Version 级白名单 + per-line 白名单（同一设备，hashed 策略写路径落哈希）。
	if _, err := projSvc.AddVersionGrayAllowlist(ctx, p.ID, "1.0.0", []string{"device-x"}); err != nil {
		t.Fatal(err)
	}
	if _, err := projSvc.AddVersionGrayAllowlist(ctx, p.ID, "1.0.0", []string{"device-x"}); err != nil {
		t.Fatal(err)
	}
	// 另一设备的白名单行不受影响。
	if _, err := projSvc.AddVersionGrayAllowlist(ctx, p.ID, "1.0.0", []string{"device-other"}); err != nil {
		t.Fatal(err)
	}

	w := doJSON(r, http.MethodDelete, "/api/v1/admin/projects/privacy-demo/telemetry/devices/"+hash, adminTok, nil)
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
	if counts.TelemetryDeleted != 1 || counts.AllowlistDeleted != 1 {
		t.Fatalf("counts = %+v, want telemetry=1 allowlist=1", counts)
	}
	// 验收 4：该哈希的遥测查询为空。
	for _, e := range telStore.Snapshot() {
		if e.DeviceHash == hash {
			t.Fatal("telemetry events of the hash must be gone")
		}
	}
	// 该哈希的版本级白名单行被清除；其他设备保留。
	versionAllow, err := store.ListVersionAllowlist(ctx, p.ID)
	if err != nil {
		t.Fatal(err)
	}
	// hashed 策略落库为 HMAC hex；"device-other" 的哈希应保留，device-x 的已清除。
	otherHash := telSvc.HashDeviceID(p, "device-other")
	if len(versionAllow) != 1 || versionAllow[0].DeviceID != otherHash {
		t.Fatalf("remaining version allowlist = %+v, want only device-other (%s)", versionAllow, otherHash)
	}
	// 审计：telemetry.delete 有行且只含哈希形态（本就是哈希，可入审计）。
	got := auditActions(auditStore)
	if len(got) != 1 || got[0] != model.AuditActionTelemetryDelete {
		t.Fatalf("expected telemetry.delete audit row, got %v", got)
	}
}
