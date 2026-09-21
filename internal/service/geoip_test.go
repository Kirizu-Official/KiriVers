package service

import (
	"bytes"
	"context"
	"io"
	"net"
	"strings"
	"testing"

	"github.com/oschwald/geoip2-golang"

	"github.com/Kirizu-Official/KiriVers/internal/model"
	"github.com/Kirizu-Official/KiriVers/internal/repository"
	"github.com/Kirizu-Official/KiriVers/internal/storage"
)

type stubGeoReader struct {
	rec geoRecord
}

func (s stubGeoReader) City(net.IP) (*geoip2.City, error) {
	return nil, io.EOF
}

func (s stubGeoReader) Country(net.IP) (*geoip2.Country, error) {
	ctry := &geoip2.Country{}
	ctry.Country.IsoCode = s.rec.CountryCode
	ctry.Country.Names = s.rec.CountryNames
	return ctry, nil
}

func (s stubGeoReader) Close() error { return nil }

func TestFuseGeoRecordsFirstNonEmpty(t *testing.T) {
	got := fuseGeoRecords([]geoRecord{
		{CountryCode: "CN", CountryNames: map[string]string{"zh-CN": "中国"}},
		{CountryCode: "US", RegionCode: "CA", CountryNames: map[string]string{"zh-CN": "美国", "en": "United States"}, RegionNames: map[string]string{"en": "California"}},
	})
	if got.CountryCode != "CN" {
		t.Fatalf("country=%q", got.CountryCode)
	}
	if got.RegionCode != "CA" {
		t.Fatalf("region=%q", got.RegionCode)
	}
	if got.GeoI18n.Country["zh-CN"] != "中国" {
		t.Fatalf("zh-CN=%q", got.GeoI18n.Country["zh-CN"])
	}
	if got.GeoI18n.Country["en"] != "United States" {
		t.Fatalf("en=%q", got.GeoI18n.Country["en"])
	}
	if got.GeoI18n.Region["en"] != "California" {
		t.Fatalf("region en=%q", got.GeoI18n.Region["en"])
	}
}

func TestParsePublicIPSkipsPrivate(t *testing.T) {
	for _, raw := range []string{"", "not-an-ip", "127.0.0.1", "::1", "10.0.0.1", "192.168.1.1", "172.16.0.9", "169.254.1.1"} {
		if parsePublicIP(raw) != nil {
			t.Fatalf("expected skip %q", raw)
		}
	}
	if parsePublicIP("8.8.8.8") == nil {
		t.Fatal("8.8.8.8 should parse")
	}
}

func TestGeoipUploadAndLookupStub(t *testing.T) {
	backend, err := storage.NewLocalFS(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	svc := NewGeoipService(repository.NewMemoryGeoipStore(), backend, t.TempDir())
	svc.SetOpener(func(string) (geoReader, error) {
		return stubGeoReader{rec: geoRecord{
			CountryCode:  "JP",
			RegionCode:   "13",
			CountryNames: map[string]string{"en": "Japan", "zh-CN": "日本"},
			RegionNames:  map[string]string{"en": "Tokyo"},
		}}, nil
	})
	row, err := svc.Upload(t.Context(), "DB-IP", "city.mmdb", bytes.NewReader([]byte("mmdb-bytes")), 10)
	if err != nil {
		t.Fatal(err)
	}
	if row.StorageKey == "" || !strings.HasPrefix(row.StorageKey, "geoip/") {
		t.Fatalf("storage key=%q", row.StorageKey)
	}
	got := svc.Lookup(t.Context(), "8.8.8.8")
	if got.CountryCode != "JP" || got.GeoI18n.Country["zh-CN"] != "日本" {
		t.Fatalf("lookup=%+v", got)
	}
	empty := svc.Lookup(t.Context(), "10.1.1.1")
	if empty.CountryCode != "" {
		t.Fatalf("private ip leaked %+v", empty)
	}
}

func TestClientGeoOnIPChange(t *testing.T) {
	store := repository.NewMemoryProjectStore()
	svc := NewProjectService(store)
	svc.SetGeoipLookup(func(_ context.Context, ip string) GeoipResult {
		if ip == "8.8.8.8" {
			return GeoipResult{CountryCode: "US", GeoI18n: model.GeoI18n{Country: map[string]string{"en": "United States"}}}
		}
		return GeoipResult{CountryCode: "JP", GeoI18n: model.GeoI18n{Country: map[string]string{"en": "Japan"}}}
	})
	slug := "geo-app"
	p, _, err := svc.Create(t.Context(), CreateProjectInput{Slug: &slug, DefaultLocale: ptr("en")})
	if err != nil {
		t.Fatal(err)
	}
	first, err := svc.LoginClient(t.Context(), p, ClientLoginInput{DeviceID: "dev-1", Version: "1.0.0", OS: "windows", Arch: "x86_64", IP: "8.8.8.8"})
	if err != nil {
		t.Fatal(err)
	}
	if first.CountryCode != "US" {
		t.Fatalf("first country=%q", first.CountryCode)
	}
	same, err := svc.LoginClient(t.Context(), p, ClientLoginInput{DeviceID: "dev-1", Version: "1.0.1", OS: "windows", Arch: "x86_64", IP: "8.8.8.8"})
	if err != nil {
		t.Fatal(err)
	}
	if same.CountryCode != "US" || same.GeoI18n.Country["en"] != "United States" {
		t.Fatalf("same ip should keep geo %+v", same)
	}
	moved, err := svc.LoginClient(t.Context(), p, ClientLoginInput{DeviceID: "dev-1", Version: "1.0.2", OS: "windows", Arch: "x86_64", IP: "1.1.1.1"})
	if err != nil {
		t.Fatal(err)
	}
	if moved.CountryCode != "JP" {
		t.Fatalf("ip change country=%q", moved.CountryCode)
	}
}

func TestDeleteClientClearsAllowlist(t *testing.T) {
	store := repository.NewMemoryProjectStore()
	svc := NewProjectService(store)
	slug := "del-app"
	p, _, err := svc.Create(t.Context(), CreateProjectInput{Slug: &slug, DefaultLocale: ptr("en")})
	if err != nil {
		t.Fatal(err)
	}
	cl, err := svc.LoginClient(t.Context(), p, ClientLoginInput{DeviceID: "dev-x", Version: "1.0.0", OS: "linux", Arch: "x86_64"})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.InsertAllowlist(t.Context(), []model.GrayAllowlist{{
		ProjectID: p.ID, VersionID: cl.ID, DeviceID: cl.DeviceHash, Source: model.GraySourceManual,
	}}); err != nil {
		t.Fatal(err)
	}
	if err := svc.DeleteClient(t.Context(), p.ID, cl.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.GetClient(t.Context(), p.ID, cl.ID); err != ErrClientNotFound {
		t.Fatalf("get after delete: %v", err)
	}
	left, err := store.ListVersionAllowlist(t.Context(), p.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(left) != 0 {
		t.Fatalf("allowlist leftover=%d", len(left))
	}
}
