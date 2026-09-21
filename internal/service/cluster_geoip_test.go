package service

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/google/uuid"

	"github.com/Kirizu-Official/KiriVers/internal/cache"
	"github.com/Kirizu-Official/KiriVers/internal/config"
	"github.com/Kirizu-Official/KiriVers/internal/model"
	"github.com/Kirizu-Official/KiriVers/internal/repository"
	"github.com/Kirizu-Official/KiriVers/internal/service/update"
	"github.com/Kirizu-Official/KiriVers/internal/storage"
	"github.com/Kirizu-Official/KiriVers/pkg/hashutil"
)

type countingBackend struct {
	storage.Backend
	mu   sync.Mutex
	gets map[string]int
	puts map[string]int
}

func wrapCount(b storage.Backend) *countingBackend {
	return &countingBackend{Backend: b, gets: map[string]int{}, puts: map[string]int{}}
}

func (c *countingBackend) Get(ctx context.Context, key string) (io.ReadCloser, error) {
	c.mu.Lock()
	c.gets[key]++
	c.mu.Unlock()
	return c.Backend.Get(ctx, key)
}

func (c *countingBackend) Put(ctx context.Context, key string, r io.Reader, size int64, contentType string) error {
	c.mu.Lock()
	c.puts[key]++
	c.mu.Unlock()
	return c.Backend.Put(ctx, key, r, size, contentType)
}

