package client

import (
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
	"github.com/rs/zerolog"

	"github.com/Kirizu-Official/KiriVers/internal/middleware"
	"github.com/Kirizu-Official/KiriVers/internal/model"
	"github.com/Kirizu-Official/KiriVers/internal/repository"
	"github.com/Kirizu-Official/KiriVers/internal/service"
	"github.com/Kirizu-Official/KiriVers/internal/service/update"
	"github.com/Kirizu-Official/KiriVers/internal/storage"
	"github.com/Kirizu-Official/KiriVers/pkg/hashutil"
)

// setupTelemetryTest 构造带遥测服务的 gin 引擎 + 内存仓储。limiter 传 nil。
func setupTelemetryTest(t *testing.T) (*gin.Engine, *service.ProjectService, *repository.MemoryProjectStore, *service.TelemetryService, *repository.MemoryTelemetryStore, context.Context) {
	t.Helper()
	return setupTelemetryTestWithLimiter(t, nil)
}

// setupTelemetryTestWithLimiter 同上，但可注入限流器（限流矩阵测试用）。
func setupTelemetryTestWithLimiter(t *testing.T, limiter *middleware.Limiter) (*gin.Engine, *service.ProjectService, *repository.MemoryProjectStore, *service.TelemetryService, *repository.MemoryTelemetryStore, context.Context) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	store := repository.NewMemoryProjectStore()
	backend, err := storage.NewLocalFS(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	projSvc := service.NewProjectService(store, backend)
	telStore := repository.NewMemoryTelemetryStore()
	telSvc := service.NewTelemetryService(telStore)
	r := gin.New()
	Register(r.Group("/api/v1"), projSvc,
		update.NewService(repository.NewMemoryUpdateCatalog(store),
			update.WithLineDetails(store), update.WithDowngradeSource(telSvc)),
		telSvc, limiter, nil, nil)
	return r, projSvc, store, telSvc, telStore, context.Background()
}

