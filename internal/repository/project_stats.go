package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/Kirizu-Official/KiriVers/internal/model"
)

// VersionStatusCounts 是某项目 Version 按生命周期状态的计数。
type VersionStatusCounts struct {
	Draft      int64
	Published  int64
	Deprecated int64
	Revoked    int64
	Total      int64
}

// TokenCountRow 是项目 Token 或 CI Token 的总数 / 未过期数。
type TokenCountRow struct {
	Total  int64
	Active int64
}

// ArtifactStorageRow 是产物行数与按 storage_key 去重后的字节数。
type ArtifactStorageRow struct {
	ArtifactCount int64
	StorageBytes  int64
}

type countRow struct {
	ProjectID uuid.UUID `gorm:"column:project_id"`
	N         int64     `gorm:"column:n"`
}

type statusCountRow struct {
	ProjectID uuid.UUID `gorm:"column:project_id"`
	Status    string    `gorm:"column:status"`
	N         int64     `gorm:"column:n"`
}

type tokenCountScan struct {
	ProjectID uuid.UUID `gorm:"column:project_id"`
	Total     int64     `gorm:"column:total"`
	Active    int64     `gorm:"column:active"`
}

func (r *ProjectRepo) CountVersionsByStatus(ctx context.Context, projectIDs []uuid.UUID) (map[uuid.UUID]VersionStatusCounts, error) {
	out := map[uuid.UUID]VersionStatusCounts{}
	if len(projectIDs) == 0 {
		return out, nil
	}
	var rows []statusCountRow
	err := r.db.WithContext(ctx).Model(&model.Version{}).
		Select("project_id, status, COUNT(*) AS n").
		Where("project_id IN ?", projectIDs).
		Group("project_id, status").
		Scan(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("count versions by status: %w", err)
	}
	for _, row := range rows {
		c := out[row.ProjectID]
		switch row.Status {
		case model.VersionStatusDraft:
			c.Draft = row.N
		case model.VersionStatusPublished:
			c.Published = row.N
		case model.VersionStatusDeprecated:
			c.Deprecated = row.N
		case model.VersionStatusRevoked:
			c.Revoked = row.N
		}
		c.Total += row.N
		out[row.ProjectID] = c
	}
	return out, nil
}

func (r *ProjectRepo) ListReleasedVersions(ctx context.Context, projectIDs []uuid.UUID) ([]model.Version, error) {
	if len(projectIDs) == 0 {
		return nil, nil
	}
	var list []model.Version
	err := r.db.WithContext(ctx).
		Where("project_id IN ? AND status IN ?", projectIDs, []string{
			model.VersionStatusPublished, model.VersionStatusDeprecated,
		}).
		Find(&list).Error
	if err != nil {
		return nil, fmt.Errorf("list released versions: %w", err)
	}
	return list, nil
}

func (r *ProjectRepo) CountChannelsByProject(ctx context.Context, projectIDs []uuid.UUID) (map[uuid.UUID]int64, error) {
	return r.countByProject(ctx, &model.Channel{}, projectIDs, "count channels")
}

func (r *ProjectRepo) CountMatrixByProject(ctx context.Context, projectIDs []uuid.UUID) (map[uuid.UUID]int64, error) {
	return r.countByProject(ctx, &model.PlatformMatrix{}, projectIDs, "count matrix")
}

func (r *ProjectRepo) CountHwRevsByProject(ctx context.Context, projectIDs []uuid.UUID) (map[uuid.UUID]int64, error) {
	return r.countByProject(ctx, &model.HwRev{}, projectIDs, "count hw revs")
}

func (r *ProjectRepo) countByProject(ctx context.Context, modelPtr any, projectIDs []uuid.UUID, op string) (map[uuid.UUID]int64, error) {
	out := map[uuid.UUID]int64{}
	if len(projectIDs) == 0 {
		return out, nil
	}
	var rows []countRow
	err := r.db.WithContext(ctx).Model(modelPtr).
		Select("project_id, COUNT(*) AS n").
		Where("project_id IN ?", projectIDs).
		Group("project_id").
		Scan(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("%s: %w", op, err)
	}
	for _, row := range rows {
		out[row.ProjectID] = row.N
	}
	return out, nil
}

func (r *ProjectRepo) CountProjectTokens(ctx context.Context, projectIDs []uuid.UUID, now time.Time) (map[uuid.UUID]TokenCountRow, error) {
	return r.countTokens(ctx, &model.ProjectToken{}, projectIDs, now, "count project tokens")
}

func (r *ProjectRepo) CountCITokens(ctx context.Context, projectIDs []uuid.UUID, now time.Time) (map[uuid.UUID]TokenCountRow, error) {
	return r.countTokens(ctx, &model.CIToken{}, projectIDs, now, "count ci tokens")
}

func (r *ProjectRepo) countTokens(ctx context.Context, modelPtr any, projectIDs []uuid.UUID, now time.Time, op string) (map[uuid.UUID]TokenCountRow, error) {
	out := map[uuid.UUID]TokenCountRow{}
	if len(projectIDs) == 0 {
		return out, nil
	}
	var rows []tokenCountScan
	err := r.db.WithContext(ctx).Model(modelPtr).
		Select("project_id, COUNT(*) AS total, COALESCE(SUM(CASE WHEN expires_at IS NULL OR expires_at > ? THEN 1 ELSE 0 END), 0) AS active", now).
		Where("project_id IN ?", projectIDs).
		Group("project_id").
		Scan(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("%s: %w", op, err)
	}
	for _, row := range rows {
		out[row.ProjectID] = TokenCountRow{Total: row.Total, Active: row.Active}
	}
	return out, nil
}

func (r *ProjectRepo) ArtifactStorageStats(ctx context.Context, projectIDs []uuid.UUID) (map[uuid.UUID]ArtifactStorageRow, error) {
	out := map[uuid.UUID]ArtifactStorageRow{}
	if len(projectIDs) == 0 {
		return out, nil
	}
	var counts []countRow
	err := r.db.WithContext(ctx).Model(&model.Artifact{}).
		Select("project_id, COUNT(*) AS n").
		Where("project_id IN ?", projectIDs).
		Group("project_id").
		Scan(&counts).Error
	if err != nil {
		return nil, fmt.Errorf("count artifacts: %w", err)
	}
	for _, row := range counts {
		out[row.ProjectID] = ArtifactStorageRow{ArtifactCount: row.N}
	}
	var sizes []countRow
	err = r.db.WithContext(ctx).Raw(`
		SELECT project_id, COALESCE(SUM(size), 0) AS n
		FROM (
			SELECT DISTINCT ON (project_id, storage_key) project_id, size
			FROM artifacts
			WHERE project_id IN ?
			ORDER BY project_id, storage_key
		) d
		GROUP BY project_id
	`, projectIDs).Scan(&sizes).Error
	if err != nil {
		return nil, fmt.Errorf("sum distinct artifact size: %w", err)
	}
	for _, row := range sizes {
		item := out[row.ProjectID]
		item.StorageBytes = row.N
		out[row.ProjectID] = item
	}
	return out, nil
}
