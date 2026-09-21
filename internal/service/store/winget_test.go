package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/Kirizu-Official/KiriVers/internal/model"
	"github.com/Kirizu-Official/KiriVers/internal/service/update"
)

// ---------- WinGet REST 源适配器单元测试（§9 WinGet REST 行 / §9.2，C22-1..C22-7） ----------

type wingetTestLine struct {
	Arch     string
	Filename string
	SHA256   string
	HwRev    *string
	Ready    bool
}

type wingetTestVer struct {
	Semver     string
	VersionInt int64
	Rollout    int
	IsCritical bool
	Changelog  string
	Lines      []wingetTestLine
}

func newWinGetCatalog(t *testing.T, slug string, vers []wingetTestVer) *update.Catalog {
	t.Helper()
	cat := &update.Catalog{
		Project: newTestProject(slug),
		Channels: []update.ChannelInfo{
			{Slug: "stable", StabilityRank: 30, Enabled: true},
			{Slug: "beta", StabilityRank: 20, Enabled: true},
		},
		Matrix:    &update.MatrixInfo{OS: "windows", Arch: "x86_64", PackageType: model.PackageTypeSingleFile},
		EnabledOS: []string{"windows"},
		Versions:  make([]update.VersionState, 0, len(vers)),
	}

	for _, vInfo := range vers {
		vID := uuid.New()
		v := model.Version{
			ID:               vID,
			ProjectID:        testProjectID,
			ChannelSlug:      "stable",
			Status:           model.VersionStatusPublished,
			GrayStartPercent: 100,
			IsCritical:       vInfo.IsCritical,
			PublishTime:      &testPublishTime,
		}
		applyFeedGray(&v, vInfo.Rollout, vInfo.IsCritical)
		if vInfo.Semver != "" {
			v.VersionSemver = &vInfo.Semver
			v.VersionSemverCanonical = &vInfo.Semver
		}
		if vInfo.VersionInt > 0 {
			v.VersionInteger = &vInfo.VersionInt
		}
		if vInfo.Changelog != "" {
			v.Changelog = model.ChangelogMap{
				"en": {Title: vInfo.Changelog, Markdown: vInfo.Changelog},
			}
		}

		vs := update.VersionState{
			Version: v,
			Lines:   make([]update.LineState, 0, len(vInfo.Lines)),
		}

		for _, lInfo := range vInfo.Lines {
			lineID := uuid.New()
			status := model.VersionLineStatusReady
			if !lInfo.Ready {
				status = model.VersionLineStatusPending
			}

			ls := update.LineState{
				ID:     lineID,
				OS:     "windows",
				Arch:   lInfo.Arch,
				Status: status,
			}
			if status == model.VersionLineStatusReady {
				ls.PacksReadyAt = packsReady()
			}

			sha := lInfo.SHA256
			if sha == "" {
				sha = "44d88612fea8a8f36de82e1278abb02f0e0123456789abcdef0123456789abcd"
			}
			fn := lInfo.Filename
			if fn == "" {
				fn = fmt.Sprintf("%s-%s-windows-%s.exe", slug, vInfo.Semver, lInfo.Arch)
			}

			ls.FullPkgs = []update.ArtifactInfo{
				{
					HwRev:      lInfo.HwRev,
					FileName:   fn,
					SHA256:     sha,
					StorageKey: slug + "/" + sha,
					Size:       10240,
				},
			}

			vs.Lines = append(vs.Lines, ls)
		}

		cat.Versions = append(cat.Versions, vs)
	}

	return cat
}

