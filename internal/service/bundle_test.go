package service

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Kirizu-Official/KiriVers/internal/model"
	"github.com/Kirizu-Official/KiriVers/internal/repository"
	"github.com/Kirizu-Official/KiriVers/internal/storage"
)

func createZipArchive(files map[string][]byte) []byte {
	buf := new(bytes.Buffer)
	zw := zip.NewWriter(buf)
	for name, content := range files {
		w, _ := zw.Create(name)
		_, _ = w.Write(content)
	}
	_ = zw.Close()
	return buf.Bytes()
}

func createTarGzArchive(files map[string][]byte) []byte {
	buf := new(bytes.Buffer)
	gw := gzip.NewWriter(buf)
	tw := tar.NewWriter(gw)
	for name, content := range files {
		_ = tw.WriteHeader(&tar.Header{
			Name: name,
			Mode: 0755,
			Size: int64(len(content)),
		})
		_, _ = tw.Write(content)
	}
	_ = tw.Close()
	_ = gw.Close()
	return buf.Bytes()
}

func setupBundleTest(t *testing.T) (*ProjectService, repository.ProjectStore, *repository.MemoryJobRepo, *model.Project) {
	t.Helper()
	store := repository.NewMemoryProjectStore()
	jobStore := repository.NewMemoryJobRepo()
	tempDir := t.TempDir()
	backend, err := storage.NewLocalFS(tempDir)
	require.NoError(t, err)

	svc := NewProjectService(store, backend)
	svc.SetJobStore(jobStore)
	svc.SetInstallPolicyStore(repository.NewMemoryInstallPolicyRuleStore())

	ctx := context.Background()
	slug := "test-bundle-proj"
	engine := "semver"
	p, _, err := svc.Create(ctx, CreateProjectInput{DefaultLocale: ptr("en"), Slug: &slug,
		CompareEngine: &engine,
	})
	require.NoError(t, err)

	return svc, store, jobStore, p
}

func TestBundleUnpack_TwoLines_LinuxAndWindows(t *testing.T) {
	svc, _, _, p := setupBundleTest(t)
	ctx := context.Background()

	// 1. 创建草稿版本
	v, _, err := svc.PutVersion(ctx, p.ID, "1.0.0", VersionWriteInput{
		Channel: "stable",
	})
	require.NoError(t, err)

	// 2. 构造含 linux/x86_64/ 与 windows/x86_64/ 的压缩包
	archive := createZipArchive(map[string][]byte{
		"linux/x86_64/app":       []byte("#!/bin/sh\necho hello"),
		"windows/x86_64/app.exe": []byte("MZ-windows-binary"),
		"__MACOSX/._app":         []byte("junk"),
		"linux/x86_64/.DS_Store": []byte("junk"),
	})

	// 3. 解压并拆线 (publish=true)
	res, err := svc.UnpackAndProcessBundle(ctx, p.ID, v.ID.String(), archive, true)
	require.NoError(t, err)
	assert.True(t, res.Success)
	assert.Len(t, res.Lines, 2)

	// 4. 验证拆出的两条 Line
	lines, err := svc.ListVersionLines(ctx, p.ID, v.ID.String())
	require.NoError(t, err)
	assert.Len(t, lines, 2)

	osMap := make(map[string]model.VersionLine)
	for _, l := range lines {
		osMap[l.OS] = l
		assert.Equal(t, model.VersionLineStatusReady, l.Status)
	}
	assert.Contains(t, osMap, "linux")
	assert.Contains(t, osMap, "windows")

	// 5. 验证版本被自动发布
	published, err := svc.ResolveVersion(ctx, p.ID, "1.0.0")
	require.NoError(t, err)
	assert.Equal(t, model.VersionStatusPublished, published.Status)
	assert.NotNil(t, published.PublishTime)
}

func TestBundleUnpack_BareFileAtRoot_FailsWithZipLayoutInvalid(t *testing.T) {
	svc, _, _, p := setupBundleTest(t)
	ctx := context.Background()

	v, _, err := svc.PutVersion(ctx, p.ID, "1.0.0", VersionWriteInput{
		Channel: "stable",
	})
	require.NoError(t, err)

	// 根目录存在裸文件
	archive := createZipArchive(map[string][]byte{
		"bare_file.txt":    []byte("bare file at root"),
		"linux/x86_64/app": []byte("linux binary"),
	})

	_, err = svc.UnpackAndProcessBundle(ctx, p.ID, v.ID.String(), archive, false)
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrZipLayoutInvalid))
}

