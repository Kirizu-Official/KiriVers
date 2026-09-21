package update

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sort"
	"strings"
	"time"

	"github.com/Kirizu-Official/KiriVers/internal/model"
)

// CatalogETag 由目录快照计算强 ETag（格式 `"<hex>"`，SHA-256 前 16 字节 hex）。
//
// 序列化覆盖整个快照的确定性投影（Publish/Revoke/yank、灰度→100%、中继配置、
// 关键标记、目标 root_hash、产物身份变更都会使输出立即变化，§10.1/§13.2），
// 但排除任何 device_id/query 输入。名单用 count+max created_at 参与哈希。
// 时间字段统一 UTC RFC3339；列表按 id/slug/(os,arch)/文件名排序，map 由
// encoding/json 按键排序输出，保证同一状态哈希恒定。
func CatalogETag(cat *Catalog) string {
	sum := sha256.Sum256(canonicalCatalogJSON(cat))
	return `"` + hex.EncodeToString(sum[:16]) + `"`
}

// MatchesETag 对 If-None-Match 头做多值匹配。RFC 7232 规定 If-None-Match
// 使用弱比较：忽略 W/ 前缀与引号后逐个比对；`*` 匹配任何当前实体。
func MatchesETag(ifNoneMatch, etag string) bool {
	if strings.TrimSpace(ifNoneMatch) == "" {
		return false
	}
	want := normalizeETag(etag)
	for _, part := range strings.Split(ifNoneMatch, ",") {
		part = strings.TrimSpace(part)
		if part == "*" {
			return true
		}
		if normalizeETag(part) == want && want != "" {
			return true
		}
	}
	return false
}

// normalizeETag 去掉 W/ 前缀与包裹引号。
func normalizeETag(etag string) string {
	etag = strings.TrimSpace(etag)
	etag = strings.TrimPrefix(etag, "W/")
	etag = strings.TrimPrefix(etag, "w/")
	etag = strings.Trim(etag, `"`)
	return etag
}

// ---------- 确定性投影 ----------

type etagDoc struct {
	Project        etagProject    `json:"project"`
	Channels       []etagChannel  `json:"channels"`
	HwRevRanks     map[string]int `json:"hw_rev_ranks"`
	Matrix         *etagMatrix    `json:"matrix,omitempty"`
	FallbackMatrix *etagMatrix    `json:"fallback_matrix,omitempty"`
	EnabledOS      []string       `json:"enabled_os"`
	Versions       []etagVersion  `json:"versions"`
}

type etagProject struct {
	ID                      string  `json:"id"`
	Slug                    string  `json:"slug"`
	CompareEngine           string  `json:"compare_engine"`
	MinimumSupportedVersion *string `json:"minimum_supported_version"`
	RequireClientToken      bool    `json:"require_client_token"`
	DefaultLocale           string  `json:"default_locale"`
	ChangelogIncludeNotes   bool    `json:"changelog_include_notes"`
	CacheSMaxageSeconds     int     `json:"cache_s_maxage_seconds"`
	SigningAlgo             string  `json:"signing_algo"`
	// SigningKeyHash 是私钥材料的 SHA-256（单向），使密钥轮换也反映到 ETag。
	SigningKeyHash string `json:"signing_key_hash"`
}

type etagChannel struct {
	Slug           string `json:"slug"`
	StabilityRank  int    `json:"stability_rank"`
	Enabled        bool   `json:"enabled"`
	Unlisted       bool   `json:"unlisted"`
	TokenProtected bool   `json:"token_protected"`
}

type etagMatrix struct {
	OS                      string  `json:"os"`
	Arch                    string  `json:"arch"`
	PackageType             string  `json:"package_type"`
	FallbackArch            string  `json:"fallback_arch"`
	DeltaAlgo               string  `json:"delta_algo"`
	HwVariantPolicy         string  `json:"hw_variant_policy"`
	MinimumSupportedVersion *string `json:"minimum_supported_version"`
}

type etagVersion struct {
	ID                   string             `json:"id"`
	Status               string             `json:"status"`
	Channel              string             `json:"channel"`
	VersionInteger       *int64             `json:"version_integer"`
	VersionSemver        *string            `json:"version_semver"`
	IsLTS            bool               `json:"is_lts"`
	IsCritical       bool               `json:"is_critical"`
	GrayCompletedAt  *string            `json:"gray_completed_at,omitempty"`
	AllowlistCount   int                `json:"allowlist_count"`
	AllowlistMaxAt   *string            `json:"allowlist_max_created_at,omitempty"`
	MinSourceVersion *string            `json:"min_source_version"`
	PublishTime      *string            `json:"publish_time"`
	Lines            []etagLine         `json:"lines"`
}

