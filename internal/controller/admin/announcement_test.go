package admin

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/rs/zerolog"

	"github.com/Kirizu-Official/KiriVers/internal/controller/client"
	"github.com/Kirizu-Official/KiriVers/internal/model"
	"github.com/Kirizu-Official/KiriVers/internal/repository"
	"github.com/Kirizu-Official/KiriVers/internal/service"
	"github.com/Kirizu-Official/KiriVers/internal/service/update"
	"github.com/Kirizu-Official/KiriVers/internal/storage"
)

func setupAnnouncementHTTP(t *testing.T) (*gin.Engine, *service.ProjectService, *repository.MemoryAuditStore, string) {
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
	announceSvc := service.NewAnnouncementService(repository.NewMemoryAnnouncementStore(), store)
	auditStore := repository.NewMemoryAuditStore()
	auditSvc := service.NewAuditService(auditStore, zerolog.Nop())
	r := gin.New()
	Register(r.Group("/api/v1/admin"), adminSvc, projSvc, nil, nil, auditSvc, announceSvc, nil)
	client.Register(r.Group("/api/v1"), projSvc, update.NewService(repository.NewMemoryUpdateCatalog(store)), nil, nil, nil, announceSvc)
	return r, projSvc, auditStore, login.Token
}

func TestAnnouncementHTTPAcceptance(t *testing.T) {
	r, projSvc, auditStore, adminTok := setupAnnouncementHTTP(t)
	ctx := t.Context()
	slug := "ann-app"
	p, _, err := projSvc.Create(ctx, service.CreateProjectInput{DefaultLocale: ptr("en"), Slug: &slug})
	if err != nil {
		t.Fatal(err)
	}
	base := "/api/v1/admin/projects/" + slug + "/announcements"
	clientBase := "/api/v1/projects/" + slug + "/announcements"
	copy := gin.H{"language": "en", "title": "Hello"}

	// AC1: create is draft, omitted from client GET.
	w := doJSON(r, http.MethodPost, base, adminTok, copy)
	if w.Code != http.StatusCreated {
		t.Fatalf("create=%d %s", w.Code, w.Body.String())
	}
	var created map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	if created["status"] != "draft" {
		t.Fatalf("status=%v", created["status"])
	}
	if created["version_id"] != nil {
		t.Fatalf("project-wide version_id=%v", created["version_id"])
	}
	if _, ok := created["version"]; ok {
		t.Fatalf("must not emit version string: %s", w.Body.String())
	}
	id := created["id"].(string)
	cw := doJSON(r, http.MethodGet, clientBase, "", nil)
	if cw.Code != http.StatusOK {
		t.Fatalf("client draft list=%d %s", cw.Code, cw.Body.String())
	}
	assertClientEmpty(t, cw)

	// AC2: publish with null window → client returns it; unpublish hides it.
	w = doJSON(r, http.MethodPatch, base+"/"+id, adminTok, gin.H{"status": "published"})
	if w.Code != http.StatusOK {
		t.Fatalf("publish=%d %s", w.Code, w.Body.String())
	}
	cw = doJSON(r, http.MethodGet, clientBase+"?locale=en", "", nil)
	if cw.Code != http.StatusOK || !strings.Contains(cw.Body.String(), `"Hello"`) {
		t.Fatalf("published client=%d %s", cw.Code, cw.Body.String())
	}

	w = doJSON(r, http.MethodPatch, base+"/"+id, adminTok, gin.H{"status": "draft"})
	if w.Code != http.StatusOK {
		t.Fatalf("unpublish=%d %s", w.Code, w.Body.String())
	}
	cw = doJSON(r, http.MethodGet, clientBase+"?locale=en", "", nil)
	assertClientEmpty(t, cw)

	w = doJSON(r, http.MethodPatch, base+"/"+id, adminTok, gin.H{"status": "published"})
	if w.Code != http.StatusOK {
		t.Fatal(w.Body.String())
	}

	// AC3: future start hidden on client, still on admin; ends_at before starts_at → 400.
	future := time.Now().UTC().Add(2 * time.Hour).Format(time.RFC3339)
	past := time.Now().UTC().Add(-2 * time.Hour).Format(time.RFC3339)
	w = doJSON(r, http.MethodPost, base, adminTok, gin.H{"language": "en", "title": "Hello", "starts_at": future})
	if w.Code != http.StatusCreated {
		t.Fatalf("future create=%d %s", w.Code, w.Body.String())
	}
	var futureRow map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &futureRow)
	w = doJSON(r, http.MethodPatch, base+"/"+futureRow["id"].(string), adminTok, gin.H{"status": "published"})
	if w.Code != http.StatusOK {
		t.Fatal(w.Body.String())
	}
	cw = doJSON(r, http.MethodGet, clientBase+"?locale=en", "", nil)
	if strings.Count(cw.Body.String(), `"Hello"`) != 1 {
		t.Fatalf("future start should stay hidden extra: %s", cw.Body.String())
	}
	aw := doJSON(r, http.MethodGet, base, adminTok, nil)
	if aw.Code != http.StatusOK || !strings.Contains(aw.Body.String(), futureRow["id"].(string)) {
		t.Fatalf("admin list missing future row: %s", aw.Body.String())
	}

	expiredRow := doJSON(r, http.MethodPost, base, adminTok, gin.H{"language": "en", "title": "Hello", "ends_at": past})
	if expiredRow.Code != http.StatusCreated {
		t.Fatalf("past end create=%d %s", expiredRow.Code, expiredRow.Body.String())
	}
	var expired map[string]any
	_ = json.Unmarshal(expiredRow.Body.Bytes(), &expired)
	w = doJSON(r, http.MethodPatch, base+"/"+expired["id"].(string), adminTok, gin.H{"status": "published"})
	if w.Code != http.StatusOK {
		t.Fatal(w.Body.String())
	}
	cw = doJSON(r, http.MethodGet, clientBase+"?locale=en", "", nil)
	if strings.Count(cw.Body.String(), `"Hello"`) != 1 {
		t.Fatalf("past end should stay hidden extra: %s", cw.Body.String())
	}
	aw = doJSON(r, http.MethodGet, base, adminTok, nil)
	if aw.Code != http.StatusOK || !strings.Contains(aw.Body.String(), expired["id"].(string)) {
		t.Fatalf("admin list missing expired row: %s", aw.Body.String())
	}

	w = doJSON(r, http.MethodPost, base, adminTok, gin.H{
		"language": "en", "title": "Hello", "starts_at": future, "ends_at": past,
	})
	if w.Code != http.StatusBadRequest || decodeErr(t, w) != "INVALID_REQUEST" {
		t.Fatalf("ends before starts=%d %s", w.Code, w.Body.String())
	}

	// AC6: illegal combo is os+arch without version. OS-only and version+arch are legal.
	w = doJSON(r, http.MethodPost, base, adminTok, gin.H{"language": "en", "title": "Hello", "os": "windows", "arch": "x86_64"})
	if w.Code != http.StatusBadRequest || decodeErr(t, w) != "INVALID_REQUEST" {
		t.Fatalf("os+arch without version=%d %s", w.Code, w.Body.String())
	}
	w = doJSON(r, http.MethodPost, base, adminTok, gin.H{"language": "en", "title": "Hello", "version_id": uuid.Must(uuid.NewRandom()).String()})
	if w.Code != http.StatusNotFound || decodeErr(t, w) != "VERSION_NOT_FOUND" {
		t.Fatalf("missing version=%d %s", w.Code, w.Body.String())
	}
	ver, _, err := projSvc.PutVersion(ctx, p.ID, "1.0.0", service.VersionWriteInput{Channel: "stable"})
	if err != nil {
		t.Fatal(err)
	}
	versionID := ver.ID.String()

	// AC4: four scopes match together, reorder changes client order.
	mk := func(body gin.H) string {
		t.Helper()
		w := doJSON(r, http.MethodPost, base, adminTok, body)
		if w.Code != http.StatusCreated {
			t.Fatalf("mk=%d %s", w.Code, w.Body.String())
		}
		var row map[string]any
		_ = json.Unmarshal(w.Body.Bytes(), &row)
		id := row["id"].(string)
		w = doJSON(r, http.MethodPatch, base+"/"+id, adminTok, gin.H{"status": "published"})
		if w.Code != http.StatusOK {
			t.Fatalf("publish mk=%d %s", w.Code, w.Body.String())
		}
		return id
	}
	idA := mk(gin.H{"language": "en", "title": "A"})
	mk(gin.H{"version_id": versionID, "language": "en", "title": "V"})
	mk(gin.H{"os": "darwin", "language": "en", "title": "O"})
	mk(gin.H{"arch": "amd64", "language": "en", "title": "R"})
	mk(gin.H{"version_id": versionID, "os": "darwin", "language": "en", "title": "VO"})
	mk(gin.H{"version_id": versionID, "arch": "amd64", "language": "en", "title": "VA"})
	idP := mk(gin.H{"version_id": versionID, "os": "darwin", "arch": "amd64", "language": "en", "title": "P"})
	cw = doJSON(r, http.MethodGet, clientBase+"?version=1.0.0&os=darwin&arch=amd64&locale=en", "", nil)
	titles := clientTitles(t, cw)
	if !containsAll(titles, "Hello", "A", "V", "O", "R", "VO", "VA", "P") {
		t.Fatalf("seven scopes union missing: %v body=%s", titles, cw.Body.String())
	}

	adminList := doJSON(r, http.MethodGet, base, adminTok, nil)
	var listed struct {
		Announcements []struct {
			ID string `json:"id"`
		} `json:"announcements"`
	}
	if err := json.Unmarshal(adminList.Body.Bytes(), &listed); err != nil {
		t.Fatal(err)
	}
	allIDs := make([]string, 0, len(listed.Announcements))
	for _, row := range listed.Announcements {
		allIDs = append(allIDs, row.ID)
	}
	// Move P to front among all ids.
	reordered := []string{idP}
	for _, existing := range allIDs {
		if existing != idP {
			reordered = append(reordered, existing)
		}
	}
	w = doJSON(r, http.MethodPut, base+"/reorder", adminTok, gin.H{"ids": reordered})
	if w.Code != http.StatusOK {
		t.Fatalf("reorder=%d %s", w.Code, w.Body.String())
	}
	cw = doJSON(r, http.MethodGet, clientBase+"?version=1.0.0&os=macos&arch=x86_64&locale=en", "", nil)
	titles = clientTitles(t, cw)
	if len(titles) == 0 || titles[0] != "P" {
		t.Fatalf("client order after reorder=%v", titles)
	}

	// AC5: omit version / arch; unknown version does not 404.
	cw = doJSON(r, http.MethodGet, clientBase+"?locale=en", "", nil)
	titles = clientTitles(t, cw)
	if contains(titles, "V") || contains(titles, "P") {
		t.Fatalf("omitted version leaked version rows: %v", titles)
	}
	if !contains(titles, "Hello") && !contains(titles, "A") {
		t.Fatalf("project-wide missing when version omitted: %v", titles)
	}
	cw = doJSON(r, http.MethodGet, clientBase+"?arch=amd64&locale=en", "", nil)
	titles = clientTitles(t, cw)
	if contains(titles, "V") || contains(titles, "P") {
		t.Fatalf("arch without version leaked version rows: %v", titles)
	}
	if !contains(titles, "R") {
		t.Fatalf("arch-only missing when version omitted: %v", titles)
	}
	cw = doJSON(r, http.MethodGet, clientBase+"?version=1.0.0&locale=en", "", nil)
	titles = clientTitles(t, cw)
	if contains(titles, "R") || contains(titles, "P") {
		t.Fatalf("omitted arch leaked arch rows: %v", titles)
	}
	cw = doJSON(r, http.MethodGet, clientBase+"?version=9.9.9&locale=en", "", nil)
	if cw.Code != http.StatusOK {
		t.Fatalf("unknown version=%d %s", cw.Code, cw.Body.String())
	}
	titles = clientTitles(t, cw)
	if contains(titles, "V") || contains(titles, "P") {
		t.Fatalf("unknown version matched: %v", titles)
	}
	if !contains(titles, "Hello") && !contains(titles, "A") {
		t.Fatalf("unknown version should still return project-wide: %v", titles)
	}
	cw = doJSON(r, http.MethodGet, clientBase+"?version=not-a-semver&locale=en", "", nil)
	if cw.Code != http.StatusBadRequest || decodeErr(t, cw) != "INVALID_QUERY_PARAM" {
		t.Fatalf("bad version=%d %s", cw.Code, cw.Body.String())
	}

	w = doJSON(r, http.MethodPatch, base+"/"+id, adminTok, gin.H{"status": "scheduled"})
	if w.Code != http.StatusBadRequest || decodeErr(t, w) != "INVALID_REQUEST" {
		t.Fatalf("scheduled without starts_at=%d %s", w.Code, w.Body.String())
	}
	w = doJSON(r, http.MethodPatch, base+"/"+id, adminTok, gin.H{"status": "scheduled", "starts_at": future})
	if w.Code != http.StatusOK {
		t.Fatalf("schedule future=%d %s", w.Code, w.Body.String())
	}
	var scheduled map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &scheduled)
	if scheduled["status"] != "scheduled" {
		t.Fatalf("scheduled status=%v", scheduled["status"])
	}
	cw = doJSON(r, http.MethodGet, clientBase+"?locale=en", "", nil)
	if strings.Contains(cw.Body.String(), `"Hello"`) {
		t.Fatalf("future scheduled leaked: %s", cw.Body.String())
	}
	w = doJSON(r, http.MethodPatch, base+"/"+id, adminTok, gin.H{"status": "scheduled", "starts_at": past})
	if w.Code != http.StatusOK {
		t.Fatalf("schedule past=%d %s", w.Code, w.Body.String())
	}
	_ = json.Unmarshal(w.Body.Bytes(), &scheduled)
	if scheduled["status"] != "published" {
		t.Fatalf("past starts_at must persist published, got %v", scheduled["status"])
	}

	// AC7: explicit locale is strict; omitted locale leftover is covered in TestAnnouncementLocaleD5HTTP.
	zhID := mk(gin.H{"language": "zh-CN", "title": "你好", "subtitle": "副", "content": "# md"})
	cw = doJSON(r, http.MethodGet, clientBase+"?locale=zh-CN", "", nil)
	if !strings.Contains(cw.Body.String(), `"你好"`) || !strings.Contains(cw.Body.String(), `"zh-CN"`) {
		t.Fatalf("zh bundle=%s", cw.Body.String())
	}
	cw = doJSON(r, http.MethodGet, clientBase+"?locale=en", "", nil)
	if strings.Contains(cw.Body.String(), `"你好"`) {
		t.Fatalf("explicit en must not leftover zh-only: %s", cw.Body.String())
	}

	indexList := doJSON(r, http.MethodGet, base, adminTok, nil)
	if indexList.Code != http.StatusOK {
		t.Fatalf("admin list=%d %s", indexList.Code, indexList.Body.String())
	}
	if strings.Contains(indexList.Body.String(), `"# md"`) {
		t.Fatalf("admin list must omit content: %s", indexList.Body.String())
	}
	detail := doJSON(r, http.MethodGet, base+"/"+zhID, adminTok, nil)
	if detail.Code != http.StatusOK || !strings.Contains(detail.Body.String(), `"# md"`) {
		t.Fatalf("admin get missing content=%d %s", detail.Code, detail.Body.String())
	}

	// AC8: token scopes.
	w = doJSON(r, http.MethodPost, "/api/v1/admin/projects/"+slug+"/tokens", adminTok, gin.H{
		"name": "pub", "scopes": []string{model.ScopeReleasePublish},
	})
	if w.Code != http.StatusCreated {
		t.Fatalf("token pub=%d %s", w.Code, w.Body.String())
	}
	var pubTok struct {
		Token string `json:"token"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &pubTok)
	w = doJSON(r, http.MethodPost, base, pubTok.Token, copy)
	if w.Code != http.StatusForbidden || decodeErr(t, w) != "FORBIDDEN" {
		t.Fatalf("release:publish write=%d %s", w.Code, w.Body.String())
	}
	w = doJSON(r, http.MethodPatch, base+"/"+idA, pubTok.Token, gin.H{"status": "draft"})
	if w.Code != http.StatusForbidden {
		t.Fatalf("release:publish patch=%d", w.Code)
	}
	w = doJSON(r, http.MethodDelete, base+"/"+idA, pubTok.Token, nil)
	if w.Code != http.StatusForbidden {
		t.Fatalf("release:publish delete=%d", w.Code)
	}
	w = doJSON(r, http.MethodPut, base+"/reorder", pubTok.Token, gin.H{"ids": reordered})
	if w.Code != http.StatusForbidden {
		t.Fatalf("release:publish reorder=%d", w.Code)
	}

	w = doJSON(r, http.MethodPost, "/api/v1/admin/projects/"+slug+"/tokens", adminTok, gin.H{
		"name": "art", "scopes": []string{model.ScopeArtifactWrite},
	})
	if w.Code != http.StatusCreated {
		t.Fatalf("token art=%d %s", w.Code, w.Body.String())
	}
	var artTok struct {
		Token string `json:"token"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &artTok)
	w = doJSON(r, http.MethodPost, base, artTok.Token, copy)
	if w.Code != http.StatusForbidden || decodeErr(t, w) != "FORBIDDEN" {
		t.Fatalf("artifact:write write=%d %s", w.Code, w.Body.String())
	}

	w = doJSON(r, http.MethodPost, "/api/v1/admin/projects/"+slug+"/tokens", adminTok, gin.H{
		"name": "read", "scopes": []string{model.ScopeProjectRead},
	})
	var readTok struct {
		Token string `json:"token"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &readTok)
	w = doJSON(r, http.MethodGet, base, readTok.Token, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("project:read list=%d %s", w.Code, w.Body.String())
	}
	w = doJSON(r, http.MethodPost, base, readTok.Token, copy)
	if w.Code != http.StatusForbidden {
		t.Fatalf("project:read write=%d", w.Code)
	}

	w = doJSON(r, http.MethodPost, "/api/v1/admin/projects/"+slug+"/tokens", adminTok, gin.H{
		"name": "adm", "scopes": []string{model.ScopeProjectAdmin},
	})
	var admTok struct {
		Token string `json:"token"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &admTok)
	w = doJSON(r, http.MethodPost, base, admTok.Token, gin.H{"language": "en", "title": "admin-write", "content": "saved body"})
	if w.Code != http.StatusCreated {
		t.Fatalf("project:admin write=%d %s", w.Code, w.Body.String())
	}
	var written map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &written); err != nil {
		t.Fatal(err)
	}
	if written["content"] != "saved body" || written["title"] != "admin-write" || written["language"] != "en" {
		t.Fatalf("create body=%s", w.Body.String())
	}
	got := doJSON(r, http.MethodGet, base+"/"+written["id"].(string), adminTok, nil)
	if got.Code != http.StatusOK || !strings.Contains(got.Body.String(), `"saved body"`) {
		t.Fatalf("get by id missing content=%d %s", got.Code, got.Body.String())
	}
	listedAfter := doJSON(r, http.MethodGet, base, adminTok, nil)
	if strings.Contains(listedAfter.Body.String(), `"saved body"`) {
		t.Fatalf("list leaked content: %s", listedAfter.Body.String())
	}

	on := true
	if _, _, err := projSvc.Patch(ctx, slug, service.PatchProjectInput{RequireClientToken: &on}); err != nil {
		t.Fatal(err)
	}
	cw = doJSON(r, http.MethodGet, clientBase, "", nil)
	if cw.Code != http.StatusUnauthorized {
		t.Fatalf("require client token=%d %s", cw.Code, cw.Body.String())
	}
	off := false
	if _, _, err := projSvc.Patch(ctx, slug, service.PatchProjectInput{RequireClientToken: &off}); err != nil {
		t.Fatal(err)
	}

	// AC9: ETag / 304 / empty 200 / s-maxage shrink.
	cw = doJSON(r, http.MethodGet, clientBase+"?locale=en", "", nil)
	if cw.Code != http.StatusOK {
		t.Fatalf("etag get=%d", cw.Code)
	}
	etag := cw.Header().Get("ETag")
	if etag == "" || !strings.Contains(cw.Header().Get("Cache-Control"), "s-maxage=") {
		t.Fatalf("headers etag=%q cc=%q", etag, cw.Header().Get("Cache-Control"))
	}
	req := httptest.NewRequest(http.MethodGet, clientBase+"?locale=en", nil)
	req.Header.Set("If-None-Match", etag)
	w304 := httptest.NewRecorder()
	r.ServeHTTP(w304, req)
	if w304.Code != http.StatusNotModified {
		t.Fatalf("304=%d body=%s", w304.Code, w304.Body.String())
	}
	if w304.Header().Get("ETag") == "" || !strings.Contains(w304.Header().Get("Cache-Control"), "s-maxage=") {
		t.Fatalf("304 headers etag=%q cc=%q", w304.Header().Get("ETag"), w304.Header().Get("Cache-Control"))
	}
	if !strings.Contains(cw.Header().Get("Vary"), "Accept-Encoding") {
		t.Fatalf("vary=%q", cw.Header().Get("Vary"))
	}

	soon := time.Now().UTC().Add(15 * time.Second).Format(time.RFC3339)
	w = doJSON(r, http.MethodPatch, base+"/"+futureRow["id"].(string), adminTok, gin.H{"starts_at": soon, "status": "published"})
	if w.Code != http.StatusOK {
		t.Fatalf("patch soon=%d %s", w.Code, w.Body.String())
	}
	cw = doJSON(r, http.MethodGet, clientBase+"?locale=en", "", nil)
	cc := cw.Header().Get("Cache-Control")
	if !strings.Contains(cc, "s-maxage=") || strings.Contains(cc, "s-maxage=60") {
		t.Fatalf("expected capped s-maxage, got %q", cc)
	}

	w = doJSON(r, http.MethodDelete, base+"/"+id, adminTok, nil)
	if w.Code != http.StatusNoContent {
		t.Fatalf("delete=%d %s", w.Code, w.Body.String())
	}

	actions := map[string]int{}
	for _, ev := range auditStore.Snapshot() {
		actions[ev.Action]++
	}
	for _, want := range []string{
		model.AuditActionAnnouncementCreate,
		model.AuditActionAnnouncementUpdate,
		model.AuditActionAnnouncementDelete,
		model.AuditActionAnnouncementReorder,
	} {
		if actions[want] == 0 {
			t.Fatalf("missing audit %s in %+v", want, actions)
		}
	}

	// leftover empty matching set: filter that cannot match platform rows without params still 200 array
	if cw.Code != http.StatusOK {
		t.Fatalf("empty-capable get=%d", cw.Code)
	}
}

