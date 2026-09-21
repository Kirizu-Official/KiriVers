package admin

import (
	"encoding/json"
	"net/http"
	"sync"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/Kirizu-Official/KiriVers/internal/model"
	"github.com/Kirizu-Official/KiriVers/internal/service"
)

func ptr[T any](v T) *T {
	return &v
}

func TestVersionHTTPExistsAlways200(t *testing.T) {
	r, projSvc, _, token := setupProjectHTTP(t)
	ctx := t.Context()
	p, _, err := projSvc.Create(ctx, service.CreateProjectInput{DefaultLocale: ptr("en"), Slug: ptr("exists-proj"),
		CompareEngine: ptr(model.CompareEngineSemver),
	})
	if err != nil {
		t.Fatal(err)
	}

	// 1. Check non-existent version -> 200 {"exists": false}, NOT 404
	w := doJSON(r, http.MethodGet, "/api/v1/admin/projects/"+p.Slug+"/versions/1.0.0/exists", token, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var res map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &res); err != nil {
		t.Fatal(err)
	}
	if res["exists"] != false {
		t.Fatalf("expected exists=false, got %v", res["exists"])
	}

	// 2. Detail GET for non-existent version returns 404 VERSION_NOT_FOUND
	w404 := doJSON(r, http.MethodGet, "/api/v1/admin/projects/"+p.Slug+"/versions/1.0.0", token, nil)
	if w404.Code != http.StatusNotFound || decodeErr(t, w404) != "VERSION_NOT_FOUND" {
		t.Fatalf("expected 404 VERSION_NOT_FOUND, got %d %s", w404.Code, decodeErr(t, w404))
	}

	// 3. Put 1.0.0
	wPut := doJSON(r, http.MethodPut, "/api/v1/admin/projects/"+p.Slug+"/versions/1.0.0", token, gin.H{
		"channel":   "stable",
		"changelog": "Initial version",
	})
	if wPut.Code != http.StatusCreated {
		t.Fatalf("put failed: %d %s", wPut.Code, wPut.Body.String())
	}

	// 4. Check exists again -> 200 {"exists": true, "status": "draft", "channel": "stable", ...}
	wExists := doJSON(r, http.MethodGet, "/api/v1/admin/projects/"+p.Slug+"/versions/1.0.0/exists", token, nil)
	if wExists.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", wExists.Code)
	}
	res = nil
	if err := json.Unmarshal(wExists.Body.Bytes(), &res); err != nil {
		t.Fatal(err)
	}
	if res["exists"] != true || res["status"] != "draft" || res["channel"] != "stable" {
		t.Fatalf("unexpected exists response: %+v", res)
	}
	// Assert no display_version
	if _, ok := res["display_version"]; ok {
		t.Fatal("display_version must not appear in exists response")
	}

	// 5. Querying 1.0.0+build233 also returns exists=true
	wBuild := doJSON(r, http.MethodGet, "/api/v1/admin/projects/"+p.Slug+"/versions/1.0.0+build233/exists", token, nil)
	if wBuild.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", wBuild.Code)
	}
	res = nil
	_ = json.Unmarshal(wBuild.Body.Bytes(), &res)
	if res["exists"] != true {
		t.Fatalf("1.0.0+build233 exists=%v, want true", res["exists"])
	}
}

