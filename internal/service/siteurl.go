package service

import (
	"net/url"
	"strings"

	"github.com/google/uuid"
)

// SiteURLPlaceholder 是写入数据库与管理端编辑载荷的 Markdown 站点前缀。
// 客户端 GET 按 Referer origin（否则请求 origin）替换为可抓取的绝对 URL。
const SiteURLPlaceholder = "${site_url}"

// ExpandSiteURL 将 markdown 中的 ${site_url} 替换为 origin（无尾斜杠）。
// origin 为空时原样返回。
func ExpandSiteURL(markdown, origin string) string {
	if markdown == "" || origin == "" {
		return markdown
	}
	return strings.ReplaceAll(markdown, SiteURLPlaceholder, strings.TrimRight(origin, "/"))
}

// OriginFromReferer 从 Referer 解析 http(s) origin（scheme://host[:port]，无 path/query）。
// Referer 缺失、无法解析或非 http(s) 时返回 fallback（请求自身 origin）。
func OriginFromReferer(referer, fallback string) string {
	fallback = strings.TrimRight(strings.TrimSpace(fallback), "/")
	raw := strings.TrimSpace(referer)
	if raw == "" {
		return fallback
	}
	u, err := url.Parse(raw)
	if err != nil {
		return fallback
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fallback
	}
	if u.Host == "" {
		return fallback
	}
	return u.Scheme + "://" + u.Host
}

// AppendVaryReferer 在既有 Vary 列表末尾追加 Referer（已存在则不变）。
func AppendVaryReferer(vary []string) []string {
	for _, v := range vary {
		if strings.EqualFold(strings.TrimSpace(v), "Referer") {
			return vary
		}
	}
	out := make([]string, len(vary)+1)
	copy(out, vary)
	out[len(vary)] = "Referer"
	return out
}

// MediaPublicURL 返回写入 Markdown 的占位 URL（含 ${site_url}）。
func MediaPublicURL(slug string, id uuid.UUID) string {
	return SiteURLPlaceholder + "/api/v1/projects/" + slug + "/media/" + id.String()
}
