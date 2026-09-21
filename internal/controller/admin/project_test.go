package admin

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/Kirizu-Official/KiriVers/internal/controller/client"
	"github.com/Kirizu-Official/KiriVers/internal/model"
	"github.com/Kirizu-Official/KiriVers/internal/repository"
	"github.com/Kirizu-Official/KiriVers/internal/service"
	"github.com/Kirizu-Official/KiriVers/internal/service/update"
	"github.com/Kirizu-Official/KiriVers/internal/storage"
	"github.com/Kirizu-Official/KiriVers/pkg/hashutil"
	"github.com/Kirizu-Official/KiriVers/pkg/response"
)

func setupProjectHTTP(t *testing.T) (*gin.Engine, *service.ProjectService, *repository.MemoryProjectStore, string) {
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
	projSvc.SetInstallPolicyStore(repository.NewMemoryInstallPolicyRuleStore())
	r := gin.New()
	Register(r.Group("/api/v1/admin"), adminSvc, projSvc, nil, nil, nil, nil, nil)
	client.Register(r.Group("/api/v1"), projSvc, update.NewService(repository.NewMemoryUpdateCatalog(store)), nil, nil, nil, nil)
	return r, projSvc, store, login.Token
}

func doJSON(r http.Handler, method, path, token string, body any) *httptest.ResponseRecorder {
	var buf bytes.Buffer
	if body != nil {
		_ = json.NewEncoder(&buf).Encode(body)
	}
	req := httptest.NewRequest(method, path, &buf)
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func decodeErr(t *testing.T, w *httptest.ResponseRecorder) string {
	t.Helper()
	var env response.Body
	if err := json.Unmarshal(w.Body.Bytes(), &env); err != nil {
		t.Fatalf("decode error: %v body=%s", err, w.Body.String())
	}
	return env.Error.Code
}

func assertAdminProjectStats(t *testing.T, stats any) {
	t.Helper()
	m, ok := stats.(map[string]any)
	if !ok {
		t.Fatalf("stats type %T", stats)
	}
	for _, key := range []string{
		"latest_version", "versions", "channels", "matrix_rows", "hw_revs",
		"project_tokens", "ci_tokens", "artifact_count", "storage_bytes", "telemetry",
	} {
		if _, exists := m[key]; !exists {
			t.Fatalf("stats missing %s: %#v", key, m)
		}
	}
	raw, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	for _, leak := range []string{"device_hash", "device_id", "fingerprint", "token_hash"} {
		if bytes.Contains(raw, []byte(`"`+leak+`"`)) {
			t.Fatalf("stats leaked %s: %s", leak, raw)
		}
	}
}

func TestProjectHTTPAcceptance(t *testing.T) {
	r, projSvc, store, adminTok := setupProjectHTTP(t)

	t.Run("missing bearer is 401 UNAUTHORIZED", func(t *testing.T) {
		w := doJSON(r, http.MethodPost, "/api/v1/admin/projects", "", map[string]string{"slug": "no-auth"})
		if w.Code != http.StatusUnauthorized {
			t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
		}
		if decodeErr(t, w) != "UNAUTHORIZED" {
			t.Fatalf("code=%s", decodeErr(t, w))
		}
	})

	t.Run("illegal slug is 400", func(t *testing.T) {
		w := doJSON(r, http.MethodPost, "/api/v1/admin/projects", adminTok, map[string]string{"slug": "no spaces!"})
		if w.Code != http.StatusBadRequest {
			t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
		}
		if decodeErr(t, w) != "INVALID_REQUEST" {
			t.Fatalf("code=%s body=%s", decodeErr(t, w), w.Body.String())
		}
	})

	t.Run("bootstrap_admin_token rejected", func(t *testing.T) {
		w := doJSON(r, http.MethodPost, "/api/v1/admin/projects", adminTok, map[string]any{
			"slug":                  "boot-no",
			"bootstrap_admin_token": "secret",
		})
		if w.Code != http.StatusBadRequest {
			t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
		}
		if !strings.Contains(w.Body.String(), "bootstrap_admin_token") {
			t.Fatalf("expected reject message: %s", w.Body.String())
		}
		if bytes.Contains(w.Body.Bytes(), []byte(`"bootstrap_admin_token"`)) && w.Code == http.StatusCreated {
			t.Fatal("API accepted bootstrap_admin_token")
		}
	})

	w := doJSON(r, http.MethodPost, "/api/v1/admin/projects", adminTok, map[string]any{
		"slug":           "my-app",
		"default_locale": "en",
	})
	if w.Code != http.StatusCreated {
		t.Fatalf("create=%d %s", w.Code, w.Body.String())
	}
	var created map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	uuidStr, _ := created["uuid"].(string)
	if uuidStr == "" || created["slug"] != "my-app" {
		t.Fatalf("create body=%s", w.Body.String())
	}
	if created["name"] != "my-app" {
		t.Fatalf("default name=%s", w.Body.String())
	}
	if _, ok := created["stats"]; ok {
		t.Fatalf("create must omit stats: %s", w.Body.String())
	}
	parsedID, err := uuid.Parse(uuidStr)
	if err != nil || parsedID.Version() != 4 {
		t.Fatalf("uuid v4 required, got %q version=%d err=%v", uuidStr, parsedID.Version(), err)
	}
	if created["compare_engine"] != "semver" {
		t.Fatalf("defaults=%s", w.Body.String())
	}
	if created["storage_driver"] != "local" {
		t.Fatalf("storage_driver=%v", created["storage_driver"])
	}
	if _, ok := created["min_client_protocol"]; ok {
		t.Fatalf("min_client_protocol must be absent: %s", w.Body.String())
	}
	if _, ok := created["signing_private_key"]; ok {
		t.Fatalf("leaked private key: %s", w.Body.String())
	}
	if _, ok := created["store_token_hash"]; ok {
		t.Fatalf("leaked feed hash: %s", w.Body.String())
	}
	if _, ok := created["bootstrap_admin_token"]; ok {
		t.Fatalf("bootstrap field in response: %s", w.Body.String())
	}

	wSlug := doJSON(r, http.MethodGet, "/api/v1/admin/projects/my-app", adminTok, nil)
	wUUID := doJSON(r, http.MethodGet, "/api/v1/admin/projects/"+uuidStr, adminTok, nil)
	if wSlug.Code != http.StatusOK || wUUID.Code != http.StatusOK {
		t.Fatalf("get slug=%d uuid=%d", wSlug.Code, wUUID.Code)
	}
	var bySlug, byUUID any
	if err := json.Unmarshal(wSlug.Body.Bytes(), &bySlug); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(wUUID.Body.Bytes(), &byUUID); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(bySlug, byUUID) {
		t.Fatalf("slug vs uuid mismatch\nslug=%s\nuuid=%s", wSlug.Body.String(), wUUID.Body.String())
	}
	got, _ := bySlug.(map[string]any)
	if got["name"] != "my-app" {
		t.Fatalf("get name=%v", got["name"])
	}
	if _, ok := got["stats"]; !ok {
		t.Fatalf("get missing stats: %s", wSlug.Body.String())
	}
	assertAdminProjectStats(t, got["stats"])
	if stats, _ := got["stats"].(map[string]any); stats["latest_version"] != nil {
		t.Fatalf("empty catalog latest_version=%v", stats["latest_version"])
	}

	wList := doJSON(r, http.MethodGet, "/api/v1/admin/projects", adminTok, nil)
	if wList.Code != http.StatusOK {
		t.Fatalf("list=%d %s", wList.Code, wList.Body.String())
	}
	var listed struct {
		Projects []map[string]any `json:"projects"`
	}
	if err := json.Unmarshal(wList.Body.Bytes(), &listed); err != nil {
		t.Fatal(err)
	}
	var listedApp map[string]any
	for _, item := range listed.Projects {
		if item["slug"] == "my-app" {
			listedApp = item
			break
		}
	}
	if listedApp == nil {
		t.Fatalf("list missing my-app: %s", wList.Body.String())
	}
	if listedApp["name"] != "my-app" {
		t.Fatalf("list name=%v", listedApp["name"])
	}
	assertAdminProjectStats(t, listedApp["stats"])

	w = doJSON(r, http.MethodPatch, "/api/v1/admin/projects/my-app", adminTok, map[string]any{"name": "Acme"})
	if w.Code != http.StatusOK {
		t.Fatalf("patch name=%d %s", w.Code, w.Body.String())
	}
	var named map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &named); err != nil {
		t.Fatal(err)
	}
	if named["name"] != "Acme" {
		t.Fatalf("patched name=%s", w.Body.String())
	}
	w = doJSON(r, http.MethodPatch, "/api/v1/admin/projects/my-app", adminTok, map[string]any{"name": "  "})
	if w.Code != http.StatusBadRequest || decodeErr(t, w) != "INVALID_REQUEST" {
		t.Fatalf("blank name=%d %s", w.Code, w.Body.String())
	}

	w = doJSON(r, http.MethodPost, "/api/v1/admin/projects", adminTok, map[string]any{
		"slug":           "named-app",
		"name":           "Named Product",
		"default_locale": "en",
	})
	if w.Code != http.StatusCreated {
		t.Fatalf("create named=%d %s", w.Code, w.Body.String())
	}
	var createdNamed map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &createdNamed); err != nil {
		t.Fatal(err)
	}
	if createdNamed["name"] != "Named Product" || createdNamed["slug"] != "named-app" {
		t.Fatalf("create named body=%s", w.Body.String())
	}
	if _, ok := createdNamed["stats"]; ok {
		t.Fatalf("named create must omit stats: %s", w.Body.String())
	}

	w = doJSON(r, http.MethodPost, "/api/v1/admin/projects", adminTok, map[string]any{
		"slug":           "blank-name-app",
		"name":           "   ",
		"default_locale": "en",
	})
	if w.Code != http.StatusBadRequest || decodeErr(t, w) != "INVALID_REQUEST" {
		t.Fatalf("create whitespace name=%d %s", w.Code, w.Body.String())
	}

	w = doJSON(r, http.MethodPost, "/api/v1/admin/projects", adminTok, map[string]string{"slug": "my-app"})
	if w.Code != http.StatusConflict || decodeErr(t, w) != "SLUG_TAKEN" {
		t.Fatalf("dup slug=%d %s", w.Code, w.Body.String())
	}

	w = doJSON(r, http.MethodPatch, "/api/v1/admin/projects/my-app", adminTok, map[string]any{
		"minimum_supported_version":   "1.0.0",
		"default_locale":              "zh-CN",
		"storage_visibility":          "public",
		"gray_weight_tenure_activity": false,
		"store_protocols": map[string]any{
			"sparkle": map[string]any{
				"enabled":     true,
				"identifiers": map[string]string{"bundle_id": "app.example.kit"},
			},
			"winget": map[string]any{
				"enabled":     false,
				"identifiers": map[string]string{"package_identifier": "Example.Kit"},
			},
		},
		"compare_engine": "integer",
	})
	if w.Code != http.StatusOK {
		t.Fatalf("patch settings=%d %s", w.Code, w.Body.String())
	}
	var patched map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &patched); err != nil {
		t.Fatal(err)
	}
	if patched["minimum_supported_version"] != "1.0.0" {
		t.Fatalf("minmax=%s", w.Body.String())
	}
	if _, ok := patched["min_client_protocol"]; ok {
		t.Fatalf("min_client_protocol must be absent: %s", w.Body.String())
	}
	if patched["default_locale"] != "zh-CN" || patched["storage_visibility"] != "public" {
		t.Fatalf("locale/storage=%s", w.Body.String())
	}
	if patched["gray_weight_tenure_activity"] != false || patched["compare_engine"] != "integer" {
		t.Fatalf("gray_weight/engine=%s", w.Body.String())
	}
	if _, ok := patched["store_protocols"]; ok {
		t.Fatalf("store_protocols must be absent: %s", w.Body.String())
	}

	w = doJSON(r, http.MethodPost, "/api/v1/admin/projects/"+uuidStr+"/tokens", adminTok, map[string]any{
		"name":   "ci-bot",
		"scopes": []string{model.ScopeProjectAdmin, model.ScopeProjectRead},
	})
	if w.Code != http.StatusCreated {
		t.Fatalf("create token=%d %s", w.Code, w.Body.String())
	}
	var issued struct {
		Token       string   `json:"token"`
		Fingerprint string   `json:"fingerprint"`
		ID          string   `json:"id"`
		Scopes      []string `json:"scopes"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &issued); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(issued.Token, "kv_") || issued.Fingerprint == "" {
		t.Fatalf("token body=%s", w.Body.String())
	}

	w = doJSON(r, http.MethodPost, "/api/v1/admin/projects", issued.Token, map[string]string{"slug": "from-project-token"})
	if w.Code != http.StatusForbidden || decodeErr(t, w) != "FORBIDDEN" {
		t.Fatalf("project token create=%d %s", w.Code, w.Body.String())
	}

	ciPlain := "kv_" + strings.Repeat("ab", 32)
	ciHash, err := hashutil.SHA256Hex(strings.NewReader(ciPlain))
	if err != nil {
		t.Fatal(err)
	}
	if err := projSvc.SeedCIToken(t.Context(), &model.CIToken{
		ProjectID:   uuid.MustParse(uuidStr),
		Name:        "ci",
		Scopes:      model.StringList{model.ScopeArtifactWrite},
		TokenHash:   ciHash,
		Fingerprint: ciPlain[:8],
	}); err != nil {
		t.Fatal(err)
	}
	w = doJSON(r, http.MethodPost, "/api/v1/admin/projects", ciPlain, map[string]string{"slug": "from-ci-token"})
	if w.Code != http.StatusForbidden || decodeErr(t, w) != "FORBIDDEN" {
		t.Fatalf("ci token create=%d %s", w.Code, w.Body.String())
	}

	w = doJSON(r, http.MethodPatch, "/api/v1/admin/projects/my-app", adminTok, map[string]any{"slug": "my-app-v2"})
	if w.Code != http.StatusOK {
		t.Fatalf("rename=%d %s", w.Code, w.Body.String())
	}
	wOld := doJSON(r, http.MethodGet, "/api/v1/admin/projects/my-app", adminTok, nil)
	if wOld.Code != http.StatusOK {
		t.Fatalf("old slug after rename=%d %s", wOld.Code, wOld.Body.String())
	}
	var oldBody map[string]any
	_ = json.Unmarshal(wOld.Body.Bytes(), &oldBody)
	if oldBody["slug"] != "my-app-v2" || oldBody["uuid"] != uuidStr {
		t.Fatalf("alias body=%s", wOld.Body.String())
	}

	wTokOld := doJSON(r, http.MethodPost, "/api/v1/admin/projects/my-app/tokens", adminTok, map[string]any{
		"name":   "via-alias",
		"scopes": []string{model.ScopeProjectRead},
	})
	if wTokOld.Code == http.StatusMovedPermanently || wTokOld.Code == http.StatusFound {
		t.Fatalf("POST used redirect %d", wTokOld.Code)
	}
	if wTokOld.Code != http.StatusCreated {
		t.Fatalf("POST tokens via old slug=%d %s", wTokOld.Code, wTokOld.Body.String())
	}

	store.ExpireAlias("my-app", time.Now().UTC().Add(-time.Hour))
	wExp := doJSON(r, http.MethodGet, "/api/v1/admin/projects/my-app", adminTok, nil)
	if wExp.Code != http.StatusNotFound || decodeErr(t, wExp) != "PROJECT_NOT_FOUND" {
		t.Fatalf("expired alias=%d %s", wExp.Code, wExp.Body.String())
	}

	if _, err := projSvc.CreatePublishedVersion(t.Context(), uuid.MustParse(uuidStr)); err != nil {
		t.Fatal(err)
	}
	w = doJSON(r, http.MethodPatch, "/api/v1/admin/projects/my-app-v2", adminTok, map[string]any{
		"compare_engine": "semver",
	})
	if w.Code != http.StatusConflict || decodeErr(t, w) != "COMPARE_ENGINE_IMMUTABLE" {
		t.Fatalf("lock=%d %s", w.Code, w.Body.String())
	}

	w = doJSON(r, http.MethodDelete, "/api/v1/admin/projects/my-app-v2", adminTok, nil)
	if w.Code != http.StatusNoContent {
		t.Fatalf("delete=%d %s", w.Code, w.Body.String())
	}
	w = doJSON(r, http.MethodGet, "/api/v1/admin/projects/"+uuidStr, adminTok, nil)
	if w.Code != http.StatusNotFound || decodeErr(t, w) != "PROJECT_NOT_FOUND" {
		t.Fatalf("deleted get=%d %s", w.Code, w.Body.String())
	}
}

func TestProjectPublicSettingsCORSAndHTTPS(t *testing.T) {
	r, projSvc, _, _ := setupProjectHTTP(t)
	slug := "pub-app"
	origin := "https://app.example"
	_, _, err := projSvc.Create(t.Context(), service.CreateProjectInput{DefaultLocale: ptr("en"), Slug: &slug,
		ForceHTTPS:  boolPtr(true),
		CORSOrigins: &[]string{origin},
	})
	if err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/projects/pub-app", nil)
	req.Header.Set("Origin", origin)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden || decodeErr(t, w) != "FORBIDDEN" {
		t.Fatalf("force https without proto=%d %s", w.Code, w.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/api/v1/projects/pub-app", nil)
	req.Header.Set("Origin", origin)
	req.Header.Set("X-Forwarded-Proto", "https")
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("https forwarded=%d %s", w.Code, w.Body.String())
	}
	if w.Header().Get("Access-Control-Allow-Origin") != origin {
		t.Fatalf("cors=%q body=%s", w.Header().Get("Access-Control-Allow-Origin"), w.Body.String())
	}
	if !strings.Contains(w.Header().Get("Access-Control-Allow-Headers"), "X-Project-Token") {
		t.Fatalf("cors headers=%q", w.Header().Get("Access-Control-Allow-Headers"))
	}
	if !strings.Contains(w.Header().Get("Access-Control-Allow-Headers"), "X-Channel-Token") {
		t.Fatalf("cors missing X-Channel-Token: %q", w.Header().Get("Access-Control-Allow-Headers"))
	}
	if strings.Contains(w.Body.String(), "signing_private_key") || strings.Contains(w.Body.String(), "store_token") {
		t.Fatalf("public leaked secrets: %s", w.Body.String())
	}

	var pubBySlug map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &pubBySlug); err != nil {
		t.Fatal(err)
	}
	created, err := projSvc.Resolve(t.Context(), slug)
	if err != nil {
		t.Fatal(err)
	}
	req = httptest.NewRequest(http.MethodGet, "/api/v1/projects/"+created.ID.String(), nil)
	req.Header.Set("Origin", origin)
	req.Header.Set("X-Forwarded-Proto", "https")
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("public uuid get=%d %s", w.Code, w.Body.String())
	}
	var pubByUUID map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &pubByUUID); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(pubBySlug, pubByUUID) {
		t.Fatalf("public slug vs uuid mismatch\nslug=%v\nuuid=%v", pubBySlug, pubByUUID)
	}

	req = httptest.NewRequest(http.MethodOptions, "/api/v1/projects/pub-app", nil)
	req.Header.Set("Origin", origin)
	req.Header.Set("Access-Control-Request-Method", http.MethodGet)
	req.Header.Set("Access-Control-Request-Headers", "X-Project-Token")
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusNoContent {
		t.Fatalf("preflight=%d %s", w.Code, w.Body.String())
	}
	if w.Header().Get("Access-Control-Allow-Origin") != origin {
		t.Fatalf("preflight origin=%q", w.Header().Get("Access-Control-Allow-Origin"))
	}
	if !strings.Contains(w.Header().Get("Access-Control-Allow-Headers"), "X-Project-Token") {
		t.Fatalf("preflight allow-headers=%q", w.Header().Get("Access-Control-Allow-Headers"))
	}
	if !strings.Contains(w.Header().Get("Access-Control-Allow-Headers"), "X-Channel-Token") {
		t.Fatalf("preflight missing X-Channel-Token: %q", w.Header().Get("Access-Control-Allow-Headers"))
	}

	_, _, err = projSvc.Patch(t.Context(), slug, service.CreateProjectInput{DefaultLocale: ptr("en"), RequireClientToken: boolPtr(true),
		ForceHTTPS: boolPtr(false),
	})
	if err != nil {
		t.Fatal(err)
	}
	w = doJSON(r, http.MethodGet, "/api/v1/projects/pub-app", "", nil)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("require token=%d %s", w.Code, w.Body.String())
	}

	issued, err := projSvc.CreateToken(t.Context(), slug, "client", []string{model.ScopeProjectRead}, nil)
	if err != nil {
		t.Fatal(err)
	}
	req = httptest.NewRequest(http.MethodGet, "/api/v1/projects/pub-app", nil)
	req.Header.Set("X-Project-Token", issued.Plaintext)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("client token=%d %s", w.Code, w.Body.String())
	}
}

func TestProjectSettingsRoundTripAndTokenScopes(t *testing.T) {
	r, _, _, adminTok := setupProjectHTTP(t)
	w := doJSON(r, http.MethodPost, "/api/v1/admin/projects", adminTok, map[string]any{
		"slug":           "settings-app",
		"default_locale": "en",
	})
	if w.Code != http.StatusCreated {
		t.Fatalf("create=%d %s", w.Code, w.Body.String())
	}
	var created map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	ref := created["uuid"].(string)

	feed := "feed-plain-token"
	priv := "test-ed25519-private"
	w = doJSON(r, http.MethodPatch, "/api/v1/admin/projects/"+ref, adminTok, map[string]any{
		"require_client_token":             true,
		"store_token":                      feed,
		"force_https":                      true,
		"cors_origins":                     []string{"https://desk.example"},
		"device_id_policy":                 "raw",
		"webhook_url":                      "https://hooks.example/p",
		"signing_algo":                     "rsa-sha256",
		"signing_public_key":               "pub-material",
		"signing_private_key":              priv,
		"changelog_scope":                  "target_only",
		"changelog_layout":                 "structured",
		"changelog_client_override":        false,
		"changelog_include_revoked":        false,
		"changelog_include_platform_notes": false,
		"storage_prefix":                   "proj-prefix",
		"storage_bucket":                   "proj-bucket",
		"rate_limit": map[string]any{
			"check_per_device_per_minute": 10,
		},
		"store_protocols": map[string]any{
			"fdroid": map[string]any{
				"enabled":     true,
				"identifiers": map[string]string{"package_name": "org.example.app"},
			},
		},
	})
	if w.Code != http.StatusOK {
		t.Fatalf("patch all=%d %s", w.Code, w.Body.String())
	}
	var patched map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &patched); err != nil {
		t.Fatal(err)
	}
	if patched["store_token"] != feed || patched["has_store_token"] != true {
		t.Fatalf("feed write=%s", w.Body.String())
	}
	if patched["signing_private_key"] != nil {
		t.Fatalf("leaked private key on patch: %s", w.Body.String())
	}
	if patched["has_signing_private_key"] != true || patched["signing_public_key"] != "pub-material" {
		t.Fatalf("signing=%s", w.Body.String())
	}

	wGet := doJSON(r, http.MethodGet, "/api/v1/admin/projects/"+ref, adminTok, nil)
	if wGet.Code != http.StatusOK {
		t.Fatalf("get after patch=%d %s", wGet.Code, wGet.Body.String())
	}
	var got map[string]any
	if err := json.Unmarshal(wGet.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if _, ok := got["store_token"]; ok {
		t.Fatalf("GET listed store_token: %s", wGet.Body.String())
	}
	if got["has_store_token"] != true || got["require_client_token"] != true {
		t.Fatalf("token flags=%s", wGet.Body.String())
	}
	if got["force_https"] != true || got["device_id_policy"] != "raw" {
		t.Fatalf("https/device=%s", wGet.Body.String())
	}
	if got["webhook_url"] != "https://hooks.example/p" || got["signing_algo"] != "rsa-sha256" {
		t.Fatalf("webhook/signing=%s", wGet.Body.String())
	}
	if got["changelog_scope"] != "target_only" || got["changelog_layout"] != "structured" {
		t.Fatalf("changelog=%s", wGet.Body.String())
	}
	if got["changelog_client_override"] != false || got["changelog_include_revoked"] != false {
		t.Fatalf("changelog flags=%s", wGet.Body.String())
	}
	if got["changelog_include_platform_notes"] != false {
		t.Fatalf("platform notes=%s", wGet.Body.String())
	}
	if got["storage_prefix"] != "proj-prefix" || got["storage_bucket"] != "proj-bucket" {
		t.Fatalf("storage override=%s", wGet.Body.String())
	}
	rate, _ := got["rate_limit"].(map[string]any)
	if rate["check_per_device_per_minute"] != float64(10) {
		t.Fatalf("rate_limit=%s", wGet.Body.String())
	}
	if _, ok := got["store_protocols"]; ok {
		t.Fatalf("store_protocols must be absent: %s", wGet.Body.String())
	}
	if got["file_list_max_files"] != float64(0) {
		t.Fatalf("stored file_list_max_files=%v", got["file_list_max_files"])
	}
	if got["file_list_max_files_limit"] != float64(16) {
		t.Fatalf("file_list_max_files_limit=%v", got["file_list_max_files_limit"])
	}
	if got["changelog_default_entries"] != float64(5) {
		t.Fatalf("changelog_default_entries=%v", got["changelog_default_entries"])
	}
	if got["changelog_max_entries"] != float64(0) {
		t.Fatalf("changelog_max_entries=%v", got["changelog_max_entries"])
	}
	if got["changelog_default_entries_limit"] != float64(5) {
		t.Fatalf("changelog_default_entries_limit=%v", got["changelog_default_entries_limit"])
	}
	if got["changelog_max_entries_limit"] != float64(50) {
		t.Fatalf("changelog_max_entries_limit=%v", got["changelog_max_entries_limit"])
	}

	w = doJSON(r, http.MethodPatch, "/api/v1/admin/projects/"+ref, adminTok, map[string]any{
		"file_list_max_files": 32,
	})
	if w.Code != http.StatusBadRequest {
		t.Fatalf("PATCH over ceiling=%d %s", w.Code, w.Body.String())
	}
	if decodeErr(t, w) != "INVALID_REQUEST" {
		t.Fatalf("file_list over ceiling code=%s", w.Body.String())
	}

	w = doJSON(r, http.MethodPatch, "/api/v1/admin/projects/"+ref, adminTok, map[string]any{
		"changelog_max_entries": 51,
	})
	if w.Code != http.StatusBadRequest || decodeErr(t, w) != "INVALID_REQUEST" {
		t.Fatalf("changelog max over ceiling=%d %s", w.Code, w.Body.String())
	}
	w = doJSON(r, http.MethodPatch, "/api/v1/admin/projects/"+ref, adminTok, map[string]any{
		"changelog_default_entries": 6,
	})
	if w.Code != http.StatusBadRequest || decodeErr(t, w) != "INVALID_REQUEST" {
		t.Fatalf("changelog default over ceiling=%d %s", w.Code, w.Body.String())
	}
	w = doJSON(r, http.MethodPatch, "/api/v1/admin/projects/"+ref, adminTok, map[string]any{
		"changelog_max_entries":     3,
		"changelog_default_entries": 4,
	})
	if w.Code != http.StatusBadRequest || decodeErr(t, w) != "INVALID_REQUEST" {
		t.Fatalf("default > effective max=%d %s", w.Code, w.Body.String())
	}

	w = doJSON(r, http.MethodPost, "/api/v1/admin/projects", adminTok, map[string]string{"slug": "other-settings", "default_locale": "en"})
	if w.Code != http.StatusCreated {
		t.Fatalf("other create=%d %s", w.Code, w.Body.String())
	}

	w = doJSON(r, http.MethodPost, "/api/v1/admin/projects/"+ref+"/tokens", adminTok, map[string]any{
		"name":   "reader",
		"scopes": []string{model.ScopeProjectRead},
	})
	if w.Code != http.StatusCreated {
		t.Fatalf("reader token=%d %s", w.Code, w.Body.String())
	}
	var reader struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &reader); err != nil {
		t.Fatal(err)
	}

	w = doJSON(r, http.MethodPatch, "/api/v1/admin/projects/"+ref, reader.Token, map[string]any{
		"default_locale": "fr",
	})
	if w.Code != http.StatusForbidden || decodeErr(t, w) != "FORBIDDEN" {
		t.Fatalf("read token patch=%d %s", w.Code, w.Body.String())
	}

	w = doJSON(r, http.MethodPatch, "/api/v1/admin/projects/other-settings", reader.Token, map[string]any{
		"default_locale": "fr",
	})
	if w.Code != http.StatusForbidden || decodeErr(t, w) != "FORBIDDEN" {
		t.Fatalf("cross-project patch=%d %s", w.Code, w.Body.String())
	}

	w = doJSON(r, http.MethodGet, "/api/v1/admin/projects/"+ref, reader.Token, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("read token get=%d %s", w.Code, w.Body.String())
	}
}

func TestListProjectsNewestFirst(t *testing.T) {
	r, _, _, tok := setupProjectHTTP(t)
	for _, slug := range []string{"old-app", "new-app"} {
		w := doJSON(r, http.MethodPost, "/api/v1/admin/projects", tok, map[string]any{
			"slug": slug, "default_locale": "en",
		})
		if w.Code != http.StatusCreated {
			t.Fatalf("create %s=%d %s", slug, w.Code, w.Body.String())
		}
		time.Sleep(2 * time.Millisecond)
	}
	w := doJSON(r, http.MethodGet, "/api/v1/admin/projects", tok, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("list=%d %s", w.Code, w.Body.String())
	}
	var listed struct {
		Projects []struct {
			Slug string `json:"slug"`
		} `json:"projects"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &listed); err != nil {
		t.Fatal(err)
	}
	if len(listed.Projects) < 2 || listed.Projects[0].Slug != "new-app" || listed.Projects[1].Slug != "old-app" {
		t.Fatalf("want newest first, got %v", listed.Projects)
	}
}

