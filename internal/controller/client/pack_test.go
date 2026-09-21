package client

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/Kirizu-Official/KiriVers/internal/model"
	"github.com/Kirizu-Official/KiriVers/internal/repository"
	"github.com/Kirizu-Official/KiriVers/internal/service"
	"github.com/Kirizu-Official/KiriVers/internal/service/update"
	"github.com/Kirizu-Official/KiriVers/internal/storage"
	"github.com/Kirizu-Official/KiriVers/pkg/hashutil"
)

func setupPackTest(t *testing.T, maxBytes int64) (*gin.Engine, *service.ProjectService, context.Context) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	store := repository.NewMemoryProjectStore()
	backend, err := storage.NewLocalFS(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	projSvc := service.NewProjectService(store, backend)
	if maxBytes > 0 {
		projSvc.SetDynamicPackMaxBytes(maxBytes)
	}
	opts := []update.Option{
		update.WithLineDetails(store),
		update.WithPackRuntime(projSvc),
	}
	if maxBytes > 0 {
		opts = append(opts, update.WithDynamicPackMaxBytes(maxBytes))
	}
	upd := update.NewService(repository.NewMemoryUpdateCatalog(store), opts...)
	r := gin.New()
	Register(r.Group("/api/v1"), projSvc, upd, nil, nil, nil, nil)
	return r, projSvc, context.Background()
}

