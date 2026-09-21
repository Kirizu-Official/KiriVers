package admin

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/Kirizu-Official/KiriVers/internal/model"
	"github.com/Kirizu-Official/KiriVers/internal/repository"
	"github.com/Kirizu-Official/KiriVers/internal/service"
	"github.com/Kirizu-Official/KiriVers/pkg/hashutil"
)

func TestArtifactHTTP_DirectUpload(t *testing.T) {
	r, projSvc, _, token := setupProjectHTTP(t)
	ctx := t.Context()

	slug := "art-proj"
	p, _, err := projSvc.Create(ctx, service.CreateProjectInput{DefaultLocale: ptr("en"), Slug:          &slug,
		CompareEngine: ptr(model.CompareEngineSemver),
	})
	if err != nil {
		t.Fatal(err)
	}

	_, _, err = projSvc.PutVersion(ctx, p.ID, "1.0.0", service.VersionWriteInput{
		Channel: "stable",
	})
	if err != nil {
		t.Fatal(err)
	}

	payload := []byte("binary payload for testing artifact upload")
	actualSha, _ := hashutil.SHA256Hex(bytes.NewReader(payload))

	// 1. 哈希不匹配 -> 400 CHECKSUM_MISMATCH
	uploadPath := fmt.Sprintf("/api/v1/admin/projects/%s/versions/1.0.0/lines/windows/x86_64/artifacts", p.Slug)
	reqMismatch := httptest.NewRequest(http.MethodPut, uploadPath, bytes.NewReader(payload))
	reqMismatch.Header.Set("Authorization", "Bearer "+token)
	reqMismatch.Header.Set("X-Filename", "tool.exe")
	reqMismatch.Header.Set("X-Content-SHA256", "badbadbadbadbadbadbadbadbadbadbadbadbadbadbadbadbadbadbadbadbadb")
	wMismatch := httptest.NewRecorder()
	r.ServeHTTP(wMismatch, reqMismatch)

	if wMismatch.Code != http.StatusBadRequest || decodeErr(t, wMismatch) != "CHECKSUM_MISMATCH" {
		t.Fatalf("expected 400 CHECKSUM_MISMATCH, got %d %s", wMismatch.Code, wMismatch.Body.String())
	}

	// 2. 正常上传 -> 200 OK
	reqOK := httptest.NewRequest(http.MethodPut, uploadPath, bytes.NewReader(payload))
	reqOK.Header.Set("Authorization", "Bearer "+token)
	reqOK.Header.Set("X-Filename", "tool.exe")
	reqOK.Header.Set("X-Content-SHA256", actualSha)
	reqOK.Header.Set("Idempotency-Key", "idem-key-1")
	wOK := httptest.NewRecorder()
	r.ServeHTTP(wOK, reqOK)

	if wOK.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d %s", wOK.Code, wOK.Body.String())
	}
	var artRes map[string]any
	if err := json.Unmarshal(wOK.Body.Bytes(), &artRes); err != nil {
		t.Fatal(err)
	}
	if artRes["sha256"] != actualSha {
		t.Fatalf("expected sha256=%s, got %v", actualSha, artRes["sha256"])
	}
	if artRes["file_name"] != "art-proj-1.0.0-windows-x86_64.exe" {
		t.Fatalf("expected filename art-proj-1.0.0-windows-x86_64.exe, got %v", artRes["file_name"])
	}

	// 3. 发布版本 -> 200 OK
	wPub := doJSON(r, http.MethodPost, fmt.Sprintf("/api/v1/admin/projects/%s/versions/1.0.0/publish", p.Slug), token, nil)
	if wPub.Code != http.StatusOK {
		t.Fatalf("publish failed: %d %s", wPub.Code, wPub.Body.String())
	}

	// 4. 已发布版本上传不同字节 -> 409 ARTIFACT_IMMUTABLE
	diffPayload := []byte("different payload tamper")
	diffSha, _ := hashutil.SHA256Hex(bytes.NewReader(diffPayload))
	reqDiff := httptest.NewRequest(http.MethodPut, uploadPath, bytes.NewReader(diffPayload))
	reqDiff.Header.Set("Authorization", "Bearer "+token)
	reqDiff.Header.Set("X-Filename", "tool.exe")
	reqDiff.Header.Set("X-Content-SHA256", diffSha)
	wDiff := httptest.NewRecorder()
	r.ServeHTTP(wDiff, reqDiff)

	if wDiff.Code != http.StatusConflict || decodeErr(t, wDiff) != "ARTIFACT_IMMUTABLE" {
		t.Fatalf("expected 409 ARTIFACT_IMMUTABLE, got %d %s", wDiff.Code, wDiff.Body.String())
	}
}