func assertClientEmpty(t *testing.T, w *httptest.ResponseRecorder) {
	t.Helper()
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	if w.Header().Get("X-KiriVers-Protocol") != "" {
		t.Fatalf("protocol header must be absent: %q", w.Header().Get("X-KiriVers-Protocol"))
	}
	var env struct {
		Announcements []any `json:"announcements"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &env); err != nil {
		t.Fatalf("decode: %v body=%s", err, w.Body.String())
	}
	if env.Announcements == nil {
		t.Fatalf("announcements key missing: %s", w.Body.String())
	}
	if len(env.Announcements) != 0 {
		t.Fatalf("want empty list, got %s", w.Body.String())
	}
}

func clientTitles(t *testing.T, w *httptest.ResponseRecorder) []string {
	t.Helper()
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	if w.Header().Get("X-KiriVers-Protocol") != "" {
		t.Fatalf("protocol header must be absent: %q", w.Header().Get("X-KiriVers-Protocol"))
	}
	var env struct {
		Announcements []map[string]any `json:"announcements"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &env); err != nil {
		t.Fatal(err)
	}
	out := make([]string, 0, len(env.Announcements))
	for _, a := range env.Announcements {
		for _, leak := range []string{"status", "sort_order", "content", "version_id"} {
			if _, ok := a[leak]; ok {
				t.Fatalf("client item leaked %s: %s", leak, w.Body.String())
			}
		}
		title, _ := a["title"].(string)
		out = append(out, title)
	}
	return out
}

