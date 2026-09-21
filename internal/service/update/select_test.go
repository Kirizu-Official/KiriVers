package update

import (
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/Kirizu-Official/KiriVers/internal/model"
)

var testGrayCompleteAt = time.Unix(1, 0).UTC()

// mkVersion 构造版本快照；默认已转全量（匿名可见）。
func mkVersion(channel, status string, integer *int64, semver *string, mutators ...func(*model.Version)) VersionState {
	done := testGrayCompleteAt
	v := model.Version{
		ID:                     uuid.New(),
		ChannelSlug:            channel,
		Status:                 status,
		VersionInteger:         integer,
		VersionSemver:          semver,
		VersionSemverCanonical: semver,
		GrayStartPercent:       100,
		GrayCompletedAt:        &done,
		Changelog:              model.ChangelogMap{},
	}
	for _, m := range mutators {
		m(&v)
	}
	return VersionState{Version: v}
}

func markGrayIncomplete(v *model.Version) {
	started := testGrayCompleteAt
	v.GrayStartedAt = &started
	v.GrayCompletedAt = nil
	v.GrayStartPercent = 10
}

func semP(s string) *string { return &s }
func intP(i int64) *int64   { return &i }
func strP(s string) *string { return &s }

func mkChannel(slug string, rank int) ChannelInfo {
	return ChannelInfo{Slug: slug, StabilityRank: rank, Enabled: true}
}

// mkLine 给版本追加一条平台切片。
func mkLine(vs *VersionState, os, arch, status string, pkgs ...ArtifactInfo) *VersionState {
	ls := LineState{OS: os, Arch: arch, Status: status, FullPkgs: pkgs}
	if status == model.VersionLineStatusReady {
		t := testGrayCompleteAt
		ls.PacksReadyAt = &t
	}
	vs.Lines = append(vs.Lines, ls)
	return vs
}

// mkPkg 构造 full 产物快照。
func mkPkg(name string, size int64, hw *string) ArtifactInfo {
	return ArtifactInfo{FileName: name, Size: size, SHA256: "sha-" + name, HwRev: hw}
}

// mkCatalog 构造目录快照（semver 引擎、系统渠道、默认缓存策略）。
func mkCatalog(engine string, channels []ChannelInfo, versions ...VersionState) *Catalog {
	return &Catalog{
		Project: ProjectInfo{
			CompareEngine:           engine,
			CacheSMaxageSeconds:     60,
			DefaultLocale:           "en",
			ChangelogScope:          model.ChangelogScopeRangeAll,
			ChangelogLayout:         model.ChangelogLayoutBoth,
			ChangelogClientOverride: true,
			ChangelogIncludeRevoked: true,
			ChangelogIncludeNotes:   true,
		},
		Channels:   channels,
		HwRevRanks: map[string]int{},
		Versions:   versions,
	}
}

// defaultReq 构造默认请求（windows/x86_64、匿名、默认变体）。
func defaultReq(cur *VersionState) Request {
	return Request{Current: cur, OS: "windows", Arch: "x86_64"}
}

func targetRef(t *testing.T, res Result, wantSemver string) {
	t.Helper()
	if res.Target == nil {
		t.Fatalf("expected target %s, got none", wantSemver)
	}
	if res.Target.Version.VersionSemverCanonical == nil || *res.Target.Version.VersionSemverCanonical != wantSemver {
		t.Fatalf("expected target %s, got %v", wantSemver, res.Target.Version.VersionSemverCanonical)
	}
}

// ---------- 正常升级与跨渠道 ----------

// 验收：跨渠道只升（beta 客户端能看到比较键更高的 stable）；stable 看不到 beta。
func TestSelectTarget_CrossChannelOnlyUp(t *testing.T) {
	cur := mkVersion("beta", model.VersionStatusPublished, intP(10), semP("1.0.0"))
	mkLine(&cur, "windows", "x86_64", model.VersionLineStatusReady, mkPkg("p-1.0.0.zip", 10, nil))

	betaNew := mkVersion("beta", model.VersionStatusPublished, intP(12), semP("1.2.0"))
	mkLine(&betaNew, "windows", "x86_64", model.VersionLineStatusReady, mkPkg("p-1.2.0.zip", 12, nil))

	stableNew := mkVersion("stable", model.VersionStatusPublished, intP(11), semP("1.1.0"))
	mkLine(&stableNew, "windows", "x86_64", model.VersionLineStatusReady, mkPkg("p-1.1.0.zip", 11, nil))

	betaOnly := mkVersion("beta", model.VersionStatusPublished, intP(15), semP("1.5.0"))
	mkLine(&betaOnly, "windows", "x86_64", model.VersionLineStatusReady, mkPkg("p-1.5.0.zip", 15, nil))

	cat := mkCatalog(model.CompareEngineSemver, []ChannelInfo{mkChannel("alpha", 10), mkChannel("beta", 20), mkChannel("stable", 30)},
		cur, betaNew, stableNew, betaOnly)

	// beta 客户端：1.5.0(beta) 与 1.1.0(stable) 都高于当前，取比较键最高者 1.5.0。
	res, err := SelectTarget(cat, defaultReq(&cur))
	if err != nil {
		t.Fatal(err)
	}
	targetRef(t, res, "1.5.0")
	if res.Reason != ReasonNormal || res.IsMandatory || res.IsDowngrade {
		t.Fatalf("unexpected result: %+v", res)
	}
	if res.Package == nil || res.Package.FileName != "p-1.5.0.zip" {
		t.Fatalf("expected matched package, got %+v", res.Package)
	}

	// stable 客户端：只能看到 stable（1.1.0），看不到 1.2.0/1.5.0(beta)。
	curStable := mkVersion("stable", model.VersionStatusPublished, intP(10), semP("1.0.0"))
	mkLine(&curStable, "windows", "x86_64", model.VersionLineStatusReady, mkPkg("p-1.0.0.zip", 10, nil))
	res, err = SelectTarget(cat, defaultReq(&curStable))
	if err != nil {
		t.Fatal(err)
	}
	targetRef(t, res, "1.1.0")
}

