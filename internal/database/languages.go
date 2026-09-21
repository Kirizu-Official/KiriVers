package database

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"gorm.io/gorm"

	"github.com/Kirizu-Official/KiriVers/internal/model"
)

// BackfillProjectLanguages 为尚无语言行的项目写入默认语言，并收获公告/changelog JSON 键。
// 仅在语言表为空时运行，幂等。历史 default_locale 为空或非法时才回落到 en。
func BackfillProjectLanguages(db *gorm.DB) error {
	if db == nil {
		return fmt.Errorf("database is nil")
	}
	var projects []model.Project
	if err := db.Find(&projects).Error; err != nil {
		return fmt.Errorf("list projects for language backfill: %w", err)
	}
	now := time.Now().UTC()
	for i := range projects {
		p := projects[i]
		var n int64
		if err := db.Model(&model.ProjectLanguage{}).Where("project_id = ?", p.ID).Count(&n).Error; err != nil {
			return fmt.Errorf("count languages for %s: %w", p.ID, err)
		}
		if n > 0 {
			continue
		}
		code := backfillDefaultCode(p.DefaultLocale)
		def := &model.ProjectLanguage{
			ProjectID: p.ID,
			Code:      code,
			IsDefault: true,
			SortOrder: 0,
			CreatedAt: now,
			UpdatedAt: now,
		}
		if err := db.Create(def).Error; err != nil {
			return fmt.Errorf("seed default language for %s: %w", p.ID, err)
		}

		var anns []model.Announcement
		if err := db.Where("project_id = ?", p.ID).Find(&anns).Error; err != nil {
			return fmt.Errorf("list announcements for language harvest: %w", err)
		}
		var vers []model.Version
		if err := db.Where("project_id = ?", p.ID).Find(&vers).Error; err != nil {
			return fmt.Errorf("list versions for language harvest: %w", err)
		}
		extras := trimLanguageHarvest(HarvestLocaleCodes(code, announcementKeys(anns), changelogKeys(vers)), 1)
		for i, extra := range extras {
			row := &model.ProjectLanguage{
				ProjectID: p.ID,
				Code:      extra,
				IsDefault: false,
				SortOrder: (i + 1) * 10,
				CreatedAt: now,
				UpdatedAt: now,
			}
			if err := db.Create(row).Error; err != nil {
				return fmt.Errorf("harvest language %s for %s: %w", extra, p.ID, err)
			}
		}
	}
	return nil
}

// HarvestLocaleCodes 从 JSON map 键中收集非默认语言 code（大小写不敏感去重，保留首次大小写）。
func HarvestLocaleCodes(defaultCode string, keySets ...[]string) []string {
	seen := map[string]string{}
	def := strings.ToLower(strings.TrimSpace(defaultCode))
	for _, keys := range keySets {
		for _, raw := range keys {
			code := strings.TrimSpace(raw)
			if code == "" {
				continue
			}
			lower := strings.ToLower(code)
			if def != "" && lower == def {
				continue
			}
			if _, ok := seen[lower]; ok {
				continue
			}
			if !model.ValidProjectLanguageCode(code) {
				continue
			}
			seen[lower] = code
		}
	}
	out := make([]string, 0, len(seen))
	for _, code := range seen {
		out = append(out, code)
	}
	sort.Strings(out)
	return out
}

func backfillDefaultCode(raw string) string {
	code := strings.TrimSpace(raw)
	if model.ValidProjectLanguageCode(code) {
		return code
	}
	return model.LegacyDefaultLocale
}

func trimLanguageHarvest(extras []string, already int) []string {
	remain := model.ProjectMaxLanguages - already
	if remain <= 0 {
		return nil
	}
	if len(extras) > remain {
		return extras[:remain]
	}
	return extras
}

func announcementKeys(rows []model.Announcement) []string {
	out := make([]string, 0, len(rows))
	for i := range rows {
		if code := strings.TrimSpace(rows[i].Language); code != "" {
			out = append(out, code)
		}
	}
	return out
}

func changelogKeys(rows []model.Version) []string {
	out := make([]string, 0)
	for i := range rows {
		for k := range rows[i].Changelog {
			out = append(out, k)
		}
	}
	return out
}

func ensureProjectLanguageCIIndex(db *gorm.DB) error {
	// GORM uniqueIndex 是大小写敏感的 (project_id, code)；再加 lower(code) 以满足大小写不敏感唯一。
	if err := db.Exec(`CREATE UNIQUE INDEX IF NOT EXISTS idx_project_languages_project_code_ci ON project_languages (project_id, lower(code))`).Error; err != nil {
		return fmt.Errorf("create project_languages case-insensitive unique index: %w", err)
	}
	return nil
}
