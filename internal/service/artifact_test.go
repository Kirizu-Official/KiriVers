package service

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/Kirizu-Official/KiriVers/internal/model"
	"github.com/Kirizu-Official/KiriVers/internal/repository"
	"github.com/Kirizu-Official/KiriVers/internal/storage"
	"github.com/Kirizu-Official/KiriVers/pkg/hashutil"
)

func setupTestService(t *testing.T) (*ProjectService, repository.ProjectStore, storage.Backend) {
	t.Helper()
	store := repository.NewMemoryProjectStore()
	tempDir := t.TempDir()
	backend, err := storage.NewLocalFS(tempDir)
	if err != nil {
		t.Fatalf("setup storage: %v", err)
	}
	svc := NewProjectService(store, backend)
	svc.SetInstallPolicyStore(repository.NewMemoryInstallPolicyRuleStore())
	return svc, store, backend
}

func TestArtifact_DirectUploadAndRange(t *testing.T) {
	svc, store, backend := setupTestService(t)
	ctx := context.Background()

	// 1. 创建项目
	slug := "app-one"
	engine := "semver"
	p, _, err := svc.Create(ctx, CreateProjectInput{DefaultLocale: ptr("en"), Slug: &slug,
		CompareEngine: &engine,
	})
	if err != nil {
		t.Fatalf("create project: %v", err)
	}

	// 2. 创建 Version (Draft)
	channel := "stable"
	v, _, err := svc.PutVersion(ctx, p.ID, "1.0.0", VersionWriteInput{
		Channel:       channel,
		ChangelogI18n: model.ChangelogMap{"en": {Markdown: "init"}},
	})
	if err != nil {
		t.Fatalf("put version: %v", err)
	}

	// 3. 上传小文件产物
	content := []byte("hello world binary payload 1234567890")
	expectedSha, _ := hashutil.SHA256Hex(bytes.NewReader(content))

	art, err := svc.UploadArtifact(ctx, p.Slug, "1.0.0", "windows", "x86_64", UploadArtifactInput{
		Filename:          "app.exe",
		ExpectedSHA256:    expectedSha,
		IdempotencyKey:    "key-1",
		Size:              int64(len(content)),
		ArtifactSignature: "sig-abc",
	}, bytes.NewReader(content))
	if err != nil {
		t.Fatalf("upload artifact: %v", err)
	}

	if art.SHA256 != expectedSha {
		t.Fatalf("sha256 mismatch: %s != %s", art.SHA256, expectedSha)
	}
	if art.Size != int64(len(content)) {
		t.Fatalf("size mismatch: %d != %d", art.Size, len(content))
	}
	if art.FileName != "app-one-1.0.0-windows-x86_64.exe" {
		t.Fatalf("unexpected stable filename: %s", art.FileName)
	}
	if art.ContentType != "application/vnd.microsoft.portable-executable" {
		t.Fatalf("unexpected content type: %s", art.ContentType)
	}
	wantKey := storage.ArtifactObjectKey(p.Slug, expectedSha)
	if art.StorageKey != wantKey {
		t.Fatalf("storage key=%q want %q", art.StorageKey, wantKey)
	}
	if _, _, err := backend.Head(ctx, wantKey); err != nil {
		t.Fatalf("canonical object missing: %v", err)
	}
	if _, _, err := backend.Head(ctx, p.Slug+"/temp/"+art.ID.String()); !errors.Is(err, storage.ErrNotFound) {
		t.Fatalf("public temp key should be absent, err=%v", err)
	}

	// 验证 Line 被自动置为 ready
	lines, err := store.ListVersionLines(ctx, v.ID)
	if err != nil || len(lines) != 1 || lines[0].Status != model.VersionLineStatusReady {
		t.Fatalf("line status not ready: %+v", lines)
	}

	// 验证幂等键再次上传直接返回原产物
	art2, err := svc.UploadArtifact(ctx, p.Slug, "1.0.0", "windows", "x86_64", UploadArtifactInput{
		Filename:       "app.exe",
		IdempotencyKey: "key-1",
	}, bytes.NewReader([]byte("different ignored")))
	if err != nil {
		t.Fatalf("idempotent upload failed: %v", err)
	}
	if art2.ID != art.ID {
		t.Fatalf("idempotency failed: %s != %s", art2.ID, art.ID)
	}

	// 4. 测试下载元数据获取与 Range
	dl, err := svc.GetArtifactDownload(ctx, p.Slug, art.SHA256, "")
	if err != nil {
		t.Fatalf("get artifact download: %v", err)
	}
	if dl.FileName != art.FileName {
		t.Fatalf("filename mismatch: %s != %s", dl.FileName, art.FileName)
	}

	// 读中间字节
	rc, err := svc.Storage().Range(ctx, dl.StorageKey, 6, 10)
	if err != nil {
		t.Fatalf("range read failed: %v", err)
	}
	defer rc.Close()
	midBuf := new(bytes.Buffer)
	_, _ = midBuf.ReadFrom(rc)
	if midBuf.String() != "world" {
		t.Fatalf("unexpected range slice: got %q, want %q", midBuf.String(), "world")
	}

	// 5. 校验哈希不匹配错误
	_, err = svc.UploadArtifact(ctx, p.Slug, "1.0.0", "linux", "x86_64", UploadArtifactInput{
		Filename:       "app.bin",
		ExpectedSHA256: "0000000000000000000000000000000000000000000000000000000000000000",
		Size:           4,
	}, bytes.NewReader([]byte("test")))
	if !errors.Is(err, ErrChecksumMismatch) {
		t.Fatalf("expected ErrChecksumMismatch, got: %v", err)
	}
}

