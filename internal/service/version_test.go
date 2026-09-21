package service

import (
	"errors"
	"sync"
	"testing"

	"github.com/Kirizu-Official/KiriVers/internal/model"
	"github.com/Kirizu-Official/KiriVers/internal/repository"
)

func TestParseVersionRef(t *testing.T) {
	tests := []struct {
		input     string
		isInteger bool
		integer   int64
		semver    string
		wantErr   bool
	}{
		{input: "10", isInteger: true, integer: 10},
		{input: "010", isInteger: true, integer: 10},
		{input: "1", isInteger: true, integer: 1},
		{input: "1.2.3", isInteger: false, semver: "1.2.3"},
		{input: "v1.2.3", isInteger: false, semver: "1.2.3"},
		{input: "V1.2.3", isInteger: false, semver: "1.2.3"},
		{input: "1.2.3+nightly", isInteger: false, semver: "1.2.3"},
		{input: "v1.2.3+20260912", isInteger: false, semver: "1.2.3"},
		{input: "1.2.3-beta.1", isInteger: false, semver: "1.2.3-beta.1"},
		{input: "1.2.3-rc.1+build.123", isInteger: false, semver: "1.2.3-rc.1"},
		{input: "1.2", wantErr: true},
		{input: "1.2.3.4", wantErr: true},
		{input: "0", wantErr: true},
		{input: "-5", wantErr: true},
		{input: "", wantErr: true},
		{input: "not-a-version", wantErr: true},
	}

	for _, tc := range tests {
		ref, err := ParseVersionRef(tc.input)
		if tc.wantErr {
			if err == nil {
				t.Fatalf("ParseVersionRef(%q) expected error, got nil", tc.input)
			}
			if !errors.Is(err, ErrEngineMismatch) {
				t.Fatalf("ParseVersionRef(%q) expected ErrEngineMismatch, got %v", tc.input, err)
			}
			continue
		}
		if err != nil {
			t.Fatalf("ParseVersionRef(%q) unexpected error: %v", tc.input, err)
		}
		if ref.IsInteger != tc.isInteger {
			t.Fatalf("ParseVersionRef(%q) isInteger=%v, want %v", tc.input, ref.IsInteger, tc.isInteger)
		}
		if tc.isInteger && ref.Integer != tc.integer {
			t.Fatalf("ParseVersionRef(%q) integer=%d, want %d", tc.input, ref.Integer, tc.integer)
		}
		if !tc.isInteger && ref.Semver != tc.semver {
			t.Fatalf("ParseVersionRef(%q) semver=%q, want %q", tc.input, ref.Semver, tc.semver)
		}
	}
}

func TestValidateChannelSuffix(t *testing.T) {
	tests := []struct {
		channel string
		semver  string
		wantErr bool
	}{
		{channel: "beta", semver: "1.2.3-beta.1", wantErr: false},
		{channel: "beta", semver: "1.2.3-beta", wantErr: false},
		{channel: "beta", semver: "1.2.3-rc.1", wantErr: true},
		{channel: "stable", semver: "1.2.3-beta.1", wantErr: true},
		{channel: "stable", semver: "1.2.3-rc.1", wantErr: true},
		{channel: "stable", semver: "1.2.3", wantErr: false},
		{channel: "beta", semver: "1.2.3", wantErr: false},
		{channel: "alpha", semver: "1.2.3-alpha.2", wantErr: false},
		{channel: "alpha", semver: "1.2.3-beta.1", wantErr: true},
	}

	for _, tc := range tests {
		err := ValidateChannelSuffix(tc.channel, tc.semver)
		if tc.wantErr && !errors.Is(err, ErrChannelSuffixMismatch) {
			t.Fatalf("ValidateChannelSuffix(%q, %q) expected ErrChannelSuffixMismatch, got %v", tc.channel, tc.semver, err)
		}
		if !tc.wantErr && err != nil {
			t.Fatalf("ValidateChannelSuffix(%q, %q) unexpected error: %v", tc.channel, tc.semver, err)
		}
	}
}

func ptr[T any](v T) *T {
	return &v
}