func TestPatchProjectStorageRules(t *testing.T) {
	r, projSvc, _, tok := setupProjectHTTP(t)
	w := doJSON(r, http.MethodPost, "/api/v1/admin/projects", tok, map[string]any{
		"slug": "my-store", "default_locale": "en",
	})
	if w.Code != http.StatusCreated {
		t.Fatalf("create=%d %s", w.Code, w.Body.String())
	}
	w = doJSON(r, http.MethodPatch, "/api/v1/admin/projects/my-store", tok, map[string]any{
		"storage_visibility": "private",
	})
	if w.Code != http.StatusBadRequest || decodeErr(t, w) != "INVALID_REQUEST" {
		t.Fatalf("private=%d %s", w.Code, w.Body.String())
	}
	w = doJSON(r, http.MethodPatch, "/api/v1/admin/projects/my-store", tok, map[string]any{
		"storage_prefix": "my-store",
	})
	if w.Code != http.StatusBadRequest || decodeErr(t, w) != "INVALID_REQUEST" {
		t.Fatalf("prefix collision=%d %s", w.Code, w.Body.String())
	}
	w = doJSON(r, http.MethodPatch, "/api/v1/admin/projects/my-store", tok, map[string]any{
		"storage_bucket": "MY-STORE",
	})
	if w.Code != http.StatusBadRequest || decodeErr(t, w) != "INVALID_REQUEST" {
		t.Fatalf("bucket collision=%d %s", w.Code, w.Body.String())
	}
	w = doJSON(r, http.MethodPatch, "/api/v1/admin/projects/my-store", tok, map[string]any{
		"storage_prefix": "artifacts/my-store/out",
	})
	if w.Code != http.StatusBadRequest || decodeErr(t, w) != "INVALID_REQUEST" {
		t.Fatalf("prefix segment collision=%d %s", w.Code, w.Body.String())
	}
	w = doJSON(r, http.MethodPatch, "/api/v1/admin/projects/my-store", tok, map[string]any{
		"storage_prefix": "/My-Store/",
	})
	if w.Code != http.StatusBadRequest || decodeErr(t, w) != "INVALID_REQUEST" {
		t.Fatalf("trimmed prefix collision=%d %s", w.Code, w.Body.String())
	}
	w = doJSON(r, http.MethodPatch, "/api/v1/admin/projects/my-store", tok, map[string]any{
		"storage_prefix": "artifacts",
	})
	if w.Code != http.StatusOK {
		t.Fatalf("prefix ok=%d %s", w.Code, w.Body.String())
	}
	projSvc.SetStorageDriver("s3")
	w = doJSON(r, http.MethodGet, "/api/v1/admin/projects/my-store", tok, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("get after driver=%d %s", w.Code, w.Body.String())
	}
	var got map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got["storage_driver"] != "s3" {
		t.Fatalf("storage_driver=%v", got["storage_driver"])
	}
}

func boolPtr(v bool) *bool { return &v }
