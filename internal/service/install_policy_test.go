package service

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"testing"

	"github.com/google/uuid"

	"github.com/Kirizu-Official/KiriVers/internal/model"
	"github.com/Kirizu-Official/KiriVers/internal/repository"
	"github.com/Kirizu-Official/KiriVers/pkg/hashutil"
)

func setupInstallPolicyService(t *testing.T) (*ProjectService, *repository.MemoryProjectStore, *repository.MemoryInstallPolicyRuleStore) {
	t.Helper()
	svc, store, _ := setupTestService(t)
	rules := repository.NewMemoryInstallPolicyRuleStore()
	svc.SetInstallPolicyStore(rules)
	return svc, store.(*repository.MemoryProjectStore), rules
}

func mustCreateMatrix(t *testing.T, svc *ProjectService, projectID uuid.UUID, os, arch string) {
	t.Helper()
	osv, archv := os, arch
	pkg := model.PackageTypeMultiFile
	if _, err := svc.CreateMatrix(context.Background(), projectID, MatrixWrite{
		OS: &osv, Arch: &archv, PackageType: &pkg,
	}); err != nil {
		t.Fatal(err)
	}
}

func TestEffectiveInstallPolicyOverlay(t *testing.T) {
	svc, _, _ := setupInstallPolicyService(t)
	ctx := context.Background()
	slug := "policy-overlay"
	p, _, err := svc.Create(ctx, CreateProjectInput{DefaultLocale: ptr("en"), Slug: &slug})
	if err != nil {
		t.Fatal(err)
	}
	mustCreateMatrix(t, svc, p.ID, "windows", "x86_64")

	if _, err := svc.PutProjectInstallPolicy(ctx, p.ID, "windows", "x86_64", []InstallPolicyEntry{
		{Path: "config/user.json", InstallPolicy: model.InstallPolicyKeepIfExists},
		{Path: "data/cache.bin", InstallPolicy: model.InstallPolicyKeepIfExists},
	}); err != nil {
		t.Fatal(err)
	}
	ch, err := svc.store.GetChannel(ctx, p.ID, model.ChannelStable)
	if err != nil {
		t.Fatal(err)
	}
	eff, err := svc.EffectiveInstallPolicy(ctx, p.ID, ch.ID, "windows", "x86_64")
	if err != nil {
		t.Fatal(err)
	}
	if eff["config/user.json"] != model.InstallPolicyKeepIfExists || eff["data/cache.bin"] != model.InstallPolicyKeepIfExists {
		t.Fatalf("empty channel must inherit project: %+v", eff)
	}

	if _, _, err := svc.PutChannelInstallPolicy(ctx, p.ID, model.ChannelBeta, "windows", "x86_64", []InstallPolicyEntry{
		{Path: "config/user.json", InstallPolicy: model.InstallPolicyOverwrite},
		{Path: "debug.log", InstallPolicy: model.InstallPolicyKeepIfExists},
	}); err != nil {
		t.Fatal(err)
	}
	beta, err := svc.store.GetChannel(ctx, p.ID, model.ChannelBeta)
	if err != nil {
		t.Fatal(err)
	}
	eff, err = svc.EffectiveInstallPolicy(ctx, p.ID, beta.ID, "windows", "x86_64")
	if err != nil {
		t.Fatal(err)
	}
	if eff["config/user.json"] != model.InstallPolicyOverwrite {
		t.Fatalf("channel OVERWRITE must undo project KEEP: %+v", eff)
	}
	if eff["debug.log"] != model.InstallPolicyKeepIfExists {
		t.Fatalf("channel extra KEEP missing: %+v", eff)
	}
	if eff["data/cache.bin"] != model.InstallPolicyKeepIfExists {
		t.Fatalf("untouched project KEEP must remain: %+v", eff)
	}
}