func TestVersionPutAndIdempotent(t *testing.T) {
	ctx := t.Context()
	store := repository.NewMemoryProjectStore()
	svc := NewProjectService(store)

	p, _, err := svc.Create(ctx, CreateProjectInput{DefaultLocale: ptr("en"), Slug: ptr("proj-semver"),
		CompareEngine: ptr(model.CompareEngineSemver),
	})
	if err != nil {
		t.Fatal(err)
	}

	// 1. Missing SemVer under semver compare engine -> ErrEngineMismatch
	_, _, err = svc.PutVersion(ctx, p.ID, "10", VersionWriteInput{Channel: "stable"})
	if !errors.Is(err, ErrEngineMismatch) {
		t.Fatalf("expected ErrEngineMismatch when putting integer version under semver engine, got %v", err)
	}

	// 2. Put 1.2.3 on stable -> 201 Created
	changelogText := "Initial release"
	v1, created, err := svc.PutVersion(ctx, p.ID, "1.2.3", VersionWriteInput{
		Channel:   "stable",
		Changelog: &changelogText,
	})
	if err != nil {
		t.Fatalf("put 1.2.3: %v", err)
	}
	if !created {
		t.Fatal("expected created=true for first PUT")
	}
	if v1.Status != model.VersionStatusDraft {
		t.Fatalf("status=%q, want %q", v1.Status, model.VersionStatusDraft)
	}
	if v1.GrayStartPercent != 30 {
		t.Fatalf("gray_start_percent=%d, want 30", v1.GrayStartPercent)
	}
	if v1.GrayStepPercent != 10 {
		t.Fatalf("gray_step_percent=%d, want 10", v1.GrayStepPercent)
	}
	if v1.VersionSemver == nil || *v1.VersionSemver != "1.2.3" {
		t.Fatalf("version_semver=%v, want 1.2.3", v1.VersionSemver)
	}

	// 3. Put 1.2.3+20260912 -> 200 OK idempotent
	v2, created, err := svc.PutVersion(ctx, p.ID, "1.2.3+20260912", VersionWriteInput{
		Channel: "stable",
	})
	if err != nil {
		t.Fatalf("idempotent put: %v", err)
	}
	if created {
		t.Fatal("expected created=false for idempotent PUT")
	}
	if v2.ID != v1.ID {
		t.Fatalf("idempotent version id=%s, want %s", v2.ID, v1.ID)
	}

	// 4. Put same SemVer on different channel -> ErrChannelConflict
	_, _, err = svc.PutVersion(ctx, p.ID, "1.2.3", VersionWriteInput{
		Channel: "beta",
	})
	if !errors.Is(err, ErrChannelConflict) {
		t.Fatalf("expected ErrChannelConflict, got %v", err)
	}

	// 5. Exists check:
	// Exists for 1.2.3 -> true
	exists, foundV, _, err := svc.CheckVersionExists(ctx, p.ID, "1.2.3")
	if err != nil || !exists || foundV.ID != v1.ID {
		t.Fatalf("exists for 1.2.3 failed: exists=%v, err=%v", exists, err)
	}
	// Exists for 1.2.3+nightly -> true (ignores +build)
	exists, foundV, _, err = svc.CheckVersionExists(ctx, p.ID, "1.2.3+nightly")
	if err != nil || !exists || foundV.ID != v1.ID {
		t.Fatalf("exists for 1.2.3+nightly failed: exists=%v, err=%v", exists, err)
	}
	// Exists for 9.9.9 -> false, err=nil
	exists, _, _, err = svc.CheckVersionExists(ctx, p.ID, "9.9.9")
	if err != nil || exists {
		t.Fatalf("exists for 9.9.9 expected false, got exists=%v, err=%v", exists, err)
	}
	// Exists for invalid string -> false, err=nil
	exists, _, _, err = svc.CheckVersionExists(ctx, p.ID, "invalid-semver")
	if err != nil || exists {
		t.Fatalf("exists for invalid expected false, got exists=%v, err=%v", exists, err)
	}
}