func TestArtifact_PublishedImmutability(t *testing.T) {
	svc, _, _ := setupTestService(t)
	ctx := context.Background()

	slug := "immutable-proj"
	p, _, _ := svc.Create(ctx, CreateProjectInput{DefaultLocale: ptr("en"), Slug: &slug})
	_, _, _ = svc.PutVersion(ctx, p.ID, "1.0.0", VersionWriteInput{Channel: "stable"})

	// 上传产物
	contentA := []byte("version 1.0 content A")
	shaA, _ := hashutil.SHA256Hex(bytes.NewReader(contentA))
	artA, err := svc.UploadArtifact(ctx, p.Slug, "1.0.0", "linux", "x86_64", UploadArtifactInput{
		Filename:       "bin.tar.gz",
		ExpectedSHA256: shaA,
		Size:           int64(len(contentA)),
	}, bytes.NewReader(contentA))
	if err != nil {
		t.Fatalf("upload artifact A: %v", err)
	}

	// 发布版本
	pubV, err := svc.PublishVersion(ctx, p.ID, "1.0.0")
	if err != nil || pubV.Status != model.VersionStatusPublished {
		t.Fatalf("publish version: %v", err)
	}

	// 已发布版本再次上传相同哈希：no-op 成功
	artSame, err := svc.UploadArtifact(ctx, p.Slug, "1.0.0", "linux", "x86_64", UploadArtifactInput{
		Filename:       "bin.tar.gz",
		ExpectedSHA256: shaA,
		Size:           int64(len(contentA)),
	}, bytes.NewReader(contentA))
	if err != nil {
		t.Fatalf("expected no-op success on identical sha, got: %v", err)
	}
	if artSame.ID != artA.ID {
		t.Fatalf("expected same artifact returned, got %s", artSame.ID)
	}

	// 已发布版本上传不同字节：报错 409 ARTIFACT_IMMUTABLE
	contentB := []byte("version 1.0 modified malicious content B")
	shaB, _ := hashutil.SHA256Hex(bytes.NewReader(contentB))
	_, err = svc.UploadArtifact(ctx, p.Slug, "1.0.0", "linux", "x86_64", UploadArtifactInput{
		Filename:       "bin.tar.gz",
		ExpectedSHA256: shaB,
		Size:           int64(len(contentB)),
	}, bytes.NewReader(contentB))
	if !errors.Is(err, ErrArtifactImmutable) {
		t.Fatalf("expected ErrArtifactImmutable, got: %v", err)
	}
}

