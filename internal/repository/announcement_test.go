package repository

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"github.com/Kirizu-Official/KiriVers/internal/model"
)

func TestMemoryAnnouncementReorder(t *testing.T) {
	store := NewMemoryAnnouncementStore()
	ctx := context.Background()
	pid := uuid.New()
	var ids []uuid.UUID
	for i := 0; i < 3; i++ {
		row := &model.Announcement{
			ProjectID: pid,
			Status:    model.AnnouncementStatusDraft,
			SortOrder: i,
			Language:  "en",
			Title:     string(rune('a' + i)),
		}
		if err := store.Create(ctx, row); err != nil {
			t.Fatal(err)
		}
		ids = append(ids, row.ID)
	}
	perm := []uuid.UUID{ids[2], ids[0], ids[1]}
	if err := store.Reorder(ctx, pid, perm); err != nil {
		t.Fatal(err)
	}
	list, err := store.ListByProject(ctx, pid)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 3 || list[0].ID != perm[0] || list[0].SortOrder != 0 || list[1].ID != perm[1] || list[2].ID != perm[2] {
		t.Fatalf("reorder mismatch: %+v", list)
	}
	max, err := store.MaxSortOrder(ctx, pid)
	if err != nil || max != 2 {
		t.Fatalf("max=%d err=%v", max, err)
	}
	n, err := store.CountByProject(ctx, pid)
	if err != nil || n != 3 {
		t.Fatalf("count=%d err=%v", n, err)
	}
	other := uuid.New()
	n, err = store.CountByProject(ctx, other)
	if err != nil || n != 0 {
		t.Fatalf("other count=%d", n)
	}
}
