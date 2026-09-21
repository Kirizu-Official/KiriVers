package store

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/Kirizu-Official/KiriVers/internal/model"
	"github.com/Kirizu-Official/KiriVers/internal/service/update"
	"github.com/google/uuid"
)

// ---------- F-Droid (index-v1 / index-v2 / entry.json) 单元测试（§9 F-Droid 行 / §9.2，C24-1..C24-7） ----------

// fdroidVer 是用于构造 F-Droid 测试目录的选项。
type fdroidVer struct {
	Semver     string
	VersionInt int64
	Rollout    int
	IsCritical bool
	FileName   string
	FileBytes  []byte
	HwRev      *string
	Ready      bool
	OS         string
	Arch       string
}

// newFDroidCatalog 构造支持多 OS / 多架构切片的测试目录。
func newFDroidCatalog(t *testing.T, slug string, vers []fdroidVer) (*update.Catalog, map[string][]byte) {
	t.Helper()
	cat := &update.Catalog{
		Project: newTestProject(slug),
		Channels: []update.ChannelInfo{
			{Slug: "stable", StabilityRank: 30, Enabled: true},
			{Slug: "beta", StabilityRank: 20, Enabled: true},
		},
		Matrix:    &update.MatrixInfo{OS: "android", Arch: "arm64", PackageType: model.PackageTypeSingleFile},
		EnabledOS: []string{"android", "windows"},
		Versions:  make([]update.VersionState, 0, len(vers)),
	}

	storageData := make(map[string][]byte)

	for _, vInfo := range vers {
		vID := uuid.New()
		lineID := uuid.New()
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

		osName := vInfo.OS
		if osName == "" {
			osName = "android"
		}
		archName := vInfo.Arch
		if archName == "" {
			archName = "arm64"
		}

		lineStatus := model.VersionLineStatusReady
		if !vInfo.Ready {
			lineStatus = model.VersionLineStatusPending
		}

		line := update.LineState{
			ID:       lineID,
			OS:       osName,
			Arch:     archName,
			Status:   lineStatus,
			FullPkgs: []update.ArtifactInfo{},
		}
		if lineStatus == model.VersionLineStatusReady {
			line.PacksReadyAt = packsReady()
		}

		if vInfo.Ready {
			fileName := vInfo.FileName
			if fileName == "" {
				fileName = slug + "-" + vInfo.Semver + ".apk"
			}
			fileBytes := vInfo.FileBytes
			if fileBytes == nil {
				fileBytes = []byte("apk-payload-for-" + fileName)
			}
			h256 := sha256.Sum256(fileBytes)
			sha256Hex := hex.EncodeToString(h256[:])
			storageKey := slug + "/" + sha256Hex
			storageData[storageKey] = fileBytes
			line.FullPkgs = append(line.FullPkgs, update.ArtifactInfo{
				FileName:    fileName,
				Size:        int64(len(fileBytes)),
				SHA256:      sha256Hex,
				ContentType: "application/vnd.android.package-archive",
				StorageKey:  storageKey,
				HwRev:       vInfo.HwRev,
			})
		}

		cat.Versions = append(cat.Versions, update.VersionState{
			Version: v,
			Lines:   []update.LineState{line},
		})
	}

	return cat, storageData
}

