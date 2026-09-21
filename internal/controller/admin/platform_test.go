package admin

import (
	"bytes"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/Kirizu-Official/KiriVers/internal/model"
	"github.com/Kirizu-Official/KiriVers/internal/platform"
)

func TestPlatformChannelHwRevHTTPAcceptance(t *testing.T) {
	r, projSvc, _, adminTok := setupProjectHTTP(t)
	w := doJSON(r, http.MethodPost, "/api/v1/admin/projects", adminTok, map[string]any{"slug": "plat-app", "default_locale": "en"})
	if w.Code != http.StatusCreated {
		t.Fatalf("create project=%d %s", w.Code, w.Body.String())
	}

	w = doJSON(r, http.MethodGet, "/api/v1/admin/projects/plat-app/channels", adminTok, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("list channels=%d %s", w.Code, w.Body.String())
	}
	var chBody struct {
		Channels []struct {
			Slug          string `json:"slug"`
			StabilityRank int    `json:"stability_rank"`
			System        bool   `json:"system"`
			Enabled       bool   `json:"enabled"`
		} `json:"channels"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &chBody); err != nil {
		t.Fatal(err)
	}
	if len(chBody.Channels) != 3 {
		t.Fatalf("channels=%d body=%s", len(chBody.Channels), w.Body.String())
	}
	if chBody.Channels[0].Slug != "alpha" || chBody.Channels[0].StabilityRank != 10 {
		t.Fatalf("alpha=%+v", chBody.Channels[0])
	}
	if chBody.Channels[1].Slug != "beta" || chBody.Channels[1].StabilityRank != 20 {
		t.Fatalf("beta=%+v", chBody.Channels[1])
	}
	if chBody.Channels[2].Slug != "stable" || chBody.Channels[2].StabilityRank != 30 || !chBody.Channels[2].System {
		t.Fatalf("stable=%+v", chBody.Channels[2])
	}

	w = doJSON(r, http.MethodDelete, "/api/v1/admin/projects/plat-app/channels/stable", adminTok, nil)
	if w.Code != http.StatusConflict {
		t.Fatalf("delete stable=%d %s", w.Code, w.Body.String())
	}
	if decodeErr(t, w) != "SYSTEM_CHANNEL" {
		t.Fatalf("delete stable code=%s", decodeErr(t, w))
	}

	w = doJSON(r, http.MethodPatch, "/api/v1/admin/projects/plat-app/channels/stable", adminTok, map[string]any{
		"enabled": false,
	})
	if w.Code != http.StatusOK {
		t.Fatalf("disable stable=%d %s", w.Code, w.Body.String())
	}

	w = doJSON(r, http.MethodPost, "/api/v1/admin/projects/plat-app/channels", adminTok, map[string]any{
		"slug": "stable",
	})
	if w.Code != http.StatusConflict || decodeErr(t, w) != "SLUG_TAKEN" {
		t.Fatalf("recreate stable=%d %s", w.Code, w.Body.String())
	}

	w = doJSON(r, http.MethodPost, "/api/v1/admin/projects/plat-app/channels", adminTok, map[string]any{
		"name": "Nightly", "slug": "nightly", "stability_rank": 5, "token": "plain-nightly-token",
	})
	if w.Code != http.StatusCreated {
		t.Fatalf("custom channel=%d %s", w.Code, w.Body.String())
	}
	var night map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &night); err != nil {
		t.Fatal(err)
	}
	if night["token"] != "plain-nightly-token" || night["token_required"] != true {
		t.Fatalf("create channel token=%s", w.Body.String())
	}
	w = doJSON(r, http.MethodGet, "/api/v1/admin/projects/plat-app/channels", adminTok, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("list channels after nightly=%d %s", w.Code, w.Body.String())
	}
	if !bytes.Contains(w.Body.Bytes(), []byte(`"token":"plain-nightly-token"`)) {
		t.Fatalf("list must return plaintext token: %s", w.Body.String())
	}

	w = doJSON(r, http.MethodPost, "/api/v1/admin/projects/plat-app/channels", adminTok, map[string]any{
		"name": "LTS", "slug": "lts",
	})
	if w.Code != http.StatusCreated {
		t.Fatalf("lts channel=%d %s", w.Code, w.Body.String())
	}
	if bytes.Contains(w.Body.Bytes(), []byte(`"is_lts"`)) {
		t.Fatalf("lts must not set is_lts: %s", w.Body.String())
	}

	w = doJSON(r, http.MethodPost, "/api/v1/admin/projects/plat-app/channels", adminTok, map[string]any{
		"slug": "has_underscore",
	})
	if w.Code != http.StatusBadRequest {
		t.Fatalf("underscore slug=%d %s", w.Code, w.Body.String())
	}

	w = doJSON(r, http.MethodPost, "/api/v1/admin/projects/plat-app/matrix", adminTok, map[string]any{
		"os": "darwin", "arch": "amd64", "package_type": "single_file",
	})
	if w.Code != http.StatusCreated {
		t.Fatalf("matrix create=%d %s", w.Code, w.Body.String())
	}
	var mtx map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &mtx); err != nil {
		t.Fatal(err)
	}
	if mtx["os"] != "macos" || mtx["arch"] != "x86_64" {
		t.Fatalf("canonical store=%s", w.Body.String())
	}
	if mtx["delta_algo"] != "hdiffpatch" {
		t.Fatalf("delta_algo=%v", mtx["delta_algo"])
	}
	if mtx["delta_source_count"] != float64(3) {
		t.Fatalf("delta_source_count=%v", mtx["delta_source_count"])
	}
	if fb, _ := mtx["fallback_arch"].(string); fb != "" {
		t.Fatalf("fallback_arch=%v", mtx["fallback_arch"])
	}
	if mtx["minimum_supported_version"] != nil {
		t.Fatalf("msv=%v", mtx["minimum_supported_version"])
	}

	w = doJSON(r, http.MethodPatch, "/api/v1/admin/projects/plat-app/matrix/darwin/amd64", adminTok, map[string]any{
		"package_type": "multi_file",
	})
	if w.Code != http.StatusOK {
		t.Fatalf("unlocked package_type=%d %s", w.Code, w.Body.String())
	}

	p, err := projSvc.Resolve(t.Context(), "plat-app")
	if err != nil {
		t.Fatal(err)
	}
	ver, err := projSvc.CreatePublishedVersion(t.Context(), p.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := projSvc.CreateVersionLine(t.Context(), ver.ID, "darwin", "amd64"); err != nil {
		t.Fatal(err)
	}
	w = doJSON(r, http.MethodPatch, "/api/v1/admin/projects/plat-app/matrix/macos/x86_64", adminTok, map[string]any{
		"package_type": "single_file",
	})
	if w.Code != http.StatusConflict {
		t.Fatalf("immutable=%d %s", w.Code, w.Body.String())
	}
	if decodeErr(t, w) != "PACKAGE_TYPE_IMMUTABLE" {
		t.Fatalf("immutable code=%s", decodeErr(t, w))
	}

	w = doJSON(r, http.MethodPost, "/api/v1/admin/projects/plat-app/hw-revs", adminTok, map[string]any{
		"slug": "v1", "rank": 1, "notes": "",
	})
	if w.Code != http.StatusCreated {
		t.Fatalf("hw v1=%d %s", w.Code, w.Body.String())
	}

	w = doJSON(r, http.MethodPost, "/api/v1/admin/projects/plat-app/hw-revs", adminTok, map[string]any{
		"slug": "rev-b", "rank": 20, "notes": "newer",
	})
	if w.Code != http.StatusCreated {
		t.Fatalf("hw b=%d %s", w.Code, w.Body.String())
	}
	w = doJSON(r, http.MethodPost, "/api/v1/admin/projects/plat-app/hw-revs", adminTok, map[string]any{
		"slug": "rev-a", "rank": 10,
	})
	if w.Code != http.StatusCreated {
		t.Fatalf("hw a=%d %s", w.Code, w.Body.String())
	}
	w = doJSON(r, http.MethodGet, "/api/v1/admin/projects/plat-app/hw-revs", adminTok, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("list hw=%d %s", w.Code, w.Body.String())
	}
	var hwBody struct {
		HwRevs []struct {
			Slug string `json:"slug"`
			Rank int    `json:"rank"`
		} `json:"hw_revs"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &hwBody); err != nil {
		t.Fatal(err)
	}
	if len(hwBody.HwRevs) != 3 || hwBody.HwRevs[0].Slug != "v1" || hwBody.HwRevs[1].Slug != "rev-a" || hwBody.HwRevs[2].Slug != "rev-b" {
		t.Fatalf("hw order=%s", w.Body.String())
	}
	if err := projSvc.AssertHwRevKnown(t.Context(), p.ID, "ghost"); err == nil {
		t.Fatal("expected HW_REV_UNKNOWN")
	}

	readTok := issueReadToken(t, r, adminTok, "plat-app")
	w = doJSON(r, http.MethodGet, "/api/v1/admin/projects/plat-app/channels", readTok, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("read token GET channels=%d %s", w.Code, w.Body.String())
	}
	w = doJSON(r, http.MethodPost, "/api/v1/admin/projects/plat-app/channels", readTok, map[string]any{"slug": "insider"})
	if w.Code != http.StatusForbidden {
		t.Fatalf("read token POST channel=%d %s", w.Code, w.Body.String())
	}

	w = doJSON(r, http.MethodGet, "/api/v1/admin/projects/plat-app/platforms/catalog", adminTok, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("catalog=%d %s", w.Code, w.Body.String())
	}
	var cat struct {
		OS []struct {
			Slug    string   `json:"slug"`
			Aliases []string `json:"aliases"`
		} `json:"os"`
		Arch []struct {
			Slug    string   `json:"slug"`
			Aliases []string `json:"aliases"`
		} `json:"arch"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &cat); err != nil {
		t.Fatal(err)
	}
	if len(cat.OS) != len(platform.PresetOS) || len(cat.Arch) != len(platform.PresetArch) {
		t.Fatalf("catalog size os=%d arch=%d", len(cat.OS), len(cat.Arch))
	}
	wantOSAliases := map[string][]string{platform.OSMacOS: {"darwin", "osx"}}
	wantArchAliases := map[string][]string{
		platform.ArchX86:    {"i386", "i686"},
		platform.ArchX86_64: {"amd64"},
		platform.ArchARM64:  {"aarch64"},
		platform.ArchARMv7:  {"armv7a", "armv7l"},
	}
	osBySlug := map[string][]string{}
	for _, e := range cat.OS {
		osBySlug[e.Slug] = e.Aliases
	}
	archBySlug := map[string][]string{}
	for _, e := range cat.Arch {
		archBySlug[e.Slug] = e.Aliases
	}
	for slug, aliases := range wantOSAliases {
		got := osBySlug[slug]
		for _, a := range aliases {
			if !containsStr(got, a) {
				t.Fatalf("os %s missing alias %s in %v", slug, a, got)
			}
		}
	}
	for slug, aliases := range wantArchAliases {
		got := archBySlug[slug]
		for _, a := range aliases {
			if !containsStr(got, a) {
				t.Fatalf("arch %s missing alias %s in %v", slug, a, got)
			}
		}
	}
	if _, ok := osBySlug[platform.OSiPadOS]; !ok {
		t.Fatal("catalog missing ipados")
	}
}

func containsStr(list []string, want string) bool {
	for _, v := range list {
		if v == want {
			return true
		}
	}
	return false
}

func TestPlatformHTTPUnauthorized(t *testing.T) {
	r, _, _, _ := setupProjectHTTP(t)
	w := doJSON(r, http.MethodGet, "/api/v1/admin/projects/missing/channels", "", nil)
	if w.Code != http.StatusUnauthorized || decodeErr(t, w) != "UNAUTHORIZED" {
		t.Fatalf("unauth=%d %s", w.Code, w.Body.String())
	}
}

func issueReadToken(t *testing.T, r http.Handler, adminTok, slug string) string {
	t.Helper()
	w := doJSON(r, http.MethodPost, "/api/v1/admin/projects/"+slug+"/tokens", adminTok, map[string]any{
		"name":   "reader",
		"scopes": []string{model.ScopeProjectRead},
	})
	if w.Code != http.StatusCreated {
		t.Fatalf("issue read token=%d %s", w.Code, w.Body.String())
	}
	var body struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	return body.Token
}
