package update

import (
	"errors"
	"fmt"
	"testing"

	"github.com/Kirizu-Official/KiriVers/internal/model"
)

// 构造带多语言 changelog 的版本。
func mkLogged(channel, status string, integer *int64, semver *string, log model.ChangelogMap, mutators ...func(*model.Version)) VersionState {
	vs := mkVersion(channel, status, integer, semver, mutators...)
	vs.Version.Changelog = log
	return vs
}

func enLog(title, body string) model.ChangelogMap {
	return model.ChangelogMap{"en": {Title: title, Markdown: body}}
}

// range_all 含未打包中间版、按比较键新→旧、(from, to] 半开区间。
func TestBuildChangelogRangeAll(t *testing.T) {
	cur := mkLogged("stable", model.VersionStatusPublished, intP(10), semP("1.0.0"), enLog("1.0.0", "first"))
	mid := mkLogged("stable", model.VersionStatusPublished, intP(11), semP("1.1.0"), enLog("1.1.0", "windows only release"))
	below := mkLogged("stable", model.VersionStatusPublished, intP(9), semP("0.9.0"), enLog("0.9.0", "below"))
	draft := mkLogged("stable", model.VersionStatusDraft, intP(14), semP("1.4.0"), enLog("1.4.0", "draft"))
	above := mkLogged("stable", model.VersionStatusPublished, intP(15), semP("1.5.0"), enLog("1.5.0", "above"))

	// 窗口版本（无本平台切片）也进入 range_all。
	cat := mkCatalog(model.CompareEngineSemver, []ChannelInfo{mkChannel("stable", 30)}, below, cur, mid, draft, above)

	agg, entries, err := BuildChangelog(cat.Versions, &cur, &above, &above, ChangelogQuery{
		Engine:         model.CompareEngineSemver,
		Scope:          model.ChangelogScopeRangeAll,
		Layout:         model.ChangelogLayoutBoth,
		IncludeRevoked: true,
		IncludeNotes:   true,
		LocaleChain:    []string{"en"},
	})
	if err != nil {
		t.Fatal(err)
	}
	// (1.0.0, 1.5.0]：含 1.1.0、1.5.0；不含 draft 与区间外。
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d: %+v", len(entries), entries)
	}
	if *entries[0].VersionSemver != "1.5.0" || *entries[1].VersionSemver != "1.1.0" {
		t.Fatalf("expected newest-first order, got %v then %v", *entries[0].VersionSemver, *entries[1].VersionSemver)
	}
	if agg == "" || !containsInOrder(agg, "## 1.5.0", "above", "---", "## 1.1.0", "windows only release") {
		t.Fatalf("unexpected aggregated render:\n%s", agg)
	}
}

func containsInOrder(s string, parts ...string) bool {
	pos := 0
	for _, p := range parts {
		idx := indexOf(s[pos:], p)
		if idx < 0 {
			return false
		}
		pos += idx + len(p)
	}
	return true
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}

// range_platform 不含无本平台产物中间版本；target_only 只有目标。
func TestBuildChangelogRangePlatformAndTargetOnly(t *testing.T) {
	cur := mkLogged("stable", model.VersionStatusPublished, intP(10), semP("1.0.0"), enLog("t", "b"))
	mkLine(&cur, "windows", "x86_64", model.VersionLineStatusReady)

	winOnly := mkLogged("stable", model.VersionStatusPublished, intP(11), semP("1.1.0"), enLog("t", "b"))
	mkLine(&winOnly, "windows", "x86_64", model.VersionLineStatusReady)

	macOnly := mkLogged("stable", model.VersionStatusPublished, intP(12), semP("1.2.0"), enLog("t", "b"))
	mkLine(&macOnly, "macos", "x86_64", model.VersionLineStatusReady)

	yankedOld := mkLogged("stable", model.VersionStatusPublished, intP(13), semP("1.3.0"), enLog("t", "b"))
	mkLine(&yankedOld, "windows", "x86_64", model.VersionLineStatusYanked) // 曾就绪后被撤

	top := mkLogged("stable", model.VersionStatusPublished, intP(14), semP("1.4.0"), enLog("t", "b"))
	mkLine(&top, "windows", "x86_64", model.VersionLineStatusReady)

	cat := mkCatalog(model.CompareEngineSemver, []ChannelInfo{mkChannel("stable", 30)}, cur, winOnly, macOnly, yankedOld, top)

	q := ChangelogQuery{
		Engine:         model.CompareEngineSemver,
		Scope:          model.ChangelogScopeRangePlatform,
		Layout:         model.ChangelogLayoutStructured,
		IncludeRevoked: true,
		IncludeNotes:   true,
		LocaleChain:    []string{"en"},
		OS:             "windows",
		Arch:           "x86_64",
	}
	_, entries, err := BuildChangelog(cat.Versions, &cur, &top, &top, q)
	if err != nil {
		t.Fatal(err)
	}
	// mac-only 的 1.2.0 不在 windows 平台范围内。
	got := ""
	for _, e := range entries {
		got += *e.VersionSemver + ","
		if !e.HadArtifactForRequestPlatform {
			t.Fatalf("range_platform entries must have had_artifact=true: %+v", e)
		}
	}
	if got != "1.4.0,1.3.0,1.1.0," {
		t.Fatalf("unexpected range_platform entries: %s", got)
	}

	// target_only：只有目标单条。
	q.Scope = model.ChangelogScopeTargetOnly
	_, entries, err = BuildChangelog(cat.Versions, &cur, &top, &winOnly, q)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || *entries[0].VersionSemver != "1.1.0" {
		t.Fatalf("target_only must contain only target, got %+v", entries)
	}
}

