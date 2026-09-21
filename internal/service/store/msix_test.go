package store

import (
	"context"
	"encoding/xml"
	"net/http"
	"strings"
	"testing"

	"github.com/Kirizu-Official/KiriVers/internal/model"
	"github.com/Kirizu-Official/KiriVers/internal/service/update"
)

// ---------- MSIX App Installer .appinstaller 单元测试（§9 MSIX 行 / §9.2，C23-1..C23-6） ----------

// appInstallerDoc 是 XML 反序列化验证根结构（验收项 1：必填 XML 元素完整）。
type appInstallerDoc struct {
	XMLName        xml.Name                   `xml:"http://schemas.microsoft.com/appx/appinstaller/2018 AppInstaller"`
	Version        string                     `xml:"Version,attr"`
	Uri            string                     `xml:"Uri,attr"`
	MainPackage    appInstallerMainPackage    `xml:"MainPackage"`
	UpdateSettings appInstallerUpdateSettings `xml:"UpdateSettings"`
}

type appInstallerMainPackage struct {
	Name                  string `xml:"Name,attr"`
	Publisher             string `xml:"Publisher,attr"`
	Version               string `xml:"Version,attr"`
	ProcessorArchitecture string `xml:"ProcessorArchitecture,attr"`
	Uri                   string `xml:"Uri,attr"`
}

type appInstallerUpdateSettings struct {
	OnLaunch                  appInstallerOnLaunch `xml:"OnLaunch"`
	AutomaticBackgroundTask   *struct{}            `xml:"AutomaticBackgroundTask"`
	ForceUpdateFromAnyVersion string               `xml:"ForceUpdateFromAnyVersion"`
	ShowPrompt                string               `xml:"ShowPrompt"`
	UpdateBlocksActivation    string               `xml:"UpdateBlocksActivation"`
}

type appInstallerOnLaunch struct {
	HoursBetweenUpdateChecks string `xml:"HoursBetweenUpdateChecks,attr"`
}

// parseAppInstallerXML 解析并验证 App Installer XML 清单。
func parseAppInstallerXML(t *testing.T, body []byte) *appInstallerDoc {
	t.Helper()
	var doc appInstallerDoc
	if err := xml.Unmarshal(body, &doc); err != nil {
		t.Fatalf("xml unmarshal failed: %v\nbody:\n%s", err, body)
	}
	return &doc
}

// TestMSIX_XMLFormatAndRequiredElements 验收项 1：必填 XML 元素完整、命名空间正确、Content-Type 正确。
func TestMSIX_XMLFormatAndRequiredElements(t *testing.T) {
	cat, storageData := newSquirrelMultiVerCatalog(t, "msixdemo", "x86_64", []squirrelVer{
		{
			Semver:     "1.2.3",
			VersionInt: 4,
			Rollout:    100,
			Ready:      true,
			FileName:   "msixdemo-1.2.3-x64.msix",
		},
	})
	storage := newFakeStorage(storageData)

	adapter := NewMSIXAdapter()
	deps := &Deps{Updates: update.NewService(&fakeLoader{cat: cat}), Storage: storage}
	req := Request{
		Project: &model.Project{
			ID:   cat.Project.ID,
			Slug: cat.Project.Slug,
		},
		Listing: testListing("msix", map[string]string{
			"publisher": "CN=ContosoCorp",
		}),
		Channel: "stable",
		Path:    "app.appinstaller",
		Arch:    "x86_64",
	}

	resp, err := adapter.Render(context.Background(), deps, req)
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if resp.Status != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.Status)
	}
	if resp.ContentType != "application/appinstaller+xml; charset=utf-8" {
		t.Fatalf("content type = %q, want application/appinstaller+xml; charset=utf-8", resp.ContentType)
	}

	doc := parseAppInstallerXML(t, resp.Body)

	// 1. 根属性与版本
	if doc.Version != "1.2.3.4" {
		t.Fatalf("root Version = %q, want 1.2.3.4", doc.Version)
	}
	if !strings.Contains(doc.Uri, "app.appinstaller") {
		t.Fatalf("root Uri = %q, want contain app.appinstaller", doc.Uri)
	}

	// 2. MainPackage 属性
	if doc.MainPackage.Name != "msixdemo" {
		t.Fatalf("MainPackage Name = %q, want msixdemo", doc.MainPackage.Name)
	}
	if doc.MainPackage.Publisher != "CN=ContosoCorp" {
		t.Fatalf("MainPackage Publisher = %q, want CN=ContosoCorp", doc.MainPackage.Publisher)
	}
	if doc.MainPackage.Version != "1.2.3.4" {
		t.Fatalf("MainPackage Version = %q, want 1.2.3.4", doc.MainPackage.Version)
	}
	if doc.MainPackage.ProcessorArchitecture != "x64" {
		t.Fatalf("MainPackage ProcessorArchitecture = %q, want x64", doc.MainPackage.ProcessorArchitecture)
	}
	if !strings.Contains(doc.MainPackage.Uri, "/packages/") {
		t.Fatalf("MainPackage Uri = %q, want hash package URL", doc.MainPackage.Uri)
	}

	// 3. UpdateSettings
	if doc.UpdateSettings.OnLaunch.HoursBetweenUpdateChecks != "0" {
		t.Fatalf("HoursBetweenUpdateChecks = %q, want 0", doc.UpdateSettings.OnLaunch.HoursBetweenUpdateChecks)
	}
	if doc.UpdateSettings.AutomaticBackgroundTask == nil {
		t.Fatalf("AutomaticBackgroundTask should not be nil")
	}
	if doc.UpdateSettings.ForceUpdateFromAnyVersion != "true" {
		t.Fatalf("ForceUpdateFromAnyVersion = %q, want true", doc.UpdateSettings.ForceUpdateFromAnyVersion)
	}
}

