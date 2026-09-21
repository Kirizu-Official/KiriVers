package service

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/Kirizu-Official/KiriVers/internal/model"
	"github.com/Kirizu-Official/KiriVers/internal/repository"
)

func setupAnnouncementService(t *testing.T) (*AnnouncementService, *repository.MemoryProjectStore, *model.Project) {
	t.Helper()
	projStore := repository.NewMemoryProjectStore()
	svc := NewAnnouncementService(repository.NewMemoryAnnouncementStore(), projStore)
	slug := "ann-svc"
	p, _, err := NewProjectService(projStore, nil).Create(t.Context(), CreateProjectInput{DefaultLocale: ptr("en"), Slug: &slug})
	if err != nil {
		t.Fatal(err)
	}
	return svc, projStore, p
}

func TestAnnouncementIllegalScope(t *testing.T) {
	svc, _, p := setupAnnouncementService(t)
	_, err := svc.Create(t.Context(), p.ID, AnnouncementCreateInput{
		OS: "windows", Arch: "x86_64", Language: "en", Title: "Hi",
	})
	if !IsInvalidRequest(err) {
		t.Fatalf("os+arch without version: got %v, want INVALID_REQUEST", err)
	}
}

func TestAnnouncementSevenScopesLegal(t *testing.T) {
	svc, projStore, p := setupAnnouncementService(t)
	ctx := t.Context()
	ver, _, err := NewProjectService(projStore, nil).PutVersion(ctx, p.ID, "1.0.0", VersionWriteInput{Channel: "stable"})
	if err != nil {
		t.Fatal(err)
	}
	base := AnnouncementCreateInput{Language: "en", Title: "Hi"}
	cases := []AnnouncementCreateInput{
		base,
		{VersionID: &ver.ID, Language: "en", Title: "Hi"},
		{OS: "windows", Language: "en", Title: "Hi"},
		{Arch: "x86_64", Language: "en", Title: "Hi"},
		{VersionID: &ver.ID, OS: "windows", Language: "en", Title: "Hi"},
		{VersionID: &ver.ID, Arch: "x86_64", Language: "en", Title: "Hi"},
		{VersionID: &ver.ID, OS: "windows", Arch: "x86_64", Language: "en", Title: "Hi"},
	}
	for i, in := range cases {
		if _, err := svc.Create(ctx, p.ID, in); err != nil {
			t.Fatalf("scope %d: %v", i, err)
		}
	}
}

func TestAnnouncementVersionScopeRequiresExistingVersion(t *testing.T) {
	svc, _, p := setupAnnouncementService(t)
	missing := uuid.Must(uuid.NewRandom())
	_, err := svc.Create(t.Context(), p.ID, AnnouncementCreateInput{
		VersionID: &missing,
		Language:  "en",
		Title:     "v",
	})
	if !errors.Is(err, ErrVersionNotFound) {
		t.Fatalf("got %v, want VERSION_NOT_FOUND", err)
	}
}

func TestAnnouncementVersionIDMustBelongToProject(t *testing.T) {
	svc, projStore, p := setupAnnouncementService(t)
	ctx := t.Context()
	projSvc := NewProjectService(projStore, nil)
	otherSlug := "ann-other"
	other, _, err := projSvc.Create(ctx, CreateProjectInput{DefaultLocale: ptr("en"), Slug: &otherSlug})
	if err != nil {
		t.Fatal(err)
	}
	foreign, _, err := projSvc.PutVersion(ctx, other.ID, "1.0.0", VersionWriteInput{Channel: "stable"})
	if err != nil {
		t.Fatal(err)
	}
	_, err = svc.Create(ctx, p.ID, AnnouncementCreateInput{
		VersionID: &foreign.ID, Language: "en", Title: "x",
	})
	if !errors.Is(err, ErrVersionNotFound) {
		t.Fatalf("foreign version id: got %v, want VERSION_NOT_FOUND", err)
	}
}

