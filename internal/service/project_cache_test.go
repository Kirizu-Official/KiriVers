package service

import (
	"context"
	"encoding/json"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/Kirizu-Official/KiriVers/internal/cache"
	"github.com/Kirizu-Official/KiriVers/internal/model"
	"github.com/Kirizu-Official/KiriVers/internal/repository"
	"github.com/Kirizu-Official/KiriVers/internal/service/update"
)

type countingProjectStore struct {
	*repository.MemoryProjectStore
	getByID    atomic.Int32
	getBySlug  atomic.Int32
	getByAlias atomic.Int32
}

func (c *countingProjectStore) GetByID(ctx context.Context, id uuid.UUID) (*model.Project, error) {
	c.getByID.Add(1)
	return c.MemoryProjectStore.GetByID(ctx, id)
}

func (c *countingProjectStore) GetBySlug(ctx context.Context, slug string) (*model.Project, error) {
	c.getBySlug.Add(1)
	return c.MemoryProjectStore.GetBySlug(ctx, slug)
}

func (c *countingProjectStore) GetByUnexpiredAlias(ctx context.Context, slug string, now time.Time) (*model.Project, *time.Time, error) {
	c.getByAlias.Add(1)
	return c.MemoryProjectStore.GetByUnexpiredAlias(ctx, slug, now)
}

type countingCatalogLoader struct {
	inner update.CatalogLoader
	n     atomic.Int32
}

func (c *countingCatalogLoader) LoadCatalog(ctx context.Context, projectID uuid.UUID, os, arch string) (*update.Catalog, error) {
	c.n.Add(1)
	return c.inner.LoadCatalog(ctx, projectID, os, arch)
}

