package service

import (
	"bytes"
	"context"
	"errors"
	"io/fs"
	"path/filepath"
	"testing"

	"github.com/Kirizu-Official/KiriVers/internal/model"
	"github.com/Kirizu-Official/KiriVers/internal/repository"
	"github.com/Kirizu-Official/KiriVers/internal/storage"
	"github.com/Kirizu-Official/KiriVers/pkg/hashutil"
)

// setupReuseService 与 setupTestService 相同，但额外返回存储根目录，
// 供「存储对象数不增加」断言（验收 1）递归计数文件。
func setupReuseService(t *testing.T) (*ProjectService, repository.ProjectStore, storage.Backend, string) {
	t.Helper()
	store := repository.NewMemoryProjectStore()
	tempDir := t.TempDir()
	backend, err := storage.NewLocalFS(tempDir)
	if err != nil {
		t.Fatalf("setup storage: %v", err)
	}
	svc := NewProjectService(store, backend)
	return svc, store, backend, tempDir
}

// countStorageObjects 递归统计存储根目录下的对象文件数（LocalFS 一对象一文件）。
func countStorageObjects(root string) int {
	n := 0
	_ = filepath.WalkDir(root, func(_ string, d fs.DirEntry, err error) error {
		if err == nil && d.Type().IsRegular() {
			n++
		}
		return nil
	})
	return n
}

// TestReuseArtifacts_ZeroCopySingleFile 验收 1：复用产物的新 Version 仅增引用行，
// 存储对象数不增加；FileName 按新版本重算；目标线自动 ready。
func TestReuseArtifacts_ZeroCopySingleFile(t *testing.T) {
	svc, store, _, root := setupReuseService(t)
	ctx := context.Background()

	slug := "app-one"
	p, _, err := svc.Create(ctx, CreateProjectInput{DefaultLocale: ptr("en"), Slug: &slug})
	if err != nil {
		t.Fatalf("create project: %v", err)
	}

	if _, _, err := svc.PutVersion(ctx, p.ID, "1.0.0", VersionWriteInput{Channel: "stable"}); err != nil {
		t.Fatalf("put source version: %v", err)
	}
	content := []byte("hello reuse payload 0123456789")
	src, err := svc.UploadArtifact(ctx, p.Slug, "1.0.0", "windows", "x86_64", UploadArtifactInput{
		Filename: "app.exe",
		Size:     int64(len(content)),
	}, bytes.NewReader(content))
	if err != nil {
		t.Fatalf("upload source artifact: %v", err)
	}

	objectsBefore := countStorageObjects(root)
	if objectsBefore != 1 {
		t.Fatalf("expected exactly 1 storage object after upload, got %d", objectsBefore)
	}

	// 目标：新 Draft 版本 2.0.0（stable 无后缀合法）。
	if _, _, err := svc.PutVersion(ctx, p.ID, "2.0.0", VersionWriteInput{Channel: "stable"}); err != nil {
		t.Fatalf("put target version: %v", err)
	}
	artID := src.ID
	result, err := svc.ReuseArtifacts(ctx, p.Slug, "2.0.0", "windows", "x86_64", ReuseArtifactsInput{
		OS:     "windows",
		Arch:   "x86_64",
		Source: ReuseSource{ArtifactID: &artID},
	})
	if err != nil {
		t.Fatalf("reuse artifacts: %v", err)
	}
	if len(result.Artifacts) != 1 {
		t.Fatalf("expected 1 reused artifact row, got %d", len(result.Artifacts))
	}
	reused := result.Artifacts[0]

	// 验收 1：存储对象数不变（仅引用行增加）。
	if got := countStorageObjects(root); got != objectsBefore {
		t.Fatalf("storage object count changed after reuse: before=%d after=%d", objectsBefore, got)
	}
	// 引用同一对象。
	if reused.StorageKey != src.StorageKey {
		t.Fatalf("storage key must be referenced, not copied: %s != %s", reused.StorageKey, src.StorageKey)
	}
	if reused.SHA256 != src.SHA256 || reused.MD5 != src.MD5 || reused.Size != src.Size {
		t.Fatalf("hash/size metadata must reference source: %+v", reused)
	}
	// FileName 按新版本重算（§5.9）。
	if reused.FileName != "app-one-2.0.0-windows-x86_64.exe" {
		t.Fatalf("unexpected recomputed file name: %s", reused.FileName)
	}
	if reused.VersionID == src.VersionID {
		t.Fatalf("reused row must belong to the target version")
	}

	// 目标线自动就绪。
	lines, err := store.ListVersionLines(ctx, reused.VersionID)
	if err != nil || len(lines) != 1 {
		t.Fatalf("list target lines: %v", err)
	}
	if lines[0].Status != model.VersionLineStatusReady {
		t.Fatalf("target line must be ready after reuse: %s", lines[0].Status)
	}

	// 引用行真实可读：经下载元数据读取到源对象字节。
	dl, err := svc.GetArtifactDownload(ctx, p.Slug, reused.SHA256, "")
	if err != nil {
		t.Fatalf("download reused artifact: %v", err)
	}
	rc, err := svc.Storage().Get(ctx, dl.StorageKey)
	if err != nil {
		t.Fatalf("open shared object: %v", err)
	}
	defer rc.Close()
	got := new(bytes.Buffer)
	if _, err := got.ReadFrom(rc); err != nil {
		t.Fatalf("read shared object: %v", err)
	}
	if !bytes.Equal(got.Bytes(), content) {
		t.Fatalf("shared object content mismatch")
	}
}