func containsAll(have []string, want ...string) bool {
	for _, w := range want {
		if !contains(have, w) {
			return false
		}
	}
	return true
}

func contains(have []string, want string) bool {
	for _, h := range have {
		if h == want {
			return true
		}
	}
	return false
}

func TestAnnouncementHTTPEmptyListIs200(t *testing.T) {
	r, projSvc, _, adminTok := setupAnnouncementHTTP(t)
	slug := "empty-ann"
	if _, _, err := projSvc.Create(t.Context(), service.CreateProjectInput{DefaultLocale: ptr("en"), Slug: &slug}); err != nil {
		t.Fatal(err)
	}
	w := doJSON(r, http.MethodGet, "/api/v1/projects/"+slug+"/announcements", "", nil)
	assertClientEmpty(t, w)
	if w.Header().Get("ETag") == "" {
		t.Fatal("empty list should still have ETag")
	}
	admin := doJSON(r, http.MethodGet, "/api/v1/admin/projects/"+slug+"/announcements", adminTok, nil)
	if admin.Code != http.StatusOK {
		t.Fatalf("admin empty=%d %s", admin.Code, admin.Body.String())
	}
	var env struct {
		Announcements []any `json:"announcements"`
	}
	if err := json.Unmarshal(admin.Body.Bytes(), &env); err != nil {
		t.Fatalf("admin empty decode: %v", err)
	}
	if env.Announcements == nil {
		t.Fatalf("admin empty missing announcements key: %s", admin.Body.String())
	}
}

