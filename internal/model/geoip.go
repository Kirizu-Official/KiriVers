package model

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

const (
	// GeoipMaxDatabases 平台最多同时保留的 MMDB 份数。
	GeoipMaxDatabases = 8
	// GeoipMaxFileBytes 单份 MMDB 上限（256 MiB）。
	GeoipMaxFileBytes = 256 << 20
)

// GeoipDatabase 是平台管理员上传的 MaxMind/DB-IP MMDB 元数据。
//
// 用途：控制台管理多份 .mmdb，按 rank 升序字段级融合查询客户端来源地。
// 文件字节走 storage.Backend，键 geoip/{id}/{filename}。项目成员不能管库。
//
// 关系：实例级，无 Project FK。
//
// 字段：
//   - ID：UUID 主键。
//   - Name：控制台显示名。
//   - FileName：原始文件名（须 .mmdb）。
//   - StorageKey：对象存储键。
//   - Size：字节数。
//   - Enabled：是否参与融合。
//   - Rank：升序优先；并列再按 created_at。
//   - CreatedAt / UpdatedAt：GORM 时间戳。
type GeoipDatabase struct {
	ID         uuid.UUID `gorm:"type:uuid;primaryKey" json:"id"`
	Name       string    `gorm:"type:text;not null;default:''" json:"name"`
	FileName   string    `gorm:"type:text;not null;default:''" json:"file_name"`
	StorageKey string    `gorm:"type:text;not null;default:''" json:"storage_key"`
	Size       int64     `gorm:"not null;default:0" json:"size"`
	Enabled    bool      `gorm:"not null;default:true" json:"enabled"`
	Rank       int       `gorm:"not null;default:0" json:"rank"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

// TableName 固定表名 geoip_databases。
func (GeoipDatabase) TableName() string {
	return "geoip_databases"
}

// BeforeCreate 补 UUID。
func (g *GeoipDatabase) BeforeCreate(_ *gorm.DB) error {
	if g.ID == uuid.Nil {
		id, err := uuid.NewRandom()
		if err != nil {
			return err
		}
		g.ID = id
	}
	return nil
}