// 验收：同 Version 只上 windows 时，android check 不把该 Version 当升级目标。
func TestSelectTarget_WindowsOnlyVersionNotAndroidTarget(t *testing.T) {
	cur := mkVersion("stable", model.VersionStatusPublished, intP(10), semP("1.0.0"))
	mkLine(&cur, "android", "arm64", model.VersionLineStatusReady, mkPkg("app-1.0.0.apk", 10, nil))

	winOnly := mkVersion("stable", model.VersionStatusPublished, intP(11), semP("1.1.0"))
	mkLine(&winOnly, "windows", "x86_64", model.VersionLineStatusReady, mkPkg("p-1.1.0.zip", 11, nil))

	androidNext := mkVersion("stable", model.VersionStatusPublished, intP(12), semP("1.2.0"))
	mkLine(&androidNext, "android", "arm64", model.VersionLineStatusReady, mkPkg("app-1.2.0.apk", 12, nil))

	cat := mkCatalog(model.CompareEngineSemver, []ChannelInfo{mkChannel("stable", 30)}, cur, winOnly, androidNext)

	req := Request{Current: &cur, OS: "android", Arch: "arm64"}
	res, err := SelectTarget(cat, req)
	if err != nil {
		t.Fatal(err)
	}
	targetRef(t, res, "1.2.0") // 跳过 windows-only 的 1.1.0
}

// 验收：无本平台就绪产物的更高 Version 不作为正常升级 target。
func TestSelectTarget_HigherVersionWithoutPlatformLineSkipped(t *testing.T) {
	cur := mkVersion("stable", model.VersionStatusPublished, intP(10), semP("1.0.0"))
	mkLine(&cur, "windows", "x86_64", model.VersionLineStatusReady, mkPkg("p-1.0.0.zip", 10, nil))

	emptyHigh := mkVersion("stable", model.VersionStatusPublished, intP(20), semP("2.0.0"))
	mkLine(&emptyHigh, "windows", "x86_64", model.VersionLineStatusPending) // 从未就绪

	next := mkVersion("stable", model.VersionStatusPublished, intP(11), semP("1.1.0"))
	mkLine(&next, "windows", "x86_64", model.VersionLineStatusReady, mkPkg("p-1.1.0.zip", 11, nil))

	cat := mkCatalog(model.CompareEngineSemver, []ChannelInfo{mkChannel("stable", 30)}, cur, emptyHigh, next)
	res, err := SelectTarget(cat, defaultReq(&cur))
	if err != nil {
		t.Fatal(err)
	}
	targetRef(t, res, "1.1.0")
}

func TestSelectTarget_UnstampedPacksReadySkipped(t *testing.T) {
	cur := mkVersion("stable", model.VersionStatusPublished, intP(10), semP("1.0.0"))
	mkLine(&cur, "windows", "x86_64", model.VersionLineStatusReady, mkPkg("p-1.0.0.zip", 10, nil))

	unstamped := mkVersion("stable", model.VersionStatusPublished, intP(20), semP("2.0.0"))
	mkLine(&unstamped, "windows", "x86_64", model.VersionLineStatusReady, mkPkg("p-2.0.0.zip", 20, nil))
	unstamped.Lines[0].PacksReadyAt = nil

	next := mkVersion("stable", model.VersionStatusPublished, intP(11), semP("1.1.0"))
	mkLine(&next, "windows", "x86_64", model.VersionLineStatusReady, mkPkg("p-1.1.0.zip", 11, nil))

	cat := mkCatalog(model.CompareEngineSemver, []ChannelInfo{mkChannel("stable", 30)}, cur, unstamped, next)
	res, err := SelectTarget(cat, defaultReq(&cur))
	if err != nil {
		t.Fatal(err)
	}
	targetRef(t, res, "1.1.0")
}

// 验收：无更新 → Target=nil（HTTP 204）；deprecated 不作为升级目标。
func TestSelectTarget_NoUpdate(t *testing.T) {
	cur := mkVersion("stable", model.VersionStatusPublished, intP(10), semP("1.0.0"))
	mkLine(&cur, "windows", "x86_64", model.VersionLineStatusReady, mkPkg("p-1.0.0.zip", 10, nil))

	dep := mkVersion("stable", model.VersionStatusDeprecated, intP(12), semP("1.2.0"))
	mkLine(&dep, "windows", "x86_64", model.VersionLineStatusReady, mkPkg("p-1.2.0.zip", 12, nil))

	draft := mkVersion("stable", model.VersionStatusDraft, intP(13), semP("1.3.0"))
	mkLine(&draft, "windows", "x86_64", model.VersionLineStatusReady, mkPkg("p-1.3.0.zip", 13, nil))

	cat := mkCatalog(model.CompareEngineSemver, []ChannelInfo{mkChannel("stable", 30)}, cur, dep, draft)
	res, err := SelectTarget(cat, defaultReq(&cur))
	if err != nil {
		t.Fatal(err)
	}
	if res.Target != nil {
		t.Fatalf("expected no target (204), got %+v", res.Target)
	}
}

// 验收：LTS 1.x 可被选到比较键更高的 2.x（不按 major 截断，§5.3）。
func TestSelectTarget_LTSCanUpgradeToNextMajor(t *testing.T) {
	cur := mkVersion("stable", model.VersionStatusPublished, intP(10), semP("1.0.0"), func(v *model.Version) { v.IsLTS = true })
	mkLine(&cur, "windows", "x86_64", model.VersionLineStatusReady, mkPkg("p-1.0.0.zip", 10, nil))

	next := mkVersion("stable", model.VersionStatusPublished, intP(20), semP("2.0.0"))
	mkLine(&next, "windows", "x86_64", model.VersionLineStatusReady, mkPkg("p-2.0.0.zip", 20, nil))

	cat := mkCatalog(model.CompareEngineSemver, []ChannelInfo{mkChannel("stable", 30)}, cur, next)
	res, err := SelectTarget(cat, defaultReq(&cur))
	if err != nil {
		t.Fatal(err)
	}
	targetRef(t, res, "2.0.0")
}

// ---------- 灰度与强制 ----------

// 验收：灰度未 100% 时匿名（无 device_id）不命中；命中设备可选到。
func TestSelectTarget_GrayRolloutAnonymousAndHit(t *testing.T) {
	cur := mkVersion("stable", model.VersionStatusPublished, intP(10), semP("1.0.0"))
	mkLine(&cur, "windows", "x86_64", model.VersionLineStatusReady, mkPkg("p-1.0.0.zip", 10, nil))

	gray := mkVersion("stable", model.VersionStatusPublished, intP(11), semP("1.1.0"), markGrayIncomplete)
	mkLine(&gray, "windows", "x86_64", model.VersionLineStatusReady, mkPkg("p-1.1.0.zip", 11, nil))
	gray.Allowlist = Allowlist{Version: []string{"hit-hash"}}

	cat := mkCatalog(model.CompareEngineSemver, []ChannelInfo{mkChannel("stable", 30)}, cur, gray)

	res, err := SelectTarget(cat, defaultReq(&cur))
	if err != nil {
		t.Fatal(err)
	}
	if res.Target != nil {
		t.Fatalf("anonymous must not hit incomplete gray")
	}

	res, err = SelectTarget(cat, Request{Current: &cur, OS: "windows", Arch: "x86_64", DeviceID: "hit", DeviceHash: "hit-hash"})
	if err != nil {
		t.Fatal(err)
	}
	targetRef(t, res, "1.1.0")

	res, err = SelectTarget(cat, Request{Current: &cur, OS: "windows", Arch: "x86_64", DeviceID: "miss", DeviceHash: "miss-hash"})
	if err != nil {
		t.Fatal(err)
	}
	if res.Target != nil {
		t.Fatalf("device miss must miss incomplete gray")
	}
}

