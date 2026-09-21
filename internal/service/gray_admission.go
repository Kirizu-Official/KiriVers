package service

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/Kirizu-Official/KiriVers/internal/model"
	"github.com/Kirizu-Official/KiriVers/internal/repository"
)

// EnsureProjectGrayAdmission 对本项目所有已发布且尚未转全量的版本懒放号。
// 登录、check、管理端灰度 GET/PATCH 共用。失败不阻断主路径（调用方忽略错误）。
func (s *ProjectService) EnsureProjectGrayAdmission(ctx context.Context, project *model.Project) error {
	if project == nil {
		return nil
	}
	list, err := s.store.ListVersions(ctx, project.ID)
	if err != nil {
		return err
	}
	var first error
	for i := range list {
		v := &list[i]
		if !v.GrayIsActive() {
			continue
		}
		if _, err := s.EnsureGrayAdmission(ctx, project, v); err != nil && first == nil {
			first = err
		}
	}
	return first
}

// EnsureGrayAdmission 对单一版本执行 D4 懒放号、写快照、必要时失效目录缓存。
func (s *ProjectService) EnsureGrayAdmission(ctx context.Context, project *model.Project, version *model.Version) (*repository.GrayAdmitResult, error) {
	if project == nil || version == nil {
		return nil, fmt.Errorf("ensure gray: missing project or version")
	}
	now := time.Now().UTC()
	res, err := s.store.AdmitGray(ctx, project, version, now)
	if err != nil {
		return nil, err
	}
	if res != nil && res.Version != nil {
		*version = *res.Version
	}
	if res != nil {
		updated := countUpdatedOnAllowlist(project.CompareEngine, version, nil, s, ctx, project.ID)
		if allow, aerr := s.store.ListAllowlist(ctx, project.ID, version.ID); aerr == nil {
			updated = countUpdatedOnAllowlist(project.CompareEngine, version, allow, s, ctx, project.ID)
		}
		snap := &model.GrayRolloutSnapshot{
			ProjectID:     project.ID,
			VersionID:     version.ID,
			TakenAt:       now,
			N:             res.N,
			Desired:       res.Desired,
			Allowlisted:   res.Allowlisted,
			Updated:       updated,
			TargetPercent: res.TargetPercent,
		}
		_ = s.store.InsertGraySnapshot(ctx, snap)
	}
	s.invalidateProject(ctx, project.ID)
	return res, nil
}

func countUpdatedOnAllowlist(engine string, version *model.Version, allow []model.GrayAllowlist, s *ProjectService, ctx context.Context, projectID uuid.UUID) int {
	if len(allow) == 0 {
		return 0
	}
	hashes := make([]string, 0, len(allow))
	for i := range allow {
		hashes = append(hashes, allow[i].DeviceID)
	}
	n := 0
	for _, h := range hashes {
		c, err := s.store.GetClientByDeviceHash(ctx, projectID, h)
		if err != nil || c == nil {
			continue
		}
		if ClientUpdatedTo(engine, c.LastVersion, version) {
			n++
		}
	}
	return n
}

// CompleteGray 立即转全量（运营者按钮）。已完成则幂等返回。
func (s *ProjectService) CompleteGray(ctx context.Context, projectID uuid.UUID, versionRef string) (*model.Version, error) {
	proj, err := s.store.GetByID(ctx, projectID)
	if err != nil {
		return nil, err
	}
	v, err := s.ResolveVersion(ctx, projectID, versionRef)
	if err != nil {
		return nil, err
	}
	if v.GrayIsComplete() {
		return v, nil
	}
	now := time.Now().UTC()
	if v.GrayStartedAt == nil {
		v.GrayStartedAt = &now
	}
	v.GrayCompletedAt = &now
	if err := s.store.SaveVersion(ctx, v); err != nil {
		return nil, err
	}
	n, _ := s.store.CountClients(ctx, projectID)
	allowlisted, _ := s.store.CountAllowlist(ctx, v.ID)
	_ = s.store.InsertGraySnapshot(ctx, &model.GrayRolloutSnapshot{
		ProjectID: projectID, VersionID: v.ID, TakenAt: now, N: int(n), Desired: int(n),
		Allowlisted: int(allowlisted), TargetPercent: 100,
	})
	s.invalidateProject(ctx, proj.ID)
	return v, nil
}

// PatchGrayKnobs 修改进行中（或草稿）版本的三旋钮。不重置 gray_started_at。
// 关键版本禁止灰度（start<100）。
func (s *ProjectService) PatchGrayKnobs(ctx context.Context, projectID uuid.UUID, versionRef string, start, step, interval *int) (*model.Version, error) {
	proj, err := s.store.GetByID(ctx, projectID)
	if err != nil {
		return nil, err
	}
	v, err := s.ResolveVersion(ctx, projectID, versionRef)
	if err != nil {
		return nil, err
	}
	if start != nil {
		if *start < 0 || *start > 100 {
			return nil, ErrInvalidRolloutPercent
		}
		v.GrayStartPercent = *start
	}
	if step != nil {
		if *step < 0 || *step > 100 {
			return nil, ErrInvalidRolloutPercent
		}
		v.GrayStepPercent = *step
	}
	if interval != nil {
		if *interval <= 0 {
			return nil, ErrInvalidRequest("gray_interval_seconds must be positive")
		}
		v.GrayIntervalSeconds = *interval
	}
	if v.IsCritical && v.GrayStartPercent < 100 {
		return nil, ErrGrayNotAllowedOnCritical
	}
	if err := s.store.SaveVersion(ctx, v); err != nil {
		return nil, err
	}
	if v.GrayIsActive() {
		if _, err := s.EnsureGrayAdmission(ctx, proj, v); err != nil {
			return nil, err
		}
	} else {
		s.invalidateProject(ctx, projectID)
	}
	return v, nil
}