func TestIntegerEngineAutoIncrement(t *testing.T) {
	ctx := t.Context()
	store := repository.NewMemoryProjectStore()
	svc := NewProjectService(store)

	p, _, err := svc.Create(ctx, CreateProjectInput{DefaultLocale: ptr("en"), Slug: ptr("proj-integer"),
		CompareEngine: ptr(model.CompareEngineInteger),
	})
	if err != nil {
		t.Fatal(err)
	}

	// 1. First version: path has semver "1.0.0", integer omitted -> auto-increment to 1
	v1, created, err := svc.PutVersion(ctx, p.ID, "1.0.0", VersionWriteInput{Channel: "stable"})
	if err != nil {
		t.Fatalf("put v1: %v", err)
	}
	if !created {
		t.Fatal("expected created=true")
	}
	if v1.VersionInteger == nil || *v1.VersionInteger != 1 {
		t.Fatalf("version_integer=%v, want 1", v1.VersionInteger)
	}

	// 2. Second version: path has semver "1.1.0", integer omitted -> auto-increment to 2
	v2, created, err := svc.PutVersion(ctx, p.ID, "1.1.0", VersionWriteInput{Channel: "stable"})
	if err != nil {
		t.Fatalf("put v2: %v", err)
	}
	if !created {
		t.Fatal("expected created=true")
	}
	if v2.VersionInteger == nil || *v2.VersionInteger != 2 {
		t.Fatalf("version_integer=%v, want 2", v2.VersionInteger)
	}

	// 3. Querying 002 vs 2 hit the same version
	exists, foundV, _, err := svc.CheckVersionExists(ctx, p.ID, "002")
	if err != nil || !exists || foundV.ID != v2.ID {
		t.Fatalf("002 lookup failed: exists=%v, err=%v", exists, err)
	}
	exists, foundV, _, err = svc.CheckVersionExists(ctx, p.ID, "2")
	if err != nil || !exists || foundV.ID != v2.ID {
		t.Fatalf("2 lookup failed: exists=%v, err=%v", exists, err)
	}
}

func TestCriticalAndIntermediateRules(t *testing.T) {
	ctx := t.Context()
	store := repository.NewMemoryProjectStore()
	svc := NewProjectService(store)

	p, _, err := svc.Create(ctx, CreateProjectInput{DefaultLocale: ptr("en"), Slug: ptr("proj-rules"),
		CompareEngine: ptr(model.CompareEngineSemver),
	})
	if err != nil {
		t.Fatal(err)
	}

	// Critical with rollout != 100 -> ErrGrayNotAllowedOnCritical
	rollout50 := 50
	_, _, err = svc.PutVersion(ctx, p.ID, "1.0.0", VersionWriteInput{
		Channel:          "stable",
		IsCritical:       ptr(true),
		GrayStartPercent: &rollout50,
	})
	if !errors.Is(err, ErrGrayNotAllowedOnCritical) {
		t.Fatalf("expected ErrGrayNotAllowedOnCritical, got %v", err)
	}
}