func TestSelectTarget_MinimumSupportedVersionMandatoryIgnoresGray(t *testing.T) {
	cur := mkVersion("stable", model.VersionStatusPublished, intP(10), semP("1.0.0"))
	mkLine(&cur, "windows", "x86_64", model.VersionLineStatusReady, mkPkg("p-1.0.0.zip", 10, nil))

	target := mkVersion("stable", model.VersionStatusPublished, intP(11), semP("1.1.0"), markGrayIncomplete)
	mkLine(&target, "windows", "x86_64", model.VersionLineStatusReady, mkPkg("p-1.1.0.zip", 11, nil))

	cat := mkCatalog(model.CompareEngineSemver, []ChannelInfo{mkChannel("stable", 30)}, cur, target)
	cat.Project.MinimumSupportedVersion = strP("1.0.5")

	// 匿名 + 未命中灰度，但最低支持强制 → 仍可选到。
	res, err := SelectTarget(cat, defaultReq(&cur))
	if err != nil {
		t.Fatal(err)
	}
	targetRef(t, res, "1.1.0")
	if !res.IsMandatory || res.Reason != ReasonMinimumSupportedVersion {
		t.Fatalf("expected mandatory via minimum_supported_version, got %+v", res)
	}

	// 矩阵行覆盖优先于项目配置：覆盖为 0.5.0（低于当前）→ 不强制，灰度生效 → 匿名 204。
	cat.Matrix = &MatrixInfo{OS: "windows", Arch: "x86_64", PackageType: model.PackageTypeSingleFile, MinimumSupportedVersion: strP("0.5.0")}
	res, err = SelectTarget(cat, Request{Current: &cur, OS: "windows", Arch: "x86_64", DeviceID: ""})
	if err != nil {
		t.Fatal(err)
	}
	if res.Target != nil {
		t.Fatalf("gray must apply when matrix floor overrides below current")
	}
}

// 验收：路径 (current, target] 上本平台曾就绪的关键 Version 使 check 强制（忽略灰度）。
func TestSelectTarget_CriticalPathMandatory(t *testing.T) {
	cur := mkVersion("stable", model.VersionStatusPublished, intP(10), semP("1.0.0"))
	mkLine(&cur, "windows", "x86_64", model.VersionLineStatusReady, mkPkg("p-1.0.0.zip", 10, nil))

	critical := mkVersion("stable", model.VersionStatusPublished, intP(11), semP("1.1.0"), func(v *model.Version) { v.IsCritical = true })
	mkLine(&critical, "windows", "x86_64", model.VersionLineStatusReady, mkPkg("p-1.1.0.zip", 11, nil))

	target := mkVersion("stable", model.VersionStatusPublished, intP(12), semP("1.2.0"), markGrayIncomplete)
	mkLine(&target, "windows", "x86_64", model.VersionLineStatusReady, mkPkg("p-1.2.0.zip", 12, nil))

	cat := mkCatalog(model.CompareEngineSemver, []ChannelInfo{mkChannel("stable", 30)}, cur, critical, target)

	// 关键路径强制 + 忽略灰度（匿名也能拿到 rollout=30 的目标）。
	res, err := SelectTarget(cat, defaultReq(&cur))
	if err != nil {
		t.Fatal(err)
	}
	targetRef(t, res, "1.2.0")
	if !res.IsMandatory || res.Reason != ReasonCriticalVersion {
		t.Fatalf("expected mandatory via critical path, got %+v", res)
	}
}

// 验收：仅对其他平台就绪过的关键 Version 不使本 os/arch 的 check 变成强制。
func TestSelectTarget_CriticalOnlyOnOtherPlatformNotMandatory(t *testing.T) {
	cur := mkVersion("stable", model.VersionStatusPublished, intP(10), semP("1.0.0"))
	mkLine(&cur, "windows", "x86_64", model.VersionLineStatusReady, mkPkg("p-1.0.0.zip", 10, nil))

	// 关键版本只有 macos 线（对本平台从未就绪）。
	critical := mkVersion("stable", model.VersionStatusPublished, intP(11), semP("1.1.0"), func(v *model.Version) { v.IsCritical = true })
	mkLine(&critical, "macos", "x86_64", model.VersionLineStatusReady, mkPkg("p-1.1.0-mac.zip", 11, nil))

	target := mkVersion("stable", model.VersionStatusPublished, intP(12), semP("1.2.0"), markGrayIncomplete)
	mkLine(&target, "windows", "x86_64", model.VersionLineStatusReady, mkPkg("p-1.2.0.zip", 12, nil))

	cat := mkCatalog(model.CompareEngineSemver, []ChannelInfo{mkChannel("stable", 30)}, cur, critical, target)

	// 匿名 + rollout=30 → 不强制也不命中 → 204。
	res, err := SelectTarget(cat, defaultReq(&cur))
	if err != nil {
		t.Fatal(err)
	}
	if res.Target != nil {
		t.Fatalf("critical version never ready on this platform must not force update")
	}
}

// ---------- 中继（§5.5.1） ----------

func relayFixture() (*Catalog, *VersionState) {
	cur := mkVersion("stable", model.VersionStatusPublished, intP(10), semP("1.0.0"))
	mkLine(&cur, "windows", "x86_64", model.VersionLineStatusReady, mkPkg("p-1.0.0.zip", 10, nil))
	inter := mkVersion("stable", model.VersionStatusPublished, intP(15), semP("1.5.0"))
	mkLine(&inter, "windows", "x86_64", model.VersionLineStatusReady, mkPkg("p-1.5.0.zip", 15, nil))
	latest := mkVersion("stable", model.VersionStatusPublished, intP(20), semP("2.0.0"),
		func(v *model.Version) {
			v.MinSourceVersion = strP("1.5.0")
		})
	mkLine(&latest, "windows", "x86_64", model.VersionLineStatusReady, mkPkg("p-2.0.0.zip", 20, nil))
	cat := mkCatalog(model.CompareEngineSemver, []ChannelInfo{mkChannel("stable", 30)}, cur, inter, latest)
	return cat, &cur
}