// TestMSIX_NonMSIXArtifactIgnored 验收项 2：非 msix 扩展名的 Line 不会出现在 appinstaller 的 MainPackage。
func TestMSIX_NonMSIXArtifactIgnored(t *testing.T) {
	// 版本 2.0.0 产物为 .zip（非 msix），版本 1.0.0 产物为 .msix
	cat, storageData := newSquirrelMultiVerCatalog(t, "msixdemo", "x86_64", []squirrelVer{
		{
			Semver:     "2.0.0",
			VersionInt: 20,
			Rollout:    100,
			Ready:      true,
			FileName:   "msixdemo-2.0.0.zip", // 非 msix 产物！
		},
		{
			Semver:     "1.0.0",
			VersionInt: 10,
			Rollout:    100,
			Ready:      true,
			FileName:   "msixdemo-1.0.0.msix", // 合法 msix 产物
		},
	})
	storage := newFakeStorage(storageData)

	adapter := NewMSIXAdapter()
	deps := &Deps{Updates: update.NewService(&fakeLoader{cat: cat}), Storage: storage}
	req := Request{
		Project: &model.Project{
			ID:   cat.Project.ID,
			Slug: cat.Project.Slug,
		},
		Channel: "stable",
		Path:    "package.appinstaller",
		Arch:    "x86_64",
	}

	resp, err := adapter.Render(context.Background(), deps, req)
	if err != nil {
		t.Fatalf("render failed: %v", err)
	}

	doc := parseAppInstallerXML(t, resp.Body)

	// MainPackage 必须指向 1.0.0.10 的 msix，而不是 2.0.0 的 zip！
	if doc.MainPackage.Version != "1.0.0.10" {
		t.Fatalf("MainPackage Version = %q, want 1.0.0.10 (should ignore 2.0.0.zip)", doc.MainPackage.Version)
	}
	if !strings.Contains(doc.MainPackage.Uri, "/packages/") {
		t.Fatalf("MainPackage Uri = %q, want hash package URL", doc.MainPackage.Uri)
	}
}

