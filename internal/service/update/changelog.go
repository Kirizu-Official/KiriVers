package update

import (
	"errors"
	"sort"
	"strings"

	"github.com/Kirizu-Official/KiriVers/internal/model"
)

// ErrInvalidChangelogQuery 非法 changelog_scope / changelog_layout 取值
// → HTTP 400 CHANGELOG_QUERY_INVALID。
var ErrInvalidChangelogQuery = errors.New("invalid changelog scope or layout")

// ChangelogEntry 是 changelog 引擎的分条输出：对外按 json tag 序列化，
// ID 仅参与 changelog ETag 计算（json:"-" 不下发）。
type ChangelogEntry struct {
	ID             string  `json:"-"`
	VersionInteger *int64  `json:"version_integer"`
	VersionSemver  *string `json:"version_semver"`
	Channel        string  `json:"channel"`
	Status         string  `json:"status"`
	Title          string  `json:"title,omitempty"`
	Changelog      string  `json:"changelog"`
	PlatformNotes  string  `json:"platform_notes,omitempty"`
	// HadArtifactForRequestPlatform 表示该 Version 在请求平台
	// (os, arch) 上是否曾经就绪（存在 ready/yanked/disabled 切片记录）。
	HadArtifactForRequestPlatform bool `json:"had_artifact_for_request_platform"`
}

// ChangelogQuery 是 changelog 引擎的请求参数。
type ChangelogQuery struct {
	// Engine 是项目 compare_engine（区间比较需要）。
	Engine string
	// Scope：range_all | range_platform | target_only。
	Scope string
	// Layout：aggregated | structured | both。
	Layout string
	// IncludeRevoked 吊销版本是否出现在列表中（出现时 status=revoked）。
	IncludeRevoked bool
	// IncludeNotes 是否附带 platform_notes。
	IncludeNotes bool
	// LocaleChain 是语言回退链（请求 → default → 任意已填）。
	LocaleChain []string
	// OS / Arch 用于 range_platform 过滤与 had_artifact_for_request_platform；
	// 空则 had_artifact 恒 false 且不做平台过滤。
	OS, Arch string
}

// validScope / validLayout 校验取值。
func validScope(s string) bool {
	switch s {
	case model.ChangelogScopeRangeAll, model.ChangelogScopeRangePlatform, model.ChangelogScopeTargetOnly:
		return true
	}
	return false
}

func validLayout(s string) bool {
	switch s {
	case model.ChangelogLayoutAggregated, model.ChangelogLayoutStructured, model.ChangelogLayoutBoth:
		return true
	}
	return false
}

