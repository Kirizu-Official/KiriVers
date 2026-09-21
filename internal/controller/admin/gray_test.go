package admin

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/Kirizu-Official/KiriVers/internal/controller/client"
	"github.com/Kirizu-Official/KiriVers/internal/model"
	"github.com/Kirizu-Official/KiriVers/internal/repository"
	"github.com/Kirizu-Official/KiriVers/internal/service"
	"github.com/Kirizu-Official/KiriVers/internal/service/update"
	"github.com/Kirizu-Official/KiriVers/internal/storage"
	"github.com/Kirizu-Official/KiriVers/pkg/hashutil"
)

// setupGrayProject 创建项目（默认 hashed 策略）+ Draft 版本 1.0.0，返回项目 slug。
func setupGrayProject(t *testing.T, r *gin.Engine, projSvc *service.ProjectService, token string, policy *string) string {
	t.Helper()
	slug := "gray-proj"
	if _, _, err := projSvc.Create(t.Context(), service.CreateProjectInput{DefaultLocale: ptr("en"), Slug: &slug,
		CompareEngine:  ptr(model.CompareEngineSemver),
		DeviceIDPolicy: policy,
	}); err != nil {
		t.Fatal(err)
	}
	w := doJSON(r, http.MethodPut, "/api/v1/admin/projects/gray-proj/versions/1.0.0", token, gin.H{
		"channel":   "stable",
		"changelog": "Initial version",
	})
	if w.Code != http.StatusCreated {
		t.Fatalf("put version failed: %d %s", w.Code, w.Body.String())
	}
	return slug
}

func loginDevice(t *testing.T, projSvc *service.ProjectService, slug, device string) uuid.UUID {
	t.Helper()
	p, err := projSvc.Resolve(t.Context(), slug)
	if err != nil {
		t.Fatal(err)
	}
	cl, err := projSvc.LoginClient(t.Context(), p, service.ClientLoginInput{
		DeviceID: device, Version: "1.0.0", OS: "windows", Arch: "x86_64",
	})
	if err != nil {
		t.Fatal(err)
	}
	return cl.ID
}

// TestGrayAllowlistAddDeleteRoundTrip 覆盖 C2 验收：按名册 UUID 增删往返、去重、哈希落库。
func TestGrayAllowlistAddDeleteRoundTrip(t *testing.T) {
	r, projSvc, _, token := setupProjectHTTP(t)
	slug := setupGrayProject(t, r, projSvc, token, nil)
	base := "/api/v1/admin/projects/" + slug + "/versions/1.0.0/gray/allowlist"
	idA := loginDevice(t, projSvc, slug, "dev-a")
	idB := loginDevice(t, projSvc, slug, "dev-b")

	w := doJSON(r, http.MethodPost, base, token, gin.H{"client_ids": []string{idA.String(), idB.String(), idA.String()}})
	if w.Code != http.StatusOK {
		t.Fatalf("add failed: %d %s", w.Code, w.Body.String())
	}
	res := decodeMap(t, w)
	entries := res["entries"].([]any)
	if len(entries) != 2 {
		t.Fatalf("expected 2 deduped entries, got %d", len(entries))
	}
	proj, err := projSvc.Resolve(t.Context(), slug)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		m := e.(map[string]any)
		id := m["device_id"].(string)
		if id == "dev-a" || id == "dev-b" {
			t.Fatalf("raw device id must not be echoed under hashed policy: %s", id)
		}
		if id != service.HashDeviceID(proj, "dev-a") && id != service.HashDeviceID(proj, "dev-b") {
			t.Fatalf("stored device_id must equal HMAC of raw id: %s", id)
		}
	}

	w = doJSON(r, http.MethodPost, base, token, gin.H{"client_ids": []string{idA.String()}})
	if w.Code != http.StatusOK {
		t.Fatalf("idempotent add failed: %d %s", w.Code, w.Body.String())
	}
	if entries = decodeMap(t, w)["entries"].([]any); len(entries) != 2 {
		t.Fatalf("expected still 2 entries after idempotent add, got %d", len(entries))
	}

	w = doJSON(r, http.MethodDelete, base, token, gin.H{"client_ids": []string{idA.String()}})
	if w.Code != http.StatusOK {
		t.Fatalf("delete failed: %d %s", w.Code, w.Body.String())
	}
	res = decodeMap(t, w)
	if res["removed"].(float64) != 1 {
		t.Fatalf("expected removed=1, got %v", res["removed"])
	}
	if entries = res["entries"].([]any); len(entries) != 1 {
		t.Fatalf("expected 1 remaining entry, got %d", len(entries))
	}

	w = doJSON(r, http.MethodDelete, base, token, gin.H{"client_ids": []string{uuid.NewString()}})
	if w.Code != http.StatusOK || decodeMap(t, w)["removed"].(float64) != 0 {
		t.Fatalf("unknown entry delete must be idempotent: %d %s", w.Code, w.Body.String())
	}
}