// TestMSIX_AllNonMSIXReturnsErrNoRelease 验证当所有版本产物均非 msix 时返回 ErrNoRelease（404）。
func TestMSIX_AllNonMSIXReturnsErrNoRelease(t *testing.T) {
	cat, storageData := newSquirrelMultiVerCatalog(t, "msixdemo", "x86_64", []squirrelVer{
		{
			Semver:     "2.0.0",
			VersionInt: 20,
			Rollout:    100,
			Ready:      true,
			FileName:   "msixdemo-2.0.0.zip",
		},
		{
			Semver:     "1.0.0",
			VersionInt: 10,
			Rollout:    100,
			Ready:      true,
			FileName:   "msixdemo-1.0.0.exe",
		},
	})
	storage := newFakeStorage(storageData)

	adapter := NewMSIXAdapter()
	deps := &Deps{Updates: update.NewService(&fakeLoader{cat: cat}), Storage: storage}
	req := Request{
		Project: &model.Project{
			ID:   cat.Project.ID,
			Slug: cat.Project.Slug,
		},
		Channel: "stable",
		Path:    "app.appinstaller",
		Arch:    "x86_64",
	}

	_, err := adapter.Render(context.Background(), deps, req)
	if err != ErrNoRelease {
		t.Fatalf("err = %v, want ErrNoRelease", err)
	}
}

// TestMSIX_QuadVersionMapping 验证四段式版本映射逻辑（C23-2）。
func TestMSIX_QuadVersionMapping(t *testing.T) {
	sem1 := "1.2.3"
	int1 := int64(4)
	if v := msixVersion(&sem1, &int1); v != "1.2.3.4" {
		t.Fatalf("want 1.2.3.4, got %q", v)
	}

	sem2 := "2.0.0"
	if v := msixVersion(&sem2, nil); v != "2.0.0.0" {
		t.Fatalf("want 2.0.0.0, got %q", v)
	}

	int3 := int64(42)
	if v := msixVersion(nil, &int3); v != "1.0.0.42" {
		t.Fatalf("want 1.0.0.42, got %q", v)
	}

	if v := msixVersion(nil, nil); v != "1.0.0.0" {
		t.Fatalf("want 1.0.0.0, got %q", v)
	}
}

// TestMSIX_ArchitectureMapping 验证架构映射（x86_64->x64, arm64->arm64 等）。
func TestMSIX_ArchitectureMapping(t *testing.T) {
	tests := []struct {
		arch string
		want string
	}{
		{"x86_64", "x64"},
		{"amd64", "x64"},
		{"x86", "x86"},
		{"386", "x86"},
		{"arm64", "arm64"},
		{"aarch64", "arm64"},
		{"arm", "arm"},
		{"mips", "neutral"},
	}
	for _, tc := range tests {
		if got := msixProcessorArch(tc.arch); got != tc.want {
			t.Errorf("msixProcessorArch(%q) = %q, want %q", tc.arch, got, tc.want)
		}
	}
}

