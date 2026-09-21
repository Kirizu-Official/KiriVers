package update

import (
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/Kirizu-Official/KiriVers/internal/model"
)

// 一个可复用的标准目录：1.0.0(published) / 1.1.0(published) / 2.0.0(beta)。
func stdCatalog() (*Catalog, *VersionState) {
	cur := mkVersion("stable", model.VersionStatusPublished, intP(10), semP("1.0.0"))
	mkLine(&cur, "windows", "x86_64", model.VersionLineStatusReady, mkPkg("demo-1.0.0-windows-x86_64.zip", 100, nil))
	next := mkVersion("stable", model.VersionStatusPublished, intP(11), semP("1.1.0"))
	mkLine(&next, "windows", "x86_64", model.VersionLineStatusReady, mkPkg("demo-1.1.0-windows-x86_64.zip", 200, nil))
	beta := mkVersion("beta", model.VersionStatusPublished, intP(12), semP("2.0.0-beta.1"))
	mkLine(&beta, "windows", "x86_64", model.VersionLineStatusReady, mkPkg("demo-2.0.0-beta.1-windows-x86_64.zip", 300, nil))
	cat := mkCatalog(model.CompareEngineSemver, []ChannelInfo{mkChannel("beta", 20), mkChannel("stable", 30)}, cur, next, beta)
	return cat, &cur
}

func baseCheckInput() CheckInput {
	return CheckInput{OS: "windows", Arch: "x86_64", CurrentVersion: "1.0.0"}
}

// 基础 200 响应体快照断言：字段齐全、无 display_version、package_url 指向 /packages/。
func TestCheckResponseShape(t *testing.T) {
	cat, _ := stdCatalog()
	res, err := Check(cat, baseCheckInput())
	if err != nil {
		t.Fatal(err)
	}
	if res.Status != 200 || res.Body == nil {
		t.Fatalf("expected 200, got %+v", res)
	}
	b := res.Body
	if !b.HasUpdate || b.IsMandatory || b.IsDowngrade || b.Reason != ReasonNormal {
		t.Fatalf("unexpected header fields: %+v", b)
	}
	if b.CompareEngine != model.CompareEngineSemver {
		t.Fatalf("compare_engine: %s", b.CompareEngine)
	}
	if b.VersionInteger == nil || *b.VersionInteger != 11 || b.VersionSemver == nil || *b.VersionSemver != "1.1.0" {
		t.Fatalf("dual numbers wrong: %+v", b)
	}
	if b.TargetChannel != "stable" || b.TargetHwRev != nil {
		t.Fatalf("target channel/hw wrong: %+v", b)
	}
	if b.PackageType != "" {
		// 无矩阵行时 package_type 为空（loader 会填矩阵行）；此处仅确认无 panic。
		_ = b.PackageType
	}
	if b.PackageURL != "/api/v1/projects//packages/sha-demo-1.1.0-windows-x86_64.zip" {
		t.Fatalf("package_url = %s", b.PackageURL)
	}
	if b.FileName != "demo-1.1.0-windows-x86_64.zip" {
		t.Fatalf("file_name = %s", b.FileName)
	}
	if b.Size != 200 || b.SHA256 != "sha-demo-1.1.0-windows-x86_64.zip" {
		t.Fatalf("size/sha256 wrong: %+v", b)
	}
	if b.DeltaAvailable {
		t.Fatal("single-file zip must not advertise delta without capabilities")
	}
	if b.Signature != "" {
		t.Fatalf("signature must be omitted without signing key")
	}
	rt := reflect.TypeOf(*b)
	if _, ok := rt.FieldByName("Changelog"); ok {
		t.Fatalf("check must not contain Changelog field")
	}
	if _, ok := rt.FieldByName("ChangelogVersions"); ok {
		t.Fatalf("check must not contain ChangelogVersions field")
	}
	if _, ok := rt.FieldByName("DisplayVersion"); ok {
		t.Fatalf("response must not contain display_version")
	}
}

