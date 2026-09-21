package update

import (
	"context"
	"sync/atomic"
	"testing"

	"github.com/google/uuid"

	"github.com/Kirizu-Official/KiriVers/internal/cache"
	"github.com/Kirizu-Official/KiriVers/internal/model"
)

type stubCatalogLoader struct {
	n   atomic.Int32
	cat *Catalog
	err error
}

func (s *stubCatalogLoader) LoadCatalog(context.Context, uuid.UUID, string, string) (*Catalog, error) {
	s.n.Add(1)
	if s.err != nil {
		return nil, s.err
	}
	return s.cat, nil
}

func TestCachedCatalogLoaderSecondCallSkipsInner(t *testing.T) {
	store, err := cache.Open(cache.Options{Driver: "memory"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })

	semver := "1.2.3"
	id := uuid.MustParse("22222222-2222-2222-2222-222222222222")
	inner := &stubCatalogLoader{cat: &Catalog{
		Project: ProjectInfo{ID: id, Slug: "app", SigningPrivateKey: "secret-pem"},
		Versions: []VersionState{{
			Version: model.Version{
				Status:                 model.VersionStatusPublished,
				VersionSemverCanonical: &semver,
			},
			Allowlist: Allowlist{
				Version: []string{"canary"},
			},
		}},
	}}
	loader := NewCachedCatalogLoader(inner, store)
	ctx := t.Context()
	first, err := loader.LoadCatalog(ctx, id, "linux", "x86_64")
	if err != nil {
		t.Fatal(err)
	}
	if first.Project.SigningPrivateKey != "secret-pem" {
		t.Fatal("signing material must round-trip")
	}
	first.Project.SigningPrivateKey = "mutated"
	second, err := loader.LoadCatalog(ctx, id, "linux", "x86_64")
	if err != nil {
		t.Fatal(err)
	}
	if inner.n.Load() != 1 {
		t.Fatalf("inner calls=%d", inner.n.Load())
	}
	if second.Project.SigningPrivateKey != "secret-pem" {
		t.Fatal("get must clone catalog")
	}
	if second.Versions[0].Version.VersionSemverCanonical == nil || *second.Versions[0].Version.VersionSemverCanonical != "1.2.3" {
		t.Fatal("canonical semver must survive json cache")
	}
	if len(second.Versions[0].Allowlist.Version) != 1 || second.Versions[0].Allowlist.Version[0] != "canary" {
		t.Fatal("version allowlist must survive json cache")
	}
}

func TestNewCachedCatalogLoaderNilStore(t *testing.T) {
	inner := &stubCatalogLoader{cat: &Catalog{}}
	loader := NewCachedCatalogLoader(inner, nil)
	if loader != inner {
		t.Fatal("nil store must return inner loader")
	}
}