// TestFDroid_IndexV1FixtureAndPackageName 验收项 1：最小 APK fixture 出现在 v1 索引中且包名来自项目配置。
func TestFDroid_IndexV1FixtureAndPackageName(t *testing.T) {
	cat, storageData := newFDroidCatalog(t, "androiddemo", []fdroidVer{
		{
			Semver:     "1.2.3",
			VersionInt: 123,
			Rollout:    100,
			Ready:      true,
			FileName:   "androiddemo-1.2.3.apk",
			OS:         "android",
			Arch:       "arm64",
		},
	})
	storage := newFakeStorage(storageData)

	adapter := NewFDroidAdapter()
	deps := &Deps{Updates: update.NewService(&fakeLoader{cat: cat}), Storage: storage}
	req := Request{
		Project: &model.Project{
			ID:   cat.Project.ID,
			Slug: cat.Project.Slug,
		},
		Listing: testListing("fdroid", map[string]string{
			"package_name": "org.example.androiddemo",
			"name":         "Android Demo App",
		}),
		Channel: "stable",
		Path:    "index-v1.json",
		Arch:    "arm64",
	}

	resp, err := adapter.Render(context.Background(), deps, req)
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if resp.Status != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.Status)
	}
	if resp.ContentType != "application/json; charset=utf-8" {
		t.Fatalf("content type = %q, want application/json; charset=utf-8", resp.ContentType)
	}

	var doc fdroidV1Response
	if err := json.Unmarshal(resp.Body, &doc); err != nil {
		t.Fatalf("unmarshal index-v1: %v\nbody:\n%s", err, resp.Body)
	}

	// 1. Repo 校验
	if doc.Repo.Name != "androiddemo Repository" {
		t.Fatalf("repo.name = %q, want androiddemo Repository", doc.Repo.Name)
	}

	// 2. Apps 校验（包名必须来自 Identifiers）
	if len(doc.Apps) != 1 {
		t.Fatalf("len(apps) = %d, want 1", len(doc.Apps))
	}
	if doc.Apps[0].PackageName != "org.example.androiddemo" {
		t.Fatalf("app packageName = %q, want org.example.androiddemo", doc.Apps[0].PackageName)
	}
	if doc.Apps[0].Name != "Android Demo App" {
		t.Fatalf("app name = %q, want Android Demo App", doc.Apps[0].Name)
	}

	// 3. Packages 校验
	pkgList, ok := doc.Packages["org.example.androiddemo"]
	if !ok || len(pkgList) != 1 {
		t.Fatalf("packages for org.example.androiddemo missing or empty: %v", doc.Packages)
	}
	p := pkgList[0]
	if p.VersionName != "1.2.3" {
		t.Fatalf("versionName = %q, want 1.2.3", p.VersionName)
	}
	if p.VersionCode != 123 {
		t.Fatalf("versionCode = %d, want 123", p.VersionCode)
	}
	if p.HashType != "sha256" {
		t.Fatalf("hashType = %q, want sha256", p.HashType)
	}
	if len(p.NativeCode) == 0 || p.NativeCode[0] != "arm64-v8a" {
		t.Fatalf("nativecode = %v, want [arm64-v8a]", p.NativeCode)
	}
}

// TestFDroid_IndexV2FixtureAndPackageName 验收项 1：最小 APK fixture 出现在 v2 索引中且包名来自项目配置。
func TestFDroid_IndexV2FixtureAndPackageName(t *testing.T) {
	cat, storageData := newFDroidCatalog(t, "androiddemo", []fdroidVer{
		{
			Semver:     "1.2.3",
			VersionInt: 123,
			Rollout:    100,
			Ready:      true,
			FileName:   "androiddemo-1.2.3.apk",
			OS:         "android",
			Arch:       "arm64",
		},
	})
	storage := newFakeStorage(storageData)

	adapter := NewFDroidAdapter()
	deps := &Deps{Updates: update.NewService(&fakeLoader{cat: cat}), Storage: storage}
	req := Request{
		Project: &model.Project{
			ID:   cat.Project.ID,
			Slug: cat.Project.Slug,
		},
		Listing: testListing("fdroid", map[string]string{
			"package_name": "org.example.androiddemo",
			"name":         "Android Demo App",
		}),
		Channel: "stable",
		Path:    "index-v2.json",
		Arch:    "arm64",
	}

	resp, err := adapter.Render(context.Background(), deps, req)
	if err != nil {
		t.Fatalf("render: %v", err)
	}

	var doc fdroidV2Response
	if err := json.Unmarshal(resp.Body, &doc); err != nil {
		t.Fatalf("unmarshal index-v2: %v\nbody:\n%s", err, resp.Body)
	}

	// 1. Repo 校验
	if doc.Repo.Name["en-US"] != "androiddemo Repository" {
		t.Fatalf("repo.name = %v, want androiddemo Repository", doc.Repo.Name)
	}

	// 2. Packages 校验
	pkgData, ok := doc.Packages["org.example.androiddemo"]
	if !ok {
		t.Fatalf("package org.example.androiddemo missing in packages map: %v", doc.Packages)
	}
	if pkgData.Metadata.Name["en-US"] != "Android Demo App" {
		t.Fatalf("metadata name = %v, want Android Demo App", pkgData.Metadata.Name)
	}

	// 3. Versions 校验
	if len(pkgData.Versions) != 1 {
		t.Fatalf("len(versions) = %d, want 1", len(pkgData.Versions))
	}
	for _, verDetail := range pkgData.Versions {
		if verDetail.Manifest.VersionName != "1.2.3" {
			t.Fatalf("versionName = %q, want 1.2.3", verDetail.Manifest.VersionName)
		}
		if verDetail.Manifest.VersionCode != 123 {
			t.Fatalf("versionCode = %d, want 123", verDetail.Manifest.VersionCode)
		}
		if len(verDetail.Manifest.NativeCode) == 0 || verDetail.Manifest.NativeCode[0] != "arm64-v8a" {
			t.Fatalf("nativecode = %v, want [arm64-v8a]", verDetail.Manifest.NativeCode)
		}
	}
}