// TestReuseArtifacts_Sha256Ambiguous sha256 引用在项目内命中多个 kind=full 产物
// 时必须 400 要求 artifact_id（C14-1）。
func TestReuseArtifacts_Sha256Ambiguous(t *testing.T) {
	svc, _, _, _ := setupReuseService(t)
	ctx := context.Background()

	slug := "ambig"
	p, _, err := svc.Create(ctx, CreateProjectInput{DefaultLocale: ptr("en"), Slug: &slug})
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	content := []byte("same bytes everywhere")
	if _, _, err := svc.PutVersion(ctx, p.ID, "1.0.0", VersionWriteInput{Channel: "stable"}); err != nil {
		t.Fatalf("put v1: %v", err)
	}
	if _, _, err := svc.PutVersion(ctx, p.ID, "1.1.0", VersionWriteInput{Channel: "stable"}); err != nil {
		t.Fatalf("put v2: %v", err)
	}
	for _, ref := range []string{"1.0.0", "1.1.0"} {
		if _, err := svc.UploadArtifact(ctx, p.Slug, ref, "windows", "x86_64", UploadArtifactInput{
			Filename: "app.exe",
			Size:     int64(len(content)),
		}, bytes.NewReader(content)); err != nil {
			t.Fatalf("upload %s: %v", ref, err)
		}
	}

	if _, _, err := svc.PutVersion(ctx, p.ID, "2.0.0", VersionWriteInput{Channel: "stable"}); err != nil {
		t.Fatalf("put target: %v", err)
	}
	sha, _ := hashutil.SHA256Hex(bytes.NewReader(content))
	_, err = svc.ReuseArtifacts(ctx, p.Slug, "2.0.0", "windows", "x86_64", ReuseArtifactsInput{
		OS:     "windows",
		Arch:   "x86_64",
		Source: ReuseSource{SHA256: sha},
	})
	if !errors.Is(err, ErrReuseAmbiguousSHA256) {
		t.Fatalf("expected ErrReuseAmbiguousSHA256, got %v", err)
	}
}