func TestVersionHTTPPutAndRules(t *testing.T) {
	r, projSvc, _, token := setupProjectHTTP(t)
	ctx := t.Context()
	p, _, err := projSvc.Create(ctx, service.CreateProjectInput{DefaultLocale: ptr("en"), Slug: ptr("rules-proj"),
		CompareEngine: ptr(model.CompareEngineSemver),
	})
	if err != nil {
		t.Fatal(err)
	}

	// 1. Channel suffix mismatch: 1.2.3-rc.1 on beta -> 400 CHANNEL_SUFFIX_MISMATCH
	wMismatch := doJSON(r, http.MethodPut, "/api/v1/admin/projects/"+p.Slug+"/versions/1.2.3-rc.1", token, gin.H{
		"channel": "beta",
	})
	if wMismatch.Code != http.StatusBadRequest || decodeErr(t, wMismatch) != "CHANNEL_SUFFIX_MISMATCH" {
		t.Fatalf("expected 400 CHANNEL_SUFFIX_MISMATCH, got %d %s", wMismatch.Code, decodeErr(t, wMismatch))
	}

	// 2. Stable channel forbids prerelease: 1.2.3-beta.1 on stable -> 400 CHANNEL_SUFFIX_MISMATCH
	wStableMismatch := doJSON(r, http.MethodPut, "/api/v1/admin/projects/"+p.Slug+"/versions/1.2.3-beta.1", token, gin.H{
		"channel": "stable",
	})
	if wStableMismatch.Code != http.StatusBadRequest || decodeErr(t, wStableMismatch) != "CHANNEL_SUFFIX_MISMATCH" {
		t.Fatalf("expected 400 CHANNEL_SUFFIX_MISMATCH, got %d %s", wStableMismatch.Code, decodeErr(t, wStableMismatch))
	}

	// 3. Illegal SemVer: 1.2 or 1.2.3.4 under semver engine -> 400 ENGINE_MISMATCH
	wIllegal := doJSON(r, http.MethodPut, "/api/v1/admin/projects/"+p.Slug+"/versions/1.2", token, gin.H{
		"channel": "stable",
	})
	if wIllegal.Code != http.StatusBadRequest || decodeErr(t, wIllegal) != "ENGINE_MISMATCH" {
		t.Fatalf("expected 400 ENGINE_MISMATCH, got %d %s", wIllegal.Code, decodeErr(t, wIllegal))
	}
	wIllegal2 := doJSON(r, http.MethodPut, "/api/v1/admin/projects/"+p.Slug+"/versions/1.2.3.4", token, gin.H{
		"channel": "stable",
	})
	if wIllegal2.Code != http.StatusBadRequest || decodeErr(t, wIllegal2) != "ENGINE_MISMATCH" {
		t.Fatalf("expected 400 ENGINE_MISMATCH, got %d %s", wIllegal2.Code, decodeErr(t, wIllegal2))
	}

	// 4. Critical with gray start != 100 -> 400 GRAY_NOT_ALLOWED_ON_CRITICAL
	wCrit := doJSON(r, http.MethodPut, "/api/v1/admin/projects/"+p.Slug+"/versions/1.0.0", token, gin.H{
		"channel":            "stable",
		"is_critical":        true,
		"gray_start_percent": 50,
	})
	if wCrit.Code != http.StatusBadRequest || decodeErr(t, wCrit) != "GRAY_NOT_ALLOWED_ON_CRITICAL" {
		t.Fatalf("expected 400 GRAY_NOT_ALLOWED_ON_CRITICAL, got %d %s", wCrit.Code, decodeErr(t, wCrit))
	}

	// 5. Create Draft 1.0.0 on stable -> 201 Created
	wCreate := doJSON(r, http.MethodPut, "/api/v1/admin/projects/"+p.Slug+"/versions/1.0.0", token, gin.H{
		"channel": "stable",
	})
	if wCreate.Code != http.StatusCreated {
		t.Fatalf("create failed: %d %s", wCreate.Code, wCreate.Body.String())
	}
	var createdV map[string]any
	_ = json.Unmarshal(wCreate.Body.Bytes(), &createdV)
	if createdV["gray_start_percent"] != float64(30) {
		t.Fatalf("expected gray_start_percent=30, got %v", createdV["gray_start_percent"])
	}
	if createdV["gray_step_percent"] != float64(10) {
		t.Fatalf("expected gray_step_percent=10, got %v", createdV["gray_step_percent"])
	}
	if _, hasDisplay := createdV["display_version"]; hasDisplay {
		t.Fatal("display_version must not appear in Version JSON")
	}

	// 6. Idempotent PUT 1.0.0+20260912 -> 200 OK
	wIdemp := doJSON(r, http.MethodPut, "/api/v1/admin/projects/"+p.Slug+"/versions/1.0.0+20260912", token, gin.H{
		"channel": "stable",
	})
	if wIdemp.Code != http.StatusOK {
		t.Fatalf("idempotent put failed: %d %s", wIdemp.Code, wIdemp.Body.String())
	}

	// 7. Same version on different channel -> 409 CHANNEL_CONFLICT
	wConflict := doJSON(r, http.MethodPut, "/api/v1/admin/projects/"+p.Slug+"/versions/1.0.0", token, gin.H{
		"channel": "beta",
	})
	if wConflict.Code != http.StatusConflict || decodeErr(t, wConflict) != "CHANNEL_CONFLICT" {
		t.Fatalf("expected 409 CHANNEL_CONFLICT, got %d %s", wConflict.Code, decodeErr(t, wConflict))
	}
}

