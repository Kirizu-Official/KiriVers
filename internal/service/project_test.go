package service

import (
	"errors"
	"testing"

	"github.com/Kirizu-Official/KiriVers/internal/model"
	"github.com/Kirizu-Official/KiriVers/internal/repository"
)

func TestProjectSlugAndCompareEngineLock(t *testing.T) {
	ctx := t.Context()
	store := repository.NewMemoryProjectStore()
	svc := NewProjectService(store)

	bad := "x"
	if _, _, err := svc.Create(ctx, CreateProjectInput{DefaultLocale: ptr("en"), Slug: &bad}); !errors.Is(err, ErrInvalidSlug) {
		t.Fatalf("short slug: %v", err)
	}
	illegal := "Nope!"
	if _, _, err := svc.Create(ctx, CreateProjectInput{DefaultLocale: ptr("en"), Slug: &illegal}); !errors.Is(err, ErrInvalidSlug) {
		t.Fatalf("illegal slug: %v", err)
	}

	slug := "lock-app"
	p, _, err := svc.Create(ctx, CreateProjectInput{DefaultLocale: ptr("en"), Slug: &slug})
	if err != nil {
		t.Fatal(err)
	}
	if p.ID.Version() != 4 {
		t.Fatalf("uuid version=%d want 4", p.ID.Version())
	}
	if p.ID.String() == "" || p.CompareEngine != model.CompareEngineSemver {
		t.Fatalf("defaults: %+v", p)
	}

	if err := store.CreateVersion(ctx, &model.Version{ProjectID: p.ID, Status: model.VersionStatusDraft}); err != nil {
		t.Fatal(err)
	}
	engine := model.CompareEngineInteger
	p, _, err = svc.Patch(ctx, slug, CreateProjectInput{DefaultLocale: ptr("en"), CompareEngine: &engine})
	if err != nil {
		t.Fatal(err)
	}
	if p.CompareEngine != model.CompareEngineInteger {
		t.Fatalf("engine=%s", p.CompareEngine)
	}

	bogus := "not-an-engine"
	if _, _, err := svc.Patch(ctx, slug, CreateProjectInput{DefaultLocale: ptr("en"), CompareEngine: &bogus}); !errors.Is(err, ErrInvalidProjectSettings) {
		t.Fatalf("bogus engine: %v", err)
	}

	if _, err := svc.CreatePublishedVersion(ctx, p.ID); err != nil {
		t.Fatal(err)
	}
	if _, _, err := svc.Patch(ctx, slug, CreateProjectInput{DefaultLocale: ptr("en"), CompareEngine: &bogus}); !errors.Is(err, ErrInvalidProjectSettings) {
		t.Fatalf("bogus after published: %v", err)
	}
	semver := model.CompareEngineSemver
	if _, _, err := svc.Patch(ctx, slug, CreateProjectInput{DefaultLocale: ptr("en"), CompareEngine: &semver}); !errors.Is(err, ErrCompareEngineImmutable) {
		t.Fatalf("lock: %v", err)
	}

	dup := slug
	if _, _, err := svc.Create(ctx, CreateProjectInput{DefaultLocale: ptr("en"), Slug: &dup}); !errors.Is(err, ErrSlugTaken) {
		t.Fatalf("dup: %v", err)
	}

	boot := "nope"
	other := "other-app"
	if _, _, err := svc.Create(ctx, CreateProjectInput{DefaultLocale: ptr("en"), Slug: &other, BootstrapAdminToken: &boot}); !errors.Is(err, ErrBootstrapTokenRejected) {
		t.Fatalf("bootstrap: %v", err)
	}
}

func TestStoragePathConflictsSlug(t *testing.T) {
	slug := "my-store"
	ptr := func(v string) *string { return &v }
	cases := []struct {
		name   string
		prefix *string
		bucket *string
		want   bool
	}{
		{name: "empty", want: false},
		{name: "prefix exact", prefix: ptr("my-store"), want: true},
		{name: "prefix segment", prefix: ptr("artifacts/my-store/out"), want: true},
		{name: "prefix trimmed", prefix: ptr("/My-Store/"), want: true},
		{name: "prefix other", prefix: ptr("artifacts"), want: false},
		{name: "bucket case", bucket: ptr("MY-STORE"), want: true},
		{name: "bucket not split", bucket: ptr("artifacts/my-store"), want: false},
		{name: "slash only", prefix: ptr("/"), want: false},
	}
	for _, tc := range cases {
		if got := storagePathConflictsSlug(slug, tc.prefix, tc.bucket); got != tc.want {
			t.Fatalf("%s: got %v want %v", tc.name, got, tc.want)
		}
	}
}
