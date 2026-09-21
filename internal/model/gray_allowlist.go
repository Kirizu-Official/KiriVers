package model

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

const (
	// GraySourceAuto 自动放号写入的白名单来源。
	GraySourceAuto = "auto"
	// GraySourceManual 运营者从名册勾选加入的白名单来源。
	GraySourceManual = "manual"
)

// GrayAllowlist 是版本级灰度白名单条目。
//
// 用途：进行中灰度时，非强制客户端仅白名单可见。来源 auto（时间增量放号）
// 或 manual（运营者从名册勾选）。只追加不踢出。
//
// DeviceID 存储形态按项目 DeviceIDPolicy 决定（写入时由 service 层完成）：
//   - hashed（默认）：hex(HMAC-SHA256(project.DeviceSecret, raw))，与遥测
//     DeviceHash / 名册 DeviceHash 同一函数（service.HashDeviceID）；
//   - raw：明文原样（不推荐）；
//   - none：拒绝写入（HTTP 400）。
//
// 关系：归属 Project（ProjectID）与 Version（VersionID）。无 VersionLine。
//
// 索引：唯一 (version_id, device_id)；复合 (project_id, device_id) 服务按设备删除。
type GrayAllowlist struct {
	ID        uuid.UUID `gorm:"type:uuid;primaryKey" json:"id"`
	ProjectID uuid.UUID `gorm:"type:uuid;not null;index:idx_gray_allowlist_project_device,priority:1" json:"project_id"`
	VersionID uuid.UUID `gorm:"type:uuid;not null;uniqueIndex:idx_gray_allowlist_version_device" json:"version_id"`
	DeviceID  string    `gorm:"type:text;not null;uniqueIndex:idx_gray_allowlist_version_device;index:idx_gray_allowlist_project_device,priority:2" json:"device_id"`
	Source    string    `gorm:"type:text;not null;default:manual" json:"source"`
	CreatedAt time.Time `json:"created_at"`
}

// TableName 固定表名 gray_allowlist，后续任务不得改名。
func (GrayAllowlist) TableName() string {
	return "gray_allowlist"
}

// BeforeCreate 补 UUID 与默认来源。
func (g *GrayAllowlist) BeforeCreate(_ *gorm.DB) error {
	if g.ID == uuid.Nil {
		g.ID = uuid.New()
	}
	if g.Source == "" {
		g.Source = GraySourceManual
	}
	return nil
}
