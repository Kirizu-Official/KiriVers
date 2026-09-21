package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/Kirizu-Official/KiriVers/internal/model"
)

// JobStore 负责任务抢占、状态更新与任务查询。
type JobStore interface {
	Claim(ctx context.Context, filter ClaimFilter) (*model.Job, error)
	Release(ctx context.Context, id uuid.UUID) error
	MarkSucceeded(ctx context.Context, id uuid.UUID) error
	MarkSucceededWithResult(ctx context.Context, id uuid.UUID, result []byte) error
	MarkFailed(ctx context.Context, id uuid.UUID, message string) error
	MarkFailedWithResult(ctx context.Context, id uuid.UUID, message string, result []byte) error
	Create(ctx context.Context, job *model.Job) error
	GetByID(ctx context.Context, id uuid.UUID) (*model.Job, error)
	Update(ctx context.Context, job *model.Job) error
	GetByIdempotencyKey(ctx context.Context, projectID uuid.UUID, key string) (*model.Job, error)
	// RequeueAfter 把任务退回 queued 并写入最早可再次被抢占的时间（退避重试，
	// C14-4）。Claim 只取 next_attempt_at 已到期的 queued 行。
	RequeueAfter(ctx context.Context, id uuid.UUID, delay time.Duration) error
}

// ClaimFilter 限制 SKIP LOCKED 抢到的行：本节点可处理的 type，以及 owner 匹配或为空。
// Types 为空表示不按类型过滤（便于未知类型仍被 Claim 后 Release）。
type ClaimFilter struct {
	NodeID uuid.UUID
	Types  []string
}

// JobRepo 是 PostgreSQL 实现，使用 SELECT ... FOR UPDATE SKIP LOCKED 在多副本间抢行。
type JobRepo struct {
	db *gorm.DB
}

// NewJobRepo 构造仓储。
func NewJobRepo(db *gorm.DB) *JobRepo {
	return &JobRepo{db: db}
}

// Claim 取出一条 queued 任务并标为 running。没有可抢行时返回 (nil, nil)。
func (r *JobRepo) Claim(ctx context.Context, filter ClaimFilter) (*model.Job, error) {
	if r == nil || r.db == nil {
		return nil, fmt.Errorf("job repo is nil")
	}
	var claimed *model.Job
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		now := time.Now().UTC()
		var job model.Job
		// SKIP LOCKED：其它事务已锁的 queued 行直接跳过，避免多 worker 阻塞互等。
		// next_attempt_at 过滤：退避重试的任务在到期前对 Claim 不可见。
		// owner_node_id：NULL 或等于本节点；type 在本地 handler 集合内（空 Types 不按类型过滤，便于未知类型 Release）。
		q := tx.Clauses(clause.Locking{Strength: "UPDATE", Options: "SKIP LOCKED"}).
			Where("status = ?", model.JobStatusQueued)
		q = q.Where("next_attempt_at IS NULL OR next_attempt_at <= ?", now)
		if filter.NodeID != uuid.Nil {
			q = q.Where("owner_node_id IS NULL OR owner_node_id = ?", filter.NodeID)
		}
		if len(filter.Types) > 0 {
			q = q.Where("type IN ?", filter.Types)
		}
		err := q.Order("created_at ASC").
			Take(&job).Error
		if err != nil {
			return err
		}
		if err := tx.Model(&job).Updates(map[string]any{
			"status":     model.JobStatusRunning,
			"started_at": now,
			"attempts":   job.Attempts + 1,
		}).Error; err != nil {
			return err
		}
		job.Status = model.JobStatusRunning
		job.StartedAt = &now
		job.Attempts++
		claimed = &job
		return nil
	})
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("claim job: %w", err)
	}
	return claimed, nil
}

// Release 把 running 任务退回 queued，供尚无 handler 时避免行卡死。
func (r *JobRepo) Release(ctx context.Context, id uuid.UUID) error {
	res := r.db.WithContext(ctx).Model(&model.Job{}).
		Where("id = ? AND status = ?", id, model.JobStatusRunning).
		Select("status", "started_at").
		Updates(map[string]any{
			"status":     model.JobStatusQueued,
			"started_at": nil,
		})
	if res.Error != nil {
		return fmt.Errorf("release job: %w", res.Error)
	}
	return nil
}