func TestArtifactHTTP_TusUpload(t *testing.T) {
	r, projSvc, _, token := setupProjectHTTP(t)
	ctx := t.Context()

	slug := "tus-http"
	p, _, err := projSvc.Create(ctx, service.CreateProjectInput{DefaultLocale: ptr("en"), Slug:          &slug,
		CompareEngine: ptr(model.CompareEngineSemver),
	})
	if err != nil {
		t.Fatal(err)
	}

	_, _, err = projSvc.PutVersion(ctx, p.ID, "1.1.0", service.VersionWriteInput{
		Channel: "stable",
	})
	if err != nil {
		t.Fatal(err)
	}

	content := []byte("0123456789abcdefghij") // 20 bytes
	metaFilename := base64.StdEncoding.EncodeToString([]byte("app.zip"))

	// 1. 初始化 TUS 上传会话 -> 201 Created
	initPath := fmt.Sprintf("/api/v1/admin/projects/%s/versions/1.1.0/lines/linux/x86_64/artifacts/tus", p.Slug)
	reqInit := httptest.NewRequest(http.MethodPost, initPath, nil)
	reqInit.Header.Set("Authorization", "Bearer "+token)
	reqInit.Header.Set("Upload-Length", "20")
	reqInit.Header.Set("Upload-Metadata", "filename "+metaFilename)
	reqInit.Header.Set("Tus-Resumable", "1.0.0")
	wInit := httptest.NewRecorder()
	r.ServeHTTP(wInit, reqInit)

	if wInit.Code != http.StatusCreated {
		t.Fatalf("expected 201 Created, got %d %s", wInit.Code, wInit.Body.String())
	}
	location := wInit.Header().Get("Location")
	if location == "" {
		t.Fatal("expected Location header in TUS init response")
	}

	// 2. HEAD 查询 offset -> 200 OK, Upload-Offset: 0
	reqHead := httptest.NewRequest(http.MethodHead, location, nil)
	reqHead.Header.Set("Authorization", "Bearer "+token)
	wHead := httptest.NewRecorder()
	r.ServeHTTP(wHead, reqHead)

	if wHead.Code != http.StatusOK || wHead.Header().Get("Upload-Offset") != "0" {
		t.Fatalf("expected 200 with Upload-Offset: 0, got %d, offset=%s", wHead.Code, wHead.Header().Get("Upload-Offset"))
	}

	// 3. 在 TUS 进行中发布版本 -> 409 UPLOAD_INCOMPLETE
	wPubIncomplete := doJSON(r, http.MethodPost, fmt.Sprintf("/api/v1/admin/projects/%s/versions/1.1.0/publish", p.Slug), token, nil)
	if wPubIncomplete.Code != http.StatusConflict || decodeErr(t, wPubIncomplete) != "UPLOAD_INCOMPLETE" {
		t.Fatalf("expected 409 UPLOAD_INCOMPLETE, got %d %s", wPubIncomplete.Code, wPubIncomplete.Body.String())
	}

	// 4. PATCH 错误 offset -> 409 OFFSET_MISMATCH
	reqBadPatch := httptest.NewRequest(http.MethodPatch, location, bytes.NewReader(content[:10]))
	reqBadPatch.Header.Set("Authorization", "Bearer "+token)
	reqBadPatch.Header.Set("Upload-Offset", "5")
	reqBadPatch.Header.Set("Content-Type", "application/offset+octet-stream")
	reqBadPatch.Header.Set("Tus-Resumable", "1.0.0")
	wBadPatch := httptest.NewRecorder()
	r.ServeHTTP(wBadPatch, reqBadPatch)

	if wBadPatch.Code != http.StatusConflict || decodeErr(t, wBadPatch) != "OFFSET_MISMATCH" {
		t.Fatalf("expected 409 OFFSET_MISMATCH, got %d %s", wBadPatch.Code, wBadPatch.Body.String())
	}

	// 5. PATCH chunk 1 -> 204 No Content, Upload-Offset: 10
	reqChunk1 := httptest.NewRequest(http.MethodPatch, location, bytes.NewReader(content[:10]))
	reqChunk1.Header.Set("Authorization", "Bearer "+token)
	reqChunk1.Header.Set("Upload-Offset", "0")
	reqChunk1.Header.Set("Content-Type", "application/offset+octet-stream")
	reqChunk1.Header.Set("Tus-Resumable", "1.0.0")
	wChunk1 := httptest.NewRecorder()
	r.ServeHTTP(wChunk1, reqChunk1)

	if wChunk1.Code != http.StatusNoContent || wChunk1.Header().Get("Upload-Offset") != "10" {
		t.Fatalf("expected 204 with Upload-Offset: 10, got %d, offset=%s", wChunk1.Code, wChunk1.Header().Get("Upload-Offset"))
	}

	// 6. PATCH chunk 2 (finish) -> 204 No Content, Upload-Offset: 20
	reqChunk2 := httptest.NewRequest(http.MethodPatch, location, bytes.NewReader(content[10:]))
	reqChunk2.Header.Set("Authorization", "Bearer "+token)
	reqChunk2.Header.Set("Upload-Offset", "10")
	reqChunk2.Header.Set("Content-Type", "application/offset+octet-stream")
	reqChunk2.Header.Set("Tus-Resumable", "1.0.0")
	wChunk2 := httptest.NewRecorder()
	r.ServeHTTP(wChunk2, reqChunk2)

	if wChunk2.Code != http.StatusNoContent || wChunk2.Header().Get("Upload-Offset") != "20" {
		t.Fatalf("expected 204 with Upload-Offset: 20, got %d, offset=%s", wChunk2.Code, wChunk2.Header().Get("Upload-Offset"))
	}

	// 7. 完成后发布版本 -> 200 OK
	wPubOK := doJSON(r, http.MethodPost, fmt.Sprintf("/api/v1/admin/projects/%s/versions/1.1.0/publish", p.Slug), token, nil)
	if wPubOK.Code != http.StatusOK {
		t.Fatalf("publish should succeed after TUS complete, got %d %s", wPubOK.Code, wPubOK.Body.String())
	}
}

