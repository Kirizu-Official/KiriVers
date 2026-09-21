package service

import (
	"bytes"
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/Kirizu-Official/KiriVers/internal/model"
	"github.com/Kirizu-Official/KiriVers/pkg/hashutil"
)

func publishGrayVersion(t *testing.T, svc *ProjectService, projectID uuid.UUID, versionRef string, start int) *model.Version {
	t.Helper()
	ctx := context.Background()
	if _, _, err := svc.PutVersion(ctx, projectID, versionRef, VersionWriteInput{
		Channel:          "stable",
		GrayStartPercent: &start,
	}); err != nil {
		t.Fatalf("put version: %v", err)
	}
	payload := bytes.Repeat([]byte("x"), 64)
	sha, _ := hashutil.SHA256Hex(bytes.NewReader(payload))
	if _, err := svc.UploadArtifact(ctx, projectID.String(), versionRef, "windows", "x86_64", UploadArtifactInput{
		Filename: "demo-" + versionRef + ".zip", ExpectedSHA256: sha, Size: int64(len(payload)),
	}, bytes.NewReader(payload)); err != nil {
		t.Fatalf("upload: %v", err)
	}
	if _, err := svc.ReadyVersionLine(ctx, projectID, versionRef, "windows", "x86_64"); err != nil {
		t.Fatalf("ready: %v", err)
	}
	v, err := svc.PublishVersion(ctx, projectID, versionRef)
	if err != nil {
		t.Fatalf("publish: %v", err)
	}
	return v
}

func TestEnsureGrayAdmissionEmptyCompletes(t *testing.T) {
	svc, _, _ := setupTestService(t)
	ctx := context.Background()
	slug := "gray-empty"
	p, _, err := svc.Create(ctx, CreateProjectInput{DefaultLocale: ptr("en"), Slug: &slug})
	if err != nil {
		t.Fatal(err)
	}
	v := publishGrayVersion(t, svc, p.ID, "1.0.0", 10)
	if !v.GrayIsComplete() {
		t.Fatalf("empty namebook must complete immediately")
	}
}

func TestEnsureGrayAdmissionT0StartPercent(t *testing.T) {
	svc, store, _ := setupTestService(t)
	ctx := context.Background()
	slug := "gray-t0"
	p, _, err := svc.Create(ctx, CreateProjectInput{DefaultLocale: ptr("en"), Slug: &slug})
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 10; i++ {
		if _, err := svc.LoginClient(ctx, p, ClientLoginInput{
			DeviceID: "dev-" + string(rune('a'+i)), Version: "0.9.0", OS: "windows", Arch: "x86_64",
		}); err != nil {
			t.Fatal(err)
		}
	}
	v := publishGrayVersion(t, svc, p.ID, "1.0.0", 10)
	if v.GrayIsComplete() {
		t.Fatalf("t0 10%% of 10 clients must not complete")
	}
	n, err := store.CountAllowlist(ctx, v.ID)
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("t0 desired=ceil(10*10/100)=1, got allowlisted=%d", n)
	}
}

func TestEnsureGrayAdmissionTickCatchup(t *testing.T) {
	svc, store, _ := setupTestService(t)
	ctx := context.Background()
	slug := "gray-tick"
	p, _, err := svc.Create(ctx, CreateProjectInput{DefaultLocale: ptr("en"), Slug: &slug})
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 10; i++ {
		if _, err := svc.LoginClient(ctx, p, ClientLoginInput{
			DeviceID: "tick-" + string(rune('a'+i)), Version: "0.9.0", OS: "windows", Arch: "x86_64",
		}); err != nil {
			t.Fatal(err)
		}
	}
	start, step, interval := 10, 10, 60
	if _, _, err := svc.PutVersion(ctx, p.ID, "1.0.0", VersionWriteInput{
		Channel: "stable", GrayStartPercent: &start, GrayStepPercent: &step, GrayIntervalSeconds: &interval,
	}); err != nil {
		t.Fatal(err)
	}
	payload := bytes.Repeat([]byte("x"), 64)
	sha, _ := hashutil.SHA256Hex(bytes.NewReader(payload))
	if _, err := svc.UploadArtifact(ctx, p.ID.String(), "1.0.0", "windows", "x86_64", UploadArtifactInput{
		Filename: "demo.zip", ExpectedSHA256: sha, Size: int64(len(payload)),
	}, bytes.NewReader(payload)); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.ReadyVersionLine(ctx, p.ID, "1.0.0", "windows", "x86_64"); err != nil {
		t.Fatal(err)
	}
	v, err := svc.PublishVersion(ctx, p.ID, "1.0.0")
	if err != nil {
		t.Fatal(err)
	}
	started := time.Now().UTC().Add(-3 * time.Minute)
	v.GrayStartedAt = &started
	v.GrayCompletedAt = nil
	if err := store.SaveVersion(ctx, v); err != nil {
		t.Fatal(err)
	}
	res, err := svc.EnsureGrayAdmission(ctx, p, v)
	if err != nil {
		t.Fatal(err)
	}
	if res.TargetPercent != 40 || res.Desired != 4 {
		t.Fatalf("k=3 catch-up want target=40 desired=4, got %+v", res)
	}
	n, _ := store.CountAllowlist(ctx, v.ID)
	if n != 4 {
		t.Fatalf("catch-up allowlisted=%d want 4", n)
	}
}

func TestPutVersionCriticalForbidsGray(t *testing.T) {
	svc, _, _ := setupTestService(t)
	ctx := context.Background()
	slug := "gray-crit"
	p, _, err := svc.Create(ctx, CreateProjectInput{DefaultLocale: ptr("en"), Slug: &slug})
	if err != nil {
		t.Fatal(err)
	}
	crit := true
	start := 50
	_, _, err = svc.PutVersion(ctx, p.ID, "1.0.0", VersionWriteInput{
		Channel: "stable", IsCritical: &crit, GrayStartPercent: &start,
	})
	if err == nil {
		t.Fatal("expected ErrGrayNotAllowedOnCritical")
	}
}

func TestCreateProjectGrayWeightDefaultOn(t *testing.T) {
	svc, _, _ := setupTestService(t)
	slug := "weight-default"
	p, _, err := svc.Create(context.Background(), CreateProjectInput{DefaultLocale: ptr("en"), Slug: &slug})
	if err != nil {
		t.Fatal(err)
	}
	if !p.GrayWeightTenureActivity {
		t.Fatal("gray_weight_tenure_activity must default true")
	}
}
