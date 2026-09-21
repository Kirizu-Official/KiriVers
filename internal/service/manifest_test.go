package service

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Kirizu-Official/KiriVers/internal/model"
	"github.com/Kirizu-Official/KiriVers/pkg/hashutil"
	"github.com/Kirizu-Official/KiriVers/pkg/pathutil"
)

func TestManifest_SetAndGet(t *testing.T) {
	svc, _, _ := setupTestService(t)
	ctx := context.Background()

	slug := "multi-app"
	engine := "semver"
	p, _, err := svc.Create(ctx, CreateProjectInput{DefaultLocale: ptr("en"), Slug: &slug,
		CompareEngine: &engine,
	})
	require.NoError(t, err)

	_, _, err = svc.PutVersion(ctx, p.ID, "1.0.0", VersionWriteInput{
		Channel:       "stable",
		ChangelogI18n: model.ChangelogMap{"en": {Markdown: "init"}},
	})
	require.NoError(t, err)

	_, err = svc.AddVersionLine(ctx, p.ID, "1.0.0", VersionLineWriteInput{
		OS:   "windows",
		Arch: "x86_64",
	})
	require.NoError(t, err)

	sha1 := "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"
	sha2 := "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad"
	md5Str := "d41d8cd98f00b204e9800998ecf8427e"

	// 1. 设置 Manifest
	entries := []ManifestEntryInput{
		{
			Path:          "bin/app.exe",
			Size:          1024,
			SHA256:        sha1,
			MD5:           md5Str,
			InstallPolicy: "OVERWRITE",
		},
		{
			Path:          "config\\settings.json", // 测试反斜杠自动规范化
			Size:          128,
			SHA256:        sha2,
			MD5:           md5Str,
			InstallPolicy: "KEEP_IF_EXISTS",
		},
	}

	res, err := svc.SetManifest(ctx, p.Slug, "1.0.0", "windows", "x86_64", entries)
	require.NoError(t, err)
	assert.NotEmpty(t, res.RootHash)
	assert.Equal(t, 2, res.Count)

	// 验证 KEEP_IF_EXISTS 的 integrity_check 字段为 false
	for _, e := range res.Entries {
		if e.Path == "config/settings.json" {
			assert.Equal(t, model.InstallPolicyKeepIfExists, e.InstallPolicy)
			assert.False(t, e.IntegrityCheck)
		} else if e.Path == "bin/app.exe" {
			assert.Equal(t, model.InstallPolicyOverwrite, e.InstallPolicy)
			assert.True(t, e.IntegrityCheck)
		}
	}

	// 2. 查询 Manifest
	getRes, err := svc.GetManifest(ctx, p.Slug, "1.0.0", "windows", "x86_64")
	require.NoError(t, err)
	assert.Equal(t, res.RootHash, getRes.RootHash)
	assert.Equal(t, 2, getRes.Count)
}

func TestManifest_InvalidPaths(t *testing.T) {
	svc, _, _ := setupTestService(t)
	ctx := context.Background()

	slug := "multi-app-err"
	engine := "semver"
	p, _, err := svc.Create(ctx, CreateProjectInput{DefaultLocale: ptr("en"), Slug: &slug,
		CompareEngine: &engine,
	})
	require.NoError(t, err)

	_, _, err = svc.PutVersion(ctx, p.ID, "1.0.0", VersionWriteInput{
		Channel: "stable",
	})
	require.NoError(t, err)

	_, err = svc.AddVersionLine(ctx, p.ID, "1.0.0", VersionLineWriteInput{
		OS:   "linux",
		Arch: "x86_64",
	})
	require.NoError(t, err)

	sha1 := "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"
	md5Str := "d41d8cd98f00b204e9800998ecf8427e"

	// 含 .. 的路径 -> INVALID_PATH，不 500
	_, err = svc.SetManifest(ctx, p.Slug, "1.0.0", "linux", "x86_64", []ManifestEntryInput{
		{
			Path:   "../etc/passwd",
			Size:   10,
			SHA256: sha1,
			MD5:    md5Str,
		},
	})
	require.Error(t, err)
	assert.True(t, errors.Is(err, pathutil.ErrInvalidPath))

	// 含盘符的路径 -> INVALID_PATH
	_, err = svc.SetManifest(ctx, p.Slug, "1.0.0", "linux", "x86_64", []ManifestEntryInput{
		{
			Path:   "C:/app.bin",
			Size:   10,
			SHA256: sha1,
			MD5:    md5Str,
		},
	})
	require.Error(t, err)
	assert.True(t, errors.Is(err, pathutil.ErrInvalidPath))

	// 重复路径
	_, err = svc.SetManifest(ctx, p.Slug, "1.0.0", "linux", "x86_64", []ManifestEntryInput{
		{Path: "bin/foo", Size: 10, SHA256: sha1, MD5: md5Str},
		{Path: "bin//foo", Size: 10, SHA256: sha1, MD5: md5Str},
	})
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrDuplicateManifestPath))
}

