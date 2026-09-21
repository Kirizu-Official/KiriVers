package repository

import (
	"context"
	"sort"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/Kirizu-Official/KiriVers/internal/model"
)

// memoryTelemetryEntry 是内存仓储的存储单元：事件 + 插入序号（同时刻排序决胜）。
type memoryTelemetryEntry struct {
	event *model.TelemetryEvent
	seq   uint64
}

// MemoryTelemetryStore 是 TelemetryStore 的进程内实现，供单测使用，不需要 PostgreSQL。
type MemoryTelemetryStore struct {
	mu     sync.Mutex
	events map[uuid.UUID]memoryTelemetryEntry
	// seq 是插入序号：ListWindow 对 created_at 相同的事件按 seq 倒序排序，
	// 保证「同一时刻后插入的更新」这一确定性行为与真实时间序一致。
	seq uint64
}

// NewMemoryTelemetryStore 构造空的内存遥测仓储。
func NewMemoryTelemetryStore() *MemoryTelemetryStore {
	return &MemoryTelemetryStore{events: map[uuid.UUID]memoryTelemetryEntry{}}
}

// Insert 写入一条事件（调用 BeforeCreate 兜底补 ID / CreatedAt）。
func (m *MemoryTelemetryStore) Insert(_ context.Context, event *model.TelemetryEvent) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := event.BeforeCreate(nil); err != nil {
		return err
	}
	m.seq++
	m.events[event.ID] = memoryTelemetryEntry{event: cloneTelemetryEvent(event), seq: m.seq}
	return nil
}

// DeleteByDeviceHash 删除项目内该设备哈希的全部事件，返回删除条数。
func (m *MemoryTelemetryStore) DeleteByDeviceHash(_ context.Context, projectID uuid.UUID, deviceHash string) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var n int64
	for id, entry := range m.events {
		if entry.event.ProjectID == projectID && entry.event.DeviceHash == deviceHash {
			delete(m.events, id)
			n++
		}
	}
	return n, nil
}

// DeleteOlderThan 删除早于 cutoff 的事件，返回删除条数。
func (m *MemoryTelemetryStore) DeleteOlderThan(_ context.Context, projectID uuid.UUID, cutoff time.Time) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var n int64
	for id, entry := range m.events {
		if entry.event.ProjectID == projectID && entry.event.CreatedAt.Before(cutoff) {
			delete(m.events, id)
			n++
		}
	}
	return n, nil
}

// ListWindow 返回窗口内事件，created_at 倒序（同时刻按插入序号倒序，与
// GORM 实现的「新→旧」语义一致）。
func (m *MemoryTelemetryStore) ListWindow(_ context.Context, projectID uuid.UUID, os, arch, deviceHash string, since time.Time) ([]model.TelemetryEvent, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	type scored struct {
		event model.TelemetryEvent
		seq   uint64
	}
	var list []scored
	for _, entry := range m.events {
		e := entry.event
		if e.ProjectID != projectID || e.OS != os || e.Arch != arch || e.DeviceHash != deviceHash {
			continue
		}
		if e.CreatedAt.Before(since) {
			continue
		}
		list = append(list, scored{event: *cloneTelemetryEvent(e), seq: entry.seq})
	}
	sort.Slice(list, func(i, j int) bool {
		if !list[i].event.CreatedAt.Equal(list[j].event.CreatedAt) {
			return list[i].event.CreatedAt.After(list[j].event.CreatedAt)
		}
		return list[i].seq > list[j].seq
	})
	out := make([]model.TelemetryEvent, 0, len(list))
	for _, s := range list {
		out = append(out, s.event)
	}
	return out, nil
}

// Snapshot 返回全部事件的副本，仅供测试断言库中内容（如 hashed 策略无明文）。
func (m *MemoryTelemetryStore) Snapshot() []model.TelemetryEvent {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]model.TelemetryEvent, 0, len(m.events))
	for _, entry := range m.events {
		out = append(out, *cloneTelemetryEvent(entry.event))
	}
	return out
}

func (m *MemoryTelemetryStore) CountStatusSince(_ context.Context, projectIDs []uuid.UUID, since time.Time, statuses []string) (map[uuid.UUID]map[string]int64, error) {
	out := map[uuid.UUID]map[string]int64{}
	if len(projectIDs) == 0 || len(statuses) == 0 {
		return out, nil
	}
	wantID := map[uuid.UUID]struct{}{}
	for _, id := range projectIDs {
		wantID[id] = struct{}{}
	}
	wantStatus := map[string]struct{}{}
	for _, status := range statuses {
		wantStatus[status] = struct{}{}
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, entry := range m.events {
		e := entry.event
		if _, ok := wantID[e.ProjectID]; !ok {
			continue
		}
		if _, ok := wantStatus[e.Status]; !ok {
			continue
		}
		if e.CreatedAt.Before(since) {
			continue
		}
		byStatus := out[e.ProjectID]
		if byStatus == nil {
			byStatus = map[string]int64{}
			out[e.ProjectID] = byStatus
		}
		byStatus[e.Status]++
	}
	return out, nil
}

func (m *MemoryTelemetryStore) CountStatusByDay(_ context.Context, projectID uuid.UUID, from, to time.Time, statuses []string) ([]TelemetryDayCount, error) {
	if len(statuses) == 0 {
		return nil, nil
	}
	if to.Before(from) {
		from, to = to, from
	}
	wantStatus := map[string]struct{}{}
	for _, status := range statuses {
		wantStatus[status] = struct{}{}
	}
	from = from.UTC()
	to = to.UTC()
	agg := map[string]map[string]int64{}
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, entry := range m.events {
		e := entry.event
		if e.ProjectID != projectID {
			continue
		}
		if _, ok := wantStatus[e.Status]; !ok {
			continue
		}
		at := e.CreatedAt.UTC()
		if at.Before(from) || at.After(to) {
			continue
		}
		day := at.Format("2006-01-02")
		byStatus := agg[day]
		if byStatus == nil {
			byStatus = map[string]int64{}
			agg[day] = byStatus
		}
		byStatus[e.Status]++
	}
	days := make([]string, 0, len(agg))
	for day := range agg {
		days = append(days, day)
	}
	sort.Strings(days)
	out := make([]TelemetryDayCount, 0, len(days)*2)
	for _, day := range days {
		parsed, _ := time.Parse("2006-01-02", day)
		for status, n := range agg[day] {
			out = append(out, TelemetryDayCount{Day: parsed, Status: status, Count: n})
		}
	}
	return out, nil
}

func cloneTelemetryEvent(e *model.TelemetryEvent) *model.TelemetryEvent {
	if e == nil {
		return nil
	}
	cp := *e
	return &cp
}
