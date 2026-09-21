package service

import (
	"context"
	"testing"

	"github.com/Kirizu-Official/KiriVers/internal/model"
)

func TestLoginClientUpsertAndCheckDoesNotClobberJSON(t *testing.T) {
	svc, _, _ := setupTestService(t)
	ctx := context.Background()
	slug := "clients-json"
	p, _, err := svc.Create(ctx, CreateProjectInput{DefaultLocale: ptr("en"), Slug: &slug})
	if err != nil {
		t.Fatal(err)
	}
	first, err := svc.LoginClient(ctx, p, ClientLoginInput{
		DeviceID: "phone-1", Version: "1.0.0", OS: "android", Arch: "arm64",
		Custom: model.JSONObject{"role": "qa", "build": "nightly"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if first.Custom["role"] != "qa" {
		t.Fatalf("login custom = %+v", first.Custom)
	}
	svc.TouchClientCheck(ctx, p, "phone-1", "1.1.0", "windows", "x86_64", "beta", "10.0.0.1")
	got, err := svc.GetClient(ctx, p.ID, first.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Custom["role"] != "qa" || got.Custom["build"] != "nightly" {
		t.Fatalf("check must not clobber JSON, got %+v", got.Custom)
	}
	if got.LastVersion != "1.1.0" || got.LastOS != "windows" || got.LastIP != "10.0.0.1" {
		t.Fatalf("check must update ops fields: %+v", got)
	}
}

func TestLoginClientNonePolicyDoesNotInsert(t *testing.T) {
	svc, store, _ := setupTestService(t)
	ctx := context.Background()
	slug := "clients-none"
	policy := model.DeviceIDPolicyNone
	p, _, err := svc.Create(ctx, CreateProjectInput{DefaultLocale: ptr("en"), Slug: &slug, DeviceIDPolicy: &policy})
	if err != nil {
		t.Fatal(err)
	}
	_, err = svc.LoginClient(ctx, p, ClientLoginInput{DeviceID: "x", Version: "1.0.0"})
	if err == nil {
		t.Fatal("none policy must reject login")
	}
	svc.TouchClientCheck(ctx, p, "x", "1.0.0", "windows", "x86_64", "", "")
	n, err := store.CountClients(ctx, p.ID)
	if err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatalf("none/anonymous must not insert, n=%d", n)
	}
}

func TestLoginClientUniquePerProjectHash(t *testing.T) {
	svc, _, _ := setupTestService(t)
	ctx := context.Background()
	slug := "clients-uniq"
	p, _, err := svc.Create(ctx, CreateProjectInput{DefaultLocale: ptr("en"), Slug: &slug})
	if err != nil {
		t.Fatal(err)
	}
	a, err := svc.LoginClient(ctx, p, ClientLoginInput{DeviceID: "same", Version: "1.0.0", OS: "linux", Arch: "x86_64"})
	if err != nil {
		t.Fatal(err)
	}
	b, err := svc.LoginClient(ctx, p, ClientLoginInput{DeviceID: "same", Version: "1.2.0", Custom: model.JSONObject{"k": "v"}})
	if err != nil {
		t.Fatal(err)
	}
	if a.ID != b.ID {
		t.Fatalf("same device must upsert one row: %s vs %s", a.ID, b.ID)
	}
	if b.LastVersion != "1.2.0" || b.Custom["k"] != "v" {
		t.Fatalf("second login upsert failed: %+v", b)
	}
}

func TestClientUpdatedToCompareKey(t *testing.T) {
	semver := "1.1.0"
	target := &model.Version{VersionSemver: &semver, VersionSemverCanonical: &semver}
	if !ClientUpdatedTo(model.CompareEngineSemver, "1.1.0", target) {
		t.Fatal("equal semver must count as updated")
	}
	if ClientUpdatedTo(model.CompareEngineSemver, "1.0.0", target) {
		t.Fatal("older semver must not count as updated")
	}
	if ClientUpdatedTo(model.CompareEngineSemver, "", target) {
		t.Fatal("empty last_version is not updated")
	}
}
