package admin

import (
	"bytes"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/rs/zerolog"

	"github.com/Kirizu-Official/KiriVers/internal/controller/client"
	"github.com/Kirizu-Official/KiriVers/internal/repository"
	"github.com/Kirizu-Official/KiriVers/internal/service"
	"github.com/Kirizu-Official/KiriVers/internal/service/update"
	"github.com/Kirizu-Official/KiriVers/internal/storage"
)

func setupMediaHTTP(t *testing.T) (*gin.Engine, *service.ProjectService, *service.MediaService, string, storage.Backend) {
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
	replica, err := storage.NewLocalFS(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	projSvc := service.NewProjectService(store, backend)
	projSvc.SetInstallPolicyStore(repository.NewMemoryInstallPolicyRuleStore())
	announceSvc := service.NewAnnouncementService(repository.NewMemoryAnnouncementStore(), store)
	mediaSvc := service.NewMediaService(repository.NewMemoryProjectMediaStore(), backend, replica)
	auditSvc := service.NewAuditService(repository.NewMemoryAuditStore(), zerolog.Nop())
	r := gin.New()
	adminGroup := r.Group("/api/v1/admin")
	Register(adminGroup, adminSvc, projSvc, nil, nil, auditSvc, announceSvc, nil)
	RegisterMedia(adminGroup, adminSvc, projSvc, mediaSvc)
	api := r.Group("/api/v1")
	client.Register(api, projSvc, update.NewService(repository.NewMemoryUpdateCatalog(store)), nil, nil, nil, announceSvc)
	client.RegisterMedia(api, projSvc, mediaSvc, nil)
	return r, projSvc, mediaSvc, login.Token, replica
}

func postMediaFile(r http.Handler, path, token, field, filename string, body []byte, contentType string) *httptest.ResponseRecorder {
	buf := new(bytes.Buffer)
	w := multipart.NewWriter(buf)
	part, err := w.CreateFormFile(field, filename)
	if err != nil {
		panic(err)
	}
	_, _ = part.Write(body)
	_ = w.Close()
	req := httptest.NewRequest(http.MethodPost, path, buf)
	req.Header.Set("Content-Type", w.FormDataContentType())
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	if contentType != "" {
		// multipart part Content-Type is set by CreateFormFile as octet-stream;
		// handler still sniffs bytes. Extra header unused.
		_ = contentType
	}
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	return rec
}

func TestMediaUploadAndPublicGet(t *testing.T) {
	r, projSvc, _, token, replica := setupMediaHTTP(t)
	ctx := t.Context()
	slug := "media-proj"
	requireToken := true
	p, _, err := projSvc.Create(ctx, service.CreateProjectInput{DefaultLocale: ptr("en"), Slug: &slug,
		RequireClientToken: &requireToken,
	})
	if err != nil {
		t.Fatal(err)
	}
	png := []byte("\x89PNG\r\n\x1a\n0123456789")
	w := postMediaFile(r, "/api/v1/admin/projects/"+slug+"/media", token, "file[]", "shot.png", png, "image/png")
	if w.Code != http.StatusOK {
		t.Fatalf("upload=%d %s", w.Code, w.Body.String())
	}
	var env struct {
		Code int `json:"code"`
		Data struct {
			SuccMap  map[string]string `json:"succMap"`
			ErrFiles []string          `json:"errFiles"`
			Files    []struct {
				ID       string `json:"id"`
				SHA256   string `json:"sha256"`
				MD5      string `json:"md5"`
				SHA512   string `json:"sha512"`
				FileName string `json:"file_name"`
				URL      string `json:"url"`
			} `json:"files"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &env); err != nil {
		t.Fatal(err)
	}
	if env.Code != 0 || env.Data.SuccMap["shot.png"] == "" {
		t.Fatalf("vditor envelope=%s", w.Body.String())
	}
	if len(env.Data.Files) != 1 || env.Data.Files[0].SHA256 == "" || env.Data.Files[0].MD5 == "" || env.Data.Files[0].SHA512 == "" {
		t.Fatalf("ac8 files hashes=%s", w.Body.String())
	}
	url := env.Data.SuccMap["shot.png"]
	if !strings.HasPrefix(url, service.SiteURLPlaceholder+"/api/v1/projects/"+slug+"/media/") {
		t.Fatalf("succMap url=%q", url)
	}
	id := strings.TrimPrefix(url, service.SiteURLPlaceholder+"/api/v1/projects/"+slug+"/media/")
	if _, err := uuid.Parse(id); err != nil {
		t.Fatalf("id=%q", id)
	}

	getPath := fmt.Sprintf("/api/v1/projects/%s/media/%s", slug, id)
	req := httptest.NewRequest(http.MethodGet, getPath, nil)
	gw := httptest.NewRecorder()
	r.ServeHTTP(gw, req)
	if gw.Code != http.StatusOK {
		t.Fatalf("get without auth=%d %s", gw.Code, gw.Body.String())
	}
	if !bytes.Equal(gw.Body.Bytes(), png) {
		t.Fatalf("payload mismatch")
	}
	if !strings.Contains(gw.Header().Get("Content-Type"), "image/png") {
		t.Fatalf("content-type=%q", gw.Header().Get("Content-Type"))
	}
	if !strings.Contains(gw.Header().Get("Content-Disposition"), "inline") {
		t.Fatalf("disposition=%q", gw.Header().Get("Content-Disposition"))
	}
	if gw.Header().Get("ETag") != strconv.Quote(env.Data.Files[0].SHA256) {
		t.Fatalf("etag=%q sha256=%q", gw.Header().Get("ETag"), env.Data.Files[0].SHA256)
	}
	if gw.Header().Get("Digest") == "" || gw.Header().Get("Content-MD5") == "" {
		t.Fatalf("digest headers missing etag=%q digest=%q md5=%q", gw.Header().Get("ETag"), gw.Header().Get("Digest"), gw.Header().Get("Content-MD5"))
	}

	req = httptest.NewRequest(http.MethodHead, getPath, nil)
	hw := httptest.NewRecorder()
	r.ServeHTTP(hw, req)
	if hw.Code != http.StatusOK || hw.Body.Len() != 0 {
		t.Fatalf("head=%d len=%d", hw.Code, hw.Body.Len())
	}
	if hw.Header().Get("ETag") != strconv.Quote(env.Data.Files[0].SHA256) {
		t.Fatalf("head etag=%q sha256=%q", hw.Header().Get("ETag"), env.Data.Files[0].SHA256)
	}
	if hw.Header().Get("Digest") == "" || hw.Header().Get("Content-MD5") == "" {
		t.Fatalf("head digest headers missing etag=%q digest=%q md5=%q", hw.Header().Get("ETag"), hw.Header().Get("Digest"), hw.Header().Get("Content-MD5"))
	}

	req = httptest.NewRequest(http.MethodGet, getPath, nil)
	req.Header.Set("Range", "bytes=0-3")
	rw := httptest.NewRecorder()
	r.ServeHTTP(rw, req)
	if rw.Code != http.StatusPartialContent || rw.Body.String() != string(png[:4]) {
		t.Fatalf("range=%d %q", rw.Code, rw.Body.String())
	}

	other := "other-proj"
	if _, _, err := projSvc.Create(ctx, service.CreateProjectInput{DefaultLocale: ptr("en"), Slug: &other}); err != nil {
		t.Fatal(err)
	}
	req = httptest.NewRequest(http.MethodGet, "/api/v1/projects/"+other+"/media/"+id, nil)
	ow := httptest.NewRecorder()
	r.ServeHTTP(ow, req)
	if ow.Code != http.StatusNotFound {
		t.Fatalf("wrong project=%d %s", ow.Code, ow.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/api/v1/projects/"+slug+"/media/"+uuid.Must(uuid.NewRandom()).String(), nil)
	nw := httptest.NewRecorder()
	r.ServeHTTP(nw, req)
	if nw.Code != http.StatusNotFound {
		t.Fatalf("unknown id=%d", nw.Code)
	}

	repKey := "media-cache/" + p.ID.String() + "/" + id + "/shot.png"
	if _, _, err := replica.Head(ctx, repKey); err != nil {
		t.Fatalf("s3 replica missing after put: %v", err)
	}
}

func TestMediaAttachmentUsesStoredFileName(t *testing.T) {
	r, projSvc, _, token, _ := setupMediaHTTP(t)
	slug := "media-notes"
	if _, _, err := projSvc.Create(t.Context(), service.CreateProjectInput{DefaultLocale: ptr("en"), Slug: &slug}); err != nil {
		t.Fatal(err)
	}
	body := []byte("release notes\n")
	w := postMediaFile(r, "/api/v1/admin/projects/"+slug+"/media", token, "file[]", "notes.txt", body, "text/plain")
	if w.Code != http.StatusOK {
		t.Fatalf("upload=%d %s", w.Code, w.Body.String())
	}
	var env struct {
		Code int `json:"code"`
		Data struct {
			SuccMap map[string]string `json:"succMap"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &env); err != nil {
		t.Fatal(err)
	}
	if env.Code != 0 || env.Data.SuccMap["notes.txt"] == "" {
		t.Fatalf("vditor envelope=%s", w.Body.String())
	}
	url := env.Data.SuccMap["notes.txt"]
	id := strings.TrimPrefix(url, service.SiteURLPlaceholder+"/api/v1/projects/"+slug+"/media/")
	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/v1/projects/%s/media/%s", slug, id), nil)
	gw := httptest.NewRecorder()
	r.ServeHTTP(gw, req)
	if gw.Code != http.StatusOK {
		t.Fatalf("get=%d %s", gw.Code, gw.Body.String())
	}
	if !bytes.Equal(gw.Body.Bytes(), body) {
		t.Fatalf("payload mismatch")
	}
	disp := gw.Header().Get("Content-Disposition")
	if !strings.Contains(disp, "attachment") || !strings.Contains(disp, "notes.txt") {
		t.Fatalf("disposition=%q", disp)
	}

	w = postMediaFile(r, "/api/v1/admin/projects/"+slug+"/media", token, "file[]", "说明.txt", body, "text/plain")
	if w.Code != http.StatusOK {
		t.Fatalf("unicode upload=%d %s", w.Code, w.Body.String())
	}
	if err := json.Unmarshal(w.Body.Bytes(), &env); err != nil {
		t.Fatal(err)
	}
	uniURL := env.Data.SuccMap["说明.txt"]
	if uniURL == "" {
		t.Fatalf("unicode succMap=%s", w.Body.String())
	}
	uniID := strings.TrimPrefix(uniURL, service.SiteURLPlaceholder+"/api/v1/projects/"+slug+"/media/")
	req = httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/v1/projects/%s/media/%s", slug, uniID), nil)
	uw := httptest.NewRecorder()
	r.ServeHTTP(uw, req)
	if uw.Code != http.StatusOK {
		t.Fatalf("unicode get=%d %s", uw.Code, uw.Body.String())
	}
	uniDisp := uw.Header().Get("Content-Disposition")
	if !strings.Contains(uniDisp, "filename*=") || !strings.Contains(uniDisp, "attachment") {
		t.Fatalf("unicode disposition=%q", uniDisp)
	}
	if !strings.Contains(uniDisp, "filename*=UTF-8''%E8%AF%B4%E6%98%8E.txt") {
		t.Fatalf("unicode filename*= missing stored name: %q", uniDisp)
	}
}

func TestMediaHTMLRejected(t *testing.T) {
	r, projSvc, _, token, _ := setupMediaHTTP(t)
	slug := "html-proj"
	if _, _, err := projSvc.Create(t.Context(), service.CreateProjectInput{DefaultLocale: ptr("en"), Slug: &slug}); err != nil {
		t.Fatal(err)
	}
	w := postMediaFile(r, "/api/v1/admin/projects/"+slug+"/media", token, "file[]", "x.html", []byte("<html>hi</html>"), "text/html")
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), `"code":1`) && !strings.Contains(w.Body.String(), `"code": 1`) {
		t.Fatalf("expected vditor failure envelope %s", w.Body.String())
	}
}

