package service

import (
	"testing"

	"github.com/Kirizu-Official/KiriVers/internal/model"
	"github.com/Kirizu-Official/KiriVers/internal/repository"
)

func TestListVisibleAndLastOwner(t *testing.T) {
	adminStore := repository.NewMemoryAdminStore()
	admins := NewTestAdminService(adminStore)
	projStore := repository.NewMemoryProjectStore()
	svc := NewProjectService(projStore)
	svc.SetAdmins(admins)
	ctx := t.Context()

	platform, err := admins.Create(ctx, "root", "password123")
	if err != nil {
		t.Fatal(err)
	}
	owner, err := admins.CreateAccount(ctx, "owner1", "password123", false)
	if err != nil {
		t.Fatal(err)
	}
	outsider, err := admins.CreateAccount(ctx, "outsider", "password123", false)
	if err != nil {
		t.Fatal(err)
	}

	slugA, slugB := "proj-a", "proj-b"
	a, _, err := svc.Create(ctx, CreateProjectInput{Slug: &slugA, DefaultLocale: ptr("en")})
	if err != nil {
		t.Fatal(err)
	}
	b, _, err := svc.Create(ctx, CreateProjectInput{Slug: &slugB, DefaultLocale: ptr("en")})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.AddMember(ctx, a.ID, platform, MemberWrite{Username: owner.Username, Role: model.ProjectMemberRoleOwner}); err != nil {
		t.Fatal(err)
	}

	visible, err := svc.ListVisible(ctx, owner)
	if err != nil {
		t.Fatal(err)
	}
	if len(visible) != 1 || visible[0].ID != a.ID {
		t.Fatalf("owner visible=%d", len(visible))
	}
	all, err := svc.ListVisible(ctx, platform)
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 2 {
		t.Fatalf("platform visible=%d", len(all))
	}
	none, err := svc.ListVisible(ctx, outsider)
	if err != nil {
		t.Fatal(err)
	}
	if len(none) != 0 {
		t.Fatalf("outsider visible=%d", len(none))
	}

	if _, err := svc.AddMember(ctx, a.ID, owner, MemberWrite{Username: "new-owner", Password: ptr("password123"), Role: model.ProjectMemberRoleOwner}); err != ErrForbidden {
		t.Fatalf("owner appoint owner: %v", err)
	}
	if err := svc.RemoveMember(ctx, a.ID, owner.ID, platform); err != ErrLastOwner {
		t.Fatalf("remove last owner: %v", err)
	}
	if _, err := svc.PatchMember(ctx, a.ID, owner.ID, platform, model.ProjectMemberRoleAdmin); err != ErrLastOwner {
		t.Fatalf("demote last owner: %v", err)
	}
	_ = b
}

func TestCreateProjectOwnerUsername(t *testing.T) {
	admins := NewTestAdminService(repository.NewMemoryAdminStore())
	svc := NewProjectService(repository.NewMemoryProjectStore())
	svc.SetAdmins(admins)
	ctx := t.Context()
	if _, err := admins.Create(ctx, "root", "password123"); err != nil {
		t.Fatal(err)
	}
	slug := "owned"
	user, pass := "alice", "password123"
	p, _, err := svc.Create(ctx, CreateProjectInput{
		Slug: &slug, DefaultLocale: ptr("en"),
		OwnerUsername: &user, OwnerPassword: &pass,
	})
	if err != nil {
		t.Fatal(err)
	}
	alice, err := admins.GetByUsername(ctx, "alice")
	if err != nil {
		t.Fatal(err)
	}
	if alice.IsPlatformAdmin {
		t.Fatal("created owner must not be platform admin")
	}
	role, err := svc.MemberRole(ctx, alice.ID, p.ID)
	if err != nil || role != model.ProjectMemberRoleOwner {
		t.Fatalf("role=%q err=%v", role, err)
	}
}