// 验收：目标配了 min_source_version 且客户端更旧时，check 必须给中继而非最新。
func TestSelectTarget_RelaySingleHop(t *testing.T) {
	cat, cur := relayFixture()
	res, err := SelectTarget(cat, defaultReq(cur))
	if err != nil {
		t.Fatal(err)
	}
	targetRef(t, res, "1.5.0")
	if !res.IsMandatory || res.Reason != ReasonIntermediate || res.RelayHops != 1 {
		t.Fatalf("expected mandatory relay hop=1, got %+v", res)
	}
	// 达到中继版本后即可直升最新。
	cur2 := mkVersion("stable", model.VersionStatusPublished, intP(15), semP("1.5.0"))
	mkLine(&cur2, "windows", "x86_64", model.VersionLineStatusReady, mkPkg("p-1.5.0.zip", 15, nil))
	res, err = SelectTarget(cat, defaultReq(&cur2))
	if err != nil {
		t.Fatal(err)
	}
	targetRef(t, res, "2.0.0")
	if res.IsMandatory {
		t.Fatalf("direct upgrade must not be mandatory: %+v", res)
	}
}

// 中继链多跳、成环、超跳数与未就绪中继 → INTERMEDIATE_UNAVAILABLE。
func TestSelectTarget_RelayErrors(t *testing.T) {
	mkVer := func(minSource string, integer int64, sem string, lineStatus string) VersionState {
		v := mkVersion("stable", model.VersionStatusPublished, intP(integer), semP(sem))
		if minSource != "" {
			v.Version.MinSourceVersion = strP(minSource)
		}
		mkLine(&v, "windows", "x86_64", lineStatus, mkPkg("p-"+sem+".zip", 20, nil))
		return v
	}
	cur := mkVersion("stable", model.VersionStatusPublished, intP(10), semP("1.0.0"))
	mkLine(&cur, "windows", "x86_64", model.VersionLineStatusReady, mkPkg("p-1.0.0.zip", 10, nil))

	// 成环：2.0 min_source=1.5；1.5 min_source=2.0。
	loopLatest := mkVer("1.5.0", 20, "2.0.0", model.VersionLineStatusReady)
	loopInter := mkVer("2.0.0", 15, "1.5.0", model.VersionLineStatusReady)
	cat := mkCatalog(model.CompareEngineSemver, []ChannelInfo{mkChannel("stable", 30)}, cur, loopInter, loopLatest)
	if _, err := SelectTarget(cat, defaultReq(&cur)); !errors.Is(err, ErrIntermediateUnavailable) {
		t.Fatalf("expected ErrIntermediateUnavailable on cycle, got %v", err)
	}

	// 中继未就绪（pending line）。
	notReady := mkVer("1.5.0", 15, "1.5.0", model.VersionLineStatusPending)
	latest := mkVer("1.5.0", 20, "2.0.0", model.VersionLineStatusReady)
	cat = mkCatalog(model.CompareEngineSemver, []ChannelInfo{mkChannel("stable", 30)}, cur, notReady, latest)
	if _, err := SelectTarget(cat, defaultReq(&cur)); !errors.Is(err, ErrIntermediateUnavailable) {
		t.Fatalf("expected ErrIntermediateUnavailable for non-ready relay, got %v", err)
	}

	// 中继不存在。
	latest = mkVer("9.9.9", 20, "2.0.0", model.VersionLineStatusReady)
	cat = mkCatalog(model.CompareEngineSemver, []ChannelInfo{mkChannel("stable", 30)}, cur, latest)
	if _, err := SelectTarget(cat, defaultReq(&cur)); !errors.Is(err, ErrIntermediateUnavailable) {
		t.Fatalf("expected ErrIntermediateUnavailable for missing relay, got %v", err)
	}

	// 多跳链路成功：1.0 → 1.2 → 1.5 → 2.0（2 跳中继后落在 1.2.0）。
	m12 := mkVer("1.0.0", 12, "1.2.0", model.VersionLineStatusReady)
	m15 := mkVer("1.2.0", 15, "1.5.0", model.VersionLineStatusReady)
	m20 := mkVer("1.5.0", 20, "2.0.0", model.VersionLineStatusReady)
	cat = mkCatalog(model.CompareEngineSemver, []ChannelInfo{mkChannel("stable", 30)}, cur, m12, m15, m20)
	res, err := SelectTarget(cat, defaultReq(&cur))
	if err != nil {
		t.Fatal(err)
	}
	targetRef(t, res, "1.2.0")
	if res.RelayHops != 2 || res.Reason != ReasonIntermediate {
		t.Fatalf("expected relay stop at 1.2.0 with 2 hops, got %+v", res)
	}
}

// ---------- 降级分支（§4.4.1 / §5.6） ----------

// 验收：吊销降级 is_downgrade；找不到安全目标 → NO_SAFE_TARGET。
func TestSelectTarget_RevokeDowngrade(t *testing.T) {
	revoked := mkVersion("stable", model.VersionStatusRevoked, intP(20), semP("2.0.0"))
	mkLine(&revoked, "windows", "x86_64", model.VersionLineStatusReady, mkPkg("p-2.0.0.zip", 20, nil))

	safe := mkVersion("stable", model.VersionStatusPublished, intP(15), semP("1.5.0"))
	mkLine(&safe, "windows", "x86_64", model.VersionLineStatusReady, mkPkg("p-1.5.0.zip", 15, nil))

	cat := mkCatalog(model.CompareEngineSemver, []ChannelInfo{mkChannel("stable", 30)}, revoked, safe)
	res, err := SelectTarget(cat, defaultReq(&revoked))
	if err != nil {
		t.Fatal(err)
	}
	targetRef(t, res, "1.5.0")
	if !res.IsMandatory || !res.IsDowngrade || res.Reason != ReasonRevoked || res.DeltaAllowed {
		t.Fatalf("unexpected downgrade result: %+v", res)
	}

	// 吊销且无任何安全目标 → NO_SAFE_TARGET。
	empty := mkCatalog(model.CompareEngineSemver, []ChannelInfo{mkChannel("stable", 30)}, revoked)
	if _, err := SelectTarget(empty, defaultReq(&revoked)); !errors.Is(err, ErrNoSafeTarget) {
		t.Fatalf("expected ErrNoSafeTarget, got %v", err)
	}
}