func newMemoryCache(t *testing.T) cache.Store {
	t.Helper()
	store, err := cache.Open(cache.Options{Driver: "memory"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store
}

func TestResolveCacheHitSkipsStore(t *testing.T) {
	ctx := t.Context()
	inner := repository.NewMemoryProjectStore()
	counted := &countingProjectStore{MemoryProjectStore: inner}
	svc := NewProjectService(counted)
	svc.SetCache(newMemoryCache(t))

	slug := "cache-app"
	p, _, err := svc.Create(ctx, CreateProjectInput{DefaultLocale: ptr("en"), Slug: &slug})
	if err != nil {
		t.Fatal(err)
	}
	secret := p.DeviceSecret
	if secret == "" {
		t.Fatal("expected device secret")
	}

	got, err := svc.Resolve(ctx, slug)
	if err != nil {
		t.Fatal(err)
	}
	if got.DeviceSecret != secret {
		t.Fatal("first resolve lost device secret")
	}
	slugReads := counted.getBySlug.Load()
	idReads := counted.getByID.Load()
	if slugReads == 0 {
		t.Fatal("first slug resolve must hit store")
	}

	got2, err := svc.Resolve(ctx, slug)
	if err != nil {
		t.Fatal(err)
	}
	if counted.getBySlug.Load() != slugReads {
		t.Fatalf("second slug resolve hit store: %d -> %d", slugReads, counted.getBySlug.Load())
	}
	got2.DeviceSecret = "mutated"
	got2.Slug = "mutated"
	got3, err := svc.Resolve(ctx, slug)
	if err != nil {
		t.Fatal(err)
	}
	if got3.DeviceSecret != secret || got3.Slug != slug {
		t.Fatalf("cache must clone; secret=%q slug=%q", got3.DeviceSecret, got3.Slug)
	}

	idReads = counted.getByID.Load()
	if _, err = svc.Resolve(ctx, p.ID.String()); err != nil {
		t.Fatal(err)
	}
	if _, err = svc.Resolve(ctx, p.ID.String()); err != nil {
		t.Fatal(err)
	}
	if counted.getByID.Load() != idReads {
		t.Fatalf("uuid resolve after slug fill must use id key, store hits %d -> %d", idReads, counted.getByID.Load())
	}
}

func TestResolveAliasExpiryDoesNotUseStaleCache(t *testing.T) {
	ctx := t.Context()
	inner := repository.NewMemoryProjectStore()
	counted := &countingProjectStore{MemoryProjectStore: inner}
	svc := NewProjectService(counted)
	c := newMemoryCache(t)
	svc.SetCache(c)

	oldSlug := "old-slug-app"
	p, _, err := svc.Create(ctx, CreateProjectInput{DefaultLocale: ptr("en"), Slug: &oldSlug})
	if err != nil {
		t.Fatal(err)
	}
	newSlug := "new-slug-app"
	if _, _, err := svc.Patch(ctx, oldSlug, PatchProjectInput{Slug: &newSlug}); err != nil {
		t.Fatal(err)
	}

	expired := time.Now().UTC().Add(-time.Minute)
	rec := aliasRecord{projectRecord: packProject(p), ExpiresAt: &expired}
	raw, err := json.Marshal(rec)
	if err != nil {
		t.Fatal(err)
	}
	if err := c.Set(ctx, cache.ProjectAliasKey(oldSlug), raw); err != nil {
		t.Fatal(err)
	}
	inner.ExpireAlias(oldSlug, expired)

	aliasBefore := counted.getByAlias.Load()
	if _, err := svc.Resolve(ctx, oldSlug); !errors.Is(err, ErrProjectNotFound) {
		t.Fatalf("expired alias must not resolve, err=%v", err)
	}
	if counted.getByAlias.Load() <= aliasBefore {
		t.Fatal("expired cache entry must fall back to store")
	}
}

func TestPatchSlugInvalidatesIdentityCache(t *testing.T) {
	ctx := t.Context()
	inner := repository.NewMemoryProjectStore()
	counted := &countingProjectStore{MemoryProjectStore: inner}
	svc := NewProjectService(counted)
	svc.SetCache(newMemoryCache(t))

	oldSlug := "slug-before"
	p, _, err := svc.Create(ctx, CreateProjectInput{DefaultLocale: ptr("en"), Slug: &oldSlug})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Resolve(ctx, oldSlug); err != nil {
		t.Fatal(err)
	}
	slugReads := counted.getBySlug.Load()
	if _, err := svc.Resolve(ctx, oldSlug); err != nil {
		t.Fatal(err)
	}
	if counted.getBySlug.Load() != slugReads {
		t.Fatal("expected cache hit before patch")
	}

	newSlug := "slug-after"
	if _, _, err := svc.Patch(ctx, p.ID.String(), PatchProjectInput{Slug: &newSlug}); err != nil {
		t.Fatal(err)
	}
	got, err := svc.Resolve(ctx, newSlug)
	if err != nil {
		t.Fatal(err)
	}
	if got.Slug != newSlug {
		t.Fatalf("slug=%q", got.Slug)
	}
	gotOld, err := svc.Resolve(ctx, oldSlug)
	if err != nil {
		t.Fatal(err)
	}
	if gotOld.Slug != newSlug {
		t.Fatalf("alias should resolve to renamed project, slug=%q", gotOld.Slug)
	}
}

func TestPublishInvalidatesCatalogCache(t *testing.T) {
	ctx := t.Context()
	mem := repository.NewMemoryProjectStore()
	svc := NewProjectService(mem)
	c := newMemoryCache(t)
	svc.SetCache(c)

	p, _, err := svc.Create(ctx, CreateProjectInput{DefaultLocale: ptr("en"), Slug: ptr("catalog-cache")})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.CreateMatrix(ctx, p.ID, MatrixWrite{
		OS: ptr("macos"), Arch: ptr("arm64"), PackageType: ptr(model.PackageTypeSingleFile),
	}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := svc.PutVersion(ctx, p.ID, "1.0.0", VersionWriteInput{Channel: "stable"}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.AddVersionLine(ctx, p.ID, "1.0.0", VersionLineWriteInput{OS: "macos", Arch: "arm64"}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.ReadyVersionLine(ctx, p.ID, "1.0.0", "macos", "arm64"); err != nil {
		t.Fatal(err)
	}

	inner := &countingCatalogLoader{inner: repository.NewMemoryUpdateCatalog(mem)}
	loader := update.NewCachedCatalogLoader(inner, c)
	cat, err := loader.LoadCatalog(ctx, p.ID, "macos", "arm64")
	if err != nil {
		t.Fatal(err)
	}
	if len(cat.Versions) == 0 || cat.Versions[0].Version.Status != model.VersionStatusDraft {
		t.Fatalf("expected draft catalog, got %+v", cat.Versions)
	}
	first := inner.n.Load()
	cat.Versions[0].Version.Status = "mutated"
	cat2, err := loader.LoadCatalog(ctx, p.ID, "macos", "arm64")
	if err != nil {
		t.Fatal(err)
	}
	if inner.n.Load() != first {
		t.Fatal("second LoadCatalog must not hit inner loader")
	}
	if cat2.Versions[0].Version.Status != model.VersionStatusDraft {
		t.Fatal("catalog get must clone")
	}

	if _, err := svc.PublishVersion(ctx, p.ID, "1.0.0"); err != nil {
		t.Fatal(err)
	}
	cat3, err := loader.LoadCatalog(ctx, p.ID, "macos", "arm64")
	if err != nil {
		t.Fatal(err)
	}
	if inner.n.Load() != first+1 {
		t.Fatalf("publish must invalidate catalog, inner calls=%d", inner.n.Load())
	}
	if cat3.Versions[0].Version.Status != model.VersionStatusPublished {
		t.Fatalf("status=%s", cat3.Versions[0].Version.Status)
	}
	if cat3.Versions[0].Version.VersionSemverCanonical == nil || *cat3.Versions[0].Version.VersionSemverCanonical != "1.0.0" {
		t.Fatal("cached catalog must keep version_semver_canonical")
	}
}

func TestNilCacheKeepsDirectStoreBehavior(t *testing.T) {
	ctx := t.Context()
	inner := repository.NewMemoryProjectStore()
	counted := &countingProjectStore{MemoryProjectStore: inner}
	svc := NewProjectService(counted)
	slug := "no-cache-app"
	if _, _, err := svc.Create(ctx, CreateProjectInput{DefaultLocale: ptr("en"), Slug: &slug}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Resolve(ctx, slug); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Resolve(ctx, slug); err != nil {
		t.Fatal(err)
	}
	if counted.getBySlug.Load() < 2 {
		t.Fatalf("nil cache must hit store every time, got %d", counted.getBySlug.Load())
	}
}

func TestLanguageDefaultChangeInvalidatesProjectCache(t *testing.T) {
	ctx := t.Context()
	mem := repository.NewMemoryProjectStore()
	svc := NewProjectService(mem)
	c := newMemoryCache(t)
	svc.SetCache(c)

	p, _, err := svc.Create(ctx, CreateProjectInput{DefaultLocale: ptr("zh-CN"), Slug: ptr("lang-cache")})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Resolve(ctx, "lang-cache"); err != nil {
		t.Fatal(err)
	}

	inner := &countingCatalogLoader{inner: repository.NewMemoryUpdateCatalog(mem)}
	loader := update.NewCachedCatalogLoader(inner, c)
	cat, err := loader.LoadCatalog(ctx, p.ID, "linux", "x86_64")
	if err != nil {
		t.Fatal(err)
	}
	if cat.Project.DefaultLocale != "zh-CN" {
		t.Fatalf("catalog default=%q", cat.Project.DefaultLocale)
	}
	first := inner.n.Load()
	if _, err := loader.LoadCatalog(ctx, p.ID, "linux", "x86_64"); err != nil {
		t.Fatal(err)
	}
	if inner.n.Load() != first {
		t.Fatal("second LoadCatalog must hit cache")
	}

	on := true
	ja := "ja"
	if _, err := svc.CreateLanguage(ctx, p.ID, LanguageWrite{Code: &ja, IsDefault: &on}); err != nil {
		t.Fatal(err)
	}

	got, err := svc.Resolve(ctx, "lang-cache")
	if err != nil {
		t.Fatal(err)
	}
	if got.DefaultLocale != "ja" {
		t.Fatalf("resolve cache stale default_locale=%q", got.DefaultLocale)
	}

	cat2, err := loader.LoadCatalog(ctx, p.ID, "linux", "x86_64")
	if err != nil {
		t.Fatal(err)
	}
	if inner.n.Load() != first+1 {
		t.Fatalf("default language change must invalidate catalog, inner=%d", inner.n.Load())
	}
	if cat2.Project.DefaultLocale != "ja" {
		t.Fatalf("catalog default_locale=%q", cat2.Project.DefaultLocale)
	}
}
