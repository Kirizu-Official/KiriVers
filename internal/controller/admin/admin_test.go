package admin

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/Kirizu-Official/KiriVers/internal/cache"
	"github.com/Kirizu-Official/KiriVers/internal/middleware"
	"github.com/Kirizu-Official/KiriVers/internal/model"
	"github.com/Kirizu-Official/KiriVers/internal/repository"
	"github.com/Kirizu-Official/KiriVers/internal/service"
	"github.com/Kirizu-Official/KiriVers/pkg/response"
	"github.com/pquerna/otp/totp"
)

func newAdminTestEngine(t *testing.T, svc *service.AdminService, proxies []string) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	if err := middleware.ApplyTrustedProxies(r, proxies); err != nil {
		t.Fatal(err)
	}
	Register(r.Group("/api/v1/admin"), svc, nil, nil, nil, nil, nil, nil)
	return r
}

func TestAdminHTTPLoginAndCRUD(t *testing.T) {
	svc := service.NewTestAdminService(repository.NewMemoryAdminStore())
	r := newAdminTestEngine(t, svc, nil)

	mustJSON := func(method, path, token string, body any) *httptest.ResponseRecorder {
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

	if w := mustJSON(http.MethodGet, "/api/v1/admin/admins", "", nil); w.Code != http.StatusUnauthorized {
		t.Fatalf("unauth list=%d", w.Code)
	} else {
		var env response.Body
		if err := json.Unmarshal(w.Body.Bytes(), &env); err != nil {
			t.Fatal(err)
		}
		if env.Error.Code != "UNAUTHORIZED" {
			t.Fatalf("unauth code=%q body=%s", env.Error.Code, w.Body.String())
		}
	}

	w := mustJSON(http.MethodPost, "/api/v1/admin/auth/login", "", map[string]string{
		"username": "missing", "password": "password123",
	})
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("bad login=%d %s", w.Code, w.Body.String())
	}

	if _, err := svc.Create(t.Context(), "root", "password123"); err != nil {
		t.Fatal(err)
	}
	w = mustJSON(http.MethodPost, "/api/v1/admin/auth/login", "", map[string]string{
		"username": "root", "password": "password123",
	})
	if w.Code != http.StatusOK {
		t.Fatalf("login=%d %s", w.Code, w.Body.String())
	}
	pending := decodePendingLogin(t, w.Body.Bytes())
	if pending.Status != "pending" || pending.PendingToken == "" || pending.Stage != service.StageEnrollTOTP {
		t.Fatalf("pending login=%+v body=%s", pending, w.Body.String())
	}
	if w := mustJSON(http.MethodGet, "/api/v1/admin/admins", pending.PendingToken, nil); w.Code != http.StatusUnauthorized {
		t.Fatalf("pending token must not list admins: %d %s", w.Code, w.Body.String())
	}

	full, err := svc.CompleteLogin(t.Context(), "root", "password123", "")
	if err != nil {
		t.Fatal(err)
	}
	token := full.Token
	if token == "" {
		t.Fatal("expected full session token")
	}

	w = mustJSON(http.MethodPost, "/api/v1/admin/admins", token, map[string]string{
		"username": "ops", "password": "password123",
	})
	if w.Code != http.StatusCreated {
		t.Fatalf("create=%d %s", w.Code, w.Body.String())
	}

	w = mustJSON(http.MethodGet, "/api/v1/admin/admins", token, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("list=%d %s", w.Code, w.Body.String())
	}
	if bytes.Contains(bytes.ToLower(w.Body.Bytes()), []byte("password")) {
		t.Fatalf("list leaked password field: %s", w.Body.String())
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/admins", nil)
	req.Header.Set("Authorization", "bearer "+token)
	lw := httptest.NewRecorder()
	r.ServeHTTP(lw, req)
	if lw.Code != http.StatusOK {
		t.Fatalf("lowercase bearer=%d %s", lw.Code, lw.Body.String())
	}
}

type publicAdminJSON struct {
	ID          string  `json:"id"`
	Username    string  `json:"username"`
	LastLoginAt     *string `json:"last_login_at"`
	LastLoginIP     *string `json:"last_login_ip"`
	IsPlatformAdmin *bool   `json:"is_platform_admin"`
}

func postAdminLogin(t *testing.T, engine *gin.Engine, remote, xff, user, pass string) *httptest.ResponseRecorder {
	t.Helper()
	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(map[string]string{"username": user, "password": pass}); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/auth/login", &buf)
	req.Header.Set("Content-Type", "application/json")
	req.RemoteAddr = remote
	if xff != "" {
		req.Header.Set("X-Forwarded-For", xff)
	}
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)
	return w
}