func TestBundleUnpack_NestedArchive_UnwrapsInnerArchive(t *testing.T) {
	svc, _, _, p := setupBundleTest(t)
	ctx := context.Background()

	v, _, err := svc.PutVersion(ctx, p.ID, "1.0.0", VersionWriteInput{
		Channel: "stable",
	})
	require.NoError(t, err)

	// 内层包含合法的 linux/x86_64/app
	innerZip := createZipArchive(map[string][]byte{
		"linux/x86_64/app": []byte("linux binary inside inner"),
	})

	// 外层再套一层 zip
	outerZip := createZipArchive(map[string][]byte{
		"inner_payload.zip": innerZip,
	})

	res, err := svc.UnpackAndProcessBundle(ctx, p.ID, v.ID.String(), outerZip, false)
	require.NoError(t, err)
	assert.True(t, res.Success)
	assert.Len(t, res.Lines, 1)
	assert.Equal(t, "linux", res.Lines[0].OS)
	assert.Equal(t, "x86_64", res.Lines[0].Arch)
}

func TestBundleUnpack_SingleDirectoryWrapper_Stripped(t *testing.T) {
	svc, _, _, p := setupBundleTest(t)
	ctx := context.Background()

	v, _, err := svc.PutVersion(ctx, p.ID, "1.0.0", VersionWriteInput{
		Channel: "stable",
	})
	require.NoError(t, err)

	// 外层仅有一个包装目录 release-v1.0.0/
	archive := createZipArchive(map[string][]byte{
		"release-v1.0.0/linux/x86_64/app":       []byte("linux binary"),
		"release-v1.0.0/windows/x86_64/app.exe": []byte("windows binary"),
	})

	res, err := svc.UnpackAndProcessBundle(ctx, p.ID, v.ID.String(), archive, false)
	require.NoError(t, err)
	assert.True(t, res.Success)
	assert.Len(t, res.Lines, 2)
}

func TestBundleUnpack_MultiFile_KeepIfExists(t *testing.T) {
	svc, _, _, p := setupBundleTest(t)
	ctx := context.Background()

	osStr := "linux"
	archStr := "x86_64"
	pkgType := model.PackageTypeMultiFile
	_, err := svc.CreateMatrix(ctx, p.ID, MatrixWrite{
		OS:          &osStr,
		Arch:        &archStr,
		PackageType: &pkgType,
	})
	require.NoError(t, err)
	if _, err := svc.PutProjectInstallPolicy(ctx, p.ID, "linux", "x86_64", []InstallPolicyEntry{
		{Path: "conf/settings.json", InstallPolicy: model.InstallPolicyKeepIfExists},
	}); err != nil {
		t.Fatal(err)
	}

	v, _, err := svc.PutVersion(ctx, p.ID, "1.0.0", VersionWriteInput{
		Channel: "stable",
	})
	require.NoError(t, err)

	archive := createZipArchive(map[string][]byte{
		"linux/x86_64/app":                []byte("executable"),
		"linux/x86_64/conf/settings.json": []byte(`{"port":8080}`),
		"linux/x86_64/keep_if_exists.txt": []byte("conf/settings.json\n"),
	})

	res, err := svc.UnpackAndProcessBundle(ctx, p.ID, v.ID.String(), archive, false)
	require.NoError(t, err)
	assert.True(t, res.Success)
	assert.Len(t, res.Lines, 1)
	assert.Equal(t, model.PackageTypeMultiFile, res.Lines[0].PackageType)
	assert.NotEmpty(t, res.Lines[0].RootHash)

	line, err := svc.GetVersionLine(ctx, p.ID, "1.0.0", "linux", "x86_64")
	require.NoError(t, err)
	assert.Equal(t, model.VersionLineStatusReady, line.Status)
	assert.Equal(t, res.Lines[0].RootHash, line.RootHash)

	manifest, err := svc.GetManifest(ctx, p.Slug, "1.0.0", "linux", "x86_64")
	require.NoError(t, err)
	assert.Equal(t, 2, manifest.Count)
	for _, entry := range manifest.Entries {
		if entry.Path == "keep_if_exists.txt" || entry.Path == "_keep.json" {
			t.Fatalf("sidecar must not enter manifest: %s", entry.Path)
		}
		if entry.Path == "conf/settings.json" {
			assert.Equal(t, "KEEP_IF_EXISTS", entry.InstallPolicy)
		}
		if entry.Path == "app" {
			assert.Equal(t, "OVERWRITE", entry.InstallPolicy)
		}
	}
}