func TestGeoipPrivateBucketNotPublic(t *testing.T) {
	public, err := storage.NewLocalFS(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	privateFS, err := storage.NewLocalFS(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	private := wrapCount(privateFS)
	svc := NewGeoipService(repository.NewMemoryGeoipStore(), private, t.TempDir())
	svc.SetOpener(func(string) (geoReader, error) {
		return stubGeoReader{rec: geoRecord{CountryCode: "DE"}}, nil
	})
	row, err := svc.Upload(t.Context(), "MaxMind", "city.mmdb", bytes.NewReader([]byte("mmdb-bytes")), 10)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := public.Head(t.Context(), row.StorageKey); !errors.Is(err, storage.ErrNotFound) {
		t.Fatalf("geoip leaked to public bucket: %v", err)
	}
	if _, _, err := privateFS.Head(t.Context(), row.StorageKey); err != nil {
		t.Fatalf("missing on private: %v", err)
	}
	getsBefore := private.gets[row.StorageKey]
	if err := svc.Reload(t.Context()); err != nil {
		t.Fatal(err)
	}
	if private.gets[row.StorageKey] != getsBefore {
		t.Fatalf("reload should skip Get when Head size matches: before=%d after=%d", getsBefore, private.gets[row.StorageKey])
	}
}

func TestGeoipUploadFailsWithoutPrivateBackend(t *testing.T) {
	public, err := storage.NewLocalFS(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	svc := NewGeoipService(repository.NewMemoryGeoipStore(), nil, t.TempDir())
	svc.SetNopOpener()
	_, err = svc.Upload(t.Context(), "MaxMind", "city.mmdb", bytes.NewReader([]byte("mmdb-bytes")), 10)
	if !errors.Is(err, ErrGeoipStorage) {
		t.Fatalf("want ErrGeoipStorage, got %v", err)
	}
	_, _, err = public.Head(t.Context(), "geoip/")
	if err == nil {
		t.Fatal("public bucket should not have geoip object")
	}
}

func TestEnqueueDynamicPackOccupancyMiniredis(t *testing.T) {
	mr := miniredis.RunT(t)
	cacheStore, err := cache.Open(cache.Options{
		Driver:      "redis",
		RedisAddr:   mr.Addr(),
		PingTimeout: time.Second,
		DialTimeout: time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cacheStore.Close() })

	projStore := repository.NewMemoryProjectStore()
	jobs := repository.NewMemoryJobRepo()
	a := NewProjectService(projStore)
	b := NewProjectService(projStore)
	a.SetJobStore(jobs)
	b.SetJobStore(jobs)
	a.SetCache(cacheStore)
	b.SetCache(cacheStore)

	projectID := uuid.New()
	lineID := uuid.New()
	req := update.DynamicPackRequest{
		ProjectID:     projectID,
		LineID:        lineID,
		FilesetSHA256: strings.Repeat("ab", 32),
		Hw:            "revA",
	}
	var wg sync.WaitGroup
	wg.Add(2)
	errs := make(chan error, 2)
	go func() { defer wg.Done(); errs <- a.EnqueueDynamicPack(t.Context(), req) }()
	go func() { defer wg.Done(); errs <- b.EnqueueDynamicPack(t.Context(), req) }()
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	n := 0
	for _, j := range jobs.All() {
		if j.Type == model.JobTypeDynamicPack {
			n++
		}
	}
	if n != 1 {
		t.Fatalf("jobs=%d want 1", n)
	}
}

func TestLocalProxyReplicaVisibility(t *testing.T) {
	public, err := storage.NewLocalFS(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	mem := repository.NewMemoryProjectStore()
	nodes := repository.NewMemoryNodeStore()
	idA := uuid.MustParse("aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa")
	idB := uuid.MustParse("bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb")

	svcA := NewProjectService(mem, public)
	svcA.SetLocalRoot(t.TempDir())
	svcA.SetCluster(true, config.ClusterDownloadLocal)
	svcA.SetNodeStore(nodes)
	svcA.SetNodeID(idA)

	svcB := NewProjectService(mem, public)
	svcB.SetLocalRoot(t.TempDir())
	svcB.SetCluster(true, config.ClusterDownloadLocal)
	svcB.SetNodeStore(nodes)
	svcB.SetNodeID(idB)

	slug := "replica-app"
	p, _, err := svcA.Create(t.Context(), CreateProjectInput{Slug: &slug, DefaultLocale: ptr("en")})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := svcA.PutVersion(t.Context(), p.ID, "1.0.0", VersionWriteInput{Channel: "stable"}); err != nil {
		t.Fatal(err)
	}
	curBody := []byte("replica-current-package-bytes")
	curSHA, _ := hashutil.SHA256Hex(bytes.NewReader(curBody))
	if _, err := svcA.UploadArtifact(t.Context(), p.Slug, "1.0.0", "linux", "x86_64", UploadArtifactInput{
		Filename: "app.bin", ExpectedSHA256: curSHA, Size: int64(len(curBody)),
	}, bytes.NewReader(curBody)); err != nil {
		t.Fatal(err)
	}
	if _, err := svcA.PublishVersion(t.Context(), p.ID, "1.0.0"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := svcA.PutVersion(t.Context(), p.ID, "2.0.0", VersionWriteInput{Channel: "stable"}); err != nil {
		t.Fatal(err)
	}
	body := []byte("replica-full-package-bytes")
	sha, _ := hashutil.SHA256Hex(bytes.NewReader(body))
	art, err := svcA.UploadArtifact(t.Context(), p.Slug, "2.0.0", "linux", "x86_64", UploadArtifactInput{
		Filename: "app.bin", ExpectedSHA256: sha, Size: int64(len(body)),
	}, bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svcA.PublishVersion(t.Context(), p.ID, "2.0.0"); err != nil {
		t.Fatal(err)
	}
	svcA.stampReadyLinesNow(t.Context(), art.VersionID)

	if !svcA.ReplicaHas(art.StorageKey) {
		t.Fatal("originating node should have local replica")
	}
	if svcB.ReplicaHas(art.StorageKey) {
		t.Fatal("other node should not have replica yet")
	}

	loader := repository.NewMemoryUpdateCatalog(mem)
	checkA := update.NewService(loader, update.WithReplicaGate(svcA.ReplicaHas))
	checkB := update.NewService(loader, update.WithReplicaGate(svcB.ReplicaHas))
	in := update.CheckInput{CurrentVersion: "1.0.0", OS: "linux", Arch: "x86_64"}
	resA, err := checkA.Check(t.Context(), p.ID, "linux", "x86_64", in)
	if err != nil {
		t.Fatal(err)
	}
	if resA.Status != 200 {
		t.Fatalf("node A should see update, status=%d", resA.Status)
	}
	if resA.CacheControl != "private, no-store" {
		t.Fatalf("local-proxy check Cache-Control=%q", resA.CacheControl)
	}
	resB, err := checkB.Check(t.Context(), p.ID, "linux", "x86_64", in)
	if err != nil {
		t.Fatal(err)
	}
	if resB.Status != 204 {
		t.Fatalf("node B should hide line until replica exists, status=%d", resB.Status)
	}

	if err := svcB.pullPublicToLocal(t.Context(), art.StorageKey); err != nil {
		t.Fatal(err)
	}
	resB2, err := checkB.Check(t.Context(), p.ID, "linux", "x86_64", in)
	if err != nil {
		t.Fatal(err)
	}
	if resB2.Status != 200 {
		t.Fatalf("node B after pull should see update, status=%d", resB2.Status)
	}
}

func TestPresignUploadNeverDirectS3(t *testing.T) {
	svc, _, _ := setupTestService(t)
	slug := "presign-app"
	p, _, err := svc.Create(t.Context(), CreateProjectInput{Slug: &slug, DefaultLocale: ptr("en")})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := svc.PutVersion(t.Context(), p.ID, "1.0.0", VersionWriteInput{Channel: "stable"}); err != nil {
		t.Fatal(err)
	}
	out, err := svc.PresignUpload(t.Context(), p.Slug, "1.0.0", "linux", "x86_64", UploadArtifactInput{Filename: "a.bin", Size: 4})
	if err != nil {
		t.Fatal(err)
	}
	if out.DirectS3 {
		t.Fatal("direct_s3 must always be false")
	}
	if !strings.HasPrefix(out.UploadURL, "/api/v1/admin/projects/") {
		t.Fatalf("upload_url=%q", out.UploadURL)
	}
}

func TestOpenStoredObjectPrefersLocalReplica(t *testing.T) {
	publicFS, err := storage.NewLocalFS(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	public := wrapCount(publicFS)
	svc := NewProjectService(repository.NewMemoryProjectStore(), public)
	svc.SetLocalRoot(t.TempDir())
	svc.SetCluster(true, config.ClusterDownloadLocal)

	slug := "open-replica"
	p, _, err := svc.Create(t.Context(), CreateProjectInput{Slug: &slug, DefaultLocale: ptr("en")})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := svc.PutVersion(t.Context(), p.ID, "1.0.0", VersionWriteInput{Channel: "stable"}); err != nil {
		t.Fatal(err)
	}
	body := []byte("replica-download-bytes")
	sha, _ := hashutil.SHA256Hex(bytes.NewReader(body))
	art, err := svc.UploadArtifact(t.Context(), p.Slug, "1.0.0", "linux", "x86_64", UploadArtifactInput{
		Filename: "app.bin", ExpectedSHA256: sha, Size: int64(len(body)),
	}, bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	if !svc.ReplicaHas(art.StorageKey) {
		t.Fatal("expected local replica after upload")
	}
	getsBefore := public.gets[art.StorageKey]
	rc, err := svc.OpenStoredObject(t.Context(), art.StorageKey)
	if err != nil {
		t.Fatal(err)
	}
	got, err := io.ReadAll(rc)
	_ = rc.Close()
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(body) {
		t.Fatalf("replica bytes=%q", got)
	}
	if public.gets[art.StorageKey] != getsBefore {
		t.Fatalf("local-proxy download must not Get public object: before=%d after=%d", getsBefore, public.gets[art.StorageKey])
	}
}
