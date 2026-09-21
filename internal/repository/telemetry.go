package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/Kirizu-Official/KiriVers/internal/model"
)

// TelemetryDayCount 是按 UTC 日聚合的遥测事件计数。
type TelemetryDayCount struct {
	Day    time.Time
	Status string
	Count  int64
}

// TelemetryStore 是遥测事件的持久化接口（§10.4）：插入上报、按哈希隐私删除、
// 留存清理与 24h 降级窗口查询。PostgreSQL 与内存实现各一份（memory_telemetry.go）。
type TelemetryStore interface {
	// Insert 写入一条遥测事件（服务端补 ID / CreatedAt）。
	Insert(ctx context.Context, event *model.TelemetryEvent) error
	// DeleteByDeviceHash 删除项目内指定设备哈希的全部事件（隐私删除 §13.11），
	// 返回删除条数；未知哈希返回 0（幂等）。
	DeleteByDeviceHash(ctx context.Context, projectID uuid.UUID, deviceHash string) (int64, error)
	// DeleteOlderThan 删除项目内 created_at 早于 cutoff 的事件（留存清理），
	// 返回删除条数。
	DeleteOlderThan(ctx context.Context, projectID uuid.UUID, cutoff time.Time) (int64, error)
	// ListWindow 返回 (project, os, arch, deviceHash) 在 [since, now] 窗口内的
	// 事件，按 created_at 倒序（新→旧），供连续失败降级判定消费。
	ListWindow(ctx context.Context, projectID uuid.UUID, os, arch, deviceHash string, since time.Time) ([]model.TelemetryEvent, error)
	// CountStatusSince 按项目与 status 统计 created_at >= since 的事件数。
	// 空 projectIDs 或 statuses 返回空 map。
	CountStatusSince(ctx context.Context, projectIDs []uuid.UUID, since time.Time, statuses []string) (map[uuid.UUID]map[string]int64, error)
	// CountStatusByDay 按 UTC 日与 status 统计 [from, to] 闭区间内的事件数。
	CountStatusByDay(ctx context.Context, projectID uuid.UUID, from, to time.Time, statuses []string) ([]TelemetryDayCount, error)
}

// TelemetryRepo 是 TelemetryStore 的 PostgreSQL 实现。
type TelemetryRepo struct {
	db *gorm.DB
}

// NewTelemetryRepo 构造仓储。
func NewTelemetryRepo(db *gorm.DB) *TelemetryRepo {
	return &TelemetryRepo{db: db}
}

// Insert 写入一条遥测事件。
func (r *TelemetryRepo) Insert(ctx context.Context, event *model.TelemetryEvent) error {
	if err := r.db.WithContext(ctx).Create(event).Error; err != nil {
		return fmt.Errorf("insert telemetry event: %w", err)
	}
	return nil
}

// DeleteByDeviceHash 按哈希删除该设备的全部事件（隐私删除）。
func (r *TelemetryRepo) DeleteByDeviceHash(ctx context.Context, projectID uuid.UUID, deviceHash string) (int64, error) {
	res := r.db.WithContext(ctx).
		Where("project_id = ? AND device_hash = ?", projectID, deviceHash).
		Delete(&model.TelemetryEvent{})
	if res.Error != nil {
		return 0, fmt.Errorf("delete telemetry by device hash: %w", res.Error)
	}
	return res.RowsAffected, nil
}

// DeleteOlderThan 删除早于 cutoff 的事件（留存清理，惰性触发）。
func (r *TelemetryRepo) DeleteOlderThan(ctx context.Context, projectID uuid.UUID, cutoff time.Time) (int64, error) {
	res := r.db.WithContext(ctx).
		Where("project_id = ? AND created_at < ?", projectID, cutoff).
		Delete(&model.TelemetryEvent{})
	if res.Error != nil {
		return 0, fmt.Errorf("delete old telemetry events: %w", res.Error)
	}
	return res.RowsAffected, nil
}

// ListWindow 返回降级窗口内的事件（created_at 倒序）。命中复合索引
// idx_telemetry_downgrade (project_id, os, arch, device_hash, created_at)。
func (r *TelemetryRepo) ListWindow(ctx context.Context, projectID uuid.UUID, os, arch, deviceHash string, since time.Time) ([]model.TelemetryEvent, error) {
	var list []model.TelemetryEvent
	err := r.db.WithContext(ctx).
		Where("project_id = ? AND os = ? AND arch = ? AND device_hash = ? AND created_at >= ?",
			projectID, os, arch, deviceHash, since).
		Order("created_at DESC").
		Find(&list).Error
	if err != nil {
		return nil, fmt.Errorf("list telemetry window: %w", err)
	}
	return list, nil
}

func (r *TelemetryRepo) CountStatusSince(ctx context.Context, projectIDs []uuid.UUID, since time.Time, statuses []string) (map[uuid.UUID]map[string]int64, error) {
	out := map[uuid.UUID]map[string]int64{}
	if len(projectIDs) == 0 || len(statuses) == 0 {
		return out, nil
	}
	type row struct {
		ProjectID uuid.UUID `gorm:"column:project_id"`
		Status    string    `gorm:"column:status"`
		N         int64     `gorm:"column:n"`
	}
	var rows []row
	err := r.db.WithContext(ctx).Model(&model.TelemetryEvent{}).
		Select("project_id, status, COUNT(*) AS n").
		Where("project_id IN ? AND created_at >= ? AND status IN ?", projectIDs, since, statuses).
		Group("project_id, status").
		Scan(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("count telemetry status: %w", err)
	}
	for _, item := range rows {
		byStatus := out[item.ProjectID]
		if byStatus == nil {
			byStatus = map[string]int64{}
			out[item.ProjectID] = byStatus
		}
		byStatus[item.Status] = item.N
	}
	return out, nil
}

func (r *TelemetryRepo) CountStatusByDay(ctx context.Context, projectID uuid.UUID, from, to time.Time, statuses []string) ([]TelemetryDayCount, error) {
	if len(statuses) == 0 {
		return nil, nil
	}
	if to.Before(from) {
		from, to = to, from
	}
	type row struct {
		Day    time.Time `gorm:"column:day"`
		Status string    `gorm:"column:status"`
		N      int64     `gorm:"column:n"`
	}
	var rows []row
	err := r.db.WithContext(ctx).Raw(
		`SELECT ((created_at AT TIME ZONE 'UTC')::date) AS day, status, COUNT(*) AS n
FROM telemetry_events
WHERE project_id = ? AND created_at >= ? AND created_at <= ? AND status IN ?
GROUP BY 1, 2
ORDER BY 1`,
		projectID, from.UTC(), to.UTC(), statuses,
	).Scan(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("count telemetry by day: %w", err)
	}
	out := make([]TelemetryDayCount, 0, len(rows))
	for _, item := range rows {
		out = append(out, TelemetryDayCount{Day: item.Day.UTC(), Status: item.Status, Count: item.N})
	}
	return out, nil
}
