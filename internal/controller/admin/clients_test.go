package admin

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/Kirizu-Official/KiriVers/internal/model"
	"github.com/Kirizu-Official/KiriVers/internal/repository"
	"github.com/Kirizu-Official/KiriVers/internal/service"
)

func TestClientBucketsIncludesChannels(t *testing.T) {
	r, projSvc, _, token := setupProjectHTTP(t)
	ctx := t.Context()
	slug := "client-channels"
	p, _, err := projSvc.Create(ctx, service.CreateProjectInput{
		DefaultLocale: ptr("en"), Slug: &slug, CompareEngine: ptr(model.CompareEngineSemver),
	})
	if err != nil {
		t.Fatal(err)
	}
	logins := []struct {
		id, channel string
	}{
		{"dev-a", "stable"},
		{"dev-b", "stable"},
		{"dev-c", "beta"},
	}
	for _, row := range logins {
		if _, err := projSvc.LoginClient(ctx, p, service.ClientLoginInput{
			DeviceID: row.id, Version: "1.0.0", OS: "windows", Arch: "x86_64", Channel: row.channel,
		}); err != nil {
			t.Fatal(err)
		}
	}

	w := doJSON(r, http.MethodGet, "/api/v1/admin/projects/"+slug+"/clients/stats", token, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("client stats: %d %s", w.Code, w.Body.String())
	}
	var buckets struct {
		Channels []struct {
			Name  string `json:"name"`
			Count int64  `json:"count"`
		} `json:"channels"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &buckets); err != nil {
		t.Fatal(err)
	}
	got := map[string]int64{}
	for _, ch := range buckets.Channels {
		got[ch.Name] = ch.Count
	}
	if got["stable"] != 2 || got["beta"] != 1 {
		t.Fatalf("channel buckets=%v want stable=2 beta=1 body=%s", got, w.Body.String())
	}
}

func TestStatsSeriesIncludesTelemetryDays(t *testing.T) {
	r, projSvc, _, token := setupProjectHTTP(t)
	telStore := repository.NewMemoryTelemetryStore()
	projSvc.SetTelemetryStore(telStore)
	ctx := t.Context()
	slug := "client-series-tel"
	p, _, err := projSvc.Create(ctx, service.CreateProjectInput{
		DefaultLocale: ptr("en"), Slug: &slug, CompareEngine: ptr(model.CompareEngineSemver),
	})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	for _, status := range []string{model.TelemetryStatusInstalled, model.TelemetryStatusInstalled, model.TelemetryStatusFailed} {
		if err := telStore.Insert(ctx, &model.TelemetryEvent{
			ProjectID: p.ID, DeviceHash: "h", OS: "windows", Arch: "x86_64",
			Status: status, CreatedAt: now,
		}); err != nil {
			t.Fatal(err)
		}
	}

	w := doJSON(r, http.MethodGet, "/api/v1/admin/projects/"+slug+"/stats/series", token, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("stats series: %d %s", w.Code, w.Body.String())
	}
	var env struct {
		Series []map[string]any `json:"series"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &env); err != nil {
		t.Fatal(err)
	}
	day := now.Format("2006-01-02")
	var found map[string]any
	for _, row := range env.Series {
		if row["day"] == day {
			found = row
			break
		}
	}
	if found == nil {
		t.Fatalf("missing %s in series %s", day, w.Body.String())
	}
	if found["installed_count"] != float64(2) || found["failed_count"] != float64(1) {
		t.Fatalf("telemetry day counts=%v want installed=2 failed=1", found)
	}
}
