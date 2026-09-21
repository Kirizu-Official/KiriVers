package client

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Kirizu-Official/KiriVers/internal/model"
	"github.com/Kirizu-Official/KiriVers/internal/service"
)

func TestReportEndpointGeoOnly(t *testing.T) {
	r, projSvc, _, ctx := setupCheckTest(t)
	slug := "rpt-geo"
	p, _, err := projSvc.Create(ctx, service.CreateProjectInput{DefaultLocale: ptr("en"), Slug: &slug})
	if err != nil {
		t.Fatal(err)
	}

	raw, _ := json.Marshal(map[string]any{
		"device_id": "dev-1",
		"version":   "1.0.0",
		"os":        "windows",
		"arch":      "x86_64",
		"channel":   "stable",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/projects/rpt-geo/clients/report", bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("report=%d %s", w.Code, w.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"ip", "country_code", "region_code", "geo_i18n"} {
		if _, ok := body[key]; !ok {
			t.Fatalf("missing %s in %s", key, w.Body.String())
		}
	}
	for key := range body {
		switch key {
		case "ip", "country_code", "region_code", "geo_i18n":
		default:
			t.Fatalf("unexpected field %q in %s", key, w.Body.String())
		}
	}

	list, _, err := projSvc.ListClients(ctx, p.ID, service.ClientListQuery{Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 {
		t.Fatalf("upsert count=%d", len(list))
	}

	wLogin := perform(r, http.MethodPost, "/api/v1/projects/rpt-geo/clients/login")
	if wLogin.Code != http.StatusNotFound {
		t.Fatalf("old login must 404, got %d %s", wLogin.Code, wLogin.Body.String())
	}

	none := "rpt-none"
	policy := model.DeviceIDPolicyNone
	if _, _, err := projSvc.Create(ctx, service.CreateProjectInput{
		DefaultLocale: ptr("en"), Slug: &none, DeviceIDPolicy: &policy,
	}); err != nil {
		t.Fatal(err)
	}
	reqNone := httptest.NewRequest(http.MethodPost, "/api/v1/projects/rpt-none/clients/report", bytes.NewReader(raw))
	reqNone.Header.Set("Content-Type", "application/json")
	wn := httptest.NewRecorder()
	r.ServeHTTP(wn, reqNone)
	if wn.Code != http.StatusBadRequest {
		t.Fatalf("none policy must 400, got %d %s", wn.Code, wn.Body.String())
	}
	assertErrorCode(t, wn, "INVALID_REQUEST")
}