func TestVersionLifecycleAndLineManagement(t *testing.T) {
	ctx := t.Context()
	store := repository.NewMemoryProjectStore()
	svc := NewProjectService(store)

	p, _, err := svc.Create(ctx, CreateProjectInput{DefaultLocale: ptr("en"), Slug: ptr("lifecycle-proj"),
		CompareEngine: ptr(model.CompareEngineSemver),
	})
	if err != nil {
		t.Fatal(err)
	}

	// Set matrix row for macos/arm64
	_, err = svc.CreateMatrix(ctx, p.ID, MatrixWrite{
		OS:          ptr("macos"),
		Arch:        ptr("arm64"),
		PackageType: ptr(model.PackageTypeSingleFile),
	})
	if err != nil {
		t.Fatal(err)
	}

	// 1. Create Draft 1.0.0
	_, created, err := svc.PutVersion(ctx, p.ID, "1.0.0", VersionWriteInput{Channel: "stable"})
	if err != nil || !created {
		t.Fatalf("create 1.0.0: %v", err)
	}

	// 2. Publish with 0 ready lines -> ErrArtifactRequired
	_, err = svc.PublishVersion(ctx, p.ID, "1.0.0")
	if !errors.Is(err, ErrArtifactRequired) {
		t.Fatalf("expected ErrArtifactRequired, got %v", err)
	}

	// 3. Add line for darwin/arm64 (should canonicalize to macos/arm64; no matrix min_os inherit)
	line, err := svc.AddVersionLine(ctx, p.ID, "1.0.0", VersionLineWriteInput{
		OS:   "darwin",
		Arch: "arm64",
	})
	if err != nil {
		t.Fatalf("add line: %v", err)
	}
	if line.OS != "macos" || line.Arch != "arm64" {
		t.Fatalf("canonical line os=%s, arch=%s, want macos arm64", line.OS, line.Arch)
	}
	if line.MinOS != nil {
		t.Fatalf("min_os=%v, want nil (no matrix inherit)", line.MinOS)
	}
	if line.Status != model.VersionLineStatusPending {
		t.Fatalf("line status=%s, want pending", line.Status)
	}

	// Still cannot publish since line is pending
	_, err = svc.PublishVersion(ctx, p.ID, "1.0.0")
	if !errors.Is(err, ErrArtifactRequired) {
		t.Fatalf("expected ErrArtifactRequired, got %v", err)
	}

	// 4. Mark line ready fixture
	readyLine, err := svc.ReadyVersionLine(ctx, p.ID, "1.0.0", "macos", "arm64")
	if err != nil {
		t.Fatalf("ready line: %v", err)
	}
	if readyLine.Status != model.VersionLineStatusReady {
		t.Fatalf("line status=%s, want ready", readyLine.Status)
	}

	// 5. Now publish succeeds!
	pubV, err := svc.PublishVersion(ctx, p.ID, "1.0.0")
	if err != nil {
		t.Fatalf("publish: %v", err)
	}
	if pubV.Status != model.VersionStatusPublished {
		t.Fatalf("status=%s, want published", pubV.Status)
	}

	// Published version cannot be deleted
	err = svc.DeleteVersion(ctx, p.ID, "1.0.0")
	if !errors.Is(err, ErrInvalidVersionTransition) {
		t.Fatalf("expected ErrInvalidVersionTransition when deleting published version, got %v", err)
	}

	// Published version cannot modify version number or changelog
	newNum := int64(99)
	_, err = svc.PatchVersion(ctx, p.ID, "1.0.0", VersionPatchInput{VersionInteger: &newNum})
	if !errors.Is(err, ErrPublishedVersionImmutable) {
		t.Fatalf("expected ErrPublishedVersionImmutable, got %v", err)
	}
	badLog := "Hacked"
	patchedLog, err := svc.PatchVersion(ctx, p.ID, "1.0.0", VersionPatchInput{Changelog: &badLog})
	if err != nil {
		t.Fatalf("published changelog patch: %v", err)
	}
	if patchedLog.Changelog["en"].Markdown != badLog && patchedLog.Changelog[p.DefaultLocale].Markdown != badLog {
		t.Fatalf("changelog not updated: %+v", patchedLog.Changelog)
	}

	// Published version CAN patch is_lts
	patchedV, err := svc.PatchVersion(ctx, p.ID, "1.0.0", VersionPatchInput{IsLTS: ptr(true)})
	if err != nil {
		t.Fatalf("patch is_lts: %v", err)
	}
	if !patchedV.IsLTS {
		t.Fatal("is_lts should be true")
	}

	// 6. Adding another line after publish is allowed (no new version number required)
	lineLinux, err := svc.AddVersionLine(ctx, p.ID, "1.0.0", VersionLineWriteInput{
		OS:   "linux",
		Arch: "x86_64",
	})
	if err != nil {
		t.Fatalf("add linux line after publish: %v", err)
	}
	if lineLinux.OS != "linux" {
		t.Fatalf("linux line os=%s, want linux", lineLinux.OS)
	}

	// 7. Yank macos/arm64 line
	yankedLine, err := svc.YankVersionLine(ctx, p.ID, "1.0.0", "macos", "arm64")
	if err != nil {
		t.Fatalf("yank line: %v", err)
	}
	if yankedLine.Status != model.VersionLineStatusYanked {
		t.Fatalf("status=%s, want yanked", yankedLine.Status)
	}
	// Version status itself remains published (C04-15)
	currentV, err := svc.ResolveVersion(ctx, p.ID, "1.0.0")
	if err != nil || currentV.Status != model.VersionStatusPublished {
		t.Fatalf("version status should remain published, got %s", currentV.Status)
	}

	// 8. State transitions: Published -> Deprecated -> Revoked
	depV, err := svc.DeprecateVersion(ctx, p.ID, "1.0.0")
	if err != nil || depV.Status != model.VersionStatusDeprecated {
		t.Fatalf("deprecate failed: %v", err)
	}
	revV, err := svc.RevokeVersion(ctx, p.ID, "1.0.0")
	if err != nil || revV.Status != model.VersionStatusRevoked {
		t.Fatalf("revoke failed: %v", err)
	}
	// Revoked cannot be re-published
	_, err = svc.PublishVersion(ctx, p.ID, "1.0.0")
	if !errors.Is(err, ErrInvalidVersionTransition) {
		t.Fatalf("expected ErrInvalidVersionTransition when publishing revoked, got %v", err)
	}
}