func TestAnnouncementMatchWindowAndLocale(t *testing.T) {
	svc, projStore, p := setupAnnouncementService(t)
	ctx := t.Context()
	projSvc := NewProjectService(projStore, nil)
	ver, _, err := projSvc.PutVersion(ctx, p.ID, "1.0.0", VersionWriteInput{Channel: "stable"})
	if err != nil {
		t.Fatal(err)
	}

	now := time.Date(2026, 9, 16, 12, 0, 0, 0, time.UTC)
	future := now.Add(time.Hour)
	past := now.Add(-time.Hour)

	mk := func(in AnnouncementCreateInput, status string) *model.Announcement {
		t.Helper()
		row, err := svc.Create(ctx, p.ID, in)
		if err != nil {
			t.Fatal(err)
		}
		if status == model.AnnouncementStatusPublished {
			st := model.AnnouncementStatusPublished
			row, err = svc.Patch(ctx, p.ID, row.ID, AnnouncementPatchInput{Status: &st})
			if err != nil {
				t.Fatal(err)
			}
		}
		return row
	}

	projectWide := mk(AnnouncementCreateInput{Language: "en", Title: "all"}, model.AnnouncementStatusPublished)
	versionOnly := mk(AnnouncementCreateInput{VersionID: &ver.ID, Language: "en", Title: "ver"}, model.AnnouncementStatusPublished)
	archOnly := mk(AnnouncementCreateInput{Arch: "amd64", Language: "en", Title: "arch"}, model.AnnouncementStatusPublished)
	platform := mk(AnnouncementCreateInput{
		VersionID: &ver.ID, OS: "darwin", Arch: "amd64",
		Language: "en", Title: "plat",
	}, model.AnnouncementStatusPublished)
	_ = mk(AnnouncementCreateInput{Language: "en", Title: "draft"}, model.AnnouncementStatusDraft)
	_ = mk(AnnouncementCreateInput{
		Language: "en", Title: "future",
		StartsAt: &future,
	}, model.AnnouncementStatusPublished)
	_ = mk(AnnouncementCreateInput{
		Language: "en", Title: "expired",
		EndsAt: &past,
	}, model.AnnouncementStatusPublished)
	zhOnly := mk(AnnouncementCreateInput{Language: "zh-CN", Title: "中文", Subtitle: "副"}, model.AnnouncementStatusPublished)

	full, err := svc.ListClient(ctx, p, ClientListInput{Version: "1.0.0", OS: "darwin", Arch: "amd64", Locale: "en", Now: now})
	if err != nil {
		t.Fatal(err)
	}
	if len(full.Items) != 4 {
		t.Fatalf("want 4 matching published en rows, got %d %+v", len(full.Items), titles(full.Items))
	}
	wantOrder := []string{"all", "ver", "arch", "plat"}
	for i, title := range wantOrder {
		if full.Items[i].Title != title {
			t.Fatalf("order[%d]=%s want %s", i, full.Items[i].Title, title)
		}
	}

	ids := []uuid.UUID{platform.ID, projectWide.ID, versionOnly.ID, archOnly.ID, zhOnly.ID}
	// include remaining rows so permutation covers ALL ids
	adminList, err := svc.ListAdmin(ctx, p.ID)
	if err != nil {
		t.Fatal(err)
	}
	seen := map[uuid.UUID]struct{}{}
	var perm []uuid.UUID
	for _, id := range ids {
		perm = append(perm, id)
		seen[id] = struct{}{}
	}
	for i := range adminList {
		if _, ok := seen[adminList[i].ID]; !ok {
			perm = append(perm, adminList[i].ID)
		}
	}
	if err := svc.Reorder(ctx, p.ID, perm); err != nil {
		t.Fatal(err)
	}
	reordered, err := svc.ListClient(ctx, p, ClientListInput{Version: "1.0.0", OS: "macos", Arch: "x86_64", Locale: "en", Now: now})
	if err != nil {
		t.Fatal(err)
	}
	if reordered.Items[0].Title != "plat" || reordered.Items[1].Title != "all" {
		t.Fatalf("reorder not applied: %v", titles(reordered.Items))
	}

	noVersion, err := svc.ListClient(ctx, p, ClientListInput{Arch: "x86_64", Locale: "en", Now: now})
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range noVersion.Items {
		if item.Title == "ver" || item.Title == "plat" {
			t.Fatalf("version-scoped leaked without version: %s", item.Title)
		}
	}

	noArch, err := svc.ListClient(ctx, p, ClientListInput{Version: "1.0.0", Locale: "en", Now: now})
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range noArch.Items {
		if item.Title == "arch" || item.Title == "plat" {
			t.Fatalf("arch-scoped leaked without arch: %s", item.Title)
		}
	}

	unknown, err := svc.ListClient(ctx, p, ClientListInput{Version: "9.9.9", Locale: "en", Now: now})
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range unknown.Items {
		if item.Title == "ver" || item.Title == "plat" {
			t.Fatalf("unknown version matched version-scoped row")
		}
	}

	_, err = svc.ListClient(ctx, p, ClientListInput{Version: "not a version", Locale: "en", Now: now})
	if !errors.Is(err, ErrInvalidQueryParam) {
		t.Fatalf("unparsable version: %v", err)
	}

	enMiss, err := svc.ListClient(ctx, p, ClientListInput{Locale: "en", Now: now})
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range enMiss.Items {
		if item.Title == "中文" {
			t.Fatalf("explicit locale=en must not leftover to zh-CN: %+v", item)
		}
	}
	omitLeftover, err := svc.ListClient(ctx, p, ClientListInput{Now: now})
	if err != nil {
		t.Fatal(err)
	}
	foundZHOmit := false
	for _, item := range omitLeftover.Items {
		if item.Title == "中文" {
			t.Fatalf("omit locale with default en must pick en rows, not leftover zh while en exists: %+v", titles(omitLeftover.Items))
		}
		if item.Locale == "en" {
			foundZHOmit = true
		}
	}
	if !foundZHOmit {
		t.Fatal("omit locale with default en should return en rows when they exist")
	}
	zhHit, err := svc.ListClient(ctx, p, ClientListInput{Locale: "zh-CN", Now: now})
	if err != nil {
		t.Fatal(err)
	}
	foundZH := false
	for _, item := range zhHit.Items {
		if item.Title == "中文" {
			foundZH = true
			if item.Subtitle != "副" || item.Locale != "zh-CN" {
				t.Fatalf("locale bundle mixed: %+v", item)
			}
		}
	}
	if !foundZH {
		t.Fatal("zh-CN bundle not returned")
	}

	if !strings.Contains(full.CacheControl, "s-maxage=") {
		t.Fatalf("cache-control=%s", full.CacheControl)
	}
}

