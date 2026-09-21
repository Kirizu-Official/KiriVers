package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/Kirizu-Official/KiriVers/internal/model"
)

var (
	_ AdminStore = (*AdminRepo)(nil)
	_ AdminStore = (*MemoryAdminStore)(nil)
)

// AdminStore 管理员持久化。不含密码策略。
type AdminStore interface {
	Create(ctx context.Context, admin *model.Admin) error
	GetByID(ctx context.Context, id uuid.UUID) (*model.Admin, error)
	GetByUsername(ctx context.Context, username string) (*model.Admin, error)
	List(ctx context.Context) ([]model.Admin, error)
	Save(ctx context.Context, admin *model.Admin) error
	UpdateLastLogin(ctx context.Context, id uuid.UUID, at time.Time, ip *string) error
	Delete(ctx context.Context, id uuid.UUID) error
	Count(ctx context.Context) (int64, error)
}

// AdminRepo 是 PostgreSQL 实现。
type AdminRepo struct {
	db *gorm.DB
}

// NewAdminRepo 构造仓储。
func NewAdminRepo(db *gorm.DB) *AdminRepo {
	return &AdminRepo{db: db}
}

func (r *AdminRepo) Create(ctx context.Context, admin *model.Admin) error {
	err := r.db.WithContext(ctx).Create(admin).Error
	if err != nil {
		return fmt.Errorf("create admin: %w", err)
	}
	return nil
}

func (r *AdminRepo) GetByID(ctx context.Context, id uuid.UUID) (*model.Admin, error) {
	var admin model.Admin
	err := r.db.WithContext(ctx).First(&admin, "id = ?", id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	if err != nil {
		return nil, fmt.Errorf("get admin by id: %w", err)
	}
	return &admin, nil
}

func (r *AdminRepo) GetByUsername(ctx context.Context, username string) (*model.Admin, error) {
	var admin model.Admin
	err := r.db.WithContext(ctx).Where("username = ?", username).First(&admin).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	if err != nil {
		return nil, fmt.Errorf("get admin by username: %w", err)
	}
	return &admin, nil
}

func (r *AdminRepo) List(ctx context.Context) ([]model.Admin, error) {
	var list []model.Admin
	if err := r.db.WithContext(ctx).Order("created_at asc").Find(&list).Error; err != nil {
		return nil, fmt.Errorf("list admins: %w", err)
	}
	return list, nil
}

func (r *AdminRepo) Save(ctx context.Context, admin *model.Admin) error {
	if err := r.db.WithContext(ctx).Save(admin).Error; err != nil {
		return fmt.Errorf("save admin: %w", err)
	}
	return nil
}

// UpdateLastLogin 只更新上次登录时间与 IP（及 GORM 自动 updated_at），禁止整行 Save。
// 必须用 map：Updates(struct) 会跳过 nil IP，无法把 last_login_ip 写成 SQL NULL。
func (r *AdminRepo) UpdateLastLogin(ctx context.Context, id uuid.UUID, at time.Time, ip *string) error {
	// 用无类型 nil 写入 SQL NULL；map 里放 (*string)(nil) 时部分驱动/GORM 版本会跳过该列。
	updates := map[string]any{
		"last_login_at": at,
		"last_login_ip": nil,
	}
	if ip != nil {
		updates["last_login_ip"] = *ip
	}
	res := r.db.WithContext(ctx).Model(&model.Admin{}).Where("id = ?", id).Updates(updates)
	if res.Error != nil {
		return fmt.Errorf("update last login: %w", res.Error)
	}
	if res.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

func (r *AdminRepo) Delete(ctx context.Context, id uuid.UUID) error {
	res := r.db.WithContext(ctx).Delete(&model.Admin{}, "id = ?", id)
	if res.Error != nil {
		return fmt.Errorf("delete admin: %w", res.Error)
	}
	if res.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

func (r *AdminRepo) Count(ctx context.Context) (int64, error) {
	var n int64
	if err := r.db.WithContext(ctx).Model(&model.Admin{}).Count(&n).Error; err != nil {
		return 0, fmt.Errorf("count admins: %w", err)
	}
	return n, nil
}