// TestReuseArtifacts_Sha256UniqueHit 唯一命中时按 sha256 复用成功。
func TestReuseArtifacts_Sha256UniqueHit(t *testing.T) {
	svc, _, _, _ := setupReuseService(t)
	ctx := context.Background()

	slug := "sha-hit"
	p, _, err := svc.Create(ctx, CreateProjectInput{DefaultLocale: ptr("en"), Slug: &slug})
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	content := []byte("unique payload")
	if _, _, err := svc.PutVersion(ctx, p.ID, "1.0.0", VersionWriteInput{Channel: "stable"}); err != nil {
		t.Fatalf("put v1: %v", err)
	}
	src, err := svc.UploadArtifact(ctx, p.Slug, "1.0.0", "windows", "x86_64", UploadArtifactInput{
		Filename: "app.exe",
		Size:     int64(len(content)),
	}, bytes.NewReader(content))
	if err != nil {
		t.Fatalf("upload: %v", err)
	}
	if _, _, err := svc.PutVersion(ctx, p.ID, "2.0.0", VersionWriteInput{Channel: "stable"}); err != nil {
		t.Fatalf("put target: %v", err)
	}
	sha, _ := hashutil.SHA256Hex(bytes.NewReader(content))
	result, err := svc.ReuseArtifacts(ctx, p.Slug, "2.0.0", "windows", "x86_64", ReuseArtifactsInput{
		OS:     "windows",
		Arch:   "x86_64",
		Source: ReuseSource{SHA256: sha},
	})
	if err != nil {
		t.Fatalf("reuse by sha256: %v", err)
	}
	if result.Artifacts[0].StorageKey != src.StorageKey {
		t.Fatalf("sha256 reuse must reference source object")
	}
}

// TestReuseArtifacts_RevokedSourceRejected 吊销版本上的对象禁止复用（§5.1）。
func TestReuseArtifacts_RevokedSourceRejected(t *testing.T) {
	svc, _, _, _ := setupReuseService(t)
	ctx := context.Background()

	slug := "rev-src"
	p, _, err := svc.Create(ctx, CreateProjectInput{DefaultLocale: ptr("en"), Slug: &slug})
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	if _, _, err := svc.PutVersion(ctx, p.ID, "1.0.0", VersionWriteInput{Channel: "stable"}); err != nil {
		t.Fatalf("put v1: %v", err)
	}
	content := []byte("revoked bytes")
	src, err := svc.UploadArtifact(ctx, p.Slug, "1.0.0", "windows", "x86_64", UploadArtifactInput{
		Filename: "app.exe",
		Size:     int64(len(content)),
	}, bytes.NewReader(content))
	if err != nil {
		t.Fatalf("upload: %v", err)
	}
	if _, err := svc.RevokeVersion(ctx, p.ID, "1.0.0"); err != nil {
		t.Fatalf("revoke: %v", err)
	}
	if _, _, err := svc.PutVersion(ctx, p.ID, "2.0.0", VersionWriteInput{Channel: "stable"}); err != nil {
		t.Fatalf("put target: %v", err)
	}
	artID := src.ID
	_, err = svc.ReuseArtifacts(ctx, p.Slug, "2.0.0", "windows", "x86_64", ReuseArtifactsInput{
		OS:     "windows",
		Arch:   "x86_64",
		Source: ReuseSource{ArtifactID: &artID},
	})
	if !errors.Is(err, ErrReuseSourceRevoked) {
		t.Fatalf("expected ErrReuseSourceRevoked, got %v", err)
	}
}