func TestArtifactHTTP_PresignAndCleanup(t *testing.T) {
	r, projSvc, _, token := setupProjectHTTP(t)
	ctx := t.Context()

	slug := "presign-clean"
	p, _, _ := projSvc.Create(ctx, service.CreateProjectInput{DefaultLocale: ptr("en"), Slug:          &slug,
		CompareEngine: ptr(model.CompareEngineSemver),
	})
	_, _, _ = projSvc.PutVersion(ctx, p.ID, "1.0.0", service.VersionWriteInput{Channel: "stable"})

	// Presign
	presignPath := fmt.Sprintf("/api/v1/admin/projects/%s/versions/1.0.0/lines/linux/x86_64/artifacts/presign", p.Slug)
	wPresign := doJSON(r, http.MethodPost, presignPath, token, gin.H{
		"filename": "bundle.zip",
	})
	if wPresign.Code != http.StatusOK {
		t.Fatalf("presign failed: %d %s", wPresign.Code, wPresign.Body.String())
	}
	var presignRes map[string]any
	_ = json.Unmarshal(wPresign.Body.Bytes(), &presignRes)
	if presignRes["upload_url"] == "" {
		t.Fatal("expected upload_url in presign response")
	}

	// Cleanup
	cleanupPath := fmt.Sprintf("/api/v1/admin/projects/%s/artifacts/cleanup?retention_days=1", p.Slug)
	wClean := doJSON(r, http.MethodPost, cleanupPath, token, nil)
	if wClean.Code != http.StatusOK {
		t.Fatalf("cleanup failed: %d %s", wClean.Code, wClean.Body.String())
	}
	var cleanRes map[string]any
	_ = json.Unmarshal(wClean.Body.Bytes(), &cleanRes)
	if cleanRes["cleaned_count"] == nil {
		t.Fatal("expected cleaned_count in cleanup response")
	}
}

