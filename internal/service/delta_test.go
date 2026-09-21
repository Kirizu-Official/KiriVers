package service

import (
	"encoding/json"
	"regexp"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/Kirizu-Official/KiriVers/internal/model"
	"github.com/Kirizu-Official/KiriVers/internal/repository"
	"github.com/Kirizu-Official/KiriVers/internal/storage"
	"github.com/Kirizu-Official/KiriVers/pkg/hashutil"
)

// ---------- 稳定文件名构造器（C10-4 / §5.9）----------

const sha64HexPattern = `[0-9a-f]{64}`

// 验收项：差量文件名含完整 64 位源与目标 SHA-256、`-from-` 结构与按算法扩展名。
func TestBuildDeltaStableFilename(t *testing.T) {
	src := strings.Repeat("a", 64)
	dst := strings.Repeat("b", 64)
	name := BuildDeltaStableFilename("demo", "2.0.0", "1.0.0", "windows", "x86_64", nil, "bsdiff", src, dst)
	want := "demo-2.0.0-from-1.0.0-windows-x86_64-bsdiff-" + src + "-" + dst + ".bsdiff"
	if name != want {
		t.Fatalf("filename = %s, want %s", name, want)
	}

	// hw 变体段与各算法扩展名。
	hw := "rev-b"
	if got := BuildDeltaStableFilename("demo", "2", "1", "linux", "arm64", &hw, "hdiffpatch", src, dst); !strings.HasSuffix(got, ".hdiff") || !strings.Contains(got, "-rev-b-") {
		t.Fatalf("hdiffpatch/hw filename wrong: %s", got)
	}
	if got := BuildDeltaStableFilename("demo", "2", "1", "linux", "arm64", nil, "xdelta3", src, dst); !strings.HasSuffix(got, ".vcdiff") {
		t.Fatalf("xdelta3 extension wrong: %s", got)
	}

	// 两个 64 位哈希段必须原样内嵌（PRD 明确 64 位，C10-4）。
	re := regexp.MustCompile(`-(` + sha64HexPattern + `)-(` + sha64HexPattern + `)\.`)
	if !re.MatchString(name) {
		t.Fatalf("filename must embed two 64-hex sha segments: %s", name)
	}

	// 大写输入哈希统一小写（§5.9 小写 hex）。
	if got := BuildDeltaStableFilename("demo", "2", "1", "linux", "arm64", nil, "bsdiff", strings.ToUpper(src), strings.ToUpper(dst)); strings.ContainsAny(got, "ABCDEF") {
		t.Fatalf("sha segments must be lowercase: %s", got)
	}
}

// ---------- 生成链路（C10-7 / design §4）----------

