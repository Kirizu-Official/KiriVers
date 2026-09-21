package model

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

const (
	// MaxClientCustomJSONBytes 登录 API custom 对象编码后上限（16 KiB）。
	MaxClientCustomJSONBytes = 16 * 1024
	// MaxDeviceIDRunes 原始 device_id 写入上限（应用层校验，列类型为 text）。
	MaxDeviceIDRunes = 128
)

// Client 是项目内一台可检索的客户端名册行。
//
// 用途：登录 API 登记 JSON + 运行信息；带 device_id 的 update/check 只更新
// 运行字段。灰度放号从本表取待放号队列。device_id_policy=none 或匿名
// 请求不建档。
//
// 关系：归属 Project（ProjectID）。无外键，与 Job/Audit 软引用同一口径。
//
// 字段：
//   - ID：UUID v4 主键，应用侧生成。
//   - ProjectID：所属项目；与 DeviceHash 组成唯一索引。
//   - DeviceHash：HashDeviceID 后的存储键（hashed=HMAC hex，raw=原样）。
//   - LastVersion / LastOS / LastArch / LastChannel：客户端最近上报的运行信息。
//   - LastIP：可信 ClientIP()；可查，不进应用日志。
//   - CountryCode / RegionCode：融合 GeoIP 的 ISO 代码；私网/失败/无库为空。
//   - GeoI18n：国家/地区多语言地名图 JSONB，不存单一 country_name。
//   - Custom：登录写入的 JSON 对象，check 不覆盖。
//   - LastCheckAt：最近一次登录或 check。
type Client struct {
	ID          uuid.UUID  `gorm:"type:uuid;primaryKey" json:"id"`
	ProjectID   uuid.UUID  `gorm:"type:uuid;not null;uniqueIndex:idx_clients_project_device;index:idx_clients_project_check,priority:1;index:idx_clients_project_created,priority:1;index:idx_clients_project_os,priority:1;index:idx_clients_project_arch,priority:1;index:idx_clients_project_version,priority:1;index:idx_clients_project_country,priority:1" json:"project_id"`
	DeviceHash  string     `gorm:"type:text;not null;uniqueIndex:idx_clients_project_device" json:"device_hash"`
	LastVersion string     `gorm:"type:text;not null;default:'';index:idx_clients_project_version,priority:2" json:"last_version"`
	LastOS      string     `gorm:"type:text;not null;default:'';index:idx_clients_project_os,priority:2" json:"last_os"`
	LastArch    string     `gorm:"type:text;not null;default:'';index:idx_clients_project_arch,priority:2" json:"last_arch"`
	LastChannel string     `gorm:"type:text;not null;default:''" json:"last_channel"`
	LastIP      string     `gorm:"type:text;not null;default:''" json:"last_ip"`
	CountryCode string     `gorm:"type:text;not null;default:'';index:idx_clients_project_country,priority:2" json:"country_code"`
	RegionCode  string     `gorm:"type:text;not null;default:''" json:"region_code"`
	GeoI18n     GeoI18n    `gorm:"type:jsonb;not null;default:'{}'" json:"geo_i18n"`
	Custom      JSONObject `gorm:"type:jsonb;not null;default:'{}'" json:"custom"`
	LastCheckAt *time.Time `gorm:"index:idx_clients_project_check,priority:2" json:"last_check_at"`
	CreatedAt   time.Time  `gorm:"index:idx_clients_project_created,priority:2" json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

// TableName 固定表名 clients。
func (Client) TableName() string {
	return "clients"
}

// BeforeCreate 补 UUID 与空 JSON。
func (c *Client) BeforeCreate(_ *gorm.DB) error {
	if c.ID == uuid.Nil {
		id, err := uuid.NewRandom()
		if err != nil {
			return err
		}
		c.ID = id
	}
	if c.Custom == nil {
		c.Custom = JSONObject{}
	}
	return nil
}
