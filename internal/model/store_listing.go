package model

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

const (
	// PackageSourceLineFull 商店完整包取该线 kind=full 产物（单文件即安装器，多文件即 zip）。
	PackageSourceLineFull = "line_full"
	// PackageSourceManifestPath 商店完整包取清单上指定路径对应的文件字节。
	PackageSourceManifestPath = "manifest_path"
)

// StoreListing 是项目内一条商店上架记录：同一协议可有多条，用 slug 区分 feed URL。
//
// 用途：替代已删除的 Project.StoreProtocols jsonb。每条 listing 绑定一个适配器
// Protocol() 名（sparkle / electron / …）、可选钉死 os/arch/channel，以及完整包选择器。
// Feed 路由按 (project, protocol, slug) 解析；停用或不存在 → 纯文本 404。
//
// 关系：归属 Project（FK）。identifiers 是协议包名袋（bundle id、PackageIdentifier 等）。
//
// 字段：
//   - ID：UUID 主键。
//   - ProjectID：所属项目。
//   - Protocol：适配器 Protocol() 名，如 sparkle、electron（不是 UI 旧名 electron-updater）。
//   - Slug：feed 身份，规则与渠道 slug 相同（[a-z0-9-]{3,64}，无下划线）；
//     在 (project_id, protocol) 内唯一。
//   - Enabled：false 时该 feed 纯文本 404。
//   - OS / Arch / Channel：可空。非空表示钉死该维，忽略冲突 query；空则协议自行枚举。
//   - Identifiers：协议相关键值（homepage、bundle_id 等）。
//   - PackageSource：line_full | manifest_path。
//   - ManifestPath：package_source=manifest_path 时必填的清单相对路径。
//   - CreatedAt / UpdatedAt：GORM 时间戳。
type StoreListing struct {
	ID            uuid.UUID     `gorm:"type:uuid;primaryKey" json:"id"`
	ProjectID     uuid.UUID     `gorm:"type:uuid;not null;uniqueIndex:idx_store_listings_project_protocol_slug" json:"project_id"`
	Protocol      string        `gorm:"type:text;not null;uniqueIndex:idx_store_listings_project_protocol_slug" json:"protocol"`
	Slug          string        `gorm:"type:text;not null;uniqueIndex:idx_store_listings_project_protocol_slug" json:"slug"`
	Enabled       bool          `gorm:"not null;default:true" json:"enabled"`
	OS            *string       `gorm:"type:text" json:"os"`
	Arch          *string       `gorm:"type:text" json:"arch"`
	Channel       *string       `gorm:"type:text" json:"channel"`
	Identifiers   IdentifierMap `gorm:"type:jsonb" json:"identifiers"`
	PackageSource string        `gorm:"type:text;not null;default:line_full" json:"package_source"`
	ManifestPath  string        `gorm:"type:text;not null;default:''" json:"manifest_path"`
	CreatedAt     time.Time     `json:"created_at"`
	UpdatedAt     time.Time     `json:"updated_at"`
}

// TableName 固定表名 project_store_listings。
func (StoreListing) TableName() string {
	return "project_store_listings"
}

// BeforeCreate 分配 UUID v4，并保证 identifiers / package_source 非空。
func (s *StoreListing) BeforeCreate(tx *gorm.DB) error {
	if s.ID == uuid.Nil {
		s.ID = uuid.New()
	}
	if s.Identifiers == nil {
		s.Identifiers = IdentifierMap{}
	}
	if s.PackageSource == "" {
		s.PackageSource = PackageSourceLineFull
	}
	return nil
}