func TestLatestChannelPlatformManifest(t *testing.T) {
	svc, _, _ := setupInstallPolicyService(t)
	ctx := context.Background()
	slug := "policy-ref"
	engine := model.CompareEngineSemver
	p, _, err := svc.Create(ctx, CreateProjectInput{DefaultLocale: ptr("en"), Slug: &slug, CompareEngine: &engine})
	if err != nil {
		t.Fatal(err)
	}
	mustCreateMatrix(t, svc, p.ID, "linux", "x86_64")

	empty, err := svc.LatestChannelPlatformManifest(ctx, p.ID, model.ChannelStable, "linux", "x86_64")
	if err != nil {
		t.Fatal(err)
	}
	if empty.Version != nil || len(empty.Entries) != 0 {
		t.Fatalf("no versions: %+v", empty)
	}

	if _, _, err := svc.PutVersion(ctx, p.ID, "1.0.0", VersionWriteInput{Channel: model.ChannelStable}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.AddVersionLine(ctx, p.ID, "1.0.0", VersionLineWriteInput{OS: "linux", Arch: "x86_64"}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := svc.BuildArchiveFromFiles(ctx, p.Slug, "1.0.0", "linux", "x86_64", map[string][]byte{
		"bin/app": []byte("v1"),
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.PublishVersion(ctx, p.ID, "1.0.0"); err != nil {
		t.Fatal(err)
	}
	sha := "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"
	md5 := "d41d8cd98f00b204e9800998ecf8427e"

	if _, _, err := svc.PutVersion(ctx, p.ID, "2.0.0", VersionWriteInput{Channel: model.ChannelStable}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.AddVersionLine(ctx, p.ID, "2.0.0", VersionLineWriteInput{OS: "linux", Arch: "x86_64"}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.SetManifest(ctx, p.Slug, "2.0.0", "linux", "x86_64", []ManifestEntryInput{
		{Path: "bin/app", Size: 2, SHA256: sha, MD5: md5},
	}); err != nil {
		t.Fatal(err)
	}

	ref, err := svc.LatestChannelPlatformManifest(ctx, p.ID, model.ChannelStable, "linux", "x86_64")
	if err != nil {
		t.Fatal(err)
	}
	if ref.Version == nil || *ref.Version != "2.0.0" {
		t.Fatalf("draft must beat published: %+v", ref)
	}
	if len(ref.Entries) != 1 || ref.Entries[0].SHA256 != sha || ref.Entries[0].MD5 != md5 {
		t.Fatalf("reference must keep hashes: %+v", ref.Entries)
	}

	if _, _, err := svc.PutVersion(ctx, p.ID, "3.0.0", VersionWriteInput{Channel: model.ChannelStable}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.RevokeVersion(ctx, p.ID, "3.0.0"); err != nil {
		t.Fatal(err)
	}
	ref, err = svc.LatestChannelPlatformManifest(ctx, p.ID, model.ChannelStable, "linux", "x86_64")
	if err != nil {
		t.Fatal(err)
	}
	if ref.Version == nil || *ref.Version != "2.0.0" {
		t.Fatalf("revoked must not be reference: %+v", ref)
	}

	if _, _, err := svc.PutVersion(ctx, p.ID, "4.0.0", VersionWriteInput{Channel: model.ChannelStable}); err != nil {
		t.Fatal(err)
	}
	ref, err = svc.LatestChannelPlatformManifest(ctx, p.ID, model.ChannelStable, "linux", "x86_64")
	if err != nil {
		t.Fatal(err)
	}
	if ref.Version == nil || *ref.Version != "4.0.0" || len(ref.Entries) != 0 {
		t.Fatalf("latest without line must return version + empty entries: %+v", ref)
	}
}

func TestInstallPolicyPutValidatesMatrixPathCap(t *testing.T) {
	svc, _, _ := setupInstallPolicyService(t)
	ctx := context.Background()
	slug := "policy-validate"
	p, _, err := svc.Create(ctx, CreateProjectInput{DefaultLocale: ptr("en"), Slug: &slug})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.PutProjectInstallPolicy(ctx, p.ID, "windows", "x86_64", nil); !IsInvalidRequest(err) {
		t.Fatalf("missing matrix: %v", err)
	}
	mustCreateMatrix(t, svc, p.ID, "windows", "x86_64")
	if _, err := svc.PutProjectInstallPolicy(ctx, p.ID, "windows", "x86_64", []InstallPolicyEntry{
		{Path: "../etc/passwd", InstallPolicy: model.InstallPolicyKeepIfExists},
	}); err == nil {
		t.Fatal("illegal path must fail")
	}
	if _, _, err := svc.ListChannelInstallPolicy(ctx, p.ID, "no-such", "windows", "x86_64"); !errors.Is(err, ErrChannelNotFound) {
		t.Fatalf("unknown channel: %v", err)
	}
	tooMany := make([]InstallPolicyEntry, model.MaxInstallPolicyRules+1)
	for i := range tooMany {
		tooMany[i] = InstallPolicyEntry{Path: fmt.Sprintf("p/%d.txt", i), InstallPolicy: model.InstallPolicyOverwrite}
	}
	if _, err := svc.PutProjectInstallPolicy(ctx, p.ID, "windows", "x86_64", tooMany); !IsInvalidRequest(err) {
		t.Fatalf("cap: %v", err)
	}
}

func TestParseZipIgnoresKeepSidecar(t *testing.T) {
	buf := new(bytes.Buffer)
	zw := zip.NewWriter(buf)
	w, err := zw.Create("bin/app")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = w.Write([]byte("exe"))
	w, err = zw.Create("keep_if_exists.txt")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = w.Write([]byte("bin/app\n"))
	w, err = zw.Create("_keep.json")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = w.Write([]byte(`["bin/app"]`))
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	entries, err := ParseZipEntries(bytes.NewReader(buf.Bytes()), int64(buf.Len()))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].NormalizedPath != "bin/app" {
		t.Fatalf("entries=%+v", entries)
	}
	if entries[0].InstallPolicy != model.InstallPolicyOverwrite {
		t.Fatalf("sidecar must not stamp KEEP: %s", entries[0].InstallPolicy)
	}
}

func TestMediaPutWritesHashes(t *testing.T) {
	ctx := context.Background()
	svc := NewMediaService(repository.NewMemoryProjectMediaStore(), newMemBackend(), nil)
	payload := []byte("fixture-bytes-for-hash")
	res, err := svc.Put(ctx, testProject(), MediaPutInput{
		FileName: "notes.txt", ContentType: "text/plain", Body: bytes.NewReader(payload),
	})
	if err != nil {
		t.Fatal(err)
	}
	h := hashutil.NewMultiHasher(true)
	_, _ = h.Write(payload)
	if res.Media.SHA256 != h.SHA256() || res.Media.MD5 != h.MD5() || res.Media.SHA512 != h.SHA512() {
		t.Fatalf("hashes sha256=%s md5=%s sha512=%s", res.Media.SHA256, res.Media.MD5, res.Media.SHA512)
	}
}

func TestNewManifestStampsEffectiveAndStripsSidecar(t *testing.T) {
	svc, _, backend := setupTestService(t)
	ctx := context.Background()
	slug := "policy-stamp"
	p, _, err := svc.Create(ctx, CreateProjectInput{DefaultLocale: ptr("en"), Slug: &slug})
	if err != nil {
		t.Fatal(err)
	}
	mustCreateMatrix(t, svc, p.ID, "windows", "x86_64")
	if _, err := svc.PutProjectInstallPolicy(ctx, p.ID, "windows", "x86_64", []InstallPolicyEntry{
		{Path: "config/user.json", InstallPolicy: model.InstallPolicyKeepIfExists},
	}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := svc.PutVersion(ctx, p.ID, "1.0.0", VersionWriteInput{Channel: model.ChannelStable}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.AddVersionLine(ctx, p.ID, "1.0.0", VersionLineWriteInput{OS: "windows", Arch: "x86_64"}); err != nil {
		t.Fatal(err)
	}

	buf := new(bytes.Buffer)
	zw := zip.NewWriter(buf)
	w, err := zw.Create("bin/app.exe")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = w.Write([]byte("exe"))
	w, err = zw.Create("config/user.json")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = w.Write([]byte(`{}`))
	w, err = zw.Create("keep_if_exists.txt")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = w.Write([]byte("bin/app.exe\n"))
	w, err = zw.Create("_keep.json")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = w.Write([]byte(`["bin/app.exe"]`))
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}

	_, mres, err := svc.BuildArchiveFromZipBuffer(ctx, p.Slug, "1.0.0", "windows", "x86_64", buf.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]string{}
	for _, e := range mres.Entries {
		got[e.Path] = e.InstallPolicy
		if e.Path == "keep_if_exists.txt" || e.Path == "_keep.json" {
			t.Fatalf("sidecar in manifest: %s", e.Path)
		}
		if e.SHA256 == "" || e.MD5 == "" {
			t.Fatalf("missing hashes for %s", e.Path)
		}
	}
	if got["config/user.json"] != model.InstallPolicyKeepIfExists {
		t.Fatalf("template KEEP missing: %+v", got)
	}
	if got["bin/app.exe"] != model.InstallPolicyOverwrite {
		t.Fatalf("unlisted path must OVERWRITE: %+v", got)
	}

	line, err := svc.GetVersionLine(ctx, p.ID, "1.0.0", "windows", "x86_64")
	if err != nil {
		t.Fatal(err)
	}
	feeds, err := svc.store.ListArtifactsByLineAndKind(ctx, line.ID, model.ArtifactKindStoreFull)
	if err != nil || len(feeds) == 0 {
		t.Fatalf("store_full: %v n=%d", err, len(feeds))
	}
	rc, err := backend.Get(ctx, feeds[0].StorageKey)
	if err != nil {
		t.Fatal(err)
	}
	zipBytes, err := io.ReadAll(rc)
	_ = rc.Close()
	if err != nil {
		t.Fatal(err)
	}
	zr, err := zip.NewReader(bytes.NewReader(zipBytes), int64(len(zipBytes)))
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range zr.File {
		if isKeepSidecarPath(f.Name) {
			t.Fatalf("generated archive still has sidecar %s", f.Name)
		}
	}
}

func TestExistingManifestZipDoesNotRestampPolicy(t *testing.T) {
	svc, _, _ := setupInstallPolicyService(t)
	ctx := context.Background()
	slug := "policy-norestamp"
	p, _, err := svc.Create(ctx, CreateProjectInput{DefaultLocale: ptr("en"), Slug: &slug})
	if err != nil {
		t.Fatal(err)
	}
	mustCreateMatrix(t, svc, p.ID, "windows", "x86_64")
	if _, err := svc.PutProjectInstallPolicy(ctx, p.ID, "windows", "x86_64", []InstallPolicyEntry{
		{Path: "config/user.json", InstallPolicy: model.InstallPolicyKeepIfExists},
	}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := svc.PutVersion(ctx, p.ID, "1.0.0", VersionWriteInput{Channel: model.ChannelStable}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.AddVersionLine(ctx, p.ID, "1.0.0", VersionLineWriteInput{OS: "windows", Arch: "x86_64"}); err != nil {
		t.Fatal(err)
	}
	files := map[string][]byte{
		"bin/app.exe":      []byte("exe"),
		"config/user.json": []byte(`{}`),
	}
	_, mres, err := svc.BuildArchiveFromFiles(ctx, p.Slug, "1.0.0", "windows", "x86_64", files)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]string{}
	for _, e := range mres.Entries {
		got[e.Path] = e.InstallPolicy
	}
	if got["config/user.json"] != model.InstallPolicyKeepIfExists {
		t.Fatalf("first stamp: %+v", got)
	}
	if _, err := svc.PutProjectInstallPolicy(ctx, p.ID, "windows", "x86_64", []InstallPolicyEntry{
		{Path: "config/user.json", InstallPolicy: model.InstallPolicyOverwrite},
	}); err != nil {
		t.Fatal(err)
	}
	_, mres, err = svc.BuildArchiveFromFiles(ctx, p.Slug, "1.0.0", "windows", "x86_64", files)
	if err != nil {
		t.Fatal(err)
	}
	got = map[string]string{}
	for _, e := range mres.Entries {
		got[e.Path] = e.InstallPolicy
	}
	if got["config/user.json"] != model.InstallPolicyKeepIfExists {
		t.Fatalf("existing Manifest zip must not restamp: %+v", got)
	}
}

func TestSetManifestOmitUsesEffectiveAndExplicitKept(t *testing.T) {
	svc, _, _ := setupInstallPolicyService(t)
	ctx := context.Background()
	slug := "policy-omit"
	p, _, err := svc.Create(ctx, CreateProjectInput{DefaultLocale: ptr("en"), Slug: &slug})
	if err != nil {
		t.Fatal(err)
	}
	mustCreateMatrix(t, svc, p.ID, "windows", "x86_64")
	if _, err := svc.PutProjectInstallPolicy(ctx, p.ID, "windows", "x86_64", []InstallPolicyEntry{
		{Path: "config/user.json", InstallPolicy: model.InstallPolicyKeepIfExists},
	}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := svc.PutVersion(ctx, p.ID, "1.0.0", VersionWriteInput{Channel: model.ChannelStable}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.AddVersionLine(ctx, p.ID, "1.0.0", VersionLineWriteInput{OS: "windows", Arch: "x86_64"}); err != nil {
		t.Fatal(err)
	}
	sha := "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"
	md5 := "d41d8cd98f00b204e9800998ecf8427e"
	res, err := svc.SetManifest(ctx, p.Slug, "1.0.0", "windows", "x86_64", []ManifestEntryInput{
		{Path: "config/user.json", Size: 0, SHA256: sha, MD5: md5},
		{Path: "bin/app.exe", Size: 0, SHA256: sha, MD5: md5, InstallPolicy: model.InstallPolicyOverwrite},
	})
	if err != nil {
		t.Fatal(err)
	}
	byPath := map[string]model.ManifestEntry{}
	for _, e := range res.Entries {
		byPath[e.Path] = e
	}
	if byPath["config/user.json"].InstallPolicy != model.InstallPolicyKeepIfExists || byPath["config/user.json"].IntegrityCheck {
		t.Fatalf("omit must use template: %+v", byPath["config/user.json"])
	}
	if byPath["bin/app.exe"].InstallPolicy != model.InstallPolicyOverwrite {
		t.Fatalf("explicit OVERWRITE: %+v", byPath["bin/app.exe"])
	}

	if _, err := svc.SetManifest(ctx, p.Slug, "1.0.0", "windows", "x86_64", []ManifestEntryInput{
		{Path: "config/user.json", Size: 0, SHA256: sha, MD5: md5, InstallPolicy: model.InstallPolicyOverwrite},
	}); err != nil {
		t.Fatal(err)
	}
	got, err := svc.GetManifest(ctx, p.Slug, "1.0.0", "windows", "x86_64")
	if err != nil {
		t.Fatal(err)
	}
	if got.Entries[0].InstallPolicy != model.InstallPolicyOverwrite {
		t.Fatalf("explicit opposite policy: %+v", got.Entries[0])
	}
	rules, err := svc.ListProjectInstallPolicy(ctx, p.ID, "windows", "x86_64")
	if err != nil {
		t.Fatal(err)
	}
	if len(rules) != 1 || rules[0].InstallPolicy != model.InstallPolicyKeepIfExists {
		t.Fatalf("one-off must not write back: %+v", rules)
	}
}
