package service

import (
	"errors"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"

	"github.com/Kirizu-Official/KiriVers/internal/model"
	"github.com/Kirizu-Official/KiriVers/internal/repository"
)

func TestProjectNameCreateAndPatch(t *testing.T) {
	ctx := t.Context()
	svc := NewProjectService(repository.NewMemoryProjectStore())

	slug := "name-app"
	p, _, err := svc.Create(ctx, CreateProjectInput{DefaultLocale: ptr("en"), Slug: &slug})
	if err != nil {
		t.Fatal(err)
	}
	if p.Name != slug {
		t.Fatalf("default name=%q want slug", p.Name)
	}

	title := "Acme Desktop"
	p, _, err = svc.Patch(ctx, slug, CreateProjectInput{DefaultLocale: ptr("en"), Name: &title})
	if err != nil {
		t.Fatal(err)
	}
	if p.Name != title {
		t.Fatalf("patched name=%q", p.Name)
	}

	blank := "   "
	if _, _, err := svc.Patch(ctx, slug, CreateProjectInput{DefaultLocale: ptr("en"), Name: &blank}); !errors.Is(err, ErrInvalidProjectSettings) {
		t.Fatalf("blank name: %v", err)
	}

	tooLong := strings.Repeat("名", model.MaxProjectNameRunes+1)
	if utf8.RuneCountInString(tooLong) <= model.MaxProjectNameRunes {
		t.Fatal("test setup")
	}
	if _, _, err := svc.Patch(ctx, slug, CreateProjectInput{DefaultLocale: ptr("en"), Name: &tooLong}); !errors.Is(err, ErrInvalidProjectSettings) {
		t.Fatalf("long name: %v", err)
	}

	named := "other-app"
	label := "其它产品"
	p2, _, err := svc.Create(ctx, CreateProjectInput{DefaultLocale: ptr("en"), Slug: &named, Name: &label})
	if err != nil {
		t.Fatal(err)
	}
	if p2.Name != label {
		t.Fatalf("create name=%q", p2.Name)
	}
	dup := "dup-name"
	if _, _, err := svc.Create(ctx, CreateProjectInput{DefaultLocale: ptr("en"), Slug: &dup, Name: &label}); err != nil {
		t.Fatalf("duplicate names must be allowed: %v", err)
	}

	wsSlug := "ws-name"
	blankCreate := "  \t"
	if _, _, err := svc.Create(ctx, CreateProjectInput{DefaultLocale: ptr("en"), Slug: &wsSlug, Name: &blankCreate}); !errors.Is(err, ErrInvalidProjectSettings) {
		t.Fatalf("create whitespace name: %v", err)
	}
}

func TestProjectDisplayNameFallback(t *testing.T) {
	p := model.Project{Slug: "slug-only", Name: ""}
	if p.DisplayName() != "slug-only" {
		t.Fatalf("empty name DisplayName=%q", p.DisplayName())
	}
	p.Name = "Title"
	if p.DisplayName() != "Title" {
		t.Fatalf("stored name DisplayName=%q", p.DisplayName())
	}
}