// TestFDroid_EntryJSON 验证 entry.json 端点指向 index-v2.json 且包含正确的 sha256 与 size。
func TestFDroid_EntryJSON(t *testing.T) {
	cat, storageData := newFDroidCatalog(t, "androiddemo", []fdroidVer{
		{
			Semver:     "1.0.0",
			VersionInt: 100,
			Rollout:    100,
			Ready:      true,
			FileName:   "app.apk",
			OS:         "android",
			Arch:       "arm64",
		},
	})
	storage := newFakeStorage(storageData)

	adapter := NewFDroidAdapter()
	deps := &Deps{Updates: update.NewService(&fakeLoader{cat: cat}), Storage: storage}
	req := Request{
		Project: &model.Project{
			ID:   cat.Project.ID,
			Slug: cat.Project.Slug,
		},
		Channel: "stable",
		Path:    "entry.json",
		Arch:    "arm64",
	}

	resp, err := adapter.Render(context.Background(), deps, req)
	if err != nil {
		t.Fatalf("render: %v", err)
	}

	var entryDoc fdroidEntryResponse
	if err := json.Unmarshal(resp.Body, &entryDoc); err != nil {
		t.Fatalf("unmarshal entry.json: %v", err)
	}
	if entryDoc.Index.Name != "/index-v2.json" {
		t.Fatalf("index.name = %q, want /index-v2.json", entryDoc.Index.Name)
	}
	if entryDoc.Index.SHA256 == "" || len(entryDoc.Index.SHA256) != 64 {
		t.Fatalf("invalid sha256: %q", entryDoc.Index.SHA256)
	}
	if entryDoc.Index.Size <= 0 {
		t.Fatalf("invalid size: %d", entryDoc.Index.Size)
	}
}

// TestFDroid_WindowsLineExcluded 验收项 2：windows 多文件 Line 不出现在 F-Droid 索引。
func TestFDroid_WindowsLineExcluded(t *testing.T) {
	// 版本 1 包含 Android apk 与 Windows zip；版本 2 只有 Windows zip
	cat, storageData := newFDroidCatalog(t, "multios", []fdroidVer{
		{
			Semver:     "1.0.0",
			VersionInt: 10,
			Rollout:    100,
			Ready:      true,
			FileName:   "multios-1.0.0.apk",
			OS:         "android",
			Arch:       "arm64",
		},
		{
			Semver:     "1.0.0",
			VersionInt: 10,
			Rollout:    100,
			Ready:      true,
			FileName:   "multios-1.0.0-win.zip",
			OS:         "windows",
			Arch:       "x86_64",
		},
		{
			Semver:     "2.0.0",
			VersionInt: 20,
			Rollout:    100,
			Ready:      true,
			FileName:   "multios-2.0.0-win.zip",
			OS:         "windows",
			Arch:       "x86_64",
		},
	})
	storage := newFakeStorage(storageData)

	adapter := NewFDroidAdapter()
	deps := &Deps{Updates: update.NewService(&fakeLoader{cat: cat}), Storage: storage}
	req := Request{
		Project: &model.Project{
			ID:   cat.Project.ID,
			Slug: cat.Project.Slug,
		},
		Channel: "stable",
		Path:    "index-v1.json",
	}

	resp, err := adapter.Render(context.Background(), deps, req)
	if err != nil {
		t.Fatalf("render: %v", err)
	}

	var doc fdroidV1Response
	if err := json.Unmarshal(resp.Body, &doc); err != nil {
		t.Fatalf("unmarshal index-v1: %v", err)
	}

	pkgs := doc.Packages["multios"]
	// 索引中必须只有 1 个包，即 1.0.0 的 Android APK；2.0.0 的 Windows 产物绝不能出现！
	if len(pkgs) != 1 {
		t.Fatalf("len(pkgs) = %d, want 1 (windows line must not appear)", len(pkgs))
	}
	if pkgs[0].VersionName != "1.0.0" {
		t.Fatalf("versionName = %q, want 1.0.0", pkgs[0].VersionName)
	}
	if !strings.Contains(pkgs[0].ApkName, "/packages/") {
		t.Fatalf("apkName = %q, want hash package URL", pkgs[0].ApkName)
	}
}