// yank 分支：reason=yanked；更高版本存在时 is_downgrade=false；同渠道优先于 LTS。
func TestSelectTarget_YankDowngrade(t *testing.T) {
	yanked := mkVersion("stable", model.VersionStatusPublished, intP(20), semP("2.0.0"))
	mkLine(&yanked, "windows", "x86_64", model.VersionLineStatusYanked, mkPkg("p-2.0.0.zip", 20, nil))

	higher := mkVersion("stable", model.VersionStatusPublished, intP(21), semP("2.1.0"))
	mkLine(&higher, "windows", "x86_64", model.VersionLineStatusReady, mkPkg("p-2.1.0.zip", 21, nil))

	lower := mkVersion("stable", model.VersionStatusPublished, intP(15), semP("1.5.0"))
	mkLine(&lower, "windows", "x86_64", model.VersionLineStatusReady, mkPkg("p-1.5.0.zip", 15, nil))

	cat := mkCatalog(model.CompareEngineSemver, []ChannelInfo{mkChannel("stable", 30)}, yanked, higher, lower)
	res, err := SelectTarget(cat, defaultReq(&yanked))
	if err != nil {
		t.Fatal(err)
	}
	targetRef(t, res, "2.1.0")
	if res.IsDowngrade || !res.IsMandatory || res.Reason != ReasonYanked {
		t.Fatalf("unexpected yank result: %+v", res)
	}

	// 无同渠道可用 → 回落 LTS（beta 渠道）。
	ltsBeta := mkVersion("beta", model.VersionStatusPublished, intP(11), semP("1.1.0"), func(v *model.Version) { v.IsLTS = true })
	mkLine(&ltsBeta, "windows", "x86_64", model.VersionLineStatusReady, mkPkg("p-1.1.0.zip", 11, nil))
	cat = mkCatalog(model.CompareEngineSemver, []ChannelInfo{mkChannel("beta", 20), mkChannel("stable", 30)}, yanked, ltsBeta)
	res, err = SelectTarget(cat, defaultReq(&yanked))
	if err != nil {
		t.Fatal(err)
	}
	targetRef(t, res, "1.1.0")
	if !res.IsDowngrade {
		t.Fatalf("expected downgrade to LTS")
	}

	// 无同渠道与 LTS → 更稳渠道（alpha 客户端 yank 后落到 stable）。
	alphaCur := mkVersion("alpha", model.VersionStatusPublished, intP(5), semP("0.5.0"))
	mkLine(&alphaCur, "windows", "x86_64", model.VersionLineStatusYanked)
	stableSafe := mkVersion("stable", model.VersionStatusPublished, intP(8), semP("0.8.0"))
	mkLine(&stableSafe, "windows", "x86_64", model.VersionLineStatusReady, mkPkg("p-0.8.0.zip", 8, nil))
	cat = mkCatalog(model.CompareEngineSemver, []ChannelInfo{mkChannel("alpha", 10), mkChannel("stable", 30)}, alphaCur, stableSafe)
	res, err = SelectTarget(cat, Request{Current: &alphaCur, OS: "windows", Arch: "x86_64"})
	if err != nil {
		t.Fatal(err)
	}
	targetRef(t, res, "0.8.0")
}

// 降级分支不受 min_source_version 阻挡。
func TestSelectTarget_DowngradeIgnoresMinSource(t *testing.T) {
	revoked := mkVersion("stable", model.VersionStatusRevoked, intP(20), semP("2.0.0"))
	mkLine(&revoked, "windows", "x86_64", model.VersionLineStatusReady)

	safe := mkVersion("stable", model.VersionStatusPublished, intP(15), semP("1.5.0"),
		func(v *model.Version) { v.MinSourceVersion = strP("2.0.0") }) // 正常分支会阻挡 2.0 以下
	mkLine(&safe, "windows", "x86_64", model.VersionLineStatusReady, mkPkg("p-1.5.0.zip", 15, nil))

	cat := mkCatalog(model.CompareEngineSemver, []ChannelInfo{mkChannel("stable", 30)}, revoked, safe)
	res, err := SelectTarget(cat, defaultReq(&revoked))
	if err != nil {
		t.Fatal(err)
	}
	targetRef(t, res, "1.5.0")
}

// ---------- min_os 与 fallback_arch ----------

// 候选因 min_os 不满足被静默跳过；当前版本自身 min_os 未满足 → MIN_OS_NOT_MET。
func TestSelectTarget_MinOS(t *testing.T) {
	cur := mkVersion("stable", model.VersionStatusPublished, intP(10), semP("1.0.0"))
	mkLine(&cur, "windows", "x86_64", model.VersionLineStatusReady, mkPkg("p-1.0.0.zip", 10, nil))

	blocked := mkVersion("stable", model.VersionStatusPublished, intP(11), semP("1.1.0"))
	mkLine(&blocked, "windows", "x86_64", model.VersionLineStatusReady, mkPkg("p-1.1.0.zip", 11, nil))
	blocked.Lines[0].MinOS = strP("10.0")

	ok := mkVersion("stable", model.VersionStatusPublished, intP(12), semP("1.2.0"))
	mkLine(&ok, "windows", "x86_64", model.VersionLineStatusReady, mkPkg("p-1.2.0.zip", 12, nil))
	ok.Lines[0].MinOS = strP("6.1")

	cat := mkCatalog(model.CompareEngineSemver, []ChannelInfo{mkChannel("stable", 30)}, cur, blocked, ok)
	cat.Matrix = &MatrixInfo{OS: "windows", Arch: "x86_64", PackageType: model.PackageTypeSingleFile}

	res, err := SelectTarget(cat, Request{Current: &cur, OS: "windows", Arch: "x86_64", OSVersion: "6.1"})
	if err != nil {
		t.Fatal(err)
	}
	targetRef(t, res, "1.2.0") // 1.1.0 要求 win10，静默跳过

	// 当前版本自身 min_os 未满足 → 409 MIN_OS_NOT_MET。
	cur.Lines[0].MinOS = strP("10.0")
	if _, err := SelectTarget(cat, Request{Current: &cur, OS: "windows", Arch: "x86_64", OSVersion: "6.1"}); !errors.Is(err, ErrMinOSNotMet) {
		t.Fatalf("expected ErrMinOSNotMet, got %v", err)
	}
	// 不带 os_version 时不比较。
	if _, err := SelectTarget(cat, defaultReq(&cur)); err != nil {
		t.Fatalf("unexpected error without os_version: %v", err)
	}
}