// TestReuseArtifacts_PackageTypeMismatch 源/目标形态不一致 → 409
// PACKAGE_TYPE_IMMUTABLE（形态属于 Version Line，§6）。
func TestReuseArtifacts_PackageTypeMismatch(t *testing.T) {
	svc, store, _, _ := setupReuseService(t)
	ctx := context.Background()

	slug := "shape"
	p, _, err := svc.Create(ctx, CreateProjectInput{DefaultLocale: ptr("en"), Slug: &slug})
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	if _, _, err := svc.PutVersion(ctx, p.ID, "1.0.0", VersionWriteInput{Channel: "stable"}); err != nil {
		t.Fatalf("put v1: %v", err)
	}
	content := []byte("single-file payload")
	src, err := svc.UploadArtifact(ctx, p.Slug, "1.0.0", "windows", "x86_64", UploadArtifactInput{
		Filename: "app.exe",
		Size:     int64(len(content)),
	}, bytes.NewReader(content))
	if err != nil {
		t.Fatalf("upload: %v", err)
	}

	// 目标 (os,arch) 为 multi_file 形态。
	if _, _, err := svc.PutVersion(ctx, p.ID, "2.0.0", VersionWriteInput{Channel: "stable"}); err != nil {
		t.Fatalf("put target: %v", err)
	}
	if err := store.CreateMatrix(ctx, &model.PlatformMatrix{
		ProjectID:   p.ID,
		OS:          "macos",
		Arch:        "arm64",
		PackageType: model.PackageTypeMultiFile,
	}); err != nil {
		t.Fatalf("create matrix: %v", err)
	}
	artID := src.ID
	_, err = svc.ReuseArtifacts(ctx, p.Slug, "2.0.0", "macos", "arm64", ReuseArtifactsInput{
		OS:     "macos",
		Arch:   "arm64",
		Source: ReuseSource{ArtifactID: &artID},
	})
	if !errors.Is(err, ErrPackageTypeImmutable) {
		t.Fatalf("expected ErrPackageTypeImmutable, got %v", err)
	}
}

// TestReuseArtifacts_PublishedLineSemantics 已发布线既有产物不可变
// （ARTIFACT_IMMUTABLE）；已发布版本的新增线允许复用（补平台）。
func TestReuseArtifacts_PublishedLineSemantics(t *testing.T) {
	svc, _, _, root := setupReuseService(t)
	ctx := context.Background()

	slug := "pub-line"
	p, _, err := svc.Create(ctx, CreateProjectInput{DefaultLocale: ptr("en"), Slug: &slug})
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	if _, _, err := svc.PutVersion(ctx, p.ID, "1.0.0", VersionWriteInput{Channel: "stable"}); err != nil {
		t.Fatalf("put v1: %v", err)
	}
	content := []byte("published payload")
	src, err := svc.UploadArtifact(ctx, p.Slug, "1.0.0", "windows", "x86_64", UploadArtifactInput{
		Filename: "app.exe",
		Size:     int64(len(content)),
	}, bytes.NewReader(content))
	if err != nil {
		t.Fatalf("upload: %v", err)
	}
	if _, err := svc.PublishVersion(ctx, p.ID, "1.0.0"); err != nil {
		t.Fatalf("publish: %v", err)
	}

	objectsBefore := countStorageObjects(root)
	// 已存在且已有产物的 Published 线禁止再复用。
	artID := src.ID
	if _, err := svc.ReuseArtifacts(ctx, p.Slug, "1.0.0", "windows", "x86_64", ReuseArtifactsInput{
		OS:     "windows",
		Arch:   "x86_64",
		Source: ReuseSource{ArtifactID: &artID},
	}); !errors.Is(err, ErrArtifactImmutable) {
		t.Fatalf("expected ErrArtifactImmutable on published existing line, got %v", err)
	}

	// 已发布版本的新增线（补平台）允许复用。
	if _, err := svc.ReuseArtifacts(ctx, p.Slug, "1.0.0", "linux", "x86_64", ReuseArtifactsInput{
		OS:     "linux",
		Arch:   "x86_64",
		Source: ReuseSource{ArtifactID: &artID},
	}); err != nil {
		t.Fatalf("reuse onto new line of published version: %v", err)
	}
	if got := countStorageObjects(root); got != objectsBefore {
		t.Fatalf("storage object count must not change: before=%d after=%d", objectsBefore, got)
	}
}

