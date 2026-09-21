package admin

import (
	"encoding/json"
	"net/http"
	"testing"
)

// TestStoreListingCRUDDuplicateAndIllegalSlug AC16：listing CRUD；重复 (protocol, slug)
// 与非法 slug 均为 400 INVALID_REQUEST（不是 409）。
func TestStoreListingCRUDDuplicateAndIllegalSlug(t *testing.T) {
	r, _, _, token := setupProjectHTTP(t)
	created := doJSON(r, http.MethodPost, "/api/v1/admin/projects", token, map[string]any{
		"slug":           "list-app",
		"default_locale": "en",
	})
	if created.Code != http.StatusCreated {
		t.Fatalf("create project=%d %s", created.Code, created.Body.String())
	}

	body := map[string]any{
		"protocol":       "sparkle",
		"slug":           "macos-app",
		"enabled":        true,
		"package_source": "line_full",
		"identifiers":    map[string]string{"homepage": "https://example.test"},
	}
	w := doJSON(r, http.MethodPost, "/api/v1/admin/projects/list-app/store-listings", token, body)
	if w.Code != http.StatusCreated {
		t.Fatalf("create listing=%d %s", w.Code, w.Body.String())
	}
	var row map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &row); err != nil {
		t.Fatal(err)
	}
	if row["protocol"] != "sparkle" || row["slug"] != "macos-app" {
		t.Fatalf("listing body=%s", w.Body.String())
	}
	storeURL, _ := row["store_url"].(string)
	if storeURL != "/api/v1/projects/list-app/store/sparkle/macos-app/appcast.xml" {
		t.Fatalf("store_url=%q", storeURL)
	}

	dup := doJSON(r, http.MethodPost, "/api/v1/admin/projects/list-app/store-listings", token, body)
	if dup.Code != http.StatusBadRequest {
		t.Fatalf("duplicate listing must be 400, got %d %s", dup.Code, dup.Body.String())
	}
	if decodeErr(t, dup) != "INVALID_REQUEST" {
		t.Fatalf("duplicate code=%s body=%s", decodeErr(t, dup), dup.Body.String())
	}

	bad := doJSON(r, http.MethodPost, "/api/v1/admin/projects/list-app/store-listings", token, map[string]any{
		"protocol": "sparkle",
		"slug":     "bad_slug",
	})
	if bad.Code != http.StatusBadRequest || decodeErr(t, bad) != "INVALID_REQUEST" {
		t.Fatalf("illegal slug must be 400 INVALID_REQUEST, got %d %s", bad.Code, bad.Body.String())
	}

	list := doJSON(r, http.MethodGet, "/api/v1/admin/projects/list-app/store-listings", token, nil)
	if list.Code != http.StatusOK {
		t.Fatalf("list=%d %s", list.Code, list.Body.String())
	}
	var env map[string]any
	if err := json.Unmarshal(list.Body.Bytes(), &env); err != nil {
		t.Fatal(err)
	}
	listings, _ := env["listings"].([]any)
	if len(listings) != 1 {
		t.Fatalf("listings=%s", list.Body.String())
	}

	id, _ := row["id"].(string)
	if id == "" {
		t.Fatalf("missing id: %s", w.Body.String())
	}
	patched := doJSON(r, http.MethodPatch, "/api/v1/admin/projects/list-app/store-listings/"+id, token, map[string]any{
		"enabled": false,
	})
	if patched.Code != http.StatusOK {
		t.Fatalf("patch=%d %s", patched.Code, patched.Body.String())
	}

	del := doJSON(r, http.MethodDelete, "/api/v1/admin/projects/list-app/store-listings/"+id, token, nil)
	if del.Code != http.StatusNoContent {
		t.Fatalf("delete=%d %s", del.Code, del.Body.String())
	}

	missingPath := doJSON(r, http.MethodPost, "/api/v1/admin/projects/list-app/store-listings", token, map[string]any{
		"protocol":       "sparkle",
		"slug":           "need-path",
		"package_source": "manifest_path",
	})
	if missingPath.Code != http.StatusBadRequest || decodeErr(t, missingPath) != "INVALID_REQUEST" {
		t.Fatalf("manifest_path required: %d %s", missingPath.Code, missingPath.Body.String())
	}
}