// RequeueAfter 把任务退回 queued 并写入退避到期时间（C14-4：webhook_deliver
// 失败重试）。返回 queued 行同时清空 started_at，Claim 在 next_attempt_at
// 到期前不会再次抢占该行。
func (r *JobRepo) RequeueAfter(ctx context.Context, id uuid.UUID, delay time.Duration) error {
	next := time.Now().UTC().Add(delay)
	res := r.db.WithContext(ctx).Model(&model.Job{}).
		Where("id = ?", id).
		Updates(map[string]any{
			"status":          model.JobStatusQueued,
			"started_at":      nil,
			"finished_at":     nil,
			"next_attempt_at": next,
		})
	if res.Error != nil {
		return fmt.Errorf("requeue job after delay: %w", res.Error)
	}
	return nil
}

// MarkSucceeded 将任务标为成功。
func (r *JobRepo) MarkSucceeded(ctx context.Context, id uuid.UUID) error {
	now := time.Now().UTC()
	if err := r.db.WithContext(ctx).Model(&model.Job{}).Where("id = ?", id).Updates(map[string]any{
		"status":      model.JobStatusSucceeded,
		"progress":    100,
		"finished_at": now,
	}).Error; err != nil {
		return fmt.Errorf("mark job succeeded: %w", err)
	}
	return nil
}

// MarkFailed 记录失败原因。
func (r *JobRepo) MarkFailed(ctx context.Context, id uuid.UUID, message string) error {
	now := time.Now().UTC()
	if err := r.db.WithContext(ctx).Model(&model.Job{}).Where("id = ?", id).Updates(map[string]any{
		"status":        model.JobStatusFailed,
		"error_message": message,
		"finished_at":   now,
	}).Error; err != nil {
		return fmt.Errorf("mark job failed: %w", err)
	}
	return nil
}

// MarkSucceededWithResult 将任务标为成功并记录结果 JSON。
func (r *JobRepo) MarkSucceededWithResult(ctx context.Context, id uuid.UUID, result []byte) error {
	now := time.Now().UTC()
	updates := map[string]any{
		"status":      model.JobStatusSucceeded,
		"progress":    100,
		"finished_at": now,
	}
	if len(result) > 0 {
		updates["result"] = result
	}
	if err := r.db.WithContext(ctx).Model(&model.Job{}).Where("id = ?", id).Updates(updates).Error; err != nil {
		return fmt.Errorf("mark job succeeded with result: %w", err)
	}
	return nil
}

// MarkFailedWithResult 记录失败原因与部分产出 JSON。
func (r *JobRepo) MarkFailedWithResult(ctx context.Context, id uuid.UUID, message string, result []byte) error {
	now := time.Now().UTC()
	updates := map[string]any{
		"status":        model.JobStatusFailed,
		"error_message": message,
		"finished_at":   now,
	}
	if len(result) > 0 {
		updates["result"] = result
	}
	if err := r.db.WithContext(ctx).Model(&model.Job{}).Where("id = ?", id).Updates(updates).Error; err != nil {
		return fmt.Errorf("mark job failed with result: %w", err)
	}
	return nil
}

// Create 插入一条新任务。
func (r *JobRepo) Create(ctx context.Context, job *model.Job) error {
	if r == nil || r.db == nil {
		return fmt.Errorf("job repo is nil")
	}
	if err := r.db.WithContext(ctx).Create(job).Error; err != nil {
		return fmt.Errorf("create job: %w", err)
	}
	return nil
}

// GetByID 根据 ID 查询任务。
func (r *JobRepo) GetByID(ctx context.Context, id uuid.UUID) (*model.Job, error) {
	if r == nil || r.db == nil {
		return nil, fmt.Errorf("job repo is nil")
	}
	var job model.Job
	if err := r.db.WithContext(ctx).Where("id = ?", id).Take(&job).Error; err != nil {
		return nil, err
	}
	return &job, nil
}

// Update 更新任务整行。
func (r *JobRepo) Update(ctx context.Context, job *model.Job) error {
	if r == nil || r.db == nil {
		return fmt.Errorf("job repo is nil")
	}
	if err := r.db.WithContext(ctx).Save(job).Error; err != nil {
		return fmt.Errorf("update job: %w", err)
	}
	return nil
}

// GetByIdempotencyKey 查找指定项目 24 小时内的幂等任务。
func (r *JobRepo) GetByIdempotencyKey(ctx context.Context, projectID uuid.UUID, key string) (*model.Job, error) {
	if r == nil || r.db == nil {
		return nil, fmt.Errorf("job repo is nil")
	}
	var job model.Job
	since := time.Now().UTC().Add(-24 * time.Hour)
	err := r.db.WithContext(ctx).
		Where("project_id = ? AND idempotency_key = ? AND created_at >= ?", projectID, key, since).
		Order("created_at DESC").
		Take(&job).Error
	if err != nil {
		return nil, err
	}
	return &job, nil
}
