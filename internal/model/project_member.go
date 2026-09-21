package model

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

const (
	// ProjectMemberRoleOwner 项目拥有者：该项目全部管理能力，可维护本项目「管理员」。
	ProjectMemberRoleOwner = "owner"
	// ProjectMemberRoleAdmin 项目管理员：该项目全部管理能力，不可任命拥有者、不可管成员（除查看）。
	ProjectMemberRoleAdmin = "admin"
)

// ProjectMember 是后台账号与项目的成员关系。
//
// 用途：项目拥有者/管理员与平台管理员走同一登录。平台管理员不需要本表行即可进全部项目。
// 同一项目同一账号只能有一种角色。
//
// 关系：软引用 Project 与 Admin，无 FK（与 Job/Audit 同一口径）。
//
// 字段：
//   - ID：UUID 主键。
//   - ProjectID：项目。
//   - AdminID：admins.id。
//   - Role：owner | admin。
//   - CreatedAt / UpdatedAt：GORM 时间戳。
type ProjectMember struct {
	ID        uuid.UUID `gorm:"type:uuid;primaryKey" json:"id"`
	ProjectID uuid.UUID `gorm:"type:uuid;not null;uniqueIndex:idx_project_members_project_admin" json:"project_id"`
	AdminID   uuid.UUID `gorm:"type:uuid;not null;uniqueIndex:idx_project_members_project_admin;index:idx_project_members_admin" json:"admin_id"`
	Role      string    `gorm:"type:text;not null" json:"role"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// TableName 固定表名 project_members。
func (ProjectMember) TableName() string {
	return "project_members"
}

// BeforeCreate 补 UUID。
func (m *ProjectMember) BeforeCreate(_ *gorm.DB) error {
	if m.ID == uuid.Nil {
		id, err := uuid.NewRandom()
		if err != nil {
			return err
		}
		m.ID = id
	}
	return nil
}

// IsOwner 是否为拥有者角色。
func (m ProjectMember) IsOwner() bool {
	return m.Role == ProjectMemberRoleOwner
}