func decodeLoginAdmin(t *testing.T, body []byte) (token string, admin publicAdminJSON) {
	t.Helper()
	var out struct {
		Status      string          `json:"status"`
		AccessToken string          `json:"access_token"`
		Admin       publicAdminJSON `json:"admin"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		t.Fatal(err)
	}
	if out.Status != "" && out.Status != "complete" {
		t.Fatalf("expected complete login, status=%q body=%s", out.Status, body)
	}
	return out.AccessToken, out.Admin
}

type pendingLoginJSON struct {
	Status       string `json:"status"`
	PendingToken string `json:"pending_token"`
	Stage        string `json:"stage"`
	Secret       string `json:"secret"`
}

func decodePendingLogin(t *testing.T, body []byte) pendingLoginJSON {
	t.Helper()
	var out pendingLoginJSON
	if err := json.Unmarshal(body, &out); err != nil {
		t.Fatal(err)
	}
	return out
}

func postJSONAuth(engine *gin.Engine, method, path, token string, body any) *httptest.ResponseRecorder {
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
	engine.ServeHTTP(w, req)
	return w
}

func completeHTTPEnrollment(t *testing.T, engine *gin.Engine, pendingToken string) (string, publicAdminJSON) {
	t.Helper()
	w := postJSONAuth(engine, http.MethodPost, "/api/v1/admin/auth/2fa/totp/setup", pendingToken, map[string]any{})
	if w.Code != http.StatusOK {
		t.Fatalf("totp setup=%d %s", w.Code, w.Body.String())
	}
	assertOmitHasPasskey(t, w.Body.Bytes())
	setup := decodePendingLogin(t, w.Body.Bytes())
	if setup.Secret == "" {
		t.Fatalf("setup missing secret: %s", w.Body.String())
	}
	code, err := totp.GenerateCode(setup.Secret, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	w = postJSONAuth(engine, http.MethodPost, "/api/v1/admin/auth/2fa/totp/confirm", pendingToken, map[string]string{"code": code})
	if w.Code != http.StatusOK {
		t.Fatalf("totp confirm=%d %s", w.Code, w.Body.String())
	}
	assertOmitHasPasskey(t, w.Body.Bytes())
	w = postJSONAuth(engine, http.MethodPost, "/api/v1/admin/auth/2fa/recovery/ack", pendingToken, map[string]bool{"confirmed": true})
	if w.Code != http.StatusOK {
		t.Fatalf("ack=%d %s", w.Code, w.Body.String())
	}
	assertOmitHasPasskey(t, w.Body.Bytes())
	w = postJSONAuth(engine, http.MethodPost, "/api/v1/admin/auth/2fa/skip-passkey", pendingToken, map[string]any{})
	if w.Code != http.StatusOK {
		t.Fatalf("skip=%d %s", w.Code, w.Body.String())
	}
	assertOmitHasPasskey(t, w.Body.Bytes())
	return decodeLoginAdmin(t, w.Body.Bytes())
}

func listAdminsJSON(t *testing.T, engine *gin.Engine, token string) []publicAdminJSON {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/admins", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("list=%d %s", w.Code, w.Body.String())
	}
	var envelope struct {
		Admins []json.RawMessage `json:"admins"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	out := make([]publicAdminJSON, 0, len(envelope.Admins))
	for _, raw := range envelope.Admins {
		assertAdminJSONHasLoginKeys(t, raw)
		var a publicAdminJSON
		if err := json.Unmarshal(raw, &a); err != nil {
			t.Fatal(err)
		}
		out = append(out, a)
	}
	return out
}

func assertAdminJSONHasLoginKeys(t *testing.T, raw []byte) {
	t.Helper()
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(raw, &obj); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"last_login_at", "last_login_ip", "is_platform_admin"} {
		if _, ok := obj[key]; !ok {
			t.Fatalf("public admin JSON missing %q: %s", key, raw)
		}
	}
}

func findAdminJSON(t *testing.T, list []publicAdminJSON, username string) publicAdminJSON {
	t.Helper()
	for _, a := range list {
		if a.Username == username {
			return a
		}
	}
	t.Fatalf("admin %q not in list: %+v", username, list)
	return publicAdminJSON{}
}