func TestAnnouncementLocaleD5HTTP(t *testing.T) {
	r, projSvc, _, adminTok := setupAnnouncementHTTP(t)
	slug := "ann-d5"
	if _, _, err := projSvc.Create(t.Context(), service.CreateProjectInput{DefaultLocale: ptr("en"), Slug: &slug}); err != nil {
		t.Fatal(err)
	}
	base := "/api/v1/admin/projects/" + slug + "/announcements"
	clientBase := "/api/v1/projects/" + slug + "/announcements"
	mk := func(body gin.H) {
		t.Helper()
		w := doJSON(r, http.MethodPost, base, adminTok, body)
		if w.Code != http.StatusCreated {
			t.Fatalf("create=%d %s", w.Code, w.Body.String())
		}
		var row map[string]any
		_ = json.Unmarshal(w.Body.Bytes(), &row)
		w = doJSON(r, http.MethodPatch, base+"/"+row["id"].(string), adminTok, gin.H{"status": "published"})
		if w.Code != http.StatusOK {
			t.Fatalf("publish=%d %s", w.Code, w.Body.String())
		}
	}
	mk(gin.H{"language": "zh-CN", "title": "中文"})
	cw := doJSON(r, http.MethodGet, clientBase+"?locale=en", "", nil)
	if titles := clientTitles(t, cw); len(titles) != 0 {
		t.Fatalf("explicit en + only zh: %v", titles)
	}
	if strings.Contains(cw.Header().Get("Vary"), "Accept-Language") {
		t.Fatalf("explicit locale Vary=%q", cw.Header().Get("Vary"))
	}
	cw = doJSON(r, http.MethodGet, clientBase, "", nil)
	if titles := clientTitles(t, cw); len(titles) != 1 || titles[0] != "中文" {
		t.Fatalf("omit locale leftover zh: %v %s", titles, cw.Body.String())
	}
	if !strings.Contains(cw.Header().Get("Vary"), "Accept-Language") {
		t.Fatalf("omitted locale Vary=%q", cw.Header().Get("Vary"))
	}
	cw = doJSON(r, http.MethodGet, clientBase+"?locale=fr", "", nil)
	if titles := clientTitles(t, cw); len(titles) != 0 {
		t.Fatalf("locale=fr empty: %v", titles)
	}
	mk(gin.H{"language": "en", "title": "English"})
	cw = doJSON(r, http.MethodGet, clientBase+"?locale=en", "", nil)
	if titles := clientTitles(t, cw); len(titles) != 1 || titles[0] != "English" {
		t.Fatalf("zh+en locale=en: %v", titles)
	}
	req := httptest.NewRequest(http.MethodGet, clientBase, nil)
	req.Header.Set("Accept-Language", "zh-CN")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if titles := clientTitles(t, w); len(titles) != 1 || titles[0] != "中文" {
		t.Fatalf("Accept-Language zh-CN: %v %s", titles, w.Body.String())
	}
}

