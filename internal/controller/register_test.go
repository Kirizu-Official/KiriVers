package controller

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/Kirizu-Official/KiriVers/pkg/response"
)

// testEngines 构建双平面测试引擎（client 与 admin 各一个，与生产装配一致）。
func testEngines(t *testing.T, ready func(context.Context) error) (clientEngine, adminEngine *gin.Engine) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	clientEngine = gin.New()
	RegisterClient(clientEngine, Deps{Ready: ready})
	adminEngine = gin.New()
	RegisterAdmin(adminEngine, Deps{Ready: ready})
	return clientEngine, adminEngine
}

func TestHealthOKWithoutDB(t *testing.T) {
	clientEngine, adminEngine := testEngines(t, nil)
	for name, engine := range map[string]*gin.Engine{"client": clientEngine, "admin": adminEngine} {
		w := httptest.NewRecorder()
		engine.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/health", nil))
		if w.Code != http.StatusOK {
			t.Fatalf("%s: status=%d body=%s", name, w.Code, w.Body.String())
		}
		got := decodeHealth(t, w)
		if got.Status != "ok" || got.Ready {
			t.Fatalf("%s: body=%+v", name, got)
		}
	}
}

func TestHealthReadyFalseWhenCheckFails(t *testing.T) {
	clientEngine, adminEngine := testEngines(t, nil)
	for name, engine := range map[string]*gin.Engine{"client": clientEngine, "admin": adminEngine} {
		got := getHealth(t, engine)
		if got.Status != "ok" || got.Ready {
			t.Fatalf("%s: nil ready fn body=%+v", name, got)
		}
	}

	clientEngine, adminEngine = testEngines(t, func(context.Context) error {
		return errors.New("storage head failed")
	})
	for name, engine := range map[string]*gin.Engine{"client": clientEngine, "admin": adminEngine} {
		got := getHealth(t, engine)
		if got.Status != "ok" || got.Ready {
			t.Fatalf("%s: err ready fn body=%+v", name, got)
		}
	}
}

func TestHealthReadyTrueWhenCheckPasses(t *testing.T) {
	clientEngine, adminEngine := testEngines(t, func(context.Context) error { return nil })
	for name, engine := range map[string]*gin.Engine{"client": clientEngine, "admin": adminEngine} {
		got := getHealth(t, engine)
		if got.Status != "ok" || !got.Ready {
			t.Fatalf("%s: body=%+v", name, got)
		}
	}
}

func TestReadyUnregistered(t *testing.T) {
	clientEngine, adminEngine := testEngines(t, func(context.Context) error { return nil })
	for name, engine := range map[string]*gin.Engine{"client": clientEngine, "admin": adminEngine} {
		w := httptest.NewRecorder()
		engine.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/ready", nil))
		if w.Code != http.StatusNotFound {
			t.Fatalf("%s: ready status=%d body=%s", name, w.Code, w.Body.String())
		}
		var env response.Body
		if err := json.Unmarshal(w.Body.Bytes(), &env); err != nil {
			t.Fatal(err)
		}
		if env.Error.Code != "NOT_FOUND" {
			t.Fatalf("%s: code=%q", name, env.Error.Code)
		}
	}
}

type healthBody struct {
	Status string `json:"status"`
	Ready  bool   `json:"ready"`
}

func getHealth(t *testing.T, engine *gin.Engine) healthBody {
	t.Helper()
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/health", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("health status=%d body=%s", w.Code, w.Body.String())
	}
	return decodeHealth(t, w)
}

func decodeHealth(t *testing.T, w *httptest.ResponseRecorder) healthBody {
	t.Helper()
	var body healthBody
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	return body
}

