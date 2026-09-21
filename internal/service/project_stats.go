package service

import (
	"context"
	"strconv"
	"time"

	"github.com/google/uuid"

	"github.com/Kirizu-Official/KiriVers/internal/model"
	"github.com/Kirizu-Official/KiriVers/internal/service/update"
)

// ProjectLatestVersion 是列表卡片上的「最新已发布版本」。
type ProjectLatestVersion struct {
	Version string `json:"version"`
	Channel string `json:"channel"`
	Status  string `json:"status"`
}

// VersionStatusStats 是 Version 生命周期计数。
type VersionStatusStats struct {
	Total      int64 `json:"total"`
	Draft      int64 `json:"draft"`
	Published  int64 `json:"published"`
	Deprecated int64 `json:"deprecated"`
	Revoked    int64 `json:"revoked"`
}

// TokenCountStats 是 Token 总数与未过期数。
type TokenCountStats struct {
	Total  int64 `json:"total"`
	Active int64 `json:"active"`
}

// TelemetryCountStats 是近 24h installed/failed 事件数。
type TelemetryCountStats struct {
	Installed24h int64 `json:"installed_24h"`
	Failed24h    int64 `json:"failed_24h"`
}

// ProjectStats 是管理端项目 list/get 附带的聚合统计。
type ProjectStats struct {
	LatestVersion *ProjectLatestVersion `json:"latest_version"`
	Versions      VersionStatusStats    `json:"versions"`
	Channels      int64                 `json:"channels"`
	MatrixRows    int64                 `json:"matrix_rows"`
	HwRevs        int64                 `json:"hw_revs"`
	ProjectTokens TokenCountStats       `json:"project_tokens"`
	CITokens      TokenCountStats       `json:"ci_tokens"`
	ArtifactCount int64                 `json:"artifact_count"`
	StorageBytes  int64                 `json:"storage_bytes"`
	Telemetry     TelemetryCountStats   `json:"telemetry"`
	Active7d      int64                 `json:"active_7d"`
}

// StatsFor 按项目 ID 批量计算 stats。engineByID 用 compare_engine 选最新版本。
func (s *ProjectService) StatsFor(ctx context.Context, ids []uuid.UUID, engineByID map[uuid.UUID]string, now time.Time) (map[uuid.UUID]ProjectStats, error) {
	out := make(map[uuid.UUID]ProjectStats, len(ids))
	for _, id := range ids {
		out[id] = ProjectStats{}
	}
	if len(ids) == 0 {
		return out, nil
	}

	versions, err := s.store.CountVersionsByStatus(ctx, ids)
	if err != nil {
		return nil, err
	}
	released, err := s.store.ListReleasedVersions(ctx, ids)
	if err != nil {
		return nil, err
	}
	channels, err := s.store.CountChannelsByProject(ctx, ids)
	if err != nil {
		return nil, err
	}
	matrix, err := s.store.CountMatrixByProject(ctx, ids)
	if err != nil {
		return nil, err
	}
	hwRevs, err := s.store.CountHwRevsByProject(ctx, ids)
	if err != nil {
		return nil, err
	}
	projectTokens, err := s.store.CountProjectTokens(ctx, ids, now)
	if err != nil {
		return nil, err
	}
	ciTokens, err := s.store.CountCITokens(ctx, ids, now)
	if err != nil {
		return nil, err
	}
	storage, err := s.store.ArtifactStorageStats(ctx, ids)
	if err != nil {
		return nil, err
	}

	var tel map[uuid.UUID]map[string]int64
	if s.telemetry != nil {
		tel, err = s.telemetry.CountStatusSince(ctx, ids, now.Add(-TelemetryDowngradeWindow), []string{
			model.TelemetryStatusInstalled, model.TelemetryStatusFailed,
		})
		if err != nil {
			return nil, err
		}
	}

	latest := pickLatestReleased(released, engineByID)

	for _, id := range ids {
		st := out[id]
		if vc, ok := versions[id]; ok {
			st.Versions = VersionStatusStats{
				Total: vc.Total, Draft: vc.Draft, Published: vc.Published,
				Deprecated: vc.Deprecated, Revoked: vc.Revoked,
			}
		}
		st.Channels = channels[id]
		st.MatrixRows = matrix[id]
		st.HwRevs = hwRevs[id]
		if row, ok := projectTokens[id]; ok {
			st.ProjectTokens = TokenCountStats{Total: row.Total, Active: row.Active}
		}
		if row, ok := ciTokens[id]; ok {
			st.CITokens = TokenCountStats{Total: row.Total, Active: row.Active}
		}
		if row, ok := storage[id]; ok {
			st.ArtifactCount = row.ArtifactCount
			st.StorageBytes = row.StorageBytes
		}
		if byStatus := tel[id]; byStatus != nil {
			st.Telemetry = TelemetryCountStats{
				Installed24h: byStatus[model.TelemetryStatusInstalled],
				Failed24h:    byStatus[model.TelemetryStatusFailed],
			}
		}
		if v, ok := latest[id]; ok {
			st.LatestVersion = v
		}
		if n, aerr := s.store.CountActiveClients(ctx, id, now.Add(-7*24*time.Hour)); aerr == nil {
			st.Active7d = n
		}
		out[id] = st
	}
	return out, nil
}

func pickLatestReleased(list []model.Version, engineByID map[uuid.UUID]string) map[uuid.UUID]*ProjectLatestVersion {
	best := map[uuid.UUID]*model.Version{}
	for i := range list {
		v := &list[i]
		engine := engineByID[v.ProjectID]
		cur := best[v.ProjectID]
		if cur == nil {
			if _, ok := update.CompareVersions(engine, v, v); !ok {
				continue
			}
			best[v.ProjectID] = v
			continue
		}
		if newerReleased(engine, v, cur) {
			best[v.ProjectID] = v
		}
	}
	out := map[uuid.UUID]*ProjectLatestVersion{}
	for id, v := range best {
		out[id] = &ProjectLatestVersion{
			Version: displayVersion(v),
			Channel: v.ChannelSlug,
			Status:  v.Status,
		}
	}
	return out
}

func newerReleased(engine string, candidate, current *model.Version) bool {
	c, ok := update.CompareVersions(engine, candidate, current)
	if !ok {
		return false
	}
	if c > 0 {
		return true
	}
	if c < 0 {
		return false
	}
	if candidate.Status == model.VersionStatusPublished && current.Status != model.VersionStatusPublished {
		return true
	}
	if candidate.Status != model.VersionStatusPublished && current.Status == model.VersionStatusPublished {
		return false
	}
	candTime := versionTime(candidate)
	curTime := versionTime(current)
	return candTime.After(curTime)
}

func versionTime(v *model.Version) time.Time {
	if v.PublishTime != nil && !v.PublishTime.IsZero() {
		return v.PublishTime.UTC()
	}
	return v.CreatedAt.UTC()
}

func displayVersion(v *model.Version) string {
	if v.VersionSemver != nil && *v.VersionSemver != "" {
		return *v.VersionSemver
	}
	if v.VersionInteger != nil {
		return strconv.FormatInt(*v.VersionInteger, 10)
	}
	return v.ID.String()
}