// 语言回退：请求 locale → default_locale → 任意已填（字典序最小 key）。
func TestBuildChangelogLocaleFallback(t *testing.T) {
	cur := mkLogged("stable", model.VersionStatusPublished, intP(10), semP("1.0.0"), model.ChangelogMap{
		"zh-CN": {Title: "中文标题", Markdown: "中文内容"},
		"en":    {Title: "EN Title", Markdown: "english body"},
	})
	top := mkLogged("stable", model.VersionStatusPublished, intP(11), semP("1.1.0"), model.ChangelogMap{
		"ja": {Title: "日本語", Markdown: "日本語本文"},
	})

	base := mkLogged("stable", model.VersionStatusPublished, intP(9), semP("0.9.0"), enLog("base", "base body"))
	cat := mkCatalog(model.CompareEngineSemver, []ChannelInfo{mkChannel("stable", 30)}, base, cur, top)
	q := ChangelogQuery{
		Engine:         model.CompareEngineSemver,
		Scope:          model.ChangelogScopeRangeAll,
		Layout:         model.ChangelogLayoutStructured,
		IncludeRevoked: true,
		LocaleChain:    []string{"zh-CN", "en"},
	}

	// 请求 zh-CN 命中（1.0.0 有 zh-CN）；1.1.0 无链内语言 → 任意已填回退 ja。
	_, entries, err := BuildChangelog(cat.Versions, &base, &top, &top, q)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %+v", entries)
	}
	if entries[1].Title != "中文标题" {
		t.Fatalf("expected zh-CN entry, got %+v", entries[1])
	}
	// 顶部版本无 zh-CN → 回退 en？链中没有 en 时 → 任意已填（字典序最小 ja）。
	q.LocaleChain = []string{"zh-CN"}
	_, entries, err = BuildChangelog(cat.Versions, &cur, &top, &top, q)
	if err != nil {
		t.Fatal(err)
	}
	if entries[0].Changelog != "日本語本文" {
		t.Fatalf("expected any-filled fallback (ja), got %+v", entries[0])
	}
	// 链中 en 命中（fr 未填 → 回退 en）。
	q.LocaleChain = []string{"fr", "en"}
	_, entries, err = BuildChangelog(cat.Versions, &base, &top, &top, q)
	if err != nil {
		t.Fatal(err)
	}
	if entries[0].Changelog != "日本語本文" || entries[1].Changelog != "english body" {
		t.Fatalf("expected en fallback for 1.0.0, got %+v", entries)
	}
}

// revoked 过滤与状态标注；非法 scope/layout 报错。
func TestBuildChangelogRevokedAndValidation(t *testing.T) {
	cur := mkLogged("stable", model.VersionStatusPublished, intP(10), semP("1.0.0"), enLog("t", "b"))
	revoked := mkLogged("stable", model.VersionStatusRevoked, intP(11), semP("1.1.0"), enLog("t", "b"))
	top := mkLogged("stable", model.VersionStatusPublished, intP(12), semP("1.2.0"), enLog("t", "b"))
	cat := mkCatalog(model.CompareEngineSemver, []ChannelInfo{mkChannel("stable", 30)}, cur, revoked, top)

	q := ChangelogQuery{
		Engine:      model.CompareEngineSemver,
		Scope:       model.ChangelogScopeRangeAll,
		Layout:      model.ChangelogLayoutStructured,
		LocaleChain: []string{"en"},
	}
	// include_revoked=true → 带上 status=revoked 的条目。
	q.IncludeRevoked = true
	_, entries, err := BuildChangelog(cat.Versions, &cur, &top, &top, q)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 || entries[0].Status != model.VersionStatusPublished || entries[1].Status != model.VersionStatusRevoked {
		t.Fatalf("unexpected entries with revoked: %+v", entries)
	}
	// include_revoked=false → 剔除。
	q.IncludeRevoked = false
	_, entries, err = BuildChangelog(cat.Versions, &cur, &top, &top, q)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("revoked must be excluded, got %+v", entries)
	}

	// 非法取值。
	if _, _, err := BuildChangelog(cat.Versions, &cur, &top, &top, ChangelogQuery{Scope: "bogus", Layout: "both"}); err == nil {
		t.Fatal("expected error for invalid scope")
	}
	if _, _, err := BuildChangelog(cat.Versions, &cur, &top, &top, ChangelogQuery{Scope: "range_all", Layout: "bogus"}); err == nil {
		t.Fatal("expected error for invalid layout")
	}
}

