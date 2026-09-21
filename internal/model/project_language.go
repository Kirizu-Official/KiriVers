package model

import (
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

var projectLanguageCodeRE = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]{1,31}$`)

const (
	// ProjectMaxLanguages 单项目语言行上限（与公告 locale 上限一致）。
	ProjectMaxLanguages = AnnouncementMaxLocales
	// ProjectLanguageMaxDisplayNameRunes 显示名最大 Unicode 字符数。
	ProjectLanguageMaxDisplayNameRunes = 64
	// LegacyDefaultLocale 仅用于 AutoMigrate 回填：历史行 default_locale 为空时写入 en。
	// 新项目创建不得回落到本常量。
	LegacyDefaultLocale = "en"
)

// ProjectLanguage 是项目内可编辑的语言列表行。
//
// 用途：约束公告 / changelog 作者侧可选的 locale；is_default 行同步到
// projects.default_locale，供客户端 BuildLocaleChain 回退。删除语言不改写
// 已存 JSON 文案。
//
// 关系：归属 Project（无外键，Channel 风格）。项目内 code 大小写不敏感唯一，
// 落库保留输入大小写（如 zh-CN）。
//
// 字段：
//   - ID：UUID 主键。
//   - ProjectID：所属项目；与 Code 组成 uniqueIndex idx_project_languages_project_code。
//   - Code：语言代码，2–32 字符，^[A-Za-z0-9][A-Za-z0-9_-]{1,31}$。
//   - DisplayName：运营显示名，可空；空时 UI 展示 code。
//   - SortOrder：列表排序，升序。
//   - IsDefault：项目内恰好一行 true。
//   - CreatedAt / UpdatedAt：GORM 时间戳。
type ProjectLanguage struct {
	ID          uuid.UUID `gorm:"type:uuid;primaryKey" json:"id"`
	ProjectID   uuid.UUID `gorm:"type:uuid;not null;uniqueIndex:idx_project_languages_project_code" json:"project_id"`
	Code        string    `gorm:"type:text;not null;uniqueIndex:idx_project_languages_project_code" json:"code"`
	DisplayName string    `gorm:"type:text;not null;default:''" json:"display_name"`
	SortOrder   int       `gorm:"not null" json:"sort_order"`
	IsDefault   bool      `gorm:"not null;default:false" json:"is_default"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// TableName 固定表名 project_languages。
func (ProjectLanguage) TableName() string {
	return "project_languages"
}

// BeforeCreate 补 UUID。
func (l *ProjectLanguage) BeforeCreate(_ *gorm.DB) error {
	if l.ID == uuid.Nil {
		l.ID = uuid.New()
	}
	return nil
}

// ValidProjectLanguageCode 判断 trim 后的 code 是否符合 2–32 字符规则。
func ValidProjectLanguageCode(code string) bool {
	return projectLanguageCodeRE.MatchString(strings.TrimSpace(code))
}
