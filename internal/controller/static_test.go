package controller

import (
	"context"
	"encoding/json"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/gin-gonic/gin"

	"github.com/Kirizu-Official/KiriVers/pkg/response"
)

const (
	staticIndexBody = "<!doctype html><html><body>kiri-admin</body></html>\n"
	staticAssetBody = "console.log('kiri-admin-app');\n"
	staticSecret    = "TOPSECRET-OUTSIDE-DIST"
)

func writeAdminDist(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "assets"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "index.html"), []byte(staticIndexBody), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "assets", "app.js"), []byte(staticAssetBody), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

func newAdminStaticEngine(t *testing.T, staticDir string) *gin.Engine {
	t.Helper()
	return newAdminStaticEngineFS(t, staticDir, nil)
}

func newAdminStaticEngineFS(t *testing.T, staticDir string, embedded fs.FS) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	RegisterAdmin(engine, Deps{
		AdminStaticDir: staticDir,
		AdminStaticFS:  embedded,
		Ready:          func(context.Context) error { return nil },
	})
	return engine
}

func mapAdminDist() fstest.MapFS {
	return fstest.MapFS{
		"index.html":    {Data: []byte(staticIndexBody)},
		"assets/app.js": {Data: []byte(staticAssetBody)},
	}
}

func serveAdminPath(engine *gin.Engine, method, path string) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, httptest.NewRequest(method, path, nil))
	return w
}

func assertJSONNotFound(t *testing.T, w *httptest.ResponseRecorder) {
	t.Helper()
	if w.Code != http.StatusNotFound {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	body := w.Body.String()
	if strings.Contains(strings.ToLower(body), "<html") {
		t.Fatalf("404 body must not be HTML: %s", body)
	}
	var env response.Body
	if err := json.Unmarshal(w.Body.Bytes(), &env); err != nil {
		t.Fatalf("json: %v body=%s", err, body)
	}
	if env.Error.Code != "NOT_FOUND" {
		t.Fatalf("code=%q", env.Error.Code)
	}
}

func TestAdminStaticServesIndexAndAssets(t *testing.T) {
	engine := newAdminStaticEngine(t, writeAdminDist(t))

	w := serveAdminPath(engine, http.MethodGet, "/")
	if w.Code != http.StatusOK {
		t.Fatalf("GET / status=%d body=%s", w.Code, w.Body.String())
	}
	if w.Body.String() != staticIndexBody {
		t.Fatalf("GET / body=%q", w.Body.String())
	}
	if ct := w.Header().Get("Content-Type"); !strings.Contains(ct, "html") {
		t.Fatalf("GET / content-type=%q", ct)
	}

	w = serveAdminPath(engine, http.MethodHead, "/")
	if w.Code != http.StatusOK {
		t.Fatalf("HEAD / status=%d", w.Code)
	}

	w = serveAdminPath(engine, http.MethodGet, "/assets/app.js")
	if w.Code != http.StatusOK {
		t.Fatalf("GET asset status=%d body=%s", w.Code, w.Body.String())
	}
	if w.Body.String() != staticAssetBody {
		t.Fatalf("GET asset body=%q", w.Body.String())
	}
	if ct := w.Header().Get("Content-Type"); strings.Contains(ct, "json") {
		t.Fatalf("asset must not be JSON, content-type=%q", ct)
	}
}

func TestAdminStaticSPAFallbackAndHashedAsset404(t *testing.T) {
	engine := newAdminStaticEngine(t, writeAdminDist(t))

	w := serveAdminPath(engine, http.MethodGet, "/login")
	if w.Code != http.StatusOK {
		t.Fatalf("GET /login status=%d body=%s", w.Code, w.Body.String())
	}
	if w.Body.String() != staticIndexBody {
		t.Fatalf("GET /login must return index.html, body=%q", w.Body.String())
	}

	w = serveAdminPath(engine, http.MethodGet, "/assets/missing.js")
	assertJSONNotFound(t, w)
	if strings.Contains(w.Body.String(), "<html") {
		t.Fatal("missing hashed asset must not return HTML")
	}
}

func TestAdminStaticAPIStillJSON(t *testing.T) {
	engine := newAdminStaticEngine(t, writeAdminDist(t))

	w := serveAdminPath(engine, http.MethodGet, "/api/v1/no-such")
	assertJSONNotFound(t, w)

	w = serveAdminPath(engine, http.MethodGet, "/api/v1/projects/demo/update/check")
	assertJSONNotFound(t, w)

	w = serveAdminPath(engine, http.MethodGet, "/api")
	assertJSONNotFound(t, w)
	w = serveAdminPath(engine, http.MethodGet, "/api/")
	assertJSONNotFound(t, w)

	w = serveAdminPath(engine, http.MethodPost, "/login")
	assertJSONNotFound(t, w)
}

func TestAdminProxiesClientProjects(t *testing.T) {
	gin.SetMode(gin.TestMode)
	backend := gin.New()
	backend.GET("/api/v1/projects/:project_ref/announcements", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"announcements": []any{}})
	})
	srv := httptest.NewServer(backend)
	t.Cleanup(srv.Close)

	engine := gin.New()
	RegisterAdmin(engine, Deps{
		AdminStaticDir: writeAdminDist(t),
		ClientProxyURL: srv.URL,
		Ready:          func(context.Context) error { return nil },
	})

	w := serveAdminPath(engine, http.MethodGet, "/api/v1/projects/demo/announcements")
	if w.Code != http.StatusOK {
		t.Fatalf("proxied announcements status=%d body=%s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "announcements") {
		t.Fatalf("proxied body=%s", w.Body.String())
	}

	w = serveAdminPath(engine, http.MethodGet, "/api/v1/admin/projects/demo")
	assertJSONNotFound(t, w)
}

