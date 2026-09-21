package database

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type legacyAnnouncementCopy struct {
	Title    string `json:"title"`
	Subtitle string `json:"subtitle"`
	Markdown string `json:"markdown"`
}

// expandedAnnouncementLocale 是旧 jsonb 字典拆出的一行文案。
type expandedAnnouncementLocale struct {
	Language string
	Title    string
	Subtitle string
	Content  string
}

type announcementJSONRow struct {
	ID        uuid.UUID  `gorm:"column:id"`
	ProjectID uuid.UUID  `gorm:"column:project_id"`
	Status    string     `gorm:"column:status"`
	SortOrder int        `gorm:"column:sort_order"`
	VersionID *uuid.UUID `gorm:"column:version_id"`
	OS        string     `gorm:"column:os"`
	Arch      string     `gorm:"column:arch"`
	StartsAt  *time.Time `gorm:"column:starts_at"`
	EndsAt    *time.Time `gorm:"column:ends_at"`
	CreatedAt time.Time  `gorm:"column:created_at"`
	UpdatedAt time.Time  `gorm:"column:updated_at"`
	Raw       []byte     `gorm:"column:content_jsonb"`
}

// renameAnnouncementJSONColumn 在 AutoMigrate 之前把 jsonb content 改名为 content_jsonb，
// 避免 GORM 把新的 text content 当成同名列而跳过 ADD。
func renameAnnouncementJSONColumn(db *gorm.DB) error {
	ok, err := tableExists(db, "announcements")
	if err != nil {
		return fmt.Errorf("announcement flatten: %w", err)
	}
	if !ok {
		return nil
	}
	hasJSON, err := tableHasColumn(db, "announcements", "content")
	if err != nil {
		return fmt.Errorf("announcement flatten: %w", err)
	}
	if !hasJSON {
		return nil
	}
	dataType, err := columnDataType(db, "announcements", "content")
	if err != nil {
		return fmt.Errorf("announcement flatten: %w", err)
	}
	if dataType != "jsonb" {
		return nil
	}
	hasLegacy, err := tableHasColumn(db, "announcements", "content_jsonb")
	if err != nil {
		return fmt.Errorf("announcement flatten: %w", err)
	}
	if hasLegacy {
		return nil
	}
	if err := db.Exec(`ALTER TABLE announcements RENAME COLUMN content TO content_jsonb`).Error; err != nil {
		return fmt.Errorf("rename announcements.content: %w", err)
	}
	return nil
}

func columnDataType(db *gorm.DB, table, col string) (string, error) {
	var dataType string
	err := db.Raw(`SELECT data_type FROM information_schema.columns WHERE table_schema = current_schema() AND table_name = ? AND column_name = ?`, table, col).Scan(&dataType).Error
	if err != nil {
		return "", err
	}
	return dataType, nil
}

// expandAnnouncementJSON 把 content_jsonb 拆成 language/title/subtitle/content 列后删除旧列。
// 每个 locale 键一行；第一条 UPDATE 原行，其余 INSERT。
func expandAnnouncementJSON(db *gorm.DB) error {
	ok, err := tableExists(db, "announcements")
	if err != nil {
		return fmt.Errorf("expand announcements: %w", err)
	}
	if !ok {
		return nil
	}
	hasLegacy, err := tableHasColumn(db, "announcements", "content_jsonb")
	if err != nil {
		return fmt.Errorf("expand announcements: %w", err)
	}
	if !hasLegacy {
		return nil
	}
	var rows []announcementJSONRow
	if err := db.Raw(`SELECT id, project_id, status, sort_order, version_id, os, arch, starts_at, ends_at, created_at, updated_at, content_jsonb FROM announcements`).Scan(&rows).Error; err != nil {
		return fmt.Errorf("list announcement jsonb: %w", err)
	}
	for i := range rows {
		if err := expandOneAnnouncementJSON(db, &rows[i]); err != nil {
			return err
		}
	}
	if err := db.Exec(`ALTER TABLE announcements DROP COLUMN IF EXISTS content_jsonb`).Error; err != nil {
		return fmt.Errorf("drop announcements.content_jsonb: %w", err)
	}
	return nil
}

func expandOneAnnouncementJSON(db *gorm.DB, row *announcementJSONRow) error {
	locales := expandLegacyAnnouncementMap(row.Raw)
	if len(locales) == 0 {
		locales = []expandedAnnouncementLocale{{}}
	}
	first := locales[0]
	if err := db.Exec(
		`UPDATE announcements SET language = ?, title = ?, subtitle = ?, content = ? WHERE id = ?`,
		first.Language, first.Title, first.Subtitle, first.Content, row.ID,
	).Error; err != nil {
		return fmt.Errorf("flatten announcement %s: %w", row.ID, err)
	}
	for _, loc := range locales[1:] {
		id, err := uuid.NewRandom()
		if err != nil {
			return fmt.Errorf("flatten announcement uuid: %w", err)
		}
		if err := db.Exec(
			`INSERT INTO announcements (
				id, project_id, status, sort_order, version_id, os, arch,
				language, title, subtitle, content, starts_at, ends_at, created_at, updated_at
			) VALUES (
				?, ?, ?, ?, ?, ?, ?,
				?, ?, ?, ?, ?, ?, ?
			)`,
			id, row.ProjectID, row.Status, row.SortOrder, row.VersionID, row.OS, row.Arch,
			loc.Language, loc.Title, loc.Subtitle, loc.Content, row.StartsAt, row.EndsAt, row.CreatedAt, row.UpdatedAt,
		).Error; err != nil {
			return fmt.Errorf("flatten announcement extra locale %s: %w", loc.Language, err)
		}
	}
	return nil
}

// expandLegacyAnnouncementMap 将旧 locale→copy 字典拆成稳定顺序的行（键字典序）。
func expandLegacyAnnouncementMap(raw []byte) []expandedAnnouncementLocale {
	if len(raw) == 0 {
		return nil
	}
	var m map[string]legacyAnnouncementCopy
	if err := json.Unmarshal(raw, &m); err != nil || len(m) == 0 {
		return nil
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := make([]expandedAnnouncementLocale, 0, len(keys))
	for _, k := range keys {
		entry := m[k]
		lang := strings.TrimSpace(k)
		title := strings.TrimSpace(entry.Title)
		if lang == "" && title == "" && strings.TrimSpace(entry.Subtitle) == "" && entry.Markdown == "" {
			continue
		}
		out = append(out, expandedAnnouncementLocale{
			Language: lang,
			Title:    title,
			Subtitle: entry.Subtitle,
			Content:  entry.Markdown,
		})
	}
	return out
}