// fallback_arch：本 arch 无更新时以矩阵回退架构重跑。
func TestSelectTarget_FallbackArch(t *testing.T) {
	cur := mkVersion("stable", model.VersionStatusPublished, intP(10), semP("1.0.0"))
	mkLine(&cur, "linux", "armv7", model.VersionLineStatusReady, mkPkg("p-1.0.0-armv7.zip", 10, nil))
	mkLine(&cur, "linux", "arm64", model.VersionLineStatusReady, mkPkg("p-1.0.0-arm64.zip", 10, nil))

	next := mkVersion("stable", model.VersionStatusPublished, intP(11), semP("1.1.0"))
	mkLine(&next, "linux", "arm64", model.VersionLineStatusReady, mkPkg("p-1.1.0-arm64.zip", 11, nil))

	cat := mkCatalog(model.CompareEngineSemver, []ChannelInfo{mkChannel("stable", 30)}, cur, next)
	cat.Matrix = &MatrixInfo{OS: "linux", Arch: "armv7", PackageType: model.PackageTypeSingleFile, FallbackArch: "arm64"}
	cat.FallbackMatrix = &MatrixInfo{OS: "linux", Arch: "arm64", PackageType: model.PackageTypeSingleFile}

	res, err := SelectTarget(cat, Request{Current: &cur, OS: "linux", Arch: "armv7"})
	if err != nil {
		t.Fatal(err)
	}
	targetRef(t, res, "1.1.0")
	if !res.UsedFallbackArch || res.Line.Arch != "arm64" || res.FallbackMatrix != cat.FallbackMatrix {
		t.Fatalf("unexpected fallback result: %+v", res)
	}
}

// fallback_arch 命中时中继必须沿回退架构解析（回归：relay 曾固定查主架构，
// 导致回退平台上的合法中继被误报 INTERMEDIATE_UNAVAILABLE）。
func TestSelectTarget_RelayOnFallbackArch(t *testing.T) {
	cur := mkVersion("stable", model.VersionStatusPublished, intP(10), semP("1.0.0"))
	mkLine(&cur, "linux", "armv7", model.VersionLineStatusReady, mkPkg("p-1.0.0-armv7.zip", 10, nil))
	mkLine(&cur, "linux", "arm64", model.VersionLineStatusReady, mkPkg("p-1.0.0-arm64.zip", 10, nil))

	inter := mkVersion("stable", model.VersionStatusPublished, intP(15), semP("1.5.0"))
	mkLine(&inter, "linux", "arm64", model.VersionLineStatusReady, mkPkg("p-1.5.0-arm64.zip", 15, nil))

	latest := mkVersion("stable", model.VersionStatusPublished, intP(20), semP("2.0.0"),
		func(v *model.Version) {
			v.MinSourceVersion = strP("1.5.0")
		})
	mkLine(&latest, "linux", "arm64", model.VersionLineStatusReady, mkPkg("p-2.0.0-arm64.zip", 20, nil))

	cat := mkCatalog(model.CompareEngineSemver, []ChannelInfo{mkChannel("stable", 30)}, cur, inter, latest)
	cat.Matrix = &MatrixInfo{OS: "linux", Arch: "armv7", PackageType: model.PackageTypeSingleFile, FallbackArch: "arm64"}
	cat.FallbackMatrix = &MatrixInfo{OS: "linux", Arch: "arm64", PackageType: model.PackageTypeSingleFile}

	res, err := SelectTarget(cat, Request{Current: &cur, OS: "linux", Arch: "armv7"})
	if err != nil {
		t.Fatal(err)
	}
	targetRef(t, res, "1.5.0")
	if !res.IsMandatory || res.Reason != ReasonIntermediate || res.RelayHops != 1 {
		t.Fatalf("expected mandatory relay on fallback arch, got %+v", res)
	}
	if !res.UsedFallbackArch || res.Line.Arch != "arm64" {
		t.Fatalf("fallback flags wrong: %+v", res)
	}
}

// ---------- hw 变体（C08-10, §2.3） ----------

// 验收：不传 hw_rev 时只选默认变体。
func TestSelectTarget_HwRevDefaultVariantOnly(t *testing.T) {
	cur := mkVersion("stable", model.VersionStatusPublished, intP(10), semP("1.0.0"))
	mkLine(&cur, "windows", "x86_64", model.VersionLineStatusReady, mkPkg("p-1.0.0.zip", 10, nil))

	next := mkVersion("stable", model.VersionStatusPublished, intP(11), semP("1.1.0"))
	// 只有 hw 变体、无默认变体 → 对未传 hw_rev 的设备不存在。
	mkLine(&next, "windows", "x86_64", model.VersionLineStatusReady,
		mkPkg("p-1.1.0-rev2.zip", 11, strP("rev2")))

	withDefault := mkVersion("stable", model.VersionStatusPublished, intP(12), semP("1.2.0"))
	mkLine(&withDefault, "windows", "x86_64", model.VersionLineStatusReady,
		mkPkg("p-1.2.0-rev2.zip", 12, strP("rev2")),
		mkPkg("p-1.2.0.zip", 12, nil))

	cat := mkCatalog(model.CompareEngineSemver, []ChannelInfo{mkChannel("stable", 30)}, cur, next, withDefault)

	res, err := SelectTarget(cat, defaultReq(&cur))
	if err != nil {
		t.Fatal(err)
	}
	targetRef(t, res, "1.2.0")
	if res.Package.FileName != "p-1.2.0.zip" {
		t.Fatalf("default variant expected, got %s", res.Package.FileName)
	}

	// 传 hw_rev=rev2 → 优先精确变体 1.2.0-rev2。
	res, err = SelectTarget(cat, Request{Current: &cur, OS: "windows", Arch: "x86_64", HwRev: "rev2"})
	if err != nil {
		t.Fatal(err)
	}
	targetRef(t, res, "1.2.0")
	if res.Package.FileName != "p-1.2.0-rev2.zip" {
		t.Fatalf("exact variant expected, got %s", res.Package.FileName)
	}
}

