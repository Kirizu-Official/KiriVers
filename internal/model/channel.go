package model

import (
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

const (
	// ChannelAlpha 系统渠道：最低稳定性。
	ChannelAlpha = "alpha"
	// ChannelBeta 系统渠道：中等稳定性。
	ChannelBeta = "beta"
	// ChannelStable 系统渠道：最高稳定性。
	ChannelStable = "stable"

	// ChannelRankAlpha 默认 stability_rank。
	ChannelRankAlpha = 10
	// ChannelRankBeta 默认 stability_rank。
	ChannelRankBeta = 20
	// ChannelRankStable 默认 stability_rank。
	ChannelRankStable = 30

	// ChannelNameMaxRunes 显示名最大 rune 数。
	ChannelNameMaxRunes = 128
)

// SystemChannelNames 是创建项目时三条系统渠道的显示名快照。
// 空字段回退为对应 slug（alpha / beta / stable）。
type SystemChannelNames struct {
	Alpha  string `json:"alpha"`
	Beta   string `json:"beta"`
	Stable string `json:"stable"`
}

// Channel 是项目内的分发渠道（stable/beta/alpha 或自定义）。
//
// 用途：约束 Version 所属轨道；stability_rank 越大越稳定，供选目标跨渠道升级。
// 系统渠道可关停（enabled=false）但不可删除。渠道 slug `lts` 只是普通渠道名，
// 不得据此给 Version 打 is_lts。
//
// Unlisted 与 TokenHash / TokenPlain 独立于 enabled：关停渠道对所有客户端不可见；
// 不可见渠道不出现在公开目录、也不作为跨渠道自动升级目标；
// 渠道 Token 保护候选，缺失/错误不使 check 403，只跳过该渠道。
//
// 关系：归属 Project；Version 通过 ChannelSlug / ChannelID 关联。
//
// 字段：
//   - ID：UUID 主键。
//   - ProjectID：所属项目。
//   - Name：显示名（创建时必填；系统渠道由项目创建快照）。
//   - Slug：渠道名，项目内唯一；自定义推荐 [a-z0-9-]{3,64}，不得含 `_`。
//   - StabilityRank：稳定性整数，系统默认 alpha=10 / beta=20 / stable=30。
//   - Enabled：是否对客户端可见；系统渠道也可关停。
//   - Unlisted：不可见；公开目录省略，且不可从其他渠道自动升入。
//   - TokenHash：可选渠道密钥的 SHA-256 hex；永不进入 JSON / ETag。
//   - TokenPlain：管理端可见的明文令牌；check 仍比对 TokenHash。仅有旧哈希、无法还原的行为空。
//   - System：是否为预置系统名（alpha/beta/stable），为 true 时禁止 DELETE。
//   - CreatedAt / UpdatedAt：GORM 时间戳。
type Channel struct {
	ID            uuid.UUID `gorm:"type:uuid;primaryKey" json:"id"`
	ProjectID     uuid.UUID `gorm:"type:uuid;not null;uniqueIndex:idx_channels_project_slug" json:"project_id"`
	Name          string    `gorm:"type:text;not null;default:''" json:"name"`
	Slug          string    `gorm:"type:text;not null;uniqueIndex:idx_channels_project_slug" json:"slug"`
	StabilityRank int       `gorm:"not null" json:"stability_rank"`
	Enabled       bool      `gorm:"not null;default:true" json:"enabled"`
	Unlisted      bool      `gorm:"not null;default:false" json:"unlisted"`
	TokenHash     string    `gorm:"type:text" json:"-"`
	TokenPlain    string    `gorm:"type:text;not null;default:''" json:"-"`
	System        bool      `gorm:"not null;default:false" json:"system"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

// TableName 固定表名 channels。
func (Channel) TableName() string {
	return "channels"
}

// BeforeCreate 补 UUID。
func (c *Channel) BeforeCreate(_ *gorm.DB) error {
	if c.ID == uuid.Nil {
		c.ID = uuid.New()
	}
	return nil
}

// TokenProtected 是否设置了渠道密钥（对外只暴露布尔，不暴露哈希）。
func (c Channel) TokenProtected() bool {
	return strings.TrimSpace(c.TokenHash) != ""
}

// IsSystemChannelSlug 是否为不可删除的系统渠道名。
func IsSystemChannelSlug(slug string) bool {
	switch slug {
	case ChannelAlpha, ChannelBeta, ChannelStable:
		return true
	default:
		return false
	}
}

// DisplayName 返回系统渠道显示名；空字段回退为 slug。
func (n SystemChannelNames) DisplayName(slug string) string {
	var raw string
	switch slug {
	case ChannelAlpha:
		raw = n.Alpha
	case ChannelBeta:
		raw = n.Beta
	case ChannelStable:
		raw = n.Stable
	}
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return slug
	}
	return raw
}

// SystemChannelSeeds 返回新项目应插入的三条系统渠道（调用方负责幂等写入）。
func SystemChannelSeeds(projectID uuid.UUID, names SystemChannelNames) []Channel {
	return []Channel{
		{ProjectID: projectID, Name: names.DisplayName(ChannelAlpha), Slug: ChannelAlpha, StabilityRank: ChannelRankAlpha, Enabled: true, System: true},
		{ProjectID: projectID, Name: names.DisplayName(ChannelBeta), Slug: ChannelBeta, StabilityRank: ChannelRankBeta, Enabled: true, System: true},
		{ProjectID: projectID, Name: names.DisplayName(ChannelStable), Slug: ChannelStable, StabilityRank: ChannelRankStable, Enabled: true, System: true},
	}
}
