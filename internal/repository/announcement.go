package repository

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/Kirizu-Official/KiriVers/internal/model"
)

// AnnouncementStore 负责 announcements 表的持久化。查询一律带 project_id。
type AnnouncementStore interface {
	ListByProject(ctx context.Context, projectID uuid.UUID) ([]model.Announcement, error)
	ListIndexByProject(ctx context.Context, projectID uuid.UUID) ([]model.Announcement, error)
	GetByID(ctx context.Context, projectID, id uuid.UUID) (*model.Announcement, error)
	Create(ctx context.Context, row *model.Announcement) error
	Update(ctx context.Context, row *model.Announcement) error
	UpdateStatus(ctx context.Context, projectID, id uuid.UUID, status string) error
	Delete(ctx context.Context, projectID, id uuid.UUID) error
	Reorder(ctx context.Context, projectID uuid.UUID, ids []uuid.UUID) error
	MaxSortOrder(ctx context.Context, projectID uuid.UUID) (int, error)
	CountByProject(ctx context.Context, projectID uuid.UUID) (int, error)
}

// AnnouncementRepo 是 PostgreSQL 实现。
type AnnouncementRepo struct {
	db *gorm.DB
}

// NewAnnouncementRepo 构造仓储。
func NewAnnouncementRepo(db *gorm.DB) *AnnouncementRepo {
	return &AnnouncementRepo{db: db}
}

func announcementOrder(db *gorm.DB) *gorm.DB {
	return db.Order("sort_order ASC, created_at ASC, id ASC")
}

// ListByProject 返回项目内全部公告（含草稿、窗外项与正文），按 sort_order、created_at、id。
func (r *AnnouncementRepo) ListByProject(ctx context.Context, projectID uuid.UUID) ([]model.Announcement, error) {
	return r.listByProject(ctx, projectID, false)
}

// ListIndexByProject 返回管理端索引行：不 SELECT content，避免列表把 Markdown 正文打满。
func (r *AnnouncementRepo) ListIndexByProject(ctx context.Context, projectID uuid.UUID) ([]model.Announcement, error) {
	return r.listByProject(ctx, projectID, true)
}

func (r *AnnouncementRepo) listByProject(ctx context.Context, projectID uuid.UUID, omitContent bool) ([]model.Announcement, error) {
	q := r.db.WithContext(ctx).Where("project_id = ?", projectID)
	if omitContent {
		q = q.Omit("Content")
	}
	var list []model.Announcement
	if err := announcementOrder(q).Find(&list).Error; err != nil {
		return nil, fmt.Errorf("list announcements: %w", err)
	}
	return list, nil
}

// GetByID 按项目与 ID 读取一条公告。
func (r *AnnouncementRepo) GetByID(ctx context.Context, projectID, id uuid.UUID) (*model.Announcement, error) {
	var row model.Announcement
	if err := r.db.WithContext(ctx).Where("id = ? AND project_id = ?", id, projectID).First(&row).Error; err != nil {
		return nil, err
	}
	return &row, nil
}

// Create 插入一条公告。
func (r *AnnouncementRepo) Create(ctx context.Context, row *model.Announcement) error {
	if err := r.db.WithContext(ctx).Create(row).Error; err != nil {
		return fmt.Errorf("create announcement: %w", err)
	}
	return nil
}

// Update 全量回写一条公告（含空 OS/Arch 与空时间窗）。
func (r *AnnouncementRepo) Update(ctx context.Context, row *model.Announcement) error {
	if err := r.db.WithContext(ctx).Save(row).Error; err != nil {
		return fmt.Errorf("update announcement: %w", err)
	}
	return nil
}

// UpdateStatus 只改 status，供到期 scheduled 惰性翻转；避免索引行缺 content 时 Save 清空正文。
func (r *AnnouncementRepo) UpdateStatus(ctx context.Context, projectID, id uuid.UUID, status string) error {
	res := r.db.WithContext(ctx).Model(&model.Announcement{}).
		Where("id = ? AND project_id = ?", id, projectID).
		Update("status", status)
	if res.Error != nil {
		return fmt.Errorf("update announcement status: %w", res.Error)
	}
	if res.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

// Delete 硬删除。
func (r *AnnouncementRepo) Delete(ctx context.Context, projectID, id uuid.UUID) error {
	res := r.db.WithContext(ctx).Where("id = ? AND project_id = ?", id, projectID).Delete(&model.Announcement{})
	if res.Error != nil {
		return fmt.Errorf("delete announcement: %w", res.Error)
	}
	if res.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

// Reorder 按 ids 顺序把 sort_order 写成 0..n-1。调用方须保证 ids 是项目全部公告的排列。
func (r *AnnouncementRepo) Reorder(ctx context.Context, projectID uuid.UUID, ids []uuid.UUID) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for i, id := range ids {
			res := tx.Model(&model.Announcement{}).Where("id = ? AND project_id = ?", id, projectID).Update("sort_order", i)
			if res.Error != nil {
				return fmt.Errorf("reorder announcement: %w", res.Error)
			}
			if res.RowsAffected != 1 {
				return gorm.ErrRecordNotFound
			}
		}
		return nil
	})
}

// MaxSortOrder 返回项目当前最大 sort_order；无行时返回 -1。
func (r *AnnouncementRepo) MaxSortOrder(ctx context.Context, projectID uuid.UUID) (int, error) {
	var max *int
	if err := r.db.WithContext(ctx).Model(&model.Announcement{}).
		Where("project_id = ?", projectID).
		Select("MAX(sort_order)").
		Scan(&max).Error; err != nil {
		return 0, fmt.Errorf("max announcement sort_order: %w", err)
	}
	if max == nil {
		return -1, nil
	}
	return *max, nil
}

// CountByProject 返回项目公告条数。
func (r *AnnouncementRepo) CountByProject(ctx context.Context, projectID uuid.UUID) (int, error) {
	var n int64
	if err := r.db.WithContext(ctx).Model(&model.Announcement{}).Where("project_id = ?", projectID).Count(&n).Error; err != nil {
		return 0, fmt.Errorf("count announcements: %w", err)
	}
	return int(n), nil
}