func TestVersionHTTPInteger010And10(t *testing.T) {
	r, projSvc, _, token := setupProjectHTTP(t)
	ctx := t.Context()
	p, _, err := projSvc.Create(ctx, service.CreateProjectInput{DefaultLocale: ptr("en"), Slug: ptr("int-proj"),
		CompareEngine: ptr(model.CompareEngineInteger),
	})
	if err != nil {
		t.Fatal(err)
	}

	// 1. Put version 010 -> 201 Created with version_integer = 10
	wPut := doJSON(r, http.MethodPut, "/api/v1/admin/projects/"+p.Slug+"/versions/010", token, gin.H{
		"channel": "stable",
	})
	if wPut.Code != http.StatusCreated {
		t.Fatalf("put 010 failed: %d %s", wPut.Code, wPut.Body.String())
	}
	var res map[string]any
	_ = json.Unmarshal(wPut.Body.Bytes(), &res)
	if vi, ok := res["version_integer"].(float64); !ok || int64(vi) != 10 {
		t.Fatalf("version_integer=%v, want 10", res["version_integer"])
	}

	// 2. Put version 10 -> 200 OK idempotent (hits the same version)
	wPut2 := doJSON(r, http.MethodPut, "/api/v1/admin/projects/"+p.Slug+"/versions/10", token, gin.H{
		"channel": "stable",
	})
	if wPut2.Code != http.StatusOK {
		t.Fatalf("put 10 failed: %d %s", wPut2.Code, wPut2.Body.String())
	}

	// 3. GET /exists with 10 and 010 both hit the same version
	wEx1 := doJSON(r, http.MethodGet, "/api/v1/admin/projects/"+p.Slug+"/versions/10/exists", token, nil)
	wEx2 := doJSON(r, http.MethodGet, "/api/v1/admin/projects/"+p.Slug+"/versions/010/exists", token, nil)
	if wEx1.Code != http.StatusOK || wEx2.Code != http.StatusOK {
		t.Fatalf("exists failed: %d %d", wEx1.Code, wEx2.Code)
	}
	var res1, res2 map[string]any
	_ = json.Unmarshal(wEx1.Body.Bytes(), &res1)
	_ = json.Unmarshal(wEx2.Body.Bytes(), &res2)
	if res1["exists"] != true || res2["exists"] != true {
		t.Fatalf("exists=%v %v, want true", res1["exists"], res2["exists"])
	}
}