func TestProjectStatsLatestAndCounts(t *testing.T) {
	ctx := t.Context()
	store := repository.NewMemoryProjectStore()
	tel := repository.NewMemoryTelemetryStore()
	svc := NewProjectService(store)
	svc.SetTelemetryStore(tel)

	slug := "stats-app"
	p, _, err := svc.Create(ctx, CreateProjectInput{DefaultLocale: ptr("en"), Slug: &slug})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()

	mustVersion(t, store, p.ID, "1.0.0", "stable", model.VersionStatusPublished, now.Add(-time.Hour))
	mustVersion(t, store, p.ID, "2.0.0-beta.1", "beta", model.VersionStatusPublished, now)
	mustVersion(t, store, p.ID, "3.0.0", "stable", model.VersionStatusDraft, now)
	mustVersion(t, store, p.ID, "0.9.0", "stable", model.VersionStatusRevoked, now)

	if err := store.CreateArtifact(ctx, &model.Artifact{
		ProjectID: p.ID, VersionID: uuid.New(), VersionLineID: uuid.New(),
		StorageKey: "blob-a", Size: 100, SHA256: strings.Repeat("a", 64), MD5: strings.Repeat("b", 32),
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.CreateArtifact(ctx, &model.Artifact{
		ProjectID: p.ID, VersionID: uuid.New(), VersionLineID: uuid.New(),
		StorageKey: "blob-a", Size: 100, SHA256: strings.Repeat("a", 64), MD5: strings.Repeat("b", 32),
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.CreateArtifact(ctx, &model.Artifact{
		ProjectID: p.ID, VersionID: uuid.New(), VersionLineID: uuid.New(),
		StorageKey: "blob-b", Size: 50, SHA256: strings.Repeat("c", 64), MD5: strings.Repeat("d", 32),
	}); err != nil {
		t.Fatal(err)
	}

	expired := now.Add(-time.Hour)
	future := now.Add(time.Hour)
	if err := store.CreateToken(ctx, &model.ProjectToken{ProjectID: p.ID, Name: "live", TokenHash: "h1", Fingerprint: "fp1live0"}); err != nil {
		t.Fatal(err)
	}
	if err := store.CreateToken(ctx, &model.ProjectToken{ProjectID: p.ID, Name: "old", TokenHash: "h2", Fingerprint: "fp2old00", ExpiresAt: &expired}); err != nil {
		t.Fatal(err)
	}
	if err := store.CreateCIToken(ctx, &model.CIToken{ProjectID: p.ID, Name: "ci", TokenHash: "h3", Fingerprint: "fp3ci000", ExpiresAt: &future}); err != nil {
		t.Fatal(err)
	}

	if err := tel.Insert(ctx, &model.TelemetryEvent{
		ProjectID: p.ID, Status: model.TelemetryStatusInstalled, CreatedAt: now.Add(-time.Hour),
	}); err != nil {
		t.Fatal(err)
	}
	if err := tel.Insert(ctx, &model.TelemetryEvent{
		ProjectID: p.ID, Status: model.TelemetryStatusFailed, CreatedAt: now.Add(-time.Hour),
	}); err != nil {
		t.Fatal(err)
	}
	if err := tel.Insert(ctx, &model.TelemetryEvent{
		ProjectID: p.ID, Status: model.TelemetryStatusFailed, CreatedAt: now.Add(-48 * time.Hour),
	}); err != nil {
		t.Fatal(err)
	}

	stats, err := svc.StatsFor(ctx, []uuid.UUID{p.ID}, map[uuid.UUID]string{p.ID: model.CompareEngineSemver}, now)
	if err != nil {
		t.Fatal(err)
	}
	st := stats[p.ID]
	if st.LatestVersion == nil || st.LatestVersion.Version != "2.0.0-beta.1" || st.LatestVersion.Channel != "beta" {
		t.Fatalf("latest=%+v", st.LatestVersion)
	}
	if st.Versions.Total != 4 || st.Versions.Published != 2 || st.Versions.Draft != 1 || st.Versions.Revoked != 1 {
		t.Fatalf("versions=%+v", st.Versions)
	}
	if st.Channels != 3 {
		t.Fatalf("channels=%d want 3 system seeds", st.Channels)
	}
	if st.ArtifactCount != 3 || st.StorageBytes != 150 {
		t.Fatalf("storage count=%d bytes=%d", st.ArtifactCount, st.StorageBytes)
	}
	if st.ProjectTokens.Total != 2 || st.ProjectTokens.Active != 1 {
		t.Fatalf("project tokens=%+v", st.ProjectTokens)
	}
	if st.CITokens.Total != 1 || st.CITokens.Active != 1 {
		t.Fatalf("ci tokens=%+v", st.CITokens)
	}
	if st.Telemetry.Installed24h != 1 || st.Telemetry.Failed24h != 1 {
		t.Fatalf("telemetry=%+v", st.Telemetry)
	}

	draftOnly := "draft-only"
	p2, _, err := svc.Create(ctx, CreateProjectInput{DefaultLocale: ptr("en"), Slug: &draftOnly})
	if err != nil {
		t.Fatal(err)
	}
	mustVersion(t, store, p2.ID, "0.1.0", "stable", model.VersionStatusDraft, now)
	stats2, err := svc.StatsFor(ctx, []uuid.UUID{p2.ID}, map[uuid.UUID]string{p2.ID: model.CompareEngineSemver}, now)
	if err != nil {
		t.Fatal(err)
	}
	if stats2[p2.ID].LatestVersion != nil {
		t.Fatalf("draft-only latest=%+v", stats2[p2.ID].LatestVersion)
	}
}

func TestProjectStatsIntegerEngine(t *testing.T) {
	ctx := t.Context()
	store := repository.NewMemoryProjectStore()
	svc := NewProjectService(store)
	slug := "int-app"
	engine := model.CompareEngineInteger
	p, _, err := svc.Create(ctx, CreateProjectInput{DefaultLocale: ptr("en"), Slug: &slug, CompareEngine: &engine})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	mustIntVersion(t, store, p.ID, 10, "stable", model.VersionStatusPublished, now.Add(-time.Hour))
	mustIntVersion(t, store, p.ID, 5, "beta", model.VersionStatusPublished, now)
	stats, err := svc.StatsFor(ctx, []uuid.UUID{p.ID}, map[uuid.UUID]string{p.ID: model.CompareEngineInteger}, now)
	if err != nil {
		t.Fatal(err)
	}
	st := stats[p.ID]
	if st.LatestVersion == nil || st.LatestVersion.Version != "10" || st.LatestVersion.Channel != "stable" {
		t.Fatalf("integer latest=%+v want 10/stable (not newer-created 5)", st.LatestVersion)
	}
}

func TestNewerReleasedTieBreak(t *testing.T) {
	sem := "1.0.0"
	now := time.Now().UTC()
	published := &model.Version{
		Status: model.VersionStatusPublished, VersionSemver: &sem, VersionSemverCanonical: &sem,
		CreatedAt: now.Add(-time.Hour),
	}
	deprecated := &model.Version{
		Status: model.VersionStatusDeprecated, VersionSemver: &sem, VersionSemverCanonical: &sem,
		CreatedAt: now,
	}
	if !newerReleased(model.CompareEngineSemver, published, deprecated) {
		t.Fatal("published should beat later deprecated at the same compare key")
	}
	if newerReleased(model.CompareEngineSemver, deprecated, published) {
		t.Fatal("deprecated should not beat published at the same compare key")
	}
	earlier := &model.Version{
		Status: model.VersionStatusPublished, VersionSemver: &sem, VersionSemverCanonical: &sem,
		CreatedAt: now.Add(-time.Hour),
	}
	later := &model.Version{
		Status: model.VersionStatusPublished, VersionSemver: &sem, VersionSemverCanonical: &sem,
		CreatedAt: now,
	}
	if !newerReleased(model.CompareEngineSemver, later, earlier) {
		t.Fatal("later created_at should win when status and compare key tie")
	}
}

func TestProjectStatsNilTelemetryIsZero(t *testing.T) {
	ctx := t.Context()
	svc := NewProjectService(repository.NewMemoryProjectStore())
	slug := "no-tel"
	p, _, err := svc.Create(ctx, CreateProjectInput{DefaultLocale: ptr("en"), Slug: &slug})
	if err != nil {
		t.Fatal(err)
	}
	stats, err := svc.StatsFor(ctx, []uuid.UUID{p.ID}, map[uuid.UUID]string{p.ID: p.CompareEngine}, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	if stats[p.ID].Telemetry.Installed24h != 0 || stats[p.ID].Telemetry.Failed24h != 0 {
		t.Fatalf("nil telemetry store should yield zeros: %+v", stats[p.ID].Telemetry)
	}
}

func mustVersion(t *testing.T, store *repository.MemoryProjectStore, projectID uuid.UUID, ver, channel, status string, created time.Time) {
	t.Helper()
	s := ver
	v := &model.Version{
		ProjectID:              projectID,
		ChannelSlug:            channel,
		Status:                 status,
		VersionSemver:          &s,
		VersionSemverCanonical: &s,
		CreatedAt:              created,
	}
	if err := store.CreateVersion(t.Context(), v); err != nil {
		t.Fatal(err)
	}
}

func mustIntVersion(t *testing.T, store *repository.MemoryProjectStore, projectID uuid.UUID, n int64, channel, status string, created time.Time) {
	t.Helper()
	num := n
	v := &model.Version{
		ProjectID:      projectID,
		ChannelSlug:    channel,
		Status:         status,
		VersionInteger: &num,
		CreatedAt:      created,
	}
	if err := store.CreateVersion(t.Context(), v); err != nil {
		t.Fatal(err)
	}
}
