package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/Kirizu-Official/KiriVers/internal/model"
)

var (
	// ErrForbidden 已登录但无权执行该操作 → HTTP 403。
	ErrForbidden = errors.New("forbidden")
	// ErrLastOwner 不可删除或降级项目最后一名拥有者 → HTTP 400。
	ErrLastOwner = errors.New("cannot remove the last owner")
	// ErrMemberNotFound 成员关系不存在 → HTTP 404。
	ErrMemberNotFound = errors.New("project member not found")
	// ErrMemberExists 同一项目已有该账号 → HTTP 400。
	ErrMemberExists = errors.New("admin is already a member of this project")
)

// MemberView 是管理端成员 JSON 的领域形状。
type MemberView struct {
	AdminID   uuid.UUID
	Username  string
	Role      string
	CreatedAt time.Time
}

// MemberWrite 是 POST/PATCH 成员输入。
type MemberWrite struct {
	Username string
	Password *string
	Role     string
}

// SetAdmins 注入管理员服务（成员 CRUD 与创建项目指定拥有者）。
func (s *ProjectService) SetAdmins(admins *AdminService) {
	s.admins = admins
}

// ListVisible 平台管理员看全部项目；否则只返回有成员关系的项目。
func (s *ProjectService) ListVisible(ctx context.Context, admin *model.Admin) ([]model.Project, error) {
	if admin == nil {
		return nil, ErrForbidden
	}
	if admin.IsPlatformAdmin {
		return s.store.List(ctx)
	}
	ids, err := s.store.ListProjectIDsForAdmin(ctx, admin.ID)
	if err != nil {
		return nil, err
	}
	return s.store.ListByIDs(ctx, ids)
}

// HasProjectMembership 是否有该项目成员行（不含平台超管短路）。
func (s *ProjectService) HasProjectMembership(ctx context.Context, adminID, projectID uuid.UUID) bool {
	_, err := s.store.GetMember(ctx, projectID, adminID)
	return err == nil
}

// CanAccessProject 平台管理员或成员可进项目。
func (s *ProjectService) CanAccessProject(ctx context.Context, admin *model.Admin, projectID uuid.UUID) bool {
	if admin == nil {
		return false
	}
	if admin.IsPlatformAdmin {
		return true
	}
	return s.HasProjectMembership(ctx, admin.ID, projectID)
}

// MemberRole 读取该账号在项目中的角色；平台管理员无行时返回空串。
func (s *ProjectService) MemberRole(ctx context.Context, adminID, projectID uuid.UUID) (string, error) {
	m, err := s.store.GetMember(ctx, projectID, adminID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return "", ErrMemberNotFound
	}
	if err != nil {
		return "", err
	}
	return m.Role, nil
}

// ListMembers 返回项目成员（带用户名）。
func (s *ProjectService) ListMembers(ctx context.Context, projectID uuid.UUID) ([]MemberView, error) {
	list, err := s.store.ListMembers(ctx, projectID)
	if err != nil {
		return nil, err
	}
	out := make([]MemberView, 0, len(list))
	for i := range list {
		m := list[i]
		username := ""
		if s.admins != nil {
			if a, err := s.admins.Get(ctx, m.AdminID); err == nil {
				username = a.Username
			}
		}
		out = append(out, MemberView{
			AdminID:   m.AdminID,
			Username:  username,
			Role:      m.Role,
			CreatedAt: m.CreatedAt,
		})
	}
	return out, nil
}

// AddMember 将已有账号或新账号加入项目。actor 决定能否任命 owner。
func (s *ProjectService) AddMember(ctx context.Context, projectID uuid.UUID, actor *model.Admin, in MemberWrite) (*MemberView, error) {
	role, err := normalizeMemberRole(in.Role)
	if err != nil {
		return nil, err
	}
	if err := s.assertCanAssignRole(ctx, actor, projectID, role); err != nil {
		return nil, err
	}
	username := strings.TrimSpace(in.Username)
	if s.admins == nil {
		return nil, fmt.Errorf("%w: admin store unavailable", ErrInvalidProjectSettings)
	}
	var target *model.Admin
	if in.Password == nil || strings.TrimSpace(*in.Password) == "" {
		existing, err := s.admins.GetByUsername(ctx, username)
		if err != nil {
			return nil, err
		}
		target = existing
	} else {
		created, err := s.admins.CreateAccount(ctx, username, *in.Password, false)
		if err != nil {
			return nil, err
		}
		target = created
	}
	if _, err := s.store.GetMember(ctx, projectID, target.ID); err == nil {
		return nil, ErrMemberExists
	} else if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	row := &model.ProjectMember{
		ProjectID: projectID,
		AdminID:   target.ID,
		Role:      role,
	}
	if err := s.store.CreateMember(ctx, row); err != nil {
		if isUniqueViolation(err) {
			return nil, ErrMemberExists
		}
		return nil, err
	}
	return &MemberView{AdminID: target.ID, Username: target.Username, Role: role, CreatedAt: row.CreatedAt}, nil
}

