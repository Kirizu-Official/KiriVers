package model

import (
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

const (
	// InstallPolicyOverwrite 覆盖安装策略（默认，进行完整性校验）。
	InstallPolicyOverwrite = "OVERWRITE"
	// InstallPolicyKeepIfExists 若本地已存在则跳过覆盖（不执行完整性校验，integrity_check=false）。
	InstallPolicyKeepIfExists = "KEEP_IF_EXISTS"
)

// ManifestEntry 表示多文件 Version Line 中的单个文件清单条目（C06-1, §6.1）。
//
// 用途：
//   记录多文件部署场景中各文件的规范化相对路径、大小、SHA-256、MD5、安装策略与完整性校验要求。
//
// 关系：
//   - 归属 Project（ProjectID，冗余索引供按项目检索）
//   - 归属 Version（VersionID，冗余索引供按版本检索）
//   - 归属 VersionLine（VersionLineID，平台切片主归属）
//
// 字段：
//   - ID：UUID 主键。
//   - ProjectID：所属项目 UUID。
//   - VersionID：所属版本 UUID。
//   - VersionLineID：所属平台切片 UUID。
//   - Path：Unicode NFC 规范化相对路径（正斜杠，无前导/，无 .. 段）。
//   - Size：文件字节数（非负整数）。
//   - SHA256：64 位小写十六进制 SHA-256 校验码。
//   - MD5：32 位小写十六进制 MD5 校验码（兼容旧设备/工具）。
//   - InstallPolicy：安装策略（OVERWRITE | KEEP_IF_EXISTS）。
//   - IntegrityCheck：完整性检查开关；OVERWRITE 为 true，KEEP_IF_EXISTS 为 false。
//   - CreatedAt / UpdatedAt：时间戳。
type ManifestEntry struct {
	ID             uuid.UUID `gorm:"type:uuid;primaryKey" json:"id"`
	ProjectID      uuid.UUID `gorm:"type:uuid;not null;index" json:"project_id"`
	VersionID      uuid.UUID `gorm:"type:uuid;not null;index" json:"version_id"`
	VersionLineID  uuid.UUID `gorm:"type:uuid;not null;uniqueIndex:idx_manifest_entries_line_path" json:"version_line_id"`
	Path           string    `gorm:"type:text;not null;uniqueIndex:idx_manifest_entries_line_path" json:"path"`
	Size           int64     `gorm:"not null" json:"size"`
	SHA256         string    `gorm:"type:text;not null" json:"sha256"`
	MD5            string    `gorm:"type:text;not null" json:"md5"`
	InstallPolicy  string    `gorm:"type:text;not null;default:OVERWRITE" json:"install_policy"`
	IntegrityCheck bool      `gorm:"not null;default:true" json:"integrity_check"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

// TableName 固定表名 manifest_entries。
func (ManifestEntry) TableName() string {
	return "manifest_entries"
}

// GetPath 实现 pathutil.RootHashItem 接口。
func (m ManifestEntry) GetPath() string {
	return m.Path
}

// GetSize 实现 pathutil.RootHashItem 接口。
func (m ManifestEntry) GetSize() int64 {
	return m.Size
}

// GetSHA256 实现 pathutil.RootHashItem 接口。
func (m ManifestEntry) GetSHA256() string {
	return m.SHA256
}

// GetInstallPolicy 实现 pathutil.RootHashItem 接口。
func (m ManifestEntry) GetInstallPolicy() string {
	return m.InstallPolicy
}

// BeforeCreate 补 UUID，设置默认安装策略，并根据策略修正 integrity_check 字段。
func (m *ManifestEntry) BeforeCreate(_ *gorm.DB) error {
	if m.ID == uuid.Nil {
		m.ID = uuid.New()
	}
	m.InstallPolicy = strings.ToUpper(strings.TrimSpace(m.InstallPolicy))
	if m.InstallPolicy == "" {
		m.InstallPolicy = InstallPolicyOverwrite
	}
	if m.InstallPolicy == InstallPolicyKeepIfExists {
		m.IntegrityCheck = false
	} else {
		m.IntegrityCheck = true
	}
	return nil
}

// BeforeUpdate 同步根据 InstallPolicy 修正 IntegrityCheck。
func (m *ManifestEntry) BeforeUpdate(_ *gorm.DB) error {
	m.InstallPolicy = strings.ToUpper(strings.TrimSpace(m.InstallPolicy))
	if m.InstallPolicy == "" {
		m.InstallPolicy = InstallPolicyOverwrite
	}
	if m.InstallPolicy == InstallPolicyKeepIfExists {
		m.IntegrityCheck = false
	} else {
		m.IntegrityCheck = true
	}
	return nil
}
