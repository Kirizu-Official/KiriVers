package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/Kirizu-Official/KiriVers/internal/model"
)

// LanguageWrite 是创建/PATCH 语言的输入；指针表示本次是否写入。
type LanguageWrite struct {
	Code        *string
	DisplayName *string
	SortOrder   *int
	IsDefault   *bool
}

// ListLanguages 按 sort_order、code 升序返回项目语言。
func (s *ProjectService) ListLanguages(ctx context.Context, projectID uuid.UUID) ([]model.ProjectLanguage, error) {
	return s.store.ListLanguages(ctx, projectID)
}

// CreateLanguage 新增语言行。重复 code（忽略大小写）→ ErrLanguageTaken。超过 16 行 → ErrInvalidLanguage。
func (s *ProjectService) CreateLanguage(ctx context.Context, projectID uuid.UUID, in LanguageWrite) (*model.ProjectLanguage, error) {
	if in.Code == nil {
		return nil, fmt.Errorf("%w: code is required", ErrInvalidLanguage)
	}
	code, err := normalizeLanguageCode(*in.Code)
	if err != nil {
		return nil, err
	}
	if _, err := s.store.GetLanguage(ctx, projectID, code); err == nil {
		return nil, ErrLanguageTaken
	} else if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	n, err := s.store.CountLanguages(ctx, projectID)
	if err != nil {
		return nil, err
	}
	if n >= int64(model.ProjectMaxLanguages) {
		return nil, fmt.Errorf("%w: at most %d languages", ErrInvalidLanguage, model.ProjectMaxLanguages)
	}
	lang := &model.ProjectLanguage{
		ProjectID: projectID,
		Code:      code,
	}
	if in.DisplayName != nil {
		name, err := normalizeLanguageDisplayName(*in.DisplayName)
		if err != nil {
			return nil, err
		}
		lang.DisplayName = name
	}
	if in.SortOrder != nil {
		lang.SortOrder = *in.SortOrder
	} else {
		lang.SortOrder = int((n + 1) * 10)
	}
	if in.IsDefault != nil {
		lang.IsDefault = *in.IsDefault
	}
	if n == 0 {
		lang.IsDefault = true
	}
	if err := s.store.CreateLanguage(ctx, lang); err != nil {
		if isUniqueViolation(err) {
			return nil, ErrLanguageTaken
		}
		return nil, err
	}
	s.invalidateProject(ctx, projectID)
	return lang, nil
}

// PatchLanguage 更新 display_name / sort_order / is_default。不可改 code。
func (s *ProjectService) PatchLanguage(ctx context.Context, projectID uuid.UUID, code string, in LanguageWrite) (*model.ProjectLanguage, error) {
	code, err := normalizeLanguageCode(code)
	if err != nil {
		return nil, err
	}
	lang, err := s.store.GetLanguage(ctx, projectID, code)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrLanguageNotFound
	}
	if err != nil {
		return nil, err
	}
	if in.DisplayName != nil {
		name, err := normalizeLanguageDisplayName(*in.DisplayName)
		if err != nil {
			return nil, err
		}
		lang.DisplayName = name
	}
	if in.SortOrder != nil {
		lang.SortOrder = *in.SortOrder
	}
	if in.IsDefault != nil {
		if !*in.IsDefault && lang.IsDefault {
			return nil, fmt.Errorf("%w: reassign the default first", ErrInvalidLanguage)
		}
		lang.IsDefault = *in.IsDefault
	}
	if err := s.store.SaveLanguage(ctx, lang); err != nil {
		return nil, err
	}
	s.invalidateProject(ctx, projectID)
	return lang, nil
}

// DeleteLanguage 删除语言。不可删除最后一行或当前默认。不改写公告/changelog JSON。
func (s *ProjectService) DeleteLanguage(ctx context.Context, projectID uuid.UUID, code string) error {
	code, err := normalizeLanguageCode(code)
	if err != nil {
		return err
	}
	lang, err := s.store.GetLanguage(ctx, projectID, code)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return ErrLanguageNotFound
	}
	if err != nil {
		return err
	}
	if lang.IsDefault {
		return ErrLanguageIsDefault
	}
	n, err := s.store.CountLanguages(ctx, projectID)
	if err != nil {
		return err
	}
	if n <= 1 {
		return ErrLanguageLast
	}
	if err := s.store.DeleteLanguage(ctx, projectID, code); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrLanguageNotFound
		}
		return err
	}
	s.invalidateProject(ctx, projectID)
	return nil
}

// SeedProjectLanguage 写入（或翻转）项目的默认语言行。创建项目与 PATCH default_locale 共用。幂等。
func (s *ProjectService) SeedProjectLanguage(ctx context.Context, projectID uuid.UUID, code string) (*model.ProjectLanguage, error) {
	code, err := normalizeLanguageCode(code)
	if err != nil {
		return nil, fmt.Errorf("%w: default_locale", ErrInvalidProjectSettings)
	}
	existing, err := s.store.GetLanguage(ctx, projectID, code)
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	if existing != nil {
		if existing.IsDefault {
			return existing, nil
		}
		on := true
		return s.PatchLanguage(ctx, projectID, existing.Code, LanguageWrite{IsDefault: &on})
	}
	on := true
	zero := 0
	return s.CreateLanguage(ctx, projectID, LanguageWrite{Code: &code, SortOrder: &zero, IsDefault: &on})
}

func normalizeLanguageCode(raw string) (string, error) {
	code := strings.TrimSpace(raw)
	if !model.ValidProjectLanguageCode(code) {
		return "", fmt.Errorf("%w: code must match [A-Za-z0-9][A-Za-z0-9_-]{1,31}", ErrInvalidLanguage)
	}
	return code, nil
}

func normalizeLanguageDisplayName(raw string) (string, error) {
	name := strings.TrimSpace(raw)
	if utf8.RuneCountInString(name) > model.ProjectLanguageMaxDisplayNameRunes {
		return "", fmt.Errorf("%w: display_name", ErrInvalidLanguage)
	}
	return name, nil
}