func TestArtifact_TusResumeAndGate(t *testing.T) {
	svc, _, _ := setupTestService(t)
	ctx := context.Background()

	slug := "tus-proj"
	p, _, _ := svc.Create(ctx, CreateProjectInput{DefaultLocale: ptr("en"), Slug: &slug})
	_, _, _ = svc.PutVersion(ctx, p.ID, "2.0.0", VersionWriteInput{Channel: "stable"})

	fullContent := []byte("part-1-data-AAAA;part-2-data-BBBB;part-3-data-CCCC")
	expectedSha, _ := hashutil.SHA256Hex(bytes.NewReader(fullContent))
	totalSize := int64(len(fullContent))

	// 1. 初始化 TUS 会话
	sess, err := svc.CreateTusUpload(ctx, p.Slug, "2.0.0", "linux", "arm64", CreateTusUploadInput{
		Size:           totalSize,
		Filename:       "firmware.bin",
		ExpectedSHA256: expectedSha,
	})
	if err != nil {
		t.Fatalf("create tus upload: %v", err)
	}
	if sess.Offset != 0 || sess.Size != totalSize {
		t.Fatalf("invalid tus session initial state: %+v", sess)
	}

	// 2. 验证：未完成的 TUS 上传会导致 PublishVersion 触发 UPLOAD_INCOMPLETE (C05-8)
	_, err = svc.PublishVersion(ctx, p.ID, "2.0.0")
	if !errors.Is(err, ErrUploadIncomplete) {
		t.Fatalf("expected ErrUploadIncomplete when TUS is in progress, got: %v", err)
	}

	// 3. 分片 1 上传
	chunk1 := fullContent[:16]
	sess, art, err := svc.WriteTusChunk(ctx, p.Slug, sess.ID, 0, bytes.NewReader(chunk1))
	if err != nil {
		t.Fatalf("write chunk 1: %v", err)
	}
	if sess.Offset != 16 || art != nil {
		t.Fatalf("chunk 1 progress invalid: offset=%d, art=%v", sess.Offset, art)
	}
	head, err := svc.GetTusUpload(ctx, p.Slug, sess.ID)
	if err != nil {
		t.Fatal(err)
	}
	if head.Offset != 16 {
		t.Fatalf("TUS offset should be local file size, got %d", head.Offset)
	}

	// 4. 模拟偏移不匹配 (重试或并发冲突) -> ErrOffsetMismatch
	_, _, err = svc.WriteTusChunk(ctx, p.Slug, sess.ID, 0, bytes.NewReader(chunk1))
	if !errors.Is(err, ErrOffsetMismatch) {
		t.Fatalf("expected ErrOffsetMismatch, got: %v", err)
	}

	// 5. 分片 2 上传
	chunk2 := fullContent[16:33]
	sess, art, err = svc.WriteTusChunk(ctx, p.Slug, sess.ID, 16, bytes.NewReader(chunk2))
	if err != nil {
		t.Fatalf("write chunk 2: %v", err)
	}
	if sess.Offset != 33 || art != nil {
		t.Fatalf("chunk 2 progress invalid: offset=%d", sess.Offset)
	}

	// 6. 分片 3 上传 (完成)
	chunk3 := fullContent[33:]
	sess, art, err = svc.WriteTusChunk(ctx, p.Slug, sess.ID, 33, bytes.NewReader(chunk3))
	if err != nil {
		t.Fatalf("write chunk 3 (finish): %v", err)
	}
	if sess.Status != model.UploadSessionStatusCompleted || art == nil {
		t.Fatalf("expected completed session and artifact, got sess=%+v, art=%v", sess, art)
	}
	if art.SHA256 != expectedSha {
		t.Fatalf("assembled artifact sha mismatch: %s != %s", art.SHA256, expectedSha)
	}

	// 7. 现在已完成，发布版本应该成功
	pubV, err := svc.PublishVersion(ctx, p.ID, "2.0.0")
	if err != nil || pubV.Status != model.VersionStatusPublished {
		t.Fatalf("publish after TUS complete failed: %v", err)
	}
}