// TestWinGet_InformationEndpoint 验证 GET /information 满足官方规范与支持版本。
func TestWinGet_InformationEndpoint(t *testing.T) {
	adapter := NewWinGetAdapter()
	p := &model.Project{
		Slug: "aidemo",
	}
	listing := testListing("winget", map[string]string{
		"package_identifier": "Contoso.AiDemo",
	})

	cat := newWinGetCatalog(t, "aidemo", nil)
	deps := &Deps{Updates: update.NewService(&fakeLoader{cat: cat})}

	resp, err := adapter.Render(context.Background(), deps, Request{
		Project: p,
		Listing: listing,
		Path:    "information",
	})
	if err != nil {
		t.Fatalf("render information: %v", err)
	}

	if resp.ContentType != wingetContentType {
		t.Fatalf("content-type = %q, want %q", resp.ContentType, wingetContentType)
	}

	var res wingetInformationResponse
	if err := json.Unmarshal(resp.Body, &res); err != nil {
		t.Fatalf("unmarshal json: %v", err)
	}

	if res.Data.SourceIdentifier != "Contoso.AiDemo" {
		t.Fatalf("SourceIdentifier = %q, want Contoso.AiDemo", res.Data.SourceIdentifier)
	}

	supportedMap := make(map[string]bool)
	for _, v := range res.Data.ServerSupportedVersions {
		supportedMap[v] = true
	}
	if !supportedMap["1.1.0"] || !supportedMap["1.4.0"] {
		t.Fatalf("ServerSupportedVersions = %v, must contain 1.1.0 and 1.4.0", res.Data.ServerSupportedVersions)
	}
	if res.Data.UnsupportedPackageMatchFields == nil || res.Data.RequiredPackageMatchFields == nil {
		t.Fatal("MatchFields arrays must not be nil")
	}
}

// TestWinGet_ManifestSearchEndpoint 验证 manifestSearch 搜索与列表输出。
func TestWinGet_ManifestSearchEndpoint(t *testing.T) {
	adapter := NewWinGetAdapter()
	p := &model.Project{
		Slug: "aidemo",
	}
	listing := testListing("winget", map[string]string{
		"package_identifier": "Contoso.AiDemo",
		"publisher":          "Contoso Corporation",
		"package_name":       "AI Demo Suite",
	})

	vers := []wingetTestVer{
		{
			Semver:  "1.1.0",
			Rollout: 100,
			Lines: []wingetTestLine{
				{Arch: "x86_64", Ready: true},
			},
		},
		{
			Semver:  "1.0.0",
			Rollout: 100,
			Lines: []wingetTestLine{
				{Arch: "x86_64", Ready: true},
			},
		},
	}

	cat := newWinGetCatalog(t, "aidemo", vers)
	deps := &Deps{Updates: update.NewService(&fakeLoader{cat: cat})}

	resp, err := adapter.Render(context.Background(), deps, Request{
		Project: p,
		Listing: listing,
		Path:    "manifestSearch",
	})
	if err != nil {
		t.Fatalf("render manifestSearch: %v", err)
	}

	var res wingetSearchResponse
	if err := json.Unmarshal(resp.Body, &res); err != nil {
		t.Fatalf("unmarshal json: %v", err)
	}

	if len(res.Data) != 1 {
		t.Fatalf("Data len = %d, want 1", len(res.Data))
	}
	pkg := res.Data[0]
	if pkg.PackageIdentifier != "Contoso.AiDemo" {
		t.Fatalf("PackageIdentifier = %q, want Contoso.AiDemo", pkg.PackageIdentifier)
	}
	if pkg.PackageName != "AI Demo Suite" {
		t.Fatalf("PackageName = %q, want AI Demo Suite", pkg.PackageName)
	}
	if pkg.Publisher != "Contoso Corporation" {
		t.Fatalf("Publisher = %q, want Contoso Corporation", pkg.Publisher)
	}
	if len(pkg.Versions) != 2 {
		t.Fatalf("Versions len = %d, want 2", len(pkg.Versions))
	}
	if pkg.Versions[0].PackageVersion != "1.1.0" || pkg.Versions[1].PackageVersion != "1.0.0" {
		t.Fatalf("Versions = %+v", pkg.Versions)
	}
}

// TestWinGet_ManifestSearchEmptyCatalog 验证可见集为空时返回空 Data 数组。
func TestWinGet_ManifestSearchEmptyCatalog(t *testing.T) {
	adapter := NewWinGetAdapter()
	p := &model.Project{
		Slug: "aidemo",
	}
	listing := testListing("winget", nil)

	cat := newWinGetCatalog(t, "aidemo", nil)
	deps := &Deps{Updates: update.NewService(&fakeLoader{cat: cat})}

	resp, err := adapter.Render(context.Background(), deps, Request{
		Project: p,
		Listing: listing,
		Path:    "manifestSearch",
	})
	if err != nil {
		t.Fatalf("render manifestSearch: %v", err)
	}

	var res wingetSearchResponse
	if err := json.Unmarshal(resp.Body, &res); err != nil {
		t.Fatalf("unmarshal json: %v", err)
	}
	if len(res.Data) != 0 {
		t.Fatalf("empty catalog Data len = %d, want 0", len(res.Data))
	}
}