// TestMSIX_CustomStoreProtocolIdentifiers 验证自定义 identifiers 参数。
func TestMSIX_CustomStoreProtocolIdentifiers(t *testing.T) {
	cat, storageData := newSquirrelMultiVerCatalog(t, "customapp", "arm64", []squirrelVer{
		{
			Semver:     "3.1.4",
			VersionInt: 15,
			Rollout:    100,
			Ready:      true,
			FileName:   "customapp-3.1.4-arm64.msix",
		},
	})
	storage := newFakeStorage(storageData)

	adapter := NewMSIXAdapter()
	deps := &Deps{Updates: update.NewService(&fakeLoader{cat: cat}), Storage: storage}
	req := Request{
		Project: &model.Project{
			ID:   cat.Project.ID,
			Slug: cat.Project.Slug,
		},
		Listing: testListing("msix", map[string]string{
			"name":                          "Contoso.CustomApp",
			"publisher":                     "CN=Contoso, O=Contoso Inc",
			"appinstaller_uri":              "https://app.contoso.com/customapp.appinstaller",
			"hours_between_update_checks":   "12",
			"automatic_background_task":     "false",
			"force_update_from_any_version": "false",
			"show_prompt":                   "true",
			"update_blocks_activation":      "true",
		}),
		Channel: "stable",
		Path:    "customapp.appinstaller",
		Arch:    "arm64",
	}

	resp, err := adapter.Render(context.Background(), deps, req)
	if err != nil {
		t.Fatalf("render: %v", err)
	}

	doc := parseAppInstallerXML(t, resp.Body)
	if doc.Uri != "https://app.contoso.com/customapp.appinstaller" {
		t.Fatalf("Uri = %q, want https://app.contoso.com/customapp.appinstaller", doc.Uri)
	}
	if doc.MainPackage.Name != "Contoso.CustomApp" {
		t.Fatalf("MainPackage.Name = %q, want Contoso.CustomApp", doc.MainPackage.Name)
	}
	if doc.MainPackage.Publisher != "CN=Contoso, O=Contoso Inc" {
		t.Fatalf("MainPackage.Publisher = %q, want CN=Contoso, O=Contoso Inc", doc.MainPackage.Publisher)
	}
	if doc.MainPackage.ProcessorArchitecture != "arm64" {
		t.Fatalf("MainPackage.ProcessorArchitecture = %q, want arm64", doc.MainPackage.ProcessorArchitecture)
	}
	if doc.UpdateSettings.OnLaunch.HoursBetweenUpdateChecks != "12" {
		t.Fatalf("HoursBetweenUpdateChecks = %q, want 12", doc.UpdateSettings.OnLaunch.HoursBetweenUpdateChecks)
	}
	if doc.UpdateSettings.AutomaticBackgroundTask != nil {
		t.Fatalf("AutomaticBackgroundTask should be omitted when false")
	}
	if doc.UpdateSettings.ForceUpdateFromAnyVersion != "" {
		t.Fatalf("ForceUpdateFromAnyVersion should be omitted when false, got %q", doc.UpdateSettings.ForceUpdateFromAnyVersion)
	}
	if doc.UpdateSettings.ShowPrompt != "true" {
		t.Fatalf("ShowPrompt = %q, want true", doc.UpdateSettings.ShowPrompt)
	}
	if doc.UpdateSettings.UpdateBlocksActivation != "true" {
		t.Fatalf("UpdateBlocksActivation = %q, want true", doc.UpdateSettings.UpdateBlocksActivation)
	}
}

// TestMSIX_GrayRolloutAndCritical 验证灰度过滤与关键版本豁免。
func TestMSIX_GrayRolloutAndCritical(t *testing.T) {
	// v2 灰度 50%（非关键，不可见）；v1 灰度 50% 且关键（可见）
	cat, storageData := newSquirrelMultiVerCatalog(t, "msixdemo", "x86_64", []squirrelVer{
		{
			Semver:     "2.0.0",
			VersionInt: 20,
			Rollout:    50,
			IsCritical: false,
			Ready:      true,
			FileName:   "msixdemo-2.0.0.msix",
		},
		{
			Semver:     "1.0.0",
			VersionInt: 10,
			Rollout:    50,
			IsCritical: true,
			Ready:      true,
			FileName:   "msixdemo-1.0.0.msix",
		},
	})
	storage := newFakeStorage(storageData)

	adapter := NewMSIXAdapter()
	deps := &Deps{Updates: update.NewService(&fakeLoader{cat: cat}), Storage: storage}
	req := Request{
		Project: &model.Project{
			ID:   cat.Project.ID,
			Slug: cat.Project.Slug,
		},
		Channel: "stable",
		Path:    "app.appinstaller",
		Arch:    "x86_64",
	}

	resp, err := adapter.Render(context.Background(), deps, req)
	if err != nil {
		t.Fatalf("render: %v", err)
	}

	doc := parseAppInstallerXML(t, resp.Body)
	if doc.MainPackage.Version != "1.0.0.10" {
		t.Fatalf("MainPackage Version = %q, want 1.0.0.10 (v2 should be excluded by gray)", doc.MainPackage.Version)
	}
}

