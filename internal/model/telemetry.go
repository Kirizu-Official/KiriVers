package model

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// TelemetryEvent.Status 枚举（docs/app-init.md §10.4）。客户端上报的更新生命周期阶段。
const (
	// TelemetryStatusDownloading 客户端开始下载更新包。
	TelemetryStatusDownloading = "downloading"
	// TelemetryStatusApplying 客户端开始应用更新（解压/落盘/切换）。
	TelemetryStatusApplying = "applying"
	// TelemetryStatusInstalled 更新成功安装（恢复降级判定的信号）。
	TelemetryStatusInstalled = "installed"
	// TelemetryStatusFailed 更新失败（累计 ≥3 触发强制全量降级）。
	TelemetryStatusFailed = "failed"
	// TelemetryStatusRolledBack 客户端回滚到旧版本。
	TelemetryStatusRolledBack = "rolled_back"
)

// ValidTelemetryStatuses 是 report 接口接受的全部 status 取值集合。
var ValidTelemetryStatuses = map[string]struct{}{
	TelemetryStatusDownloading: {},
	TelemetryStatusApplying:    {},
	TelemetryStatusInstalled:   {},
	TelemetryStatusFailed:      {},
	TelemetryStatusRolledBack:  {},
}

// DefaultTelemetryRetentionDays 遥测事件默认留存天数（§13.11：默认 90 天，可配；
// 0 视为 90）。
const DefaultTelemetryRetentionDays = 90

// TelemetryEvent 是一次客户端更新上报事件（§10.4 遥测）。
//
// 用途：
//   - 供发布者观察更新成败（status 分布、error_code 聚合）；
//   - 驱动「连续失败降级」读模型：同一 (project, os, arch, device_hash) 在
//     24h 窗口内 failed ≥ 3 且未被 installed 恢复 → check/diff 强制全量（§15.2）；
//   - 按 TelemetryRetentionDays 留存，过期后由插入路径惰性清理（1/50 采样），
//     或由管理员按 device_hash 隐私删除（§13.11）。
//
// 隐私约束（C11-1/C11-2）：
//   - DeviceHash 按项目 DeviceIDPolicy 落库：hashed = hex(HMAC-SHA256(project.DeviceSecret, raw))；
//     raw = 明文原样（不推荐）；none = 空串（不做基于设备的降级判定）。
//   - 原始 device_id 永不落库、永不进应用日志。
//
// 关系：
//   - 归属于 Project（ProjectID，无外键约束，软删项目的事件随留存清理自然过期）。
//
// 字段：
//   - ID：UUID v4 主键，应用侧生成。
//   - DeviceHash：设备标识（按策略哈希/明文/空）。
//   - OS / Arch：请求平台（已规范化 slug）。
//   - ChannelSlug：上报渠道；降级判定键不含本字段（渠道可能已跨）。
//   - FromVersion / ToVersion：客户端原始引用字符串（integer 或 SemVer 均可）。
//   - Status：downloading | applying | installed | failed | rolled_back。
//   - ErrorCode / ErrorMessage：失败时的错误信息（可选）。
//   - DiffMode：本次更新使用的形态（full_package / patch_package / binary_delta / file_list，可选）。
//   - CreatedAt：事件时间（UTC），留存清理与降级窗口均以此为准。
type TelemetryEvent struct {
	// ID 是事件主键（UUID v4）。
	ID uuid.UUID `gorm:"type:uuid;primaryKey" json:"id"`
	// ProjectID 是所属项目；降级查询与留存清理的第一维。
	ProjectID uuid.UUID `gorm:"type:uuid;not null;index:idx_telemetry_downgrade,priority:1;index:idx_telemetry_retention,priority:1;index:idx_telemetry_privacy,priority:1" json:"project_id"`
	// DeviceHash 是按项目策略处理后的设备标识；hashed 策略下库中不含明文（验收项）。
	DeviceHash string `gorm:"type:text;not null;default:'';index:idx_telemetry_downgrade,priority:4;index:idx_telemetry_privacy,priority:2" json:"device_hash"`
	// OS 是规范化平台 OS slug。
	OS string `gorm:"type:text;not null;index:idx_telemetry_downgrade,priority:2" json:"os"`
	// Arch 是规范化平台架构 slug。
	Arch string `gorm:"type:text;not null;index:idx_telemetry_downgrade,priority:3" json:"arch"`
	// ChannelSlug 是上报时的渠道；不参与降级判定键。
	ChannelSlug string `gorm:"type:text;not null;default:''" json:"channel_slug"`
	// FromVersion 是客户端更新前的原始版本引用。
	FromVersion string `gorm:"type:text;not null;default:''" json:"from_version"`
	// ToVersion 是客户端更新目标的原始版本引用。
	ToVersion string `gorm:"type:text;not null;default:''" json:"to_version"`
	// Status 是更新生命周期阶段（枚举见上）。
	Status string `gorm:"type:text;not null" json:"status"`
	// ErrorCode 是失败错误码（可选，仅 failed/rolled_back 有意义）。
	ErrorCode string `gorm:"type:text;not null;default:''" json:"error_code"`
	// ErrorMessage 是失败错误描述（可选）。
	ErrorMessage string `gorm:"type:text;not null;default:''" json:"error_message"`
	// DiffMode 是本次更新形态（可选）。
	DiffMode string `gorm:"type:text;not null;default:''" json:"diff_mode"`
	// CreatedAt 是事件时间；idx_telemetry_retention 服务留存清理，
	// idx_telemetry_downgrade 服务 24h 降级窗口查询。
	CreatedAt time.Time `gorm:"not null;index:idx_telemetry_downgrade,priority:5;index:idx_telemetry_retention,priority:2" json:"created_at"`
}

// TableName 固定表名 telemetry_events。
func (TelemetryEvent) TableName() string {
	return "telemetry_events"
}

// BeforeCreate 补 UUID 与事件时间，避免空值破坏窗口/留存查询。
func (e *TelemetryEvent) BeforeCreate(_ *gorm.DB) error {
	if e.ID == uuid.Nil {
		id, err := uuid.NewRandom()
		if err != nil {
			return err
		}
		e.ID = id
	}
	if e.CreatedAt.IsZero() {
		e.CreatedAt = time.Now().UTC()
	}
	return nil
}
