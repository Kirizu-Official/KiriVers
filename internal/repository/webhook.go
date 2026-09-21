package repository

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/Kirizu-Official/KiriVers/internal/model"
)

// WebhookStore 负责 Publish webhook 投递记录（webhook_deliveries）的持久化（C14-3/C14-4）。
// 事件触发点只写行 + 入队（入队即返回，§11.5）；webhook_deliver worker 读写状态；
// 管理端 deliveries 查询端点做游标分页排障。
type WebhookStore interface {
	// CreateDelivery 插入一条 pending 投递记录。
	CreateDelivery(ctx context.Context, delivery *model.WebhookDelivery) error
	// GetDelivery 按 ID 读取投递记录。
	GetDelivery(ctx context.Context, id uuid.UUID) (*model.WebhookDelivery, error)
	// UpdateDelivery 回写投递状态（status/attempts/last_*）。
	UpdateDelivery(ctx context.Context, delivery *model.WebhookDelivery) error
	// ListDeliveries 按项目游标分页列出投递记录（created_at DESC, id DESC）。
	// cursor 为上一页最后一条的 ID；nil 表示第一页。limit <= 0 时取默认 50。
	ListDeliveries(ctx context.Context, projectID uuid.UUID, cursor *uuid.UUID, limit int) ([]model.WebhookDelivery, error)
}

// WebhookRepo 是 PostgreSQL 实现。
type WebhookRepo struct {
	db *gorm.DB
}

// NewWebhookRepo 构造仓储。
func NewWebhookRepo(db *gorm.DB) *WebhookRepo {
	return &WebhookRepo{db: db}
}

// CreateDelivery 插入一条投递记录。
func (r *WebhookRepo) CreateDelivery(ctx context.Context, delivery *model.WebhookDelivery) error {
	if err := r.db.WithContext(ctx).Create(delivery).Error; err != nil {
		return fmt.Errorf("create webhook delivery: %w", err)
	}
	return nil
}

// GetDelivery 按 ID 读取投递记录。
func (r *WebhookRepo) GetDelivery(ctx context.Context, id uuid.UUID) (*model.WebhookDelivery, error) {
	var d model.WebhookDelivery
	if err := r.db.WithContext(ctx).Where("id = ?", id).First(&d).Error; err != nil {
		return nil, err
	}
	return &d, nil
}

// UpdateDelivery 回写投递状态。
func (r *WebhookRepo) UpdateDelivery(ctx context.Context, delivery *model.WebhookDelivery) error {
	if err := r.db.WithContext(ctx).Save(delivery).Error; err != nil {
		return fmt.Errorf("update webhook delivery: %w", err)
	}
	return nil
}

// ListDeliveries 按 (created_at DESC, id DESC) 游标分页。cursor 为上一页末条的 ID：
// 先取该行的 (created_at, id)，再取严格排在其之前的行，保证翻页无重复无遗漏。
func (r *WebhookRepo) ListDeliveries(ctx context.Context, projectID uuid.UUID, cursor *uuid.UUID, limit int) ([]model.WebhookDelivery, error) {
	if limit <= 0 {
		limit = 50
	}
	q := r.db.WithContext(ctx).Where("project_id = ?", projectID)
	if cursor != nil {
		var last model.WebhookDelivery
		if err := r.db.WithContext(ctx).Where("id = ? AND project_id = ?", *cursor, projectID).First(&last).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil, fmt.Errorf("webhook delivery cursor not found")
			}
			return nil, fmt.Errorf("load webhook delivery cursor: %w", err)
		}
		q = q.Where("(created_at < ?) OR (created_at = ? AND id < ?)", last.CreatedAt, last.CreatedAt, last.ID)
	}
	var list []model.WebhookDelivery
	if err := q.Order("created_at DESC, id DESC").Limit(limit).Find(&list).Error; err != nil {
		return nil, fmt.Errorf("list webhook deliveries: %w", err)
	}
	return list, nil
}
