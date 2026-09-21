package store

import (
	"context"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/Kirizu-Official/KiriVers/internal/model"
	"github.com/Kirizu-Official/KiriVers/internal/service"
	"github.com/Kirizu-Official/KiriVers/internal/service/update"
)

// ---------- Squirrel RELEASES 单元测试（§9 Squirrel 行 / §9.2，C19-1..C19-6） ----------

// squirrelRegex 是 Squirrel.Windows 官方解析器使用的正则模型：
// ^([0-9a-fA-F]{40})\s+(\S+)\s+(\d+)$
var squirrelRegex = regexp.MustCompile(`^([0-9a-fA-F]{40})\s+(\S+)\s+(\d+)$`)

// squirrelEntry 是 RELEASES 清单单行解析目标。
type squirrelEntry struct {
	SHA1     string
	FileName string
	Size     int64
}

// parseRELEASES 解析并校验 RELEASES 纯文本（验收项 1：符合 Squirrel 解析器契约）。
func parseRELEASES(t *testing.T, body []byte) []squirrelEntry {
	t.Helper()
	lines := strings.Split(string(body), "\n")
	var entries []squirrelEntry
	for i, line := range lines {
		if line == "" {
			// 最后一行末尾换行导致的空串合法；中间不得有空行
			if i != len(lines)-1 {
				t.Fatalf("unexpected empty line at index %d:\n%s", i, body)
			}
			continue
		}
		m := squirrelRegex.FindStringSubmatch(line)
		if m == nil {
			t.Fatalf("line %d %q does not match squirrel regex %s", i, line, squirrelRegex.String())
		}
		sz, err := strconv.ParseInt(m[3], 10, 64)
		if err != nil {
			t.Fatalf("line %d invalid size %q: %v", i, m[3], err)
		}
		entries = append(entries, squirrelEntry{
			SHA1:     m[1],
			FileName: m[2],
			Size:     sz,
		})
	}
	return entries
}

// squirrelVerOpt 是构建测试目录版本的选项。
type squirrelVer struct {
	Semver     string
	VersionInt int64
	Rollout    int
	IsCritical bool
	FileBytes  []byte
	FileName   string
	HwRev      *string
	Ready      bool
}