// TestWinGet_PackageManifestsSingleAndMultiple 验证单包与列表清单模式。
func TestWinGet_PackageManifestsSingleAndMultiple(t *testing.T) {
	adapter := NewWinGetAdapter()
	p := &model.Project{
		Slug: "aidemo",
	}
	listing := testListing("winget", map[string]string{
		"package_identifier": "Contoso.AiDemo",
		"publisher":          "Contoso",
		"package_name":       "DemoApp",
		"license":            "MIT",
		"short_description":  "AI desktop app",
	})

	vers := []wingetTestVer{
		{
			Semver:    "1.0.0",
			Rollout:   100,
			Changelog: "Initial stable release",
			Lines: []wingetTestLine{
				{
					Arch:     "x86_64",
					Filename: "aidemo-1.0.0-windows-x86_64.exe",
					SHA256:   "aabbccddeeff00112233445566778899aabbccddeeff00112233445566778899",
					Ready:    true,
				},
				{
					Arch:     "arm64",
					Filename: "aidemo-1.0.0-windows-arm64.exe",
					SHA256:   "112233445566778899aabbccddeeff00112233445566778899aabbccddeeff00",
					Ready:    true,
				},
			},
		},
	}

	cat := newWinGetCatalog(t, "aidemo", vers)
	deps := &Deps{Updates: update.NewService(&fakeLoader{cat: cat})}

	// 1. 单包路径：packageManifests/Contoso.AiDemo
	respSingle, err := adapter.Render(context.Background(), deps, Request{
		Project: p,
		Listing: listing,
		Path:    "packageManifests/Contoso.AiDemo",
	})
	if err != nil {
		t.Fatalf("render single packageManifest: %v", err)
	}

	var singleRes wingetSingleManifestResponse
	if err := json.Unmarshal(respSingle.Body, &singleRes); err != nil {
		t.Fatalf("unmarshal single: %v", err)
	}
	if singleRes.Data.PackageIdentifier != "Contoso.AiDemo" {
		t.Fatalf("single PackageIdentifier = %q", singleRes.Data.PackageIdentifier)
	}
	if len(singleRes.Data.Versions) != 1 {
		t.Fatalf("single Versions len = %d, want 1", len(singleRes.Data.Versions))
	}
	vItem := singleRes.Data.Versions[0]
	if vItem.PackageVersion != "1.0.0" {
		t.Fatalf("PackageVersion = %q, want 1.0.0", vItem.PackageVersion)
	}
	if vItem.DefaultLocale.Publisher != "Contoso" || vItem.DefaultLocale.PackageName != "DemoApp" {
		t.Fatalf("DefaultLocale mismatch: %+v", vItem.DefaultLocale)
	}
	if vItem.DefaultLocale.ShortDescription != "Initial stable release" {
		t.Fatalf("ShortDescription = %q", vItem.DefaultLocale.ShortDescription)
	}

	// 验证包含两个架构安装器：x64 与 arm64
	if len(vItem.Installers) != 2 {
		t.Fatalf("Installers len = %d, want 2", len(vItem.Installers))
	}
	arches := make(map[string]wingetInstaller)
	for _, inst := range vItem.Installers {
		arches[inst.Architecture] = inst
	}
	x64Inst, ok := arches["x64"]
	if !ok {
		t.Fatalf("missing x64 installer in %v", arches)
	}
	if x64Inst.InstallerType != "exe" {
		t.Fatalf("x64 InstallerType = %q, want exe", x64Inst.InstallerType)
	}
	if x64Inst.InstallerSha256 != "AABBCCDDEEFF00112233445566778899AABBCCDDEEFF00112233445566778899" {
		t.Fatalf("x64 SHA256 = %q, want uppercase", x64Inst.InstallerSha256)
	}
	if !strings.Contains(x64Inst.InstallerUrl, "/packages/") {
		t.Fatalf("InstallerUrl = %q", x64Inst.InstallerUrl)
	}

	// 2. 列表路径：packageManifests
	respMulti, err := adapter.Render(context.Background(), deps, Request{
		Project: p,
		Listing: listing,
		Path:    "packageManifests",
	})
	if err != nil {
		t.Fatalf("render multi packageManifests: %v", err)
	}

	var multiRes wingetMultipleManifestResponse
	if err := json.Unmarshal(respMulti.Body, &multiRes); err != nil {
		t.Fatalf("unmarshal multi: %v", err)
	}
	if len(multiRes.Data) != 1 {
		t.Fatalf("multi Data len = %d, want 1", len(multiRes.Data))
	}
	if multiRes.Data[0].PackageIdentifier != "Contoso.AiDemo" {
		t.Fatalf("multi PackageIdentifier = %q", multiRes.Data[0].PackageIdentifier)
	}
}

