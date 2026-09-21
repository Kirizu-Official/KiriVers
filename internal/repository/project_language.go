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

func (r *ProjectRepo) ListLanguages(ctx context.Context, projectID uuid.UUID) ([]model.ProjectLanguage, error) {
	var list []model.ProjectLanguage
	if err := r.db.WithContext(ctx).Where("project_id = ?", projectID).
		Order("sort_order ASC, code ASC").Find(&list).Error; err != nil {
		return nil, fmt.Errorf("list languages: %w", err)
	}
	return list, nil
}

func (r *ProjectRepo) GetLanguage(ctx context.Context, projectID uuid.UUID, code string) (*model.ProjectLanguage, error) {
	var row model.ProjectLanguage
	err := r.db.WithContext(ctx).
		Where("project_id = ? AND lower(code) = lower(?)", projectID, strings.TrimSpace(code)).
		First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	if err != nil {
		return nil, fmt.Errorf("get language: %w", err)
	}
	return &row, nil
}

func (r *ProjectRepo) CountLanguages(ctx context.Context, projectID uuid.UUID) (int64, error) {
	var n int64
	if err := r.db.WithContext(ctx).Model(&model.ProjectLanguage{}).
		Where("project_id = ?", projectID).Count(&n).Error; err != nil {
		return 0, fmt.Errorf("count languages: %w", err)
	}
	return n, nil
}

func (r *ProjectRepo) CreateLanguage(ctx context.Context, lang *model.ProjectLanguage) error {
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if lang.IsDefault {
			if err := clearLanguageDefaults(tx, lang.ProjectID); err != nil {
				return err
			}
		}
		if err := tx.Create(lang).Error; err != nil {
			return err
		}
		if lang.IsDefault {
			return syncProjectDefaultLocale(tx, lang.ProjectID, lang.Code)
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("create language: %w", err)
	}
	return nil
}

func (r *ProjectRepo) SaveLanguage(ctx context.Context, lang *model.ProjectLanguage) error {
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if lang.IsDefault {
			if err := clearLanguageDefaults(tx, lang.ProjectID); err != nil {
				return err
			}
			lang.IsDefault = true
		}
		if err := tx.Save(lang).Error; err != nil {
			return err
		}
		if lang.IsDefault {
			return syncProjectDefaultLocale(tx, lang.ProjectID, lang.Code)
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("save language: %w", err)
	}
	return nil
}

func (r *ProjectRepo) DeleteLanguage(ctx context.Context, projectID uuid.UUID, code string) error {
	res := r.db.WithContext(ctx).
		Where("project_id = ? AND lower(code) = lower(?)", projectID, strings.TrimSpace(code)).
		Delete(&model.ProjectLanguage{})
	if res.Error != nil {
		return fmt.Errorf("delete language: %w", res.Error)
	}
	if res.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

func clearLanguageDefaults(tx *gorm.DB, projectID uuid.UUID) error {
	return tx.Model(&model.ProjectLanguage{}).
		Where("project_id = ?", projectID).
		Update("is_default", false).Error
}

func syncProjectDefaultLocale(tx *gorm.DB, projectID uuid.UUID, code string) error {
	return tx.Model(&model.Project{}).
		Where("id = ?", projectID).
		Update("default_locale", code).Error
}
