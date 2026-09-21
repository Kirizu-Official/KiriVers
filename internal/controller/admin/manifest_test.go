package admin

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Kirizu-Official/KiriVers/internal/model"
	"github.com/Kirizu-Official/KiriVers/internal/service"
)

func TestManifestHTTP_PutAndGet(t *testing.T) {
	r, projSvc, _, token := setupProjectHTTP(t)
	ctx := t.Context()

	slug := "test-manifest-proj"
	p, _, err := projSvc.Create(ctx, service.CreateProjectInput{DefaultLocale: ptr("en"), Slug: &slug,
		CompareEngine: ptr(model.CompareEngineSemver),
	})
	require.NoError(t, err)

	_, _, err = projSvc.PutVersion(ctx, p.ID, "1.0.0", service.VersionWriteInput{
		Channel: "stable",
	})
	require.NoError(t, err)

	_, err = projSvc.AddVersionLine(ctx, p.ID, "1.0.0", service.VersionLineWriteInput{
		OS:   "windows",
		Arch: "x86_64",
	})
	require.NoError(t, err)

	sha1 := "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"
	md5Str := "d41d8cd98f00b204e9800998ecf8427e"

	putBody := gin.H{
		"entries": []gin.H{
			{
				"path":           "bin/app.exe",
				"size":           2048,
				"sha256":         sha1,
				"md5":            md5Str,
				"install_policy": "OVERWRITE",
			},
			{
				"path":           "config/app.ini",
				"size":           512,
				"sha256":         sha1,
				"md5":            md5Str,
				"install_policy": "KEEP_IF_EXISTS",
			},
		},
	}

	// 1. PUT Manifest
	wPut := doJSON(r, http.MethodPut, fmt.Sprintf("/api/v1/admin/projects/%s/versions/1.0.0/lines/windows/x86_64/manifest", p.Slug), token, putBody)
	require.Equal(t, http.StatusOK, wPut.Code)

	var putResp struct {
		RootHash string `json:"root_hash"`
		Count    int    `json:"count"`
		Entries  []struct {
			Path           string `json:"path"`
			InstallPolicy  string `json:"install_policy"`
			IntegrityCheck bool   `json:"integrity_check"`
		} `json:"entries"`
	}
	require.NoError(t, json.Unmarshal(wPut.Body.Bytes(), &putResp))
	assert.NotEmpty(t, putResp.RootHash)
	assert.Equal(t, 2, putResp.Count)

	// 验证 KEEP_IF_EXISTS 存入时 integrity_check 为 false
	for _, e := range putResp.Entries {
		if e.Path == "config/app.ini" {
			assert.Equal(t, "KEEP_IF_EXISTS", e.InstallPolicy)
			assert.False(t, e.IntegrityCheck)
		} else if e.Path == "bin/app.exe" {
			assert.Equal(t, "OVERWRITE", e.InstallPolicy)
			assert.True(t, e.IntegrityCheck)
		}
	}

	// 2. Admin GET Manifest
	wGet := doJSON(r, http.MethodGet, fmt.Sprintf("/api/v1/admin/projects/%s/versions/1.0.0/lines/windows/x86_64/manifest", p.Slug), token, nil)
	require.Equal(t, http.StatusOK, wGet.Code)

	var getResp struct {
		RootHash string `json:"root_hash"`
		Count    int    `json:"count"`
	}
	require.NoError(t, json.Unmarshal(wGet.Body.Bytes(), &getResp))
	assert.Equal(t, putResp.RootHash, getResp.RootHash)
	assert.Equal(t, 2, getResp.Count)

}

