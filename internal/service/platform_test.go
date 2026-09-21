package service

import (
	"errors"
	"testing"


	"github.com/Kirizu-Official/KiriVers/internal/model"
	"github.com/Kirizu-Official/KiriVers/internal/platform"
	"github.com/Kirizu-Official/KiriVers/internal/repository"
)

func TestSystemChannelsSeededAndIdempotent(t *testing.T) {
	ctx := t.Context()
	store := repository.NewMemoryProjectStore()
	svc := NewProjectService(store)
	slug := "chan-app"
	p, _, err := svc.Create(ctx, CreateProjectInput{DefaultLocale: ptr("en"), Slug: &slug})
	if err != nil {
		t.Fatal(err)
	}
	list, err := svc.ListChannels(ctx, p.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 3 {
		t.Fatalf("channels=%d", len(list))
	}
	want := []struct {
		slug string
		rank int
		sys  bool
	}{
		{model.ChannelAlpha, model.ChannelRankAlpha, true},
		{model.ChannelBeta, model.ChannelRankBeta, true},
		{model.ChannelStable, model.ChannelRankStable, true},
	}
	for i, w := range want {
		if list[i].Slug != w.slug || list[i].StabilityRank != w.rank || !list[i].System || !list[i].Enabled {
			t.Fatalf("channel[%d]=%+v want %+v", i, list[i], w)
		}
		if list[i].Name != w.slug {
			t.Fatalf("seeded name=%q want slug fallback %q", list[i].Name, w.slug)
		}
	}
	if err := store.SeedSystemChannels(ctx, p.ID, model.SystemChannelNames{}); err != nil {
		t.Fatal(err)
	}
	again, err := svc.ListChannels(ctx, p.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(again) != 3 {
		t.Fatalf("after reseed=%d", len(again))
	}
}

func TestCustomChannelAndSystemDelete(t *testing.T) {
	ctx := t.Context()
	svc := NewProjectService(repository.NewMemoryProjectStore())
	slug := "chan-crud"
	p, _, err := svc.Create(ctx, CreateProjectInput{DefaultLocale: ptr("en"), Slug: &slug})
	if err != nil {
		t.Fatal(err)
	}
	nightly := "nightly"
	rank := 5
	name := "Nightly"
	ch, err := svc.CreateChannel(ctx, p.ID, ChannelWrite{Name: &name, Slug: &nightly, StabilityRank: &rank})
	if err != nil {
		t.Fatal(err)
	}
	if ch.System || ch.Slug != "nightly" || ch.Name != "Nightly" || ch.StabilityRank != 5 || !ch.Enabled {
		t.Fatalf("custom=%+v", ch)
	}
	bad := "has_underscore"
	if _, err := svc.CreateChannel(ctx, p.ID, ChannelWrite{Slug: &bad}); !errors.Is(err, ErrInvalidChannel) {
		t.Fatalf("underscore: %v", err)
	}
	lts := "lts"
	ltsName := "LTS"
	if _, err := svc.CreateChannel(ctx, p.ID, ChannelWrite{Name: &ltsName, Slug: &lts}); err != nil {
		t.Fatalf("lts channel: %v", err)
	}
	ver, _, err := svc.PutVersion(ctx, p.ID, "1.0.0", VersionWriteInput{Channel: "lts"})
	if err != nil {
		t.Fatalf("put version in lts channel: %v", err)
	}
	if ver.IsLTS {
		t.Fatal("Version must not gain is_lts from channel slug lts")
	}
	if err := svc.DeleteChannel(ctx, p.ID, model.ChannelStable); !errors.Is(err, ErrSystemChannel) {
		t.Fatalf("delete stable: %v", err)
	}
	off := false
	patched, err := svc.PatchChannel(ctx, p.ID, model.ChannelStable, ChannelWrite{Enabled: &off})
	if err != nil {
		t.Fatal(err)
	}
	if patched.Enabled {
		t.Fatal("stable should be disabled")
	}
	if err := svc.DeleteChannel(ctx, p.ID, nightly); err != nil {
		t.Fatal(err)
	}
	sys := model.ChannelStable
	if _, err := svc.CreateChannel(ctx, p.ID, ChannelWrite{Slug: &sys}); !errors.Is(err, ErrSlugTaken) {
		t.Fatalf("recreate stable: %v", err)
	}

	bare := repository.NewMemoryProjectStore()
	orphan := &model.Project{Slug: "no-seed-channels"}
	if err := bare.Create(ctx, orphan); err != nil {
		t.Fatal(err)
	}
	bareSvc := NewProjectService(bare)
	if _, err := bareSvc.CreateChannel(ctx, orphan.ID, ChannelWrite{Slug: &sys}); !errors.Is(err, ErrSlugTaken) {
		t.Fatalf("reserved system slug without seed: %v", err)
	}
	if err := bareSvc.DeleteChannel(ctx, orphan.ID, model.ChannelStable); !errors.Is(err, ErrChannelNotFound) {
		t.Fatalf("unseeded stable must not exist as custom: %v", err)
	}
}

func TestMatrixDefaultsCanonicalAndPackageTypeLock(t *testing.T) {
	ctx := t.Context()
	store := repository.NewMemoryProjectStore()
	svc := NewProjectService(store)
	slug := "mtx-app"
	p, _, err := svc.Create(ctx, CreateProjectInput{DefaultLocale: ptr("en"), Slug: &slug})
	if err != nil {
		t.Fatal(err)
	}
	osRaw, archRaw, pkg := "darwin", "amd64", model.PackageTypeSingleFile
	row, err := svc.CreateMatrix(ctx, p.ID, MatrixWrite{OS: &osRaw, Arch: &archRaw, PackageType: &pkg})
	if err != nil {
		t.Fatal(err)
	}
	if row.OS != platform.OSMacOS || row.Arch != platform.ArchX86_64 {
		t.Fatalf("canonical os/arch=%s/%s", row.OS, row.Arch)
	}
	if row.DeltaAlgo != model.DeltaAlgoHDiffPatch {
		t.Fatalf("delta_algo=%s", row.DeltaAlgo)
	}
	if row.DeltaSourceCount != 3 {
		t.Fatalf("delta_source_count=%d", row.DeltaSourceCount)
	}
	if row.FallbackArch != "" {
		t.Fatalf("fallback_arch=%q", row.FallbackArch)
	}
	if row.MinimumSupportedVersion != nil {
		t.Fatalf("msv=%v", row.MinimumSupportedVersion)
	}
	if row.HwVariantPolicy != model.HwVariantIndependent {
		t.Fatalf("hw_variant_policy=%s", row.HwVariantPolicy)
	}
	macos, x64 := platform.OSMacOS, platform.ArchX86_64
	if _, err := svc.CreateMatrix(ctx, p.ID, MatrixWrite{OS: &macos, Arch: &x64, PackageType: &pkg}); !errors.Is(err, ErrMatrixExists) {
		t.Fatalf("darwin write must occupy macos/x86_64: %v", err)
	}
	osx := "osx"
	if _, err := svc.CreateMatrix(ctx, p.ID, MatrixWrite{OS: &osx, Arch: &archRaw, PackageType: &pkg}); !errors.Is(err, ErrMatrixExists) {
		t.Fatalf("osx write must occupy macos: %v", err)
	}
	if (model.VersionLine{}).TableName() != "version_lines" {
		t.Fatalf("VersionLine table=%s", (model.VersionLine{}).TableName())
	}

	multi := model.PackageTypeMultiFile
	single := model.PackageTypeSingleFile
	draft := &model.Version{ProjectID: p.ID, Status: model.VersionStatusDraft}
	if err := store.CreateVersion(ctx, draft); err != nil {
		t.Fatal(err)
	}
	if err := svc.CreateVersionLine(ctx, draft.ID, "darwin", "amd64"); err != nil {
		t.Fatal(err)
	}
	row, err = svc.PatchMatrix(ctx, p.ID, "darwin", "amd64", MatrixWrite{PackageType: &multi})
	if err != nil {
		t.Fatalf("draft Version Line must not lock package_type: %v", err)
	}
	if row.PackageType != model.PackageTypeMultiFile {
		t.Fatalf("unlocked patch=%s", row.PackageType)
	}

	ver, err := svc.CreatePublishedVersion(ctx, p.ID)
	if err != nil {
		t.Fatal(err)
	}
	row, err = svc.PatchMatrix(ctx, p.ID, "macos", "x86_64", MatrixWrite{PackageType: &single})
	if err != nil {
		t.Fatalf("published Version without this (os,arch) line must not lock: %v", err)
	}

	other, err := svc.CreatePublishedVersion(ctx, p.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.CreateVersionLine(ctx, other.ID, "linux", "arm64"); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.PatchMatrix(ctx, p.ID, "darwin", "amd64", MatrixWrite{PackageType: &multi}); err != nil {
		t.Fatalf("other (os,arch) published line must not lock: %v", err)
	}

	if err := svc.CreateVersionLine(ctx, ver.ID, "darwin", "amd64"); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.PatchMatrix(ctx, p.ID, "macos", "x86_64", MatrixWrite{PackageType: &single}); !errors.Is(err, ErrPackageTypeImmutable) {
		t.Fatalf("lock after aliased line write: %v", err)
	}
	ipad := "ipados"
	arm := "aarch64"
	if _, err := svc.CreateMatrix(ctx, p.ID, MatrixWrite{OS: &ipad, Arch: &arm, PackageType: &pkg}); err != nil {
		t.Fatal(err)
	}
	enabled, err := svc.MatrixEnabledOS(ctx, p.ID)
	if err != nil {
		t.Fatal(err)
	}
	if platform.CanonicalOS(enabled, "ipados") != platform.OSiPadOS {
		t.Fatalf("ipados should stay ipados when matrix enables it, enabled=%v", enabled)
	}
	if platform.CanonicalOS(nil, "ipados") != platform.OSiOS {
		t.Fatal("default ipados maps to ios")
	}
}

func TestHwRevOrderAndAssertKnown(t *testing.T) {
	ctx := t.Context()
	svc := NewProjectService(repository.NewMemoryProjectStore())
	slug := "hw-app"
	p, _, err := svc.Create(ctx, CreateProjectInput{DefaultLocale: ptr("en"), Slug: &slug})
	if err != nil {
		t.Fatal(err)
	}
	high, low := "rev-b", "rev-a"
	rHigh, rLow := 20, 10
	if _, err := svc.CreateHwRev(ctx, p.ID, HwRevWrite{Slug: &high, Rank: &rHigh}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.CreateHwRev(ctx, p.ID, HwRevWrite{Slug: &low, Rank: &rLow}); err != nil {
		t.Fatal(err)
	}
	list, err := svc.ListHwRevs(ctx, p.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 || list[0].Slug != "rev-a" || list[1].Slug != "rev-b" {
		t.Fatalf("order=%v", list)
	}
	if err := svc.AssertHwRevKnown(ctx, p.ID, "rev-a"); err != nil {
		t.Fatal(err)
	}
	if err := svc.AssertHwRevKnown(ctx, p.ID, ""); err != nil {
		t.Fatal(err)
	}
	if err := svc.AssertHwRevKnown(ctx, p.ID, "missing"); !errors.Is(err, ErrHwRevUnknown) {
		t.Fatalf("unknown: %v", err)
	}
}

func TestHwRevSlugV1EmptyNotes(t *testing.T) {
	ctx := t.Context()
	svc := NewProjectService(repository.NewMemoryProjectStore())
	slug := "hw-v1"
	p, _, err := svc.Create(ctx, CreateProjectInput{DefaultLocale: ptr("en"), Slug: &slug})
	if err != nil {
		t.Fatal(err)
	}
	code := "v1"
	rank := 1
	notes := ""
	hw, err := svc.CreateHwRev(ctx, p.ID, HwRevWrite{Slug: &code, Rank: &rank, Notes: &notes})
	if err != nil {
		t.Fatal(err)
	}
	if hw.Slug != "v1" || hw.Rank != 1 || hw.Notes != "" {
		t.Fatalf("hw=%+v", hw)
	}
	upper := "V1"
	if _, err := svc.CreateHwRev(ctx, p.ID, HwRevWrite{Slug: &upper, Rank: &rank}); !errors.Is(err, ErrSlugTaken) {
		t.Fatalf("duplicate after normalize: %v", err)
	}
	list, err := svc.ListHwRevs(ctx, p.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].Slug != "v1" {
		t.Fatalf("list=%v", list)
	}
}

func TestChannelNameUnlistedAndToken(t *testing.T) {
	ctx := t.Context()
	svc := NewProjectService(repository.NewMemoryProjectStore())
	zh := "chan-zh"
	p, _, err := svc.Create(ctx, CreateProjectInput{
		DefaultLocale: ptr("en"),
		Slug:          &zh,
		SystemChannelNames: model.SystemChannelNames{Alpha: "内测版", Beta: "公测版", Stable: "正式版"},
	})
	if err != nil {
		t.Fatal(err)
	}
	list, err := svc.ListChannels(ctx, p.ID)
	if err != nil {
		t.Fatal(err)
	}
	bySlug := map[string]model.Channel{}
	for _, ch := range list {
		bySlug[ch.Slug] = ch
	}
	if bySlug[model.ChannelAlpha].Name != "内测版" || bySlug[model.ChannelStable].Name != "正式版" {
		t.Fatalf("localized names=%v", bySlug)
	}

	name := "Night"
	unlisted := true
	token := "plain-token"
	ch, err := svc.CreateChannel(ctx, p.ID, ChannelWrite{
		Name: &name, Slug: ptr("night"), Unlisted: &unlisted, Token: &token,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !ch.Unlisted || !ch.TokenProtected() || ch.TokenHash == token || ch.TokenPlain != token {
		t.Fatalf("token must be hashed and stored plaintext, channel=%+v", ch)
	}
	if !ChannelTokenMatches(ch.TokenHash, token) {
		t.Fatal("hmac match failed")
	}
	empty := ""
	cleared, err := svc.PatchChannel(ctx, p.ID, "night", ChannelWrite{Token: &empty})
	if err != nil {
		t.Fatal(err)
	}
	if cleared.TokenProtected() || cleared.TokenPlain != "" || cleared.TokenHash != "" {
		t.Fatal("empty token must clear hash and plaintext")
	}
	keep, err := svc.PatchChannel(ctx, p.ID, "night", ChannelWrite{Unlisted: ptr(false)})
	if err != nil {
		t.Fatal(err)
	}
	if keep.Unlisted || keep.TokenHash != cleared.TokenHash {
		t.Fatalf("omit token must keep hash, got %+v", keep)
	}
}