// TestOpenAPIJSON 断言每个平面引擎只暴露本平面的契约：title 按平面、
// 本平面路径存在、跨平面路径不存在（09-14-split-dual-server D2）。
func TestOpenAPIJSON(t *testing.T) {
	clientEngine, adminEngine := testEngines(t, nil)

	check := func(t *testing.T, engine *gin.Engine, wantTitle string, present, absent []string) {
		t.Helper()
		w := httptest.NewRecorder()
		engine.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/openapi.json", nil))
		if w.Code != http.StatusOK {
			t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
		}
		if ct := w.Header().Get("Content-Type"); !bytes.Contains([]byte(ct), []byte("application/json")) {
			t.Fatalf("content-type=%q", ct)
		}
		var spec struct {
			OpenAPI string `json:"openapi"`
			Info    struct {
				Title string `json:"title"`
			} `json:"info"`
			Paths map[string]json.RawMessage `json:"paths"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &spec); err != nil {
			t.Fatal(err)
		}
		if spec.Info.Title != wantTitle {
			t.Fatalf("title=%q, want %q", spec.Info.Title, wantTitle)
		}
		for _, p := range present {
			if _, ok := spec.Paths[p]; !ok {
				t.Fatalf("missing path %s", p)
			}
		}
		for _, p := range absent {
			if _, ok := spec.Paths[p]; ok {
				t.Fatalf("unexpected cross-plane path %s", p)
			}
		}
	}

	check(t, adminEngine, "KiriVers Admin API", []string{
		"/api/v1/health",
		"/api/v1/openapi.json",
		"/api/v1/admin/auth/login",
		"/api/v1/admin/admins",
		"/api/v1/admin/projects",
		"/api/v1/admin/projects/{project_ref}",
		"/api/v1/admin/projects/{project_ref}/tokens",
		"/api/v1/admin/projects/{project_ref}/tokens/{token_id}",
		"/api/v1/admin/projects/{project_ref}/ci-tokens",
		"/api/v1/admin/projects/{project_ref}/ci-tokens/{token_id}",
		"/api/v1/admin/projects/{project_ref}/channels",
		"/api/v1/admin/projects/{project_ref}/channels/{slug}",
		"/api/v1/admin/projects/{project_ref}/install-policy-rules",
		"/api/v1/admin/projects/{project_ref}/install-policy-rules/reference",
		"/api/v1/admin/projects/{project_ref}/channels/{slug}/install-policy-rules",
		"/api/v1/admin/projects/{project_ref}/languages",
		"/api/v1/admin/projects/{project_ref}/languages/{code}",
		"/api/v1/admin/projects/{project_ref}/matrix",
		"/api/v1/admin/projects/{project_ref}/matrix/{os}/{arch}",
		"/api/v1/admin/projects/{project_ref}/hw-revs",
		"/api/v1/admin/projects/{project_ref}/hw-revs/{slug}",
		"/api/v1/admin/projects/{project_ref}/platforms/catalog",
		"/api/v1/admin/projects/{project_ref}/versions",
		"/api/v1/admin/projects/{project_ref}/versions/{version}",
		"/api/v1/admin/projects/{project_ref}/versions/{version}/exists",
		"/api/v1/admin/projects/{project_ref}/versions/{version}/publish",
		"/api/v1/admin/projects/{project_ref}/versions/{version}/deprecate",
		"/api/v1/admin/projects/{project_ref}/versions/{version}/revoke",
		"/api/v1/admin/projects/{project_ref}/versions/{version}/lines",
		"/api/v1/admin/projects/{project_ref}/versions/{version}/lines/{os}/{arch}",
		"/api/v1/admin/projects/{project_ref}/versions/{version}/lines/{os}/{arch}/ready",
		"/api/v1/admin/projects/{project_ref}/versions/{version}/lines/{os}/{arch}/yank",
		"/api/v1/admin/projects/{project_ref}/versions/{version}/lines/{os}/{arch}/disable",
		"/api/v1/admin/projects/{project_ref}/versions/{version}/lines/{os}/{arch}/manifest",
		"/api/v1/admin/projects/{project_ref}/versions/{version}/lines/{os}/{arch}/build-archive",
		"/api/v1/admin/projects/{project_ref}/ci/releases",
		"/api/v1/admin/projects/{project_ref}/versions/{version}/artifacts/bundle",
		"/api/v1/admin/jobs/{job_id}",
		"/api/v1/admin/geoip/databases",
		"/api/v1/admin/geoip/databases/{id}",
		"/api/v1/admin/projects/{project_ref}/members",
		"/api/v1/admin/projects/{project_ref}/members/{admin_id}",
		"/api/v1/admin/projects/{project_ref}/audit",
		"/api/v1/admin/projects/{project_ref}/telemetry/devices/{device_hash}",
		"/api/v1/admin/projects/{project_ref}/store-listings",
		"/api/v1/admin/projects/{project_ref}/store-listings/{listing_id}",
	}, []string{
		"/api/v1/projects/{project_ref}/update/check",
		"/api/v1/projects/{project_ref}/update/diff",
		"/api/v1/projects/{project_ref}/update/pack",
		"/api/v1/projects/{project_ref}/artifacts/{artifact_id}/{filename}",
		"/api/v1/projects/{project_ref}/store/{protocol}/{path}",
		"/api/v1/ready",
	})

	check(t, clientEngine, "KiriVers Client API", []string{
		"/api/v1/health",
		"/api/v1/openapi.json",
		"/api/v1/projects/{project_ref}",
		"/api/v1/projects/{project_ref}/update/check",
		"/api/v1/projects/{project_ref}/update/diff",
		"/api/v1/projects/{project_ref}/update/pack",
		"/api/v1/projects/{project_ref}/packages/{ref}",
		"/api/v1/projects/{project_ref}/changelog/{channel}/{os}/{arch}",
		"/api/v1/projects/{project_ref}/clients/report",
		"/api/v1/projects/{project_ref}/languages",
		"/api/v1/projects/{project_ref}/telemetry/report",
		"/api/v1/projects/{project_ref}/versions/{version}/integrity",
		"/api/v1/projects/{project_ref}/store/{protocol}/{listing_slug}",
		"/api/v1/projects/{project_ref}/store/{protocol}/{listing_slug}/{doc}",
	}, []string{
		"/api/v1/admin/auth/login",
		"/api/v1/admin/admins",
		"/api/v1/admin/projects/{project_ref}/versions/{version}/publish",
		"/api/v1/projects/{project_ref}/update/pack/status",
		"/api/v1/projects/{project_ref}/versions/{version}/lines/{os}/{arch}/manifest",
		"/api/v1/projects/{project_ref}/channels/{slug}",
		"/api/v1/ready",
	})
}

func TestUnknownRouteJSON(t *testing.T) {
	clientEngine, adminEngine := testEngines(t, nil)
	for name, engine := range map[string]*gin.Engine{"client": clientEngine, "admin": adminEngine} {
		w := httptest.NewRecorder()
		engine.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/no-such", nil))
		if w.Code != http.StatusNotFound {
			t.Fatalf("%s: status=%d", name, w.Code)
		}
		var env response.Body
		if err := json.Unmarshal(w.Body.Bytes(), &env); err != nil {
			t.Fatal(err)
		}
		if env.Error.Code != "NOT_FOUND" {
			t.Fatalf("%s: code=%q", name, env.Error.Code)
		}
	}
}

// TestPlaneRouteIsolation 验证双端口隔离的可观察行为：管理路由只在 admin 引擎、
// 客户端路由只在 client 引擎（AC1 的引擎级等价，真实双端口由 cmd 包 server 模式验收）。
func TestPlaneRouteIsolation(t *testing.T) {
	clientEngine, adminEngine := testEngines(t, func(context.Context) error { return nil })

	// client 引擎上管理登录应 404（NOT_FOUND JSON，而非 gin 默认 404 页）。
	w := httptest.NewRecorder()
	clientEngine.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/api/v1/admin/auth/login", nil))
	if w.Code != http.StatusNotFound {
		t.Fatalf("client engine admin login: status=%d", w.Code)
	}
	var env response.Body
	if err := json.Unmarshal(w.Body.Bytes(), &env); err != nil {
		t.Fatal(err)
	}
	if env.Error.Code != "NOT_FOUND" {
		t.Fatalf("client engine admin login: code=%q", env.Error.Code)
	}

	// admin 引擎上客户端 check 应 404。
	w = httptest.NewRecorder()
	adminEngine.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/projects/demo/update/check", nil))
	if w.Code != http.StatusNotFound {
		t.Fatalf("admin engine client check: status=%d", w.Code)
	}
	if err := json.Unmarshal(w.Body.Bytes(), &env); err != nil {
		t.Fatal(err)
	}
	if env.Error.Code != "NOT_FOUND" {
		t.Fatalf("admin engine client check: code=%q", env.Error.Code)
	}
}

// TestAPI404WhenDBUnavailable 运行中数据库不可用：业务 /api 为 404 NOT_FOUND，
// /health 仍 200（ready=false）、openapi 仍 200；管理台静态页不走 404。
func TestAPI404WhenDBUnavailable(t *testing.T) {
	gin.SetMode(gin.TestMode)
	dist := writeAdminDist(t)
	deps := Deps{
		Ready:          func(context.Context) error { return errors.New("database ping") },
		DBAvailable:    func() bool { return false },
		AdminStaticDir: dist,
	}
	clientEngine := gin.New()
	RegisterClient(clientEngine, deps)
	adminEngine := gin.New()
	RegisterAdmin(adminEngine, deps)

	assertOK := func(engine *gin.Engine, path string) {
		t.Helper()
		w := httptest.NewRecorder()
		engine.ServeHTTP(w, httptest.NewRequest(http.MethodGet, path, nil))
		if w.Code != http.StatusOK {
			t.Fatalf("%s status=%d body=%s", path, w.Code, w.Body.String())
		}
	}
	assertJSONNotFound := func(engine *gin.Engine, method, path string) {
		t.Helper()
		w := httptest.NewRecorder()
		engine.ServeHTTP(w, httptest.NewRequest(method, path, nil))
		if w.Code != http.StatusNotFound {
			t.Fatalf("%s %s status=%d body=%s", method, path, w.Code, w.Body.String())
		}
		var env response.Body
		if err := json.Unmarshal(w.Body.Bytes(), &env); err != nil {
			t.Fatal(err)
		}
		if env.Error.Code != "NOT_FOUND" {
			t.Fatalf("%s %s code=%q", method, path, env.Error.Code)
		}
	}

	for _, engine := range []*gin.Engine{clientEngine, adminEngine} {
		got := getHealth(t, engine)
		if got.Status != "ok" || got.Ready {
			t.Fatalf("health when db down: %+v", got)
		}
		assertOK(engine, "/api/v1/openapi.json")
		assertJSONNotFound(engine, http.MethodGet, "/api/v1/ready")
	}
	assertJSONNotFound(clientEngine, http.MethodGet, "/api/v1/projects/demo")
	assertJSONNotFound(adminEngine, http.MethodPost, "/api/v1/admin/auth/login")

	w := httptest.NewRecorder()
	adminEngine.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/login", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("admin UI status=%d body=%s", w.Code, w.Body.String())
	}
}
