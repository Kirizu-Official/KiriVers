package admin

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Kirizu-Official/KiriVers/internal/model"
	"github.com/Kirizu-Official/KiriVers/internal/repository"
	"github.com/Kirizu-Official/KiriVers/internal/service"
	"github.com/Kirizu-Official/KiriVers/pkg/response"
)

func makeTestZip(files map[string][]byte) []byte {
	buf := new(bytes.Buffer)
	zw := zip.NewWriter(buf)
	for name, content := range files {
		w, _ := zw.Create(name)
		_, _ = w.Write(content)
	}
	_ = zw.Close()
	return buf.Bytes()
}

func createMultipartReq(url, token, idempotencyKey string, fields map[string]string, fileFieldName, fileName string, fileContent []byte) *http.Request {
	body := new(bytes.Buffer)
	writer := multipart.NewWriter(body)
	for k, v := range fields {
		_ = writer.WriteField(k, v)
	}
	if len(fileContent) > 0 {
		part, _ := writer.CreateFormFile(fileFieldName, fileName)
		_, _ = part.Write(fileContent)
	}
	_ = writer.Close()

	req := httptest.NewRequest(http.MethodPost, url, body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	if idempotencyKey != "" {
		req.Header.Set("Idempotency-Key", idempotencyKey)
	}
	return req
}

// TestCIToken_CreateListMasked_AndRevoke 测试 CI Token 签发、明文只出现一次、GET 脱敏与吊销 (AC 1, C07-1)。
func TestCIToken_CreateListMasked_AndRevoke(t *testing.T) {
	r, projSvc, _, token := setupProjectHTTP(t)
	jobRepo := repository.NewMemoryJobRepo()
	projSvc.SetJobStore(jobRepo)
	ctx := context.Background()

	slug := "ci-tok-proj"
	engine := model.CompareEngineSemver
	p, _, err := projSvc.Create(ctx, service.CreateProjectInput{DefaultLocale: ptr("en"), Slug:          &slug,
		CompareEngine: &engine,
	})
	require.NoError(t, err)

	// 1. 签发 CI Token (POST /ci-tokens)
	createURL := fmt.Sprintf("/api/v1/admin/projects/%s/ci-tokens", p.Slug)
	wCreate := doJSON(r, http.MethodPost, createURL, token, map[string]any{
		"name":   "github-actions",
		"scopes": []string{"project:read", "release:publish", "artifact:write"},
	})
	require.Equal(t, http.StatusCreated, wCreate.Code)

	var tokResp struct {
		ID          string   `json:"id"`
		Name        string   `json:"name"`
		Token       string   `json:"token"`
		Fingerprint string   `json:"fingerprint"`
		Scopes      []string `json:"scopes"`
	}
	require.NoError(t, json.Unmarshal(wCreate.Body.Bytes(), &tokResp))
	assert.NotEmpty(t, tokResp.ID)
	assert.Equal(t, "github-actions", tokResp.Name)
	assert.True(t, strings.HasPrefix(tokResp.Token, "kvc_"), "token must start with kvc_")
	assert.Len(t, tokResp.Token, 68, "kvc_ + 64 hex characters = 68")
	assert.NotEmpty(t, tokResp.Fingerprint)
	assert.True(t, strings.HasPrefix(tokResp.Token, tokResp.Fingerprint))

	ciTokenStr := tokResp.Token

	// 2. 查询 CI Token 列表 (GET /ci-tokens)，验证第二次看无法看到明文
	wList := doJSON(r, http.MethodGet, createURL, token, nil)
	require.Equal(t, http.StatusOK, wList.Code)

	var listResp struct {
		Tokens []struct {
			ID          string   `json:"id"`
			Name        string   `json:"name"`
			Token       string   `json:"token"`
			Fingerprint string   `json:"fingerprint"`
			Scopes      []string `json:"scopes"`
		} `json:"tokens"`
	}
	require.NoError(t, json.Unmarshal(wList.Body.Bytes(), &listResp))
	require.Len(t, listResp.Tokens, 1)
	assert.Equal(t, tokResp.ID, listResp.Tokens[0].ID)
	assert.Empty(t, listResp.Tokens[0].Token, "GET ci-tokens must never return plaintext token")
	assert.Equal(t, tokResp.Fingerprint, listResp.Tokens[0].Fingerprint)

	// 3. 使用 CI Token 访问自身项目版本列表 -> 成功 (200)
	versURL := fmt.Sprintf("/api/v1/admin/projects/%s/versions", p.Slug)
	wVers := doJSON(r, http.MethodGet, versURL, ciTokenStr, nil)
	assert.Equal(t, http.StatusOK, wVers.Code)

	// 4. 吊销 CI Token (DELETE /ci-tokens/:token_id)
	delURL := fmt.Sprintf("/api/v1/admin/projects/%s/ci-tokens/%s", p.Slug, tokResp.ID)
	wDel := doJSON(r, http.MethodDelete, delURL, token, nil)
	assert.Equal(t, http.StatusNoContent, wDel.Code)

	// 5. 再次使用已吊销的 CI Token -> 401 UNAUTHORIZED
	wRevoked := doJSON(r, http.MethodGet, versURL, ciTokenStr, nil)
	assert.Equal(t, http.StatusUnauthorized, wRevoked.Code)
}

// TestCIToken_SecurityIsolation 测试 CI Token 不能创建项目，也不能越权访问其他项目 (AC 11, §11.1)。
func TestCIToken_SecurityIsolation(t *testing.T) {
	r, projSvc, _, adminToken := setupProjectHTTP(t)
	jobRepo := repository.NewMemoryJobRepo()
	projSvc.SetJobStore(jobRepo)
	ctx := context.Background()

	slugA := "proj-iso-a"
	slugB := "proj-iso-b"
	engine := model.CompareEngineSemver

	pA, _, err := projSvc.Create(ctx, service.CreateProjectInput{DefaultLocale: ptr("en"), Slug: &slugA, CompareEngine: &engine})
	require.NoError(t, err)
	pB, _, err := projSvc.Create(ctx, service.CreateProjectInput{DefaultLocale: ptr("en"), Slug: &slugB, CompareEngine: &engine})
	require.NoError(t, err)

	// 签发项目 A 的 CI Token
	ciTokA, err := projSvc.CreateCIToken(ctx, pA.Slug, "ci-a", []string{"release:publish", "artifact:write"}, nil)
	require.NoError(t, err)

	// 1. CI Token 尝试调用 POST /api/v1/admin/projects (创建项目) -> 403 FORBIDDEN
	wCreateProj := doJSON(r, http.MethodPost, "/api/v1/admin/projects", ciTokA.Plaintext, map[string]any{
		"name":           "hacked-proj",
		"slug":           "hacked-proj",
		"compare_engine": "semver",
	})
	assert.Equal(t, http.StatusForbidden, wCreateProj.Code)

	// 2. 项目 A 的 CI Token 尝试访问项目 B 的资源 -> 403 FORBIDDEN
	wAccessB := doJSON(r, http.MethodGet, fmt.Sprintf("/api/v1/admin/projects/%s/versions", pB.Slug), ciTokA.Plaintext, nil)
	assert.Equal(t, http.StatusForbidden, wAccessB.Code)

	_ = adminToken
}

// TestCIRelease_JSONNoOsArch_BundleUnpack_TwoLines_AutoPublish 测试一键发版及自动发布 (AC 2, 4, 8, 9)。
func TestCIRelease_JSONNoOsArch_BundleUnpack_TwoLines_AutoPublish(t *testing.T) {
	r, projSvc, _, token := setupProjectHTTP(t)
	jobRepo := repository.NewMemoryJobRepo()
	projSvc.SetJobStore(jobRepo)
	ctx := context.Background()

	slug := "ci-release-bundle"
	engine := model.CompareEngineSemver
	p, _, err := projSvc.Create(ctx, service.CreateProjectInput{DefaultLocale: ptr("en"), Slug: &slug, CompareEngine: &engine})
	require.NoError(t, err)

	// 构造含两平台切片的 zip 归档包 (含特殊垃圾文件)
	zipData := makeTestZip(map[string][]byte{
		"linux/x86_64/myapp":       []byte("#!/bin/sh\necho linux"),
		"windows/x86_64/myapp.exe": []byte("MZ-windows-binary"),
		"__MACOSX/._myapp":         []byte("junk"),
		"linux/x86_64/.DS_Store":   []byte("junk"),
		"windows/x86_64/Thumbs.db": []byte("junk"),
	})

	// 请求元数据：JSON 没有 os、arch 字段 (AC 8)，且省略 publish (AC 9: 默认 true)
	metadataJSON := `{"version": "1.0.0", "channel": "stable", "changelog": "v1.0 release"}`

	req := createMultipartReq(
		fmt.Sprintf("/api/v1/admin/projects/%s/ci/releases", p.Slug),
		token,
		"",
		map[string]string{"metadata": metadataJSON},
		"file",
		"release.zip",
		zipData,
	)

	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusAccepted, w.Code)

	var jobResp struct {
		JobID  string `json:"job_id"`
		Status string `json:"status"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &jobResp))
	assert.NotEmpty(t, jobResp.JobID)
	jobUUID, err := uuid.Parse(jobResp.JobID)
	require.NoError(t, err)

	// 执行 Bundle 任务并同步等待完成
	err = projSvc.ExecuteBundleJob(ctx, jobUUID)
	require.NoError(t, err)

	// 轮询 Job 状态接口 GET /api/v1/admin/jobs/:job_id (AC 4)
	wJob := doJSON(r, http.MethodGet, fmt.Sprintf("/api/v1/admin/jobs/%s", jobResp.JobID), token, nil)
	require.Equal(t, http.StatusOK, wJob.Code)

	var jobDetail struct {
		ID     string `json:"id"`
		Status string `json:"status"`
	}
	require.NoError(t, json.Unmarshal(wJob.Body.Bytes(), &jobDetail))
	assert.Equal(t, model.JobStatusSucceeded, jobDetail.Status)

	// 验证拆出的两条 Line 状态均已就绪 (AC 2)
	lines, err := projSvc.ListVersionLines(ctx, p.ID, "1.0.0")
	require.NoError(t, err)
	assert.Len(t, lines, 2)
	for _, l := range lines {
		assert.Equal(t, model.VersionLineStatusReady, l.Status)
	}

	// 验证版本已被自动发布 (AC 9: 省略 publish 默认发布)
	v, err := projSvc.ResolveVersion(ctx, p.ID, "1.0.0")
	require.NoError(t, err)
	assert.Equal(t, model.VersionStatusPublished, v.Status)
	assert.NotNil(t, v.PublishTime)
}

// TestCIRelease_BareFileAtRoot_FailsWithZipLayoutInvalid 测试根目录裸文件拒绝 (AC 3, C07-4)。
func TestCIRelease_BareFileAtRoot_FailsWithZipLayoutInvalid(t *testing.T) {
	r, projSvc, _, token := setupProjectHTTP(t)
	jobRepo := repository.NewMemoryJobRepo()
	projSvc.SetJobStore(jobRepo)
	ctx := context.Background()

	slug := "ci-bare-file"
	engine := model.CompareEngineSemver
	p, _, err := projSvc.Create(ctx, service.CreateProjectInput{DefaultLocale: ptr("en"), Slug: &slug, CompareEngine: &engine})
	require.NoError(t, err)

	// 根目录存在裸文件
	zipData := makeTestZip(map[string][]byte{
		"bare_file.txt":      []byte("bare file at root"),
		"linux/x86_64/myapp": []byte("linux binary"),
	})

	metadataJSON := `{"version": "1.0.0", "channel": "stable"}`
	req := createMultipartReq(
		fmt.Sprintf("/api/v1/admin/projects/%s/ci/releases", p.Slug),
		token,
		"",
		map[string]string{"metadata": metadataJSON},
		"file",
		"bundle.zip",
		zipData,
	)

	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusAccepted, w.Code)

	var jobResp struct {
		JobID string `json:"job_id"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &jobResp))
	jobUUID, err := uuid.Parse(jobResp.JobID)
	require.NoError(t, err)

	// 执行 Job，应当报错并在 job 状态记录 failed
	_ = projSvc.ExecuteBundleJob(ctx, jobUUID)

	wJob := doJSON(r, http.MethodGet, fmt.Sprintf("/api/v1/admin/jobs/%s", jobResp.JobID), token, nil)
	require.Equal(t, http.StatusOK, wJob.Code)

	var jobDetail struct {
		Status       string `json:"status"`
		ErrorMessage string `json:"error_message"`
	}
	require.NoError(t, json.Unmarshal(wJob.Body.Bytes(), &jobDetail))
	assert.Equal(t, model.JobStatusFailed, jobDetail.Status)
	assert.Contains(t, strings.ToLower(jobDetail.ErrorMessage), "zip layout invalid")
}

