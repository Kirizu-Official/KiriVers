package repository

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/Kirizu-Official/KiriVers/internal/model"
)

func projectIDSet(ids []uuid.UUID) map[uuid.UUID]struct{} {
	out := make(map[uuid.UUID]struct{}, len(ids))
	for _, id := range ids {
		out[id] = struct{}{}
	}
	return out
}

func (m *MemoryProjectStore) CountVersionsByStatus(_ context.Context, projectIDs []uuid.UUID) (map[uuid.UUID]VersionStatusCounts, error) {
	out := map[uuid.UUID]VersionStatusCounts{}
	if len(projectIDs) == 0 {
		return out, nil
	}
	want := projectIDSet(projectIDs)
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, v := range m.versions {
		if _, ok := want[v.ProjectID]; !ok {
			continue
		}
		c := out[v.ProjectID]
		switch v.Status {
		case model.VersionStatusDraft:
			c.Draft++
		case model.VersionStatusPublished:
			c.Published++
		case model.VersionStatusDeprecated:
			c.Deprecated++
		case model.VersionStatusRevoked:
			c.Revoked++
		}
		c.Total++
		out[v.ProjectID] = c
	}
	return out, nil
}

func (m *MemoryProjectStore) ListReleasedVersions(_ context.Context, projectIDs []uuid.UUID) ([]model.Version, error) {
	if len(projectIDs) == 0 {
		return nil, nil
	}
	want := projectIDSet(projectIDs)
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]model.Version, 0)
	for _, v := range m.versions {
		if _, ok := want[v.ProjectID]; !ok {
			continue
		}
		if v.Status != model.VersionStatusPublished && v.Status != model.VersionStatusDeprecated {
			continue
		}
		cp := *v
		out = append(out, cp)
	}
	return out, nil
}

func (m *MemoryProjectStore) CountChannelsByProject(_ context.Context, projectIDs []uuid.UUID) (map[uuid.UUID]int64, error) {
	return m.countMapByProject(projectIDs, func() map[uuid.UUID]uuid.UUID {
		ids := map[uuid.UUID]uuid.UUID{}
		for id, ch := range m.channels {
			ids[id] = ch.ProjectID
		}
		return ids
	})
}

func (m *MemoryProjectStore) CountMatrixByProject(_ context.Context, projectIDs []uuid.UUID) (map[uuid.UUID]int64, error) {
	return m.countMapByProject(projectIDs, func() map[uuid.UUID]uuid.UUID {
		ids := map[uuid.UUID]uuid.UUID{}
		for id, row := range m.matrix {
			ids[id] = row.ProjectID
		}
		return ids
	})
}

func (m *MemoryProjectStore) CountHwRevsByProject(_ context.Context, projectIDs []uuid.UUID) (map[uuid.UUID]int64, error) {
	return m.countMapByProject(projectIDs, func() map[uuid.UUID]uuid.UUID {
		ids := map[uuid.UUID]uuid.UUID{}
		for id, hw := range m.hwRevs {
			ids[id] = hw.ProjectID
		}
		return ids
	})
}

func (m *MemoryProjectStore) countMapByProject(projectIDs []uuid.UUID, collect func() map[uuid.UUID]uuid.UUID) (map[uuid.UUID]int64, error) {
	out := map[uuid.UUID]int64{}
	if len(projectIDs) == 0 {
		return out, nil
	}
	want := projectIDSet(projectIDs)
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, projectID := range collect() {
		if _, ok := want[projectID]; !ok {
			continue
		}
		out[projectID]++
	}
	return out, nil
}

func (m *MemoryProjectStore) CountProjectTokens(_ context.Context, projectIDs []uuid.UUID, now time.Time) (map[uuid.UUID]TokenCountRow, error) {
	out := map[uuid.UUID]TokenCountRow{}
	if len(projectIDs) == 0 {
		return out, nil
	}
	want := projectIDSet(projectIDs)
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, tok := range m.tokens {
		if _, ok := want[tok.ProjectID]; !ok {
			continue
		}
		row := out[tok.ProjectID]
		row.Total++
		if !model.TokenExpired(tok.ExpiresAt, now) {
			row.Active++
		}
		out[tok.ProjectID] = row
	}
	return out, nil
}

func (m *MemoryProjectStore) CountCITokens(_ context.Context, projectIDs []uuid.UUID, now time.Time) (map[uuid.UUID]TokenCountRow, error) {
	out := map[uuid.UUID]TokenCountRow{}
	if len(projectIDs) == 0 {
		return out, nil
	}
	want := projectIDSet(projectIDs)
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, tok := range m.ciTokens {
		if _, ok := want[tok.ProjectID]; !ok {
			continue
		}
		row := out[tok.ProjectID]
		row.Total++
		if !model.TokenExpired(tok.ExpiresAt, now) {
			row.Active++
		}
		out[tok.ProjectID] = row
	}
	return out, nil
}

func (m *MemoryProjectStore) ArtifactStorageStats(_ context.Context, projectIDs []uuid.UUID) (map[uuid.UUID]ArtifactStorageRow, error) {
	out := map[uuid.UUID]ArtifactStorageRow{}
	if len(projectIDs) == 0 {
		return out, nil
	}
	want := projectIDSet(projectIDs)
	m.mu.Lock()
	defer m.mu.Unlock()
	seen := map[uuid.UUID]map[string]int64{}
	for _, a := range m.artifacts {
		if _, ok := want[a.ProjectID]; !ok {
			continue
		}
		row := out[a.ProjectID]
		row.ArtifactCount++
		out[a.ProjectID] = row
		keys := seen[a.ProjectID]
		if keys == nil {
			keys = map[string]int64{}
			seen[a.ProjectID] = keys
		}
		keys[a.StorageKey] = a.Size
	}
	for projectID, keys := range seen {
		var sum int64
		for _, size := range keys {
			sum += size
		}
		row := out[projectID]
		row.StorageBytes = sum
		out[projectID] = row
	}
	return out, nil
}
