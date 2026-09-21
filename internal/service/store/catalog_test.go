package store

import (
	"strconv"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/Kirizu-Official/KiriVers/internal/model"
	"github.com/Kirizu-Official/KiriVers/internal/service/update"
)

func TestCacheControlLocalProxy(t *testing.T) {
	p := &model.Project{CacheSMaxageSeconds: 60}
	if got := CacheControlFor(p, true); got != "private, no-store" {
		t.Fatalf("local-proxy Cache-Control=%q", got)
	}
	if got := CacheControlFor(p, false); got != "public, s-maxage=60, stale-while-revalidate=30" {
		t.Fatalf("single-node Cache-Control=%q", got)
	}
}

// ---------- 目录快照测试夹具（纯数据，§9 灰度 / 状态 / 变体场景） ----------

var testProjectID = uuid.MustParse("00000000-0000-0000-0000-000000000001")

var testPublishTime = time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)

// newTestProject 返回项目级快照（semver 引擎、默认 stable 渠道表）。
func newTestProject(slug string) update.ProjectInfo {
	return update.ProjectInfo{
		ID:                  testProjectID,
		Slug:                slug,
		CompareEngine:       model.CompareEngineSemver,
		DefaultLocale:       "en",
		CacheSMaxageSeconds: 60,
		SigningAlgo:         model.SigningAlgoEd25519,
	}
}

// versionOpt 是测试版本的可变字段注入器。
type versionOpt func(*model.Version, *update.LineState)

// newTestCatalog 构造单版本目录：channel 渠道、semver/int 双号、
// rollout 灰度、lineStatus 切片状态、hw 变体产物与 min_os。
// 产物默认变体文件名 {slug}-{ver}-macos-x86_64.dmg，存储键含版本号。
func newTestCatalog(t *testing.T, slug, channel, semver string, versionInt int64, rollout int, critical bool, lineStatus string, hwVariants map[string]string, minOS string, opts ...versionOpt) *update.Catalog {
	t.Helper()
	v := model.Version{
		ID:             uuid.New(),
		ProjectID:      testProjectID,
		ChannelSlug:    channel,
		Status:         model.VersionStatusPublished,
		IsCritical:     critical,
		PublishTime:    &testPublishTime,
		Changelog:      model.ChangelogMap{"en": {Title: "v" + semver, Markdown: "changes for " + semver}},
	}
	applyFeedGray(&v, rollout, critical)
	if semver != "" {
		v.VersionSemver = &semver
		v.VersionSemverCanonical = &semver
	}
	if versionInt > 0 {
		v.VersionInteger = &versionInt
	}
	line := update.LineState{
		ID:       uuid.New(),
		OS:       "macos",
		Arch:     "x86_64",
		Status:   lineStatus,
		MinOS:    nil,
		FullPkgs: []update.ArtifactInfo{},
	}
	if lineStatus == model.VersionLineStatusReady {
		line.PacksReadyAt = packsReady()
	}
	if minOS != "" {
		line.MinOS = &minOS
	}
	// 默认变体（HwRev=nil）恒存在；hwVariants 追加非默认变体。
	if lineStatus == model.VersionLineStatusReady {
		line.FullPkgs = append(line.FullPkgs, update.ArtifactInfo{
			FileName:    slug + "-" + displayRef(semver, versionInt) + "-macos-x86_64.dmg",
			Size:        100,
			SHA256:      "aa",
			ContentType: "application/x-apple-diskimage",
			StorageKey:  slug + "/aa",
		})
	}
	for hw := range hwVariants {
		hw := hw
		line.FullPkgs = append(line.FullPkgs, update.ArtifactInfo{
			FileName:    slug + "-" + displayRef(semver, versionInt) + "-macos-x86_64-" + hw + ".dmg",
			Size:        200,
			SHA256:      "bb-" + hw,
			ContentType: "application/x-apple-diskimage",
			StorageKey:  slug + "/bb-" + hw,
			HwRev:       &hw,
		})
	}
	for _, opt := range opts {
		opt(&v, &line)
	}
	return &update.Catalog{
		Project: newTestProject(slug),
		Channels: []update.ChannelInfo{
			{Slug: "stable", StabilityRank: 30, Enabled: true},
			{Slug: "beta", StabilityRank: 20, Enabled: true},
		},
		Matrix:    &update.MatrixInfo{OS: "macos", Arch: "x86_64", PackageType: model.PackageTypeSingleFile},
		EnabledOS: []string{"macos"},
		Versions:  []update.VersionState{{Version: v, Lines: []update.LineState{line}}},
	}
}

// newUUID 生成测试用随机 UUID。
func newUUID(t *testing.T) uuid.UUID {
	t.Helper()
	id, err := uuid.NewRandom()
	if err != nil {
		t.Fatal(err)
	}
	return id
}

// displayRef 返回文件名用版本串（SemVer 优先，否则整数）。
func displayRef(semver string, versionInt int64) string {
	if semver != "" {
		return semver
	}
	return strconv.FormatInt(versionInt, 10)
}

func applyFeedGray(v *model.Version, rollout int, critical bool) {
	if critical || rollout >= 100 {
		v.GrayCompletedAt = &testPublishTime
		v.GrayStartPercent = 100
		return
	}
	v.GrayStartedAt = &testPublishTime
	v.GrayCompletedAt = nil
	v.GrayStartPercent = rollout
}
