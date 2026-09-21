package admin

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/Kirizu-Official/KiriVers/internal/model"
	"github.com/Kirizu-Official/KiriVers/internal/repository"
	"github.com/Kirizu-Official/KiriVers/internal/service"
	"github.com/Kirizu-Official/KiriVers/internal/storage"
)

func setupIdentityHTTP(t *testing.T) (*gin.Engine, *service.AdminService, *service.ProjectService, *service.GeoipService, string) {
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
	geoipSvc := service.NewGeoipService(repository.NewMemoryGeoipStore(), backend, t.TempDir())
	geoipSvc.SetNopOpener()
	r := gin.New()
	Register(r.Group("/api/v1/admin"), adminSvc, projSvc, nil, nil, nil, nil, geoipSvc)
	return r, adminSvc, projSvc, geoipSvc, login.Token
}

func TestProjectMemberVisibilityAndForbidden(t *testing.T) {
	r, adminSvc, _, _, platformTok := setupIdentityHTTP(t)

	w := doJSON(r, http.MethodPost, "/api/v1/admin/projects", platformTok, map[string]any{
		"slug": "owned-app", "default_locale": "en",
		"owner_username": "alice", "owner_password": "password123",
	})
	if w.Code != http.StatusCreated {
		t.Fatalf("create owned=%d %s", w.Code, w.Body.String())
	}
	w = doJSON(r, http.MethodPost, "/api/v1/admin/projects", platformTok, map[string]any{
		"slug": "other-app", "default_locale": "en",
	})
	if w.Code != http.StatusCreated {
		t.Fatalf("create other=%d %s", w.Code, w.Body.String())
	}

	aliceLogin, err := adminSvc.CompleteLogin(t.Context(), "alice", "password123", "")
	if err != nil {
		t.Fatal(err)
	}
	aliceTok := aliceLogin.Token
	if aliceLogin.Admin.IsPlatformAdmin {
		t.Fatal("alice should not be platform admin")
	}

	w = doJSON(r, http.MethodGet, "/api/v1/admin/projects", aliceTok, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("alice list=%d %s", w.Code, w.Body.String())
	}
	var listed struct {
		Projects []struct {
			Slug string `json:"slug"`
		} `json:"projects"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &listed); err != nil {
		t.Fatal(err)
	}
	if len(listed.Projects) != 1 || listed.Projects[0].Slug != "owned-app" {
		t.Fatalf("alice projects=%s", w.Body.String())
	}

	w = doJSON(r, http.MethodGet, "/api/v1/admin/admins", aliceTok, nil)
	if w.Code != http.StatusForbidden || decodeErr(t, w) != "FORBIDDEN" {
		t.Fatalf("alice admins=%d %s", w.Code, w.Body.String())
	}
	w = doJSON(r, http.MethodGet, "/api/v1/admin/geoip/databases", aliceTok, nil)
	if w.Code != http.StatusForbidden || decodeErr(t, w) != "FORBIDDEN" {
		t.Fatalf("alice geoip=%d %s", w.Code, w.Body.String())
	}
	w = doJSON(r, http.MethodGet, "/api/v1/admin/nodes", aliceTok, nil)
	if w.Code != http.StatusForbidden || decodeErr(t, w) != "FORBIDDEN" {
		t.Fatalf("alice nodes=%d %s", w.Code, w.Body.String())
	}
	w = doJSON(r, http.MethodGet, "/api/v1/admin/projects/owned-app/node-sync", aliceTok, nil)
	if w.Code != http.StatusForbidden || decodeErr(t, w) != "FORBIDDEN" {
		t.Fatalf("alice node-sync=%d %s", w.Code, w.Body.String())
	}
	w = doJSON(r, http.MethodPost, "/api/v1/admin/projects", aliceTok, map[string]any{
		"slug": "from-owner", "default_locale": "en",
	})
	if w.Code != http.StatusForbidden || decodeErr(t, w) != "FORBIDDEN" {
		t.Fatalf("alice create project=%d %s", w.Code, w.Body.String())
	}
	w = doJSON(r, http.MethodGet, "/api/v1/admin/projects/other-app", aliceTok, nil)
	if w.Code != http.StatusNotFound || decodeErr(t, w) != "PROJECT_NOT_FOUND" {
		t.Fatalf("alice foreign get=%d %s", w.Code, w.Body.String())
	}

	w = doJSON(r, http.MethodPost, "/api/v1/admin/projects/owned-app/members", aliceTok, map[string]any{
		"username": "bob", "password": "password123", "role": "owner",
	})
	if w.Code != http.StatusForbidden || decodeErr(t, w) != "FORBIDDEN" {
		t.Fatalf("alice add owner=%d %s", w.Code, w.Body.String())
	}

	w = doJSON(r, http.MethodGet, "/api/v1/admin/projects/owned-app/members", aliceTok, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("list members=%d %s", w.Code, w.Body.String())
	}
	var members struct {
		Members []struct {
			AdminID  string `json:"admin_id"`
			Username string `json:"username"`
			Role     string `json:"role"`
		} `json:"members"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &members); err != nil {
		t.Fatal(err)
	}
	if len(members.Members) != 1 || members.Members[0].Username != "alice" || members.Members[0].Role != model.ProjectMemberRoleOwner {
		t.Fatalf("members=%s", w.Body.String())
	}

	w = doJSON(r, http.MethodDelete, "/api/v1/admin/projects/owned-app/members/"+members.Members[0].AdminID, platformTok, nil)
	if w.Code != http.StatusBadRequest || decodeErr(t, w) != "LAST_OWNER" {
		t.Fatalf("delete last owner=%d %s", w.Code, w.Body.String())
	}

	w = doJSON(r, http.MethodGet, "/api/v1/admin/projects", platformTok, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("platform list=%d %s", w.Code, w.Body.String())
	}
	if err := json.Unmarshal(w.Body.Bytes(), &listed); err != nil {
		t.Fatal(err)
	}
	if len(listed.Projects) != 2 {
		t.Fatalf("platform should see 2 projects: %s", w.Body.String())
	}
}