// telemetryPOST 发起遥测上报请求。
func telemetryPOST(r *gin.Engine, project string, body map[string]any) *httptest.ResponseRecorder {
	raw, err := json.Marshal(body)
	if err != nil {
		panic(err)
	}
	req := httptest.NewRequest(http.MethodPost,
		fmt.Sprintf("/api/v1/projects/%s/telemetry/report", project), bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

// validTelemetryBody 合法上报体。
func validTelemetryBody(extra map[string]any) map[string]any {
	b := map[string]any{
		"device_id":    "device-abc-123",
		"os":           "windows",
		"arch":         "x86_64",
		"channel":      "stable",
		"from_version": "1.0.0",
		"to_version":   "1.1.0",
		"status":       "downloading",
	}
	for k, v := range extra {
		b[k] = v
	}
	return b
}

// TestTelemetryReportAccepted202 只收不挡：合法上报恒 202，事件按 hashed
// 策略落库（库中无明文 device_id，验收项）。
func TestTelemetryReportAccepted202(t *testing.T) {
	r, projSvc, _, telSvc, telStore, ctx := setupTelemetryTest(t)
	p := createSimpleProject(t, projSvc, ctx, "tel-demo")

	w := telemetryPOST(r, p.Slug, validTelemetryBody(map[string]any{"status": "failed", "error_code": "E_IO", "diff_mode": "full_package"}))
	if w.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d %s", w.Code, w.Body.String())
	}
	events := telStore.Snapshot()
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	e := events[0]
	if e.Status != model.TelemetryStatusFailed || e.ErrorCode != "E_IO" || e.DiffMode != "full_package" {
		t.Fatalf("event fields wrong: %+v", e)
	}
	if e.DeviceHash != telSvc.HashDeviceID(p, "device-abc-123") || len(e.DeviceHash) != 64 {
		t.Fatalf("device_hash must be HMAC hex, got %q", e.DeviceHash)
	}
	if strings.Contains(e.DeviceHash, "device-abc-123") {
		t.Fatal("hashed policy must never store raw device id")
	}
}

// TestTelemetryReportValidation 缺必填字段 / 非法 status → 400。
func TestTelemetryReportValidation(t *testing.T) {
	r, projSvc, _, _, _, ctx := setupTelemetryTest(t)
	p := createSimpleProject(t, projSvc, ctx, "tel-valid")

	for name, body := range map[string]map[string]any{
		"missing status": {"os": "windows", "arch": "x86_64", "channel": "stable", "from_version": "1", "to_version": "2"},
		"bad status":     validTelemetryBody(map[string]any{"status": "exploded"}),
		"missing os":     validTelemetryBody(map[string]any{"os": ""}),
		"missing channel": validTelemetryBody(map[string]any{"channel": ""}),
	} {
		if w := telemetryPOST(r, p.Slug, body); w.Code != http.StatusBadRequest {
			t.Fatalf("%s: expected 400, got %d %s", name, w.Code, w.Body.String())
		}
	}
	// 未知项目 → 404。
	if w := telemetryPOST(r, "nope", validTelemetryBody(nil)); w.Code != http.StatusNotFound {
		t.Fatalf("unknown project: expected 404, got %d", w.Code)
	}
	// device_id 缺失仍接受（C11-3）。
	w := telemetryPOST(r, p.Slug, validTelemetryBody(map[string]any{"device_id": ""}))
	if w.Code != http.StatusAccepted {
		t.Fatalf("missing device_id must be accepted, got %d", w.Code)
	}
}

// TestTelemetryReportNoPlaintextInLogs 日志隐私（验收：上报明文 id 时日志文本
// 不含该明文）：请求经由 AccessLog 中间件，捕获完整日志缓冲断言明文不出现。
func TestTelemetryReportNoPlaintextInLogs(t *testing.T) {
	gin.SetMode(gin.TestMode)
	var logBuf bytes.Buffer
	log := zerolog.New(&logBuf)

	store := repository.NewMemoryProjectStore()
	backend, err := storage.NewLocalFS(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	projSvc := service.NewProjectService(store, backend)
	telSvc := service.NewTelemetryService(repository.NewMemoryTelemetryStore())
	r := gin.New()
	r.Use(middleware.RequestID(), middleware.AccessLog(log))
	Register(r.Group("/api/v1"), projSvc, nil, telSvc, nil, nil, nil)

	p := createSimpleProject(t, projSvc, context.Background(), "tel-logs")
	const rawID = "PLAINTEXT-DEVICE-ID-4f2b"
	if w := telemetryPOST(r, p.Slug, validTelemetryBody(map[string]any{"device_id": rawID})); w.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d", w.Code)
	}
	// check 同样携带 device_id：一并断言。
	cw := performCheck(r, fmt.Sprintf("/api/v1/projects/%s/update/check?os=windows&arch=x86_64&current_version=1.0.0&device_id=%s", p.Slug, rawID))
	if cw.Code == http.StatusInternalServerError {
		t.Fatalf("check failed: %s", cw.Body.String())
	}
	if bytes.Contains(logBuf.Bytes(), []byte(rawID)) {
		t.Fatalf("raw device_id leaked into logs:\n%s", logBuf.String())
	}
}

// TestTelemetryReportNonePolicyNoDowngrade none 策略：DeviceHash 空、不做
// 基于设备的降级判定（C11-7）。
func TestTelemetryReportNonePolicyNoDowngrade(t *testing.T) {
	r, projSvc, _, telSvc, telStore, ctx := setupTelemetryTest(t)
	policy := model.DeviceIDPolicyNone
	p, _, err := projSvc.Create(ctx, service.CreateProjectInput{DefaultLocale: ptr("en"), Slug: ptr("tel-none"), DeviceIDPolicy: &policy})
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 5; i++ {
		if w := telemetryPOST(r, p.Slug, validTelemetryBody(map[string]any{"status": "failed"})); w.Code != http.StatusAccepted {
			t.Fatalf("report %d: %d", i, w.Code)
		}
	}
	for _, e := range telStore.Snapshot() {
		if e.DeviceHash != "" {
			t.Fatal("none policy must store empty device hash")
		}
	}
	// 5 次 failed 也不产生降级（DowngradeActive 对空哈希恒 false）。
	active, err := telSvc.DowngradeActive(ctx, p.ID, "windows", "x86_64", "")
	if err != nil || active {
		t.Fatalf("none policy must never downgrade: active=%v err=%v", active, err)
	}
}

// createSimpleProject 建一个 single_file + windows/x86_64 + 1.0.0 已发布的项目。
func createSimpleProject(t *testing.T, projSvc *service.ProjectService, ctx context.Context, slug string) *model.Project {
	t.Helper()
	p, _, err := projSvc.Create(ctx, service.CreateProjectInput{DefaultLocale: ptr("en"), Slug: &slug})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := projSvc.CreateMatrix(ctx, p.ID, service.MatrixWrite{
		OS: ptr("windows"), Arch: ptr("x86_64"), PackageType: ptr(model.PackageTypeSingleFile),
	}); err != nil {
		t.Fatal(err)
	}
	publishSingleFile(t, projSvc, ctx, p.ID, "1.0.0", "stable", "windows", "x86_64", 100, "a-1.0.0.zip")
	return p
}

// publishSingleFileAndDelta 在 1.0.0/1.1.0 之间注入一条 binary_delta 产物
//（直接写入内存仓储，模拟 delta_generate Job 的落库结果）。
func publishSingleFileAndDelta(t *testing.T, projSvc *service.ProjectService, store *repository.MemoryProjectStore, ctx context.Context, slug string) *model.Project {
	t.Helper()
	p, _, err := projSvc.Create(ctx, service.CreateProjectInput{DefaultLocale: ptr("en"), Slug: &slug})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := projSvc.CreateMatrix(ctx, p.ID, service.MatrixWrite{
		OS: ptr("windows"), Arch: ptr("x86_64"), PackageType: ptr(model.PackageTypeSingleFile),
	}); err != nil {
		t.Fatal(err)
	}
	v1 := publishSingleFile(t, projSvc, ctx, p.ID, "1.0.0", "stable", "windows", "x86_64", 100, "a-1.0.0.zip")
	v2 := publishSingleFile(t, projSvc, ctx, p.ID, "1.1.0", "stable", "windows", "x86_64", 200, "a-1.1.0.zip")

	srcPkg, err := store.GetArtifactByLineID(ctx, mustLineID(t, store, ctx, v1.ID, "windows", "x86_64"))
	if err != nil {
		t.Fatal(err)
	}
	tgtLine, err := store.GetVersionLine(ctx, v2.ID, "windows", "x86_64")
	if err != nil {
		t.Fatal(err)
	}
	tgtPkg, err := store.GetArtifactByLineID(ctx, tgtLine.ID)
	if err != nil {
		t.Fatal(err)
	}
	art := &model.Artifact{
		ProjectID:         p.ID,
		VersionID:         v2.ID,
		VersionLineID:     tgtLine.ID,
		Kind:              model.ArtifactKindDelta,
		FileName:          "a-1.0.0_1.1.0.bsdiff",
		Size:              42,
		SHA256:            strings.Repeat("ab", 32),
		DeltaAlgo:         "bsdiff",
		DeltaSourceSHA256: srcPkg.SHA256,
		DeltaTargetSHA256: tgtPkg.SHA256,
	}
	if err := store.CreateArtifact(ctx, art); err != nil {
		t.Fatal(err)
	}
	return p
}

func mustLineID(t *testing.T, store *repository.MemoryProjectStore, ctx context.Context, versionID uuid.UUID, os, arch string) uuid.UUID {
	t.Helper()
	line, err := store.GetVersionLine(ctx, versionID, os, arch)
	if err != nil {
		t.Fatal(err)
	}
	return line.ID
}

// TestDowngradeEndToEnd 验收：3 次 failed 后 diff 强制 full_package 即使有
// 差量；一次 installed 后恢复。check 的 delta_available 同步翻转，且 ETag
// 不受降级状态影响。
func TestDowngradeEndToEnd(t *testing.T) {
	r, projSvc, store, _, _, ctx := setupTelemetryTest(t)
	p := publishSingleFileAndDelta(t, projSvc, store, ctx, "dg-e2e")

	const deviceID = "device-dg-1"
	diffBody := func() map[string]any {
		return map[string]any{
			"source_version": "1.0.0", "target_version": "1.1.0",
			"os": "windows", "arch": "x86_64",
			"device_id":            deviceID,
			"local_sha256":         sourceSHA(t),
			"capabilities":         []string{"binary_delta"},
			"accepted_delta_algos": []string{"bsdiff"},
		}
	}

	// 基线：有差量 → binary_delta。
	w := diffPOST(r, p.Slug, diffBody())
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"diff_mode":"binary_delta"`) {
		t.Fatalf("baseline expected binary_delta, got %d %s", w.Code, w.Body.String())
	}

	// check 基线：delta_available=true（能力由 accepted_delta_algos 授予，
	// 与是否匿名无关）；ETag 与匿名请求一致。
	etag := checkDeltaAvailable(t, r, p.Slug, deviceID, true)
	anonETag := checkDeltaAvailable(t, r, p.Slug, "", true)
	if etag != anonETag {
		t.Fatalf("device state must not affect ETag: %q vs %q", etag, anonETag)
	}

	// 3 次 failed → diff 强制 full_package、check delta_available=false。
	for i := 0; i < 3; i++ {
		body := validTelemetryBody(map[string]any{
			"status": "failed", "device_id": deviceID,
			"from_version": "1.0.0", "to_version": "1.1.0",
		})
		if w := telemetryPOST(r, p.Slug, body); w.Code != http.StatusAccepted {
			t.Fatalf("failed report %d: %d", i, w.Code)
		}
	}
	w = diffPOST(r, p.Slug, diffBody())
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"diff_mode":"full_package"`) {
		t.Fatalf("after 3 failed expected full_package, got %d %s", w.Code, w.Body.String())
	}
	checkDeltaAvailable(t, r, p.Slug, deviceID, false)
	// ETag 在降级生效时仍与匿名一致（快照不含设备状态）。
	if got := checkDeltaAvailable(t, r, p.Slug, deviceID, false); got != anonETag {
		t.Fatalf("downgrade must not change ETag: %q vs %q", got, anonETag)
	}

	// 一次 installed → 恢复差量。
	if w := telemetryPOST(r, p.Slug, validTelemetryBody(map[string]any{
		"status": "installed", "device_id": deviceID,
		"from_version": "1.0.0", "to_version": "1.1.0",
	})); w.Code != http.StatusAccepted {
		t.Fatalf("installed report: %d", w.Code)
	}
	w = diffPOST(r, p.Slug, diffBody())
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"diff_mode":"binary_delta"`) {
		t.Fatalf("after installed expected binary_delta, got %d %s", w.Code, w.Body.String())
	}
	checkDeltaAvailable(t, r, p.Slug, deviceID, true)
}

// checkDeltaAvailable 发起带 device_id 的 check，断言 delta_available 并返回 ETag。
func checkDeltaAvailable(t *testing.T, r *gin.Engine, project, deviceID string, want bool) string {
	t.Helper()
	path := fmt.Sprintf("/api/v1/projects/%s/update/check?os=windows&arch=x86_64&current_version=1.0.0&accepted_delta_algos=bsdiff", project)
	if deviceID != "" {
		path += "&device_id=" + deviceID
	}
	w := performCheck(r, path)
	if w.Code != http.StatusOK {
		t.Fatalf("check: expected 200, got %d %s", w.Code, w.Body.String())
	}
	var body struct {
		DeltaAvailable bool   `json:"delta_available"`
		ETag           string `json:"-"`
	}
	body.ETag = w.Header().Get("ETag")
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.DeltaAvailable != want {
		t.Fatalf("delta_available = %v, want %v (body=%s)", body.DeltaAvailable, want, w.Body.String())
	}
	return body.ETag
}

// sourceSHA 计算 1.0.0 全量包（publishSingleFile 夹具的 100 字节 'x' 载荷）
// 的 SHA-256，作 binary_delta 基线匹配输入。
func sourceSHA(t *testing.T) string {
	t.Helper()
	payload := bytes.Repeat([]byte("x"), 100)
	sha, err := hashutil.SHA256Hex(bytes.NewReader(payload))
	if err != nil {
		t.Fatal(err)
	}
	return sha
}