func TestVersionHTTPPublishGatesAndLifecycle(t *testing.T) {
	r, projSvc, _, token := setupProjectHTTP(t)
	ctx := t.Context()
	p, _, err := projSvc.Create(ctx, service.CreateProjectInput{DefaultLocale: ptr("en"), Slug: ptr("gate-proj"),
		CompareEngine: ptr(model.CompareEngineSemver),
	})
	if err != nil {
		t.Fatal(err)
	}

	// Create 1.0.0
	wPut := doJSON(r, http.MethodPut, "/api/v1/admin/projects/"+p.Slug+"/versions/1.0.0", token, gin.H{
		"channel": "stable",
	})
	if wPut.Code != http.StatusCreated {
		t.Fatalf("create 1.0.0 failed: %d %s", wPut.Code, wPut.Body.String())
	}

	// 1. Publish with 0 lines -> 409 ARTIFACT_REQUIRED
	wPub1 := doJSON(r, http.MethodPost, "/api/v1/admin/projects/"+p.Slug+"/versions/1.0.0/publish", token, nil)
	if wPub1.Code != http.StatusConflict || decodeErr(t, wPub1) != "ARTIFACT_REQUIRED" {
		t.Fatalf("expected 409 ARTIFACT_REQUIRED, got %d %s", wPub1.Code, decodeErr(t, wPub1))
	}

	// 2. Add line for windows/x86_64
	wLine := doJSON(r, http.MethodPost, "/api/v1/admin/projects/"+p.Slug+"/versions/1.0.0/lines", token, gin.H{
		"os":   "windows",
		"arch": "x86_64",
	})
	if wLine.Code != http.StatusCreated {
		t.Fatalf("create line failed: %d %s", wLine.Code, wLine.Body.String())
	}

	// 3. Still pending line -> 409 ARTIFACT_REQUIRED
	wPub2 := doJSON(r, http.MethodPost, "/api/v1/admin/projects/"+p.Slug+"/versions/1.0.0/publish", token, nil)
	if wPub2.Code != http.StatusConflict || decodeErr(t, wPub2) != "ARTIFACT_REQUIRED" {
		t.Fatalf("expected 409 ARTIFACT_REQUIRED, got %d %s", wPub2.Code, decodeErr(t, wPub2))
	}

	// 4. Mark ready via fixture
	wReady := doJSON(r, http.MethodPost, "/api/v1/admin/projects/"+p.Slug+"/versions/1.0.0/lines/windows/x86_64/ready", token, nil)
	if wReady.Code != http.StatusOK {
		t.Fatalf("ready line failed: %d %s", wReady.Code, wReady.Body.String())
	}

	// 5. Publish now succeeds!
	wPub3 := doJSON(r, http.MethodPost, "/api/v1/admin/projects/"+p.Slug+"/versions/1.0.0/publish", token, nil)
	if wPub3.Code != http.StatusOK {
		t.Fatalf("publish failed: %d %s", wPub3.Code, wPub3.Body.String())
	}

	// 6. Published version allows adding another line without new version number
	wLine2 := doJSON(r, http.MethodPost, "/api/v1/admin/projects/"+p.Slug+"/versions/1.0.0/lines", token, gin.H{
		"os":   "linux",
		"arch": "x86_64",
	})
	if wLine2.Code != http.StatusCreated {
		t.Fatalf("add line after publish failed: %d %s", wLine2.Code, wLine2.Body.String())
	}

	// 7. Published version can PATCH is_lts
	wPatchLTS := doJSON(r, http.MethodPatch, "/api/v1/admin/projects/"+p.Slug+"/versions/1.0.0", token, gin.H{
		"is_lts": true,
	})
	if wPatchLTS.Code != http.StatusOK {
		t.Fatalf("patch is_lts failed: %d %s", wPatchLTS.Code, wPatchLTS.Body.String())
	}
	var resLTS map[string]any
	_ = json.Unmarshal(wPatchLTS.Body.Bytes(), &resLTS)
	if resLTS["is_lts"] != true {
		t.Fatalf("is_lts=%v, want true", resLTS["is_lts"])
	}

	// 8. Published version CANNOT modify version numbers or changelog
	wPatchNum := doJSON(r, http.MethodPatch, "/api/v1/admin/projects/"+p.Slug+"/versions/1.0.0", token, gin.H{
		"version_integer": 99,
	})
	if wPatchNum.Code != http.StatusConflict {
		t.Fatalf("expected 409 conflict when patching number on published version, got %d", wPatchNum.Code)
	}

	// 9. Yank windows/x86_64 line
	wYank := doJSON(r, http.MethodPost, "/api/v1/admin/projects/"+p.Slug+"/versions/1.0.0/lines/windows/x86_64/yank", token, nil)
	if wYank.Code != http.StatusOK {
		t.Fatalf("yank line failed: %d %s", wYank.Code, wYank.Body.String())
	}
	var resYank map[string]any
	_ = json.Unmarshal(wYank.Body.Bytes(), &resYank)
	if resYank["status"] != "yanked" {
		t.Fatalf("yank status=%v, want yanked", resYank["status"])
	}

	// Linux line remains pending
	wLinuxLine := doJSON(r, http.MethodGet, "/api/v1/admin/projects/"+p.Slug+"/versions/1.0.0/lines/linux/x86_64", token, nil)
	if wLinuxLine.Code != http.StatusOK {
		t.Fatalf("get linux line failed: %d", wLinuxLine.Code)
	}
	var resLinux map[string]any
	_ = json.Unmarshal(wLinuxLine.Body.Bytes(), &resLinux)
	if resLinux["status"] != "pending" {
		t.Fatalf("linux line status=%v, want pending", resLinux["status"])
	}

	// 10. Deprecate and Revoke
	wDep := doJSON(r, http.MethodPost, "/api/v1/admin/projects/"+p.Slug+"/versions/1.0.0/deprecate", token, nil)
	if wDep.Code != http.StatusOK {
		t.Fatalf("deprecate failed: %d %s", wDep.Code, wDep.Body.String())
	}
	wRev := doJSON(r, http.MethodPost, "/api/v1/admin/projects/"+p.Slug+"/versions/1.0.0/revoke", token, nil)
	if wRev.Code != http.StatusOK {
		t.Fatalf("revoke failed: %d %s", wRev.Code, wRev.Body.String())
	}
	// Revoked cannot be published again
	wPubAgain := doJSON(r, http.MethodPost, "/api/v1/admin/projects/"+p.Slug+"/versions/1.0.0/publish", token, nil)
	if wPubAgain.Code != http.StatusConflict {
		t.Fatalf("expected 409 conflict when publishing revoked version, got %d", wPubAgain.Code)
	}
}

