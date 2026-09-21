package repository

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/Kirizu-Official/KiriVers/internal/model"
)

func TestMemoryAdminUpdateLastLogin(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryAdminStore()
	admin := &model.Admin{Username: "root", PasswordHash: "hash"}
	if err := store.Create(ctx, admin); err != nil {
		t.Fatal(err)
	}
	originalName := admin.Username
	at := time.Now().UTC().Truncate(time.Second)
	ip := "192.0.2.1"
	if err := store.UpdateLastLogin(ctx, admin.ID, at, &ip); err != nil {
		t.Fatal(err)
	}
	got, err := store.GetByID(ctx, admin.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Username != originalName {
		t.Fatalf("username mutated: %q", got.Username)
	}
	if got.LastLoginAt == nil || !got.LastLoginAt.Equal(at) {
		t.Fatalf("last_login_at=%v want %v", got.LastLoginAt, at)
	}
	if got.LastLoginIP == nil || *got.LastLoginIP != ip {
		t.Fatalf("last_login_ip=%v", got.LastLoginIP)
	}

	if err := store.UpdateLastLogin(ctx, admin.ID, at.Add(time.Minute), nil); err != nil {
		t.Fatal(err)
	}
	got, err = store.GetByID(ctx, admin.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.LastLoginIP != nil {
		t.Fatalf("nil ip must clear last_login_ip, got %v", got.LastLoginIP)
	}
	if got.LastLoginAt == nil || !got.LastLoginAt.Equal(at.Add(time.Minute)) {
		t.Fatalf("second last_login_at=%v", got.LastLoginAt)
	}

	missing := uuid.New()
	if err := store.UpdateLastLogin(ctx, missing, at, &ip); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("missing id: %v", err)
	}
}
