package repository

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/Kirizu-Official/KiriVers/internal/model"
)

// ProjectMediaStore 负责 project_media 表。查询一律带 project_id。
type ProjectMediaStore interface {
	Create(ctx context.Context, row *model.ProjectMedia) error
	GetByProjectAndID(ctx context.Context, projectID, id uuid.UUID) (*model.ProjectMedia, error)
}

// ProjectMediaRepo 是 PostgreSQL 实现。
type ProjectMediaRepo struct {
	db *gorm.DB
}

// NewProjectMediaRepo 构造仓储。
func NewProjectMediaRepo(db *gorm.DB) *ProjectMediaRepo {
	return &ProjectMediaRepo{db: db}
}

// Create 插入一条媒体元数据。
func (r *ProjectMediaRepo) Create(ctx context.Context, row *model.ProjectMedia) error {
	if err := r.db.WithContext(ctx).Create(row).Error; err != nil {
		return fmt.Errorf("create project media: %w", err)
	}
	return nil
}

// GetByProjectAndID 按项目与 ID 读取；不匹配则 gorm.ErrRecordNotFound。
func (r *ProjectMediaRepo) GetByProjectAndID(ctx context.Context, projectID, id uuid.UUID) (*model.ProjectMedia, error) {
	var row model.ProjectMedia
	if err := r.db.WithContext(ctx).Where("id = ? AND project_id = ?", id, projectID).First(&row).Error; err != nil {
		return nil, err
	}
	return &row, nil
}