func TestVersionHTTPConcurrentPut(t *testing.T) {
	r, projSvc, _, token := setupProjectHTTP(t)
	ctx := t.Context()
	p, _, err := projSvc.Create(ctx, service.CreateProjectInput{DefaultLocale: ptr("en"), Slug: ptr("concurrent-proj"),
		CompareEngine: ptr(model.CompareEngineSemver),
	})
	if err != nil {
		t.Fatal(err)
	}

	var wg sync.WaitGroup
	codes := make([]int, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			w := doJSON(r, http.MethodPut, "/api/v1/admin/projects/"+p.Slug+"/versions/2.0.0", token, gin.H{
				"channel": "stable",
			})
			codes[idx] = w.Code
		}(i)
	}
	wg.Wait()

	// Exactly one 201 Created and one 200 OK
	has201 := (codes[0] == http.StatusCreated || codes[1] == http.StatusCreated)
	has200 := (codes[0] == http.StatusOK || codes[1] == http.StatusOK)
	if !has201 || !has200 {
		t.Fatalf("expected one 201 and one 200, got %d and %d", codes[0], codes[1])
	}
}

func putReadyPublished(t *testing.T, r *gin.Engine, slug, token, version, os, arch string) {
	t.Helper()
	w := doJSON(r, http.MethodPut, "/api/v1/admin/projects/"+slug+"/versions/"+version, token, gin.H{
		"channel": "stable",
	})
	if w.Code != http.StatusCreated && w.Code != http.StatusOK {
		t.Fatalf("put %s: %d %s", version, w.Code, w.Body.String())
	}
	w = doJSON(r, http.MethodPost, "/api/v1/admin/projects/"+slug+"/versions/"+version+"/lines", token, gin.H{
		"os": os, "arch": arch,
	})
	if w.Code != http.StatusCreated && w.Code != http.StatusConflict {
		t.Fatalf("line %s: %d %s", version, w.Code, w.Body.String())
	}
	w = doJSON(r, http.MethodPost, "/api/v1/admin/projects/"+slug+"/versions/"+version+"/lines/"+os+"/"+arch+"/ready", token, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("ready %s: %d %s", version, w.Code, w.Body.String())
	}
	w = doJSON(r, http.MethodPost, "/api/v1/admin/projects/"+slug+"/versions/"+version+"/publish", token, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("publish %s: %d %s", version, w.Code, w.Body.String())
	}
}