// TestGrayAllowlistValidation 非法条目与 none 策略 → 400（无新增错误码）。
func TestGrayAllowlistValidation(t *testing.T) {
	r, projSvc, _, token := setupProjectHTTP(t)
	slug := setupGrayProject(t, r, projSvc, token, nil)
	base := "/api/v1/admin/projects/" + slug + "/versions/1.0.0/gray/allowlist"

	w := doJSON(r, http.MethodPost, base, token, gin.H{"client_ids": []string{}})
	if w.Code != http.StatusBadRequest || decodeErr(t, w) != "INVALID_REQUEST" {
		t.Fatalf("empty client_ids must be 400 INVALID_REQUEST: %d %s", w.Code, w.Body.String())
	}
	w = doJSON(r, http.MethodPost, base, token, gin.H{"client_ids": []string{"not-a-uuid"}})
	if w.Code != http.StatusBadRequest {
		t.Fatalf("invalid uuid must be 400: %d %s", w.Code, w.Body.String())
	}

	nonePolicy := model.DeviceIDPolicyNone
	r2, projSvc2, _, token2 := setupProjectHTTP(t)
	slug2 := "none-policy-proj"
	if _, _, err := projSvc2.Create(t.Context(), service.CreateProjectInput{DefaultLocale: ptr("en"), Slug: &slug2,
		CompareEngine:  ptr(model.CompareEngineSemver),
		DeviceIDPolicy: &nonePolicy,
	}); err != nil {
		t.Fatal(err)
	}
	w = doJSON(r2, http.MethodPut, "/api/v1/admin/projects/"+slug2+"/versions/1.0.0", token2, gin.H{
		"channel": "stable", "changelog": "v",
	})
	if w.Code != http.StatusCreated {
		t.Fatalf("put version failed: %d %s", w.Code, w.Body.String())
	}
	w = doJSON(r2, http.MethodPost, "/api/v1/admin/projects/"+slug2+"/versions/1.0.0/gray/allowlist", token2,
		gin.H{"client_ids": []string{uuid.NewString()}})
	if w.Code != http.StatusBadRequest {
		t.Fatalf("none policy must reject allowlist writes: %d %s", w.Code, w.Body.String())
	}

	w = doJSON(r, http.MethodPost, "/api/v1/admin/projects/"+slug+"/versions/9.9.9/gray/allowlist", token,
		gin.H{"client_ids": []string{uuid.NewString()}})
	if w.Code != http.StatusNotFound || decodeErr(t, w) != "VERSION_NOT_FOUND" {
		t.Fatalf("unknown version must be 404: %d %s", w.Code, w.Body.String())
	}
}