// TestMSIX_DefaultHwVariantOnly 验证仅包含默认硬件变体产物。
func TestMSIX_DefaultHwVariantOnly(t *testing.T) {
	hwRev := "rev-b"
	cat, storageData := newSquirrelMultiVerCatalog(t, "msixdemo", "x86_64", []squirrelVer{
		{
			Semver:     "2.0.0",
			VersionInt: 20,
			Rollout:    100,
			Ready:      true,
			FileName:   "msixdemo-2.0.0-revb.msix",
			HwRev:      &hwRev, // 非默认变体
		},
		{
			Semver:     "1.0.0",
			VersionInt: 10,
			Rollout:    100,
			Ready:      true,
			FileName:   "msixdemo-1.0.0.msix", // 默认变体（hwRev == nil）
		},
	})
	storage := newFakeStorage(storageData)

	adapter := NewMSIXAdapter()
	deps := &Deps{Updates: update.NewService(&fakeLoader{cat: cat}), Storage: storage}
	req := Request{
		Project: &model.Project{
			ID:   cat.Project.ID,
			Slug: cat.Project.Slug,
		},
		Channel: "stable",
		Path:    "app.appinstaller",
		Arch:    "x86_64",
	}

	resp, err := adapter.Render(context.Background(), deps, req)
	if err != nil {
		t.Fatalf("render: %v", err)
	}

	doc := parseAppInstallerXML(t, resp.Body)
	if doc.MainPackage.Version != "1.0.0.10" {
		t.Fatalf("MainPackage Version = %q, want 1.0.0.10 (rev-b should be excluded)", doc.MainPackage.Version)
	}
}

// TestMSIX_InvalidPath 验证路径不以 .appinstaller 结尾报 ErrUnknownPath（404）。
func TestMSIX_InvalidPath(t *testing.T) {
	cat, storageData := newSquirrelMultiVerCatalog(t, "msixdemo", "x86_64", []squirrelVer{
		{Semver: "1.0.0", Rollout: 100, Ready: true, FileName: "app.msix"},
	})
	storage := newFakeStorage(storageData)

	adapter := NewMSIXAdapter()
	deps := &Deps{Updates: update.NewService(&fakeLoader{cat: cat}), Storage: storage}

	for _, badPath := range []string{"package.xml", "app.exe", "RELEASES", "latest.json"} {
		req := Request{
			Project: &model.Project{ID: cat.Project.ID, Slug: cat.Project.Slug},
			Channel: "stable",
			Path:    badPath,
			Arch:    "x86_64",
		}
		_, err := adapter.Render(context.Background(), deps, req)
		if err == nil || !strings.Contains(err.Error(), ErrUnknownPath.Error()) {
			t.Fatalf("path %q err = %v, want ErrUnknownPath", badPath, err)
		}
	}
}

// TestMSIX_EnabledFlag：HTTP 闸是 listing；Adapter.Enabled 仅 nil 为 false。
func TestMSIX_EnabledFlag(t *testing.T) {
	adapter := NewMSIXAdapter()
	if adapter.Enabled(nil) {
		t.Fatalf("Enabled(nil) should be false")
	}
	if !adapter.Enabled(&model.Project{}) {
		t.Fatalf("non-nil project must be enabled (listing is the HTTP gate)")
	}
}

// TestMSIX_XMLEscaping 验证特殊字符安全转义。
func TestMSIX_XMLEscaping(t *testing.T) {
	cat, storageData := newSquirrelMultiVerCatalog(t, "msixdemo", "x86_64", []squirrelVer{
		{Semver: "1.0.0", Rollout: 100, Ready: true, FileName: "app-1.0.0.msix"},
	})
	storage := newFakeStorage(storageData)

	adapter := NewMSIXAdapter()
	deps := &Deps{Updates: update.NewService(&fakeLoader{cat: cat}), Storage: storage}
	req := Request{
		Project: &model.Project{
			ID:   cat.Project.ID,
			Slug: cat.Project.Slug,
		},
		Listing: testListing("msix", map[string]string{
			"name":      "Contoso <App> & Co",
			"publisher": "CN=Contoso & Sons \"Quotes\"",
		}),
		Channel: "stable",
		Path:    "test.appinstaller",
		Arch:    "x86_64",
	}

	resp, err := adapter.Render(context.Background(), deps, req)
	if err != nil {
		t.Fatalf("render: %v", err)
	}

	// 必须能够成功反序列化 XML
	doc := parseAppInstallerXML(t, resp.Body)
	if doc.MainPackage.Name != "Contoso <App> & Co" {
		t.Fatalf("Name = %q, want Contoso <App> & Co", doc.MainPackage.Name)
	}
	if doc.MainPackage.Publisher != "CN=Contoso & Sons \"Quotes\"" {
		t.Fatalf("Publisher = %q, want CN=Contoso & Sons \"Quotes\"", doc.MainPackage.Publisher)
	}
}