func TestArtifact_HardwareRevisionPolicy(t *testing.T) {
	svc, store, _ := setupTestService(t)
	ctx := context.Background()

	slug := "hw-proj"
	p, _, _ := svc.Create(ctx, CreateProjectInput{DefaultLocale: ptr("en"), Slug: &slug})

	// 注册硬件代号
	_ = store.CreateHwRev(ctx, &model.HwRev{ProjectID: p.ID, Slug: "revA", Rank: 10})
	_ = store.CreateHwRev(ctx, &model.HwRev{ProjectID: p.ID, Slug: "revB", Rank: 20})
	_ = store.CreateHwRev(ctx, &model.HwRev{ProjectID: p.ID, Slug: "revC", Rank: 30})

	// 配置平台矩阵策略为 higher_compatible_with_lower
	_ = store.CreateMatrix(ctx, &model.PlatformMatrix{
		ProjectID:       p.ID,
		OS:              "linux",
		Arch:            "arm64",
		PackageType:     "single_file",
		HwVariantPolicy: model.HwVariantHigherCompatible,
	})

	_, _, _ = svc.PutVersion(ctx, p.ID, "1.0.0", VersionWriteInput{Channel: "stable"})

	// 未知硬件代号上传被拒绝
	unreg := "revUnknown"
	_, err := svc.UploadArtifact(ctx, p.Slug, "1.0.0", "linux", "arm64", UploadArtifactInput{
		Filename: "fw.bin",
		HwRev:    &unreg,
		Size:     4,
	}, strings.NewReader("data"))
	if !errors.Is(err, ErrHwRevUnknown) {
		t.Fatalf("expected ErrHwRevUnknown, got: %v", err)
	}

	// 正常上传绑定 revC 产物，设置 min_hw_rev = revB
	revC := "revC"
	minB := "revB"
	art, err := svc.UploadArtifact(ctx, p.Slug, "1.0.0", "linux", "arm64", UploadArtifactInput{
		Filename: "fw.bin",
		HwRev:    &revC,
		MinHwRev: &minB,
		Size:     4,
	}, strings.NewReader("data"))
	if err != nil {
		t.Fatalf("upload hw variant failed: %v", err)
	}

	// 客户端未传 hw_rev 下载此产物：拒绝 HW_REV_INCOMPATIBLE
	_, err = svc.GetArtifactDownload(ctx, p.Slug, art.SHA256, "")
	if err != nil {
		t.Fatalf("hash download must skip HW_REV_INCOMPATIBLE, got: %v", err)
	}

	_, err = svc.GetArtifactDownload(ctx, p.Slug, art.SHA256, "revA")
	if err != nil {
		t.Fatalf("hash download must skip hw_rev query, got: %v", err)
	}

	dlB, err := svc.GetArtifactDownload(ctx, p.Slug, art.SHA256, "revB")
	if err != nil || dlB == nil {
		t.Fatalf("download for revB should succeed, got: %v", err)
	}

	dlC, err := svc.GetArtifactDownload(ctx, p.Slug, art.SHA256, "revC")
	if err != nil || dlC == nil {
		t.Fatalf("download for revC should succeed, got: %v", err)
	}
}

func TestArtifact_Cleanup(t *testing.T) {
	svc, store, _ := setupTestService(t)
	ctx := context.Background()

	slug := "clean-proj"
	p, _, _ := svc.Create(ctx, CreateProjectInput{DefaultLocale: ptr("en"), Slug: &slug})

	// 创建已过期的会话
	sessExpired := &model.UploadSession{
		ID:         uuid.New(),
		ProjectID:  p.ID,
		StorageKey: "uploads/clean/exp",
		Status:     model.UploadSessionStatusUploading,
		ExpiresAt:  time.Now().Add(-48 * time.Hour),
	}
	_ = store.CreateUploadSession(ctx, sessExpired)

	// 创建已中止的会话
	sessAborted := &model.UploadSession{
		ID:         uuid.New(),
		ProjectID:  p.ID,
		StorageKey: "uploads/clean/abort",
		Status:     model.UploadSessionStatusAborted,
		ExpiresAt:  time.Now().Add(24 * time.Hour),
	}
	_ = store.CreateUploadSession(ctx, sessAborted)

	cleaned, err := svc.CleanupArtifacts(ctx, p.Slug, 1) // 1 day retention
	if err != nil {
		t.Fatalf("cleanup failed: %v", err)
	}
	if cleaned != 2 {
		t.Fatalf("expected 2 cleaned sessions, got %d", cleaned)
	}
}

