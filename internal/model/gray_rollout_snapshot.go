package model

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// GrayRolloutSnapshot 是一次 EnsureGrayAdmission（含转全量）的覆盖快照。
//
// 用途：灰度页时间序列只读本表，不从变化中的 N 反推历史。只追加不改写。
//
// 字段：
//   - N：当时名册人数。
//   - Desired：公式算出的目标放号人数。
//   - Allowlisted：当时白名单人数（含手动）。
//   - Updated：名册当前版本 ≥ 该灰度版本的人数。
//   - TargetPercent：当时目标覆盖百分比。
type GrayRolloutSnapshot struct {
	ID            uuid.UUID `gorm:"type:uuid;primaryKey" json:"id"`
	ProjectID     uuid.UUID `gorm:"type:uuid;not null;index:idx_gray_snapshots_project_taken,priority:1" json:"project_id"`
	VersionID     uuid.UUID `gorm:"type:uuid;not null;index:idx_gray_snapshots_version_taken,priority:1" json:"version_id"`
	TakenAt       time.Time `gorm:"not null;index:idx_gray_snapshots_version_taken,priority:2;index:idx_gray_snapshots_project_taken,priority:2" json:"taken_at"`
	N             int       `gorm:"not null;default:0" json:"n"`
	Desired       int       `gorm:"not null;default:0" json:"desired"`
	Allowlisted   int       `gorm:"not null;default:0" json:"allowlisted"`
	Updated       int       `gorm:"not null;default:0" json:"updated"`
	TargetPercent int       `gorm:"not null;default:0" json:"target_percent"`
}

// TableName 固定表名 gray_rollout_snapshots。
func (GrayRolloutSnapshot) TableName() string {
	return "gray_rollout_snapshots"
}

// BeforeCreate 补 UUID。
func (s *GrayRolloutSnapshot) BeforeCreate(_ *gorm.DB) error {
	if s.ID == uuid.Nil {
		id, err := uuid.NewRandom()
		if err != nil {
			return err
		}
		s.ID = id
	}
	return nil
}
