package update

import (
	"context"
	"encoding/json"

	"github.com/google/uuid"

	"github.com/Kirizu-Official/KiriVers/internal/cache"
	"github.com/Kirizu-Official/KiriVers/internal/model"
)

// NewCachedCatalogLoader 在 inner 上做 cache-aside。store 为 nil 时直接返回 inner。
func NewCachedCatalogLoader(inner CatalogLoader, store cache.Store) CatalogLoader {
	if inner == nil || store == nil {
		return inner
	}
	return &cachedCatalogLoader{inner: inner, store: store}
}

type cachedCatalogLoader struct {
	inner CatalogLoader
	store cache.Store
}

type catalogRecord struct {
	Project        ProjectInfo     `json:"project"`
	Channels       []ChannelInfo   `json:"channels"`
	HwRevRanks     map[string]int  `json:"hw_rev_ranks"`
	Matrix         *MatrixInfo     `json:"matrix"`
	FallbackMatrix *MatrixInfo     `json:"fallback_matrix"`
	EnabledOS      []string        `json:"enabled_os"`
	Versions       []versionRecord `json:"versions"`
}

// versionRecord 单独保存 json:"-" 的 VersionSemverCanonical，否则选目标比较键会在命中后丢失。
type versionRecord struct {
	Version                model.Version `json:"version"`
	VersionSemverCanonical *string       `json:"version_semver_canonical"`
	Lines                  []LineState   `json:"lines"`
	Allowlist              Allowlist     `json:"allowlist"`
}

func (c *cachedCatalogLoader) LoadCatalog(ctx context.Context, projectID uuid.UUID, os, arch string) (*Catalog, error) {
	key := cache.CatalogKey(projectID, os, arch)
	if raw, ok, err := c.store.Get(ctx, key); err == nil && ok {
		if cat, decErr := unmarshalCatalog(raw); decErr == nil {
			return cat, nil
		}
		_ = c.store.Delete(ctx, key)
	}
	cat, err := c.inner.LoadCatalog(ctx, projectID, os, arch)
	if err != nil {
		return nil, err
	}
	if raw, encErr := marshalCatalog(cat); encErr == nil {
		_ = c.store.Set(ctx, key, raw)
		if cloned, decErr := unmarshalCatalog(raw); decErr == nil {
			return cloned, nil
		}
	}
	return cat, nil
}

func marshalCatalog(cat *Catalog) ([]byte, error) {
	if cat == nil {
		return json.Marshal(catalogRecord{})
	}
	rec := catalogRecord{
		Project:        cat.Project,
		Channels:       cat.Channels,
		HwRevRanks:     cat.HwRevRanks,
		Matrix:         cat.Matrix,
		FallbackMatrix: cat.FallbackMatrix,
		EnabledOS:      cat.EnabledOS,
		Versions:       make([]versionRecord, len(cat.Versions)),
	}
	for i := range cat.Versions {
		rec.Versions[i] = versionRecord{
			Version:                cat.Versions[i].Version,
			VersionSemverCanonical: cat.Versions[i].Version.VersionSemverCanonical,
			Lines:                  cat.Versions[i].Lines,
			Allowlist:              cat.Versions[i].Allowlist,
		}
	}
	return json.Marshal(rec)
}

func unmarshalCatalog(raw []byte) (*Catalog, error) {
	var rec catalogRecord
	if err := json.Unmarshal(raw, &rec); err != nil {
		return nil, err
	}
	cat := &Catalog{
		Project:        rec.Project,
		Channels:       rec.Channels,
		HwRevRanks:     rec.HwRevRanks,
		Matrix:         rec.Matrix,
		FallbackMatrix: rec.FallbackMatrix,
		EnabledOS:      rec.EnabledOS,
		Versions:       make([]VersionState, len(rec.Versions)),
	}
	for i := range rec.Versions {
		v := rec.Versions[i].Version
		v.VersionSemverCanonical = rec.Versions[i].VersionSemverCanonical
		cat.Versions[i] = VersionState{
			Version:   v,
			Lines:     rec.Versions[i].Lines,
			Allowlist: rec.Versions[i].Allowlist,
		}
	}
	return cat, nil
}
