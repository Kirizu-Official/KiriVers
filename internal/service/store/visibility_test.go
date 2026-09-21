package store

import (
	"testing"

	"github.com/Kirizu-Official/KiriVers/internal/model"
	"github.com/Kirizu-Official/KiriVers/internal/service/update"
)

// TestAnonymousVisibleGrayAndStatus 匿名可见性口径（§4.4 末条 / §9 / C16-5）：
// 灰度 <100% 的非强制版本不可见；100% / 关键版本可见；draft / revoked /
// yanked / 仅非默认变体 / 其他渠道一律不可见；输出按比较键新→旧。
func TestAnonymousVisibleGrayAndStatus(t *testing.T) {
	cat := &update.Catalog{
		Project: newTestProject("demo"),
		Channels: []update.ChannelInfo{
			{Slug: "stable", StabilityRank: 30, Enabled: true},
			{Slug: "beta", StabilityRank: 20, Enabled: true},
		},
		Matrix: &update.MatrixInfo{OS: "macos", Arch: "x86_64"},
	}

	// 辅助：追加一个版本状态，返回 slug 化标记便于断言。
	add := func(semver string, vInt int64, status string, rollout int, critical bool, channel string, lineStatus string, hwOnly bool) {
		v := model.Version{
			ID:          newUUID(t),
			ChannelSlug: channel,
			Status:      status,
			IsCritical:  critical,
		}
		applyFeedGray(&v, rollout, critical)
		if semver != "" {
			v.VersionSemverCanonical = &semver
		}
		if vInt > 0 {
			v.VersionInteger = &vInt
		}
		line := update.LineState{OS: "macos", Arch: "x86_64", Status: lineStatus}
		if lineStatus == model.VersionLineStatusReady {
			line.PacksReadyAt = packsReady()
			pkg := update.ArtifactInfo{FileName: "demo-" + semver + "-macos-x86_64.dmg", Size: 1}
			if !hwOnly {
				line.FullPkgs = append(line.FullPkgs, pkg)
			}
			hw := "revA"
			line.FullPkgs = append(line.FullPkgs, update.ArtifactInfo{FileName: "demo-" + semver + "-macos-x86_64-revA.dmg", Size: 2, HwRev: &hw})
		}
		cat.Versions = append(cat.Versions, update.VersionState{Version: v, Lines: []update.LineState{line}})
	}

	// 可见：100% stable。
	add("1.0.0", 10, model.VersionStatusPublished, 100, false, "stable", model.VersionLineStatusReady, false)
	// 不可见：灰度 50% 非强制（匿名无 device_id，白名单亦不生效）。
	add("1.1.0", 11, model.VersionStatusPublished, 50, false, "stable", model.VersionLineStatusReady, false)
	// 可见：关键版本（强制路径忽略灰度，§5.4）。
	add("1.2.0", 12, model.VersionStatusPublished, 100, true, "stable", model.VersionLineStatusReady, false)
	// 不可见：draft。
	add("1.3.0", 13, model.VersionStatusDraft, 100, false, "stable", model.VersionLineStatusReady, false)
	// 不可见：revoked。
	add("1.4.0", 14, model.VersionStatusRevoked, 100, false, "stable", model.VersionLineStatusReady, false)
	// 不可见：Line yanked。
	add("1.5.0", 15, model.VersionStatusPublished, 100, false, "stable", model.VersionLineStatusYanked, false)
	// 不可见：beta 渠道（feed 请求 stable）。
	add("2.0.0-beta.1", 20, model.VersionStatusPublished, 100, false, "beta", model.VersionLineStatusReady, false)
	// 不可见：仅非默认 hw 变体（匿名只匹配默认变体，C16-8）。
	add("2.1.0", 21, model.VersionStatusPublished, 100, false, "stable", model.VersionLineStatusReady, true)

	items := AnonymousVisible(cat, cat.Project.CompareEngine, "stable", "macos", "x86_64")
	if len(items) != 2 {
		t.Fatalf("visible items = %d, want 2", len(items))
	}
	// 新→旧：1.2.0 在前，1.0.0 在后。
	first, second := items[0].Version.Version, items[1].Version.Version
	if first.VersionSemverCanonical == nil || *first.VersionSemverCanonical != "1.2.0" {
		t.Fatalf("first item = %v", first.VersionSemverCanonical)
	}
	if second.VersionSemverCanonical == nil || *second.VersionSemverCanonical != "1.0.0" {
		t.Fatalf("second item = %v", second.VersionSemverCanonical)
	}
	// 可见项的 Package 必须是默认变体。
	for _, it := range items {
		if it.Package.HwRev != nil {
			t.Fatalf("item %v package is not default variant: %v", it.Package.FileName, *it.Package.HwRev)
		}
	}
}

// TestAnonymousVisibleDisabledChannel 渠道停用时该渠道版本整体不可见。
func TestAnonymousVisibleDisabledChannel(t *testing.T) {
	cat := &update.Catalog{
		Project:  newTestProject("demo"),
		Channels: []update.ChannelInfo{{Slug: "stable", StabilityRank: 30, Enabled: false}},
	}
	semver := "1.0.0"
	vInt := int64(10)
	cat.Versions = append(cat.Versions, update.VersionState{
		Version: model.Version{ChannelSlug: "stable", Status: model.VersionStatusPublished, GrayCompletedAt: &testPublishTime, VersionSemverCanonical: &semver, VersionInteger: &vInt},
		Lines: []update.LineState{{OS: "macos", Arch: "x86_64", Status: model.VersionLineStatusReady,
			PacksReadyAt: packsReady(),
			FullPkgs: []update.ArtifactInfo{{FileName: "demo-1.0.0-macos-x86_64.dmg"}}}},
	})
	if items := AnonymousVisible(cat, cat.Project.CompareEngine, "stable", "macos", "x86_64"); len(items) != 0 {
		t.Fatalf("disabled channel must be invisible, got %d items", len(items))
	}
}
