package repository

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/Kirizu-Official/KiriVers/internal/model"
)

func (r *ProjectRepo) ListStoreListings(ctx context.Context, projectID uuid.UUID) ([]model.StoreListing, error) {
	var list []model.StoreListing
	if err := r.db.WithContext(ctx).
		Where("project_id = ?", projectID).
		Order("protocol ASC, slug ASC").
		Find(&list).Error; err != nil {
		return nil, fmt.Errorf("list store listings: %w", err)
	}
	return list, nil
}

func (r *ProjectRepo) GetStoreListing(ctx context.Context, projectID uuid.UUID, protocol, slug string) (*model.StoreListing, error) {
	var row model.StoreListing
	err := r.db.WithContext(ctx).
		Where("project_id = ? AND protocol = ? AND slug = ?", projectID, protocol, slug).
		First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	if err != nil {
		return nil, fmt.Errorf("get store listing: %w", err)
	}
	return &row, nil
}

func (r *ProjectRepo) GetStoreListingByID(ctx context.Context, projectID, id uuid.UUID) (*model.StoreListing, error) {
	var row model.StoreListing
	err := r.db.WithContext(ctx).
		Where("project_id = ? AND id = ?", projectID, id).
		First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	if err != nil {
		return nil, fmt.Errorf("get store listing by id: %w", err)
	}
	return &row, nil
}

func (r *ProjectRepo) CreateStoreListing(ctx context.Context, listing *model.StoreListing) error {
	if err := r.db.WithContext(ctx).Create(listing).Error; err != nil {
		return fmt.Errorf("create store listing: %w", err)
	}
	return nil
}

func (r *ProjectRepo) SaveStoreListing(ctx context.Context, listing *model.StoreListing) error {
	if err := r.db.WithContext(ctx).Save(listing).Error; err != nil {
		return fmt.Errorf("save store listing: %w", err)
	}
	return nil
}

func (r *ProjectRepo) DeleteStoreListing(ctx context.Context, projectID, id uuid.UUID) error {
	res := r.db.WithContext(ctx).Where("project_id = ? AND id = ?", projectID, id).Delete(&model.StoreListing{})
	if res.Error != nil {
		return fmt.Errorf("delete store listing: %w", res.Error)
	}
	if res.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

func (r *ProjectRepo) HasEnabledStoreListing(ctx context.Context, projectID uuid.UUID, protocol string) (bool, error) {
	var n int64
	if err := r.db.WithContext(ctx).Model(&model.StoreListing{}).
		Where("project_id = ? AND protocol = ? AND enabled = ?", projectID, strings.ToLower(protocol), true).
		Count(&n).Error; err != nil {
		return false, fmt.Errorf("count enabled store listings: %w", err)
	}
	return n > 0, nil
}