func TestBundleUnpack_MatrixPackageTypeConflict(t *testing.T) {
	svc, _, _, p := setupBundleTest(t)
	ctx := context.Background()

	// 在矩阵中显式声明 linux/x86_64 为 single_file
	osStr := "linux"
	archStr := "x86_64"
	pkgType := model.PackageTypeSingleFile
	_, err := svc.CreateMatrix(ctx, p.ID, MatrixWrite{
		OS:          &osStr,
		Arch:        &archStr,
		PackageType: &pkgType,
	})
	require.NoError(t, err)

	v, _, err := svc.PutVersion(ctx, p.ID, "1.0.0", VersionWriteInput{
		Channel: "stable",
	})
	require.NoError(t, err)

	// 上传多文件结构
	archive := createZipArchive(map[string][]byte{
		"linux/x86_64/app":        []byte("binary"),
		"linux/x86_64/extra.data": []byte("extra"),
	})

	_, err = svc.UnpackAndProcessBundle(ctx, p.ID, v.ID.String(), archive, false)
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrZipLayoutInvalid))
}

func TestAutoPublishWhen_ParallelUploadsTriggerPublish(t *testing.T) {
	svc, _, _, p := setupBundleTest(t)
	ctx := context.Background()

	// 1. 创建带有 auto_publish_when 规则的草稿版本
	rule := &model.AutoPublishRule{
		RequiredLines: []string{"linux/x86_64", "windows/x86_64"},
		AllowPartial:  false,
	}
	_, _, err := svc.PutVersion(ctx, p.ID, "1.0.0", VersionWriteInput{
		Channel:         "stable",
		AutoPublishWhen: rule,
	})
	require.NoError(t, err)

	// 2. 上传 linux 切片
	_, err = svc.UploadArtifact(ctx, p.Slug, "1.0.0", "linux", "x86_64", UploadArtifactInput{
		Filename: "app-linux",
		Size:     10,
	}, bytes.NewReader([]byte("linux-app-")))
	require.NoError(t, err)

	// 此时版本仍为 draft
	curV, err := svc.ResolveVersion(ctx, p.ID, "1.0.0")
	require.NoError(t, err)
	assert.Equal(t, model.VersionStatusDraft, curV.Status)

	// 手动触发 publish 会因为未齐套返回 ErrAutoPublishPending (409)
	_, err = svc.PublishVersion(ctx, p.ID, "1.0.0")
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrAutoPublishPending))

	// 3. 上传 windows 切片，满足全部 required_lines
	_, err = svc.UploadArtifact(ctx, p.Slug, "1.0.0", "windows", "x86_64", UploadArtifactInput{
		Filename: "app-win.exe",
		Size:     10,
	}, bytes.NewReader([]byte("windows-app")))
	require.NoError(t, err)

	// 4. 齐套后自动触发发版
	finalV, err := svc.ResolveVersion(ctx, p.ID, "1.0.0")
	require.NoError(t, err)
	assert.Equal(t, model.VersionStatusPublished, finalV.Status)
	assert.NotNil(t, finalV.PublishTime)
}

func TestBundleJob_IdempotencyKey(t *testing.T) {
	svc, _, _, p := setupBundleTest(t)
	ctx := context.Background()

	archive := createZipArchive(map[string][]byte{
		"linux/x86_64/app": []byte("bin"),
	})

	in := CreateBundleReleaseInput{
		Version:         "1.0.0",
		Channel:         "stable",
		IdempotencyKey:  "ci-run-12345",
		ArchiveData:     archive,
		ArchiveFilename: "release.zip",
	}

	jobID1, created1, err := svc.CreateBundleJob(ctx, p.ID, in)
	require.NoError(t, err)
	assert.True(t, created1)
	assert.NotEqual(t, uuid.Nil, jobID1)

	// 第二次使用相同 IdempotencyKey
	jobID2, created2, err := svc.CreateBundleJob(ctx, p.ID, in)
	require.NoError(t, err)
	assert.False(t, created2)
	assert.Equal(t, jobID1, jobID2)
}