// TestFDroid_NonAPKExcluded 验证非 apk 产物（如 android 平台下的 tar.gz）不进入索引。
func TestFDroid_NonAPKExcluded(t *testing.T) {
	cat, storageData := newFDroidCatalog(t, "nonapk", []fdroidVer{
		{
			Semver:     "1.0.0",
			VersionInt: 10,
			Rollout:    100,
			Ready:      true,
			FileName:   "android-rootfs.tar.gz", // 非 apk！
			OS:         "android",
			Arch:       "arm64",
		},
	})
	storage := newFakeStorage(storageData)

	adapter := NewFDroidAdapter()
	deps := &Deps{Updates: update.NewService(&fakeLoader{cat: cat}), Storage: storage}
	req := Request{
		Project: &model.Project{
			ID:   cat.Project.ID,
			Slug: cat.Project.Slug,
		},
		Channel: "stable",
		Path:    "index-v1.json",
		Arch:    "arm64",
	}

	// 当无任何 .apk 产物时返回 ErrNoRelease
	_, err := adapter.Render(context.Background(), deps, req)
	if err != ErrNoRelease {
		t.Fatalf("err = %v, want ErrNoRelease", err)
	}
}

// TestFDroid_DualNumberMapping 验证双号映射（C24-3）。
func TestFDroid_DualNumberMapping(t *testing.T) {
	sem1 := "2.1.4"
	int1 := int64(20104)
	v1 := &model.Version{
		VersionSemverCanonical: &sem1,
		VersionInteger:         &int1,
	}
	if code := fdroidVersionCode(v1); code != 20104 {
		t.Fatalf("versionCode = %d, want 20104", code)
	}
	if name := fdroidVersionName(v1); name != "2.1.4" {
		t.Fatalf("versionName = %q, want 2.1.4", name)
	}

	// 仅有 semver 时推算 versionCode
	sem2 := "1.2.3"
	v2 := &model.Version{
		VersionSemverCanonical: &sem2,
	}
	if code := fdroidVersionCode(v2); code != 10203 {
		t.Fatalf("versionCode = %d, want 10203", code)
	}

	// 仅有 version_integer 时 versionName 回退
	int3 := int64(42)
	v3 := &model.Version{
		VersionInteger: &int3,
	}
	if code := fdroidVersionCode(v3); code != 42 {
		t.Fatalf("versionCode = %d, want 42", code)
	}
	if name := fdroidVersionName(v3); name != "42" {
		t.Fatalf("versionName = %q, want 42", name)
	}
}

// TestFDroid_ABIMapping 验证架构转译为 Android ABIs。
func TestFDroid_ABIMapping(t *testing.T) {
	tests := []struct {
		arch string
		want string
	}{
		{"arm64", "arm64-v8a"},
		{"aarch64", "arm64-v8a"},
		{"arm", "armeabi-v7a"},
		{"armv7", "armeabi-v7a"},
		{"x86_64", "x86_64"},
		{"amd64", "x86_64"},
		{"x86", "x86"},
	}
	for _, tc := range tests {
		abis := fdroidNativeCode(tc.arch)
		if len(abis) == 0 || abis[0] != tc.want {
			t.Errorf("fdroidNativeCode(%q) = %v, want %q", tc.arch, abis, tc.want)
		}
	}
}

