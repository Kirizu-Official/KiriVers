package service

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/Kirizu-Official/KiriVers/internal/model"
	"github.com/Kirizu-Official/KiriVers/internal/repository"
	"github.com/Kirizu-Official/KiriVers/internal/storage"
)

func TestNodeRegisterReusesIdentityFile(t *testing.T) {
	root := t.TempDir()
	store := repository.NewMemoryNodeStore()
	id := uuid.MustParse("aaaaaaaa-bbbb-4ccc-8ddd-eeeeeeeeeeee")
	if err := os.WriteFile(filepath.Join(root, storage.NodeIdentityFileName), []byte(id.String()), 0o600); err != nil {
		t.Fatal(err)
	}
	svc := NewNodeService(NodeServiceOptions{Store: store, LocalRoot: root, DisplayName: "alpha", AdminEnabled: true})
	row, err := svc.Register(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if row.ID != id || row.DisplayName != "alpha" {
		t.Fatalf("row=%+v", row)
	}
	again, err := NewNodeService(NodeServiceOptions{Store: store, LocalRoot: root, DisplayName: "beta", AdminEnabled: false}).Register(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if again.ID != id {
		t.Fatalf("uuid changed: %s", again.ID)
	}
	if again.DisplayName != "beta" || again.AdminEnabled {
		t.Fatalf("display/admin not updated: %+v", again)
	}
	raw, err := os.ReadFile(filepath.Join(root, storage.NodeIdentityFileName))
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(string(raw)) != id.String() {
		t.Fatalf("file=%q", raw)
	}
}

func TestNodeRegisterRetriesUnique(t *testing.T) {
	root := t.TempDir()
	store := repository.NewMemoryNodeStore()
	taken := uuid.MustParse("11111111-1111-4111-8111-111111111111")
	fresh := uuid.MustParse("22222222-2222-4222-8222-222222222222")
	if err := store.Create(t.Context(), &model.Node{ID: taken, DisplayName: "taken"}); err != nil {
		t.Fatal(err)
	}
	n := 0
	svc := NewNodeService(NodeServiceOptions{
		Store:     store,
		LocalRoot: root,
		NewUUID: func() (uuid.UUID, error) {
			n++
			if n == 1 {
				return taken, nil
			}
			return fresh, nil
		},
	})
	row, err := svc.Register(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if row.ID != fresh {
		t.Fatalf("id=%s", row.ID)
	}
	if n < 2 {
		t.Fatal("expected unique retry")
	}
}

func TestClaimFilterOwnerAndTypes(t *testing.T) {
	jobs := repository.NewMemoryJobRepo()
	admin := uuid.MustParse("aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa")
	replica := uuid.MustParse("bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb")
	auto := &model.Job{Type: model.JobTypeAutoDelta, Status: model.JobStatusQueued, OwnerNodeID: &admin}
	if err := jobs.Create(t.Context(), auto); err != nil {
		t.Fatal(err)
	}
	pack := &model.Job{Type: model.JobTypeDynamicPack, Status: model.JobStatusQueued}
	if err := jobs.Create(t.Context(), pack); err != nil {
		t.Fatal(err)
	}
	got, err := jobs.Claim(t.Context(), repository.ClaimFilter{NodeID: replica, Types: []string{model.JobTypeDynamicPack}})
	if err != nil || got == nil || got.Type != model.JobTypeDynamicPack {
		t.Fatalf("replica pack: %+v %v", got, err)
	}
	stolen, err := jobs.Claim(t.Context(), repository.ClaimFilter{NodeID: replica, Types: []string{model.JobTypeDynamicPack}})
	if err != nil {
		t.Fatal(err)
	}
	if stolen != nil {
		t.Fatalf("replica must not claim auto_delta: %+v", stolen)
	}
	unknown := &model.Job{Type: "no-such-type", Status: model.JobStatusQueued}
	if err := jobs.Create(t.Context(), unknown); err != nil {
		t.Fatal(err)
	}
	claimed, err := jobs.Claim(t.Context(), repository.ClaimFilter{NodeID: replica})
	if err != nil || claimed == nil || claimed.Type != "no-such-type" {
		t.Fatalf("unknown should be claimable without type filter: %+v %v", claimed, err)
	}
	if err := jobs.Release(t.Context(), claimed.ID); err != nil {
		t.Fatal(err)
	}
	back, err := jobs.GetByID(t.Context(), claimed.ID)
	if err != nil || back.Status != model.JobStatusQueued {
		t.Fatalf("released=%+v", back)
	}
}
