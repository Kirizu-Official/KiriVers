package repository

import (
	"context"
	"fmt"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/Kirizu-Official/KiriVers/internal/model"
)

func (r *ProjectRepo) AdmitGray(ctx context.Context, project *model.Project, version *model.Version, now time.Time) (*GrayAdmitResult, error) {
	if project == nil || version == nil {
		return nil, fmt.Errorf("admit gray: missing project or version")
	}
	now = now.UTC()
	out := &GrayAdmitResult{}
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var v model.Version
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&v, "id = ?", version.ID).Error; err != nil {
			return err
		}
		var n int64
		if err := tx.Model(&model.Client{}).Where("project_id = ?", v.ProjectID).Count(&n).Error; err != nil {
			return err
		}
		var allowlisted int64
		if err := tx.Model(&model.GrayAllowlist{}).Where("version_id = ?", v.ID).Count(&allowlisted).Error; err != nil {
			return err
		}
		if v.GrayIsComplete() {
			out.N, out.Allowlisted, out.Desired, out.TargetPercent = int(n), int(allowlisted), int(n), 100
			out.Completed = true
			out.Version = &v
			return nil
		}
		if n == 0 {
			if v.GrayStartedAt == nil {
				v.GrayStartedAt = &now
			}
			v.GrayCompletedAt = &now
			if err := tx.Save(&v).Error; err != nil {
				return err
			}
			out.N, out.Desired, out.Allowlisted, out.TargetPercent = 0, 0, 0, 100
			out.Completed = true
			out.Version = &v
			return nil
		}
		started := now
		if v.GrayStartedAt != nil {
			started = v.GrayStartedAt.UTC()
		} else {
			v.GrayStartedAt = &now
		}
		target, desired := model.GrayCoverage(int(n), v.GrayStartPercent, v.GrayStepPercent, v.GrayIntervalSeconds, started, now)
		need := desired - int(allowlisted)
		if need > 0 {
			var remaining []model.Client
			if err := tx.Where("project_id = ? AND device_hash NOT IN (?)",
				v.ProjectID,
				tx.Model(&model.GrayAllowlist{}).Select("device_id").Where("version_id = ?", v.ID),
			).Find(&remaining).Error; err != nil {
				return err
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
			if len(entries) > 0 {
				if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&entries).Error; err != nil {
					return err
				}
			}
			if err := tx.Model(&model.GrayAllowlist{}).Where("version_id = ?", v.ID).Count(&allowlisted).Error; err != nil {
				return err
			}
		}
		completed := target >= 100 || int(allowlisted) >= int(n)
		if completed {
			v.GrayCompletedAt = &now
		}
		if err := tx.Save(&v).Error; err != nil {
			return err
		}
		out.N, out.Desired, out.Allowlisted, out.TargetPercent = int(n), desired, int(allowlisted), target
		out.Completed = completed
		out.Version = &v
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("admit gray: %w", err)
	}
	return out, nil
}