// newSquirrelMultiVerCatalog 构造含多版本的 windows 目录。
func newSquirrelMultiVerCatalog(t *testing.T, slug, arch string, vers []squirrelVer) (*update.Catalog, map[string][]byte) {
	t.Helper()
	cat := &update.Catalog{
		Project: newTestProject(slug),
		Channels: []update.ChannelInfo{
			{Slug: "stable", StabilityRank: 30, Enabled: true},
			{Slug: "beta", StabilityRank: 20, Enabled: true},
		},
		Matrix:    &update.MatrixInfo{OS: "windows", Arch: arch, PackageType: model.PackageTypeSingleFile},
		EnabledOS: []string{"windows"},
		Versions:  make([]update.VersionState, 0, len(vers)),
	}

	storageData := make(map[string][]byte)

	for _, vInfo := range vers {
		vID := uuid.New()
		lineID := uuid.New()
		v := model.Version{
			ID:             vID,
			ProjectID:      testProjectID,
			ChannelSlug:    "stable",
			Status:         model.VersionStatusPublished,
			GrayStartPercent: 100,
			IsCritical:     vInfo.IsCritical,
			PublishTime:    &testPublishTime,
		}
		applyFeedGray(&v, vInfo.Rollout, vInfo.IsCritical)
		if vInfo.Semver != "" {
			v.VersionSemver = &vInfo.Semver
			v.VersionSemverCanonical = &vInfo.Semver
		}
		if vInfo.VersionInt > 0 {
			v.VersionInteger = &vInfo.VersionInt
		}

		lineStatus := model.VersionLineStatusReady
		if !vInfo.Ready {
			lineStatus = model.VersionLineStatusPending
		}

		line := update.LineState{
			ID:       lineID,
			OS:       "windows",
			Arch:     arch,
			Status:   lineStatus,
			FullPkgs: []update.ArtifactInfo{},
		}
		if lineStatus == model.VersionLineStatusReady {
			line.PacksReadyAt = packsReady()
		}

		if vInfo.Ready {
			fileName := vInfo.FileName
			if fileName == "" {
				fileName = slug + "-" + displayRef(vInfo.Semver, vInfo.VersionInt) + "-full.nupkg"
			}
			fileBytes := vInfo.FileBytes
			if fileBytes == nil {
				fileBytes = []byte("default-bytes-for-" + fileName)
			}
			h256 := sha256.Sum256(fileBytes)
			sha256Hex := hex.EncodeToString(h256[:])
			storageKey := slug + "/" + sha256Hex
			storageData[storageKey] = fileBytes
			line.FullPkgs = append(line.FullPkgs, update.ArtifactInfo{
				FileName:    fileName,
				Size:        int64(len(fileBytes)),
				SHA256:      sha256Hex,
				ContentType: "application/octet-stream",
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

// TestSquirrelRELEASES_LineFormatAndFixture 验收项 1：RELEASES 行格式符合 Squirrel 解析器契约。
func TestSquirrelRELEASES_LineFormatAndFixture(t *testing.T) {
	fileContent := []byte("squirrel-test-nupkg-content-v1.0.0")
	cat, storageData := newSquirrelMultiVerCatalog(t, "sdemo", "x86_64", []squirrelVer{
		{Semver: "1.0.0", Rollout: 100, Ready: true, FileBytes: fileContent},
	})
	storage := newFakeStorage(storageData)

	adapter := NewSquirrelAdapter()
	deps := &Deps{Updates: update.NewService(&fakeLoader{cat: cat}), Storage: storage}
	req := Request{
		Project: &model.Project{ID: cat.Project.ID, Slug: cat.Project.Slug},
		Channel: "stable",
		Path:    "RELEASES",
		Arch:    "x86_64",
	}

	resp, err := adapter.Render(context.Background(), deps, req)
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if resp.Status != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.Status)
	}
	if resp.ContentType != "text/plain; charset=utf-8" {
		t.Fatalf("content type = %q, want text/plain; charset=utf-8", resp.ContentType)
	}

	entries := parseRELEASES(t, resp.Body)
	if len(entries) != 1 {
		t.Fatalf("entries = %d, want 1", len(entries))
	}

	expectedSHA1 := sha1.Sum(fileContent)
	expectedSHA1Hex := hex.EncodeToString(expectedSHA1[:])
	e := entries[0]
	if e.SHA1 != expectedSHA1Hex {
		t.Fatalf("sha1 = %q, want %q", e.SHA1, expectedSHA1Hex)
	}
	if e.FileName != service.DecoratedPackageName(cat.Versions[0].Lines[0].FullPkgs[0].SHA256, cat.Versions[0].Lines[0].FullPkgs[0].FileName) {
		t.Fatalf("filename = %q, want decorated sha256 name", e.FileName)
	}
	if e.Size != int64(len(fileContent)) {
		t.Fatalf("size = %d, want %d", e.Size, len(fileContent))
	}

	// 验证末尾换行且格式为严格三个 token 由单空格分隔
	wantName := service.DecoratedPackageName(cat.Versions[0].Lines[0].FullPkgs[0].SHA256, cat.Versions[0].Lines[0].FullPkgs[0].FileName)
	wantLine := expectedSHA1Hex + " " + wantName + " " + strconv.Itoa(len(fileContent)) + "\n"
	if string(resp.Body) != wantLine {
		t.Fatalf("body = %q, want %q", string(resp.Body), wantLine)
	}
}

// TestSquirrelRELEASES_SHA1ComputationAndLRU 独立验证 SHA-1 流式计算，且 LRU 缓存第二次不再读存储。
func TestSquirrelRELEASES_SHA1ComputationAndLRU(t *testing.T) {
	fileContent := []byte("squirrel-artifact-bytes-for-sha1-test")
	cat, storageData := newSquirrelMultiVerCatalog(t, "sdemo", "x86_64", []squirrelVer{
		{Semver: "1.0.0", Rollout: 100, Ready: true, FileBytes: fileContent},
	})
	storage := newFakeStorage(storageData)

	adapter := NewSquirrelAdapter()
	deps := &Deps{Updates: update.NewService(&fakeLoader{cat: cat}), Storage: storage}
	req := Request{
		Project: &model.Project{ID: cat.Project.ID, Slug: cat.Project.Slug},
		Channel: "stable",
		Path:    "RELEASES",
		Arch:    "x86_64",
	}

	resp1, err := adapter.Render(context.Background(), deps, req)
	if err != nil {
		t.Fatalf("render 1: %v", err)
	}
	key := cat.Versions[0].Lines[0].FullPkgs[0].StorageKey
	if storage.readCount(key) != 1 {
		t.Fatalf("storage read count = %d, want 1", storage.readCount(key))
	}

	resp2, err := adapter.Render(context.Background(), deps, req)
	if err != nil {
		t.Fatalf("render 2: %v", err)
	}
	// 第二次渲染命中 LRU 缓存，存储读取计数仍为 1
	if storage.readCount(key) != 1 {
		t.Fatalf("storage read count after second render = %d, want 1", storage.readCount(key))
	}
	if string(resp1.Body) != string(resp2.Body) {
		t.Fatalf("body mismatch across renders:\n%s\nvs\n%s", resp1.Body, resp2.Body)
	}
}

// TestSquirrelRELEASES_OrderingNewestToOldest 验证 RELEASES 多版本行序从最新到最旧（新→旧）。
func TestSquirrelRELEASES_OrderingNewestToOldest(t *testing.T) {
	cat, storageData := newSquirrelMultiVerCatalog(t, "sdemo", "x86_64", []squirrelVer{
		{Semver: "1.0.0", Rollout: 100, Ready: true, FileBytes: []byte("v1.0.0")},
		{Semver: "1.2.0", Rollout: 100, Ready: true, FileBytes: []byte("v1.2.0")},
		{Semver: "2.0.0", Rollout: 100, Ready: true, FileBytes: []byte("v2.0.0")},
	})
	storage := newFakeStorage(storageData)

	adapter := NewSquirrelAdapter()
	deps := &Deps{Updates: update.NewService(&fakeLoader{cat: cat}), Storage: storage}
	req := Request{
		Project: &model.Project{ID: cat.Project.ID, Slug: cat.Project.Slug},
		Channel: "stable",
		Path:    "RELEASES",
		Arch:    "x86_64",
	}

	resp, err := adapter.Render(context.Background(), deps, req)
	if err != nil {
		t.Fatalf("render: %v", err)
	}

	entries := parseRELEASES(t, resp.Body)
	if len(entries) != 3 {
		t.Fatalf("entries = %d, want 3", len(entries))
	}

	// 新→旧排序：2.0.0 在前，其次 1.2.0，最后 1.0.0
	if !strings.HasSuffix(entries[0].FileName, ".nupkg") || len(strings.TrimSuffix(entries[0].FileName, ".nupkg")) != 64 {
		t.Fatalf("entry 0 = %q, want {sha256}.nupkg", entries[0].FileName)
	}
	if !strings.HasSuffix(entries[1].FileName, ".nupkg") || len(strings.TrimSuffix(entries[1].FileName, ".nupkg")) != 64 {
		t.Fatalf("entry 1 = %q, want {sha256}.nupkg", entries[1].FileName)
	}
	if !strings.HasSuffix(entries[2].FileName, ".nupkg") || len(strings.TrimSuffix(entries[2].FileName, ".nupkg")) != 64 {
		t.Fatalf("entry 2 = %q, want {sha256}.nupkg", entries[2].FileName)
	}
}

// TestSquirrelRELEASES_GrayRolloutAndCritical 验证灰度投影：<100% 不可见，100% 可见，关键版本即使 <100% 也可见。
func TestSquirrelRELEASES_GrayRolloutAndCritical(t *testing.T) {
	cat, storageData := newSquirrelMultiVerCatalog(t, "sdemo", "x86_64", []squirrelVer{
		{Semver: "1.0.0", Rollout: 100, IsCritical: false, Ready: true, FileBytes: []byte("v1.0.0")},
		{Semver: "1.1.0", Rollout: 50, IsCritical: false, Ready: true, FileBytes: []byte("v1.1.0")},
		{Semver: "1.2.0", Rollout: 20, IsCritical: true, Ready: true, FileBytes: []byte("v1.2.0")},
	})
	storage := newFakeStorage(storageData)

	adapter := NewSquirrelAdapter()
	deps := &Deps{Updates: update.NewService(&fakeLoader{cat: cat}), Storage: storage}
	req := Request{
		Project: &model.Project{ID: cat.Project.ID, Slug: cat.Project.Slug},
		Channel: "stable",
		Path:    "RELEASES",
		Arch:    "x86_64",
	}

	resp, err := adapter.Render(context.Background(), deps, req)
	if err != nil {
		t.Fatalf("render: %v", err)
	}

	entries := parseRELEASES(t, resp.Body)
	// 1.1.0 灰度 50% 且非关键版本，不可见；1.2.0 虽 20% 但关键版本可见；1.0.0 100% 可见
	if len(entries) != 2 {
		t.Fatalf("entries count = %d, want 2", len(entries))
	}
	if !strings.HasSuffix(entries[0].FileName, ".nupkg") || len(strings.TrimSuffix(entries[0].FileName, ".nupkg")) != 64 {
		t.Fatalf("entry 0 = %q, want {sha256}.nupkg", entries[0].FileName)
	}
	if !strings.HasSuffix(entries[1].FileName, ".nupkg") || len(strings.TrimSuffix(entries[1].FileName, ".nupkg")) != 64 {
		t.Fatalf("entry 1 = %q, want {sha256}.nupkg", entries[1].FileName)
	}
}

// TestSquirrelRELEASES_DefaultHwVariantOnly 验证仅默认变体投影（C19-6：Squirrel 无法表达 hw_rev）。
func TestSquirrelRELEASES_DefaultHwVariantOnly(t *testing.T) {
	hwNonDefault := "rev-a"
	cat, storageData := newSquirrelMultiVerCatalog(t, "sdemo", "x86_64", []squirrelVer{
		{Semver: "1.0.0", Rollout: 100, Ready: true, FileBytes: []byte("v1.0.0-default")},
		{Semver: "2.0.0", Rollout: 100, Ready: true, HwRev: &hwNonDefault, FileBytes: []byte("v2.0.0-hw")},
	})
	storage := newFakeStorage(storageData)

	adapter := NewSquirrelAdapter()
	deps := &Deps{Updates: update.NewService(&fakeLoader{cat: cat}), Storage: storage}
	req := Request{
		Project: &model.Project{ID: cat.Project.ID, Slug: cat.Project.Slug},
		Channel: "stable",
		Path:    "RELEASES",
		Arch:    "x86_64",
	}

	resp, err := adapter.Render(context.Background(), deps, req)
	if err != nil {
		t.Fatalf("render: %v", err)
	}

	entries := parseRELEASES(t, resp.Body)
	// 2.0.0 仅有非默认变体，匿名客户端不可见；仅 1.0.0 默认变体可见
	if len(entries) != 1 {
		t.Fatalf("entries = %d, want 1", len(entries))
	}
	if !strings.HasSuffix(entries[0].FileName, ".nupkg") || len(strings.TrimSuffix(entries[0].FileName, ".nupkg")) != 64 {
		t.Fatalf("entry 0 = %q, want {sha256}.nupkg", entries[0].FileName)
	}
}

// TestSquirrelRELEASES_EmptyCatalogReturnsErrNoRelease 验收项 2 衍生：可见集为空时返回 ErrNoRelease（404，不发空清单冒充）。
func TestSquirrelRELEASES_EmptyCatalogReturnsErrNoRelease(t *testing.T) {
	cat, storageData := newSquirrelMultiVerCatalog(t, "sdemo", "x86_64", []squirrelVer{
		{Semver: "1.0.0", Rollout: 50, IsCritical: false, Ready: true}, // 灰度不可见
	})
	storage := newFakeStorage(storageData)

	adapter := NewSquirrelAdapter()
	deps := &Deps{Updates: update.NewService(&fakeLoader{cat: cat}), Storage: storage}
	req := Request{
		Project: &model.Project{ID: cat.Project.ID, Slug: cat.Project.Slug},
		Channel: "stable",
		Path:    "RELEASES",
		Arch:    "x86_64",
	}

	_, err := adapter.Render(context.Background(), deps, req)
	if err != ErrNoRelease {
		t.Fatalf("err = %v, want ErrNoRelease", err)
	}
}

// TestSquirrelRELEASES_UnknownPathReturnsErrUnknownPath 校验路径非 RELEASES 时报 ErrUnknownPath（404）。
func TestSquirrelRELEASES_UnknownPathReturnsErrUnknownPath(t *testing.T) {
	cat, storageData := newSquirrelMultiVerCatalog(t, "sdemo", "x86_64", []squirrelVer{
		{Semver: "1.0.0", Rollout: 100, Ready: true},
	})
	storage := newFakeStorage(storageData)

	adapter := NewSquirrelAdapter()
	deps := &Deps{Updates: update.NewService(&fakeLoader{cat: cat}), Storage: storage}

	for _, badPath := range []string{"releases", "RELEASES.txt", "latest.json", "foo/bar"} {
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

// TestSquirrelRELEASES_UnknownChannelReturnsErrUnknownChannel 校验未知渠道报 ErrUnknownChannel（400）。
func TestSquirrelRELEASES_UnknownChannelReturnsErrUnknownChannel(t *testing.T) {
	cat, storageData := newSquirrelMultiVerCatalog(t, "sdemo", "x86_64", []squirrelVer{
		{Semver: "1.0.0", Rollout: 100, Ready: true},
	})
	storage := newFakeStorage(storageData)

	adapter := NewSquirrelAdapter()
	deps := &Deps{Updates: update.NewService(&fakeLoader{cat: cat}), Storage: storage}
	req := Request{
		Project: &model.Project{ID: cat.Project.ID, Slug: cat.Project.Slug},
		Channel: "nonexistent",
		Path:    "RELEASES",
		Arch:    "x86_64",
	}

	_, err := adapter.Render(context.Background(), deps, req)
	if err == nil || !strings.Contains(err.Error(), ErrUnknownChannel.Error()) {
		t.Fatalf("err = %v, want ErrUnknownChannel", err)
	}
}

// TestSquirrelRELEASES_DefaultArchX86_64 验证请求省略 arch query 时默认采用 x86_64。
func TestSquirrelRELEASES_DefaultArchX86_64(t *testing.T) {
	cat, storageData := newSquirrelMultiVerCatalog(t, "sdemo", "x86_64", []squirrelVer{
		{Semver: "1.0.0", Rollout: 100, Ready: true, FileBytes: []byte("v1.0.0")},
	})
	storage := newFakeStorage(storageData)

	adapter := NewSquirrelAdapter()
	deps := &Deps{Updates: update.NewService(&fakeLoader{cat: cat}), Storage: storage}
	req := Request{
		Project: &model.Project{ID: cat.Project.ID, Slug: cat.Project.Slug},
		Channel: "stable",
		Path:    "RELEASES",
		Arch:    "", // 省略 arch
	}

	resp, err := adapter.Render(context.Background(), deps, req)
	if err != nil {
		t.Fatalf("render with empty arch: %v", err)
	}
	entries := parseRELEASES(t, resp.Body)
	if len(entries) != 1 {
		t.Fatalf("entries = %d, want 1", len(entries))
	}
}

// TestSquirrelRELEASES_EnabledFlag 验收项 2：开关控制与包标识配置（C19-2）。
func TestSquirrelRELEASES_EnabledFlag(t *testing.T) {
	adapter := NewSquirrelAdapter()
	if adapter.Enabled(nil) {
		t.Fatal("nil project must not be enabled")
	}
	if !adapter.Enabled(&model.Project{}) {
		t.Fatal("non-nil project must be enabled (listing is the HTTP gate)")
	}
}