func TestJob_CrossProjectIsolation(t *testing.T) {
	svc, _, _, p1 := setupBundleTest(t)
	ctx := context.Background()

	p2Slug := "proj-two"
	p2, _, err := svc.Create(ctx, CreateProjectInput{DefaultLocale: ptr("en"), Slug: &p2Slug})
	require.NoError(t, err)

	archive := createZipArchive(map[string][]byte{
		"linux/x86_64/app": []byte("bin"),
	})

	jobID, _, err := svc.CreateBundleJob(ctx, p1.ID, CreateBundleReleaseInput{
		Version:         "1.0.0",
		ArchiveData:     archive,
		ArchiveFilename: "bundle.zip",
	})
	require.NoError(t, err)

	// 1. Project 2 访问 Project 1 的 Job -> ErrJobForbidden
	_, err = svc.GetJob(ctx, &p2.ID, false, jobID)
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrJobForbidden))

	// 2. Project 1 访问自己的 Job -> 成功
	job1, err := svc.GetJob(ctx, &p1.ID, false, jobID)
	require.NoError(t, err)
	assert.Equal(t, jobID, job1.ID)

	// 3. 实例管理员访问任意 Job -> 成功
	adminJob, err := svc.GetJob(ctx, nil, true, jobID)
	require.NoError(t, err)
	assert.Equal(t, jobID, adminJob.ID)

	// 4. 执行任务并检验状态
	err = svc.ExecuteBundleJob(ctx, jobID)
	require.NoError(t, err)

	updatedJob, err := svc.GetJob(ctx, &p1.ID, false, jobID)
	require.NoError(t, err)
	assert.Equal(t, model.JobStatusSucceeded, updatedJob.Status)
	assert.NotEmpty(t, updatedJob.Result)
}

func TestBundleUnpack_CustomMatrixOS_Unpacks(t *testing.T) {
	svc, _, _, p := setupBundleTest(t)
	ctx := context.Background()

	osSlug := "custom-os"
	archSlug := "x86_64"
	pkg := model.PackageTypeSingleFile
	_, err := svc.CreateMatrix(ctx, p.ID, MatrixWrite{
		OS:          &osSlug,
		Arch:        &archSlug,
		PackageType: &pkg,
	})
	require.NoError(t, err)

	v, _, err := svc.PutVersion(ctx, p.ID, "1.0.0", VersionWriteInput{Channel: "stable"})
	require.NoError(t, err)

	archive := createZipArchive(map[string][]byte{
		"custom-os/x86_64/app": []byte("custom-matrix-binary"),
	})
	res, err := svc.UnpackAndProcessBundle(ctx, p.ID, v.ID.String(), archive, false)
	require.NoError(t, err)
	assert.True(t, res.Success)
	require.Len(t, res.Lines, 1)
	assert.Equal(t, "custom-os", res.Lines[0].OS)
	assert.Equal(t, "x86_64", res.Lines[0].Arch)

	lines, err := svc.ListVersionLines(ctx, p.ID, v.ID.String())
	require.NoError(t, err)
	require.Len(t, lines, 1)
	assert.Equal(t, "custom-os", lines[0].OS)
	assert.Equal(t, model.VersionLineStatusReady, lines[0].Status)
}

func TestBundleUnpack_CustomOSWithoutMatrix_Fails(t *testing.T) {
	svc, _, _, p := setupBundleTest(t)
	ctx := context.Background()

	v, _, err := svc.PutVersion(ctx, p.ID, "1.0.0", VersionWriteInput{Channel: "stable"})
	require.NoError(t, err)

	archive := createZipArchive(map[string][]byte{
		"custom-os/x86_64/app": []byte("unregistered"),
	})
	_, err = svc.UnpackAndProcessBundle(ctx, p.ID, v.ID.String(), archive, false)
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrZipLayoutInvalid))
}