func TestAnnouncementVersionIDBindHTTP(t *testing.T) {
	r, projSvc, _, adminTok := setupAnnouncementHTTP(t)
	ctx := t.Context()
	slug := "ann-uuid"
	p, _, err := projSvc.Create(ctx, service.CreateProjectInput{DefaultLocale: ptr("en"), Slug: &slug})
	if err != nil {
		t.Fatal(err)
	}
	base := "/api/v1/admin/projects/" + slug + "/announcements"
	ver, _, err := projSvc.PutVersion(ctx, p.ID, "2.0.0", service.VersionWriteInput{Channel: "stable"})
	if err != nil {
		t.Fatal(err)
	}
	w := doJSON(r, http.MethodPost, base, adminTok, gin.H{
		"version_id": ver.ID.String(), "language": "en", "title": "bound",
	})
	if w.Code != http.StatusCreated {
		t.Fatalf("create=%d %s", w.Code, w.Body.String())
	}
	var created map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	if created["version_id"] != ver.ID.String() {
		t.Fatalf("version_id=%v", created["version_id"])
	}
	if _, ok := created["content"]; !ok {
		t.Fatalf("create must include content: %s", w.Body.String())
	}
	list := doJSON(r, http.MethodGet, base, adminTok, nil)
	if strings.Contains(list.Body.String(), `"content"`) {
		t.Fatalf("list leaked content: %s", list.Body.String())
	}
	if !strings.Contains(list.Body.String(), `"version_id"`) || !strings.Contains(list.Body.String(), `"language"`) {
		t.Fatalf("list missing index fields: %s", list.Body.String())
	}

	otherSlug := "ann-uuid-other"
	other, _, err := projSvc.Create(ctx, service.CreateProjectInput{DefaultLocale: ptr("en"), Slug: &otherSlug})
	if err != nil {
		t.Fatal(err)
	}
	foreign, _, err := projSvc.PutVersion(ctx, other.ID, "9.0.0", service.VersionWriteInput{Channel: "stable"})
	if err != nil {
		t.Fatal(err)
	}
	w = doJSON(r, http.MethodPost, base, adminTok, gin.H{
		"version_id": foreign.ID.String(), "language": "en", "title": "nope",
	})
	if w.Code != http.StatusNotFound || decodeErr(t, w) != "VERSION_NOT_FOUND" {
		t.Fatalf("foreign version=%d %s", w.Code, w.Body.String())
	}

	w = doJSON(r, http.MethodPatch, base+"/"+created["id"].(string), adminTok, gin.H{"status": "published"})
	if w.Code != http.StatusOK {
		t.Fatal(w.Body.String())
	}
	cw := doJSON(r, http.MethodGet, "/api/v1/projects/"+slug+"/announcements?version=2.0.0&locale=en", "", nil)
	if titles := clientTitles(t, cw); len(titles) != 1 || titles[0] != "bound" {
		t.Fatalf("client match=%v %s", titles, cw.Body.String())
	}
	if err := projSvc.DeleteVersion(ctx, p.ID, "2.0.0"); err != nil {
		t.Fatal(err)
	}
	cw = doJSON(r, http.MethodGet, "/api/v1/projects/"+slug+"/announcements?version=2.0.0&locale=en", "", nil)
	if titles := clientTitles(t, cw); contains(titles, "bound") {
		t.Fatalf("deleted version still matched: %v", titles)
	}
	admin := doJSON(r, http.MethodGet, base, adminTok, nil)
	if !strings.Contains(admin.Body.String(), created["id"].(string)) {
		t.Fatalf("admin list dropped orphan: %s", admin.Body.String())
	}
}