// TestVersionLineMinOSPatch 覆盖线级 min_os PATCH（已无 per-line 灰度覆盖）。
func TestVersionLineMinOSPatch(t *testing.T) {
	r, projSvc, _, token := setupProjectHTTP(t)
	slug := setupGrayProject(t, r, projSvc, token, nil)

	w := doJSON(r, http.MethodPost, "/api/v1/admin/projects/"+slug+"/versions/1.0.0/lines", token,
		gin.H{"os": "windows", "arch": "x86_64"})
	if w.Code != http.StatusCreated {
		t.Fatalf("create line failed: %d %s", w.Code, w.Body.String())
	}

	lineBase := "/api/v1/admin/projects/" + slug + "/versions/1.0.0/lines/windows/x86_64"
	w = doJSON(r, http.MethodPatch, lineBase, token, gin.H{"min_os": "10.0"})
	if w.Code != http.StatusOK {
		t.Fatalf("patch min_os failed: %d %s", w.Code, w.Body.String())
	}
	if got := decodeMap(t, w)["min_os"]; got != "10.0" {
		t.Fatalf("expected min_os=10.0, got %v", got)
	}

	w = doJSON(r, http.MethodPatch, lineBase, token, gin.H{"min_os": nil})
	if w.Code != http.StatusOK {
		t.Fatalf("clear min_os failed: %d %s", w.Code, w.Body.String())
	}
	if _, has := decodeMap(t, w)["min_os"]; has {
		t.Fatalf("cleared min_os must not be echoed")
	}

	w = doJSON(r, http.MethodPatch, "/api/v1/admin/projects/"+slug+"/versions/1.0.0/lines/linux/arm64", token,
		gin.H{"min_os": "12"})
	if w.Code != http.StatusNotFound || decodeErr(t, w) != "VERSION_LINE_NOT_FOUND" {
		t.Fatalf("unknown line must be 404: %d %s", w.Code, w.Body.String())
	}
}

func decodeMap(t *testing.T, w *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var res map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &res); err != nil {
		t.Fatalf("decode response: %v; body=%s", err, w.Body.String())
	}
	return res
}

func publishReadyLine(t *testing.T, projSvc *service.ProjectService, projectID uuid.UUID, versionRef string, rollout *int) {
	t.Helper()
	ctx := t.Context()
	if _, _, err := projSvc.PutVersion(ctx, projectID, versionRef, service.VersionWriteInput{
		Channel:          "stable",
		GrayStartPercent: rollout,
	}); err != nil {
		t.Fatalf("put version %s: %v", versionRef, err)
	}
	payload := bytes.Repeat([]byte("x"), 64)
	sha, _ := hashutil.SHA256Hex(bytes.NewReader(payload))
	fname := "demo-" + versionRef + "-windows-x86_64.zip"
	if _, err := projSvc.UploadArtifact(ctx, projectID.String(), versionRef, "windows", "x86_64", service.UploadArtifactInput{
		Filename:       fname,
		ExpectedSHA256: sha,
		Size:           int64(len(payload)),
	}, bytes.NewReader(payload)); err != nil {
		t.Fatalf("upload artifact %s: %v", versionRef, err)
	}
	if _, err := projSvc.ReadyVersionLine(ctx, projectID, versionRef, "windows", "x86_64"); err != nil {
		t.Fatalf("ready line %s: %v", versionRef, err)
	}
	if _, err := projSvc.PublishVersion(ctx, projectID, versionRef); err != nil {
		t.Fatalf("publish %s: %v", versionRef, err)
	}
}

