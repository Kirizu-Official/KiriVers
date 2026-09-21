package client

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Kirizu-Official/KiriVers/internal/model"
	"github.com/Kirizu-Official/KiriVers/internal/service"
)

func TestClientCatalogChannelsAndMatrix(t *testing.T) {
	r, projSvc, _, ctx := setupCheckTest(t)
	slug := "cat-app"
	p, _, err := projSvc.Create(ctx, service.CreateProjectInput{DefaultLocale: ptr("en"), Slug: &slug})
	if err != nil {
		t.Fatal(err)
	}
	name := "Insider"
	unlisted := true
	token := "s3cret"
	if _, err := projSvc.CreateChannel(ctx, p.ID, service.ChannelWrite{
		Name: &name, Slug: ptr("insider"), Unlisted: &unlisted, Token: &token,
	}); err != nil {
		t.Fatal(err)
	}
	hiddenName := "Hidden"
	if _, err := projSvc.CreateChannel(ctx, p.ID, service.ChannelWrite{
		Name: &hiddenName, Slug: ptr("hidden"), Unlisted: &unlisted,
	}); err != nil {
		t.Fatal(err)
	}
	off := false
	if _, err := projSvc.PatchChannel(ctx, p.ID, model.ChannelAlpha, service.ChannelWrite{Enabled: &off}); err != nil {
		t.Fatal(err)
	}
	if _, err := projSvc.CreateMatrix(ctx, p.ID, service.MatrixWrite{
		OS: ptr("windows"), Arch: ptr("x86_64"), PackageType: ptr(model.PackageTypeSingleFile),
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := projSvc.CreateMatrix(ctx, p.ID, service.MatrixWrite{
		OS: ptr("linux"), Arch: ptr("arm64"), PackageType: ptr(model.PackageTypeMultiFile),
	}); err != nil {
		t.Fatal(err)
	}

	w := perform(r, http.MethodGet, "/api/v1/projects/cat-app/channels")
	if w.Code != http.StatusOK {
		t.Fatalf("list channels=%d %s", w.Code, w.Body.String())
	}
	var listed struct {
		Channels []struct {
			Name          string `json:"name"`
			Slug          string `json:"slug"`
			StabilityRank int    `json:"stability_rank"`
		} `json:"channels"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &listed); err != nil {
		t.Fatal(err)
	}
	got := map[string]bool{}
	for _, ch := range listed.Channels {
		got[ch.Slug] = true
		if ch.Slug == "stable" && (ch.Name == "" || ch.StabilityRank != 30) {
			t.Fatalf("stable=%+v", ch)
		}
	}
	if !got["stable"] || !got["beta"] {
		t.Fatalf("public list=%v", got)
	}
	if got["alpha"] || got["insider"] || got["hidden"] {
		t.Fatalf("disabled/unlisted leaked: %v", got)
	}

	w = perform(r, http.MethodGet, "/api/v1/projects/cat-app/channels/hidden")
	if w.Code != http.StatusNotFound {
		t.Fatalf("client channel-by-slug must be unregistered: %d %s", w.Code, w.Body.String())
	}

	w = perform(r, http.MethodGet, "/api/v1/projects/cat-app/matrix")
	if w.Code != http.StatusOK {
		t.Fatalf("matrix=%d %s", w.Code, w.Body.String())
	}
	var mtx struct {
		Matrix []struct {
			OS          string `json:"os"`
			Arch        string `json:"arch"`
			PackageType string `json:"package_type"`
		} `json:"matrix"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &mtx); err != nil {
		t.Fatal(err)
	}
	if len(mtx.Matrix) != 2 {
		t.Fatalf("matrix rows=%d %s", len(mtx.Matrix), w.Body.String())
	}
}

func TestClientLanguagesCatalog(t *testing.T) {
	r, projSvc, _, ctx := setupCheckTest(t)
	slug := "lang-cat"
	p, _, err := projSvc.Create(ctx, service.CreateProjectInput{DefaultLocale: ptr("en"), Slug: &slug})
	if err != nil {
		t.Fatal(err)
	}
	w := perform(r, http.MethodGet, "/api/v1/projects/lang-cat/languages")
	if w.Code != http.StatusOK {
		t.Fatalf("languages=%d %s", w.Code, w.Body.String())
	}
	var env struct {
		Languages []struct {
			Code        string `json:"code"`
			DisplayName string `json:"display_name"`
			IsDefault   bool   `json:"is_default"`
			SortOrder   int    `json:"sort_order"`
			ProjectID   string `json:"project_id"`
		} `json:"languages"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &env); err != nil {
		t.Fatal(err)
	}
	if env.Languages == nil {
		t.Fatalf("missing languages key: %s", w.Body.String())
	}
	if len(env.Languages) < 1 {
		t.Fatalf("want default row, got %s", w.Body.String())
	}
	foundDefault := false
	for _, row := range env.Languages {
		if row.ProjectID != "" {
			t.Fatalf("client languages must omit project_id: %s", w.Body.String())
		}
		if row.IsDefault && row.Code == "en" {
			foundDefault = true
		}
	}
	if !foundDefault {
		t.Fatalf("missing default en: %s", w.Body.String())
	}

	if _, err := projSvc.CreateLanguage(ctx, p.ID, service.LanguageWrite{Code: ptr("zh-CN")}); err != nil {
		t.Fatal(err)
	}
	w = perform(r, http.MethodGet, "/api/v1/projects/lang-cat/languages")
	if err := json.Unmarshal(w.Body.Bytes(), &env); err != nil {
		t.Fatal(err)
	}
	codes := map[string]bool{}
	for _, row := range env.Languages {
		codes[row.Code] = true
	}
	if !codes["en"] || !codes["zh-CN"] {
		t.Fatalf("codes=%v", codes)
	}

	on := true
	if _, _, err := projSvc.Patch(ctx, slug, service.PatchProjectInput{RequireClientToken: &on}); err != nil {
		t.Fatal(err)
	}
	w = perform(r, http.MethodGet, "/api/v1/projects/lang-cat/languages")
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("require token=%d %s", w.Code, w.Body.String())
	}
	issued, err := projSvc.CreateToken(ctx, slug, "client", []string{model.ScopeProjectRead}, nil)
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/projects/lang-cat/languages", nil)
	req.Header.Set("Authorization", "Bearer "+issued.Plaintext)
	authW := httptest.NewRecorder()
	r.ServeHTTP(authW, req)
	if authW.Code != http.StatusOK {
		t.Fatalf("token languages=%d %s", authW.Code, authW.Body.String())
	}
}