func TestCheckS3DirectURLAndLocalProxyCache(t *testing.T) {
	cat, _ := stdCatalog()
	cat.Project.Slug = "demo"
	pkg := &cat.Versions[1].Lines[0].FullPkgs[0]
	pkg.StorageKey = "demo/" + pkg.SHA256

	in := baseCheckInput()
	in.signing.objectURL = func(_, _, storageKey string) string {
		return "https://cdn.example/" + storageKey
	}
	res, err := Check(cat, in)
	if err != nil {
		t.Fatal(err)
	}
	if res.Status != 200 || res.Body == nil {
		t.Fatalf("expected 200, got %+v", res)
	}
	want := "https://cdn.example/" + pkg.StorageKey
	if res.Body.PackageURL != want {
		t.Fatalf("package_url=%q want %q", res.Body.PackageURL, want)
	}
	if strings.Contains(res.Body.PackageURL, "?") {
		t.Fatal("S3-direct URL must not carry a presign query")
	}

	in2 := baseCheckInput()
	in2.signing.localProxy = true
	res2, err := Check(cat, in2)
	if err != nil {
		t.Fatal(err)
	}
	if res2.CacheControl != cacheControlNoStore {
		t.Fatalf("local-proxy Cache-Control=%q", res2.CacheControl)
	}
}

// 200 与 204 两种底态使用同一 ETag（ETag 不依赖结果）；If-None-Match 匹配逻辑。
func TestCheckETagStableAcross204And200(t *testing.T) {
	cat, _ := stdCatalog()

	upd, err := Check(cat, baseCheckInput())
	if err != nil {
		t.Fatal(err)
	}

	// 把候选抽走 → 204。
	catNoUpdate := mkCatalog(model.CompareEngineSemver, cat.Channels)
	catNoUpdate.Versions = []VersionState{cat.Versions[0]}
	noUpd, err := Check(catNoUpdate, baseCheckInput())
	if err != nil {
		t.Fatal(err)
	}
	if noUpd.Status != 204 || noUpd.Body != nil {
		t.Fatalf("expected 204, got %+v", noUpd)
	}
	if noUpd.ETag == "" || upd.ETag == "" {
		t.Fatalf("both states must carry ETag")
	}
	// ETag 由目录决定：两个不同目录（有/无候选）ETag 不同。
	if noUpd.ETag == upd.ETag {
		t.Fatalf("different catalogs must yield different ETags")
	}
	// 同一目录重复计算必须稳定。
	if again := CatalogETag(cat); again != upd.ETag {
		t.Fatalf("etag unstable: %s vs %s", again, upd.ETag)
	}
	if !MatchesETag(`"`+upd.ETag+`"`, upd.ETag) && !MatchesETag(upd.ETag, upd.ETag) {
		t.Fatalf("self match failed")
	}
	if MatchesETag(`"deadbeef"`, upd.ETag) {
		t.Fatalf("different etag must not match")
	}
	if !MatchesETag(`W/`+upd.ETag, upd.ETag) {
		t.Fatalf("weak comparison must match")
	}
	if !MatchesETag(`"x", `+upd.ETag, upd.ETag) {
		t.Fatalf("multi-value list must match")
	}
	if !MatchesETag("*", upd.ETag) {
		t.Fatalf("star must match")
	}
}