func TestAdminStaticDisabledOrMissingIndex(t *testing.T) {
	emptyEngine := newAdminStaticEngine(t, "")
	w := serveAdminPath(emptyEngine, http.MethodGet, "/")
	assertJSONNotFound(t, w)
	w = serveAdminPath(emptyEngine, http.MethodGet, "/api/v1/health")
	if w.Code != http.StatusOK {
		t.Fatalf("health with empty static_dir status=%d", w.Code)
	}

	dir := t.TempDir()
	missingEngine := newAdminStaticEngine(t, dir)
	w = serveAdminPath(missingEngine, http.MethodGet, "/")
	assertJSONNotFound(t, w)
	w = serveAdminPath(missingEngine, http.MethodGet, "/api/v1/health")
	if w.Code != http.StatusOK {
		t.Fatalf("health without index.html status=%d", w.Code)
	}

	dirHTML := t.TempDir()
	if err := os.Mkdir(filepath.Join(dirHTML, "index.html"), 0o755); err != nil {
		t.Fatal(err)
	}
	dirIndexEngine := newAdminStaticEngine(t, dirHTML)
	w = serveAdminPath(dirIndexEngine, http.MethodGet, "/")
	assertJSONNotFound(t, w)
}

func TestAdminStaticIndexAppearsWithoutRestart(t *testing.T) {
	dir := t.TempDir()
	engine := newAdminStaticEngine(t, dir)

	w := serveAdminPath(engine, http.MethodGet, "/")
	assertJSONNotFound(t, w)

	if err := os.WriteFile(filepath.Join(dir, "index.html"), []byte(staticIndexBody), 0o644); err != nil {
		t.Fatal(err)
	}
	w = serveAdminPath(engine, http.MethodGet, "/")
	if w.Code != http.StatusOK || w.Body.String() != staticIndexBody {
		t.Fatalf("index.html after build: status=%d body=%q", w.Code, w.Body.String())
	}

	if err := os.Remove(filepath.Join(dir, "index.html")); err != nil {
		t.Fatal(err)
	}
	w = serveAdminPath(engine, http.MethodGet, "/")
	assertJSONNotFound(t, w)
}

func TestAdminStaticRejectsPathTraversal(t *testing.T) {
	parent := t.TempDir()
	dist := filepath.Join(parent, "dist")
	if err := os.MkdirAll(filepath.Join(dist, "assets"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dist, "index.html"), []byte(staticIndexBody), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(parent, "secret.txt"), []byte(staticSecret), 0o644); err != nil {
		t.Fatal(err)
	}

	engine := newAdminStaticEngine(t, dist)
	for _, p := range []string{
		"/../secret.txt",
		"/%2e%2e/secret.txt",
		"/assets/../../secret.txt",
		"/..\\secret.txt",
	} {
		w := serveAdminPath(engine, http.MethodGet, p)
		if strings.Contains(w.Body.String(), staticSecret) {
			t.Fatalf("path %q leaked outside file: %s", p, w.Body.String())
		}
		assertJSONNotFound(t, w)
	}
}

