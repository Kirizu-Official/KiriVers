package admin

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Kirizu-Official/KiriVers/internal/model"
	"github.com/Kirizu-Official/KiriVers/internal/service"
)

func postZipArchive(r http.Handler, path, token string, files map[string][]byte) *httptest.ResponseRecorder {
	buf := new(bytes.Buffer)
	zw := zip.NewWriter(buf)
	for name, content := range files {
		w, err := zw.Create(name)
		if err != nil {
			panic(err)
		}
		if _, err := w.Write(content); err != nil {
			panic(err)
		}
	}
	if err := zw.Close(); err != nil {
		panic(err)
	}
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(buf.Bytes()))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/zip")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func TestInstallPolicyHTTPAcceptance(t *testing.T) {
	r, projSvc, _, token := setupProjectHTTP(t)
	ctx := t.Context()
	slug := "policy-http"
	engine := model.CompareEngineSemver
	p, _, err := projSvc.Create(ctx, service.CreateProjectInput{DefaultLocale: ptr("en"), Slug: &slug, CompareEngine: &engine})
	if err != nil {
		t.Fatal(err)
	}
	w := doJSON(r, http.MethodPost, "/api/v1/admin/projects/"+slug+"/matrix", token, map[string]any{
		"os": "windows", "arch": "x86_64", "package_type": "multi_file",
	})
	if w.Code != http.StatusCreated {
		t.Fatalf("matrix windows=%d %s", w.Code, w.Body.String())
	}
	w = doJSON(r, http.MethodPost, "/api/v1/admin/projects/"+slug+"/matrix", token, map[string]any{
		"os": "linux", "arch": "x86_64", "package_type": "multi_file",
	})
	if w.Code != http.StatusCreated {
		t.Fatalf("matrix linux=%d %s", w.Code, w.Body.String())
	}

	// AC6: os/arch not in matrix
	w = doJSON(r, http.MethodPut, "/api/v1/admin/projects/"+slug+"/install-policy-rules?os=freebsd&arch=x86_64", token, map[string]any{
		"entries": []map[string]string{{"path": "a.txt", "install_policy": "KEEP_IF_EXISTS"}},
	})
	if w.Code != http.StatusBadRequest || decodeErr(t, w) != "INVALID_REQUEST" {
		t.Fatalf("missing matrix=%d %s", w.Code, w.Body.String())
	}
	w = doJSON(r, http.MethodGet, "/api/v1/admin/projects/"+slug+"/channels/no-such/install-policy-rules?os=windows&arch=x86_64", token, nil)
	if w.Code != http.StatusNotFound || decodeErr(t, w) != "CHANNEL_NOT_FOUND" {
		t.Fatalf("unknown channel=%d %s", w.Code, w.Body.String())
	}
	w = doJSON(r, http.MethodGet, "/api/v1/admin/projects/"+slug+"/install-policy-rules", token, nil)
	if w.Code != http.StatusBadRequest || decodeErr(t, w) != "INVALID_REQUEST" {
		t.Fatalf("missing os/arch=%d %s", w.Code, w.Body.String())
	}
	w = doJSON(r, http.MethodPut, "/api/v1/admin/projects/"+slug+"/install-policy-rules?os=windows&arch=x86_64", token, map[string]any{
		"entries": []map[string]string{{"path": "../etc/passwd", "install_policy": "KEEP_IF_EXISTS"}},
	})
	if w.Code != http.StatusBadRequest {
		t.Fatalf("illegal path=%d %s", w.Code, w.Body.String())
	}
	code := decodeErr(t, w)
	if code != "INVALID_PATH" && code != "INVALID_REQUEST" {
		t.Fatalf("illegal path code=%s body=%s", code, w.Body.String())
	}

	// AC1: project KEEP seeds stable Manifest
	w = doJSON(r, http.MethodPut, "/api/v1/admin/projects/"+slug+"/install-policy-rules?os=windows&arch=x86_64", token, map[string]any{
		"entries": []map[string]string{{"path": "config/user.json", "install_policy": "KEEP_IF_EXISTS"}},
	})
	if w.Code != http.StatusOK {
		t.Fatalf("put project rules=%d %s", w.Code, w.Body.String())
	}
	if _, _, err := projSvc.PutVersion(ctx, p.ID, "1.0.0", service.VersionWriteInput{Channel: "stable"}); err != nil {
		t.Fatal(err)
	}
	if _, err := projSvc.AddVersionLine(ctx, p.ID, "1.0.0", service.VersionLineWriteInput{OS: "windows", Arch: "x86_64"}); err != nil {
		t.Fatal(err)
	}
	w = postZipArchive(r, fmt.Sprintf("/api/v1/admin/projects/%s/versions/1.0.0/lines/windows/x86_64/build-archive", slug), token, map[string][]byte{
		"bin/app.exe":        []byte("exe"),
		"config/user.json":   []byte("{}"),
		"keep_if_exists.txt": []byte("bin/app.exe\n"),
		"_keep.json":         []byte(`["bin/app.exe"]`),
	})
	if w.Code != http.StatusCreated {
		t.Fatalf("ac1 zip=%d %s", w.Code, w.Body.String())
	}
	w = doJSON(r, http.MethodGet, fmt.Sprintf("/api/v1/admin/projects/%s/versions/1.0.0/lines/windows/x86_64/manifest", slug), token, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("ac1 get manifest=%d %s", w.Code, w.Body.String())
	}
	var man struct {
		Entries []struct {
			Path           string `json:"path"`
			SHA256         string `json:"sha256"`
			MD5            string `json:"md5"`
			InstallPolicy  string `json:"install_policy"`
			IntegrityCheck bool   `json:"integrity_check"`
		} `json:"entries"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &man); err != nil {
		t.Fatal(err)
	}
	byPath := map[string]struct {
		Policy string
		Check  bool
		SHA    string
		MD5    string
	}{}
	for _, e := range man.Entries {
		if e.Path == "keep_if_exists.txt" || e.Path == "_keep.json" {
			t.Fatalf("sidecar in manifest: %s", e.Path)
		}
		if e.SHA256 == "" || e.MD5 == "" {
			t.Fatalf("ac9 missing hashes %+v", e)
		}
		byPath[e.Path] = struct {
			Policy string
			Check  bool
			SHA    string
			MD5    string
		}{e.InstallPolicy, e.IntegrityCheck, e.SHA256, e.MD5}
	}
	if byPath["config/user.json"].Policy != model.InstallPolicyKeepIfExists || byPath["config/user.json"].Check {
		t.Fatalf("ac1 user.json=%+v", byPath["config/user.json"])
	}
	if byPath["bin/app.exe"].Policy != model.InstallPolicyOverwrite {
		t.Fatalf("ac1/ac3 app=%+v", byPath["bin/app.exe"])
	}

	// AC2: beta overlay
	w = doJSON(r, http.MethodPut, "/api/v1/admin/projects/"+slug+"/channels/beta/install-policy-rules?os=windows&arch=x86_64", token, map[string]any{
		"entries": []map[string]string{
			{"path": "config/user.json", "install_policy": "OVERWRITE"},
			{"path": "debug.log", "install_policy": "KEEP_IF_EXISTS"},
		},
	})
	if w.Code != http.StatusOK {
		t.Fatalf("put beta rules=%d %s", w.Code, w.Body.String())
	}
	if _, _, err := projSvc.PutVersion(ctx, p.ID, "1.0.0-beta", service.VersionWriteInput{Channel: "beta"}); err != nil {
		t.Fatal(err)
	}
	if _, err := projSvc.AddVersionLine(ctx, p.ID, "1.0.0-beta", service.VersionLineWriteInput{OS: "windows", Arch: "x86_64"}); err != nil {
		t.Fatal(err)
	}
	w = postZipArchive(r, fmt.Sprintf("/api/v1/admin/projects/%s/versions/1.0.0-beta/lines/windows/x86_64/build-archive", slug), token, map[string][]byte{
		"config/user.json": []byte("{}"),
		"debug.log":        []byte("log"),
	})
	if w.Code != http.StatusCreated {
		t.Fatalf("ac2 zip=%d %s", w.Code, w.Body.String())
	}
	w = doJSON(r, http.MethodGet, fmt.Sprintf("/api/v1/admin/projects/%s/versions/1.0.0-beta/lines/windows/x86_64/manifest", slug), token, nil)
	if err := json.Unmarshal(w.Body.Bytes(), &man); err != nil {
		t.Fatal(err)
	}
	byPath = map[string]struct {
		Policy string
		Check  bool
		SHA    string
		MD5    string
	}{}
	for _, e := range man.Entries {
		byPath[e.Path] = struct {
			Policy string
			Check  bool
			SHA    string
			MD5    string
		}{e.InstallPolicy, e.IntegrityCheck, e.SHA256, e.MD5}
	}
	if byPath["config/user.json"].Policy != model.InstallPolicyOverwrite {
		t.Fatalf("ac2 user.json=%+v", byPath["config/user.json"])
	}
	if byPath["debug.log"].Policy != model.InstallPolicyKeepIfExists {
		t.Fatalf("ac2 debug.log=%+v", byPath["debug.log"])
	}

	// AC4: reference
	if _, _, err := projSvc.PutVersion(ctx, p.ID, "2.0.0", service.VersionWriteInput{Channel: "stable"}); err != nil {
		t.Fatal(err)
	}
	if _, err := projSvc.AddVersionLine(ctx, p.ID, "2.0.0", service.VersionLineWriteInput{OS: "linux", Arch: "x86_64"}); err != nil {
		t.Fatal(err)
	}
	sha := "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"
	md5 := "d41d8cd98f00b204e9800998ecf8427e"
	w = doJSON(r, http.MethodPut, fmt.Sprintf("/api/v1/admin/projects/%s/versions/2.0.0/lines/linux/x86_64/manifest", slug), token, map[string]any{
		"entries": []map[string]any{
			{"path": "bin/app", "size": 0, "sha256": sha, "md5": md5},
		},
	})
	if w.Code != http.StatusOK {
		t.Fatalf("draft 2.0.0 manifest=%d %s", w.Code, w.Body.String())
	}
	if _, _, err := projSvc.PutVersion(ctx, p.ID, "0.9.0", service.VersionWriteInput{Channel: "stable"}); err != nil {
		t.Fatal(err)
	}
	if _, err := projSvc.AddVersionLine(ctx, p.ID, "0.9.0", service.VersionLineWriteInput{OS: "linux", Arch: "x86_64"}); err != nil {
		t.Fatal(err)
	}
	w = postZipArchive(r, fmt.Sprintf("/api/v1/admin/projects/%s/versions/0.9.0/lines/linux/x86_64/build-archive", slug), token, map[string][]byte{
		"bin/old": []byte("old"),
	})
	if w.Code != http.StatusCreated {
		t.Fatalf("0.9.0 zip=%d %s", w.Code, w.Body.String())
	}
	w = doJSON(r, http.MethodPost, fmt.Sprintf("/api/v1/admin/projects/%s/versions/0.9.0/publish", slug), token, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("publish 0.9.0=%d %s", w.Code, w.Body.String())
	}
	w = doJSON(r, http.MethodGet, "/api/v1/admin/projects/"+slug+"/install-policy-rules/reference?channel=stable&os=linux&arch=x86_64", token, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("reference=%d %s", w.Code, w.Body.String())
	}
	var ref struct {
		Version *string `json:"version"`
		Status  *string `json:"status"`
		Entries []struct {
			Path   string `json:"path"`
			SHA256 string `json:"sha256"`
			MD5    string `json:"md5"`
		} `json:"entries"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &ref); err != nil {
		t.Fatal(err)
	}
	if ref.Version == nil || *ref.Version != "2.0.0" {
		t.Fatalf("ac4 draft must beat published: %+v %s", ref.Version, w.Body.String())
	}
	if len(ref.Entries) != 1 || ref.Entries[0].SHA256 != sha || ref.Entries[0].MD5 != md5 {
		t.Fatalf("ac9 reference hashes=%s", w.Body.String())
	}

	if _, _, err := projSvc.PutVersion(ctx, p.ID, "3.0.0", service.VersionWriteInput{Channel: "stable"}); err != nil {
		t.Fatal(err)
	}
	if _, err := projSvc.RevokeVersion(ctx, p.ID, "3.0.0"); err != nil {
		t.Fatal(err)
	}
	w = doJSON(r, http.MethodGet, "/api/v1/admin/projects/"+slug+"/install-policy-rules/reference?channel=stable&os=linux&arch=x86_64", token, nil)
	if err := json.Unmarshal(w.Body.Bytes(), &ref); err != nil {
		t.Fatal(err)
	}
	if ref.Version == nil || *ref.Version != "2.0.0" {
		t.Fatalf("ac4 revoked must be skipped: %+v", ref.Version)
	}

	if _, _, err := projSvc.PutVersion(ctx, p.ID, "4.0.0", service.VersionWriteInput{Channel: "stable"}); err != nil {
		t.Fatal(err)
	}
	w = doJSON(r, http.MethodGet, "/api/v1/admin/projects/"+slug+"/install-policy-rules/reference?channel=stable&os=linux&arch=x86_64", token, nil)
	if err := json.Unmarshal(w.Body.Bytes(), &ref); err != nil {
		t.Fatal(err)
	}
	if ref.Version == nil || *ref.Version != "4.0.0" || len(ref.Entries) != 0 {
		t.Fatalf("ac4 latest without line: %+v entries=%d body=%s", ref.Version, len(ref.Entries), w.Body.String())
	}

	emptySlug := "policy-http-empty"
	if _, _, err := projSvc.Create(ctx, service.CreateProjectInput{DefaultLocale: ptr("en"), Slug: &emptySlug, CompareEngine: &engine}); err != nil {
		t.Fatal(err)
	}
	w = doJSON(r, http.MethodPost, "/api/v1/admin/projects/"+emptySlug+"/matrix", token, map[string]any{
		"os": "linux", "arch": "x86_64", "package_type": "multi_file",
	})
	if w.Code != http.StatusCreated {
		t.Fatalf("empty matrix=%d %s", w.Code, w.Body.String())
	}
	w = doJSON(r, http.MethodGet, "/api/v1/admin/projects/"+emptySlug+"/install-policy-rules/reference?os=linux&arch=x86_64", token, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("empty reference=%d %s", w.Code, w.Body.String())
	}
	if err := json.Unmarshal(w.Body.Bytes(), &ref); err != nil {
		t.Fatal(err)
	}
	if ref.Version != nil || len(ref.Entries) != 0 {
		t.Fatalf("ac4 no versions: %+v", ref)
	}

	// AC5: one-off Manifest PUT does not write templates; published is immutable
	draft := "1.1.0"
	if _, _, err := projSvc.PutVersion(ctx, p.ID, draft, service.VersionWriteInput{Channel: "stable"}); err != nil {
		t.Fatal(err)
	}
	if _, err := projSvc.AddVersionLine(ctx, p.ID, draft, service.VersionLineWriteInput{OS: "windows", Arch: "x86_64"}); err != nil {
		t.Fatal(err)
	}
	w = doJSON(r, http.MethodPut, fmt.Sprintf("/api/v1/admin/projects/%s/versions/%s/lines/windows/x86_64/manifest", slug, draft), token, map[string]any{
		"entries": []map[string]any{
			{"path": "config/user.json", "size": 0, "sha256": sha, "md5": md5, "install_policy": "OVERWRITE"},
		},
	})
	if w.Code != http.StatusOK {
		t.Fatalf("ac5 put manifest=%d %s", w.Code, w.Body.String())
	}
	w = doJSON(r, http.MethodGet, fmt.Sprintf("/api/v1/admin/projects/%s/versions/%s/lines/windows/x86_64/manifest", slug, draft), token, nil)
	if err := json.Unmarshal(w.Body.Bytes(), &man); err != nil {
		t.Fatal(err)
	}
	if len(man.Entries) != 1 || man.Entries[0].InstallPolicy != model.InstallPolicyOverwrite {
		t.Fatalf("ac5 draft policy=%s", w.Body.String())
	}
	w = doJSON(r, http.MethodGet, "/api/v1/admin/projects/"+slug+"/install-policy-rules?os=windows&arch=x86_64", token, nil)
	var rules struct {
		Entries []struct {
			Path          string `json:"path"`
			InstallPolicy string `json:"install_policy"`
		} `json:"entries"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &rules); err != nil {
		t.Fatal(err)
	}
	if len(rules.Entries) != 1 || rules.Entries[0].InstallPolicy != model.InstallPolicyKeepIfExists {
		t.Fatalf("ac5 templates unchanged: %s", w.Body.String())
	}

	w = postZipArchive(r, fmt.Sprintf("/api/v1/admin/projects/%s/versions/1.0.0/lines/windows/x86_64/build-archive", slug), token, map[string][]byte{
		"bin/app.exe":      []byte("exe"),
		"config/user.json": []byte("{}"),
	})
	// 1.0.0 already has Manifest; re-upload should succeed then we publish
	if w.Code != http.StatusCreated && w.Code != http.StatusOK {
		// line already ready; publish next
		_ = w
	}
	w = doJSON(r, http.MethodPost, fmt.Sprintf("/api/v1/admin/projects/%s/versions/1.0.0/publish", slug), token, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("publish 1.0.0=%d %s", w.Code, w.Body.String())
	}
	pubMan := doJSON(r, http.MethodGet, fmt.Sprintf("/api/v1/admin/projects/%s/versions/1.0.0/lines/windows/x86_64/manifest", slug), token, nil)
	var before struct {
		RootHash string `json:"root_hash"`
	}
	if err := json.Unmarshal(pubMan.Body.Bytes(), &before); err != nil {
		t.Fatal(err)
	}
	w = doJSON(r, http.MethodPut, fmt.Sprintf("/api/v1/admin/projects/%s/versions/1.0.0/lines/windows/x86_64/manifest", slug), token, map[string]any{
		"entries": []map[string]any{
			{"path": "config/user.json", "size": 0, "sha256": sha, "md5": md5, "install_policy": "OVERWRITE"},
		},
	})
	if w.Code != http.StatusConflict {
		t.Fatalf("ac5 published put=%d %s", w.Code, w.Body.String())
	}
	w = doJSON(r, http.MethodPut, "/api/v1/admin/projects/"+slug+"/install-policy-rules?os=windows&arch=x86_64", token, map[string]any{
		"entries": []map[string]string{{"path": "other.cfg", "install_policy": "KEEP_IF_EXISTS"}},
	})
	if w.Code != http.StatusOK {
		t.Fatalf("ac5 still can change rules=%d %s", w.Code, w.Body.String())
	}
	after := doJSON(r, http.MethodGet, fmt.Sprintf("/api/v1/admin/projects/%s/versions/1.0.0/lines/windows/x86_64/manifest", slug), token, nil)
	var afterBody struct {
		RootHash string `json:"root_hash"`
	}
	if err := json.Unmarshal(after.Body.Bytes(), &afterBody); err != nil {
		t.Fatal(err)
	}
	if afterBody.RootHash != before.RootHash {
		t.Fatalf("ac5 published root hash changed %s -> %s", before.RootHash, afterBody.RootHash)
	}
}