// TestCIRelease_NestedZipArchive_UnwrapsInnerArchive 测试外层嵌套归档自动解包 (AC 10, §11.3.3)。
func TestCIRelease_NestedZipArchive_UnwrapsInnerArchive(t *testing.T) {
	r, projSvc, _, token := setupProjectHTTP(t)
	jobRepo := repository.NewMemoryJobRepo()
	projSvc.SetJobStore(jobRepo)
	ctx := context.Background()

	slug := "ci-nested-zip"
	engine := model.CompareEngineSemver
	p, _, err := projSvc.Create(ctx, service.CreateProjectInput{DefaultLocale: ptr("en"), Slug: &slug, CompareEngine: &engine})
	require.NoError(t, err)

	// 1. 构造内层 zip
	innerZip := makeTestZip(map[string][]byte{
		"linux/x86_64/app": []byte("inner linux binary"),
	})

	// 2. 构造外层嵌套 zip (包含 inner.zip)
	outerZip := makeTestZip(map[string][]byte{
		"nested.zip": innerZip,
	})

	metadataJSON := `{"version": "1.0.0", "channel": "stable"}`
	req := createMultipartReq(
		fmt.Sprintf("/api/v1/admin/projects/%s/ci/releases", p.Slug),
		token,
		"",
		map[string]string{"metadata": metadataJSON},
		"file",
		"outer.zip",
		outerZip,
	)

	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusAccepted, w.Code)

	var jobResp struct {
		JobID string `json:"job_id"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &jobResp))
	jobUUID, err := uuid.Parse(jobResp.JobID)
	require.NoError(t, err)

	err = projSvc.ExecuteBundleJob(ctx, jobUUID)
	require.NoError(t, err)

	wJob := doJSON(r, http.MethodGet, fmt.Sprintf("/api/v1/admin/jobs/%s", jobResp.JobID), token, nil)
	require.Equal(t, http.StatusOK, wJob.Code)

	var jobDetail struct {
		Status string `json:"status"`
	}
	require.NoError(t, json.Unmarshal(wJob.Body.Bytes(), &jobDetail))
	assert.Equal(t, model.JobStatusSucceeded, jobDetail.Status)
}

// TestCIRelease_IdempotencyKey_24h 测试同 Idempotency-Key 幂等不重复解包 (AC 6, C07-10)。
func TestCIRelease_IdempotencyKey_24h(t *testing.T) {
	r, projSvc, _, token := setupProjectHTTP(t)
	jobRepo := repository.NewMemoryJobRepo()
	projSvc.SetJobStore(jobRepo)

	slug := "ci-idemp"
	engine := model.CompareEngineSemver
	p, _, err := projSvc.Create(context.Background(), service.CreateProjectInput{DefaultLocale: ptr("en"), Slug: &slug, CompareEngine: &engine})
	require.NoError(t, err)

	zipData := makeTestZip(map[string][]byte{
		"linux/x86_64/app": []byte("linux binary"),
	})

	idempKey := "idem-key-release-24h"
	metadataJSON := `{"version": "1.0.0", "channel": "stable"}`

	// 第一次提交
	req1 := createMultipartReq(
		fmt.Sprintf("/api/v1/admin/projects/%s/ci/releases", p.Slug),
		token,
		idempKey,
		map[string]string{"metadata": metadataJSON},
		"file",
		"bundle.zip",
		zipData,
	)
	w1 := httptest.NewRecorder()
	r.ServeHTTP(w1, req1)
	require.Equal(t, http.StatusAccepted, w1.Code)

	var job1 struct {
		JobID string `json:"job_id"`
	}
	require.NoError(t, json.Unmarshal(w1.Body.Bytes(), &job1))

	// 第二次提交相同 Idempotency-Key
	req2 := createMultipartReq(
		fmt.Sprintf("/api/v1/admin/projects/%s/ci/releases", p.Slug),
		token,
		idempKey,
		map[string]string{"metadata": metadataJSON},
		"file",
		"bundle.zip",
		zipData,
	)
	w2 := httptest.NewRecorder()
	r.ServeHTTP(w2, req2)
	require.Equal(t, http.StatusAccepted, w2.Code)

	var job2 struct {
		JobID string `json:"job_id"`
	}
	require.NoError(t, json.Unmarshal(w2.Body.Bytes(), &job2))

	// 两次返回必须是同一个 JobID (AC 6)
	assert.Equal(t, job1.JobID, job2.JobID)
}

// TestJob_CrossProjectIsolation 测试跨项目查询 Job 返回 403 FORBIDDEN (AC 7, §11.4)。
func TestJob_CrossProjectIsolation(t *testing.T) {
	r, projSvc, _, _ := setupProjectHTTP(t)
	jobRepo := repository.NewMemoryJobRepo()
	projSvc.SetJobStore(jobRepo)
	ctx := context.Background()

	slugA := "proj-job-a"
	slugB := "proj-job-b"
	engine := model.CompareEngineSemver

	pA, _, err := projSvc.Create(ctx, service.CreateProjectInput{DefaultLocale: ptr("en"), Slug: &slugA, CompareEngine: &engine})
	require.NoError(t, err)
	pB, _, err := projSvc.Create(ctx, service.CreateProjectInput{DefaultLocale: ptr("en"), Slug: &slugB, CompareEngine: &engine})
	require.NoError(t, err)

	// 签发项目 B 的 CI Token
	ciTokB, err := projSvc.CreateCIToken(ctx, pB.Slug, "ci-b", []string{"release:publish", "artifact:write"}, nil)
	require.NoError(t, err)

	// 为项目 A 创建一个 Job
	jobA := &model.Job{
		ID:        uuid.New(),
		ProjectID: &pA.ID,
		Type:      "bundle_unpack",
		Status:    model.JobStatusQueued,
	}
	require.NoError(t, jobRepo.Create(ctx, jobA))

	// 项目 B 的 CI Token 尝试查询项目 A 的 Job -> 403 FORBIDDEN (AC 7)
	w := doJSON(r, http.MethodGet, fmt.Sprintf("/api/v1/admin/jobs/%s", jobA.ID), ciTokB.Plaintext, nil)
	assert.Equal(t, http.StatusForbidden, w.Code)

	var env response.Body
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &env))
	assert.Equal(t, "FORBIDDEN", env.Error.Code)
}

// TestManualPublish_AutoPublishPending 测试发布时齐套检查未满足返回 409 AUTO_PUBLISH_PENDING (C07-9)。
func TestManualPublish_AutoPublishPending(t *testing.T) {
	r, projSvc, _, token := setupProjectHTTP(t)
	ctx := context.Background()

	slug := "proj-gate-check"
	engine := model.CompareEngineSemver
	p, _, err := projSvc.Create(ctx, service.CreateProjectInput{DefaultLocale: ptr("en"), Slug: &slug, CompareEngine: &engine})
	require.NoError(t, err)

	// 创建 Draft 版本，要求齐套发布 linux/x86_64 和 windows/x86_64
	allowPartial := false
	_, _, err = projSvc.PutVersion(ctx, p.ID, "1.0.0", service.VersionWriteInput{
		Channel: "stable",
		AutoPublishWhen: &model.AutoPublishRule{
			RequiredLines: []string{"linux/x86_64", "windows/x86_64"},
			AllowPartial:  allowPartial,
		},
	})
	require.NoError(t, err)

	// 只上传 linux/x86_64 产物
	_, err = projSvc.UploadArtifact(ctx, p.Slug, "1.0.0", "linux", "x86_64", service.UploadArtifactInput{
		Filename: "app",
		Size:     4,
	}, strings.NewReader("exec"))
	require.NoError(t, err)

	// 手动触发发版 POST /api/v1/admin/projects/:slug/versions/1.0.0/publish
	pubURL := fmt.Sprintf("/api/v1/admin/projects/%s/versions/1.0.0/publish", p.Slug)
	wPub := doJSON(r, http.MethodPost, pubURL, token, nil)
	assert.Equal(t, http.StatusConflict, wPub.Code)

	var env response.Body
	require.NoError(t, json.Unmarshal(wPub.Body.Bytes(), &env))
	assert.Equal(t, "AUTO_PUBLISH_PENDING", env.Error.Code)
}
