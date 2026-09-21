package client

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/Kirizu-Official/KiriVers/internal/middleware"
	"github.com/Kirizu-Official/KiriVers/internal/model"
	"github.com/Kirizu-Official/KiriVers/internal/service"
)

// performHeader 执行带自定义头的 GET 请求。
func performHeader(r *gin.Engine, method, path string, header map[string]string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, nil)
	for k, v := range header {
		req.Header.Set(k, v)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

// setupRateLimitTest 构造带限流器的引擎，项目配额按 limits 覆盖
//（其余键仍用文档默认）。项目为 single_file + windows/x86_64 + 1.0.0 已发布。
func setupRateLimitTest(t *testing.T, limits model.JSONObject) (*gin.Engine, *model.Project) {
	t.Helper()
	r, projSvc, _, _, _, ctx := setupTelemetryTestWithLimiter(t, middleware.NewLimiter())
	slug := "rl-demo"
	p, _, err := projSvc.Create(ctx, service.CreateProjectInput{DefaultLocale: ptr("en"), Slug: &slug, RateLimit: &limits})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := projSvc.CreateMatrix(ctx, p.ID, service.MatrixWrite{
		OS: ptr("windows"), Arch: ptr("x86_64"), PackageType: ptr(model.PackageTypeSingleFile),
	}); err != nil {
		t.Fatal(err)
	}
	publishSingleFile(t, projSvc, ctx, p.ID, "1.0.0", "stable", "windows", "x86_64", 100, "a-1.0.0.zip")
	publishSingleFile(t, projSvc, ctx, p.ID, "1.1.0", "stable", "windows", "x86_64", 200, "a-1.1.0.zip")
	return r, p
}

// rateLimitedBody 断言 429 响应形状：RATE_LIMITED + Retry-After 整秒 + private,no-store。
func rateLimitedBody(t *testing.T, w *httptest.ResponseRecorder) {
	t.Helper()
	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429, got %d %s", w.Code, w.Body.String())
	}
	var body struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil || body.Error.Code != "RATE_LIMITED" {
		t.Fatalf("error code = %q err=%v", body.Error.Code, err)
	}
	if got := w.Header().Get("Retry-After"); got == "" {
		t.Fatal("missing Retry-After")
	} else if n, err := strconv.Atoi(got); err != nil || n < 1 {
		t.Fatalf("Retry-After = %q, want integer >= 1", got)
	}
	if got := w.Header().Get("Cache-Control"); got != "private, no-store" {
		t.Fatalf("Cache-Control = %q, want private, no-store", got)
	}
}

// TestCheckRateLimited429 验收：超限 check 429 RATE_LIMITED 且有 Retry-After。
func TestCheckRateLimited429(t *testing.T) {
	r, p := setupRateLimitTest(t, model.JSONObject{model.RateLimitKeyCheckPerIP: 2})
	path := "/api/v1/projects/" + p.Slug + "/update/check?os=windows&arch=x86_64&current_version=1.0.0"

	if w := performCheck(r, path); w.Code != http.StatusOK {
		t.Fatalf("hit 1: %d", w.Code)
	}
	if w := performCheck(r, path); w.Code != http.StatusOK {
		t.Fatalf("hit 2: %d", w.Code)
	}
	rateLimitedBody(t, performCheck(r, path))
}

// TestCheck304StillCounts 验收：HTTP 304 仍计入限额（限流判定在 304 短路之前）。
func TestCheck304StillCounts(t *testing.T) {
	r, p := setupRateLimitTest(t, model.JSONObject{model.RateLimitKeyCheckPerIP: 2})
	path := "/api/v1/projects/" + p.Slug + "/update/check?os=windows&arch=x86_64&current_version=1.0.0"

	// 第 1 次：200 + ETag。
	w1 := performCheck(r, path)
	if w1.Code != http.StatusOK {
		t.Fatalf("hit 1: %d", w1.Code)
	}
	etag := w1.Header().Get("ETag")
	if etag == "" {
		t.Fatal("missing ETag")
	}

	// 第 2 次：If-None-Match 命中 → 304，但计入限额（配额 2 已用完）。
	w2 := performCheckHdr(r, path, http.Header{"If-None-Match": []string{etag}})
	if w2.Code != http.StatusNotModified {
		t.Fatalf("hit 2: expected 304, got %d", w2.Code)
	}

	// 第 3 次：304 已消耗配额 → 429。
	rateLimitedBody(t, performCheck(r, path))
}

// TestTelemetryRateLimitedPerDevice 遥测设备维度限流：默认键 30/分被覆盖为 2/分。
func TestTelemetryRateLimitedPerDevice(t *testing.T) {
	r, p := setupRateLimitTest(t, model.JSONObject{model.RateLimitKeyTelemetryPerDevice: 2})
	body := validTelemetryBody(nil)

	if w := telemetryPOST(r, p.Slug, body); w.Code != http.StatusAccepted {
		t.Fatalf("report 1: %d", w.Code)
	}
	if w := telemetryPOST(r, p.Slug, body); w.Code != http.StatusAccepted {
		t.Fatalf("report 2: %d", w.Code)
	}
	rateLimitedBody(t, telemetryPOST(r, p.Slug, body))
}

// TestTelemetryDeviceKeyFallbackToIP 无 device_id（或 none 策略）回退 IP 维度，
// 且设备键与 IP 键独立计数。
func TestTelemetryDeviceKeyFallbackToIP(t *testing.T) {
	r, p := setupRateLimitTest(t, model.JSONObject{model.RateLimitKeyTelemetryPerDevice: 1})

	// 设备上报与匿名上报各占一个键：各自第 1 次放行。
	if w := telemetryPOST(r, p.Slug, validTelemetryBody(nil)); w.Code != http.StatusAccepted {
		t.Fatalf("device report: %d", w.Code)
	}
	anon := validTelemetryBody(map[string]any{"device_id": ""})
	if w := telemetryPOST(r, p.Slug, anon); w.Code != http.StatusAccepted {
		t.Fatalf("anonymous report: %d", w.Code)
	}
	// 各自第 2 次超限。
	rateLimitedBody(t, telemetryPOST(r, p.Slug, validTelemetryBody(nil)))
	rateLimitedBody(t, telemetryPOST(r, p.Slug, anon))
}

// TestDiffRateLimited diff 双维度：设备 20/分 / IP 60/分，此处覆盖为 1/分验证挂载。
func TestDiffRateLimited(t *testing.T) {
	r, p := setupRateLimitTest(t, model.JSONObject{model.RateLimitKeyDiffPerDevice: 1})
	body := diffBody(nil)
	body["device_id"] = "device-rl"

	if w := diffPOST(r, p.Slug, body); w.Code != http.StatusOK {
		t.Fatalf("diff 1: %d %s", w.Code, w.Body.String())
	}
	// 同设备第 2 次 → 429（设备维度）。
	rateLimitedBody(t, diffPOST(r, p.Slug, body))
	// 匿名（只占 IP 维度）仍放行。
	anon := diffBody(nil)
	if w := diffPOST(r, p.Slug, anon); w.Code != http.StatusOK {
		t.Fatalf("anonymous diff: %d %s", w.Code, w.Body.String())
	}
}