func TestManifest_MultiFileLine_ReadyAndPublishGates(t *testing.T) {
	svc, store, _ := setupTestService(t)
	ctx := context.Background()

	slug := "multi-gate"
	engine := "semver"
	p, _, err := svc.Create(ctx, CreateProjectInput{DefaultLocale: ptr("en"), Slug: &slug,
		CompareEngine: &engine,
	})
	require.NoError(t, err)

	// 配置平台矩阵为 multi_file
	err = store.CreateMatrix(ctx, &model.PlatformMatrix{
		ProjectID:   p.ID,
		OS:          "windows",
		Arch:        "x86_64",
		PackageType: model.PackageTypeMultiFile,
	})
	require.NoError(t, err)

	// 创建版本与切片
	_, _, err = svc.PutVersion(ctx, p.ID, "1.0.0", VersionWriteInput{
		Channel: "stable",
	})
	require.NoError(t, err)

	line, err := svc.AddVersionLine(ctx, p.ID, "1.0.0", VersionLineWriteInput{
		OS:   "windows",
		Arch: "x86_64",
	})
	require.NoError(t, err)
	assert.Equal(t, model.VersionLineStatusPending, line.Status)

	// 1. 在全量 zip 上传/生成前，尝试将多文件 Line 标为 ready -> 必须被拒绝并返回 ErrArchiveRequired
	_, err = svc.ReadyVersionLine(ctx, p.ID, "1.0.0", "windows", "x86_64")
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrArchiveRequired))

	// 状态依然是 pending
	lineRefreshed, err := svc.GetVersionLine(ctx, p.ID, "1.0.0", "windows", "x86_64")
	require.NoError(t, err)
	assert.Equal(t, model.VersionLineStatusPending, lineRefreshed.Status)

	// 2. 尝试发布版本 -> 必须被拒绝（无就绪切片且多文件切片缺少全量包）
	_, err = svc.PublishVersion(ctx, p.ID, "1.0.0")
	require.Error(t, err)

	// 3. 构建并上传合法的全量 zip 归档包
	files := map[string][]byte{
		"bin/app.exe":          []byte("binary executable content"),
		"config/settings.json": []byte(`{"env":"prod"}`),
	}

	art, manifestRes, err := svc.BuildArchiveFromFiles(ctx, p.Slug, "1.0.0", "windows", "x86_64", files)
	require.NoError(t, err)
	assert.NotNil(t, art)
	assert.NotEmpty(t, manifestRes.RootHash)

	// 此时平台切片状态应已变为 ready
	lineReady, err := svc.GetVersionLine(ctx, p.ID, "1.0.0", "windows", "x86_64")
	require.NoError(t, err)
	assert.Equal(t, model.VersionLineStatusReady, lineReady.Status)
	assert.Equal(t, manifestRes.RootHash, lineReady.RootHash)

	// 4. 此时发布版本能够成功
	pubVer, err := svc.PublishVersion(ctx, p.ID, "1.0.0")
	require.NoError(t, err)
	assert.Equal(t, model.VersionStatusPublished, pubVer.Status)
}

func TestManifest_ZipValidationAgainstManifest(t *testing.T) {
	svc, store, _ := setupTestService(t)
	ctx := context.Background()

	slug := "multi-validate"
	engine := "semver"
	p, _, err := svc.Create(ctx, CreateProjectInput{DefaultLocale: ptr("en"), Slug: &slug,
		CompareEngine: &engine,
	})
	require.NoError(t, err)

	err = store.CreateMatrix(ctx, &model.PlatformMatrix{
		ProjectID:   p.ID,
		OS:          "linux",
		Arch:        "x86_64",
		PackageType: model.PackageTypeMultiFile,
	})
	require.NoError(t, err)

	_, _, err = svc.PutVersion(ctx, p.ID, "1.0.0", VersionWriteInput{
		Channel: "stable",
	})
	require.NoError(t, err)

	_, err = svc.AddVersionLine(ctx, p.ID, "1.0.0", VersionLineWriteInput{
		OS:   "linux",
		Arch: "x86_64",
	})
	require.NoError(t, err)

	file1 := []byte("content 1")
	file2 := []byte("content 2")
	sha1, _ := hashutil.SHA256Hex(bytes.NewReader(file1))
	md51, _ := hashutil.MD5Hex(bytes.NewReader(file1))
	sha2, _ := hashutil.SHA256Hex(bytes.NewReader(file2))
	md52, _ := hashutil.MD5Hex(bytes.NewReader(file2))

	// 先通过 Manifest API 登记条目
	_, err = svc.SetManifest(ctx, p.Slug, "1.0.0", "linux", "x86_64", []ManifestEntryInput{
		{Path: "bin/app", Size: int64(len(file1)), SHA256: sha1, MD5: md51},
		{Path: "data/db.sqlite", Size: int64(len(file2)), SHA256: sha2, MD5: md52},
	})
	require.NoError(t, err)

	// Case 1: 上传的 zip 缺少 data/db.sqlite
	bufMissing := new(bytes.Buffer)
	zw := zip.NewWriter(bufMissing)
	w, _ := zw.Create("bin/app")
	_, _ = w.Write(file1)
	_ = zw.Close()

	_, _, err = svc.BuildArchiveFromZipBuffer(ctx, p.Slug, "1.0.0", "linux", "x86_64", bufMissing.Bytes())
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrZipManifestMismatch))

	// 切片状态依然为 pending
	l, err := svc.GetVersionLine(ctx, p.ID, "1.0.0", "linux", "x86_64")
	require.NoError(t, err)
	assert.Equal(t, model.VersionLineStatusPending, l.Status)

	// Case 2: 上传的 zip 路径完全一致
	bufCorrect := new(bytes.Buffer)
	zw2 := zip.NewWriter(bufCorrect)
	w1, _ := zw2.Create("bin/app")
	_, _ = w1.Write(file1)
	w2, _ := zw2.Create("data/db.sqlite")
	_, _ = w2.Write(file2)
	_ = zw2.Close()

	art, mRes, err := svc.BuildArchiveFromZipBuffer(ctx, p.Slug, "1.0.0", "linux", "x86_64", bufCorrect.Bytes())
	require.NoError(t, err)
	assert.NotNil(t, art)
	assert.Equal(t, 2, mRes.Count)

	// 切片状态就绪
	l2, err := svc.GetVersionLine(ctx, p.ID, "1.0.0", "linux", "x86_64")
	require.NoError(t, err)
	assert.Equal(t, model.VersionLineStatusReady, l2.Status)
}