// BuildChangelog 是 changelog 查询引擎（§5.7）。
//
// 范围：
//   - range_all：比较键 (from, to] 内全部非 Draft Version（含当时没给本平台打包的中间版本）；
//   - range_platform：同上，再过滤「本 os/arch 当时曾就绪」；
//   - target_only：仅 target 单条。
//
// include_revoked=false 时剔除 revoked。分条数组按比较键新→旧；
// aggregated 渲染：新→旧逐条 `## {title}\n\n{markdown}`，条目间 `\n\n---\n\n`，
// 无 title 则只渲染正文。非法 scope/layout 返回 ErrInvalidChangelogQuery。
//
// 分条数组无论 layout 如何恒返回（可能为空切片）：layout 只决定呈现方式
// （是否下发分条/aggregated 文本由调用方裁决），而独立 changelog 端点的
// ETag 依赖分条集合，aggregated 布局下也必须随内容变化（§5.7）。
func BuildChangelog(versions []VersionState, from, to, target *VersionState, q ChangelogQuery) (string, []ChangelogEntry, error) {
	if !validScope(q.Scope) {
		return "", nil, ErrInvalidChangelogQuery
	}
	if !validLayout(q.Layout) {
		return "", nil, ErrInvalidChangelogQuery
	}

	var selected []*VersionState
	switch q.Scope {
	case model.ChangelogScopeTargetOnly:
		if target != nil && (q.IncludeRevoked || target.Version.Status != model.VersionStatusRevoked) {
			selected = append(selected, target)
		}
	default:
		var fromKey, toKey cmpKey
		var haveFrom, haveTo bool
		if from != nil {
			if k, ok := versionKey(q.Engine, &from.Version); ok {
				fromKey, haveFrom = k, true
			}
		}
		if to != nil {
			if k, ok := versionKey(q.Engine, &to.Version); ok {
				toKey, haveTo = k, true
			}
		}
		for i := range versions {
			v := &versions[i]
			if v.Version.Status == model.VersionStatusDraft {
				continue
			}
			if !q.IncludeRevoked && v.Version.Status == model.VersionStatusRevoked {
				continue
			}
			key, ok := versionKey(q.Engine, &v.Version)
			if !ok {
				continue
			}
			if haveFrom {
				if c, comparable := compareKeys(key, fromKey); !comparable || c <= 0 {
					continue
				}
			}
			if haveTo {
				if c, comparable := compareKeys(key, toKey); !comparable || c > 0 {
					continue
				}
			}
			if q.Scope == model.ChangelogScopeRangePlatform && !everReadyOnPlatform(v, q.OS, q.Arch) {
				continue
			}
			selected = append(selected, v)
		}
	}

	// 数组按比较键新→旧。
	sort.SliceStable(selected, func(i, j int) bool {
		a, okA := versionKey(q.Engine, &selected[i].Version)
		b, okB := versionKey(q.Engine, &selected[j].Version)
		if okA && okB {
			if c, comparable := compareKeys(a, b); comparable {
				return c > 0
			}
		}
		return false
	})

	entries := make([]ChangelogEntry, 0, len(selected))
	for _, v := range selected {
		title, markdown := pickLocalized(v.Version.Changelog, q.LocaleChain)
		entry := ChangelogEntry{
			ID:             v.Version.ID.String(),
			VersionInteger: v.Version.VersionInteger,
			VersionSemver:  v.Version.VersionSemverCanonical,
			Channel:        v.Version.ChannelSlug,
			Status:         v.Version.Status,
			Title:          title,
			Changelog:      markdown,
		}
		if q.IncludeNotes {
			if line := v.Line(q.OS, q.Arch); line != nil {
				entry.PlatformNotes = line.PlatformNotes
			}
		}
		entry.HadArtifactForRequestPlatform = q.OS != "" && q.Arch != "" && everReadyOnPlatform(v, q.OS, q.Arch)
		entries = append(entries, entry)
	}

	// aggregated 文本仅在 aggregated / both 布局渲染；分条恒返回，
	// 由调用方按布局决定是否下发（并供 ETag 计算，见函数 doc）。
	var aggregated string
	if q.Layout == model.ChangelogLayoutAggregated || q.Layout == model.ChangelogLayoutBoth {
		aggregated = renderAggregated(entries)
	}
	return aggregated, entries, nil
}

// renderAggregated 把分条渲染为一篇 Markdown（新→旧，条目间分隔线）。
func renderAggregated(entries []ChangelogEntry) string {
	var b strings.Builder
	for i, e := range entries {
		if i > 0 {
			b.WriteString("\n\n---\n\n")
		}
		if e.Title != "" {
			b.WriteString("## ")
			b.WriteString(e.Title)
			b.WriteString("\n\n")
		}
		b.WriteString(e.Changelog)
	}
	return b.String()
}

// pickLocalized 按语言回退链选择 changelog：链中首个命中（大小写不敏感）
// → 链尾仍无命中时取「任意已填」中字典序最小的 key（保证确定性）。
func pickLocalized(m model.ChangelogMap, chain []string) (title, markdown string) {
	if len(m) == 0 {
		return "", ""
	}
	for _, want := range chain {
		want = strings.TrimSpace(want)
		if want == "" {
			continue
		}
		for k, e := range m {
			if strings.EqualFold(k, want) {
				return e.Title, e.Markdown
			}
		}
	}
	// 任意已填：取字典序最小 key。
	smallest := ""
	for k := range m {
		if smallest == "" || k < smallest {
			smallest = k
		}
	}
	if smallest == "" {
		return "", ""
	}
	e := m[smallest]
	return e.Title, e.Markdown
}

// BuildLocaleChain 组装语言回退链：changelog_locale → locale →
// Accept-Language 首段 → default_locale（去重、跳过空值）。
func BuildLocaleChain(changelogLocale, locale, acceptLanguage, defaultLocale string) []string {
	chain := make([]string, 0, 4)
	add := func(v string) {
		v = firstLanguageTag(v)
		if v == "" {
			return
		}
		for _, existing := range chain {
			if strings.EqualFold(existing, v) {
				return
			}
		}
		chain = append(chain, v)
	}
	add(changelogLocale)
	add(locale)
	add(acceptLanguage)
	add(defaultLocale)
	return chain
}

// firstLanguageTag 从 Accept-Language 头取首个语言标签：
// "zh-CN,zh;q=0.9,en;q=0.8" → "zh-CN"。
func firstLanguageTag(header string) string {
	header = strings.TrimSpace(header)
	if header == "" {
		return ""
	}
	first := header
	if i := strings.IndexByte(first, ','); i >= 0 {
		first = first[:i]
	}
	if i := strings.IndexByte(first, ';'); i >= 0 {
		first = first[:i]
	}
	return strings.TrimSpace(first)
}
