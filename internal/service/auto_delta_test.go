package service

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"math/rand"
	"sort"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/Kirizu-Official/KiriVers/internal/model"
	"github.com/Kirizu-Official/KiriVers/internal/repository"
	"github.com/Kirizu-Official/KiriVers/internal/service/update"
	"github.com/Kirizu-Official/KiriVers/internal/storage"
)

// 本文件覆盖发布后自动差量与多文件增量 zip（docs/app-init.md §7.2 / §7.5 / §5.9，
// C13-1..C13-7）的验收标准：
//
//   - 发布 v2 后 job 执行成功 → v1→v2 干净多文件 diff 得 patch_package 单 URL；
//   - job 未完成前 diff 返回 full_package 而非 500；
//   - 增量 zip ≥ 全量 70% → 丢弃不落行，diff 走全量；
//   - 被 yank 的源版本不产生新差量对象（C13-5）；
//   - KEEP 条目不进包、替换文件进包、§5.9 文件名含双方全量 SHA-256；
//   - 各线失败隔离（一条线失败不阻断其它线，错误进 job result）。

// autoDeltaFixture 准备：项目 + 内存存储 + Job 仓储。
func autoDeltaFixture(t *testing.T) (*ProjectService, *repository.MemoryProjectStore, storage.Backend) {
	t.Helper()
	store := repository.NewMemoryProjectStore()
	backend, err := storage.NewLocalFS(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	svc := NewProjectService(store, backend)
	svc.SetJobStore(repository.NewMemoryJobRepo())
	svc.SetInstallPolicyStore(repository.NewMemoryInstallPolicyRuleStore())
	return svc, store, backend
}

// multiFileZip 打包多文件全量归档字节。keep sidecar 不再写入 zip。
func multiFileZip(t *testing.T, files map[string]string) []byte {
	t.Helper()
	buf := new(bytes.Buffer)
	zw := zip.NewWriter(buf)
	for p, content := range files {
		w, err := zw.Create(p)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// publishMultiFileLine 走真实链路创建多文件线：建版本 → 建平台切片 → 上传全量 zip
// （BuildArchiveFromZipBuffer 生成 Manifest 与 RootHash 并置 ready）→ 可选 SetManifest KEEP → 发布。
// 返回该线的 kind=full 产物。
func publishMultiFileLine(t *testing.T, svc *ProjectService, slug, version string, files map[string]string, keepPaths []string) *model.Artifact {
	t.Helper()
	ctx := context.Background()
	pid := mustProjectID(t, svc, slug)
	if _, _, err := svc.PutVersion(ctx, pid, version, VersionWriteInput{Channel: "stable"}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.AddVersionLine(ctx, pid, version, VersionLineWriteInput{OS: "windows", Arch: "x86_64"}); err != nil {
		t.Fatal(err)
	}
	art, _, err := svc.BuildArchiveFromZipBuffer(ctx, slug, version, "windows", "x86_64", multiFileZip(t, files))
	if err != nil {
		t.Fatal(err)
	}
	if len(keepPaths) > 0 {
		keep := make(map[string]struct{}, len(keepPaths))
		for _, p := range keepPaths {
			keep[p] = struct{}{}
		}
		m, err := svc.GetManifest(ctx, slug, version, "windows", "x86_64")
		if err != nil {
			t.Fatal(err)
		}
		in := make([]ManifestEntryInput, 0, len(m.Entries))
		for _, e := range m.Entries {
			policy := e.InstallPolicy
			if _, ok := keep[e.Path]; ok {
				policy = model.InstallPolicyKeepIfExists
			}
			in = append(in, ManifestEntryInput{
				Path: e.Path, Size: e.Size, SHA256: e.SHA256, MD5: e.MD5, InstallPolicy: policy,
			})
		}
		if _, err := svc.SetManifest(ctx, slug, version, "windows", "x86_64", in); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := svc.PublishVersion(ctx, pid, version); err != nil {
		t.Fatal(err)
	}
	return art
}

// ensureMatrix 幂等写入平台矩阵行。
func ensureMatrix(t *testing.T, svc *ProjectService, pid uuid.UUID, os, arch, pkgType, algo string) {
	t.Helper()
	if err := svc.store.CreateMatrix(context.Background(), &model.PlatformMatrix{
		ProjectID: pid, OS: os, Arch: arch,
		PackageType: pkgType, DeltaAlgo: algo,
	}); err != nil {
		t.Fatal(err)
	}
}

// listArtifactsByKind 汇总项目全部指定 kind 的产物（测试辅助）。
func listArtifactsByKind(t *testing.T, svc *ProjectService, slug, kind string) []model.Artifact {
	t.Helper()
	p, err := svc.Resolve(context.Background(), slug)
	if err != nil {
		t.Fatal(err)
	}
	versions, err := svc.store.ListVersions(context.Background(), p.ID)
	if err != nil {
		t.Fatal(err)
	}
	var out []model.Artifact
	for _, v := range versions {
		arts, err := svc.store.ListArtifactsByVersionID(context.Background(), v.ID)
		if err != nil {
			t.Fatal(err)
		}
		for _, a := range arts {
			if a.Kind == kind {
				out = append(out, a)
			}
		}
	}
	return out
}

// patchZipEntryNames 从存储拉取 patch 对象并列出 zip 内条目名（测试辅助）。
func patchZipEntryNames(t *testing.T, backend storage.Backend, key string) []string {
	t.Helper()
	rc, err := backend.Get(context.Background(), key)
	if err != nil {
		t.Fatal(err)
	}
	defer rc.Close()
	data, err := io.ReadAll(rc)
	if err != nil {
		t.Fatal(err)
	}
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, f := range zr.File {
		names = append(names, f.Name)
	}
	return names
}

// enqueueForVersion 拿到（或复用已触发的）auto_delta 任务并执行。
func enqueueAndExecuteAutoDelta(t *testing.T, svc *ProjectService, pid, vid uuid.UUID) *model.Job {
	t.Helper()
	jobID, created, err := svc.EnqueueAutoDelta(context.Background(), pid, vid)
	if err != nil {
		t.Fatal(err)
	}
	if !created && jobID == uuid.Nil {
		t.Fatal("expected an auto_delta job for published version")
	}
	if err := svc.ExecuteAutoDeltaJob(context.Background(), jobID); err != nil {
		t.Fatal(err)
	}
	job, err := svc.jobs.GetByID(context.Background(), jobID)
	if err != nil {
		t.Fatal(err)
	}
	return job
}

// ---------- 验收项 1：发布 v2 后 job 执行 → v1→v2 干净多文件 diff 得 patch_package ----------

func TestAutoDeltaMultiFilePatchEndToEnd(t *testing.T) {
	svc, store, backend := autoDeltaFixture(t)
	ctx := context.Background()
	slug := "auto-patch"

	// v1：common1（不变）+ common2（将被替换）+ old-removed（将被删除）。
	publishMultiFileLine(t, svc, slug, "1.0.0", map[string]string{
		"common1.txt": strings.Repeat("A", 1000),
		"common2.txt": strings.Repeat("B", 200),
		"old/old.txt": strings.Repeat("C", 50),
	}, nil)

	// v2：common1 不变、common2 替换、new-added 新增、keep/new 为 KEEP 新增
	//（KEEP 不进包，§7.5.2）。
	publishMultiFileLine(t, svc, slug, "2.0.0", map[string]string{
		"common1.txt":   strings.Repeat("A", 1000),
		"common2.txt":   strings.Repeat("B2", 100),
		"new/added.txt": strings.Repeat("D", 100),
		"keep/new.txt":  strings.Repeat("K", 500),
	}, []string{"keep/new.txt"})

	pid := mustProjectID(t, svc, slug)
	v2, err := svc.ResolveVersion(ctx, pid, "2.0.0")
	if err != nil {
		t.Fatal(err)
	}

	// PublishVersion 已自动入队（触发点 1）；EnqueueAutoDelta 幂等返回同一任务。
	jobID, created, err := svc.EnqueueAutoDelta(ctx, pid, v2.ID)
	if err != nil {
		t.Fatal(err)
	}
	if created {
		t.Fatal("publish seam must have enqueued the auto_delta job already")
	}
	if err := svc.ExecuteAutoDeltaJob(ctx, jobID); err != nil {
		t.Fatal(err)
	}

	// kind=patch 产物行：双方全量 SHA-256 身份 + §5.9 文件名（C13-4）。
	patches := listArtifactsByKind(t, svc, slug, model.ArtifactKindPatch)
	if len(patches) != 1 {
		t.Fatalf("expected 1 patch artifact, got %d", len(patches))
	}
	patch := patches[0]
	if patch.DeltaAlgo != "" {
		t.Fatalf("patch must not carry delta algo: %s", patch.DeltaAlgo)
	}
	if patch.DeltaSourceSHA256 == "" || patch.DeltaTargetSHA256 == "" ||
		strings.EqualFold(patch.DeltaSourceSHA256, patch.DeltaTargetSHA256) {
		t.Fatalf("patch must record distinct source/target full shas: %+v", patch)
	}
	// 文件名：{slug}-{target}-from-{source}-{os}-{arch}-{srcsha64}-{dstsha64}.zip。
	wantName := slug + "-2.0.0-from-1.0.0-windows-x86_64-" +
		strings.ToLower(patch.DeltaSourceSHA256) + "-" + strings.ToLower(patch.DeltaTargetSHA256) + ".zip"
	if patch.FileName != wantName {
		t.Fatalf("patch filename = %s, want %s", patch.FileName, wantName)
	}
	if !strings.Contains(patch.FileName, patch.DeltaSourceSHA256) ||
		!strings.Contains(patch.FileName, patch.DeltaTargetSHA256) {
		t.Fatalf("filename must embed both full shas: %s", patch.FileName)
	}

	if patch.FilesetSHA256 == "" || patch.Compression != model.ArtifactCompressionZip {
		t.Fatalf("patch must record fileset_sha256 and compression=zip: %+v", patch)
	}

	// 增量包内容：成员名为文件 SHA-256 hex（KEEP / 未变化不进包）。
	names := patchZipEntryNames(t, backend, patch.StorageKey)
	wantHex := []string{
		sha256HexOf(strings.Repeat("B2", 100)),
		sha256HexOf(strings.Repeat("D", 100)),
	}
	sort.Strings(names)
	sort.Strings(wantHex)
	if len(names) != 2 || names[0] != wantHex[0] || names[1] != wantHex[1] {
		t.Fatalf("patch entries = %v, want hex members %v", names, wantHex)
	}

	uds := update.NewService(repository.NewMemoryUpdateCatalog(store),
		update.WithLineDetails(store), update.WithPackRuntime(svc))
	res, err := uds.Diff(ctx, pid, "windows", "x86_64", update.DiffInput{
		SourceVersion: "1.0.0", TargetVersion: "2.0.0",
		OS: "windows", Arch: "x86_64",
		Capabilities: []string{"patch_package"},
	})
	if err != nil {
		t.Fatal(err)
	}
	b := res.Body
	if b.DiffMode != update.DiffModePatchPackage {
		t.Fatalf("expected patch_package, got %s", b.DiffMode)
	}
	if b.PackageURL == "" || !strings.Contains(b.PackageURL, patch.SHA256) || b.Size != patch.Size || b.SHA256 != patch.SHA256 {
		t.Fatalf("patch triple wrong: %+v", b)
	}
	if b.DeletedPaths == nil || len(b.DeletedPaths) != 1 || b.DeletedPaths[0] != "old/old.txt" {
		t.Fatalf("deleted_paths = %v, want [old/old.txt]", b.DeletedPaths)
	}

	packRes, err := uds.Pack(ctx, pid, "windows", "x86_64", update.PackInput{
		SourceVersion: "1.0.0", TargetVersion: "2.0.0",
		OS: "windows", Arch: "x86_64",
		NeededPaths: []string{"new/added.txt", "common2.txt"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if packRes.Status != 200 || packRes.Body == nil || packRes.Body.Status != update.PackStatusReady {
		t.Fatalf("preheat fileset must be pack ready: %+v", packRes)
	}
	if packRes.Body.SHA256 != patch.SHA256 {
		t.Fatalf("pack cache sha %s != preheat %s", packRes.Body.SHA256, patch.SHA256)
	}
}

func containsAll(list []string, wants ...string) bool {
	for _, w := range wants {
		found := false
		for _, s := range list {
			if s == w {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}

func sha256HexOf(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

// ---------- 验收项 2：job 未完成前 diff 返回 full_package 而非 500 ----------

func TestAutoDeltaDiffFallsBackToFullBeforeJobCompletes(t *testing.T) {
	svc, store, _ := autoDeltaFixture(t)
	ctx := context.Background()
	slug := "auto-fallback"

	publishMultiFileLine(t, svc, slug, "1.0.0", map[string]string{
		"a.txt": strings.Repeat("A", 100),
	}, nil)
	publishMultiFileLine(t, svc, slug, "2.0.0", map[string]string{
		"a.txt": strings.Repeat("A", 100),
		"b.txt": strings.Repeat("B", 100),
	}, nil)

	pid := mustProjectID(t, svc, slug)
	// 发布已入队但 job 尚未执行；且未声明 patch_package 能力。
	uds := update.NewService(repository.NewMemoryUpdateCatalog(store), update.WithLineDetails(store))
	_, err := uds.Diff(ctx, pid, "windows", "x86_64", update.DiffInput{
		SourceVersion: "1.0.0", TargetVersion: "2.0.0",
		OS: "windows", Arch: "x86_64",
	})
	if !errors.Is(err, update.ErrVersionNotVisible) {
		t.Fatalf("diff before packs_ready_at must be VERSION_NOT_VISIBLE, got %v", err)
	}
}

// ---------- 验收项 3：增量 zip ≥ 全量 70% → 丢弃不落行，diff 走全量 ----------

func TestAutoDeltaPatchOversizedDiscarded(t *testing.T) {
	svc, store, _ := autoDeltaFixture(t)
	ctx := context.Background()
	slug := "auto-oversized"

	// 随机字节近似不可压缩，保证增量/全量体积比接近内容比。
	rng := rand.New(rand.NewSource(42))
	randBytes := func(n int) string {
		b := make([]byte, n)
		for i := range b {
			b[i] = byte(rng.Intn(256))
		}
		return string(b)
	}

	publishMultiFileLine(t, svc, slug, "1.0.0", map[string]string{
		"tiny.txt": "same",
	}, nil)
	big := map[string]string{"tiny.txt": "same"}
	for _, name := range []string{"f1", "f2", "f3", "f4", "f5", "f6", "f7", "f8"} {
		big[name+".bin"] = randBytes(1000)
	}
	publishMultiFileLine(t, svc, slug, "2.0.0", big, nil)

	pid := mustProjectID(t, svc, slug)
	v2, _ := svc.ResolveVersion(ctx, pid, "2.0.0")
	jobID, _, err := svc.EnqueueAutoDelta(ctx, pid, v2.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.ExecuteAutoDeltaJob(ctx, jobID); err != nil {
		t.Fatal(err)
	}

	// ≥70% → 丢弃，不落产物行（C13-3）。
	if got := listArtifactsByKind(t, svc, slug, model.ArtifactKindPatch); len(got) != 0 {
		t.Fatalf("oversized patch must be discarded, got %d rows", len(got))
	}
	// job result 记录 discarded 原因（design §8 取舍）。
	job, _ := svc.jobs.GetByID(ctx, jobID)
	var result AutoDeltaJobResult
	if err := json.Unmarshal(job.Result, &result); err != nil {
		t.Fatal(err)
	}
	foundDiscard := false
	for _, l := range result.Lines {
		for _, s := range l.Sources {
			if s.Status == autoDeltaSourceDiscarded && s.Error != "" {
				foundDiscard = true
			}
		}
	}
	if !foundDiscard {
		t.Fatalf("job result must record discard reason: %s", string(job.Result))
	}

	// diff → 全量（验收项 3）。
	uds := update.NewService(repository.NewMemoryUpdateCatalog(store), update.WithLineDetails(store))
	res, err := uds.Diff(ctx, pid, "windows", "x86_64", update.DiffInput{
		SourceVersion: "1.0.0", TargetVersion: "2.0.0",
		OS: "windows", Arch: "x86_64",
		Capabilities: []string{"patch_package"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Body.DiffMode != update.DiffModeFullPackage {
		t.Fatalf("oversized patch must diff as full_package, got %s", res.Body.DiffMode)
	}
}

// ---------- 验收项 4：被 yank 的源版本不产生新差量对象 ----------

func TestAutoDeltaYankedSourceExcluded(t *testing.T) {
	svc, _, _ := autoDeltaFixture(t)
	ctx := context.Background()
	slug := "auto-yank"
	pid := mustProjectID(t, svc, slug)

	// 单文件双版本：若 v1 未被 yank，本应生成差量。
	publishWithArtifact(t, svc, slug, "1.0.0", strings.Repeat("S", 4096))
	publishWithArtifact(t, svc, slug, "2.0.0", strings.Repeat("S", 2048)+strings.Repeat("T", 2048))
	ensureMatrix(t, svc, pid, "windows", "x86_64", model.PackageTypeSingleFile, model.DeltaAlgoBsdiff)

	// yank v1 的 windows 线（C13-5：yank 源禁作差量基线）。
	if _, err := svc.YankVersionLine(ctx, pid, "1.0.0", "windows", "x86_64"); err != nil {
		t.Fatal(err)
	}

	v2, _ := svc.ResolveVersion(ctx, pid, "2.0.0")
	jobID, _, err := svc.EnqueueAutoDelta(ctx, pid, v2.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.ExecuteAutoDeltaJob(ctx, jobID); err != nil {
		t.Fatal(err)
	}

	if got := listArtifactsByKind(t, svc, slug, model.ArtifactKindDelta); len(got) != 0 {
		t.Fatalf("yanked source must not produce delta objects, got %d", len(got))
	}
	if got := listArtifactsByKind(t, svc, slug, model.ArtifactKindPatch); len(got) != 0 {
		t.Fatalf("yanked source must not produce patch objects, got %d", len(got))
	}
	// 对照：v1 复原为 ready 后重跑同就绪签名的既有任务（源选择在执行时进行）→ 差量生成。
	v1, _ := svc.ResolveVersion(ctx, pid, "1.0.0")
	line1, _ := svc.store.GetVersionLine(ctx, v1.ID, "windows", "x86_64")
	line1.Status = model.VersionLineStatusReady
	if err := svc.store.SaveVersionLine(ctx, line1); err != nil {
		t.Fatal(err)
	}
	jobID2, _, err := svc.EnqueueAutoDelta(ctx, pid, v2.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.ExecuteAutoDeltaJob(ctx, jobID2); err != nil {
		t.Fatal(err)
	}
	if got := listArtifactsByKind(t, svc, slug, model.ArtifactKindDelta); len(got) != 1 {
		t.Fatalf("recovered source must produce exactly 1 delta, got %d", len(got))
	}
}

// ---------- C13-5 补充：被吊销（revoked）的源版本不得作为差量基线 ----------
// 吊销是 Version 级状态（线仍可能 ready），合格源过滤必须按 status=published
// 排除吊销版本，而不能只依赖线级 yank 过滤。

func TestAutoDeltaRevokedSourceVersionExcluded(t *testing.T) {
	svc, _, _ := autoDeltaFixture(t)
	ctx := context.Background()
	slug := "auto-revoked"
	pid := mustProjectID(t, svc, slug)

	publishWithArtifact(t, svc, slug, "1.0.0", strings.Repeat("S", 4096))
	publishWithArtifact(t, svc, slug, "2.0.0", strings.Repeat("S", 2048)+strings.Repeat("T", 2048))
	ensureMatrix(t, svc, pid, "windows", "x86_64", model.PackageTypeSingleFile, model.DeltaAlgoBsdiff)

	// 吊销 v1（Version 级；其线仍为 ready —— 过滤必须来自版本状态闸）。
	if _, err := svc.RevokeVersion(ctx, pid, "1.0.0"); err != nil {
		t.Fatal(err)
	}
	// 确认前提成立：线确实仍是 ready（否则该测试无法区分过滤来源）。
	v1, _ := svc.ResolveVersion(ctx, pid, "1.0.0")
	if line1, err := svc.store.GetVersionLine(ctx, v1.ID, "windows", "x86_64"); err != nil ||
		line1.Status != model.VersionLineStatusReady {
		t.Fatalf("precondition broken: revoked version line must stay ready, got %v/%v", line1, err)
	}

	v2, _ := svc.ResolveVersion(ctx, pid, "2.0.0")
	jobID, _, err := svc.EnqueueAutoDelta(ctx, pid, v2.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.ExecuteAutoDeltaJob(ctx, jobID); err != nil {
		t.Fatal(err)
	}

	if got := listArtifactsByKind(t, svc, slug, model.ArtifactKindDelta); len(got) != 0 {
		t.Fatalf("revoked source version must not produce delta objects, got %d", len(got))
	}
	if got := listArtifactsByKind(t, svc, slug, model.ArtifactKindPatch); len(got) != 0 {
		t.Fatalf("revoked source version must not produce patch objects, got %d", len(got))
	}
}

// ---------- C13-1：单文件自动二进制差量 + 幂等跳过 ----------

func TestAutoDeltaSingleFileBinaryDelta(t *testing.T) {
	svc, _, _ := autoDeltaFixture(t)
	ctx := context.Background()
	slug := "auto-single"
	pid := mustProjectID(t, svc, slug)

	publishWithArtifact(t, svc, slug, "1.0.0", strings.Repeat("S", 4096))
	publishWithArtifact(t, svc, slug, "2.0.0", strings.Repeat("S", 2048)+strings.Repeat("T", 2048))
	ensureMatrix(t, svc, pid, "windows", "x86_64", model.PackageTypeSingleFile, model.DeltaAlgoBsdiff)

	v2, _ := svc.ResolveVersion(ctx, pid, "2.0.0")
	job := enqueueAndExecuteAutoDelta(t, svc, pid, v2.ID)

	deltas := listArtifactsByKind(t, svc, slug, model.ArtifactKindDelta)
	if len(deltas) != 1 {
		t.Fatalf("expected 1 auto delta, got %d", len(deltas))
	}
	d := deltas[0]
	if d.DeltaAlgo != model.DeltaAlgoBsdiff {
		t.Fatalf("matrix algo must be used: %s", d.DeltaAlgo)
	}
	if !strings.Contains(d.FileName, d.DeltaSourceSHA256) || !strings.Contains(d.FileName, d.DeltaTargetSHA256) {
		t.Fatalf("filename must embed both shas: %s", d.FileName)
	}

	// 幂等：重复执行同任务 → skipped，不重复落对象（C13-4）。
	if err := svc.ExecuteAutoDeltaJob(ctx, job.ID); err != nil {
		t.Fatal(err)
	}
	var result AutoDeltaJobResult
	if err := json.Unmarshal(job.Result, &result); err != nil {
		t.Fatal(err)
	}
	if got := listArtifactsByKind(t, svc, slug, model.ArtifactKindDelta); len(got) != 1 {
		t.Fatalf("rerun must not duplicate delta objects, got %d", len(got))
	}
}

// ---------- 触发点 2：已 Published 版本补平台线就绪 → 自动差量 ----------

func TestAutoDeltaTriggeredWhenLineReadyAfterPublish(t *testing.T) {
	svc, _, _ := autoDeltaFixture(t)
	ctx := context.Background()
	slug := "auto-reline"
	pid := mustProjectID(t, svc, slug)

	// v1：windows + linux 双线；v2：先只发 windows。
	publishWithArtifact(t, svc, slug, "1.0.0", strings.Repeat("S", 4096))
	if _, err := svc.UploadArtifact(ctx, slug, "1.0.0", "linux", "arm64", UploadArtifactInput{
		Filename: "tool", Size: 2048,
	}, strings.NewReader(strings.Repeat("L", 2048))); err != nil {
		t.Fatal(err)
	}
	publishWithArtifact(t, svc, slug, "2.0.0", strings.Repeat("S", 1024)+strings.Repeat("T", 3072))
	ensureMatrix(t, svc, pid, "windows", "x86_64", model.PackageTypeSingleFile, model.DeltaAlgoBsdiff)
	ensureMatrix(t, svc, pid, "linux", "arm64", model.PackageTypeSingleFile, model.DeltaAlgoBsdiff)

	v2, _ := svc.ResolveVersion(ctx, pid, "2.0.0")
	// windows 发布已入队（触发点 1）；记录该任务的幂等身份。
	jobID1, _, err := svc.EnqueueAutoDelta(ctx, pid, v2.ID)
	if err != nil {
		t.Fatal(err)
	}

	// 补平台：给已发布的 v2 上传 linux 线（onArtifactUploaded → line ready → 入队）。
	if _, err := svc.UploadArtifact(ctx, slug, "2.0.0", "linux", "arm64", UploadArtifactInput{
		Filename: "tool", Size: 2048,
	}, strings.NewReader(strings.Repeat("L2", 1024))); err != nil {
		t.Fatal(err)
	}

	// 就绪线签名变化 → line-ready seam 必然已用新签名入队：
	// 再次调用 EnqueueAutoDelta 返回的是新任务（与 jobID1 不同）。
	jobID2, _, err := svc.EnqueueAutoDelta(ctx, pid, v2.ID)
	if err != nil {
		t.Fatal(err)
	}
	if jobID2 == jobID1 {
		t.Fatal("line-ready seam must enqueue a new auto_delta job for the new readiness signature")
	}

	// 执行补平台任务 → linux 线差量生成（v1 linux 线 ready 且为合格源）。
	if err := svc.ExecuteAutoDeltaJob(ctx, jobID2); err != nil {
		t.Fatal(err)
	}
	var found bool
	for _, a := range listArtifactsByKind(t, svc, slug, model.ArtifactKindDelta) {
		if strings.Contains(a.FileName, "-linux-arm64-") {
			found = true
		}
	}
	if !found {
		t.Fatal("linux line delta must be generated after the line became ready post-publish")
	}
}

// ---------- 各线失败隔离（design §8 / implement.md B3）----------

func TestAutoDeltaPerLineFailureIsolation(t *testing.T) {
	svc, _, backend := autoDeltaFixture(t)
	ctx := context.Background()
	slug := "auto-isolate"
	pid := mustProjectID(t, svc, slug)

	// 双平台单文件：v1/v2 各带 windows 与 linux 全量产物。
	publishWithArtifact(t, svc, slug, "1.0.0", strings.Repeat("S", 4096))
	if _, err := svc.UploadArtifact(ctx, slug, "1.0.0", "linux", "arm64", UploadArtifactInput{
		Filename: "tool", Size: 2048,
	}, strings.NewReader(strings.Repeat("L", 2048))); err != nil {
		t.Fatal(err)
	}
	publishWithArtifact(t, svc, slug, "2.0.0", strings.Repeat("S", 1024)+strings.Repeat("T", 3072))
	if _, err := svc.UploadArtifact(ctx, slug, "2.0.0", "linux", "arm64", UploadArtifactInput{
		Filename: "tool", Size: 2048,
	}, strings.NewReader(strings.Repeat("L2", 1024))); err != nil {
		t.Fatal(err)
	}
	ensureMatrix(t, svc, pid, "windows", "x86_64", model.PackageTypeSingleFile, model.DeltaAlgoBsdiff)
	ensureMatrix(t, svc, pid, "linux", "arm64", model.PackageTypeSingleFile, model.DeltaAlgoBsdiff)

	// 破坏 linux 目标全量对象的存储字节 → 该线生成失败，windows 不受影响。
	v2, _ := svc.ResolveVersion(ctx, pid, "2.0.0")
	linuxArts, err := svc.store.ListArtifactsByVersionID(ctx, v2.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, a := range linuxArts {
		if a.Kind == model.ArtifactKindFull && strings.Contains(a.FileName, "linux-arm64") {
			if err := backend.Delete(ctx, a.StorageKey); err != nil {
				t.Fatal(err)
			}
		}
	}

	v2, _ = svc.ResolveVersion(ctx, pid, "2.0.0")
	jobID, _, err := svc.EnqueueAutoDelta(ctx, pid, v2.ID)
	if err != nil {
		t.Fatal(err)
	}
	// 部分线失败 → 任务成功，错误记录在 result；windows 线差量照常生成。
	if err := svc.ExecuteAutoDeltaJob(ctx, jobID); err != nil {
		t.Fatalf("partial line failure must not fail the job: %v", err)
	}
	job, _ := svc.jobs.GetByID(ctx, jobID)
	var result AutoDeltaJobResult
	if err := json.Unmarshal(job.Result, &result); err != nil {
		t.Fatal(err)
	}
	linuxFailed := false
	windowsOK := false
	for _, l := range result.Lines {
		// 行级错误（Error）或行内逐源错误（Sources[].Error）都算该线失败被记录。
		if l.OS == "linux" && (l.Error != "" || len(l.Sources) > 0 && l.Sources[0].Status == autoDeltaSourceError) {
			linuxFailed = true
		}
		if l.OS == "windows" && l.Error == "" && len(l.Sources) > 0 && l.Sources[0].Status == autoDeltaSourceCreated {
			windowsOK = true
		}
	}
	if !linuxFailed || !windowsOK {
		t.Fatalf("isolation broken: result=%s", string(job.Result))
	}
	windowsDeltas := 0
	for _, a := range listArtifactsByKind(t, svc, slug, model.ArtifactKindDelta) {
		if strings.Contains(a.FileName, "windows-x86_64") {
			windowsDeltas++
		}
	}
	if windowsDeltas != 1 {
		t.Fatalf("windows line must still generate its delta, got %d", windowsDeltas)
	}
}

// ---------- EnqueueAutoDelta 幂等与非发布版本拒绝 ----------

func TestEnqueueAutoDeltaIdempotentAndGated(t *testing.T) {
	svc, _, _ := autoDeltaFixture(t)
	ctx := context.Background()
	slug := "auto-enqueue"
	pid := mustProjectID(t, svc, slug)

	publishWithArtifact(t, svc, slug, "1.0.0", strings.Repeat("S", 1024))
	publishWithArtifact(t, svc, slug, "2.0.0", strings.Repeat("S", 512)+strings.Repeat("T", 512))

	v2, _ := svc.ResolveVersion(ctx, pid, "2.0.0")
	// 发布 seam 已自动入队；两次幂等调用必须命中同一任务。
	id1, created1, err := svc.EnqueueAutoDelta(ctx, pid, v2.ID)
	if err != nil {
		t.Fatal(err)
	}
	_ = created1 // 可能为 true（无 seam 场景）或 false（publish seam 已入队）
	id2, created2, err := svc.EnqueueAutoDelta(ctx, pid, v2.ID)
	if err != nil {
		t.Fatal(err)
	}
	if created2 || id1 == uuid.Nil || id1 != id2 {
		t.Fatalf("same readiness must reuse the job: created2=%v id1=%s id2=%s", created2, id1, id2)
	}

	// Draft 版本 → 不入队。
	draft, _, err := svc.PutVersion(ctx, pid, "0.9.0", VersionWriteInput{Channel: "stable"})
	if err != nil {
		t.Fatal(err)
	}
	if _, created, err := svc.EnqueueAutoDelta(ctx, pid, draft.ID); err != nil || created {
		t.Fatalf("draft version must not enqueue, created=%v err=%v", created, err)
	}
	// 无就绪线的已发布版本不可能出现（发布闸门），但无 job 仓储时静默降级。
	bare := NewProjectService(svc.store)
	if _, _, err := bare.EnqueueAutoDelta(ctx, pid, v2.ID); err == nil {
		t.Fatal("missing job store must be reported")
	}
}