// 无标题条目在 aggregated 中只渲染正文；BuildLocaleChain 去重与 Accept-Language 首段。
func TestAggregatedNoTitleAndLocaleChain(t *testing.T) {
	cur := mkLogged("stable", model.VersionStatusPublished, intP(10), semP("1.0.0"), model.ChangelogMap{"en": {Markdown: "plain body"}})
	top := mkLogged("stable", model.VersionStatusPublished, intP(11), semP("1.1.0"), model.ChangelogMap{"en": {Title: "T", Markdown: "B"}})
	cat := mkCatalog(model.CompareEngineSemver, []ChannelInfo{mkChannel("stable", 30)}, cur, top)

	agg, _, err := BuildChangelog(cat.Versions, nil, &top, &top, ChangelogQuery{
		Engine: model.CompareEngineSemver, Scope: model.ChangelogScopeRangeAll,
		Layout: model.ChangelogLayoutAggregated, IncludeRevoked: true, LocaleChain: []string{"en"},
	})
	if err != nil {
		t.Fatal(err)
	}
	want := "## T\n\nB\n\n---\n\nplain body"
	if agg != want {
		t.Fatalf("aggregated = %q, want %q", agg, want)
	}
	// aggregated 布局渲染文本；分条恒返回（独立端点 ETag 依赖分条集合）。
	agg2, entries, _ := BuildChangelog(cat.Versions, &cur, &top, &top, ChangelogQuery{
		Engine: model.CompareEngineSemver, Scope: model.ChangelogScopeRangeAll,
		Layout: model.ChangelogLayoutAggregated, IncludeRevoked: true, LocaleChain: []string{"en"},
	})
	if agg2 == "" || entries == nil {
		t.Fatalf("aggregated layout must render text and still return entries for ETag")
	}

	chain := BuildLocaleChain("", "", "zh-CN,zh;q=0.9,en;q=0.8", "en")
	if len(chain) != 2 || chain[0] != "zh-CN" || chain[1] != "en" {
		t.Fatalf("unexpected chain: %v", chain)
	}
	chain = BuildLocaleChain("en", "fr", "", "en")
	if len(chain) != 2 || chain[0] != "en" || chain[1] != "fr" {
		t.Fatalf("unexpected dedup chain: %v", chain)
	}
}

func TestChangelogWindowDefaultAndTruncate(t *testing.T) {
	var vers []VersionState
	for i := 0; i <= 6; i++ {
		sem := fmt.Sprintf("1.%d.0", i)
		vs := mkLogged("stable", model.VersionStatusPublished, intP(int64(10+i)), semP(sem), enLog(sem, "b"))
		mkLine(&vs, "windows", "x86_64", model.VersionLineStatusReady)
		vers = append(vers, vs)
	}
	cat := mkCatalog(model.CompareEngineSemver, []ChannelInfo{mkChannel("stable", 30)}, vers...)
	cat.Project.ChangelogDefaultEntries = 5
	cat.Project.ChangelogMaxEntries = 2
	cat.Project.ChangelogLayout = model.ChangelogLayoutStructured

	noFrom, err := Changelog(cat, ChangelogInput{Channel: "stable", OS: "windows", Arch: "x86_64", Layout: model.ChangelogLayoutStructured})
	if err != nil {
		t.Fatal(err)
	}
	if n := len(noFrom.Body.ChangelogVersions); n != 5 {
		t.Fatalf("no-from count=%d want 5", n)
	}
	if *noFrom.Body.ChangelogVersions[0].VersionSemver != "1.6.0" || *noFrom.Body.ChangelogVersions[4].VersionSemver != "1.2.0" {
		t.Fatalf("no-from order=%+v", noFrom.Body.ChangelogVersions)
	}

	withFrom, err := Changelog(cat, ChangelogInput{
		Channel: "stable", FromVersion: "1.0.0", OS: "windows", Arch: "x86_64", Layout: model.ChangelogLayoutStructured,
	})
	if err != nil {
		t.Fatal(err)
	}
	if n := len(withFrom.Body.ChangelogVersions); n != 2 {
		t.Fatalf("truncate count=%d want 2", n)
	}
	if *withFrom.Body.ChangelogVersions[0].VersionSemver != "1.6.0" || *withFrom.Body.ChangelogVersions[1].VersionSemver != "1.5.0" {
		t.Fatalf("truncate order=%+v", withFrom.Body.ChangelogVersions)
	}

	if _, err := Changelog(cat, ChangelogInput{Channel: "nightly"}); !errors.Is(err, ErrInvalidQuery) {
		t.Fatalf("unknown channel err=%v", err)
	}
}
