package model

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Admin 是后台登录账号（平台管理员或仅项目成员）。
//
// 用途：同一 /login 与 2FA。平台管理员是超管，不必写入 project_members。
// kirivers admin 与 POST /admins 创建的账号 IsPlatformAdmin=true。
// 成员流程创建的账号为 false，只看见有成员关系的项目。
//
// 关系：可不归属任何 Project；项目角色见 ProjectMember。
//
// 字段：
//   - ID：UUID 主键，应用侧生成，避免依赖 PostgreSQL 扩展默认值。
//   - Username：登录名，实例内唯一。
//   - PasswordHash：bcrypt 哈希，API/CLI 列表永不输出。
//   - IsPlatformAdmin：是否平台管理员。列默认 true，AutoMigrate 后已有行保持超管。
//   - LastLoginAt / LastLoginIP：最近一次成功 HTTP 登录的 UTC 时间与客户端 IP；CLI 创建后尚未登录时为 null。失败登录不更新。
//   - CreatedAt / UpdatedAt：GORM 时间戳。
type Admin struct {
	ID              uuid.UUID  `gorm:"type:uuid;primaryKey" json:"id"`
	Username        string     `gorm:"type:text;uniqueIndex;not null" json:"username"`
	PasswordHash    string     `gorm:"type:text;not null" json:"-"`
	IsPlatformAdmin bool       `gorm:"not null;default:true" json:"is_platform_admin"`
	LastLoginAt     *time.Time `json:"last_login_at"`
	LastLoginIP     *string    `gorm:"type:text" json:"last_login_ip"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
}

// TableName 固定表名，避免 GORM 复数推断在多语言环境下漂移。
func (Admin) TableName() string {
	return "admins"
}

// BeforeCreate 在插入前补 UUID。
func (a *Admin) BeforeCreate(_ *gorm.DB) error {
	if a.ID == uuid.Nil {
		a.ID = uuid.New()
	}
	return nil
}
