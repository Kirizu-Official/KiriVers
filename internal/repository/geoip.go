package repository

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/Kirizu-Official/KiriVers/internal/model"
)

// GeoipStore 平台 GeoIP 库元数据。不含对象字节。
type GeoipStore interface {
	List(ctx context.Context) ([]model.GeoipDatabase, error)
	GetByID(ctx context.Context, id uuid.UUID) (*model.GeoipDatabase, error)
	Count(ctx context.Context) (int64, error)
	MaxRank(ctx context.Context) (int, error)
	Create(ctx context.Context, row *model.GeoipDatabase) error
	Save(ctx context.Context, row *model.GeoipDatabase) error
	Delete(ctx context.Context, id uuid.UUID) error
}

// GeoipRepo 是 PostgreSQL 实现。
type GeoipRepo struct {
	db *gorm.DB
}

// NewGeoipRepo 构造仓储。
func NewGeoipRepo(db *gorm.DB) *GeoipRepo {
	return &GeoipRepo{db: db}
}

func (r *GeoipRepo) List(ctx context.Context) ([]model.GeoipDatabase, error) {
	var list []model.GeoipDatabase
	if err := r.db.WithContext(ctx).Order("rank ASC, created_at ASC").Find(&list).Error; err != nil {
		return nil, fmt.Errorf("list geoip databases: %w", err)
	}
	return list, nil
}

func (r *GeoipRepo) GetByID(ctx context.Context, id uuid.UUID) (*model.GeoipDatabase, error) {
	var row model.GeoipDatabase
	err := r.db.WithContext(ctx).Where("id = ?", id).First(&row).Error
	if err != nil {
		return nil, err
	}
	return &row, nil
}

func (r *GeoipRepo) Count(ctx context.Context) (int64, error) {
	var n int64
	if err := r.db.WithContext(ctx).Model(&model.GeoipDatabase{}).Count(&n).Error; err != nil {
		return 0, fmt.Errorf("count geoip databases: %w", err)
	}
	return n, nil
}

func (r *GeoipRepo) MaxRank(ctx context.Context) (int, error) {
	var max *int
	if err := r.db.WithContext(ctx).Model(&model.GeoipDatabase{}).Select("MAX(rank)").Scan(&max).Error; err != nil {
		return 0, fmt.Errorf("max geoip rank: %w", err)
	}
	if max == nil {
		return 0, nil
	}
	return *max, nil
}

func (r *GeoipRepo) Create(ctx context.Context, row *model.GeoipDatabase) error {
	if err := r.db.WithContext(ctx).Create(row).Error; err != nil {
		return fmt.Errorf("create geoip database: %w", err)
	}
	return nil
}

func (r *GeoipRepo) Save(ctx context.Context, row *model.GeoipDatabase) error {
	if err := r.db.WithContext(ctx).Save(row).Error; err != nil {
		return fmt.Errorf("save geoip database: %w", err)
	}
	return nil
}

func (r *GeoipRepo) Delete(ctx context.Context, id uuid.UUID) error {
	res := r.db.WithContext(ctx).Where("id = ?", id).Delete(&model.GeoipDatabase{})
	if res.Error != nil {
		return fmt.Errorf("delete geoip database: %w", res.Error)
	}
	if res.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}