func TestAdminHTTPLastLogin(t *testing.T) {
	svc := service.NewTestAdminService(repository.NewMemoryAdminStore())
	r := newAdminTestEngine(t, svc, []string{})
	rootAdmin, err := svc.Create(t.Context(), "root", "password123")
	if err != nil {
		t.Fatal(err)
	}

	w := postAdminLogin(t, r, "192.0.2.1:1234", "203.0.113.9", "root", "password123")
	if w.Code != http.StatusOK {
		t.Fatalf("login=%d %s", w.Code, w.Body.String())
	}
	pending := decodePendingLogin(t, w.Body.Bytes())
	if pending.PendingToken == "" {
		t.Fatalf("expected pending token: %s", w.Body.String())
	}
	afterPassword, err := svc.Get(t.Context(), rootAdmin.ID)
	if err != nil {
		t.Fatal(err)
	}
	if afterPassword.LastLoginAt != nil || afterPassword.LastLoginIP != nil {
		t.Fatalf("password pending must not write last login: at=%v ip=%v", afterPassword.LastLoginAt, afterPassword.LastLoginIP)
	}

	before := time.Now().UTC().Add(-time.Second)
	token, loginAdmin := completeHTTPEnrollment(t, r, pending.PendingToken)
	after := time.Now().UTC().Add(time.Second)
	if token == "" {
		t.Fatal("expected access token after enrollment")
	}
	if loginAdmin.LastLoginIP == nil || *loginAdmin.LastLoginIP != "192.0.2.1" {
		t.Fatalf("login ip=%v (empty trust must ignore XFF)", loginAdmin.LastLoginIP)
	}
	if loginAdmin.LastLoginAt == nil {
		t.Fatal("login last_login_at is null")
	}
	parsed, err := time.Parse("2006-01-02T15:04:05Z", *loginAdmin.LastLoginAt)
	if err != nil {
		t.Fatalf("last_login_at=%q: %v", *loginAdmin.LastLoginAt, err)
	}
	if parsed.Before(before) || parsed.After(after) {
		t.Fatalf("last_login_at=%s not in window", *loginAdmin.LastLoginAt)
	}

	createBuf := bytes.NewBufferString(`{"username":"ops","password":"password123"}`)
	createReq := httptest.NewRequest(http.MethodPost, "/api/v1/admin/admins", createBuf)
	createReq.Header.Set("Content-Type", "application/json")
	createReq.Header.Set("Authorization", "Bearer "+token)
	cw := httptest.NewRecorder()
	r.ServeHTTP(cw, createReq)
	if cw.Code != http.StatusCreated {
		t.Fatalf("create ops=%d %s", cw.Code, cw.Body.String())
	}
	assertAdminJSONHasLoginKeys(t, cw.Body.Bytes())
	var created publicAdminJSON
	if err := json.Unmarshal(cw.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	if created.LastLoginAt != nil || created.LastLoginIP != nil {
		t.Fatalf("POST create never-logged-in must be JSON null: %+v", created)
	}

	listed := listAdminsJSON(t, r, token)
	root := findAdminJSON(t, listed, "root")
	if root.LastLoginIP == nil || *root.LastLoginIP != "192.0.2.1" || root.LastLoginAt == nil || *root.LastLoginAt != *loginAdmin.LastLoginAt {
		t.Fatalf("list root last login mismatch: %+v login=%+v", root, loginAdmin)
	}
	ops := findAdminJSON(t, listed, "ops")
	if ops.LastLoginAt != nil || ops.LastLoginIP != nil {
		t.Fatalf("never-logged-in admin must be JSON null: %+v", ops)
	}

	fail := postAdminLogin(t, r, "198.51.100.1:9", "", "root", "wrong-password")
	if fail.Code != http.StatusUnauthorized {
		t.Fatalf("bad login=%d %s", fail.Code, fail.Body.String())
	}
	afterFail := findAdminJSON(t, listAdminsJSON(t, r, token), "root")
	if afterFail.LastLoginIP == nil || *afterFail.LastLoginIP != "192.0.2.1" || afterFail.LastLoginAt == nil || *afterFail.LastLoginAt != *loginAdmin.LastLoginAt {
		t.Fatalf("failed login overwrote fields: %+v", afterFail)
	}

	patchBuf := bytes.NewBufferString(`{"username":"root2"}`)
	patchReq := httptest.NewRequest(http.MethodPatch, "/api/v1/admin/admins/"+root.ID, patchBuf)
	patchReq.Header.Set("Content-Type", "application/json")
	patchReq.Header.Set("Authorization", "Bearer "+token)
	pw := httptest.NewRecorder()
	r.ServeHTTP(pw, patchReq)
	if pw.Code != http.StatusOK {
		t.Fatalf("patch=%d %s", pw.Code, pw.Body.String())
	}
	assertAdminJSONHasLoginKeys(t, pw.Body.Bytes())
	var patched publicAdminJSON
	if err := json.Unmarshal(pw.Body.Bytes(), &patched); err != nil {
		t.Fatal(err)
	}
	if patched.Username != "root2" {
		t.Fatalf("username=%q", patched.Username)
	}
	if patched.LastLoginIP == nil || *patched.LastLoginIP != "192.0.2.1" || patched.LastLoginAt == nil || *patched.LastLoginAt != *loginAdmin.LastLoginAt {
		t.Fatalf("patch dropped last login: %+v", patched)
	}
}

func TestAdminHTTPLoginTrustedProxies(t *testing.T) {
	const remote = "192.0.2.1:1234"
	const xff = "203.0.113.9, 192.0.2.1"

	untrusted := service.NewTestAdminService(repository.NewMemoryAdminStore())
	if _, err := untrusted.Create(t.Context(), "root", "password123"); err != nil {
		t.Fatal(err)
	}
	emptyEngine := newAdminTestEngine(t, untrusted, []string{})
	w := postAdminLogin(t, emptyEngine, remote, xff, "root", "password123")
	if w.Code != http.StatusOK {
		t.Fatalf("empty trust login=%d %s", w.Code, w.Body.String())
	}
	pending := decodePendingLogin(t, w.Body.Bytes())
	_, admin := completeHTTPEnrollment(t, emptyEngine, pending.PendingToken)
	if admin.LastLoginIP == nil || *admin.LastLoginIP != "192.0.2.1" {
		t.Fatalf("empty trust recorded %v, want RemoteAddr host", derefStr(admin.LastLoginIP))
	}

	trusted := service.NewTestAdminService(repository.NewMemoryAdminStore())
	if _, err := trusted.Create(t.Context(), "root", "password123"); err != nil {
		t.Fatal(err)
	}
	trustedEngine := newAdminTestEngine(t, trusted, []string{"192.0.2.1"})
	w = postAdminLogin(t, trustedEngine, remote, xff, "root", "password123")
	if w.Code != http.StatusOK {
		t.Fatalf("trusted login=%d %s", w.Code, w.Body.String())
	}
	pending = decodePendingLogin(t, w.Body.Bytes())
	_, admin = completeHTTPEnrollment(t, trustedEngine, pending.PendingToken)
	if admin.LastLoginIP == nil || *admin.LastLoginIP != "203.0.113.9" {
		t.Fatalf("trusted proxy recorded %q, want leftmost XFF", derefStr(admin.LastLoginIP))
	}
}

func derefStr(p *string) string {
	if p == nil {
		return "<nil>"
	}
	return *p
}

type failLastLoginStore struct {
	*repository.MemoryAdminStore
}

func (f *failLastLoginStore) UpdateLastLogin(context.Context, uuid.UUID, time.Time, *string) error {
	return errors.New("persist failed")
}

func TestAdminHTTPLoginPersistFailure(t *testing.T) {
	store := &failLastLoginStore{MemoryAdminStore: repository.NewMemoryAdminStore()}
	svc := service.NewTestAdminService(store)
	if _, err := svc.Create(t.Context(), "root", "password123"); err != nil {
		t.Fatal(err)
	}
	r := newAdminTestEngine(t, svc, nil)
	w := postAdminLogin(t, r, "192.0.2.1:1234", "", "root", "password123")
	if w.Code != http.StatusOK {
		t.Fatalf("password login=%d %s", w.Code, w.Body.String())
	}
	pending := decodePendingLogin(t, w.Body.Bytes())
	w = postJSONAuth(r, http.MethodPost, "/api/v1/admin/auth/2fa/totp/setup", pending.PendingToken, map[string]any{})
	if w.Code != http.StatusOK {
		t.Fatalf("setup=%d %s", w.Code, w.Body.String())
	}
	setup := decodePendingLogin(t, w.Body.Bytes())
	code, err := totp.GenerateCode(setup.Secret, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	w = postJSONAuth(r, http.MethodPost, "/api/v1/admin/auth/2fa/totp/confirm", pending.PendingToken, map[string]string{"code": code})
	if w.Code != http.StatusOK {
		t.Fatalf("confirm=%d %s", w.Code, w.Body.String())
	}
	w = postJSONAuth(r, http.MethodPost, "/api/v1/admin/auth/2fa/recovery/ack", pending.PendingToken, map[string]bool{"confirmed": true})
	if w.Code != http.StatusOK {
		t.Fatalf("ack=%d %s", w.Code, w.Body.String())
	}
	w = postJSONAuth(r, http.MethodPost, "/api/v1/admin/auth/2fa/skip-passkey", pending.PendingToken, map[string]any{})
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("persist fail skip=%d %s", w.Code, w.Body.String())
	}
	var env response.Body
	if err := json.Unmarshal(w.Body.Bytes(), &env); err != nil {
		t.Fatal(err)
	}
	if env.Error.Code != "INTERNAL_ERROR" {
		t.Fatalf("code=%q body=%s", env.Error.Code, w.Body.String())
	}
	if bytes.Contains(w.Body.Bytes(), []byte("access_token")) {
		t.Fatalf("must not issue token: %s", w.Body.String())
	}
	got, err := store.GetByUsername(t.Context(), "root")
	if err != nil {
		t.Fatal(err)
	}
	if got.LastLoginAt != nil || got.LastLoginIP != nil {
		t.Fatalf("persist fail must not write last login: at=%v ip=%v", got.LastLoginAt, got.LastLoginIP)
	}
}

func TestAdminHTTPTOTPRateLimited(t *testing.T) {
	mem, err := cache.Open(cache.Options{Driver: "memory"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = mem.Close() })
	store := repository.NewMemoryAdminStore()
	svc := service.NewAdminService(service.AdminServiceOptions{
		Store:           store,
		TwoFA:           store,
		Cache:           mem,
		TOTPMaxAttempts: 2,
		PendingTTL:      time.Minute,
		SessionIdle:     time.Hour,
	})
	if _, err := svc.Create(t.Context(), "root", "password123"); err != nil {
		t.Fatal(err)
	}
	r := newAdminTestEngine(t, svc, nil)
	w := postAdminLogin(t, r, "192.0.2.1:1", "", "root", "password123")
	if w.Code != http.StatusOK {
		t.Fatalf("login=%d %s", w.Code, w.Body.String())
	}
	pending := decodePendingLogin(t, w.Body.Bytes())
	firstToken, _ := completeHTTPEnrollment(t, r, pending.PendingToken)

	w = postAdminLogin(t, r, "192.0.2.1:1", "", "root", "password123")
	second := decodePendingLogin(t, w.Body.Bytes())
	if second.Stage != service.StageSecondFactor {
		t.Fatalf("stage=%s", second.Stage)
	}
	for i := 0; i < 2; i++ {
		bad := postJSONAuth(r, http.MethodPost, "/api/v1/admin/auth/2fa/verify", second.PendingToken, map[string]string{"totp": "000000"})
		if bad.Code != http.StatusUnauthorized {
			t.Fatalf("bad totp %d: %d %s", i, bad.Code, bad.Body.String())
		}
	}
	limited := postJSONAuth(r, http.MethodPost, "/api/v1/admin/auth/2fa/verify", second.PendingToken, map[string]string{"totp": "000000"})
	if limited.Code != http.StatusTooManyRequests {
		t.Fatalf("rate limit=%d %s", limited.Code, limited.Body.String())
	}
	var env response.Body
	if err := json.Unmarshal(limited.Body.Bytes(), &env); err != nil {
		t.Fatal(err)
	}
	if env.Error.Code != "TOTP_RATE_LIMITED" {
		t.Fatalf("code=%q body=%s", env.Error.Code, limited.Body.String())
	}
	alive := postJSONAuth(r, http.MethodGet, "/api/v1/admin/admins", firstToken, nil)
	if alive.Code != http.StatusOK {
		t.Fatalf("existing session must survive rate limit: %d %s", alive.Code, alive.Body.String())
	}
}

func jsonFieldPresent(t *testing.T, body []byte, key string) (json.RawMessage, bool) {
	t.Helper()
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(body, &obj); err != nil {
		t.Fatal(err)
	}
	v, ok := obj[key]
	return v, ok
}

func assertOmitHasPasskey(t *testing.T, body []byte) {
	t.Helper()
	if _, ok := jsonFieldPresent(t, body, "has_passkey"); ok {
		t.Fatalf("response must omit has_passkey: %s", body)
	}
}

func TestAdminHTTPLoginHasPasskey(t *testing.T) {
	store := repository.NewMemoryAdminStore()
	svc := service.NewTestAdminService(store)
	r := newAdminTestEngine(t, svc, nil)

	unauthorized := postAdminLogin(t, r, "192.0.2.1:1", "", "root", "wrong-password")
	if unauthorized.Code != http.StatusUnauthorized {
		t.Fatalf("bad login=%d %s", unauthorized.Code, unauthorized.Body.String())
	}
	if bytes.Contains(unauthorized.Body.Bytes(), []byte("has_passkey")) {
		t.Fatalf("password 401 must omit has_passkey: %s", unauthorized.Body.String())
	}

	admin, err := svc.Create(t.Context(), "root", "password123")
	if err != nil {
		t.Fatal(err)
	}
	enroll := postAdminLogin(t, r, "192.0.2.1:1", "", "root", "password123")
	if enroll.Code != http.StatusOK {
		t.Fatalf("enroll login=%d %s", enroll.Code, enroll.Body.String())
	}
	pending := decodePendingLogin(t, enroll.Body.Bytes())
	if pending.Stage != service.StageEnrollTOTP {
		t.Fatalf("stage=%s", pending.Stage)
	}
	assertOmitHasPasskey(t, enroll.Body.Bytes())

	sessionToken, _ := completeHTTPEnrollment(t, r, pending.PendingToken)
	if sessionToken == "" {
		t.Fatal("expected session after enrollment")
	}

	second := postAdminLogin(t, r, "192.0.2.1:1", "", "root", "password123")
	if second.Code != http.StatusOK {
		t.Fatalf("second_factor login=%d %s", second.Code, second.Body.String())
	}
	decoded := decodePendingLogin(t, second.Body.Bytes())
	if decoded.Stage != service.StageSecondFactor {
		t.Fatalf("stage=%s", decoded.Stage)
	}
	raw, ok := jsonFieldPresent(t, second.Body.Bytes(), "has_passkey")
	if !ok {
		t.Fatalf("second_factor must include has_passkey: %s", second.Body.String())
	}
	var hasPasskey bool
	if err := json.Unmarshal(raw, &hasPasskey); err != nil {
		t.Fatalf("has_passkey must be boolean: %s", raw)
	}
	if hasPasskey {
		t.Fatalf("zero passkeys must be has_passkey=false, got %s", raw)
	}

	wrong := postAdminLogin(t, r, "192.0.2.1:1", "", "root", "wrong-password")
	if wrong.Code != http.StatusUnauthorized {
		t.Fatalf("wrong password=%d %s", wrong.Code, wrong.Body.String())
	}
	if bytes.Contains(wrong.Body.Bytes(), []byte("has_passkey")) {
		t.Fatalf("password 401 must omit has_passkey: %s", wrong.Body.String())
	}

	if err := store.CreatePasskey(t.Context(), &model.AdminPasskey{
		AdminID:      admin.ID,
		CredentialID: []byte("cred-http-1"),
		PublicKey:    []byte("pubkey-http-1"),
		Name:         "laptop",
	}); err != nil {
		t.Fatal(err)
	}
	withKey := postAdminLogin(t, r, "192.0.2.1:1", "", "root", "password123")
	if withKey.Code != http.StatusOK {
		t.Fatalf("passkey login=%d %s", withKey.Code, withKey.Body.String())
	}
	raw, ok = jsonFieldPresent(t, withKey.Body.Bytes(), "has_passkey")
	if !ok {
		t.Fatalf("second_factor with passkey must include has_passkey: %s", withKey.Body.String())
	}
	if err := json.Unmarshal(raw, &hasPasskey); err != nil {
		t.Fatalf("has_passkey must be boolean: %s", raw)
	}
	if !hasPasskey {
		t.Fatalf("registered passkey must be has_passkey=true, got %s", raw)
	}

	row, err := store.GetTOTP(t.Context(), admin.ID)
	if err != nil {
		t.Fatal(err)
	}
	code, err := totp.GenerateCode(row.Secret, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	complete := decodePendingLogin(t, withKey.Body.Bytes())
	verified := postJSONAuth(r, http.MethodPost, "/api/v1/admin/auth/2fa/verify", complete.PendingToken, map[string]string{"totp": code})
	if verified.Code != http.StatusOK {
		t.Fatalf("verify=%d %s", verified.Code, verified.Body.String())
	}
	assertOmitHasPasskey(t, verified.Body.Bytes())
}
