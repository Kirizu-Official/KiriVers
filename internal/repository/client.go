package repository

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/Kirizu-Official/KiriVers/internal/model"
)

// ClientListFilter 管理端名册列表筛选。
type ClientListFilter struct {
	Q           string
	OS          string
	Arch        string
	Version     string
	ActiveSince *time.Time
	Limit       int
	Offset      int
}

// CountryCount 是国家分布桶：ISO 代码 + 计数 + 融合地名图（按当前 UI 语言选词）。
type CountryCount struct {
	Code  string            `json:"code"`
	Count int64             `json:"count"`
	Names map[string]string `json:"names"`
}

// ClientBuckets 客户端列表页与概览聚合桶。
type ClientBuckets struct {
	Total        int64          `json:"total"`
	Active24h    int64          `json:"active_24h"`
	Active7d     int64          `json:"active_7d"`
	Versions     []NameCount    `json:"versions"`
	OS           []NameCount    `json:"os"`
	Arch         []NameCount    `json:"arch"`
	Channels     []NameCount    `json:"channels"`
	Countries    []CountryCount `json:"countries"`
	RecencyHours []NameCount    `json:"recency_hours"`
}

// NameCount 是图表用的 {name, count} 桶。
type NameCount struct {
	Name  string `json:"name"`
	Count int64  `json:"count"`
}

// GrayAdmitResult 是一次懒放号的结果。
type GrayAdmitResult struct {
	N             int
	Desired       int
	Allowlisted   int
	TargetPercent int
	Completed     bool
	Version       *model.Version
}

func (r *ProjectRepo) UpsertClientLogin(ctx context.Context, row *model.Client, custom model.JSONObject, now time.Time) (*model.Client, bool, error) {
	return r.upsertClient(ctx, row, custom, true, now)
}

func (r *ProjectRepo) UpsertClientCheck(ctx context.Context, row *model.Client, now time.Time) (*model.Client, bool, error) {
	return r.upsertClient(ctx, row, nil, false, now)
}

func (r *ProjectRepo) upsertClient(ctx context.Context, row *model.Client, custom model.JSONObject, writeCustom bool, now time.Time) (*model.Client, bool, error) {
	if row == nil || row.DeviceHash == "" {
		return nil, false, fmt.Errorf("client upsert requires device_hash")
	}
	var created bool
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var existing model.Client
		qerr := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("project_id = ? AND device_hash = ?", row.ProjectID, row.DeviceHash).
			First(&existing).Error
		if qerr != nil && !errorsIsNotFound(qerr) {
			return qerr
		}
		if errorsIsNotFound(qerr) {
			if err := row.BeforeCreate(nil); err != nil {
				return err
			}
			row.CreatedAt = now
			row.UpdatedAt = now
			row.LastCheckAt = &now
			if writeCustom {
				if custom == nil {
					custom = model.JSONObject{}
				}
				row.Custom = custom
			} else if row.Custom == nil {
				row.Custom = model.JSONObject{}
			}
			if err := tx.Create(row).Error; err != nil {
				return err
			}
			created = true
			if err := bumpDailyStats(tx, row.ProjectID, now, true, true); err != nil {
				return err
			}
			return nil
		}
		existing.LastVersion = row.LastVersion
		existing.LastOS = row.LastOS
		existing.LastArch = row.LastArch
		existing.LastChannel = row.LastChannel
		existing.LastIP = row.LastIP
		existing.CountryCode = row.CountryCode
		existing.RegionCode = row.RegionCode
		existing.GeoI18n = row.GeoI18n.Clone()
		existing.LastCheckAt = &now
		existing.UpdatedAt = now
		if writeCustom {
			if custom == nil {
				custom = model.JSONObject{}
			}
			existing.Custom = custom
		}
		if err := tx.Save(&existing).Error; err != nil {
			return err
		}
		*row = existing
		return bumpDailyStats(tx, row.ProjectID, now, false, true)
	})
	if err != nil {
		return nil, false, fmt.Errorf("upsert client: %w", err)
	}
	out := *row
	return &out, created, nil
}

func errorsIsNotFound(err error) bool {
	return err != nil && err == gorm.ErrRecordNotFound
}

func bumpDailyStats(tx *gorm.DB, projectID uuid.UUID, now time.Time, isNew, isActive bool) error {
	day := time.Date(now.UTC().Year(), now.UTC().Month(), now.UTC().Day(), 0, 0, 0, 0, time.UTC)
	row := model.ClientDailyStats{ProjectID: projectID, Day: day, UpdatedAt: now.UTC()}
	if isNew {
		row.NewCount = 1
	}
	if isActive {
		row.ActiveCount = 1
	}
	assigns := map[string]any{"updated_at": now.UTC()}
	if isNew {
		assigns["new_count"] = gorm.Expr("client_daily_stats.new_count + 1")
	}
	if isActive {
		assigns["active_count"] = gorm.Expr("client_daily_stats.active_count + 1")
	}
	return tx.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "project_id"}, {Name: "day"}},
		DoUpdates: clause.Assignments(assigns),
	}).Create(&row).Error
}

