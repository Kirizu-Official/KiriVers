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

// MemoryWebhookStore 是进程内的 WebhookStore 实现，供单测使用。
type MemoryWebhookStore struct {
	mu         sync.Mutex
	deliveries map[uuid.UUID]*model.WebhookDelivery
}

// NewMemoryWebhookStore 构造空的内存仓储。
func NewMemoryWebhookStore() *MemoryWebhookStore {
	return &MemoryWebhookStore{deliveries: map[uuid.UUID]*model.WebhookDelivery{}}
}

// CreateDelivery 插入一条投递记录。
func (m *MemoryWebhookStore) CreateDelivery(_ context.Context, delivery *model.WebhookDelivery) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := delivery.BeforeCreate(nil); err != nil {
		return err
	}
	now := time.Now().UTC()
	if delivery.CreatedAt.IsZero() {
		delivery.CreatedAt = now
	}
	delivery.UpdatedAt = now
	cp := *delivery
	m.deliveries[delivery.ID] = &cp
	return nil
}

// GetDelivery 按 ID 读取投递记录。
func (m *MemoryWebhookStore) GetDelivery(_ context.Context, id uuid.UUID) (*model.WebhookDelivery, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	d, ok := m.deliveries[id]
	if !ok {
		return nil, gorm.ErrRecordNotFound
	}
	cp := *d
	return &cp, nil
}

// UpdateDelivery 回写投递状态。
func (m *MemoryWebhookStore) UpdateDelivery(_ context.Context, delivery *model.WebhookDelivery) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.deliveries[delivery.ID]; !ok {
		return gorm.ErrRecordNotFound
	}
	delivery.UpdatedAt = time.Now().UTC()
	cp := *delivery
	m.deliveries[delivery.ID] = &cp
	return nil
}

// ListDeliveries 按 (created_at DESC, id DESC) 游标分页，语义与 GORM 实现一致。
func (m *MemoryWebhookStore) ListDeliveries(_ context.Context, projectID uuid.UUID, cursor *uuid.UUID, limit int) ([]model.WebhookDelivery, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if limit <= 0 {
		limit = 50
	}
	var cutoff *model.WebhookDelivery
	if cursor != nil {
		d, ok := m.deliveries[*cursor]
		if !ok || d.ProjectID != projectID {
			return nil, gorm.ErrRecordNotFound
		}
		cutoff = d
	}
	var list []model.WebhookDelivery
	for _, d := range m.deliveries {
		if d.ProjectID != projectID {
			continue
		}
		if cutoff != nil {
			if d.CreatedAt.After(cutoff.CreatedAt) {
				continue
			}
			if d.CreatedAt.Equal(cutoff.CreatedAt) && d.ID.String() >= cutoff.ID.String() {
				continue
			}
		}
		list = append(list, *d)
	}
	sort.Slice(list, func(i, j int) bool {
		if !list[i].CreatedAt.Equal(list[j].CreatedAt) {
			return list[i].CreatedAt.After(list[j].CreatedAt)
		}
		return list[i].ID.String() > list[j].ID.String()
	})
	if len(list) > limit {
		list = list[:limit]
	}
	return list, nil
}