// TestWinGet_DualNumberMapping 验证 C22-3 双号映射规则。
func TestWinGet_DualNumberMapping(t *testing.T) {
	adapter := NewWinGetAdapter()
	p := &model.Project{
		Slug: "aidemo",
	}
	listing := testListing("winget", map[string]string{"package_identifier": "Contoso.AiDemo"})

	// Case 1: 仅有 VersionInteger
	vers := []wingetTestVer{
		{
			VersionInt: 42,
			Rollout:    100,
			Lines: []wingetTestLine{
				{Arch: "x86_64", Ready: true},
			},
		},
	}
	cat := newWinGetCatalog(t, "aidemo", vers)
	deps := &Deps{Updates: update.NewService(&fakeLoader{cat: cat})}

	resp, err := adapter.Render(context.Background(), deps, Request{
		Project: p,
		Listing: listing,
		Path:    "packageManifests/Contoso.AiDemo",
	})
	if err != nil {
		t.Fatalf("render int version: %v", err)
	}

	var singleRes wingetSingleManifestResponse
	if err := json.Unmarshal(resp.Body, &singleRes); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if singleRes.Data.Versions[0].PackageVersion != "42" {
		t.Fatalf("PackageVersion = %q, want 42", singleRes.Data.Versions[0].PackageVersion)
	}
}

// TestWinGet_InstallerTypeDetection 验证安装器类型推断（C22-2）。
func TestWinGet_InstallerTypeDetection(t *testing.T) {
	tests := []struct {
		name       string
		filename   string
		configured string
		wantType   string
	}{
		{"exe default", "app.exe", "", "exe"},
		{"msi extension", "setup.msi", "", "msi"},
		{"msix extension", "app.msix", "", "msix"},
		{"appx extension", "app.appx", "", "msix"},
		{"zip extension", "portable.zip", "", "zip"},
		{"configured override", "app.exe", "inno", "inno"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := detectWinGetInstallerType(tt.filename, tt.configured)
			if got != tt.wantType {
				t.Fatalf("detectWinGetInstallerType(%q, %q) = %q, want %q", tt.filename, tt.configured, got, tt.wantType)
			}
		})
	}
}

// TestWinGet_GrayRolloutAndCritical 验证灰度过滤与关键版本豁免（C22-4）。
func TestWinGet_GrayRolloutAndCritical(t *testing.T) {
	adapter := NewWinGetAdapter()
	p := &model.Project{
		Slug: "aidemo",
	}
	listing := testListing("winget", map[string]string{"package_identifier": "Contoso.AiDemo"})

	vers := []wingetTestVer{
		{
			Semver:  "1.2.0",
			Rollout: 50, // 未全量，非关键 -> 过滤
			Lines:   []wingetTestLine{{Arch: "x86_64", Ready: true}},
		},
		{
			Semver:     "1.1.0",
			Rollout:    20, // 未全量，但关键版本 -> 包含
			IsCritical: true,
			Lines:      []wingetTestLine{{Arch: "x86_64", Ready: true}},
		},
		{
			Semver:  "1.0.0",
			Rollout: 100, // 全量 -> 包含
			Lines:   []wingetTestLine{{Arch: "x86_64", Ready: true}},
		},
	}

	cat := newWinGetCatalog(t, "aidemo", vers)
	deps := &Deps{Updates: update.NewService(&fakeLoader{cat: cat})}

	resp, err := adapter.Render(context.Background(), deps, Request{
		Project: p,
		Listing: listing,
		Path:    "packageManifests/Contoso.AiDemo",
	})
	if err != nil {
		t.Fatalf("render: %v", err)
	}

	var res wingetSingleManifestResponse
	if err := json.Unmarshal(resp.Body, &res); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if len(res.Data.Versions) != 2 {
		t.Fatalf("Versions count = %d, want 2 (1.1.0 and 1.0.0)", len(res.Data.Versions))
	}
	if res.Data.Versions[0].PackageVersion != "1.1.0" || res.Data.Versions[1].PackageVersion != "1.0.0" {
		t.Fatalf("Versions = %+v", res.Data.Versions)
	}
}

