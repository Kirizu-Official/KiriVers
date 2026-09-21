package repository

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/Kirizu-Official/KiriVers/internal/model"
)

func (r *ProjectRepo) ListByIDs(ctx context.Context, ids []uuid.UUID) ([]model.Project, error) {
	if len(ids) == 0 {
		return []model.Project{}, nil
	}
	var list []model.Project
	if err := r.db.WithContext(ctx).Where("id IN ?", ids).Order("created_at asc").Find(&list).Error; err != nil {
		return nil, fmt.Errorf("list projects by id: %w", err)
	}
	return list, nil
}

func (r *ProjectRepo) CreateMember(ctx context.Context, member *model.ProjectMember) error {
	if err := r.db.WithContext(ctx).Create(member).Error; err != nil {
		return fmt.Errorf("create project member: %w", err)
	}
	return nil
}

func (r *ProjectRepo) GetMember(ctx context.Context, projectID, adminID uuid.UUID) (*model.ProjectMember, error) {
	var row model.ProjectMember
	err := r.db.WithContext(ctx).Where("project_id = ? AND admin_id = ?", projectID, adminID).First(&row).Error
	if err != nil {
		return nil, err
	}
	return &row, nil
}

func (r *ProjectRepo) ListMembers(ctx context.Context, projectID uuid.UUID) ([]model.ProjectMember, error) {
	var list []model.ProjectMember
	if err := r.db.WithContext(ctx).Where("project_id = ?", projectID).Order("created_at asc").Find(&list).Error; err != nil {
		return nil, fmt.Errorf("list project members: %w", err)
	}
	return list, nil
}

func (r *ProjectRepo) ListProjectIDsForAdmin(ctx context.Context, adminID uuid.UUID) ([]uuid.UUID, error) {
	var ids []uuid.UUID
	if err := r.db.WithContext(ctx).Model(&model.ProjectMember{}).
		Where("admin_id = ?", adminID).Pluck("project_id", &ids).Error; err != nil {
		return nil, fmt.Errorf("list project ids for admin: %w", err)
	}
	return ids, nil
}

func (r *ProjectRepo) CountOwners(ctx context.Context, projectID uuid.UUID) (int64, error) {
	var n int64
	if err := r.db.WithContext(ctx).Model(&model.ProjectMember{}).
		Where("project_id = ? AND role = ?", projectID, model.ProjectMemberRoleOwner).
		Count(&n).Error; err != nil {
		return 0, fmt.Errorf("count project owners: %w", err)
	}
	return n, nil
}

func (r *ProjectRepo) SaveMember(ctx context.Context, member *model.ProjectMember) error {
	if err := r.db.WithContext(ctx).Save(member).Error; err != nil {
		return fmt.Errorf("save project member: %w", err)
	}
	return nil
}

func (r *ProjectRepo) DeleteMember(ctx context.Context, projectID, adminID uuid.UUID) error {
	res := r.db.WithContext(ctx).Where("project_id = ? AND admin_id = ?", projectID, adminID).Delete(&model.ProjectMember{})
	if res.Error != nil {
		return fmt.Errorf("delete project member: %w", res.Error)
	}
	if res.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

func (r *ProjectRepo) DeleteClient(ctx context.Context, projectID, id uuid.UUID) error {
	res := r.db.WithContext(ctx).Where("project_id = ? AND id = ?", projectID, id).Delete(&model.Client{})
	if res.Error != nil {
		return fmt.Errorf("delete client: %w", res.Error)
	}
	if res.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}
