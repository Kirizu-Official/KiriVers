package repository

import (
	"context"
	"sort"

	"github.com/google/uuid"

	"github.com/Kirizu-Official/KiriVers/internal/model"
)

func allowlistKey(versionID uuid.UUID, deviceID string) string {
	return versionID.String() + "|" + deviceID
}

func (m *MemoryProjectStore) ListVersionAllowlist(_ context.Context, projectID uuid.UUID) ([]model.GrayAllowlist, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return sortAllowlist(m.listVersionAllowlistLocked(projectID)), nil
}

func (m *MemoryProjectStore) listVersionAllowlistLocked(projectID uuid.UUID) []model.GrayAllowlist {
	var list []model.GrayAllowlist
	for _, e := range m.grayAllowlist {
		if e.ProjectID == projectID {
			list = append(list, *e)
		}
	}
	return list
}

func (m *MemoryProjectStore) ListAllowlist(_ context.Context, projectID, versionID uuid.UUID) ([]model.GrayAllowlist, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var list []model.GrayAllowlist
	for _, e := range m.grayAllowlist {
		if e.ProjectID == projectID && e.VersionID == versionID {
			list = append(list, *e)
		}
	}
	sortAllowlist(list)
	return list, nil
}

func (m *MemoryProjectStore) InsertAllowlist(_ context.Context, entries []model.GrayAllowlist) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.insertAllowlistLocked(entries)
}

func (m *MemoryProjectStore) insertAllowlistLocked(entries []model.GrayAllowlist) error {
	for i := range entries {
		e := &entries[i]
		if err := e.BeforeCreate(nil); err != nil {
			return err
		}
		key := allowlistKey(e.VersionID, e.DeviceID)
		if _, exists := m.grayAllowlist[key]; exists {
			continue
		}
		cp := *e
		m.grayAllowlist[key] = &cp
	}
	return nil
}

func (m *MemoryProjectStore) DeleteAllowlist(_ context.Context, projectID, versionID uuid.UUID, deviceIDs []string) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	wanted := make(map[string]bool, len(deviceIDs))
	for _, d := range deviceIDs {
		wanted[d] = true
	}
	var removed int64
	for key, e := range m.grayAllowlist {
		if e.ProjectID == projectID && e.VersionID == versionID && wanted[e.DeviceID] {
			delete(m.grayAllowlist, key)
			removed++
		}
	}
	return removed, nil
}

func (m *MemoryProjectStore) DeleteAllowlistByVersion(_ context.Context, versionID uuid.UUID) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for key, e := range m.grayAllowlist {
		if e.VersionID == versionID {
			delete(m.grayAllowlist, key)
		}
	}
	return nil
}

func (m *MemoryProjectStore) DeleteAllowlistByDeviceID(_ context.Context, projectID uuid.UUID, deviceID string) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var removed int64
	for key, e := range m.grayAllowlist {
		if e.ProjectID == projectID && e.DeviceID == deviceID {
			delete(m.grayAllowlist, key)
			removed++
		}
	}
	return removed, nil
}

func (m *MemoryProjectStore) CountAllowlist(_ context.Context, versionID uuid.UUID) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var n int64
	for _, e := range m.grayAllowlist {
		if e.VersionID == versionID {
			n++
		}
	}
	return n, nil
}

func sortAllowlist(list []model.GrayAllowlist) []model.GrayAllowlist {
	sort.Slice(list, func(i, j int) bool { return list[i].DeviceID < list[j].DeviceID })
	return list
}
