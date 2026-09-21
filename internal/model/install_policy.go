package model

import (
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

const (
	// MaxInstallPolicyRules 是每个 (project_id, channel_id, os, arch) 作用域最多允许的路径规则数。
	MaxInstallPolicyRules = 256
)

// InstallPolicyRule 是项目或渠道在某一平台矩阵 (os, arch) 上的路径安装策略模板。
//
// 用途：
//
//	运营按「项目 × 渠道 × 平台」维护多文件路径的 OVERWRITE / KEEP_IF_EXISTS。
//	新建多文件 Manifest 时按 D1 叠加套用；策略只作为 Manifest 元数据，不写入 zip。
//	单次 Manifest PUT 可覆盖个别路径，且不写回本表。
//
// 关系：
//   - 软引用 Project（ProjectID），无 FK（与 Job / Audit 相同）。
//   - ChannelID 为 uuid.Nil 表示项目作用域；非 Nil 为该渠道自定义列表。
//     PostgreSQL 14 无法对 NULL 做 UNIQUE，故 ChannelID 必须 NOT NULL。
//   - 不引用 Version / VersionLine；改模板不回写已发布 Manifest。
//
// 字段：
//   - ID：UUID 主键，应用侧 BeforeCreate 生成 v4。
//   - ProjectID：所属项目。
//   - ChannelID：渠道 UUID；uuid.Nil = 项目作用域。
//   - OS / Arch：规范名（CanonicalOSWrite / CanonicalArch），写入时必须已在 platform_matrix。
//   - Path：NFC + 正斜杠相对路径（pathutil.NormalizeAndValidatePath）。
//   - InstallPolicy：OVERWRITE 或 KEEP_IF_EXISTS。
//   - CreatedAt / UpdatedAt：UTC 时间戳。
type InstallPolicyRule struct {
	ID            uuid.UUID `gorm:"type:uuid;primaryKey" json:"id"`
	ProjectID     uuid.UUID `gorm:"type:uuid;not null;uniqueIndex:idx_install_policy_rules_scope_path" json:"project_id"`
	ChannelID     uuid.UUID `gorm:"type:uuid;not null;uniqueIndex:idx_install_policy_rules_scope_path" json:"channel_id"`
	OS            string    `gorm:"type:text;not null;uniqueIndex:idx_install_policy_rules_scope_path" json:"os"`
	Arch          string    `gorm:"type:text;not null;uniqueIndex:idx_install_policy_rules_scope_path" json:"arch"`
	Path          string    `gorm:"type:text;not null;uniqueIndex:idx_install_policy_rules_scope_path" json:"path"`
	InstallPolicy string    `gorm:"type:text;not null" json:"install_policy"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

// TableName 固定表名 install_policy_rules。
func (InstallPolicyRule) TableName() string {
	return "install_policy_rules"
}

// BeforeCreate 补 UUID v4，并规范化安装策略。
func (r *InstallPolicyRule) BeforeCreate(_ *gorm.DB) error {
	if r.ID == uuid.Nil {
		id, err := uuid.NewRandom()
		if err != nil {
			return err
		}
		r.ID = id
	}
	r.InstallPolicy = strings.ToUpper(strings.TrimSpace(r.InstallPolicy))
	if r.InstallPolicy == "" {
		r.InstallPolicy = InstallPolicyOverwrite
	}
	return nil
}