// deltaJobFixture 准备：项目 + 两个已发布版本（1.0.0 / 2.0.0）各带 windows/x86_64
// 单文件全量产物（部分重叠字节），返回服务与存储。
func deltaJobFixture(t *testing.T) (*ProjectService, *repository.MemoryProjectStore) {
	t.Helper()
	store := repository.NewMemoryProjectStore()
	backend, err := storage.NewLocalFS(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	svc := NewProjectService(store, backend)
	svc.SetJobStore(repository.NewMemoryJobRepo())
	return svc, store
}

// publishWithArtifact 走真实上传链路（复用 UploadArtifact）创建并发布一个版本。
func publishWithArtifact(t *testing.T, svc *ProjectService, slug, version, content string) *model.Artifact {
	t.Helper()
	ctx := t.Context()
	if _, _, err := svc.PutVersion(ctx, mustProjectID(t, svc, slug), version, VersionWriteInput{Channel: "stable"}); err != nil {
		t.Fatal(err)
	}
	sha, _ := hashutil.SHA256Hex(strings.NewReader(content))
	art, err := svc.UploadArtifact(ctx, slug, version, "windows", "x86_64", UploadArtifactInput{
		Filename:       "tool.exe",
		Size:           int64(len(content)),
		ExpectedSHA256: sha,
	}, strings.NewReader(content))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.PublishVersion(ctx, mustProjectID(t, svc, slug), version); err != nil {
		t.Fatal(err)
	}
	return art
}

// mustProjectID 通过 slug 建项目并返回 ID（幂等：已存在直接查）。
func mustProjectID(t *testing.T, svc *ProjectService, slug string) uuid.UUID {
	t.Helper()
	p, err := svc.Resolve(t.Context(), slug)
	if err == nil {
		return p.ID
	}
	s := slug
	p, _, err = svc.Create(t.Context(), CreateProjectInput{DefaultLocale: ptr("en"), Slug: &s})
	if err != nil {
		t.Fatal(err)
	}
	return p.ID
}

// 验收项：admin 生成请求校验矩阵 —— 未知算法 / source==target / 源未就绪 / 缺 source_version。
func TestCreateDeltaJobValidation(t *testing.T) {
	svc, _ := deltaJobFixture(t)
	ctx := t.Context()

	slug := "delta-proj"
	mustProjectID(t, svc, slug)
	publishWithArtifact(t, svc, slug, "1.0.0", strings.Repeat("S", 4096))
	publishWithArtifact(t, svc, slug, "2.0.0", strings.Repeat("S", 2048)+strings.Repeat("T", 2048))

	// 未知算法 → ErrDeltaAlgoUnsupported（→ 400 DELTA_ALGO_UNSUPPORTED）。
	_, _, err := svc.CreateDeltaJob(ctx, slug, "2.0.0", CreateDeltaJobInput{
		SourceVersion: "1.0.0", OS: "windows", Arch: "x86_64", Algo: "zstd-dict",
	})
	if err != ErrDeltaAlgoUnsupported {
		t.Fatalf("unknown algo must be ErrDeltaAlgoUnsupported, got %v", err)
	}

	// source == target → 400。
	_, _, err = svc.CreateDeltaJob(ctx, slug, "2.0.0", CreateDeltaJobInput{
		SourceVersion: "2.0.0", OS: "windows", Arch: "x86_64",
	})
	if err != ErrDeltaSameVersion {
		t.Fatalf("same version must be ErrDeltaSameVersion, got %v", err)
	}

	// 缺 source_version。
	if _, _, err = svc.CreateDeltaJob(ctx, slug, "2.0.0", CreateDeltaJobInput{OS: "windows", Arch: "x86_64"}); err == nil {
		t.Fatal("missing source_version must fail")
	}

	// 源版本不存在 → ErrVersionNotFound。
	if _, _, err = svc.CreateDeltaJob(ctx, slug, "2.0.0", CreateDeltaJobInput{
		SourceVersion: "9.9.9", OS: "windows", Arch: "x86_64",
	}); err != ErrVersionNotFound {
		t.Fatalf("unknown source must be ErrVersionNotFound, got %v", err)
	}

	// 未发布源 → 拒绝（Draft 不可见）。
	if _, _, err := svc.PutVersion(ctx, mustProjectID(t, svc, slug), "0.9.0", VersionWriteInput{Channel: "stable"}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := svc.CreateDeltaJob(ctx, slug, "2.0.0", CreateDeltaJobInput{
		SourceVersion: "0.9.0", OS: "windows", Arch: "x86_64",
	}); err != ErrVersionNotFound {
		t.Fatalf("draft source must be rejected, got %v", err)
	}
}

// 验收项：job 执行成功 —— 差量对象落存储、Artifact 行元数据齐全（C10-2/C10-4/C10-6）。
func TestExecuteDeltaJobSuccessAndIdempotency(t *testing.T) {
	svc, store := deltaJobFixture(t)
	ctx := t.Context()

	slug := "delta-gen"
	publishWithArtifact(t, svc, slug, "1.0.0", strings.Repeat("S", 4096))
	publishWithArtifact(t, svc, slug, "2.0.0", strings.Repeat("S", 2048)+strings.Repeat("T", 2048))

	// 矩阵默认算法：显式写入 bsdiff，验证「缺省取矩阵 delta_algo」（§7.2，不另造默认）。
	if err := store.CreateMatrix(ctx, &model.PlatformMatrix{
		ProjectID: mustProjectID(t, svc, slug), OS: "windows", Arch: "x86_64",
		PackageType: model.PackageTypeSingleFile, DeltaAlgo: model.DeltaAlgoBsdiff,
	}); err != nil {
		t.Fatal(err)
	}

	jobID, created, err := svc.CreateDeltaJob(ctx, slug, "2.0.0", CreateDeltaJobInput{
		SourceVersion: "1.0.0", OS: "windows", Arch: "x86_64",
	})
	if err != nil || !created {
		t.Fatalf("create delta job: %v created=%v", err, created)
	}
	if err := svc.ExecuteDeltaJob(ctx, jobID); err != nil {
		t.Fatalf("execute delta job: %v", err)
	}

	// 校验落库的 delta Artifact 行。
	deltas := listDeltaArtifacts(t, svc, slug)
	if len(deltas) != 1 {
		t.Fatalf("expected 1 delta artifact, got %d", len(deltas))
	}
	d := deltas[0]
	if d.Kind != model.ArtifactKindDelta || d.DeltaAlgo != model.DeltaAlgoBsdiff {
		t.Fatalf("delta metadata wrong: kind=%s algo=%s", d.Kind, d.DeltaAlgo)
	}
	if d.DeltaSourceSHA256 == "" || d.DeltaTargetSHA256 == "" {
		t.Fatalf("delta source/target sha must be recorded: %+v", d)
	}
	if !strings.Contains(d.FileName, "-from-") || !strings.HasSuffix(d.FileName, ".bsdiff") {
		t.Fatalf("stable filename wrong: %s", d.FileName)
	}
	if !strings.Contains(d.FileName, d.DeltaSourceSHA256) || !strings.Contains(d.FileName, d.DeltaTargetSHA256) {
		t.Fatalf("filename must embed source/target sha: %s", d.FileName)
	}

	// 幂等：再次创建同参数 job 并执行 → Skipped，不产生第二个对象。
	jobID2, _, err := svc.CreateDeltaJob(ctx, slug, "2.0.0", CreateDeltaJobInput{
		SourceVersion: "1.0.0", OS: "windows", Arch: "x86_64", Algo: "bsdiff",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.ExecuteDeltaJob(ctx, jobID2); err != nil {
		t.Fatal(err)
	}
	if got := listDeltaArtifacts(t, svc, slug); len(got) != 1 {
		t.Fatalf("idempotent rerun must not duplicate delta artifacts, got %d", len(got))
	}

	// C10-6 逆用：换源版本（不同字节）→ 生成新对象、新文件名。
	publishWithArtifact(t, svc, slug, "1.5.0", strings.Repeat("U", 4096))
	jobID3, _, err := svc.CreateDeltaJob(ctx, slug, "2.0.0", CreateDeltaJobInput{
		SourceVersion: "1.5.0", OS: "windows", Arch: "x86_64", Algo: "bsdiff",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.ExecuteDeltaJob(ctx, jobID3); err != nil {
		t.Fatal(err)
	}
	got := listDeltaArtifacts(t, svc, slug)
	if len(got) != 2 {
		t.Fatalf("different source must yield a new delta object, got %d", len(got))
	}
	if got[0].FileName == got[1].FileName {
		t.Fatalf("different source must not reuse filename: %s", got[0].FileName)
	}
}

// 验收项：实体缺失（平台无线）→ 请求阶段即拒绝，不产生任务。
func TestExecuteDeltaJobFailure(t *testing.T) {
	svc, _ := deltaJobFixture(t)

	slug := "delta-fail"
	publishWithArtifact(t, svc, slug, "1.0.0", strings.Repeat("S", 1024))
	publishWithArtifact(t, svc, slug, "2.0.0", strings.Repeat("T", 1024))

	jobID, _, err := svc.CreateDeltaJob(t.Context(), slug, "2.0.0", CreateDeltaJobInput{
		SourceVersion: "1.0.0", OS: "linux", Arch: "arm64", Algo: "bsdiff",
	})
	if err != ErrVersionLineNotFound {
		t.Fatalf("missing platform line must be rejected upfront, got %v", err)
	}
	if jobID != uuid.Nil {
		t.Fatalf("rejected request must not enqueue a job")
	}
}

// listDeltaArtifacts 汇总项目全部 kind=delta 产物（测试辅助）。
func listDeltaArtifacts(t *testing.T, svc *ProjectService, slug string) []model.Artifact {
	t.Helper()
	p, err := svc.Resolve(t.Context(), slug)
	if err != nil {
		t.Fatal(err)
	}
	versions, err := svc.store.ListVersions(t.Context(), p.ID)
	if err != nil {
		t.Fatal(err)
	}
	var out []model.Artifact
	for _, v := range versions {
		arts, err := svc.store.ListArtifactsByVersionID(t.Context(), v.ID)
		if err != nil {
			t.Fatal(err)
		}
		for _, a := range arts {
			if a.Kind == model.ArtifactKindDelta {
				out = append(out, a)
			}
		}
	}
	return out
}

// deltaJobResultDecode 解析 job.result JSON（测试辅助）。
func deltaJobResultDecode(t *testing.T, raw []byte) DeltaJobResult {
	t.Helper()
	var r DeltaJobResult
	if err := json.Unmarshal(raw, &r); err != nil {
		t.Fatal(err)
	}
	return r
}

// 验收项（design §4）：携带相同 Idempotency-Key 的重复请求 → 复用既有任务
// （created=false、同 job_id），不重复入队。
func TestCreateDeltaJobIdempotencyKeyReuse(t *testing.T) {
	svc, _ := deltaJobFixture(t)
	ctx := t.Context()

	slug := "delta-idem"
	publishWithArtifact(t, svc, slug, "1.0.0", strings.Repeat("S", 4096))
	publishWithArtifact(t, svc, slug, "2.0.0", strings.Repeat("S", 2048)+strings.Repeat("T", 2048))

	in := CreateDeltaJobInput{
		SourceVersion: "1.0.0", OS: "windows", Arch: "x86_64",
		Algo: "bsdiff", IdempotencyKey: "delta-run-1",
	}
	jobID1, created1, err := svc.CreateDeltaJob(ctx, slug, "2.0.0", in)
	if err != nil || !created1 {
		t.Fatalf("first create: %v created=%v", err, created1)
	}
	jobID2, created2, err := svc.CreateDeltaJob(ctx, slug, "2.0.0", in)
	if err != nil {
		t.Fatal(err)
	}
	if created2 || jobID1 != jobID2 {
		t.Fatalf("same Idempotency-Key must reuse the job, got created=%v job1=%s job2=%s",
			created2, jobID1, jobID2)
	}

	// 不同 key → 新任务。
	in.IdempotencyKey = "delta-run-2"
	jobID3, created3, err := svc.CreateDeltaJob(ctx, slug, "2.0.0", in)
	if err != nil {
		t.Fatal(err)
	}
	if !created3 || jobID3 == jobID1 {
		t.Fatalf("different Idempotency-Key must create a new job, got created=%v job3=%s",
			created3, jobID3)
	}
}