// TestArtifactHTTP_DeltaJob 覆盖差量生成端点（C10-7 / §7.2 / design §4）：
// 未知算法 → 400 DELTA_ALGO_UNSUPPORTED；source==target → 400 DELTA_SAME_VERSION；
// 合法请求 → 202 + job_id（delta_generate 任务可执行成功）。
func TestArtifactHTTP_DeltaJob(t *testing.T) {
	r, projSvc, _, token := setupProjectHTTP(t)
	ctx := t.Context()
	projSvc.SetJobStore(repository.NewMemoryJobRepo())

	slug := "delta-http"
	p, _, err := projSvc.Create(ctx, service.CreateProjectInput{DefaultLocale: ptr("en"), Slug:          &slug,
		CompareEngine: ptr(model.CompareEngineSemver),
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, ver := range []string{"1.0.0", "2.0.0"} {
		if _, _, err := projSvc.PutVersion(ctx, p.ID, ver, service.VersionWriteInput{Channel: "stable"}); err != nil {
			t.Fatal(err)
		}
		content := fmt.Sprintf("delta-http fixture %s %s", ver, strings.Repeat("x", 1024))
		sha, _ := hashutil.SHA256Hex(strings.NewReader(content))
		uploadPath := fmt.Sprintf("/api/v1/admin/projects/%s/versions/%s/lines/windows/x86_64/artifacts", p.Slug, ver)
		req := httptest.NewRequest(http.MethodPut, uploadPath, strings.NewReader(content))
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("X-Filename", "tool.exe")
		req.Header.Set("X-Content-SHA256", sha)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("upload %s failed: %d %s", ver, w.Code, w.Body.String())
		}
		wPub := doJSON(r, http.MethodPost, fmt.Sprintf("/api/v1/admin/projects/%s/versions/%s/publish", p.Slug, ver), token, nil)
		if wPub.Code != http.StatusOK {
			t.Fatalf("publish %s failed: %d %s", ver, wPub.Code, wPub.Body.String())
		}
	}

	deltaPath := fmt.Sprintf("/api/v1/admin/projects/%s/versions/2.0.0/artifacts/delta", p.Slug)

	// 未知算法 → 400 DELTA_ALGO_UNSUPPORTED（验收项）。
	wBad := doJSON(r, http.MethodPost, deltaPath, token, map[string]string{
		"source_version": "1.0.0", "os": "windows", "arch": "x86_64", "algo": "zstd-dict",
	})
	if wBad.Code != http.StatusBadRequest || decodeErr(t, wBad) != "DELTA_ALGO_UNSUPPORTED" {
		t.Fatalf("expected 400 DELTA_ALGO_UNSUPPORTED, got %d %s", wBad.Code, wBad.Body.String())
	}

	// source == target → 400。
	wSame := doJSON(r, http.MethodPost, deltaPath, token, map[string]string{
		"source_version": "2.0.0", "os": "windows", "arch": "x86_64",
	})
	if wSame.Code != http.StatusBadRequest || decodeErr(t, wSame) != "DELTA_SAME_VERSION" {
		t.Fatalf("expected 400 DELTA_SAME_VERSION, got %d %s", wSame.Code, wSame.Body.String())
	}

	// 缺 source_version → 400。
	wMissing := doJSON(r, http.MethodPost, deltaPath, token, map[string]string{
		"os": "windows", "arch": "x86_64",
	})
	if wMissing.Code != http.StatusBadRequest {
		t.Fatalf("missing source_version must be 400, got %d %s", wMissing.Code, wMissing.Body.String())
	}

	// 合法请求 → 202 + job_id + 引擎可用性快照。
	wOK := doJSON(r, http.MethodPost, deltaPath, token, map[string]string{
		"source_version": "1.0.0", "os": "windows", "arch": "x86_64",
	})
	if wOK.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d %s", wOK.Code, wOK.Body.String())
	}
	var res struct {
		JobID   string `json:"job_id"`
		Created bool   `json:"created"`
		Engines []struct {
			Algo           string `json:"algo"`
			Implementation string `json:"implementation"`
		} `json:"engines"`
	}
	if err := json.Unmarshal(wOK.Body.Bytes(), &res); err != nil {
		t.Fatalf("decode 202 body: %v %s", err, wOK.Body.String())
	}
	if res.JobID == "" || !res.Created {
		t.Fatalf("expected job_id/created in 202 body: %s", wOK.Body.String())
	}
	if len(res.Engines) != 3 {
		t.Fatalf("expected 3 engines in response, got %d", len(res.Engines))
	}
	implByAlgo := map[string]string{}
	for _, e := range res.Engines {
		if e.Implementation == "official-cli" {
			t.Fatalf("official-cli must not appear: %+v", e)
		}
		implByAlgo[e.Algo] = e.Implementation
	}
	if implByAlgo["hdiffpatch"] != "cgo" {
		t.Fatalf("hdiffpatch implementation=%q, want cgo", implByAlgo["hdiffpatch"])
	}
	jobID, err := uuid.Parse(res.JobID)
	if err != nil {
		t.Fatalf("job_id must be uuid: %v", err)
	}

	// 任务可执行成功（与 worker 同路径）。
	if err := projSvc.ExecuteDeltaJob(ctx, jobID); err != nil {
		t.Fatalf("execute delta job: %v", err)
	}
}
