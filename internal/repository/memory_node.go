package repository

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/Kirizu-Official/KiriVers/internal/model"
)

// MemoryNodeStore 是测试用节点仓储。
type MemoryNodeStore struct {
	mu    sync.Mutex
	nodes map[uuid.UUID]*model.Node
	syncs []model.NodeArtifactSync
}

// NewMemoryNodeStore 构造空表。
func NewMemoryNodeStore() *MemoryNodeStore {
	return &MemoryNodeStore{nodes: map[uuid.UUID]*model.Node{}}
}

// Create 插入；主键已存在则返回 duplicate 错误（供身份分配重试）。
func (m *MemoryNodeStore) Create(_ context.Context, node *model.Node) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if node.ID == uuid.Nil {
		node.ID = uuid.New()
	}
	if _, ok := m.nodes[node.ID]; ok {
		return fmt.Errorf("duplicate key value violates unique constraint")
	}
	now := time.Now().UTC()
	if node.CreatedAt.IsZero() {
		node.CreatedAt = now
	}
	node.UpdatedAt = now
	cp := *node
	m.nodes[node.ID] = &cp
	return nil
}

// GetByID 按主键读取。
func (m *MemoryNodeStore) GetByID(_ context.Context, id uuid.UUID) (*model.Node, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	n, ok := m.nodes[id]
	if !ok {
		return nil, gorm.ErrRecordNotFound
	}
	cp := *n
	return &cp, nil
}

// Save 覆盖整行。
func (m *MemoryNodeStore) Save(_ context.Context, node *model.Node) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.nodes[node.ID]; !ok {
		return gorm.ErrRecordNotFound
	}
	node.UpdatedAt = time.Now().UTC()
	cp := *node
	m.nodes[node.ID] = &cp
	return nil
}

// List 返回全部节点。
func (m *MemoryNodeStore) List(_ context.Context) ([]model.Node, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]model.Node, 0, len(m.nodes))
	for _, n := range m.nodes {
		out = append(out, *n)
	}
	return out, nil
}

// UpdateHeartbeat 写心跳列。
func (m *MemoryNodeStore) UpdateHeartbeat(_ context.Context, id uuid.UUID, cpu float64, memUsed, memTotal int64, lastSeen time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	n, ok := m.nodes[id]
	if !ok {
		return gorm.ErrRecordNotFound
	}
	n.CPUPercent = cpu
	n.MemUsedBytes = memUsed
	n.MemTotalBytes = memTotal
	n.LastSeenAt = lastSeen
	n.UpdatedAt = time.Now().UTC()
	return nil
}

// ListSyncByProject 过滤项目。
func (m *MemoryNodeStore) ListSyncByProject(_ context.Context, projectID uuid.UUID) ([]model.NodeArtifactSync, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []model.NodeArtifactSync
	for _, row := range m.syncs {
		if row.ProjectID == projectID {
			out = append(out, row)
		}
	}
	return out, nil
}

// UpsertSync 按范围替换或追加。
func (m *MemoryNodeStore) UpsertSync(_ context.Context, row *model.NodeArtifactSync) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if row.ID == uuid.Nil {
		row.ID = uuid.New()
	}
	now := time.Now().UTC()
	if row.CreatedAt.IsZero() {
		row.CreatedAt = now
	}
	row.UpdatedAt = now
	for i := range m.syncs {
		if m.syncs[i].NodeID == row.NodeID && m.syncs[i].VersionID == row.VersionID && sameLine(m.syncs[i].LineID, row.LineID) {
			row.ID = m.syncs[i].ID
			row.CreatedAt = m.syncs[i].CreatedAt
			m.syncs[i] = *row
			return nil
		}
	}
	m.syncs = append(m.syncs, *row)
	return nil
}

func sameLine(a, b *uuid.UUID) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	return *a == *b
}