// higher_compatible_with_lower：高 rank 产物覆盖低 rank 设备；independent 需精确匹配。
func TestSelectTarget_HwRevPolicies(t *testing.T) {
	cur := mkVersion("stable", model.VersionStatusPublished, intP(10), semP("1.0.0"))
	mkLine(&cur, "windows", "x86_64", model.VersionLineStatusReady, mkPkg("p-1.0.0.zip", 10, nil))

	next := mkVersion("stable", model.VersionStatusPublished, intP(11), semP("1.1.0"))
	mkLine(&next, "windows", "x86_64", model.VersionLineStatusReady, mkPkg("p-1.1.0-rev3.zip", 11, strP("rev3")))

	// independent：rev2 设备无法使用 rev3 变体，也没有默认变体 → 无更新。
	cat := mkCatalog(model.CompareEngineSemver, []ChannelInfo{mkChannel("stable", 30)}, cur, next)
	cat.HwRevRanks = map[string]int{"rev2": 2, "rev3": 3}
	cat.Matrix = &MatrixInfo{OS: "windows", Arch: "x86_64", PackageType: model.PackageTypeSingleFile, HwVariantPolicy: model.HwVariantIndependent}
	res, err := SelectTarget(cat, Request{Current: &cur, OS: "windows", Arch: "x86_64", HwRev: "rev2"})
	if err != nil {
		t.Fatal(err)
	}
	if res.Target != nil {
		t.Fatalf("independent policy must not cover rev2 with rev3 artifact")
	}

	// higher_compatible_with_lower：rev3 产物覆盖 rev2 设备。
	cat.Matrix.HwVariantPolicy = model.HwVariantHigherCompatible
	res, err = SelectTarget(cat, Request{Current: &cur, OS: "windows", Arch: "x86_64", HwRev: "rev2"})
	if err != nil {
		t.Fatal(err)
	}
	targetRef(t, res, "1.1.0")
	if res.Package.FileName != "p-1.1.0-rev3.zip" {
		t.Fatalf("covered variant expected, got %s", res.Package.FileName)
	}

	// 收窄 compatible_hw_revs 排除 rev2。
	next.Lines[0].FullPkgs[0].CompatibleHwRevs = model.StringList{"rev3"}
	res, err = SelectTarget(cat, Request{Current: &cur, OS: "windows", Arch: "x86_64", HwRev: "rev2"})
	if err != nil {
		t.Fatal(err)
	}
	if res.Target != nil {
		t.Fatalf("narrowed artifact must not serve rev2")
	}
}

// ---------- integer 引擎与同记录解析 ----------

// 102 与 1.2.3 解析同一记录（compare.go 解析与查找）。
func TestParseVersionRefParity(t *testing.T) {
	refInt, ok := parseVersionRef("010")
	if !ok || !refInt.isInteger || refInt.integer != 10 {
		t.Fatalf("010 must parse to integer 10, got %+v", refInt)
	}
	refSem, ok := parseVersionRef("v1.2.3+build.7")
	if !ok || refSem.isInteger || refSem.semver != "1.2.3" {
		t.Fatalf("v1.2.3+build.7 must parse to 1.2.3, got %+v", refSem)
	}
	if _, ok := parseVersionRef("1.2"); ok {
		t.Fatalf("1.2 must not parse")
	}

	// 同一目录下整数与 SemVer 定位到同一 VersionState。
	v := mkVersion("stable", model.VersionStatusPublished, intP(102), semP("1.2.3"))
	mkLine(&v, "windows", "x86_64", model.VersionLineStatusReady, mkPkg("p.zip", 1, nil))
	cat := mkCatalog(model.CompareEngineSemver, []ChannelInfo{mkChannel("stable", 30)}, v)
	ri, _ := parseVersionRef("102")
	rs, _ := parseVersionRef("1.2.3")
	a := findVersionState(cat, ri)
	b := findVersionState(cat, rs)
	if a == nil || b == nil || a != b {
		t.Fatalf("102 and 1.2.3 must resolve to the same record")
	}
}

// integer 引擎比较键分支。
func TestSelectTarget_IntegerEngine(t *testing.T) {
	cur := mkVersion("stable", model.VersionStatusPublished, intP(100), nil)
	mkLine(&cur, "windows", "x86_64", model.VersionLineStatusReady, mkPkg("p-100.zip", 10, nil))
	next := mkVersion("stable", model.VersionStatusPublished, intP(102), semP("1.2.3"))
	mkLine(&next, "windows", "x86_64", model.VersionLineStatusReady, mkPkg("p-102.zip", 11, nil))
	below := mkVersion("stable", model.VersionStatusPublished, intP(90), nil)
	mkLine(&below, "windows", "x86_64", model.VersionLineStatusReady, mkPkg("p-90.zip", 9, nil))

	cat := mkCatalog(model.CompareEngineInteger, []ChannelInfo{mkChannel("stable", 30)}, cur, next, below)
	res, err := SelectTarget(cat, defaultReq(&cur))
	if err != nil {
		t.Fatal(err)
	}
	if res.Target == nil || res.Target.Version.VersionInteger == nil || *res.Target.Version.VersionInteger != 102 {
		t.Fatalf("expected target 102 under integer engine")
	}
}

// AC4：3.0←2.0←1.5；客户端 1.0 先拿 1.5，再到 2.0，再到 3.0。不得跳过被挡的最新版。
func TestSelectTarget_AC4HopChain(t *testing.T) {
	mk := func(sem string, integer int64, minSource string) VersionState {
		v := mkVersion("stable", model.VersionStatusPublished, intP(integer), semP(sem))
		if minSource != "" {
			v.Version.MinSourceVersion = strP(minSource)
		}
		mkLine(&v, "windows", "x86_64", model.VersionLineStatusReady, mkPkg("p-"+sem+".zip", integer, nil))
		return v
	}
	v10 := mk("1.0.0", 10, "")
	v15 := mk("1.5.0", 15, "")
	v20 := mk("2.0.0", 20, "1.5.0")
	v30 := mk("3.0.0", 30, "2.0.0")
	cat := mkCatalog(model.CompareEngineSemver, []ChannelInfo{mkChannel("stable", 30)}, v10, v15, v20, v30)

	res, err := SelectTarget(cat, defaultReq(&v10))
	if err != nil {
		t.Fatal(err)
	}
	targetRef(t, res, "1.5.0")
	if !res.IsMandatory || res.Reason != ReasonIntermediate || res.RelayHops != 2 {
		t.Fatalf("1.0 expected hop to 1.5 hops=2, got %+v", res)
	}

	res, err = SelectTarget(cat, defaultReq(&v15))
	if err != nil {
		t.Fatal(err)
	}
	targetRef(t, res, "2.0.0")
	if !res.IsMandatory || res.Reason != ReasonIntermediate || res.RelayHops != 1 {
		t.Fatalf("1.5 expected hop to 2.0 hops=1, got %+v", res)
	}

	res, err = SelectTarget(cat, defaultReq(&v20))
	if err != nil {
		t.Fatal(err)
	}
	targetRef(t, res, "3.0.0")
	if res.IsMandatory || res.Reason != ReasonNormal || res.RelayHops != 0 {
		t.Fatalf("2.0 expected direct 3.0, got %+v", res)
	}
}