// TestWinGet_DefaultHwVariantOnly 验证仅包含默认硬件变体（C22-7）。
func TestWinGet_DefaultHwVariantOnly(t *testing.T) {
	adapter := NewWinGetAdapter()
	p := &model.Project{
		Slug: "aidemo",
	}
	listing := testListing("winget", map[string]string{"package_identifier": "Contoso.AiDemo"})

	hwRevNonDefault := "rev2"
	vers := []wingetTestVer{
		{
			Semver:  "1.0.0",
			Rollout: 100,
			Lines: []wingetTestLine{
				{Arch: "x86_64", HwRev: &hwRevNonDefault, Ready: true}, // 非默认变体 -> 不收录
			},
		},
	}

	cat := newWinGetCatalog(t, "aidemo", vers)
	deps := &Deps{Updates: update.NewService(&fakeLoader{cat: cat})}

	_, err := adapter.Render(context.Background(), deps, Request{
		Project: p,
		Listing: listing,
		Path:    "packageManifests/Contoso.AiDemo",
	})
	if !errors.Is(err, ErrNoRelease) {
		t.Fatalf("err = %v, want ErrNoRelease", err)
	}
}

// TestWinGet_UnknownPathReturnsErrUnknownPath 非标准路径返回 ErrUnknownPath。
func TestWinGet_UnknownPathReturnsErrUnknownPath(t *testing.T) {
	adapter := NewWinGetAdapter()
	p := &model.Project{
		Slug: "aidemo",
	}
	listing := testListing("winget", nil)

	cat := newWinGetCatalog(t, "aidemo", nil)
	deps := &Deps{Updates: update.NewService(&fakeLoader{cat: cat})}

	badPaths := []string{"latest.json", "RELEASES", "index.xml", "foo.zsync", ""}
	for _, bp := range badPaths {
		_, err := adapter.Render(context.Background(), deps, Request{
			Project: p,
			Listing: listing,
			Path:    bp,
		})
		if !errors.Is(err, ErrUnknownPath) {
			t.Fatalf("path %q err = %v, want ErrUnknownPath", bp, err)
		}
	}
}

// TestWinGet_WrongPackageIdentifier404 指定不匹配的 PackageIdentifier 返回 ErrNoRelease。
func TestWinGet_WrongPackageIdentifier404(t *testing.T) {
	adapter := NewWinGetAdapter()
	p := &model.Project{
		Slug: "aidemo",
	}
	listing := testListing("winget", map[string]string{"package_identifier": "Contoso.AiDemo"})

	vers := []wingetTestVer{
		{
			Semver:  "1.0.0",
			Rollout: 100,
			Lines:   []wingetTestLine{{Arch: "x86_64", Ready: true}},
		},
	}

	cat := newWinGetCatalog(t, "aidemo", vers)
	deps := &Deps{Updates: update.NewService(&fakeLoader{cat: cat})}

	_, err := adapter.Render(context.Background(), deps, Request{
		Project: p,
		Listing: listing,
		Path:    "packageManifests/Wrong.PackageId",
	})
	if !errors.Is(err, ErrNoRelease) {
		t.Fatalf("err = %v, want ErrNoRelease", err)
	}
}

// TestWinGet_EnabledFlag 验证协议开关。
func TestWinGet_EnabledFlag(t *testing.T) {
	adapter := NewWinGetAdapter()
	if adapter.Enabled(nil) {
		t.Fatal("nil project must not be enabled")
	}
	if !adapter.Enabled(&model.Project{}) {
		t.Fatal("non-nil project must be enabled (listing is the HTTP gate)")
	}
}