func TestManifestHTTP_InvalidPath(t *testing.T) {
	r, projSvc, _, token := setupProjectHTTP(t)
	ctx := t.Context()

	slug := "test-inv-path"
	p, _, err := projSvc.Create(ctx, service.CreateProjectInput{DefaultLocale: ptr("en"), Slug: &slug,
		CompareEngine: ptr(model.CompareEngineSemver),
	})
	require.NoError(t, err)

	_, _, err = projSvc.PutVersion(ctx, p.ID, "1.0.0", service.VersionWriteInput{
		Channel: "stable",
	})
	require.NoError(t, err)

	_, err = projSvc.AddVersionLine(ctx, p.ID, "1.0.0", service.VersionLineWriteInput{
		OS:   "windows",
		Arch: "x86_64",
	})
	require.NoError(t, err)

	sha1 := "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"
	md5Str := "d41d8cd98f00b204e9800998ecf8427e"

	// 含 .. 的相对路径 -> 400 INVALID_PATH，不 500
	badBody := gin.H{
		"entries": []gin.H{
			{
				"path":   "../secret/key.pem",
				"size":   100,
				"sha256": sha1,
				"md5":    md5Str,
			},
		},
	}
	w := doJSON(r, http.MethodPut, fmt.Sprintf("/api/v1/admin/projects/%s/versions/1.0.0/lines/windows/x86_64/manifest", p.Slug), token, badBody)
	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Equal(t, "INVALID_PATH", decodeErr(t, w))
}

func TestManifestHTTP_MultiFileLine_ArchiveGates(t *testing.T) {
	r, projSvc, store, token := setupProjectHTTP(t)
	ctx := t.Context()

	slug := "test-mf-gates"
	p, _, err := projSvc.Create(ctx, service.CreateProjectInput{DefaultLocale: ptr("en"), Slug: &slug,
		CompareEngine: ptr(model.CompareEngineSemver),
	})
	require.NoError(t, err)

	// 配置 package_type 为 multi_file
	err = store.CreateMatrix(ctx, &model.PlatformMatrix{
		ProjectID:   p.ID,
		OS:          "linux",
		Arch:        "x86_64",
		PackageType: model.PackageTypeMultiFile,
	})
	require.NoError(t, err)

	_, _, err = projSvc.PutVersion(ctx, p.ID, "1.0.0", service.VersionWriteInput{
		Channel: "stable",
	})
	require.NoError(t, err)

	_, err = projSvc.AddVersionLine(ctx, p.ID, "1.0.0", service.VersionLineWriteInput{
		OS:   "linux",
		Arch: "x86_64",
	})
	require.NoError(t, err)

	// 1. 无全量 zip 归档前，调用 ready 接口 -> 409 ARCHIVE_REQUIRED
	wReady := doJSON(r, http.MethodPost, fmt.Sprintf("/api/v1/admin/projects/%s/versions/1.0.0/lines/linux/x86_64/ready", p.Slug), token, nil)
	assert.Equal(t, http.StatusConflict, wReady.Code)
	assert.Equal(t, "ARCHIVE_REQUIRED", decodeErr(t, wReady))

	// 2. 无全量归档前，尝试发布版本 -> 失败
	wPub := doJSON(r, http.MethodPost, fmt.Sprintf("/api/v1/admin/projects/%s/versions/1.0.0/publish", p.Slug), token, nil)
	assert.Equal(t, http.StatusConflict, wPub.Code)

	// 3. 上传合法的全量 zip
	zipBuf := new(bytes.Buffer)
	zw := zip.NewWriter(zipBuf)
	wFile, _ := zw.Create("bin/server")
	_, _ = wFile.Write([]byte("server binary executable content"))
	_ = zw.Close()

	reqBuild := httptest.NewRequest(
		http.MethodPost,
		fmt.Sprintf("/api/v1/admin/projects/%s/versions/1.0.0/lines/linux/x86_64/build-archive", p.Slug),
		bytes.NewReader(zipBuf.Bytes()),
	)
	reqBuild.Header.Set("Authorization", "Bearer "+token)
	reqBuild.Header.Set("Content-Type", "application/zip")
	wBuild := httptest.NewRecorder()
	r.ServeHTTP(wBuild, reqBuild)

	assert.Equal(t, http.StatusCreated, wBuild.Code)

	// 4. 现在切片已就绪，调用 publish -> 200 OK
	wPub2 := doJSON(r, http.MethodPost, fmt.Sprintf("/api/v1/admin/projects/%s/versions/1.0.0/publish", p.Slug), token, nil)
	assert.Equal(t, http.StatusOK, wPub2.Code)
}