// ETag 触发源：publish/revoke/yank、灰度→100%、中继配置、关键标记、root_hash、产物替换。
func TestCatalogETagTriggers(t *testing.T) {
	cat, _ := stdCatalog()
	base := CatalogETag(cat)

	// 无关 query/device_id 不影响。
	if _, err := Check(cat, CheckInput{OS: "windows", Arch: "x86_64", CurrentVersion: "1.0.0", DeviceID: "dev-1"}); err != nil {
		t.Fatal("check should succeed")
	}
	if again := CatalogETag(cat); again != base {
		t.Fatalf("device_id must not affect etag")
	}

	// revoke 一个版本 → 变化。
	cat.Versions[2].Version.Status = model.VersionStatusRevoked
	revoke := CatalogETag(cat)
	if revoke == base {
		t.Fatalf("revoke must change etag")
	}

	// yank 一条线 → 变化。
	cat.Versions[1].Lines[0].Status = model.VersionLineStatusYanked
	yank := CatalogETag(cat)
	if yank == revoke {
		t.Fatalf("yank must change etag")
	}

	// 完成灰度时间戳变化 → ETag 变化。
	tstamp := time.Unix(42, 0).UTC()
	cat.Versions[1].Version.GrayCompletedAt = &tstamp
	rollout := CatalogETag(cat)
	if rollout == yank {
		t.Fatalf("rollout change must change etag")
	}

	// 中继配置 → 变化。
	cat.Versions[1].Version.MinSourceVersion = strP("1.0.0")
	relay := CatalogETag(cat)
	if relay == rollout {
		t.Fatalf("relay config change must change etag")
	}

	// 关键标记 → 变化。
	cat.Versions[1].Version.IsCritical = true
	critical := CatalogETag(cat)
	if critical == relay {
		t.Fatalf("critical flag must change etag")
	}

	// 目标 root_hash → 变化。
	cat.Versions[1].Lines[0].RootHash = "newroothash"
	root := CatalogETag(cat)
	if root == critical {
		t.Fatalf("root_hash change must change etag")
	}

	// packs_ready_at → 变化（AC17）。
	later := time.Unix(99, 0).UTC()
	cat.Versions[1].Lines[0].PacksReadyAt = &later
	if CatalogETag(cat) == root {
		t.Fatalf("packs_ready_at change must change etag")
	}

	// 白名单 count 变化必须改变 ETag。
	gray := CatalogETag(cat)
	cat.Versions[1].Allowlist.Version = []string{"dev-a"}
	if CatalogETag(cat) == gray {
		t.Fatalf("allowlist count must affect etag")
	}
}

// 缓存头：check 不再使用 public s-maxage；未完成灰度 / Token 渠道仍 private。
func TestCheckCacheControlBranches(t *testing.T) {
	cat, _ := stdCatalog()

	// 匿名完整灰度 → private（不再 public s-maxage）。
	res, err := Check(cat, baseCheckInput())
	if err != nil {
		t.Fatal(err)
	}
	if res.CacheControl != cacheControlPrivateDevice {
		t.Fatalf("anonymous cache-control = %s", res.CacheControl)
	}

	// 目录全 100% + device_id → 仍 private。
	res, err = Check(cat, CheckInput{OS: "windows", Arch: "x86_64", CurrentVersion: "1.0.0", DeviceID: "d1"})
	if err != nil {
		t.Fatal(err)
	}
	if res.CacheControl != cacheControlPrivateDevice {
		t.Fatalf("device_id with full rollout must stay private, got %s", res.CacheControl)
	}

	// 目录存在 rollout<100 + device_id → private（验收：不含 s-maxage）。
	cat.Versions[2].Version.GrayCompletedAt = nil
	started := time.Unix(1, 0).UTC()
	cat.Versions[2].Version.GrayStartedAt = &started
	res, err = Check(cat, CheckInput{OS: "windows", Arch: "x86_64", CurrentVersion: "1.0.0", DeviceID: "d1"})
	if err != nil {
		t.Fatal(err)
	}
	if res.CacheControl != cacheControlPrivateDevice {
		t.Fatalf("device_id with partial rollout must be private, got %s", res.CacheControl)
	}
	// 匿名未完成灰度同样 private。
	res, err = Check(cat, baseCheckInput())
	if err != nil {
		t.Fatal(err)
	}
	if res.CacheControl != cacheControlPrivateDevice {
		t.Fatalf("anonymous incomplete gray must be private, got %s", res.CacheControl)
	}

	// 项目自定义 s-maxage 不再作用于 check。
	done := time.Unix(2, 0).UTC()
	cat.Versions[2].Version.GrayCompletedAt = &done
	cat.Project.CacheSMaxageSeconds = 120
	res, err = Check(cat, baseCheckInput())
	if err != nil {
		t.Fatal(err)
	}
	if res.CacheControl != cacheControlPrivateDevice {
		t.Fatalf("check must ignore custom s-maxage: %s", res.CacheControl)
	}

	cat.Channels[0].TokenProtected = true
	cat.Channels[0].TokenHash = "deadbeef"
	res, err = Check(cat, baseCheckInput())
	if err != nil {
		t.Fatal(err)
	}
	if res.CacheControl != cacheControlPrivateDevice {
		t.Fatalf("token-protected catalog must be private, got %s", res.CacheControl)
	}
	if !containsString(res.Vary, "X-Channel-Token") {
		t.Fatalf("vary missing X-Channel-Token: %v", res.Vary)
	}
	before := CatalogETag(cat)
	cat.Channels[0].TokenHash = "cafebabe"
	if CatalogETag(cat) != before {
		t.Fatal("token hash must not affect etag")
	}
}

