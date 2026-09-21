package repository

import (
	"context"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/Kirizu-Official/KiriVers/internal/model"
)

func cloneLanguage(row *model.ProjectLanguage) *model.ProjectLanguage {
	if row == nil {
		return nil
	}
	cp := *row
	return &cp
}

func (m *MemoryProjectStore) ListLanguages(_ context.Context, projectID uuid.UUID) ([]model.ProjectLanguage, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]model.ProjectLanguage, 0)
	for _, row := range m.languages {
		if row.ProjectID == projectID {
			out = append(out, *cloneLanguage(row))
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].SortOrder != out[j].SortOrder {
			return out[i].SortOrder < out[j].SortOrder
		}
		return out[i].Code < out[j].Code
	})
	return out, nil
}

func (m *MemoryProjectStore) GetLanguage(_ context.Context, projectID uuid.UUID, code string) (*model.ProjectLanguage, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	row := m.languageLocked(projectID, code)
	if row == nil {
		return nil, gorm.ErrRecordNotFound
	}
	return cloneLanguage(row), nil
}

func (m *MemoryProjectStore) CountLanguages(_ context.Context, projectID uuid.UUID) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var n int64
	for _, row := range m.languages {
		if row.ProjectID == projectID {
			n++
		}
	}
	return n, nil
}

func (m *MemoryProjectStore) CreateLanguage(_ context.Context, lang *model.ProjectLanguage) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := lang.BeforeCreate(nil); err != nil {
		return err
	}
	now := time.Now().UTC()
	if lang.CreatedAt.IsZero() {
		lang.CreatedAt = now
	}
	lang.UpdatedAt = now
	if m.languageLocked(lang.ProjectID, lang.Code) != nil {
		return gorm.ErrDuplicatedKey
	}
	if lang.IsDefault {
		m.clearLanguageDefaultsLocked(lang.ProjectID)
		m.setProjectDefaultLocaleLocked(lang.ProjectID, lang.Code)
	}
	m.languages[lang.ID] = cloneLanguage(lang)
	return nil
}

func (m *MemoryProjectStore) SaveLanguage(_ context.Context, lang *model.ProjectLanguage) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	old, ok := m.languages[lang.ID]
	if !ok {
		return gorm.ErrRecordNotFound
	}
	lang.UpdatedAt = time.Now().UTC()
	if lang.CreatedAt.IsZero() {
		lang.CreatedAt = old.CreatedAt
	}
	if lang.IsDefault {
		m.clearLanguageDefaultsLocked(lang.ProjectID)
		lang.IsDefault = true
		m.setProjectDefaultLocaleLocked(lang.ProjectID, lang.Code)
	}
	m.languages[lang.ID] = cloneLanguage(lang)
	return nil
}

func (m *MemoryProjectStore) DeleteLanguage(_ context.Context, projectID uuid.UUID, code string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	row := m.languageLocked(projectID, code)
	if row == nil {
		return gorm.ErrRecordNotFound
	}
	delete(m.languages, row.ID)
	return nil
}

func (m *MemoryProjectStore) languageLocked(projectID uuid.UUID, code string) *model.ProjectLanguage {
	want := strings.ToLower(strings.TrimSpace(code))
	for _, row := range m.languages {
		if row.ProjectID == projectID && strings.ToLower(row.Code) == want {
			return row
		}
	}
	return nil
}

func (m *MemoryProjectStore) clearLanguageDefaultsLocked(projectID uuid.UUID) {
	for _, row := range m.languages {
		if row.ProjectID == projectID {
			row.IsDefault = false
		}
	}
}

func (m *MemoryProjectStore) setProjectDefaultLocaleLocked(projectID uuid.UUID, code string) {
	if p, ok := m.projects[projectID]; ok && p != nil {
		p.DefaultLocale = code
	}
}