// TestFDroid_GrayRolloutAndCritical 验证灰度过滤与关键版本豁免。
func TestFDroid_GrayRolloutAndCritical(t *testing.T) {
	// v2 灰度 50%（非关键，不可见）；v1 灰度 50% 且关键（可见）
	cat, storageData := newFDroidCatalog(t, "grayapp", []fdroidVer{
		{
			Semver:     "2.0.0",
			VersionInt: 20,
			Rollout:    50,
			IsCritical: false,
			Ready:      true,
			FileName:   "grayapp-2.0.0.apk",
		},
		{
			Semver:     "1.0.0",
			VersionInt: 10,
			Rollout:    50,
			IsCritical: true,
			Ready:      true,
			FileName:   "grayapp-1.0.0.apk",
		},
	})
	storage := newFakeStorage(storageData)

	adapter := NewFDroidAdapter()
	deps := &Deps{Updates: update.NewService(&fakeLoader{cat: cat}), Storage: storage}
	req := Request{
		Project: &model.Project{
			ID:   cat.Project.ID,
			Slug: cat.Project.Slug,
		},
		Channel: "stable",
		Path:    "index-v1.json",
		Arch:    "arm64",
	}

	resp, err := adapter.Render(context.Background(), deps, req)
	if err != nil {
		t.Fatalf("render: %v", err)
	}

	var doc fdroidV1Response
	if err := json.Unmarshal(resp.Body, &doc); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	pkgs := doc.Packages["grayapp"]
	if len(pkgs) != 1 || pkgs[0].VersionName != "1.0.0" {
		t.Fatalf("pkgs = %v, want only 1.0.0", pkgs)
	}
}

// TestFDroid_DefaultHwVariantOnly 验证仅包含默认硬件变体产物。
func TestFDroid_DefaultHwVariantOnly(t *testing.T) {
	hwRev := "rev-b"
	cat, storageData := newFDroidCatalog(t, "hwapp", []fdroidVer{
		{
			Semver:     "2.0.0",
			VersionInt: 20,
			Rollout:    100,
			Ready:      true,
			FileName:   "hwapp-2.0.0-revb.apk",
			HwRev:      &hwRev, // 非默认变体
		},
		{
			Semver:     "1.0.0",
			VersionInt: 10,
			Rollout:    100,
			Ready:      true,
			FileName:   "hwapp-1.0.0.apk", // 默认变体
		},
	})
	storage := newFakeStorage(storageData)

	adapter := NewFDroidAdapter()
	deps := &Deps{Updates: update.NewService(&fakeLoader{cat: cat}), Storage: storage}
	req := Request{
		Project: &model.Project{
			ID:   cat.Project.ID,
			Slug: cat.Project.Slug,
		},
		Channel: "stable",
		Path:    "index-v1.json",
		Arch:    "arm64",
	}

	resp, err := adapter.Render(context.Background(), deps, req)
	if err != nil {
		t.Fatalf("render: %v", err)
	}

	var doc fdroidV1Response
	if err := json.Unmarshal(resp.Body, &doc); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	pkgs := doc.Packages["hwapp"]
	if len(pkgs) != 1 || pkgs[0].VersionName != "1.0.0" {
		t.Fatalf("pkgs = %v, want only 1.0.0", pkgs)
	}
}

// TestFDroid_UnknownPathReturnsErrUnknownPath 验证未知路径返回 ErrUnknownPath。
func TestFDroid_UnknownPathReturnsErrUnknownPath(t *testing.T) {
	cat, storageData := newFDroidCatalog(t, "fdroiddemo", []fdroidVer{
		{Semver: "1.0.0", Rollout: 100, Ready: true, FileName: "app.apk"},
	})
	storage := newFakeStorage(storageData)

	adapter := NewFDroidAdapter()
	deps := &Deps{Updates: update.NewService(&fakeLoader{cat: cat}), Storage: storage}

	for _, badPath := range []string{"appcast.xml", "RELEASES", "latest.json", "index.xml"} {
		req := Request{
			Project: &model.Project{ID: cat.Project.ID, Slug: cat.Project.Slug},
			Channel: "stable",
			Path:    badPath,
		}
		_, err := adapter.Render(context.Background(), deps, req)
		if err == nil || !strings.Contains(err.Error(), ErrUnknownPath.Error()) {
			t.Fatalf("path %q err = %v, want ErrUnknownPath", badPath, err)
		}
	}
}

// TestFDroid_EnabledFlag 验证协议开关。
func TestFDroid_EnabledFlag(t *testing.T) {
	adapter := NewFDroidAdapter()
	if adapter.Enabled(nil) {
		t.Fatalf("Enabled(nil) should be false")
	}
	if !adapter.Enabled(&model.Project{}) {
		t.Fatalf("non-nil project must be enabled (listing is the HTTP gate)")
	}
}