func TestListVersionsPlatformFilterLatestAndUnfiltered(t *testing.T) {
	r, projSvc, _, token := setupProjectHTTP(t)
	ctx := t.Context()
	p, _, err := projSvc.Create(ctx, service.CreateProjectInput{
		DefaultLocale: ptr("en"),
		Slug:          ptr("list-plat-proj"),
		CompareEngine: ptr(model.CompareEngineSemver),
	})
	if err != nil {
		t.Fatal(err)
	}
	slug := p.Slug
	base := "/api/v1/admin/projects/" + slug + "/versions"

	// 创建顺序刻意不是 semver 序：1.0.0 → 1.2.0 → 1.1.0，latest 必须按 CompareVersions。
	putReadyPublished(t, r, slug, token, "1.0.0", "linux", "x86_64")
	putReadyPublished(t, r, slug, token, "1.2.0", "linux", "x86_64")
	putReadyPublished(t, r, slug, token, "1.1.0", "linux", "x86_64")
	putReadyPublished(t, r, slug, token, "2.0.0", "windows", "x86_64")

	// draft + ready linux 线不得进入过滤列表
	wDraft := doJSON(r, http.MethodPut, base+"/0.9.0", token, gin.H{"channel": "stable"})
	if wDraft.Code != http.StatusCreated {
		t.Fatalf("draft put: %d %s", wDraft.Code, wDraft.Body.String())
	}
	wLine := doJSON(r, http.MethodPost, base+"/0.9.0/lines", token, gin.H{"os": "linux", "arch": "x86_64"})
	if wLine.Code != http.StatusCreated {
		t.Fatalf("draft line: %d %s", wLine.Code, wLine.Body.String())
	}
	wReady := doJSON(r, http.MethodPost, base+"/0.9.0/lines/linux/x86_64/ready", token, nil)
	if wReady.Code != http.StatusOK {
		t.Fatalf("draft ready: %d %s", wReady.Code, wReady.Body.String())
	}

	// published 但 linux 线未 ready
	wPend := doJSON(r, http.MethodPut, base+"/1.5.0", token, gin.H{"channel": "stable"})
	if wPend.Code != http.StatusCreated {
		t.Fatalf("pending put: %d %s", wPend.Code, wPend.Body.String())
	}
	wPendLine := doJSON(r, http.MethodPost, base+"/1.5.0/lines", token, gin.H{"os": "linux", "arch": "x86_64"})
	if wPendLine.Code != http.StatusCreated {
		t.Fatalf("pending line: %d %s", wPendLine.Code, wPendLine.Body.String())
	}
	putReadyPublished(t, r, slug, token, "1.5.0", "windows", "arm64")

	w := doJSON(r, http.MethodGet, base+"?os=linux&arch=x86_64", token, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("filtered list: %d %s", w.Code, w.Body.String())
	}
	var filtered map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &filtered); err != nil {
		t.Fatal(err)
	}
	if _, ok := filtered["latest"]; ok {
		t.Fatalf("os+arch filters must omit latest: %s", w.Body.String())
	}
	vers, _ := filtered["versions"].([]any)
	got := map[string]bool{}
	for _, item := range vers {
		row, _ := item.(map[string]any)
		semver, _ := row["version_semver"].(string)
		got[semver] = true
		if _, ok := row["lines"]; ok {
			t.Fatalf("filtered list must omit full line payloads: %s", w.Body.String())
		}
	}
	if !got["1.0.0"] || !got["1.1.0"] || !got["1.2.0"] || !got["0.9.0"] || !got["1.5.0"] {
		t.Fatalf("expected versions that have linux/x86_64 line (any status), got %v", got)
	}
	if got["2.0.0"] {
		t.Fatalf("filter leaked windows-only version: %v", got)
	}

	wLatest := doJSON(r, http.MethodGet, base+"?latest=true&os=linux&arch=x86_64", token, nil)
	if wLatest.Code != http.StatusOK {
		t.Fatalf("latest list: %d %s", wLatest.Code, wLatest.Body.String())
	}
	var latestBody map[string]any
	if err := json.Unmarshal(wLatest.Body.Bytes(), &latestBody); err != nil {
		t.Fatal(err)
	}
	latest, _ := latestBody["latest"].(map[string]any)
	if latest == nil || latest["version"] != "1.2.0" || latest["channel"] != "stable" || latest["status"] != "published" {
		t.Fatalf("latest=%v body=%s", latestBody["latest"], wLatest.Body.String())
	}

	// 未知 pair → 200 空列表，不含 latest
	wEmpty := doJSON(r, http.MethodGet, base+"?os=unknown-os&arch=unknown-arch", token, nil)
	if wEmpty.Code != http.StatusOK {
		t.Fatalf("empty pair: %d %s", wEmpty.Code, wEmpty.Body.String())
	}
	var emptyBody map[string]any
	if err := json.Unmarshal(wEmpty.Body.Bytes(), &emptyBody); err != nil {
		t.Fatal(err)
	}
	emptyVers, _ := emptyBody["versions"].([]any)
	if len(emptyVers) != 0 {
		t.Fatalf("unknown pair versions=%v", emptyVers)
	}
	if _, ok := emptyBody["latest"]; ok {
		t.Fatalf("unknown pair must omit latest: %s", wEmpty.Body.String())
	}

	// 未带 os/arch：今天的全量列表，不含 latest；versions/index unwrap 依赖此形状
	wAll := doJSON(r, http.MethodGet, base, token, nil)
	if wAll.Code != http.StatusOK {
		t.Fatalf("unfiltered: %d %s", wAll.Code, wAll.Body.String())
	}
	var allBody map[string]any
	if err := json.Unmarshal(wAll.Body.Bytes(), &allBody); err != nil {
		t.Fatal(err)
	}
	if _, ok := allBody["latest"]; ok {
		t.Fatalf("unfiltered list must omit latest: %s", wAll.Body.String())
	}
	allVers, _ := allBody["versions"].([]any)
	if len(allVers) < 6 {
		t.Fatalf("unfiltered must keep drafts and other platforms, got %d", len(allVers))
	}

	// 只带 os 或只带 arch：仍走全量列表，不含 latest（过滤必须成对）
	wOnlyOS := doJSON(r, http.MethodGet, base+"?os=linux", token, nil)
	if wOnlyOS.Code != http.StatusOK {
		t.Fatalf("os-only: %d %s", wOnlyOS.Code, wOnlyOS.Body.String())
	}
	var onlyOSBody map[string]any
	if err := json.Unmarshal(wOnlyOS.Body.Bytes(), &onlyOSBody); err != nil {
		t.Fatal(err)
	}
	if _, ok := onlyOSBody["latest"]; ok {
		t.Fatalf("os-only list must omit latest: %s", wOnlyOS.Body.String())
	}
	onlyOSVers, _ := onlyOSBody["versions"].([]any)
	if len(onlyOSVers) != len(allVers) {
		t.Fatalf("os-only must be unfiltered, got %d want %d", len(onlyOSVers), len(allVers))
	}

	// 别名 os/arch 规范化后命中 macos/x86_64 线
	putReadyPublished(t, r, slug, token, "3.0.0", "macos", "x86_64")
	wAlias := doJSON(r, http.MethodGet, base+"?os=darwin&arch=amd64", token, nil)
	if wAlias.Code != http.StatusOK {
		t.Fatalf("alias filter: %d %s", wAlias.Code, wAlias.Body.String())
	}
	var aliasBody map[string]any
	_ = json.Unmarshal(wAlias.Body.Bytes(), &aliasBody)
	if _, ok := aliasBody["latest"]; ok {
		t.Fatalf("alias filter must omit latest: %s", wAlias.Body.String())
	}
	aliasVers, _ := aliasBody["versions"].([]any)
	found30 := false
	for _, item := range aliasVers {
		row, _ := item.(map[string]any)
		if row["version_semver"] == "3.0.0" {
			found30 = true
		}
	}
	if !found30 {
		t.Fatalf("alias filter missing 3.0.0: %s", wAlias.Body.String())
	}
}

