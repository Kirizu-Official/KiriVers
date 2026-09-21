package admin

import (
	"encoding/json"
	"net/http"
	"regexp"
	"testing"

	"github.com/Kirizu-Official/KiriVers/internal/buildinfo"
	"github.com/Kirizu-Official/KiriVers/internal/repository"
	"github.com/Kirizu-Official/KiriVers/internal/service"
	"github.com/Kirizu-Official/KiriVers/pkg/response"
)

// buildInfoKeys 是 GET /api/v1/admin/build-info 的全部键，必须与
// openapi.admin.json 的 BuildInfo schema 一致（字段表测试把守），这里再锁一层
// “无多余键”，防止 handler 悄悄加字段而契约不动。
var buildInfoKeys = []string{"build_time", "cgo_enabled", "commit", "go_version", "platform", "version"}

var platformRe = regexp.MustCompile(`^[a-z0-9]+/[a-z0-9]+$`)

// TestBuildInfoEndpointReportsBuildStamp 覆盖 AC1 的链路：完整会话 → 200 裸 payload，
// 六个字段逐一等于 internal/buildinfo 的进程内快照（不查库、不依赖注入值来自哪条链路）。
// 未注入构建（常规 go test）时 version 必须是 dev、build_time 必须是 unknown。
func TestBuildInfoEndpointReportsBuildStamp(t *testing.T) {
	svc := service.NewTestAdminService(repository.NewMemoryAdminStore())
	engine := newAdminTestEngine(t, svc, nil)

	if _, err := svc.Create(t.Context(), "root", "password123"); err != nil {
		t.Fatal(err)
	}
	w := postAdminLogin(t, engine, "127.0.0.1:1234", "", "root", "password123")
	if w.Code != http.StatusOK {
		t.Fatalf("login=%d %s", w.Code, w.Body.String())
	}
	pending := decodePendingLogin(t, w.Body.Bytes())
	token, _ := completeHTTPEnrollment(t, engine, pending.PendingToken)

	w = postJSONAuth(engine, http.MethodGet, "/api/v1/admin/build-info", token, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("build-info=%d %s", w.Code, w.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if len(body) != len(buildInfoKeys) {
		t.Fatalf("build-info keys=%v, want exactly %v", body, buildInfoKeys)
	}
	want := map[string]any{
		"version":     buildinfo.Version(),
		"commit":      buildinfo.Commit(),
		"build_time":  buildinfo.BuildTime(),
		"go_version":  buildinfo.GoVersion(),
		"platform":    buildinfo.Platform(),
		"cgo_enabled": buildinfo.CgoEnabled(),
	}
	for key, value := range want {
		got, ok := body[key]
		if !ok {
			t.Errorf("missing key %q in %v", key, body)
			continue
		}
		if got != value {
			t.Errorf("%s = %#v, want %#v", key, got, value)
		}
	}
	// 回退值必须可辨识：任何字段都不得为空串（前端 footer 直接展示版本号）。
	for _, key := range []string{"version", "commit", "build_time", "go_version", "platform"} {
		if s, _ := body[key].(string); s == "" {
			t.Errorf("%s = %q, must never be empty", key, s)
		}
	}
	if s, _ := body["platform"].(string); !platformRe.MatchString(s) {
		t.Errorf("platform = %q, want form goos/goarch", s)
	}
	if buildinfo.Version() == buildinfo.FallbackVersion {
		if s, _ := body["version"].(string); s != buildinfo.FallbackVersion {
			t.Errorf("version = %q, want fallback %q", s, buildinfo.FallbackVersion)
		}
		if s, _ := body["build_time"].(string); s != buildinfo.FallbackBuildTime {
			t.Errorf("build_time = %q, want fallback %q", s, buildinfo.FallbackBuildTime)
		}
	}
}

// TestBuildInfoEndpointRequiresFullSession 锁 R4 的暴露面：匿名与 pending 会话
// （强制改密 / 2FA 绑定中）都拿不到编译信息，响应走 pkg/response 错误信封。
func TestBuildInfoEndpointRequiresFullSession(t *testing.T) {
	svc := service.NewTestAdminService(repository.NewMemoryAdminStore())
	engine := newAdminTestEngine(t, svc, nil)

	if _, err := svc.Create(t.Context(), "root", "password123"); err != nil {
		t.Fatal(err)
	}
	w := postAdminLogin(t, engine, "127.0.0.1:1234", "", "root", "password123")
	if w.Code != http.StatusOK {
		t.Fatalf("login=%d %s", w.Code, w.Body.String())
	}
	pending := decodePendingLogin(t, w.Body.Bytes())

	for label, token := range map[string]string{"anonymous": "", "pending session": pending.PendingToken} {
		got := postJSONAuth(engine, http.MethodGet, "/api/v1/admin/build-info", token, nil)
		if got.Code != http.StatusUnauthorized {
			t.Errorf("%s: code=%d want 401 body=%s", label, got.Code, got.Body.String())
			continue
		}
		var env response.Body
		if err := json.Unmarshal(got.Body.Bytes(), &env); err != nil {
			t.Fatalf("%s: unmarshal: %v", label, err)
		}
		if env.Error.Code != "UNAUTHORIZED" {
			t.Errorf("%s: code=%q want UNAUTHORIZED", label, env.Error.Code)
		}
	}
}