type etagLine struct {
	OS            string  `json:"os"`
	Arch          string  `json:"arch"`
	Status        string  `json:"status"`
	RootHash      string  `json:"root_hash"`
	MinOS         *string `json:"min_os"`
	MinAPILevel   *int    `json:"min_api_level"`
	PlatformNotes string  `json:"platform_notes"`
	PacksReadyAt  *string `json:"packs_ready_at,omitempty"`
	FullPkgs      []etagArtifact `json:"full_pkgs"`
	StoreFullPkgs  []etagArtifact `json:"store_full_pkgs"`
}

type etagArtifact struct {
	FileName          string           `json:"file_name"`
	Size              int64            `json:"size"`
	SHA256            string           `json:"sha256"`
	ArtifactSignature string           `json:"artifact_signature"`
	ContentType       string           `json:"content_type"`
	StorageKey        string           `json:"storage_key"`
	HwRev             *string          `json:"hw_rev"`
	MinHwRev          *string          `json:"min_hw_rev"`
	MaxHwRev          *string          `json:"max_hw_rev"`
	CompatibleHwRevs  model.StringList `json:"compatible_hw_revs"`
}

// canonicalCatalogJSON 把快照转为确定性 JSON 字节。
func canonicalCatalogJSON(cat *Catalog) []byte {
	doc := etagDoc{
		Project: etagProject{
			ID:                      cat.Project.ID.String(),
			Slug:                    cat.Project.Slug,
			CompareEngine:           cat.Project.CompareEngine,
			MinimumSupportedVersion: cat.Project.MinimumSupportedVersion,
			RequireClientToken:      cat.Project.RequireClientToken,
			DefaultLocale:           cat.Project.DefaultLocale,
			ChangelogIncludeNotes:   cat.Project.ChangelogIncludeNotes,
			CacheSMaxageSeconds:     cat.Project.CacheSMaxageSeconds,
			SigningAlgo:             cat.Project.SigningAlgo,
			SigningKeyHash:          shortHash(cat.Project.SigningPrivateKey),
		},
		Channels:   make([]etagChannel, 0, len(cat.Channels)),
		HwRevRanks: cat.HwRevRanks,
		Matrix:     projMatrix(cat.Matrix),
		EnabledOS:  append([]string(nil), cat.EnabledOS...),
		Versions:   make([]etagVersion, 0, len(cat.Versions)),
	}
	if cat.FallbackMatrix != nil {
		doc.FallbackMatrix = projMatrix(cat.FallbackMatrix)
	}
	for _, ch := range cat.Channels {
		doc.Channels = append(doc.Channels, etagChannel{
			Slug: ch.Slug, StabilityRank: ch.StabilityRank, Enabled: ch.Enabled,
			Unlisted: ch.Unlisted, TokenProtected: ch.TokenProtected,
		})
	}
	sort.Slice(doc.Channels, func(i, j int) bool { return doc.Channels[i].Slug < doc.Channels[j].Slug })
	sort.Strings(doc.EnabledOS)

	for i := range cat.Versions {
		v := &cat.Versions[i]
		ev := etagVersion{
			ID:               v.Version.ID.String(),
			Status:           v.Version.Status,
			Channel:          v.Version.ChannelSlug,
			VersionInteger:   v.Version.VersionInteger,
			VersionSemver:    v.Version.VersionSemverCanonical,
			IsLTS:            v.Version.IsLTS,
			IsCritical:       v.Version.IsCritical,
			MinSourceVersion: v.Version.MinSourceVersion,
			AllowlistCount:   len(v.Allowlist.Version),
		}
		if v.Version.PublishTime != nil {
			s := v.Version.PublishTime.UTC().Format(time.RFC3339Nano)
			ev.PublishTime = &s
		}
		if v.Version.GrayCompletedAt != nil {
			s := v.Version.GrayCompletedAt.UTC().Format(time.RFC3339Nano)
			ev.GrayCompletedAt = &s
		}
		if v.Allowlist.MaxCreatedAt != nil {
			s := v.Allowlist.MaxCreatedAt.UTC().Format(time.RFC3339Nano)
			ev.AllowlistMaxAt = &s
		}
		ev.Lines = make([]etagLine, 0, len(v.Lines))
		for _, l := range v.Lines {
			el := etagLine{
				OS: l.OS, Arch: l.Arch, Status: l.Status,
				RootHash: l.RootHash, MinOS: l.MinOS, MinAPILevel: l.MinAPILevel, PlatformNotes: l.PlatformNotes,
				FullPkgs:     make([]etagArtifact, 0, len(l.FullPkgs)),
				StoreFullPkgs: make([]etagArtifact, 0, len(l.StoreFullPkgs)),
			}
			if l.PacksReadyAt != nil {
				s := l.PacksReadyAt.UTC().Format(time.RFC3339Nano)
				el.PacksReadyAt = &s
			}
			for _, p := range l.FullPkgs {
				el.FullPkgs = append(el.FullPkgs, etagArtifact{
					FileName: p.FileName, Size: p.Size, SHA256: p.SHA256,
					ArtifactSignature: p.ArtifactSignature,
					ContentType:       p.ContentType, StorageKey: p.StorageKey,
					HwRev: p.HwRev, MinHwRev: p.MinHwRev, MaxHwRev: p.MaxHwRev,
					CompatibleHwRevs: p.CompatibleHwRevs,
				})
			}
			sort.Slice(el.FullPkgs, func(i, j int) bool { return el.FullPkgs[i].FileName < el.FullPkgs[j].FileName })
			for _, p := range l.StoreFullPkgs {
				el.StoreFullPkgs = append(el.StoreFullPkgs, etagArtifact{
					FileName: p.FileName, Size: p.Size, SHA256: p.SHA256,
					ArtifactSignature: p.ArtifactSignature,
					ContentType:       p.ContentType, StorageKey: p.StorageKey,
					HwRev: p.HwRev, MinHwRev: p.MinHwRev, MaxHwRev: p.MaxHwRev,
					CompatibleHwRevs: p.CompatibleHwRevs,
				})
			}
			sort.Slice(el.StoreFullPkgs, func(i, j int) bool { return el.StoreFullPkgs[i].FileName < el.StoreFullPkgs[j].FileName })
			ev.Lines = append(ev.Lines, el)
		}
		sort.Slice(ev.Lines, func(i, j int) bool {
			if ev.Lines[i].OS != ev.Lines[j].OS {
				return ev.Lines[i].OS < ev.Lines[j].OS
			}
			return ev.Lines[i].Arch < ev.Lines[j].Arch
		})
		doc.Versions = append(doc.Versions, ev)
	}
	sort.Slice(doc.Versions, func(i, j int) bool { return doc.Versions[i].ID < doc.Versions[j].ID })

	b, err := json.Marshal(doc)
	if err != nil {
		// 投影全部为可序列化基础类型，此处仅为兜底。
		return []byte("unserializable")
	}
	return b
}