// AC11：Token 保护渠道缺头/错头不 403，跳过该渠道；公开渠道仍可升级。
func TestSelectTarget_TokenProtectedFallback(t *testing.T) {
	cur := mkVersion("stable", model.VersionStatusPublished, intP(10), semP("1.0.0"))
	mkLine(&cur, "windows", "x86_64", model.VersionLineStatusReady, mkPkg("p-1.0.0.zip", 10, nil))
	pub := mkVersion("stable", model.VersionStatusPublished, intP(20), semP("2.0.0"))
	mkLine(&pub, "windows", "x86_64", model.VersionLineStatusReady, mkPkg("p-2.0.0.zip", 20, nil))
	secret := mkVersion("insider", model.VersionStatusPublished, intP(30), semP("3.0.0"))
	mkLine(&secret, "windows", "x86_64", model.VersionLineStatusReady, mkPkg("p-3.0.0.zip", 30, nil))
	hash := sha256Hex("s3cret")
	cat := mkCatalog(model.CompareEngineSemver, []ChannelInfo{
		mkChannel("stable", 30),
		{Slug: "insider", StabilityRank: 30, Enabled: true, TokenProtected: true, TokenHash: hash},
	}, cur, pub, secret)

	res, err := SelectTarget(cat, defaultReq(&cur))
	if err != nil {
		t.Fatal(err)
	}
	targetRef(t, res, "2.0.0")

	res, err = SelectTarget(cat, Request{Current: &cur, OS: "windows", Arch: "x86_64", ChannelToken: "wrong"})
	if err != nil {
		t.Fatal(err)
	}
	targetRef(t, res, "2.0.0")

	res, err = SelectTarget(cat, Request{Current: &cur, OS: "windows", Arch: "x86_64", ChannelToken: "s3cret"})
	if err != nil {
		t.Fatal(err)
	}
	targetRef(t, res, "3.0.0")

	only := mkCatalog(model.CompareEngineSemver, []ChannelInfo{
		mkChannel("stable", 30),
		{Slug: "insider", StabilityRank: 30, Enabled: true, TokenProtected: true, TokenHash: hash},
	}, cur, secret)
	res, err = SelectTarget(only, defaultReq(&cur))
	if err != nil {
		t.Fatal(err)
	}
	if res.Target != nil {
		t.Fatalf("missing token must 204 when no public candidate, got %+v", res)
	}
}

// 不可见渠道不得被其他渠道自动升入；当前渠道或 channel query 可进入。
func TestSelectTarget_UnlistedNotAutoEntered(t *testing.T) {
	cur := mkVersion("stable", model.VersionStatusPublished, intP(10), semP("1.0.0"))
	mkLine(&cur, "windows", "x86_64", model.VersionLineStatusReady, mkPkg("p-1.0.0.zip", 10, nil))
	pub := mkVersion("stable", model.VersionStatusPublished, intP(20), semP("2.0.0"))
	mkLine(&pub, "windows", "x86_64", model.VersionLineStatusReady, mkPkg("p-2.0.0.zip", 20, nil))
	hidden := mkVersion("insider", model.VersionStatusPublished, intP(30), semP("3.0.0"))
	mkLine(&hidden, "windows", "x86_64", model.VersionLineStatusReady, mkPkg("p-3.0.0.zip", 30, nil))
	cat := mkCatalog(model.CompareEngineSemver, []ChannelInfo{
		mkChannel("stable", 30),
		{Slug: "insider", StabilityRank: 30, Enabled: true, Unlisted: true},
	}, cur, pub, hidden)

	res, err := SelectTarget(cat, defaultReq(&cur))
	if err != nil {
		t.Fatal(err)
	}
	targetRef(t, res, "2.0.0")

	res, err = SelectTarget(cat, Request{Current: &cur, OS: "windows", Arch: "x86_64", ChannelQuery: "insider"})
	if err != nil {
		t.Fatal(err)
	}
	targetRef(t, res, "3.0.0")

	onHidden := mkVersion("insider", model.VersionStatusPublished, intP(10), semP("1.0.0"))
	mkLine(&onHidden, "windows", "x86_64", model.VersionLineStatusReady, mkPkg("p-1.0.0-i.zip", 10, nil))
	res, err = SelectTarget(cat, defaultReq(&onHidden))
	if err != nil {
		t.Fatal(err)
	}
	targetRef(t, res, "3.0.0")
}

// AC7：线级 min_os / min_api_level；二者都设时 AND；无矩阵回退。
func TestSelectTarget_LineMinAPILevel(t *testing.T) {
	cur := mkVersion("stable", model.VersionStatusPublished, intP(10), semP("1.0.0"))
	mkLine(&cur, "android", "arm64", model.VersionLineStatusReady, mkPkg("p-1.0.0.apk", 10, nil))
	low := mkVersion("stable", model.VersionStatusPublished, intP(11), semP("1.1.0"))
	mkLine(&low, "android", "arm64", model.VersionLineStatusReady, mkPkg("p-1.1.0.apk", 11, nil))
	api := 31
	low.Lines[0].MinAPILevel = &api
	ok := mkVersion("stable", model.VersionStatusPublished, intP(12), semP("1.2.0"))
	mkLine(&ok, "android", "arm64", model.VersionLineStatusReady, mkPkg("p-1.2.0.apk", 12, nil))
	okAPI := 21
	ok.Lines[0].MinAPILevel = &okAPI

	cat := mkCatalog(model.CompareEngineSemver, []ChannelInfo{mkChannel("stable", 30)}, cur, low, ok)
	cat.Matrix = &MatrixInfo{OS: "android", Arch: "arm64", PackageType: model.PackageTypeSingleFile}

	res, err := SelectTarget(cat, Request{Current: &cur, OS: "android", Arch: "arm64", OSVersion: "29"})
	if err != nil {
		t.Fatal(err)
	}
	targetRef(t, res, "1.2.0")

	cur.Lines[0].MinAPILevel = &api
	if _, err := SelectTarget(cat, Request{Current: &cur, OS: "android", Arch: "arm64", OSVersion: "29"}); !errors.Is(err, ErrMinOSNotMet) {
		t.Fatalf("expected ErrMinOSNotMet, got %v", err)
	}

	cur.Lines[0].MinAPILevel = nil
	cur.Lines[0].MinOS = strP("10.0")
	ok.Lines[0].MinOS = strP("10.0")
	ok.Lines[0].MinAPILevel = &okAPI
	if _, err := SelectTarget(cat, Request{Current: &cur, OS: "android", Arch: "arm64", OSVersion: "6.1"}); !errors.Is(err, ErrMinOSNotMet) {
		t.Fatalf("current min_os AND must fail, got %v", err)
	}
}
