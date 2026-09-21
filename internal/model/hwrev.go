package model

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// HwRev 是项目内可排序的硬件代号（不进入版本号，挂在产物上）。
//
// 用途：客户端 hw_rev 匹配与 higher_compatible_with_lower 策略的 rank 比较。
// 未登记的 slug 在绑定产物时应返回 HW_REV_UNKNOWN（见 service.AssertHwRevKnown）。
//
// 关系：归属 Project。列表按 rank 升序。
//
// 字段：
//   - ID：UUID 主键。
//   - ProjectID：所属项目。
//   - Slug：硬件代号，项目内唯一。
//   - Rank：越大越「高版本」。
//   - Notes：运营备注，可空。
type HwRev struct {
	ID        uuid.UUID `gorm:"type:uuid;primaryKey" json:"id"`
	ProjectID uuid.UUID `gorm:"type:uuid;not null;uniqueIndex:idx_hw_revs_project_slug" json:"project_id"`
	Slug      string    `gorm:"type:text;not null;uniqueIndex:idx_hw_revs_project_slug" json:"slug"`
	Rank      int       `gorm:"not null" json:"rank"`
	Notes     string    `gorm:"type:text" json:"notes"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// TableName 固定表名 hw_revs。
func (HwRev) TableName() string {
	return "hw_revs"
}

// BeforeCreate 补 UUID。
func (h *HwRev) BeforeCreate(_ *gorm.DB) error {
	if h.ID == uuid.Nil {
		h.ID = uuid.New()
	}
	return nil
}