// sortedCopy 返回升序副本；nil 保持 nil（避免序列化出空数组与缺省不一致）。
func sortedCopy(list []string) []string {
	if len(list) == 0 {
		return nil
	}
	out := append([]string(nil), list...)
	sort.Strings(out)
	return out
}

func projMatrix(m *MatrixInfo) *etagMatrix {
	if m == nil {
		return nil
	}
	return &etagMatrix{
		OS: m.OS, Arch: m.Arch, PackageType: m.PackageType,
		FallbackArch: m.FallbackArch,
		DeltaAlgo: m.DeltaAlgo, HwVariantPolicy: m.HwVariantPolicy,
		MinimumSupportedVersion: m.MinimumSupportedVersion,
	}
}

// shortHash 对字符串取 SHA-256 hex；空串返回空。
func shortHash(s string) string {
	if s == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

// ChangelogETag 由分条集合（版本 id+status+本地化内容哈希）计算强 ETag（§5.7）。
func ChangelogETag(items []changelogETagItem) string {
	b, err := json.Marshal(items)
	if err != nil {
		b = []byte("unserializable")
	}
	sum := sha256.Sum256(b)
	return `"` + hex.EncodeToString(sum[:16]) + `"`
}

// changelogETagItem 是 changelog ETag 的最小投影。
type changelogETagItem struct {
	ID            string `json:"id"`
	Status        string `json:"status"`
	Title         string `json:"title"`
	Changelog     string `json:"changelog"`
	PlatformNotes string `json:"platform_notes"`
}
