package repository

import (
	"context"
	"sync"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/Kirizu-Official/KiriVers/internal/model"
)

// MemoryProjectMediaStore 是进程内 ProjectMediaStore，供单测使用。
type MemoryProjectMediaStore struct {
	mu   sync.Mutex
	rows map[uuid.UUID]*model.ProjectMedia
}

// NewMemoryProjectMediaStore 构造空的内存仓储。
func NewMemoryProjectMediaStore() *MemoryProjectMediaStore {
	return &MemoryProjectMediaStore{rows: map[uuid.UUID]*model.ProjectMedia{}}
}

func cloneProjectMedia(src *model.ProjectMedia) *model.ProjectMedia {
	if src == nil {
		return nil
	}
	cp := *src
	return &cp
}

// Create 插入一条媒体元数据。
func (m *MemoryProjectMediaStore) Create(_ context.Context, row *model.ProjectMedia) error {
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
	m.rows[row.ID] = cloneProjectMedia(row)
	return nil
}

// GetByProjectAndID 按项目与 ID 读取。
func (m *MemoryProjectMediaStore) GetByProjectAndID(_ context.Context, projectID, id uuid.UUID) (*model.ProjectMedia, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	row, ok := m.rows[id]
	if !ok || row.ProjectID != projectID {
		return nil, gorm.ErrRecordNotFound
	}
	return cloneProjectMedia(row), nil
}