// PatchMember 仅平台管理员可改角色（owner↔admin）。
func (s *ProjectService) PatchMember(ctx context.Context, projectID, adminID uuid.UUID, actor *model.Admin, role string) (*MemberView, error) {
	if actor == nil || !actor.IsPlatformAdmin {
		return nil, ErrForbidden
	}
	role, err := normalizeMemberRole(role)
	if err != nil {
		return nil, err
	}
	row, err := s.store.GetMember(ctx, projectID, adminID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrMemberNotFound
	}
	if err != nil {
		return nil, err
	}
	if row.Role == model.ProjectMemberRoleOwner && role != model.ProjectMemberRoleOwner {
		n, err := s.store.CountOwners(ctx, projectID)
		if err != nil {
			return nil, err
		}
		if n <= 1 {
			return nil, ErrLastOwner
		}
	}
	row.Role = role
	row.UpdatedAt = time.Now().UTC()
	if err := s.store.SaveMember(ctx, row); err != nil {
		return nil, err
	}
	username := ""
	if s.admins != nil {
		if a, err := s.admins.Get(ctx, adminID); err == nil {
			username = a.Username
		}
	}
	return &MemberView{AdminID: adminID, Username: username, Role: row.Role, CreatedAt: row.CreatedAt}, nil
}

// RemoveMember 平台可删拥有者/管理员；拥有者只能删管理员。最后一名拥有者不可删。
func (s *ProjectService) RemoveMember(ctx context.Context, projectID, adminID uuid.UUID, actor *model.Admin) error {
	if actor == nil {
		return ErrForbidden
	}
	row, err := s.store.GetMember(ctx, projectID, adminID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return ErrMemberNotFound
	}
	if err != nil {
		return err
	}
	if row.Role == model.ProjectMemberRoleOwner {
		if !actor.IsPlatformAdmin {
			return ErrForbidden
		}
		n, err := s.store.CountOwners(ctx, projectID)
		if err != nil {
			return err
		}
		if n <= 1 {
			return ErrLastOwner
		}
	} else if !actor.IsPlatformAdmin {
		own, err := s.store.GetMember(ctx, projectID, actor.ID)
		if err != nil || own.Role != model.ProjectMemberRoleOwner {
			return ErrForbidden
		}
	}
	return s.store.DeleteMember(ctx, projectID, adminID)
}

func (s *ProjectService) assertCanAssignRole(ctx context.Context, actor *model.Admin, projectID uuid.UUID, role string) error {
	if actor == nil {
		return ErrForbidden
	}
	if actor.IsPlatformAdmin {
		return nil
	}
	if role == model.ProjectMemberRoleOwner {
		return ErrForbidden
	}
	own, err := s.store.GetMember(ctx, projectID, actor.ID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return ErrForbidden
	}
	if err != nil {
		return err
	}
	if own.Role != model.ProjectMemberRoleOwner {
		return ErrForbidden
	}
	return nil
}

func normalizeMemberRole(raw string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case model.ProjectMemberRoleOwner:
		return model.ProjectMemberRoleOwner, nil
	case model.ProjectMemberRoleAdmin:
		return model.ProjectMemberRoleAdmin, nil
	default:
		return "", fmt.Errorf("%w: role must be owner or admin", ErrInvalidProjectSettings)
	}
}

// GetJobAsAdmin 任务查询：平台管理员看全部；项目成员只能看自己能进的项目任务；实例级（project_id 空）仅平台。
func (s *ProjectService) GetJobAsAdmin(ctx context.Context, admin *model.Admin, jobID uuid.UUID) (*model.Job, error) {
	if admin == nil {
		return nil, ErrJobForbidden
	}
	if admin.IsPlatformAdmin {
		return s.GetJob(ctx, nil, true, jobID)
	}
	if s.jobs == nil {
		return nil, fmt.Errorf("job store is not configured")
	}
	job, err := s.jobs.GetByID(ctx, jobID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrJobNotFound
		}
		return nil, err
	}
	if job.ProjectID == nil {
		return nil, ErrJobForbidden
	}
	if !s.HasProjectMembership(ctx, admin.ID, *job.ProjectID) {
		return nil, ErrJobForbidden
	}
	return job, nil
}