func TestGeoipDatabaseHTTP(t *testing.T) {
	r, _, _, _, token := setupIdentityHTTP(t)
	w := postMMDB(r, token, "city.mmdb", []byte("not-a-real-mmdb"))
	if w.Code != http.StatusCreated {
		t.Fatalf("upload=%d %s", w.Code, w.Body.String())
	}
	var created map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	id, _ := created["id"].(string)
	if id == "" || created["name"] == "" {
		t.Fatalf("upload body=%s", w.Body.String())
	}
	w = doJSON(r, http.MethodGet, "/api/v1/admin/geoip/databases", token, nil)
	if w.Code != http.StatusOK || !bytes.Contains(w.Body.Bytes(), []byte(id)) {
		t.Fatalf("list=%d %s", w.Code, w.Body.String())
	}
	w = doJSON(r, http.MethodPatch, "/api/v1/admin/geoip/databases/"+id, token, map[string]any{
		"enabled": false, "rank": 3, "name": "City",
	})
	if w.Code != http.StatusOK {
		t.Fatalf("patch=%d %s", w.Code, w.Body.String())
	}
	w = doJSON(r, http.MethodDelete, "/api/v1/admin/geoip/databases/"+id, token, nil)
	if w.Code != http.StatusNoContent {
		t.Fatalf("delete=%d %s", w.Code, w.Body.String())
	}
}

func TestDeleteClientHTTP(t *testing.T) {
	r, _, projSvc, _, token := setupIdentityHTTP(t)
	w := doJSON(r, http.MethodPost, "/api/v1/admin/projects", token, map[string]any{
		"slug": "clients-app", "default_locale": "en",
	})
	if w.Code != http.StatusCreated {
		t.Fatalf("create=%d %s", w.Code, w.Body.String())
	}
	p, err := projSvc.Resolve(t.Context(), "clients-app")
	if err != nil {
		t.Fatal(err)
	}
	cl, err := projSvc.LoginClient(t.Context(), p, service.ClientLoginInput{
		DeviceID: "dev-1", Version: "1.0.0", OS: "windows", Arch: "x86_64", Channel: "stable",
		IP: "8.8.8.8",
	})
	if err != nil {
		t.Fatal(err)
	}
	w = doJSON(r, http.MethodGet, "/api/v1/admin/projects/clients-app/clients/"+cl.ID.String(), token, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("get client=%d %s", w.Code, w.Body.String())
	}
	if !bytes.Contains(w.Body.Bytes(), []byte(`"country_code"`)) || !bytes.Contains(w.Body.Bytes(), []byte(`"geo_i18n"`)) {
		t.Fatalf("client missing geo fields: %s", w.Body.String())
	}
	w = doJSON(r, http.MethodDelete, "/api/v1/admin/projects/clients-app/clients/"+cl.ID.String(), token, nil)
	if w.Code != http.StatusNoContent {
		t.Fatalf("delete client=%d %s", w.Code, w.Body.String())
	}
	w = doJSON(r, http.MethodGet, "/api/v1/admin/projects/clients-app/clients/"+cl.ID.String(), token, nil)
	if w.Code != http.StatusNotFound {
		t.Fatalf("get deleted=%d %s", w.Code, w.Body.String())
	}
}

func postMMDB(r http.Handler, token, filename string, body []byte) *httptest.ResponseRecorder {
	buf := new(bytes.Buffer)
	mw := multipart.NewWriter(buf)
	_ = mw.WriteField("name", "Test DB")
	part, err := mw.CreateFormFile("file", filename)
	if err != nil {
		panic(err)
	}
	_, _ = part.Write(body)
	_ = mw.Close()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/geoip/databases", buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}