func TestConcurrentPutSameVersion(t *testing.T) {
	ctx := t.Context()
	store := repository.NewMemoryProjectStore()
	svc := NewProjectService(store)

	p, _, err := svc.Create(ctx, CreateProjectInput{DefaultLocale: ptr("en"), Slug: ptr("proj-concurrent"),
		CompareEngine: ptr(model.CompareEngineSemver),
	})
	if err != nil {
		t.Fatal(err)
	}

	var wg sync.WaitGroup
	results := make([]bool, 2)
	errs := make([]error, 2)

	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			_, created, err := svc.PutVersion(ctx, p.ID, "1.0.0", VersionWriteInput{
				Channel: "stable",
			})
			results[idx] = created
			errs[idx] = err
		}(i)
	}
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Fatalf("goroutine %d failed: %v", i, err)
		}
	}

	// Exactly one must be created=true (201) and one created=false (200)
	if !((results[0] && !results[1]) || (!results[0] && results[1])) {
		t.Fatalf("expected one 201 and one 200, got %v and %v", results[0], results[1])
	}
}

func TestLineDefaultsPrefillsFromHighestBelow(t *testing.T) {
	ctx := t.Context()
	store := repository.NewMemoryProjectStore()
	svc := NewProjectService(store)
	p, _, err := svc.Create(ctx, CreateProjectInput{
		DefaultLocale: ptr("en"),
		Slug:          ptr("line-defaults-proj"),
		CompareEngine: ptr(model.CompareEngineSemver),
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.CreateMatrix(ctx, p.ID, MatrixWrite{
		OS: ptr("windows"), Arch: ptr("x86_64"), PackageType: ptr(model.PackageTypeSingleFile),
	}); err != nil {
		t.Fatal(err)
	}
	for _, ver := range []string{"1.0.0", "1.5.0", "2.0.0"} {
		if _, _, err := svc.PutVersion(ctx, p.ID, ver, VersionWriteInput{Channel: "stable"}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := svc.AddVersionLine(ctx, p.ID, "1.0.0", VersionLineWriteInput{
		OS: "windows", Arch: "x86_64", MinOS: ptr("10.0"), MinAPILevel: ptr(21),
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.AddVersionLine(ctx, p.ID, "1.5.0", VersionLineWriteInput{
		OS: "windows", Arch: "x86_64", MinOS: ptr("11.0"), MinAPILevel: ptr(26),
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.AddVersionLine(ctx, p.ID, "2.0.0", VersionLineWriteInput{
		OS: "windows", Arch: "x86_64",
	}); err != nil {
		t.Fatal(err)
	}

	minOS, minAPI, err := svc.LineDefaults(ctx, p, "2.0.0", "windows", "x86_64")
	if err != nil {
		t.Fatal(err)
	}
	if minOS == nil || *minOS != "11.0" {
		t.Fatalf("min_os=%v, want 11.0 from 1.5.0", minOS)
	}
	if minAPI == nil || *minAPI != 26 {
		t.Fatalf("min_api_level=%v, want 26 from 1.5.0", minAPI)
	}

	emptyOS, emptyAPI, err := svc.LineDefaults(ctx, p, "1.0.0", "windows", "x86_64")
	if err != nil {
		t.Fatal(err)
	}
	if emptyOS != nil || emptyAPI != nil {
		t.Fatalf("lowest version should have null defaults, got %v %v", emptyOS, emptyAPI)
	}
}
