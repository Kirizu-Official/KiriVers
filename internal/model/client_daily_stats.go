package model

import (
	"time"

	"github.com/google/uuid"
)

// ClientDailyStats 是项目按自然日聚合的新设备与活跃设备计数。
//
// 用途：概览折线读本表，避免全表扫描 clients。首次见到设备（created_at 当日）
// 增加 new_count；last_check_at 当日 upsert active_count。
//
// 主键：(project_id, day)。Day 为 UTC 日期。
type ClientDailyStats struct {
	ProjectID   uuid.UUID `gorm:"type:uuid;primaryKey" json:"project_id"`
	Day         time.Time `gorm:"type:date;primaryKey" json:"day"`
	NewCount    int64     `gorm:"not null;default:0" json:"new_count"`
	ActiveCount int64     `gorm:"not null;default:0" json:"active_count"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// TableName 固定表名 client_daily_stats。
func (ClientDailyStats) TableName() string {
	return "client_daily_stats"
}
