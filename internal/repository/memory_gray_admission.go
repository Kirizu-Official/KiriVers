package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/Kirizu-Official/KiriVers/internal/model"
)

func (m *MemoryProjectStore) AdmitGray(_ context.Context, project *model.Project, version *model.Version, now time.Time) (*GrayAdmitResult, error) {
	if project == nil || version == nil {
		return nil, fmt.Errorf("admit gray: missing project or version")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	now = now.UTC()
	v := m.versions[version.ID]
	if v == nil {
		return nil, fmt.Errorf("admit gray: version not found")
	}
	var n int
	for _, c := range m.clients {
		if c.ProjectID == v.ProjectID {
			n++
		}
	}
	var allowlisted int
	listed := map[string]bool{}
	for _, e := range m.grayAllowlist {
		if e.VersionID == v.ID {
			allowlisted++
			listed[e.DeviceID] = true
		}
	}
	if v.GrayIsComplete() {
		return &GrayAdmitResult{N: n, Desired: n, Allowlisted: allowlisted, TargetPercent: 100, Completed: true, Version: cloneVersion(v)}, nil
	}
	if n == 0 {
		if v.GrayStartedAt == nil {
			t := now
			v.GrayStartedAt = &t
		}
		t := now
		v.GrayCompletedAt = &t
		return &GrayAdmitResult{N: 0, Desired: 0, Allowlisted: 0, TargetPercent: 100, Completed: true, Version: cloneVersion(v)}, nil
	}
	started := now
	if v.GrayStartedAt != nil {
		started = v.GrayStartedAt.UTC()
	} else {
		t := now
		v.GrayStartedAt = &t
	}
	target, desired := model.GrayCoverage(n, v.GrayStartPercent, v.GrayStepPercent, v.GrayIntervalSeconds, started, now)
	need := desired - allowlisted
	if need > 0 {
		var remaining []model.Client
		for _, c := range m.clients {
			if c.ProjectID == v.ProjectID && !listed[c.DeviceHash] {
				remaining = append(remaining, *c)
			}
		}
		ranked := model.RankClientsForGray(remaining, project.GrayWeightTenureActivity)
		if len(ranked) > need {
			ranked = ranked[:need]
		}
		entries := make([]model.GrayAllowlist, 0, len(ranked))
		for i := range ranked {
			entries = append(entries, model.GrayAllowlist{
				ProjectID: v.ProjectID,
				VersionID: v.ID,
				DeviceID:  ranked[i].DeviceHash,
				Source:    model.GraySourceAuto,
				CreatedAt: now,
			})
		}
		if err := m.insertAllowlistLocked(entries); err != nil {
			return nil, err
		}
		allowlisted = 0
		for _, e := range m.grayAllowlist {
			if e.VersionID == v.ID {
				allowlisted++
			}
		}
	}
	completed := target >= 100 || allowlisted >= n
	if completed {
		t := now
		v.GrayCompletedAt = &t
	}
	return &GrayAdmitResult{
		N: n, Desired: desired, Allowlisted: allowlisted, TargetPercent: target,
		Completed: completed, Version: cloneVersion(v),
	}, nil
}
