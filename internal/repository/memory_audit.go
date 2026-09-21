package repository

import (
	"context"
	"sort"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/Kirizu-Official/KiriVers/internal/model"
)

// MemoryAuditStore 是 AuditStore 的进程内实现（单元 / handler 集成测试用）。
// 语义与 GORM 实现一致：Insert 补 ID/CreatedAt；ListByProject 倒序游标分页
//（游标 = created_at(RFC3339Nano)|id，与 encodeAuditCursor 共用编码）。
type MemoryAuditStore struct {
	mu     sync.Mutex
	events []*model.AuditEvent
}

// NewMemoryAuditStore 构造内存仓储。
func NewMemoryAuditStore() *MemoryAuditStore {
	return &MemoryAuditStore{}
}

// Insert 写入一条审计事件。
func (m *MemoryAuditStore) Insert(_ context.Context, event *model.AuditEvent) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := event.BeforeCreate(nil); err != nil {
		return err
	}
	cp := *event
	// 保证仓储内 created_at 严格递增：同纳秒事件在无 id 先验时次序不稳定，
	// 追加微秒级偏移维持「后写在后」的确定语义（与 GORM 实现的游标语义兼容）。
	if n := len(m.events); n > 0 && !cp.CreatedAt.After(m.events[n-1].CreatedAt) {
		cp.CreatedAt = m.events[n-1].CreatedAt.Add(time.Microsecond)
	}
	m.events = append(m.events, &cp)
	return nil
}

// ListByProject 项目级倒序分页（与 GORM 实现同游标语义）。
func (m *MemoryAuditStore) ListByProject(_ context.Context, projectID uuid.UUID, cursor string, limit int) ([]model.AuditEvent, string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	limit = clampAuditLimit(limit)

	// 收集项目内事件并倒序（新→旧；同刻按 id 降序）。
	rows := make([]model.AuditEvent, 0, len(m.events))
	for _, e := range m.events {
		if e.ProjectID != nil && *e.ProjectID == projectID {
			rows = append(rows, *e)
		}
	}
	sort.Slice(rows, func(i, j int) bool {
		if !rows[i].CreatedAt.Equal(rows[j].CreatedAt) {
			return rows[i].CreatedAt.After(rows[j].CreatedAt)
		}
		return rows[i].ID.String() > rows[j].ID.String()
	})

	// 游标定位：跳过 (created_at, id) >= 游标元组的行。
	if cursor != "" {
		at, id, err := decodeAuditCursor(cursor)
		if err != nil {
			return nil, "", err
		}
		kept := rows[:0]
		for _, e := range rows {
			if e.CreatedAt.Before(at) || (e.CreatedAt.Equal(at) && e.ID.String() < id.String()) {
				kept = append(kept, e)
			}
		}
		rows = kept
	}

	var next string
	if len(rows) > limit {
		rows = rows[:limit]
		last := rows[len(rows)-1]
		next = encodeAuditCursor(last.CreatedAt, last.ID)
	}
	return rows, next, nil
}

// Snapshot 测试辅助：返回全部事件的拷贝。
func (m *MemoryAuditStore) Snapshot() []model.AuditEvent {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]model.AuditEvent, 0, len(m.events))
	for _, e := range m.events {
		out = append(out, *e)
	}
	return out
}
