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

// MemoryJobRepo 提供纯内存的 JobStore 实现，供测试使用。
type MemoryJobRepo struct {
	mu   sync.Mutex
	jobs map[uuid.UUID]*model.Job
}

// NewMemoryJobRepo 创建内存任务仓储。
func NewMemoryJobRepo() *MemoryJobRepo {
	return &MemoryJobRepo{
		jobs: make(map[uuid.UUID]*model.Job),
	}
}

// Create 插入一条任务。
func (m *MemoryJobRepo) Create(ctx context.Context, job *model.Job) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if job.ID == uuid.Nil {
		job.ID = uuid.New()
	}
	if job.Status == "" {
		job.Status = model.JobStatusQueued
	}
	if job.CreatedAt.IsZero() {
		job.CreatedAt = time.Now().UTC()
	}
	job.UpdatedAt = time.Now().UTC()

	cp := *job
	m.jobs[job.ID] = &cp
	return nil
}

// Claim 抢占一条最早的可抢占 queued 任务（next_attempt_at 已到期）并标为 running。
func (m *MemoryJobRepo) Claim(ctx context.Context, filter ClaimFilter) (*model.Job, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	now := time.Now().UTC()
	allowed := map[string]struct{}{}
	for _, typ := range filter.Types {
		allowed[typ] = struct{}{}
	}
	var oldest *model.Job
	for _, j := range m.jobs {
		if j.Status != model.JobStatusQueued {
			continue
		}
		if j.NextAttemptAt != nil && j.NextAttemptAt.After(now) {
			continue
		}
		if filter.NodeID != uuid.Nil {
			if j.OwnerNodeID != nil && *j.OwnerNodeID != filter.NodeID {
				continue
			}
		}
		if len(allowed) > 0 {
			if _, ok := allowed[j.Type]; !ok {
				continue
			}
		}
		if oldest == nil || j.CreatedAt.Before(oldest.CreatedAt) {
			oldest = j
		}
	}
	if oldest == nil {
		return nil, nil
	}

	oldest.Status = model.JobStatusRunning
	oldest.StartedAt = &now
	oldest.Attempts++
	oldest.UpdatedAt = now

	cp := *oldest
	return &cp, nil
}

// Release 把 running 任务退回 queued。
func (m *MemoryJobRepo) Release(ctx context.Context, id uuid.UUID) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	j, ok := m.jobs[id]
	if !ok {
		return gorm.ErrRecordNotFound
	}
	if j.Status == model.JobStatusRunning {
		j.Status = model.JobStatusQueued
		j.StartedAt = nil
		j.UpdatedAt = time.Now().UTC()
	}
	return nil
}

// RequeueAfter 把任务退回 queued 并写入退避到期时间（C14-4）。
func (m *MemoryJobRepo) RequeueAfter(ctx context.Context, id uuid.UUID, delay time.Duration) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	j, ok := m.jobs[id]
	if !ok {
		return gorm.ErrRecordNotFound
	}
	now := time.Now().UTC()
	next := now.Add(delay)
	j.Status = model.JobStatusQueued
	j.StartedAt = nil
	j.FinishedAt = nil
	j.NextAttemptAt = &next
	j.UpdatedAt = now
	return nil
}

// MarkSucceeded 将任务标为成功。
func (m *MemoryJobRepo) MarkSucceeded(ctx context.Context, id uuid.UUID) error {
	return m.MarkSucceededWithResult(ctx, id, nil)
}

// MarkSucceededWithResult 将任务标为成功并记录结果。
func (m *MemoryJobRepo) MarkSucceededWithResult(ctx context.Context, id uuid.UUID, result []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	j, ok := m.jobs[id]
	if !ok {
		return gorm.ErrRecordNotFound
	}
	now := time.Now().UTC()
	j.Status = model.JobStatusSucceeded
	j.Progress = 100
	j.FinishedAt = &now
	j.UpdatedAt = now
	if len(result) > 0 {
		j.Result = result
	}
	return nil
}

// MarkFailed 记录失败。
func (m *MemoryJobRepo) MarkFailed(ctx context.Context, id uuid.UUID, message string) error {
	return m.MarkFailedWithResult(ctx, id, message, nil)
}

// MarkFailedWithResult 记录失败与部分结果。
func (m *MemoryJobRepo) MarkFailedWithResult(ctx context.Context, id uuid.UUID, message string, result []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	j, ok := m.jobs[id]
	if !ok {
		return gorm.ErrRecordNotFound
	}
	now := time.Now().UTC()
	j.Status = model.JobStatusFailed
	j.ErrorMessage = message
	j.FinishedAt = &now
	j.UpdatedAt = now
	if len(result) > 0 {
		j.Result = result
	}
	return nil
}

// GetByID 查询任务。
func (m *MemoryJobRepo) GetByID(ctx context.Context, id uuid.UUID) (*model.Job, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	j, ok := m.jobs[id]
	if !ok {
		return nil, gorm.ErrRecordNotFound
	}
	cp := *j
	return &cp, nil
}

// Update 更新任务。
func (m *MemoryJobRepo) Update(ctx context.Context, job *model.Job) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, ok := m.jobs[job.ID]; !ok {
		return gorm.ErrRecordNotFound
	}
	job.UpdatedAt = time.Now().UTC()
	cp := *job
	m.jobs[job.ID] = &cp
	return nil
}

// GetByIdempotencyKey 查找指定项目 24 小时内的幂等任务。
func (m *MemoryJobRepo) GetByIdempotencyKey(ctx context.Context, projectID uuid.UUID, key string) (*model.Job, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	since := time.Now().UTC().Add(-24 * time.Hour)
	var latest *model.Job
	for _, j := range m.jobs {
		if j.ProjectID != nil && *j.ProjectID == projectID && j.IdempotencyKey != nil && *j.IdempotencyKey == key {
			if j.CreatedAt.After(since) || j.CreatedAt.Equal(since) {
				if latest == nil || j.CreatedAt.After(latest.CreatedAt) {
					latest = j
				}
			}
		}
	}
	if latest == nil {
		return nil, fmt.Errorf("%w: idempotency key not found", gorm.ErrRecordNotFound)
	}
	cp := *latest
	return &cp, nil
}

// All 返回当前全部任务副本（测试用）。
func (m *MemoryJobRepo) All() []*model.Job {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]*model.Job, 0, len(m.jobs))
	for _, j := range m.jobs {
		cp := *j
		out = append(out, &cp)
	}
	return out
}
