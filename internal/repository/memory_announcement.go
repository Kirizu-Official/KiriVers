package repository

import (
	"context"
	"sort"
	"sync"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/Kirizu-Official/KiriVers/internal/model"
)

// MemoryAnnouncementStore 是进程内 AnnouncementStore，供单测使用。
type MemoryAnnouncementStore struct {
	mu   sync.Mutex
	rows map[uuid.UUID]*model.Announcement
}

// NewMemoryAnnouncementStore 构造空的内存仓储。
func NewMemoryAnnouncementStore() *MemoryAnnouncementStore {
	return &MemoryAnnouncementStore{rows: map[uuid.UUID]*model.Announcement{}}
}

func cloneAnnouncement(src *model.Announcement) *model.Announcement {
	if src == nil {
		return nil
	}
	cp := *src
	if src.VersionID != nil {
		id := *src.VersionID
		cp.VersionID = &id
	}
	if src.StartsAt != nil {
		t := *src.StartsAt
		cp.StartsAt = &t
	}
	if src.EndsAt != nil {
		t := *src.EndsAt
		cp.EndsAt = &t
	}
	return &cp
}

func sortAnnouncementSlice(list []model.Announcement) {
	sort.Slice(list, func(i, j int) bool {
		if list[i].SortOrder != list[j].SortOrder {
			return list[i].SortOrder < list[j].SortOrder
		}
		if !list[i].CreatedAt.Equal(list[j].CreatedAt) {
			return list[i].CreatedAt.Before(list[j].CreatedAt)
		}
		return list[i].ID.String() < list[j].ID.String()
	})
}

// ListByProject 返回项目内全部公告，顺序与 Postgres 实现一致。
func (m *MemoryAnnouncementStore) ListByProject(_ context.Context, projectID uuid.UUID) ([]model.Announcement, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.listLocked(projectID, false), nil
}

func (m *MemoryAnnouncementStore) listLocked(projectID uuid.UUID, omitContent bool) []model.Announcement {
	var list []model.Announcement
	for _, row := range m.rows {
		if row.ProjectID != projectID {
			continue
		}
		cp := cloneAnnouncement(row)
		if omitContent {
			cp.Content = ""
		}
		list = append(list, *cp)
	}
	sortAnnouncementSlice(list)
	return list
}

// ListIndexByProject 返回索引行（Content 为空串）。
func (m *MemoryAnnouncementStore) ListIndexByProject(_ context.Context, projectID uuid.UUID) ([]model.Announcement, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.listLocked(projectID, true), nil
}

// GetByID 按项目与 ID 读取一条公告。
func (m *MemoryAnnouncementStore) GetByID(_ context.Context, projectID, id uuid.UUID) (*model.Announcement, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	row, ok := m.rows[id]
	if !ok || row.ProjectID != projectID {
		return nil, gorm.ErrRecordNotFound
	}
	return cloneAnnouncement(row), nil
}

// Create 插入一条公告。
func (m *MemoryAnnouncementStore) Create(_ context.Context, row *model.Announcement) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := row.BeforeCreate(nil); err != nil {
		return err
	}
	now := time.Now().UTC()
	if row.CreatedAt.IsZero() {
		row.CreatedAt = now
	}
	row.UpdatedAt = now
	m.rows[row.ID] = cloneAnnouncement(row)
	return nil
}

// Update 全量回写一条公告。
func (m *MemoryAnnouncementStore) Update(_ context.Context, row *model.Announcement) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	existing, ok := m.rows[row.ID]
	if !ok || existing.ProjectID != row.ProjectID {
		return gorm.ErrRecordNotFound
	}
	row.UpdatedAt = time.Now().UTC()
	m.rows[row.ID] = cloneAnnouncement(row)
	return nil
}

// UpdateStatus 只改 status。
func (m *MemoryAnnouncementStore) UpdateStatus(_ context.Context, projectID, id uuid.UUID, status string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	row, ok := m.rows[id]
	if !ok || row.ProjectID != projectID {
		return gorm.ErrRecordNotFound
	}
	row.Status = status
	row.UpdatedAt = time.Now().UTC()
	return nil
}

// Delete 硬删除。
func (m *MemoryAnnouncementStore) Delete(_ context.Context, projectID, id uuid.UUID) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	row, ok := m.rows[id]
	if !ok || row.ProjectID != projectID {
		return gorm.ErrRecordNotFound
	}
	delete(m.rows, id)
	return nil
}

// Reorder 按 ids 顺序把 sort_order 写成 0..n-1。
func (m *MemoryAnnouncementStore) Reorder(_ context.Context, projectID uuid.UUID, ids []uuid.UUID) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	now := time.Now().UTC()
	for i, id := range ids {
		row, ok := m.rows[id]
		if !ok || row.ProjectID != projectID {
			return gorm.ErrRecordNotFound
		}
		row.SortOrder = i
		row.UpdatedAt = now
	}
	return nil
}

// MaxSortOrder 返回项目当前最大 sort_order；无行时返回 -1。
func (m *MemoryAnnouncementStore) MaxSortOrder(_ context.Context, projectID uuid.UUID) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	max := -1
	found := false
	for _, row := range m.rows {
		if row.ProjectID != projectID {
			continue
		}
		if !found || row.SortOrder > max {
			max = row.SortOrder
			found = true
		}
	}
	if !found {
		return -1, nil
	}
	return max, nil
}

// CountByProject 返回项目公告条数。
func (m *MemoryAnnouncementStore) CountByProject(_ context.Context, projectID uuid.UUID) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	n := 0
	for _, row := range m.rows {
		if row.ProjectID == projectID {
			n++
		}
	}
	return n, nil
}