func (r *ProjectRepo) GetClientByID(ctx context.Context, projectID, id uuid.UUID) (*model.Client, error) {
	var c model.Client
	err := r.db.WithContext(ctx).Where("project_id = ? AND id = ?", projectID, id).First(&c).Error
	if err != nil {
		return nil, err
	}
	return &c, nil
}

func (r *ProjectRepo) GetClientByDeviceHash(ctx context.Context, projectID uuid.UUID, hash string) (*model.Client, error) {
	var c model.Client
	err := r.db.WithContext(ctx).Where("project_id = ? AND device_hash = ?", projectID, hash).First(&c).Error
	if err != nil {
		return nil, err
	}
	return &c, nil
}

func (r *ProjectRepo) ListClients(ctx context.Context, projectID uuid.UUID, filter ClientListFilter) ([]model.Client, int64, error) {
	q := r.db.WithContext(ctx).Model(&model.Client{}).Where("project_id = ?", projectID)
	if os := strings.TrimSpace(filter.OS); os != "" {
		q = q.Where("last_os = ?", os)
	}
	if arch := strings.TrimSpace(filter.Arch); arch != "" {
		q = q.Where("last_arch = ?", arch)
	}
	if ver := strings.TrimSpace(filter.Version); ver != "" {
		q = q.Where("last_version = ?", ver)
	}
	if filter.ActiveSince != nil {
		q = q.Where("last_check_at >= ?", filter.ActiveSince.UTC())
	}
	if s := strings.TrimSpace(filter.Q); s != "" {
		like := "%" + s + "%"
		q = q.Where(`last_version ILIKE ? OR last_os ILIKE ? OR last_arch ILIKE ? OR last_channel ILIKE ? OR last_ip ILIKE ? OR custom::text ILIKE ?`,
			like, like, like, like, like, like)
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("count clients: %w", err)
	}
	limit := filter.Limit
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	var list []model.Client
	if err := q.Order("last_check_at DESC NULLS LAST, created_at DESC").
		Limit(limit).Offset(filter.Offset).Find(&list).Error; err != nil {
		return nil, 0, fmt.Errorf("list clients: %w", err)
	}
	return list, total, nil
}

func (r *ProjectRepo) CountClients(ctx context.Context, projectID uuid.UUID) (int64, error) {
	var n int64
	if err := r.db.WithContext(ctx).Model(&model.Client{}).Where("project_id = ?", projectID).Count(&n).Error; err != nil {
		return 0, fmt.Errorf("count clients: %w", err)
	}
	return n, nil
}

func (r *ProjectRepo) ListClientsByIDs(ctx context.Context, projectID uuid.UUID, ids []uuid.UUID) ([]model.Client, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	var list []model.Client
	if err := r.db.WithContext(ctx).Where("project_id = ? AND id IN ?", projectID, ids).Find(&list).Error; err != nil {
		return nil, fmt.Errorf("list clients by id: %w", err)
	}
	return list, nil
}

func (r *ProjectRepo) CountActiveClients(ctx context.Context, projectID uuid.UUID, since time.Time) (int64, error) {
	var n int64
	if err := r.db.WithContext(ctx).Model(&model.Client{}).
		Where("project_id = ? AND last_check_at >= ?", projectID, since.UTC()).
		Count(&n).Error; err != nil {
		return 0, fmt.Errorf("count active clients: %w", err)
	}
	return n, nil
}

func (r *ProjectRepo) ClientBuckets(ctx context.Context, projectID uuid.UUID, now time.Time) (ClientBuckets, error) {
	var out ClientBuckets
	var err error
	if out.Total, err = r.CountClients(ctx, projectID); err != nil {
		return out, err
	}
	if out.Active24h, err = r.CountActiveClients(ctx, projectID, now.Add(-24*time.Hour)); err != nil {
		return out, err
	}
	if out.Active7d, err = r.CountActiveClients(ctx, projectID, now.Add(-7*24*time.Hour)); err != nil {
		return out, err
	}
	if out.Versions, err = r.groupClientColumn(ctx, projectID, "last_version"); err != nil {
		return out, err
	}
	if out.OS, err = r.groupClientColumn(ctx, projectID, "last_os"); err != nil {
		return out, err
	}
	if out.Arch, err = r.groupClientColumn(ctx, projectID, "last_arch"); err != nil {
		return out, err
	}
	if out.Channels, err = r.groupClientColumn(ctx, projectID, "last_channel"); err != nil {
		return out, err
	}
	if out.Countries, err = r.groupClientCountries(ctx, projectID); err != nil {
		return out, err
	}
	type recencyRow struct {
		Bucket int64
		Count  int64
	}
	var recency []recencyRow
	sql := `SELECT FLOOR(EXTRACT(EPOCH FROM (? - last_check_at)) / 3600) AS bucket, COUNT(*) AS count
FROM clients WHERE project_id = ? AND last_check_at IS NOT NULL
GROUP BY 1 ORDER BY 1`
	if err := r.db.WithContext(ctx).Raw(sql, now.UTC(), projectID).Scan(&recency).Error; err != nil {
		return out, fmt.Errorf("client recency: %w", err)
	}
	for _, row := range recency {
		out.RecencyHours = append(out.RecencyHours, NameCount{Name: fmt.Sprintf("%d", row.Bucket), Count: row.Count})
	}
	return out, nil
}