func TestClientMarkdownExpandSiteURL(t *testing.T) {
	r, projSvc, _, token, _ := setupMediaHTTP(t)
	ctx := t.Context()
	slug := "expand-app"
	p, _, err := projSvc.Create(ctx, service.CreateProjectInput{DefaultLocale: ptr("en"), Slug: &slug})
	if err != nil {
		t.Fatal(err)
	}
	md := "logo ${site_url}/api/v1/projects/" + slug + "/media/" + uuid.Must(uuid.NewRandom()).String()
	base := "/api/v1/admin/projects/" + slug + "/announcements"
	w := doJSON(r, http.MethodPost, base, token, gin.H{
		"language": "en", "title": "Hi", "content": md,
	})
	if w.Code != http.StatusCreated {
		t.Fatalf("create=%d %s", w.Code, w.Body.String())
	}
	var created map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	id := created["id"].(string)
	w = doJSON(r, http.MethodPatch, base+"/"+id, token, gin.H{"status": "published"})
	if w.Code != http.StatusOK {
		t.Fatalf("publish=%d %s", w.Code, w.Body.String())
	}

	clientURL := "/api/v1/projects/" + slug + "/announcements?locale=en"
	req := httptest.NewRequest(http.MethodGet, clientURL, nil)
	req.Header.Set("Referer", "https://app.example/path")
	cw := httptest.NewRecorder()
	r.ServeHTTP(cw, req)
	if cw.Code != http.StatusOK || !strings.Contains(cw.Body.String(), "https://app.example/api/v1/projects/") {
		t.Fatalf("referer expand=%d %s", cw.Code, cw.Body.String())
	}
	if strings.Contains(cw.Body.String(), service.SiteURLPlaceholder) {
		t.Fatalf("placeholder leaked: %s", cw.Body.String())
	}
	if !strings.Contains(cw.Header().Get("Vary"), "Referer") {
		t.Fatalf("vary=%q", cw.Header().Get("Vary"))
	}
	etag := cw.Header().Get("ETag")

	req2 := httptest.NewRequest(http.MethodGet, clientURL, nil)
	req2.Header.Set("Referer", "https://other.example/z")
	cw2 := httptest.NewRecorder()
	r.ServeHTTP(cw2, req2)
	if cw2.Header().Get("ETag") != etag {
		t.Fatalf("etag changed with referer: %q vs %q", etag, cw2.Header().Get("ETag"))
	}

	req3 := httptest.NewRequest(http.MethodGet, "http://api.test"+clientURL, nil)
	cw3 := httptest.NewRecorder()
	r.ServeHTTP(cw3, req3)
	if !strings.Contains(cw3.Body.String(), "http://api.test/api/v1/projects/") {
		t.Fatalf("missing referer fallback=%s", cw3.Body.String())
	}

	adminGet := doJSON(r, http.MethodGet, base+"/"+id, token, nil)
	if !strings.Contains(adminGet.Body.String(), service.SiteURLPlaceholder) {
		t.Fatalf("admin get lost placeholder: %s", adminGet.Body.String())
	}
	_ = p
}
