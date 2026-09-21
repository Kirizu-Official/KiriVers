package model

import (
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestGrayCoverageT0AndTicks(t *testing.T) {
	now := time.Date(2026, 9, 17, 12, 0, 0, 0, time.UTC)
	target, desired := GrayCoverage(10, 10, 10, 60, now, now)
	if target != 10 || desired != 1 {
		t.Fatalf("t0: target=%d desired=%d", target, desired)
	}
	target, desired = GrayCoverage(10, 10, 10, 60, now, now.Add(3*time.Minute))
	if target != 40 || desired != 4 {
		t.Fatalf("k=3: target=%d desired=%d", target, desired)
	}
	target, desired = GrayCoverage(0, 10, 10, 60, now, now)
	if target != 100 || desired != 0 {
		t.Fatalf("empty N: target=%d desired=%d", target, desired)
	}
}

func TestRankClientsForGrayUnweightedOldestFirst(t *testing.T) {
	t0 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	a := Client{ID: uuid.MustParse("00000000-0000-0000-0000-00000000000a"), CreatedAt: t0.Add(2 * time.Hour)}
	b := Client{ID: uuid.MustParse("00000000-0000-0000-0000-00000000000b"), CreatedAt: t0}
	c := Client{ID: uuid.MustParse("00000000-0000-0000-0000-00000000000c"), CreatedAt: t0.Add(time.Hour)}
	got := RankClientsForGray([]Client{a, b, c}, false)
	if got[0].ID != b.ID || got[1].ID != c.ID || got[2].ID != a.ID {
		t.Fatalf("unweighted order = %v", []uuid.UUID{got[0].ID, got[1].ID, got[2].ID})
	}
}

func TestRankClientsForGrayWeightedTenureRecency(t *testing.T) {
	t0 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	recent := t0.Add(10 * time.Hour)
	oldRecent := t0
	checkRecent := recent
	// oldest + most recent check should rank first when weighted.
	best := Client{ID: uuid.MustParse("00000000-0000-0000-0000-000000000001"), CreatedAt: oldRecent, LastCheckAt: &checkRecent}
	newestIdle := Client{ID: uuid.MustParse("00000000-0000-0000-0000-000000000002"), CreatedAt: recent}
	got := RankClientsForGray([]Client{newestIdle, best}, true)
	if got[0].ID != best.ID {
		t.Fatalf("weighted should prefer older+recent, got %s", got[0].ID)
	}
}