func (r *ProjectRepo) groupClientColumn(ctx context.Context, projectID uuid.UUID, col string) ([]NameCount, error) {
	var rows []NameCount
	sql := fmt.Sprintf(`SELECT COALESCE(%s, '') AS name, COUNT(*) AS count FROM clients WHERE project_id = ? GROUP BY 1 ORDER BY count DESC, name ASC`, col)
	if err := r.db.WithContext(ctx).Raw(sql, projectID).Scan(&rows).Error; err != nil {
		return nil, fmt.Errorf("group %s: %w", col, err)
	}
	return rows, nil
}

func (r *ProjectRepo) groupClientCountries(ctx context.Context, projectID uuid.UUID) ([]CountryCount, error) {
	type agg struct {
		Code  string `gorm:"column:code"`
		Count int64  `gorm:"column:count"`
	}
	var aggs []agg
	sql := `SELECT COALESCE(country_code, '') AS code, COUNT(*) AS count FROM clients WHERE project_id = ? GROUP BY 1 ORDER BY count DESC, code ASC`
	if err := r.db.WithContext(ctx).Raw(sql, projectID).Scan(&aggs).Error; err != nil {
		return nil, fmt.Errorf("group countries: %w", err)
	}
	type nameRow struct {
		Code string       `gorm:"column:code"`
		I18n model.GeoI18n `gorm:"column:geo_i18n"`
	}
	var names []nameRow
	nameSQL := `SELECT DISTINCT ON (COALESCE(country_code, '')) COALESCE(country_code, '') AS code, geo_i18n
FROM clients WHERE project_id = ? ORDER BY COALESCE(country_code, ''), updated_at DESC`
	if err := r.db.WithContext(ctx).Raw(nameSQL, projectID).Scan(&names).Error; err != nil {
		return nil, fmt.Errorf("country names: %w", err)
	}
	byCode := map[string]map[string]string{}
	for _, row := range names {
		if row.I18n.Country != nil {
			byCode[row.Code] = row.I18n.Country
		} else {
			byCode[row.Code] = map[string]string{}
		}
	}
	out := make([]CountryCount, 0, len(aggs))
	for _, a := range aggs {
		n := byCode[a.Code]
		if n == nil {
			n = map[string]string{}
		}
		out = append(out, CountryCount{Code: a.Code, Count: a.Count, Names: n})
	}
	return out, nil
}

func (r *ProjectRepo) ListClientDailyStats(ctx context.Context, projectID uuid.UUID, from, to time.Time) ([]model.ClientDailyStats, error) {
	var list []model.ClientDailyStats
	q := r.db.WithContext(ctx).Where("project_id = ?", projectID)
	if !from.IsZero() {
		q = q.Where("day >= ?", from.UTC().Truncate(24*time.Hour))
	}
	if !to.IsZero() {
		q = q.Where("day <= ?", to.UTC().Truncate(24*time.Hour))
	}
	if err := q.Order("day ASC").Find(&list).Error; err != nil {
		return nil, fmt.Errorf("list client daily stats: %w", err)
	}
	return list, nil
}

func (r *ProjectRepo) InsertGraySnapshot(ctx context.Context, snap *model.GrayRolloutSnapshot) error {
	if snap == nil {
		return nil
	}
	if err := r.db.WithContext(ctx).Create(snap).Error; err != nil {
		return fmt.Errorf("insert gray snapshot: %w", err)
	}
	return nil
}

func (r *ProjectRepo) ListGraySnapshots(ctx context.Context, versionID uuid.UUID, from, to time.Time) ([]model.GrayRolloutSnapshot, error) {
	q := r.db.WithContext(ctx).Where("version_id = ?", versionID)
	if !from.IsZero() {
		q = q.Where("taken_at >= ?", from.UTC())
	}
	if !to.IsZero() {
		q = q.Where("taken_at <= ?", to.UTC())
	}
	var list []model.GrayRolloutSnapshot
	if err := q.Order("taken_at ASC").Find(&list).Error; err != nil {
		return nil, fmt.Errorf("list gray snapshots: %w", err)
	}
	return list, nil
}
