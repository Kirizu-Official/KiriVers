package repository

import (
	"context"
	"sort"
	"sync"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/Kirizu-Official/KiriVers/internal/model"
)

// MemoryGeoipStore 是进程内实现，供单测使用。
type MemoryGeoipStore struct {
	mu   sync.Mutex
	byID map[uuid.UUID]*model.GeoipDatabase
}

// NewMemoryGeoipStore 构造空仓储。
func NewMemoryGeoipStore() *MemoryGeoipStore {
	return &MemoryGeoipStore{byID: map[uuid.UUID]*model.GeoipDatabase{}}
}

func cloneGeoip(row *model.GeoipDatabase) *model.GeoipDatabase {
	if row == nil {
		return nil
	}
	cp := *row
	return &cp
}

func (m *MemoryGeoipStore) List(_ context.Context) ([]model.GeoipDatabase, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]model.GeoipDatabase, 0, len(m.byID))
	for _, row := range m.byID {
		out = append(out, *cloneGeoip(row))
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Rank != out[j].Rank {
			return out[i].Rank < out[j].Rank
		}
		return out[i].CreatedAt.Before(out[j].CreatedAt)
	})
	return out, nil
}

func (m *MemoryGeoipStore) GetByID(_ context.Context, id uuid.UUID) (*model.GeoipDatabase, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	row, ok := m.byID[id]
	if !ok {
		return nil, gorm.ErrRecordNotFound
	}
	return cloneGeoip(row), nil
}

func (m *MemoryGeoipStore) Count(_ context.Context) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return int64(len(m.byID)), nil
}

func (m *MemoryGeoipStore) MaxRank(_ context.Context) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	max := 0
	for _, row := range m.byID {
		if row.Rank > max {
			max = row.Rank
		}
	}
	return max, nil
}

func (m *MemoryGeoipStore) Create(_ context.Context, row *model.GeoipDatabase) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := row.BeforeCreate(nil); err != nil {
		return err
	}
	m.byID[row.ID] = cloneGeoip(row)
	return nil
}

func (m *MemoryGeoipStore) Save(_ context.Context, row *model.GeoipDatabase) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.byID[row.ID]; !ok {
		return gorm.ErrRecordNotFound
	}
	m.byID[row.ID] = cloneGeoip(row)
	return nil
}

func (m *MemoryGeoipStore) Delete(_ context.Context, id uuid.UUID) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.byID[id]; !ok {
		return gorm.ErrRecordNotFound
	}
	delete(m.byID, id)
	return nil
}