// GrayStatus 是管理端灰度页状态。
type GrayStatus struct {
	Version       *model.Version
	N             int
	Desired       int
	Allowlisted   int
	Updated       int
	TargetPercent int
	ActualPercent int
	Completed     bool
}

// GetGrayStatus 推进放号后返回 KPI。
func (s *ProjectService) GetGrayStatus(ctx context.Context, project *model.Project, versionRef string) (*GrayStatus, error) {
	if project == nil {
		return nil, ErrProjectNotFound
	}
	v, err := s.ResolveVersion(ctx, project.ID, versionRef)
	if err != nil {
		return nil, err
	}
	if v.GrayIsActive() {
		if _, err := s.EnsureGrayAdmission(ctx, project, v); err != nil {
			return nil, err
		}
		fresh, rerr := s.store.GetVersionByID(ctx, v.ID)
		if rerr == nil {
			v = fresh
		}
	}
	n, err := s.store.CountClients(ctx, project.ID)
	if err != nil {
		return nil, err
	}
	allowlisted, err := s.store.CountAllowlist(ctx, v.ID)
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	target, desired := 100, int(n)
	if !v.GrayIsComplete() && v.GrayStartedAt != nil {
		target, desired = model.GrayCoverage(int(n), v.GrayStartPercent, v.GrayStepPercent, v.GrayIntervalSeconds, *v.GrayStartedAt, now)
	}
	allow, err := s.store.ListAllowlist(ctx, project.ID, v.ID)
	if err != nil {
		return nil, err
	}
	updated := countUpdatedOnAllowlist(project.CompareEngine, v, allow, s, ctx, project.ID)
	return &GrayStatus{
		Version:       v,
		N:             int(n),
		Desired:       desired,
		Allowlisted:   int(allowlisted),
		Updated:       updated,
		TargetPercent: target,
		ActualPercent: grayActualPercent(v.GrayIsComplete(), n, allowlisted),
		Completed:     v.GrayIsComplete(),
	}, nil
}

// grayActualPercent 是进行中灰度的实际发布率：allowlisted/N*100（全量或空名册为 100）。
func grayActualPercent(complete bool, n, allowlisted int64) int {
	if complete || n <= 0 {
		return 100
	}
	return int(float64(allowlisted) * 100.0 / float64(n))
}

// GrayActualPercents 批量计算版本列表/详情 chip 用的实际发布率。
// 进行中或已转全量才写入 map；草稿未开灰度的版本省略。
func (s *ProjectService) GrayActualPercents(ctx context.Context, projectID uuid.UUID, versions []model.Version) (map[uuid.UUID]int, error) {
	out := map[uuid.UUID]int{}
	if len(versions) == 0 {
		return out, nil
	}
	n, err := s.store.CountClients(ctx, projectID)
	if err != nil {
		return nil, err
	}
	for i := range versions {
		v := &versions[i]
		if v.GrayIsActive() {
			allowlisted, err := s.store.CountAllowlist(ctx, v.ID)
			if err != nil {
				return nil, err
			}
			out[v.ID] = grayActualPercent(false, n, allowlisted)
			continue
		}
		if v.GrayCompletedAt != nil {
			out[v.ID] = 100
		}
	}
	return out, nil
}

// GrayClientRow 是灰度页名单行。
type GrayClientRow struct {
	Client  model.Client
	Source  string
	Updated bool
}

// ListGrayClients 白名单 ∩ 名册，附是否已更新。
func (s *ProjectService) ListGrayClients(ctx context.Context, project *model.Project, versionRef string) ([]GrayClientRow, error) {
	if project == nil {
		return nil, ErrProjectNotFound
	}
	v, err := s.ResolveVersion(ctx, project.ID, versionRef)
	if err != nil {
		return nil, err
	}
	allow, err := s.store.ListAllowlist(ctx, project.ID, v.ID)
	if err != nil {
		return nil, err
	}
	out := make([]GrayClientRow, 0, len(allow))
	for i := range allow {
		c, err := s.store.GetClientByDeviceHash(ctx, project.ID, allow[i].DeviceID)
		if err != nil || c == nil {
			continue
		}
		out = append(out, GrayClientRow{
			Client:  *c,
			Source:  allow[i].Source,
			Updated: ClientUpdatedTo(project.CompareEngine, c.LastVersion, v),
		})
	}
	return out, nil
}

// ListGraySnapshots 灰度时间序列。
func (s *ProjectService) ListGraySnapshots(ctx context.Context, projectID uuid.UUID, versionRef string, from, to time.Time) ([]model.GrayRolloutSnapshot, error) {
	v, err := s.ResolveVersion(ctx, projectID, versionRef)
	if err != nil {
		return nil, err
	}
	if to.IsZero() {
		to = time.Now().UTC()
	}
	if from.IsZero() {
		from = to.Add(-30 * 24 * time.Hour)
	}
	return s.store.ListGraySnapshots(ctx, v.ID, from, to)
}