// TestCheckE2EAllowlistDeviceGetsUpdate 端到端：名册登录后按 client_id 写入白名单，
// 公开 check 用同一 device_id 必须命中该版本灰度。
func TestCheckE2EAllowlistDeviceGetsUpdate(t *testing.T) {
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
	telemetrySvc := service.NewTelemetryService(repository.NewMemoryTelemetryStore())

	r := gin.New()
	Register(r.Group("/api/v1/admin"), adminSvc, projSvc, nil, nil, nil, nil, nil)
	client.Register(r.Group("/api/v1"), projSvc,
		update.NewService(repository.NewMemoryUpdateCatalog(store)), telemetrySvc, nil, nil, nil)
	token := login.Token

	ctx := t.Context()
	slug := "e2e-gray"
	p, _, err := projSvc.Create(ctx, service.CreateProjectInput{DefaultLocale: ptr("en"), Slug: &slug,
		CompareEngine: ptr(model.CompareEngineSemver),
	})
	if err != nil {
		t.Fatal(err)
	}
	idA, err := projSvc.LoginClient(ctx, p, service.ClientLoginInput{
		DeviceID: "dev-a", Version: "1.0.0", OS: "windows", Arch: "x86_64",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := projSvc.LoginClient(ctx, p, service.ClientLoginInput{
		DeviceID: "dev-b", Version: "1.0.0", OS: "windows", Arch: "x86_64",
	}); err != nil {
		t.Fatal(err)
	}

	zero := 0
	publishReadyLine(t, projSvc, p.ID, "1.0.0", nil)
	publishReadyLine(t, projSvc, p.ID, "1.1.0", &zero)
	publishReadyLine(t, projSvc, p.ID, "1.2.0", &zero)

	w := doJSON(r, http.MethodPost, "/api/v1/admin/projects/"+slug+"/versions/1.1.0/gray/allowlist", token,
		gin.H{"client_ids": []string{idA.ID.String()}})
	if w.Code != http.StatusOK {
		t.Fatalf("allowlist add failed: %d %s", w.Code, w.Body.String())
	}

	check := func(deviceID string) (int, string) {
		t.Helper()
		body := gin.H{
			"current_version": "1.0.0",
			"os":              "windows",
			"arch":            "x86_64",
		}
		if deviceID != "" {
			body["device_id"] = deviceID
		}
		w := doJSON(r, http.MethodPost, "/api/v1/projects/"+slug+"/update/check", "", body)
		var out map[string]any
		_ = json.Unmarshal(w.Body.Bytes(), &out)
		ver, _ := out["version_semver"].(string)
		return w.Code, ver
	}

	if got, _ := check("dev-a"); got != http.StatusOK {
		t.Fatalf("allowlisted device must get 200, got %d", got)
	}
	if _, ver := check("dev-a"); ver != "1.1.0" {
		t.Fatalf("allowlist must be scoped per version, got target %s", ver)
	}
	if got, _ := check("dev-b"); got != http.StatusNoContent {
		t.Fatalf("non-allowlisted device must get 204, got %d", got)
	}
	if got, _ := check(""); got != http.StatusNoContent {
		t.Fatalf("anonymous must get 204, got %d", got)
	}
}

func TestVersionListShowsActualGrayPercent(t *testing.T) {
	r, projSvc, _, token := setupProjectHTTP(t)
	ctx := t.Context()
	slug := "gray-actual-chip"
	p, _, err := projSvc.Create(ctx, service.CreateProjectInput{
		DefaultLocale: ptr("en"), Slug: &slug, CompareEngine: ptr(model.CompareEngineSemver),
	})
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 4; i++ {
		if _, err := projSvc.LoginClient(ctx, p, service.ClientLoginInput{
			DeviceID: "chip-" + string(rune('a'+i)), Version: "0.9.0", OS: "windows", Arch: "x86_64",
		}); err != nil {
			t.Fatal(err)
		}
	}
	start := 10
	publishReadyLine(t, projSvc, p.ID, "1.0.0", &start)

	w := doJSON(r, http.MethodGet, "/api/v1/admin/projects/"+slug+"/versions", token, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("list versions: %d %s", w.Code, w.Body.String())
	}
	var env struct {
		Versions []map[string]any `json:"versions"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &env); err != nil {
		t.Fatal(err)
	}
	var found map[string]any
	for _, item := range env.Versions {
		if item["version_semver"] == "1.0.0" {
			found = item
			break
		}
	}
	if found == nil {
		t.Fatalf("missing 1.0.0 in %s", w.Body.String())
	}
	if found["gray_start_percent"] != float64(10) {
		t.Fatalf("gray_start_percent=%v want 10", found["gray_start_percent"])
	}
	if found["actual_percent"] != float64(25) {
		t.Fatalf("actual_percent=%v want 25 (1/4 allowlisted), body=%s", found["actual_percent"], w.Body.String())
	}

	detail := doJSON(r, http.MethodGet, "/api/v1/admin/projects/"+slug+"/versions/1.0.0", token, nil)
	if detail.Code != http.StatusOK {
		t.Fatalf("get version: %d %s", detail.Code, detail.Body.String())
	}
	got := decodeMap(t, detail)
	if got["actual_percent"] != float64(25) {
		t.Fatalf("detail actual_percent=%v want 25", got["actual_percent"])
	}
}