func packPOST(r *gin.Engine, project, suffix string, body map[string]any) *httptest.ResponseRecorder {
	raw, err := json.Marshal(body)
	if err != nil {
		panic(err)
	}
	req := httptest.NewRequest(http.MethodPost,
		fmt.Sprintf("/api/v1/projects/%s/update/pack%s", project, suffix), bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func packBody(extra map[string]any) map[string]any {
	b := map[string]any{
		"source_version": "1.0.0",
		"target_version": "1.1.0",
		"os":             "windows",
		"arch":           "x86_64",
		"needed_paths":   []string{"m/02.txt"},
	}
	for k, v := range extra {
		b[k] = v
	}
	return b
}

type packJSON struct {
	Status     string `json:"status"`
	DiffMode   string `json:"diff_mode"`
	PackageURL string `json:"package_url"`
	SHA256     string `json:"sha256"`
	Size       int64  `json:"size"`
}

func decodePack(t *testing.T, w *httptest.ResponseRecorder) packJSON {
	t.Helper()
	var res packJSON
	if err := json.Unmarshal(w.Body.Bytes(), &res); err != nil {
		t.Fatalf("decode pack body: %v %s", err, w.Body.String())
	}
	return res
}

func zipNamedFiles(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, body := range files {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write([]byte(body)); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func publishNamedZip(t *testing.T, projSvc *service.ProjectService, ctx context.Context, projectID uuid.UUID, version string, files map[string]string) {
	t.Helper()
	if _, _, err := projSvc.PutVersion(ctx, projectID, version, service.VersionWriteInput{Channel: "stable"}); err != nil {
		t.Fatalf("put version %s: %v", version, err)
	}
	payload := zipNamedFiles(t, files)
	sha, _ := hashutil.SHA256Hex(bytes.NewReader(payload))
	fname := fmt.Sprintf("pack-%s-windows-x86_64.zip", version)
	if _, err := projSvc.UploadArtifact(ctx, fmt.Sprint(projectID), version, "windows", "x86_64",
		service.UploadArtifactInput{Filename: fname, ExpectedSHA256: sha, Size: int64(len(payload))},
		bytes.NewReader(payload)); err != nil {
		t.Fatalf("upload %s: %v", version, err)
	}
	if _, err := projSvc.PublishVersion(ctx, projectID, version); err != nil {
		t.Fatalf("publish %s: %v", version, err)
	}
}

func setupRealPackProject(t *testing.T, projSvc *service.ProjectService, ctx context.Context) *model.Project {
	t.Helper()
	slug := "pack-e2e"
	p, _, err := projSvc.Create(ctx, service.CreateProjectInput{
		DefaultLocale: ptr("en"),
		Slug:          &slug,
		CompareEngine: ptr(model.CompareEngineSemver),
	})
	if err != nil {
		t.Fatal(err)
	}
	osStr, archStr := "windows", "x86_64"
	pkgType := model.PackageTypeMultiFile
	if _, err := projSvc.CreateMatrix(ctx, p.ID, service.MatrixWrite{
		OS: &osStr, Arch: &archStr, PackageType: &pkgType,
	}); err != nil {
		t.Fatal(err)
	}
	publishNamedZip(t, projSvc, ctx, p.ID, "1.0.0", map[string]string{
		"keep.txt":  "keep-bytes",
		"dir/a.txt": "old-a",
		"dir/b.txt": "old-b",
	})
	publishNamedZip(t, projSvc, ctx, p.ID, "1.1.0", map[string]string{
		"keep.txt":  "keep-bytes",
		"dir/a.txt": "new-a",
		"dir/b.txt": "new-b",
	})
	return p
}

func enablePackJobs(t *testing.T, projSvc *service.ProjectService) *repository.MemoryJobRepo {
	t.Helper()
	jobs := repository.NewMemoryJobRepo()
	projSvc.SetJobStore(jobs)
	return jobs
}

func claimDynamicPack(t *testing.T, jobs *repository.MemoryJobRepo) *model.Job {
	t.Helper()
	job, err := jobs.Claim(context.Background(), repository.ClaimFilter{})
	if err != nil {
		t.Fatal(err)
	}
	if job == nil {
		t.Fatal("expected a queued dynamic_pack job")
	}
	return job
}

func TestPackUnknownPathFullPackageNoJob(t *testing.T) {
	r, projSvc, ctx := setupPackTest(t, 0)
	p := setupIntegrityProject(t, projSvc, ctx)
	jobs := enablePackJobs(t, projSvc)

	w := packPOST(r, p.Slug, "", packBody(map[string]any{"needed_paths": []string{"m/02.txt", "not/there.bin"}}))
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d %s", w.Code, w.Body.String())
	}
	if w.Header().Get("Cache-Control") != "private, no-store" {
		t.Fatalf("cache-control = %q", w.Header().Get("Cache-Control"))
	}
	res := decodePack(t, w)
	if res.Status != update.PackStatusFullPackage || res.DiffMode != update.DiffModeFullPackage {
		t.Fatalf("unknown path must be full_package: %+v", res)
	}
	if res.PackageURL == "" {
		t.Fatal("full_package must include package_url")
	}
	job, err := jobs.Claim(ctx, repository.ClaimFilter{})
	if err != nil {
		t.Fatal(err)
	}
	if job != nil {
		t.Fatalf("unknown path must not enqueue, got job %s", job.ID)
	}
}

func TestPackD6UncompressedCapFullPackage(t *testing.T) {
	r, projSvc, ctx := setupPackTest(t, 20)
	p := setupIntegrityProject(t, projSvc, ctx)
	jobs := enablePackJobs(t, projSvc)

	w := packPOST(r, p.Slug, "", packBody(map[string]any{
		"needed_paths": []string{"m/01.txt", "m/02.txt"},
	}))
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d %s", w.Code, w.Body.String())
	}
	res := decodePack(t, w)
	if res.Status != update.PackStatusFullPackage {
		t.Fatalf("D6 must be full_package, got %+v", res)
	}
	job, err := jobs.Claim(ctx, repository.ClaimFilter{})
	if err != nil || job != nil {
		t.Fatalf("D6 must not enqueue: %v %+v", err, job)
	}
}

func TestPackD7ManifestRatioFullPackage(t *testing.T) {
	r, projSvc, ctx := setupPackTest(t, 0)
	p := setupIntegrityProject(t, projSvc, ctx)
	jobs := enablePackJobs(t, projSvc)

	w := packPOST(r, p.Slug, "", packBody(map[string]any{
		"needed_paths": []string{"m/01.txt", "m/02.txt", "m/03.txt"},
	}))
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d %s", w.Code, w.Body.String())
	}
	res := decodePack(t, w)
	if res.Status != update.PackStatusFullPackage {
		t.Fatalf("D7 must be full_package, got %+v", res)
	}
	job, err := jobs.Claim(ctx, repository.ClaimFilter{})
	if err != nil || job != nil {
		t.Fatalf("D7 must not enqueue: %v %+v", err, job)
	}
}

func TestPackShuffledNeededPathsOneJobThenReady(t *testing.T) {
	r, projSvc, ctx := setupPackTest(t, 0)
	p := setupRealPackProject(t, projSvc, ctx)
	jobs := enablePackJobs(t, projSvc)

	first := packPOST(r, p.Slug, "", packBody(map[string]any{
		"needed_paths": []string{`dir\a.txt`, "dir/b.txt"},
	}))
	if first.Code != http.StatusAccepted {
		t.Fatalf("expected 202 pending, got %d %s", first.Code, first.Body.String())
	}
	if decodePack(t, first).Status != update.PackStatusPending {
		t.Fatalf("first pack status: %s", first.Body.String())
	}

	second := packPOST(r, p.Slug, "", packBody(map[string]any{
		"needed_paths": []string{"dir/b.txt", "dir/a.txt"},
	}))
	if second.Code != http.StatusAccepted || decodePack(t, second).Status != update.PackStatusPending {
		t.Fatalf("shuffled fileset must coalesce to pending: %d %s", second.Code, second.Body.String())
	}

	var wg sync.WaitGroup
	codes := make([]int, 2)
	wg.Add(2)
	go func() {
		defer wg.Done()
		codes[0] = packPOST(r, p.Slug, "", packBody(map[string]any{
			"needed_paths": []string{"dir/b.txt", "dir/a.txt"},
		})).Code
	}()
	go func() {
		defer wg.Done()
		codes[1] = packPOST(r, p.Slug, "", packBody(map[string]any{
			"needed_paths": []string{`dir\a.txt`, `dir\b.txt`},
		})).Code
	}()
	wg.Wait()
	for i, code := range codes {
		if code != http.StatusAccepted {
			t.Fatalf("concurrent pack %d = %d", i, code)
		}
	}

	job := claimDynamicPack(t, jobs)
	extra, err := jobs.Claim(ctx, repository.ClaimFilter{})
	if err != nil {
		t.Fatal(err)
	}
	if extra != nil {
		t.Fatalf("identical filesets must create one job, extra=%s", extra.ID)
	}

	st := packPOST(r, p.Slug, "", packBody(map[string]any{
		"needed_paths": []string{"dir/a.txt", "dir/b.txt"},
	}))
	if st.Code != http.StatusAccepted || decodePack(t, st).Status != update.PackStatusPending {
		t.Fatalf("poll while queued must be 202 pending: %d %s", st.Code, st.Body.String())
	}
	if decodePack(t, st).PackageURL != "" {
		t.Fatal("pending must not expose package_url")
	}

	if err := projSvc.ExecuteDynamicPackJob(ctx, job.ID); err != nil {
		t.Fatal(err)
	}
	ready := packPOST(r, p.Slug, "", packBody(map[string]any{
		"needed_paths": []string{`dir\b.txt`, "dir/a.txt"},
	}))
	if ready.Code != http.StatusOK {
		t.Fatalf("status after job must be 200, got %d %s", ready.Code, ready.Body.String())
	}
	body := decodePack(t, ready)
	if body.Status != update.PackStatusReady || body.PackageURL == "" || body.SHA256 == "" || body.Size <= 0 {
		t.Fatalf("ready payload: %+v", body)
	}

	hit := packPOST(r, p.Slug, "", packBody(map[string]any{
		"needed_paths": []string{"dir/b.txt", "dir/a.txt"},
	}))
	if hit.Code != http.StatusOK || decodePack(t, hit).Status != update.PackStatusReady {
		t.Fatalf("cache hit must be 200 ready: %d %s", hit.Code, hit.Body.String())
	}
}

func TestPackPollDoesNotCreateSecondJob(t *testing.T) {
	r, projSvc, ctx := setupPackTest(t, 0)
	p := setupIntegrityProject(t, projSvc, ctx)
	jobs := enablePackJobs(t, projSvc)

	first := packPOST(r, p.Slug, "", packBody(nil))
	if first.Code != http.StatusAccepted {
		t.Fatalf("first pack must enqueue: %d %s", first.Code, first.Body.String())
	}
	poll := packPOST(r, p.Slug, "", packBody(nil))
	if poll.Code != http.StatusAccepted || decodePack(t, poll).Status != update.PackStatusPending {
		t.Fatalf("poll must stay pending: %d %s", poll.Code, poll.Body.String())
	}
	job, err := jobs.Claim(ctx, repository.ClaimFilter{})
	if err != nil || job == nil {
		t.Fatalf("want one job: %v %+v", err, job)
	}
	extra, err := jobs.Claim(ctx, repository.ClaimFilter{})
	if err != nil || extra != nil {
		t.Fatalf("poll must not enqueue a second job: %v %+v", err, extra)
	}
}

func TestPackEmptyNeededReadyNoEnqueue(t *testing.T) {
	r, projSvc, ctx := setupPackTest(t, 0)
	p := setupIntegrityProject(t, projSvc, ctx)
	jobs := enablePackJobs(t, projSvc)

	w := packPOST(r, p.Slug, "", packBody(map[string]any{"needed_paths": []string{}}))
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d %s", w.Code, w.Body.String())
	}
	if decodePack(t, w).Status != update.PackStatusReady {
		t.Fatalf("empty needed must be ready: %s", w.Body.String())
	}
	job, err := jobs.Claim(ctx, repository.ClaimFilter{})
	if err != nil || job != nil {
		t.Fatalf("empty needed must not enqueue: %v %+v", err, job)
	}
}