func TestLineDefaultsHTTP(t *testing.T) {
	r, projSvc, _, token := setupProjectHTTP(t)
	ctx := t.Context()
	p, _, err := projSvc.Create(ctx, service.CreateProjectInput{
		DefaultLocale: ptr("en"),
		Slug:          ptr("line-def-proj"),
		CompareEngine: ptr(model.CompareEngineSemver),
	})
	if err != nil {
		t.Fatal(err)
	}
	slug := p.Slug
	w := doJSON(r, http.MethodPost, "/api/v1/admin/projects/"+slug+"/matrix", token, gin.H{
		"os": "windows", "arch": "x86_64", "package_type": "single_file",
	})
	if w.Code != http.StatusCreated {
		t.Fatalf("matrix: %d %s", w.Code, w.Body.String())
	}
	base := "/api/v1/admin/projects/" + slug + "/versions"
	for _, ver := range []string{"1.0.0", "2.0.0"} {
		w = doJSON(r, http.MethodPut, base+"/"+ver, token, gin.H{"channel": "stable"})
		if w.Code != http.StatusCreated {
			t.Fatalf("put %s: %d %s", ver, w.Code, w.Body.String())
		}
	}
	w = doJSON(r, http.MethodPost, base+"/1.0.0/lines", token, gin.H{
		"os": "windows", "arch": "x86_64", "min_os": "10.0", "min_api_level": 24,
	})
	if w.Code != http.StatusCreated {
		t.Fatalf("line 1.0.0: %d %s", w.Code, w.Body.String())
	}
	w = doJSON(r, http.MethodPost, base+"/2.0.0/lines", token, gin.H{
		"os": "windows", "arch": "x86_64",
	})
	if w.Code != http.StatusCreated {
		t.Fatalf("line 2.0.0: %d %s", w.Code, w.Body.String())
	}
	w = doJSON(r, http.MethodGet, base+"/2.0.0/line-defaults?os=windows&arch=x86_64", token, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("line-defaults: %d %s", w.Code, w.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body["min_os"] != "10.0" {
		t.Fatalf("min_os=%v want 10.0", body["min_os"])
	}
	if api, ok := body["min_api_level"].(float64); !ok || int(api) != 24 {
		t.Fatalf("min_api_level=%v want 24", body["min_api_level"])
	}
}
