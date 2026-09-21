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
	_ Admin2FAStore = (*Admin2FARepo)(nil)
	_ Admin2FAStore = (*MemoryAdminStore)(nil)
)

// Admin2FAStore 持久化管理员 TOTP / Passkey / 恢复码。每次校验直连本接口，禁止经缓存。
type Admin2FAStore interface {
	GetTOTP(ctx context.Context, adminID uuid.UUID) (*model.AdminTOTP, error)
	UpsertTOTP(ctx context.Context, row *model.AdminTOTP) error
	DeleteTOTP(ctx context.Context, adminID uuid.UUID) error

	ListPasskeys(ctx context.Context, adminID uuid.UUID) ([]model.AdminPasskey, error)
	GetPasskey(ctx context.Context, adminID, id uuid.UUID) (*model.AdminPasskey, error)
	GetPasskeyByCredentialID(ctx context.Context, credentialID []byte) (*model.AdminPasskey, error)
	CreatePasskey(ctx context.Context, row *model.AdminPasskey) error
	UpdatePasskey(ctx context.Context, row *model.AdminPasskey) error
	DeletePasskey(ctx context.Context, adminID, id uuid.UUID) error
	DeletePasskeysByAdmin(ctx context.Context, adminID uuid.UUID) error

	ListRecoveryCodes(ctx context.Context, adminID uuid.UUID) ([]model.AdminRecoveryCode, error)
	ReplaceRecoveryCodes(ctx context.Context, adminID uuid.UUID, codes []model.AdminRecoveryCode) error
	MarkRecoveryUsed(ctx context.Context, id uuid.UUID, at time.Time) error
	DeleteRecoveryCodes(ctx context.Context, adminID uuid.UUID) error
}

// Admin2FARepo 是 PostgreSQL 实现。
type Admin2FARepo struct {
	db *gorm.DB
}

// NewAdmin2FARepo 构造 2FA 仓储。
func NewAdmin2FARepo(db *gorm.DB) *Admin2FARepo {
	return &Admin2FARepo{db: db}
}

func (r *Admin2FARepo) GetTOTP(ctx context.Context, adminID uuid.UUID) (*model.AdminTOTP, error) {
	var row model.AdminTOTP
	err := r.db.WithContext(ctx).First(&row, "admin_id = ?", adminID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	if err != nil {
		return nil, fmt.Errorf("get admin totp: %w", err)
	}
	return &row, nil
}

func (r *Admin2FARepo) UpsertTOTP(ctx context.Context, row *model.AdminTOTP) error {
	if err := r.db.WithContext(ctx).Save(row).Error; err != nil {
		return fmt.Errorf("upsert admin totp: %w", err)
	}
	return nil
}

func (r *Admin2FARepo) DeleteTOTP(ctx context.Context, adminID uuid.UUID) error {
	if err := r.db.WithContext(ctx).Delete(&model.AdminTOTP{}, "admin_id = ?", adminID).Error; err != nil {
		return fmt.Errorf("delete admin totp: %w", err)
	}
	return nil
}

func (r *Admin2FARepo) ListPasskeys(ctx context.Context, adminID uuid.UUID) ([]model.AdminPasskey, error) {
	var list []model.AdminPasskey
	if err := r.db.WithContext(ctx).Where("admin_id = ?", adminID).Order("created_at asc").Find(&list).Error; err != nil {
		return nil, fmt.Errorf("list admin passkeys: %w", err)
	}
	return list, nil
}

func (r *Admin2FARepo) GetPasskey(ctx context.Context, adminID, id uuid.UUID) (*model.AdminPasskey, error) {
	var row model.AdminPasskey
	err := r.db.WithContext(ctx).Where("admin_id = ? AND id = ?", adminID, id).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	if err != nil {
		return nil, fmt.Errorf("get admin passkey: %w", err)
	}
	return &row, nil
}

func (r *Admin2FARepo) GetPasskeyByCredentialID(ctx context.Context, credentialID []byte) (*model.AdminPasskey, error) {
	var row model.AdminPasskey
	err := r.db.WithContext(ctx).Where("credential_id = ?", credentialID).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	if err != nil {
		return nil, fmt.Errorf("get admin passkey by credential: %w", err)
	}
	return &row, nil
}

func (r *Admin2FARepo) CreatePasskey(ctx context.Context, row *model.AdminPasskey) error {
	if err := r.db.WithContext(ctx).Create(row).Error; err != nil {
		return fmt.Errorf("create admin passkey: %w", err)
	}
	return nil
}

func (r *Admin2FARepo) UpdatePasskey(ctx context.Context, row *model.AdminPasskey) error {
	if err := r.db.WithContext(ctx).Save(row).Error; err != nil {
		return fmt.Errorf("update admin passkey: %w", err)
	}
	return nil
}

func (r *Admin2FARepo) DeletePasskey(ctx context.Context, adminID, id uuid.UUID) error {
	res := r.db.WithContext(ctx).Where("admin_id = ? AND id = ?", adminID, id).Delete(&model.AdminPasskey{})
	if res.Error != nil {
		return fmt.Errorf("delete admin passkey: %w", res.Error)
	}
	if res.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

func (r *Admin2FARepo) DeletePasskeysByAdmin(ctx context.Context, adminID uuid.UUID) error {
	if err := r.db.WithContext(ctx).Where("admin_id = ?", adminID).Delete(&model.AdminPasskey{}).Error; err != nil {
		return fmt.Errorf("delete admin passkeys: %w", err)
	}
	return nil
}

func (r *Admin2FARepo) ListRecoveryCodes(ctx context.Context, adminID uuid.UUID) ([]model.AdminRecoveryCode, error) {
	var list []model.AdminRecoveryCode
	if err := r.db.WithContext(ctx).Where("admin_id = ?", adminID).Order("created_at asc").Find(&list).Error; err != nil {
		return nil, fmt.Errorf("list admin recovery codes: %w", err)
	}
	return list, nil
}

func (r *Admin2FARepo) ReplaceRecoveryCodes(ctx context.Context, adminID uuid.UUID, codes []model.AdminRecoveryCode) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("admin_id = ?", adminID).Delete(&model.AdminRecoveryCode{}).Error; err != nil {
			return fmt.Errorf("delete old recovery codes: %w", err)
		}
		if len(codes) == 0 {
			return nil
		}
		if err := tx.Create(&codes).Error; err != nil {
			return fmt.Errorf("insert recovery codes: %w", err)
		}
		return nil
	})
}

func (r *Admin2FARepo) MarkRecoveryUsed(ctx context.Context, id uuid.UUID, at time.Time) error {
	res := r.db.WithContext(ctx).Model(&model.AdminRecoveryCode{}).Where("id = ? AND used_at IS NULL", id).Updates(map[string]any{"used_at": at})
	if res.Error != nil {
		return fmt.Errorf("mark recovery used: %w", res.Error)
	}
	if res.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

func (r *Admin2FARepo) DeleteRecoveryCodes(ctx context.Context, adminID uuid.UUID) error {
	if err := r.db.WithContext(ctx).Where("admin_id = ?", adminID).Delete(&model.AdminRecoveryCode{}).Error; err != nil {
		return fmt.Errorf("delete admin recovery codes: %w", err)
	}
	return nil
}
