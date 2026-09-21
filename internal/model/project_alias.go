package model

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// ProjectSlugAlias 保存项目改名后的旧 Slug，供 :project_ref 继续解析。
//
// 用途：GET/PATCH 等用旧名命中同一项目；不对 POST 做 301。过期后解析为 PROJECT_NOT_FOUND。
//
// 关系：归属 Project（ProjectID）。Slug 在「未过期 alias + 未删除项目 live slug」这一域内唯一，
// 由仓储层校验；过期行不占用该域，因此本表不做数据库 UNIQUE(slug)。
//
// 字段：
//   - ID：UUID 主键。
//   - ProjectID：所属项目。
//   - Slug：旧名。
//   - ExpiresAt：可空；NULL 表示永不过期。过期判定为 expires_at <= now。
type ProjectSlugAlias struct {
	ID        uuid.UUID  `gorm:"type:uuid;primaryKey" json:"id"`
	ProjectID uuid.UUID  `gorm:"type:uuid;not null;index" json:"project_id"`
	Slug      string     `gorm:"type:text;not null;index" json:"slug"`
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
	CreatedAt time.Time  `json:"created_at"`
}

// TableName 固定表名 project_slug_aliases。
func (ProjectSlugAlias) TableName() string {
	return "project_slug_aliases"
}

// BeforeCreate 补 UUID。
func (a *ProjectSlugAlias) BeforeCreate(_ *gorm.DB) error {
	if a.ID == uuid.Nil {
		a.ID = uuid.New()
	}
	return nil
}

// Unexpired 表示在 now 时刻仍可用于解析。
func (a *ProjectSlugAlias) Unexpired(now time.Time) bool {
	if a == nil {
		return false
	}
	return a.ExpiresAt == nil || a.ExpiresAt.After(now)
}