func TestAnnouncementWindowBoundaryCap(t *testing.T) {
	svc, _, p := setupAnnouncementService(t)
	now := time.Date(2026, 9, 16, 12, 0, 0, 0, time.UTC)
	soon := now.Add(10 * time.Second)
	row, err := svc.Create(t.Context(), p.ID, AnnouncementCreateInput{
		Language: "en", Title: "soon",
		StartsAt: &soon,
	})
	if err != nil {
		t.Fatal(err)
	}
	st := model.AnnouncementStatusPublished
	if _, err := svc.Patch(t.Context(), p.ID, row.ID, AnnouncementPatchInput{Status: &st}); err != nil {
		t.Fatal(err)
	}
	res, err := svc.ListClient(t.Context(), p, ClientListInput{Locale: "en", Now: now})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Items) != 0 {
		t.Fatalf("future start should be hidden, got %v", titles(res.Items))
	}
	if res.CacheControl != "public, s-maxage=10, stale-while-revalidate=30" {
		t.Fatalf("s-maxage cap=%s", res.CacheControl)
	}
}

func titles(items []ClientAnnouncement) []string {
	out := make([]string, 0, len(items))
	for _, it := range items {
		out = append(out, it.Title)
	}
	return out
}

func TestAnnouncementReorderIncomplete(t *testing.T) {
	svc, _, p := setupAnnouncementService(t)
	a, err := svc.Create(t.Context(), p.ID, AnnouncementCreateInput{Language: "en", Title: "a"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Create(t.Context(), p.ID, AnnouncementCreateInput{Language: "en", Title: "b"}); err != nil {
		t.Fatal(err)
	}
	if err := svc.Reorder(t.Context(), p.ID, []uuid.UUID{a.ID}); !IsInvalidRequest(err) {
		t.Fatalf("incomplete reorder: %v", err)
	}
}

func TestAnnouncementEmptyTitleRejected(t *testing.T) {
	svc, _, p := setupAnnouncementService(t)
	_, err := svc.Create(t.Context(), p.ID, AnnouncementCreateInput{
		Language: "en", Title: "   ",
	})
	if !IsInvalidRequest(err) {
		t.Fatalf("empty title: %v", err)
	}
	_, err = svc.Create(t.Context(), p.ID, AnnouncementCreateInput{
		Language: "x", Title: "ok",
	})
	if !IsInvalidRequest(err) {
		t.Fatalf("invalid language: %v", err)
	}
}

func TestAnnouncementEndsBeforeStarts(t *testing.T) {
	svc, _, p := setupAnnouncementService(t)
	start := time.Date(2026, 9, 16, 12, 0, 0, 0, time.UTC)
	end := start.Add(-time.Minute)
	_, err := svc.Create(t.Context(), p.ID, AnnouncementCreateInput{
		Language: "en", Title: "w",
		StartsAt: &start,
		EndsAt:   &end,
	})
	if !IsInvalidRequest(err) {
		t.Fatalf("got %v", err)
	}
}

// AC10：定时发布在 starts_at 到期后，客户端 GET 视为已发布并惰性落库。
func TestAnnouncementScheduledLazyFlip(t *testing.T) {
	svc, _, p := setupAnnouncementService(t)
	ctx := t.Context()
	start := time.Now().UTC().Add(time.Hour)
	row, err := svc.Create(ctx, p.ID, AnnouncementCreateInput{
		Language: "en", Title: "later",
		StartsAt: &start,
	})
	if err != nil {
		t.Fatal(err)
	}
	st := model.AnnouncementStatusScheduled
	row, err = svc.Patch(ctx, p.ID, row.ID, AnnouncementPatchInput{
		Status: &st, StartsAtSet: true, StartsAt: &start,
	})
	if err != nil {
		t.Fatal(err)
	}
	if row.Status != model.AnnouncementStatusScheduled {
		t.Fatalf("status=%s want scheduled", row.Status)
	}

	hidden, err := svc.ListClient(ctx, p, ClientListInput{Locale: "en", Now: time.Now().UTC()})
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range hidden.Items {
		if item.Title == "later" {
			t.Fatal("future scheduled must stay off client GET")
		}
	}

	shown, err := svc.ListClient(ctx, p, ClientListInput{Locale: "en", Now: start.Add(time.Second)})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, item := range shown.Items {
		if item.Title == "later" {
			found = true
		}
	}
	if !found {
		t.Fatalf("elapsed scheduled must be client-visible, got %v", titles(shown.Items))
	}

	admin, err := svc.GetAdmin(ctx, p.ID, row.ID)
	if err != nil {
		t.Fatal(err)
	}
	if admin.Status != model.AnnouncementStatusPublished {
		t.Fatalf("lazy flip must persist published, got %s", admin.Status)
	}
}

func TestAnnouncementLocaleD5(t *testing.T) {
	svc, projStore, p := setupAnnouncementService(t)
	ctx := t.Context()
	now := time.Date(2026, 9, 16, 12, 0, 0, 0, time.UTC)
	st := model.AnnouncementStatusPublished
	publish := func(language, title string) {
		t.Helper()
		row, err := svc.Create(ctx, p.ID, AnnouncementCreateInput{Language: language, Title: title})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := svc.Patch(ctx, p.ID, row.ID, AnnouncementPatchInput{Status: &st}); err != nil {
			t.Fatal(err)
		}
	}

	publish("zh-CN", "中文")
	onlyZH, err := svc.ListClient(ctx, p, ClientListInput{Locale: "en", Now: now})
	if err != nil {
		t.Fatal(err)
	}
	if len(onlyZH.Items) != 0 {
		t.Fatalf("explicit en with only zh must be empty, got %v", titles(onlyZH.Items))
	}
	if containsString(onlyZH.Vary, "Accept-Language") {
		t.Fatalf("explicit locale must not Vary Accept-Language: %v", onlyZH.Vary)
	}

	leftover, err := svc.ListClient(ctx, p, ClientListInput{Now: now})
	if err != nil {
		t.Fatal(err)
	}
	if len(leftover.Items) != 1 || leftover.Items[0].Title != "中文" || leftover.Items[0].Locale != "zh-CN" {
		t.Fatalf("omit locale leftover zh: %+v", leftover.Items)
	}
	if !containsString(leftover.Vary, "Accept-Language") {
		t.Fatalf("omitted locale must Vary Accept-Language: %v", leftover.Vary)
	}

	fr, err := svc.ListClient(ctx, p, ClientListInput{Locale: "fr", Now: now})
	if err != nil {
		t.Fatal(err)
	}
	if len(fr.Items) != 0 {
		t.Fatalf("explicit missing locale must be empty, got %v", titles(fr.Items))
	}

	publish("en", "English")
	enOnly, err := svc.ListClient(ctx, p, ClientListInput{Locale: "en", Now: now})
	if err != nil {
		t.Fatal(err)
	}
	if len(enOnly.Items) != 1 || enOnly.Items[0].Title != "English" {
		t.Fatalf("locale=en with zh+en: %v", titles(enOnly.Items))
	}

	acceptZH, err := svc.ListClient(ctx, p, ClientListInput{AcceptLanguage: "zh-CN,zh;q=0.9", Now: now})
	if err != nil {
		t.Fatal(err)
	}
	if len(acceptZH.Items) != 1 || acceptZH.Items[0].Title != "中文" {
		t.Fatalf("Accept-Language zh-CN should win when locale omitted: %v", titles(acceptZH.Items))
	}

	_ = projStore
}

func TestAnnouncementMatchSkipsDeletedVersion(t *testing.T) {
	svc, projStore, p := setupAnnouncementService(t)
	ctx := t.Context()
	projSvc := NewProjectService(projStore, nil)
	ver, _, err := projSvc.PutVersion(ctx, p.ID, "1.0.0", VersionWriteInput{Channel: "stable"})
	if err != nil {
		t.Fatal(err)
	}
	row, err := svc.Create(ctx, p.ID, AnnouncementCreateInput{VersionID: &ver.ID, Language: "en", Title: "bound"})
	if err != nil {
		t.Fatal(err)
	}
	st := model.AnnouncementStatusPublished
	if _, err := svc.Patch(ctx, p.ID, row.ID, AnnouncementPatchInput{Status: &st}); err != nil {
		t.Fatal(err)
	}
	hit, err := svc.ListClient(ctx, p, ClientListInput{Version: "1.0.0", Locale: "en"})
	if err != nil {
		t.Fatal(err)
	}
	if len(hit.Items) != 1 || hit.Items[0].Title != "bound" {
		t.Fatalf("want bound row, got %v", titles(hit.Items))
	}
	if err := projSvc.DeleteVersion(ctx, p.ID, "1.0.0"); err != nil {
		t.Fatal(err)
	}
	miss, err := svc.ListClient(ctx, p, ClientListInput{Version: "1.0.0", Locale: "en"})
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range miss.Items {
		if item.Title == "bound" {
			t.Fatal("deleted Version must not match version-scoped announcement")
		}
	}
	admin, err := svc.GetAdmin(ctx, p.ID, row.ID)
	if err != nil || admin.Title != "bound" {
		t.Fatalf("admin row must remain: %v %+v", err, admin)
	}
}

func containsString(have []string, want string) bool {
	for _, h := range have {
		if h == want {
			return true
		}
	}
	return false
}
