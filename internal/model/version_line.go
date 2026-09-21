package model

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

const (
	// VersionLineStatusPending 等待产物上传或处理。
	VersionLineStatusPending = "pending"
	// VersionLineStatusUploading 产物正在流式上传中。
	VersionLineStatusUploading = "uploading"
	// VersionLineStatusProcessing 产物正在解压、校验或构建归档中。
	VersionLineStatusProcessing = "processing"
	// VersionLineStatusReady 产物已就绪，可供下载与发版校验。
	VersionLineStatusReady = "ready"
	// VersionLineStatusFailed 产物处理或拆线失败。
	VersionLineStatusFailed = "failed"
	// VersionLineStatusDisabled 平台切片临时停用。
	VersionLineStatusDisabled = "disabled"
	// VersionLineStatusYanked 平台切片已被废弃/撤回，客户端视作不存在。
	VersionLineStatusYanked = "yanked"
)

// VersionLine 是 Version 在某一 (os, arch) 上的平台切片。
//
// 用途：存放对应平台切片的状态（pending/ready/disabled/yanked）、最低系统版本 /
// 最低 API 等级，以及平台专属说明（platform_notes）。无独立版本号与主 changelog。
//
// 关系：
//   - 归属 Version（VersionID）。
//   - 间接归属 Project（ProjectID 冗余字段供快速查询）。
//
// 字段：
//   - ID：UUID 主键。
//   - VersionID：所属 Version UUID。
//   - ProjectID：所属 Project UUID。
//   - OS：规范操作系统 slug（写入时通过 CanonicalOSWrite 规范化）。
//   - Arch：规范架构 slug（写入时通过 CanonicalArch 规范化）。
//   - Status：pending | ready | disabled | yanked，默认 pending。
//   - MinOS：该线最低操作系统版本（点分数字）；空表示不限制。
//   - MinAPILevel：该线最低 API 等级（非负整数）；nil 表示不限制。
//   - PlatformNotes：平台补充说明（非主 changelog）。
//   - RootHash：多文件模式下的 Root Hash（64 位小写十六进制 SHA-256），单文件模式留空。
//   - PacksReadyAt：系统预热（auto_delta，渠道 × delta_source_count）完成时间；
//     nil 时 check/feed 不可见该线。客户端动态打包不改写本字段。
//   - CreatedAt / UpdatedAt：时间戳。
type VersionLine struct {
	ID            uuid.UUID  `gorm:"type:uuid;primaryKey" json:"id"`
	VersionID     uuid.UUID  `gorm:"type:uuid;not null;uniqueIndex:idx_version_lines_version_os_arch;index:idx_version_lines_version_status,priority:1" json:"version_id"`
	ProjectID     uuid.UUID  `gorm:"type:uuid;index" json:"project_id"`
	OS            string     `gorm:"type:text;not null;uniqueIndex:idx_version_lines_version_os_arch" json:"os"`
	Arch          string     `gorm:"type:text;not null;uniqueIndex:idx_version_lines_version_os_arch" json:"arch"`
	Status        string     `gorm:"type:text;not null;default:pending;index:idx_version_lines_version_status,priority:2" json:"status"`
	MinOS         *string    `gorm:"type:text" json:"min_os,omitempty"`
	MinAPILevel   *int       `json:"min_api_level,omitempty"`
	PlatformNotes string     `gorm:"type:text" json:"platform_notes,omitempty"`
	RootHash      string     `gorm:"type:text" json:"root_hash,omitempty"`
	PacksReadyAt  *time.Time `json:"packs_ready_at,omitempty"`
	CreatedAt     time.Time  `json:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at"`
}

// TableName 固定表名 version_lines，后续任务不得改名。
func (VersionLine) TableName() string {
	return "version_lines"
}

// BeforeCreate 补 UUID 与默认 pending 状态。
func (l *VersionLine) BeforeCreate(_ *gorm.DB) error {
	if l.ID == uuid.Nil {
		l.ID = uuid.New()
	}
	if l.Status == "" {
		l.Status = VersionLineStatusPending
	}
	return nil
}