func TestAdminStaticDoesNotListDirectories(t *testing.T) {
	engine := newAdminStaticEngine(t, writeAdminDist(t))
	w := serveAdminPath(engine, http.MethodGet, "/assets")
	if w.Code != http.StatusOK {
		t.Fatalf("GET /assets status=%d body=%s", w.Code, w.Body.String())
	}
	if w.Body.String() != staticIndexBody {
		t.Fatalf("directory must not be listed, body=%q", w.Body.String())
	}
}

func TestAdminStaticDoesNotRegisterWildcardRoute(t *testing.T) {
	engine := newAdminStaticEngine(t, writeAdminDist(t))
	for _, r := range engine.Routes() {
		if strings.Contains(r.Path, "*") {
			t.Fatalf("static hosting must not register gin.Static path %s %s", r.Method, r.Path)
		}
	}
}

func TestClientPlaneDoesNotServeAdminStatic(t *testing.T) {
	gin.SetMode(gin.TestMode)
	client := gin.New()
	RegisterClient(client, Deps{
		Ready:          func(context.Context) error { return nil },
		AdminStaticDir: writeAdminDist(t),
		AdminStaticFS:  mapAdminDist(),
	})

	w := serveAdminPath(client, http.MethodGet, "/")
	assertJSONNotFound(t, w)
	w = serveAdminPath(client, http.MethodGet, "/login")
	assertJSONNotFound(t, w)
}

func TestAdminStaticEmbeddedFallback(t *testing.T) {
	engine := newAdminStaticEngineFS(t, t.TempDir(), mapAdminDist())

	w := serveAdminPath(engine, http.MethodGet, "/")
	if w.Code != http.StatusOK || w.Body.String() != staticIndexBody {
		t.Fatalf("GET / embed status=%d body=%q", w.Code, w.Body.String())
	}

	w = serveAdminPath(engine, http.MethodGet, "/assets/app.js")
	if w.Code != http.StatusOK || w.Body.String() != staticAssetBody {
		t.Fatalf("GET asset embed status=%d body=%q", w.Code, w.Body.String())
	}

	w = serveAdminPath(engine, http.MethodGet, "/login")
	if w.Code != http.StatusOK || w.Body.String() != staticIndexBody {
		t.Fatalf("GET /login embed status=%d body=%q", w.Code, w.Body.String())
	}

	w = serveAdminPath(engine, http.MethodGet, "/assets")
	if w.Code != http.StatusOK || w.Body.String() != staticIndexBody {
		t.Fatalf("GET /assets embed must not list directory, body=%q", w.Body.String())
	}

	w = serveAdminPath(engine, http.MethodGet, "/assets/missing.js")
	assertJSONNotFound(t, w)

	w = serveAdminPath(engine, http.MethodGet, "/api/v1/no-such")
	assertJSONNotFound(t, w)
}

func TestAdminStaticDiskOverridesEmbed(t *testing.T) {
	const diskBody = "<!doctype html><html><body>disk-ui</body></html>\n"
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "index.html"), []byte(diskBody), 0o644); err != nil {
		t.Fatal(err)
	}
	engine := newAdminStaticEngineFS(t, dir, mapAdminDist())
	w := serveAdminPath(engine, http.MethodGet, "/")
	if w.Code != http.StatusOK || w.Body.String() != diskBody {
		t.Fatalf("disk must win over embed: status=%d body=%q", w.Code, w.Body.String())
	}
}

func TestAdminStaticEmptyDirDisablesEmbed(t *testing.T) {
	engine := newAdminStaticEngineFS(t, "", mapAdminDist())
	w := serveAdminPath(engine, http.MethodGet, "/")
	assertJSONNotFound(t, w)
	w = serveAdminPath(engine, http.MethodGet, "/api/v1/health")
	if w.Code != http.StatusOK {
		t.Fatalf("health with empty static_dir status=%d", w.Code)
	}
}