func TestCleanupStalePatchesKeepsFullFeedFullAndSharedKeys(t *testing.T) {
	svc, store, backend := setupTestService(t)
	ctx := context.Background()
	slug := "cleanup-shared"
	p, _, err := svc.Create(ctx, CreateProjectInput{DefaultLocale: ptr("en"), Slug: &slug})
	if err != nil {
		t.Fatal(err)
	}
	osStr, archStr := "windows", "x86_64"
	pkgType := model.PackageTypeSingleFile
	if _, err := svc.CreateMatrix(ctx, p.ID, MatrixWrite{OS: &osStr, Arch: &archStr, PackageType: &pkgType}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := svc.PutVersion(ctx, p.ID, "1.0.0", VersionWriteInput{Channel: "stable"}); err != nil {
		t.Fatal(err)
	}
	content := []byte("full-bytes-keep-me")
	sha, _ := hashutil.SHA256Hex(bytes.NewReader(content))
	art, err := svc.UploadArtifact(ctx, p.Slug, "1.0.0", "windows", "x86_64", UploadArtifactInput{
		Filename: "app.exe", ExpectedSHA256: sha, Size: int64(len(content)),
	}, bytes.NewReader(content))
	if err != nil {
		t.Fatalf("upload: %v", err)
	}
	if _, err := svc.PublishVersion(ctx, p.ID, "1.0.0"); err != nil {
		t.Fatal(err)
	}

	feedKey := storage.ArtifactObjectKey(p.Slug, strings.Repeat("f", 64))
	if err := backend.Put(ctx, feedKey, bytes.NewReader([]byte("feed")), 4, "application/zip"); err != nil {
		t.Fatal(err)
	}
	feedArt := &model.Artifact{
		ID: uuid.New(), ProjectID: p.ID, VersionID: art.VersionID, VersionLineID: art.VersionLineID,
		Kind: model.ArtifactKindStoreFull, FileName: "app-store.zip", StorageKey: feedKey,
		Size: 4, SHA256: strings.Repeat("f", 64), MD5: strings.Repeat("0", 32), ContentType: "application/zip",
	}
	if err := store.CreateArtifact(ctx, feedArt); err != nil {
		t.Fatal(err)
	}

	uniqueKey := storage.ArtifactObjectKey(p.Slug, strings.Repeat("1", 64))
	if err := backend.Put(ctx, uniqueKey, bytes.NewReader([]byte("patch")), 5, "application/zip"); err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-48 * time.Hour)
	staleUnique := &model.Artifact{
		ID: uuid.New(), ProjectID: p.ID, VersionID: art.VersionID, VersionLineID: art.VersionLineID,
		Kind: model.ArtifactKindPatch, FileName: "stale.zip", StorageKey: uniqueKey,
		Size: 5, SHA256: strings.Repeat("1", 64), MD5: strings.Repeat("0", 32), ContentType: "application/zip",
		DeltaSourceSHA256: strings.Repeat("2", 64), DeltaTargetSHA256: strings.Repeat("3", 64),
		CreatedAt: old,
	}
	if err := store.CreateArtifact(ctx, staleUnique); err != nil {
		t.Fatal(err)
	}
	staleShared := &model.Artifact{
		ID: uuid.New(), ProjectID: p.ID, VersionID: art.VersionID, VersionLineID: art.VersionLineID,
		Kind: model.ArtifactKindPatch, FileName: "shared.zip", StorageKey: art.StorageKey,
		Size: art.Size, SHA256: strings.Repeat("4", 64), MD5: strings.Repeat("0", 32), ContentType: "application/zip",
		DeltaSourceSHA256: strings.Repeat("5", 64), DeltaTargetSHA256: strings.Repeat("6", 64),
		CreatedAt: old,
	}
	if err := store.CreateArtifact(ctx, staleShared); err != nil {
		t.Fatal(err)
	}

	cleaned, err := svc.CleanupArtifacts(ctx, p.Slug, 1)
	if err != nil {
		t.Fatalf("cleanup: %v", err)
	}
	if cleaned < 2 {
		t.Fatalf("expected stale patches cleaned, got %d", cleaned)
	}
	if _, err := store.GetArtifactByID(ctx, art.ID); err != nil {
		t.Fatal("full artifact row must remain")
	}
	if _, err := store.GetArtifactByID(ctx, feedArt.ID); err != nil {
		t.Fatal("store_full artifact row must remain")
	}
	if _, err := store.GetArtifactByID(ctx, staleUnique.ID); err == nil {
		t.Fatal("unique stale patch row must be deleted")
	}
	if _, err := store.GetArtifactByID(ctx, staleShared.ID); err == nil {
		t.Fatal("shared-key stale patch row must be deleted")
	}
	if _, _, err := backend.Head(ctx, art.StorageKey); err != nil {
		t.Fatalf("shared full blob must survive: %v", err)
	}
	if _, _, err := backend.Head(ctx, feedKey); err != nil {
		t.Fatalf("store_full blob must survive: %v", err)
	}
}