func TestAnnouncementPatchContentRejectsLocaleMap(t *testing.T) {
	r, projSvc, _, adminTok := setupAnnouncementHTTP(t)
	slug := "ann-patch"
	if _, _, err := projSvc.Create(t.Context(), service.CreateProjectInput{DefaultLocale: ptr("en"), Slug: &slug}); err != nil {
		t.Fatal(err)
	}
	base := "/api/v1/admin/projects/" + slug + "/announcements"
	w := doJSON(r, http.MethodPost, base, adminTok, gin.H{
		"language": "en", "title": "Hello", "content": "old ${site_url}/x",
	})
	if w.Code != http.StatusCreated {
		t.Fatalf("create=%d %s", w.Code, w.Body.String())
	}
	var created map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	id := created["id"].(string)
	w = doJSON(r, http.MethodPatch, base+"/"+id, adminTok, gin.H{
		"content": map[string]any{"en": map[string]any{"markdown": "nope"}},
	})
	if w.Code != http.StatusBadRequest || decodeErr(t, w) != "INVALID_REQUEST" {
		t.Fatalf("locale map patch=%d %s", w.Code, w.Body.String())
	}
	w = doJSON(r, http.MethodPatch, base+"/"+id, adminTok, gin.H{"content": "new ${site_url}/y"})
	if w.Code != http.StatusOK {
		t.Fatalf("patch=%d %s", w.Code, w.Body.String())
	}
	got := doJSON(r, http.MethodGet, base+"/"+id, adminTok, nil)
	if got.Code != http.StatusOK || !strings.Contains(got.Body.String(), `"new ${site_url}/y"`) {
		t.Fatalf("get after patch=%d %s", got.Code, got.Body.String())
	}
	if strings.Contains(got.Body.String(), "old ${site_url}") {
		t.Fatalf("old content remained: %s", got.Body.String())
	}
}