// TestReuseArtifacts_MultiFileClonesManifest 多文件线复用：Manifest 克隆到目标线、
// RootHash 内容一致则相同、全量 zip 对象零拷贝引用、对象数不增。
func TestReuseArtifacts_MultiFileClonesManifest(t *testing.T) {
	svc, store, _, root := setupReuseService(t)
	ctx := context.Background()

	slug := "multi-reuse"
	p, _, err := svc.Create(ctx, CreateProjectInput{DefaultLocale: ptr("en"), Slug: &slug})
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	if err := store.CreateMatrix(ctx, &model.PlatformMatrix{
		ProjectID:   p.ID,
		OS:          "windows",
		Arch:        "x86_64",
		PackageType: model.PackageTypeMultiFile,
	}); err != nil {
		t.Fatalf("create matrix: %v", err)
	}

	if _, _, err := svc.PutVersion(ctx, p.ID, "1.0.0", VersionWriteInput{Channel: "stable"}); err != nil {
		t.Fatalf("put v1: %v", err)
	}
	if _, err := svc.AddVersionLine(ctx, p.ID, "1.0.0", VersionLineWriteInput{OS: "windows", Arch: "x86_64"}); err != nil {
		t.Fatalf("add source line: %v", err)
	}
	files := map[string][]byte{
		"bin/app.exe": []byte("binary"),
		"docs/readme": []byte("readme"),
	}
	srcArt, srcManifest, err := svc.BuildArchiveFromFiles(ctx, p.Slug, "1.0.0", "windows", "x86_64", files)
	if err != nil {
		t.Fatalf("build source archive: %v", err)
	}

	objectsBefore := countStorageObjects(root)
	if _, _, err := svc.PutVersion(ctx, p.ID, "2.0.0", VersionWriteInput{Channel: "stable"}); err != nil {
		t.Fatalf("put target: %v", err)
	}
	if _, err := svc.AddVersionLine(ctx, p.ID, "2.0.0", VersionLineWriteInput{OS: "windows", Arch: "x86_64"}); err != nil {
		t.Fatalf("add target line: %v", err)
	}
	srcID := srcArt.ID
	result, err := svc.ReuseArtifacts(ctx, p.Slug, "2.0.0", "windows", "x86_64", ReuseArtifactsInput{
		OS:     "windows",
		Arch:   "x86_64",
		Source: ReuseSource{ArtifactID: &srcID},
	})
	if err != nil {
		t.Fatalf("reuse multi-file: %v", err)
	}

	// 对象数不变：zip 对象零拷贝。
	if got := countStorageObjects(root); got != objectsBefore {
		t.Fatalf("storage object count must not change: before=%d after=%d", objectsBefore, got)
	}
	// Manifest 克隆且 RootHash 相同（内容一致）。
	if result.Manifest == nil {
		t.Fatalf("multi-file reuse must return manifest result")
	}
	if result.Manifest.RootHash != srcManifest.RootHash {
		t.Fatalf("root hash mismatch: %s != %s", result.Manifest.RootHash, srcManifest.RootHash)
	}
	if result.Manifest.Count != len(files) {
		t.Fatalf("manifest count mismatch: %d != %d", result.Manifest.Count, len(files))
	}
	targetEntries, err := store.ListManifestEntries(ctx, result.Artifacts[0].VersionLineID)
	if err != nil || len(targetEntries) != len(files) {
		t.Fatalf("target manifest entries: %v", err)
	}
	for _, e := range targetEntries {
		if e.VersionLineID != result.Artifacts[0].VersionLineID {
			t.Fatalf("cloned entry not attached to target line")
		}
	}
	// 全量 zip 引用行：同 StorageKey、同哈希。
	if result.Artifacts[0].StorageKey != srcArt.StorageKey {
		t.Fatalf("full zip must be referenced, not copied")
	}
	if result.Artifacts[0].SHA256 != srcArt.SHA256 {
		t.Fatalf("full zip sha256 must match source")
	}
	// 目标线 RootHash 与就绪状态。
	lines, _ := store.ListVersionLines(ctx, result.Artifacts[0].VersionID)
	if lines[0].RootHash != srcManifest.RootHash || lines[0].Status != model.VersionLineStatusReady {
		t.Fatalf("target line must carry root hash and be ready: %+v", lines[0])
	}
}