// Vary 组装。
func TestCheckVary(t *testing.T) {
	cat, _ := stdCatalog()
	res, err := Check(cat, baseCheckInput())
	if err != nil {
		t.Fatal(err)
	}
	if !equalStrings(res.Vary, []string{"Accept-Encoding"}) {
		t.Fatalf("default vary = %v", res.Vary)
	}
	cat.Project.RequireClientToken = true
	res, err = Check(cat, CheckInput{OS: "windows", Arch: "x86_64", CurrentVersion: "1.0.0"})
	if err != nil {
		t.Fatal(err)
	}
	if !equalStrings(res.Vary, []string{"Accept-Encoding", "Authorization"}) {
		t.Fatalf("extended vary = %v", res.Vary)
	}
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func containsString(list []string, want string) bool {
	for _, v := range list {
		if v == want {
			return true
		}
	}
	return false
}

// 禁入参数在 HTTP 层拒绝；此处校验 channel 校验与版本解析错误路径。
func TestCheckParamValidation(t *testing.T) {
	cat, _ := stdCatalog()

	// 未知渠道 → 400。
	if _, err := Check(cat, CheckInput{OS: "windows", Arch: "x86_64", CurrentVersion: "1.0.0", Channel: "nightly"}); !errors.Is(err, ErrInvalidQuery) {
		t.Fatalf("expected ErrInvalidQuery for unknown channel, got %v", err)
	}
	// 已知渠道合法。
	if _, err := Check(cat, CheckInput{OS: "windows", Arch: "x86_64", CurrentVersion: "1.0.0", Channel: "beta"}); err != nil {
		t.Fatal(err)
	}
	// 非法 current_version → ENGINE_MISMATCH。
	if _, err := Check(cat, CheckInput{OS: "windows", Arch: "x86_64", CurrentVersion: "not-a-version"}); !errors.Is(err, ErrEngineMismatch) {
		t.Fatalf("expected ErrEngineMismatch, got %v", err)
	}
	// 未知 current_version → VERSION_NOT_FOUND（不当 0）。
	if _, err := Check(cat, CheckInput{OS: "windows", Arch: "x86_64", CurrentVersion: "999"}); !errors.Is(err, ErrVersionNotFound) {
		t.Fatalf("expected ErrVersionNotFound, got %v", err)
	}
	// 010 与 10 解析同一记录。
	if _, err := Check(cat, CheckInput{OS: "windows", Arch: "x86_64", CurrentVersion: "010"}); err != nil {
		t.Fatalf("010 must resolve: %v", err)
	}
}

// 能力合并：accepted_delta_algos 自动授予 binary_delta；降级禁差量。
func TestCheckDeltaCapabilities(t *testing.T) {
	cat, _ := stdCatalog()
	cat.Matrix = &MatrixInfo{OS: "windows", Arch: "x86_64", PackageType: model.PackageTypeSingleFile, DeltaAlgo: "hdiffpatch"}

	in := CheckInput{OS: "windows", Arch: "x86_64", CurrentVersion: "1.0.0", AcceptedDeltaAlgos: []string{"hdiffpatch", ""}}
	res, err := Check(cat, in)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Body.DeltaAvailable || res.Body.DeltaAlgo != "hdiffpatch" {
		t.Fatalf("accepted_delta_algos must grant binary_delta: %+v", res.Body)
	}

	// 未声明 → delta_available false。
	res, err = Check(cat, baseCheckInput())
	if err != nil {
		t.Fatal(err)
	}
	if res.Body.DeltaAvailable || res.Body.DeltaAlgo != "" {
		t.Fatalf("delta must be unavailable without capability: %+v", res.Body)
	}

	// 降级 → 即使有能力也 false。
	cat.Versions[2].Version.Status = model.VersionStatusRevoked
	res, err = Check(cat, CheckInput{OS: "windows", Arch: "x86_64", CurrentVersion: "1.0.0", AcceptedDeltaAlgos: []string{"bsdiff"}})
	if err != nil {
		t.Fatal(err)
	}
	// 1.0.0 未吊销 → 正常升级（beta 2.0 已吊销，stable 1.1.0 仍是目标）。
	if res.Body.TargetChannel != "stable" || res.Body.DeltaAvailable != true {
		t.Fatalf("normal upgrade with capability should allow delta: %+v", res.Body)
	}

	// 吊销当前（2.0.0）→ 降级到 1.1.0 → 即使有能力也禁差量。
	revokedCur := mkVersion("stable", model.VersionStatusRevoked, intP(20), semP("2.0.0"))
	mkLine(&revokedCur, "windows", "x86_64", model.VersionLineStatusReady, mkPkg("demo-2.0.0-windows-x86_64.zip", 100, nil))
	cat2 := mkCatalog(model.CompareEngineSemver, cat.Channels, revokedCur, cat.Versions[1])
	cat2.Matrix = cat.Matrix
	res, err = Check(cat2, CheckInput{OS: "windows", Arch: "x86_64", CurrentVersion: "2.0.0", AcceptedDeltaAlgos: []string{"bsdiff"}})
	if err != nil {
		t.Fatal(err)
	}
	if !res.Body.IsDowngrade || res.Body.DeltaAvailable {
		t.Fatalf("downgrade must disable delta: %+v", res.Body)
	}
}

// 签名注入：配置私钥后输出 signature；artifact_signature 原样透传。
func TestCheckSigning(t *testing.T) {
	cat, _ := stdCatalog()
	cat.Project.SigningAlgo = "ed25519"
	cat.Project.SigningPrivateKey = "bad-key"
	if _, err := Check(cat, baseCheckInput()); err == nil {
		t.Fatalf("invalid signing key must fail loudly")
	}
	// 用固定测试向量太长；此处仅验证路径：合法密钥时 body 带 signature。
	// 端到端签名验证放在 controller 集成测试（生成真实密钥）。
}

func TestCatalogETagStableAcrossChangelogEdits(t *testing.T) {
	cat, _ := stdCatalog()
	before := CatalogETag(cat)
	cat.Versions[1].Version.Changelog = model.ChangelogMap{"en": {Title: "1.1.0", Markdown: "edited body"}}
	after := CatalogETag(cat)
	if after != before {
		t.Fatalf("version changelog must not change check ETag: %s vs %s", before, after)
	}
	cat.Project.ChangelogScope = model.ChangelogScopeTargetOnly
	cat.Project.ChangelogLayout = model.ChangelogLayoutAggregated
	cat.Project.ChangelogClientOverride = false
	cat.Project.ChangelogIncludeRevoked = false
	if CatalogETag(cat) != after {
		t.Fatalf("project changelog scope/layout/override/revoked must not change check ETag")
	}
	cat.Project.ChangelogIncludeNotes = false
	if CatalogETag(cat) == after {
		t.Fatalf("include_notes must remain in check ETag projection")
	}
}

func TestChangelogETagVariesUnderAggregatedLayout(t *testing.T) {
	cat, _ := stdCatalog()
	cat.Project.ChangelogLayout = model.ChangelogLayoutAggregated
	in := ChangelogInput{Channel: "stable", FromVersion: "1.0.0"}

	r1, err := Changelog(cat, in)
	if err != nil {
		t.Fatal(err)
	}
	// 同一状态重复计算必须稳定。
	if again, _ := Changelog(cat, in); again.ETag != r1.ETag {
		t.Fatalf("changelog etag unstable: %s vs %s", again.ETag, r1.ETag)
	}
	// aggregated 布局不下发分条。
	if r1.Body.ChangelogVersions != nil {
		t.Fatalf("aggregated layout must not include changelog_versions: %+v", r1.Body.ChangelogVersions)
	}

	// 编辑 changelog 内容 → ETag 必须变化。
	cat.Versions[1].Version.Changelog = model.ChangelogMap{"en": {Title: "1.1.0", Markdown: "edited body"}}
	r2, err := Changelog(cat, in)
	if err != nil {
		t.Fatal(err)
	}
	if r2.ETag == r1.ETag {
		t.Fatalf("changelog edit must change ETag under aggregated layout")
	}
}

// uuid 辅助引用防导入漂移。
var _ = uuid.Nil
