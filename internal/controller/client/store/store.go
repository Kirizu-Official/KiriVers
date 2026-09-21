// Package store 是商店协议 feed（docs/app-init.md §9 / §9.2）的 HTTP 接入层。
//
// 路由：GET /api/v1/projects/:project_ref/store/{protocol}/{listing_slug}/{doc}。
// 旧路径 /store/{protocol}/{doc}（无 listing slug）走两段路由，listing 解析失败 → 纯文本 404。
//
// 分层约束：本包只做协议分发、鉴权装配（StoreAuth 中间件）、状态码与
// ETag / Cache-Control / Vary 写头；版本可见性、协议 schema 投影全部位于
// internal/service/store（Adapter 永不接触 HTTP 头）。
//
// 统一语义（所有协议共享，§9.2）：
//   - 启用 listing 解析失败（不存在 / 停用 / 旧路径无 slug）或协议未适配 → 纯文本 404，绝不回退为
//     「通用全量 zip」（C16-4）；
//   - require_client_token / store_token → StoreAuth 401 UNAUTHORIZED（C16-7，
//     已由 internal/middleware.StoreAuth 实现）；
//   - ETag = 渲染 body 的 SHA-256 前 16 字节；If-None-Match 匹配 → 304；
//   - Cache-Control: public, s-maxage=<项目配置>, stale-while-revalidate=30，
//     本机代拉或私有存储项目恒 private, no-store（按节点可见 / 签名 URL 不进共享 CDN）；
//   - Vary: Accept-Encoding。
package store

import (
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/Kirizu-Official/KiriVers/internal/middleware"
	"github.com/Kirizu-Official/KiriVers/internal/model"
	"github.com/Kirizu-Official/KiriVers/internal/platform"
	"github.com/Kirizu-Official/KiriVers/internal/service"
	storesvc "github.com/Kirizu-Official/KiriVers/internal/service/store"
	"github.com/Kirizu-Official/KiriVers/internal/service/update"
	"github.com/Kirizu-Official/KiriVers/pkg/response"
	"github.com/Kirizu-Official/KiriVers/pkg/urlsign"
)

// storeHandler 承载 /store/{protocol}/{path...} 的协议分发。
type storeHandler struct {
	projects *service.ProjectService
	updates  *update.Service
	// signer 是私有存储短时签名器（§13.7 / C15-1，可 nil）：私有项目
	// enclosure URL 附加 ?exp=&sig=，公开项目直链不变（C15-2）。
	signer *urlsign.Signer
	// registry 是协议 Adapter 注册表（DefaultRegistry：当前含 sparkle）。
	registry *storesvc.Registry
}

// Register 在 StoreAuth 保护的分组上挂载 feed 路由。调用方保证 updates 非 nil
// （feed 依赖目录加载）；limiter 可 nil（不限流，测试装配省略）。
// registry 可 nil（使用 DefaultRegistry）。
func Register(rg *gin.RouterGroup, projects *service.ProjectService, updates *update.Service, signer *urlsign.Signer, limiter *middleware.Limiter, registry *storesvc.Registry) {
	if projects == nil || updates == nil {
		return
	}
	if registry == nil {
		registry = storesvc.DefaultRegistry()
	}
	h := &storeHandler{projects: projects, updates: updates, signer: signer, registry: registry}
	limit := middleware.IPRateLimit(limiter, model.RateLimitKeyStorePerIP)
	// 两段路径（无 *doc）承接旧 URL /store/{protocol}/{doc}：listing_slug 会落到文档名，解析失败 → 纯文本 404（AC15）。
	rg.GET("/store/:protocol/:listing_slug", limit, h.serveStore)
	rg.POST("/store/:protocol/:listing_slug", limit, h.serveStore)
	rg.GET("/store/:protocol/:listing_slug/*doc", limit, h.serveStore)
	rg.POST("/store/:protocol/:listing_slug/*doc", limit, h.serveStore)
}

// serveStore 统一入口：解析协议段 → 开关闸 → 组装 Request → Adapter 渲染 →
// 缓存头与 body 写出。
func (h *storeHandler) serveStore(c *gin.Context) {
	p := middleware.ProjectFrom(c)
	if p == nil {
		// StoreAuth 已解析项目；此处仅为防御。
		response.Error(c, http.StatusNotFound, "PROJECT_NOT_FOUND", "project not found", nil)
		return
	}

	// 协议名与 listing slug 来自路径；旧路径 /store/{protocol}/{doc} 会把文档名当成 slug → 404。
	protocol := strings.ToLower(strings.TrimSpace(c.Param("protocol")))
	listingSlug := strings.TrimSpace(c.Param("listing_slug"))
	docPath := strings.TrimPrefix(c.Param("doc"), "/")
	adapter, ok := h.registry.Get(protocol)
	if !ok || listingSlug == "" {
		writePlainNotFound(c)
		return
	}
	listing, err := h.projects.GetEnabledStoreListing(c.Request.Context(), p.ID, protocol, listingSlug)
	if err != nil || listing == nil {
		writePlainNotFound(c)
		return
	}

	var rawBody []byte
	if c.Request.Body != nil {
		rawBody, _ = io.ReadAll(c.Request.Body)
	}

	queryParams := make(map[string]string)
	for k, v := range c.Request.URL.Query() {
		if len(v) > 0 {
			queryParams[k] = v[0]
		}
	}

	curVer := strings.TrimSpace(c.Query("current_version"))
	if curVer == "" {
		if v := strings.TrimSpace(c.Query("Version")); v != "" {
			curVer = v
		} else if v := strings.TrimSpace(c.Query("version")); v != "" {
			curVer = v
		}
	}

	osName := platform.CanonicalOS(nil, c.Query("os"))
	arch := platform.CanonicalArch(c.Query("arch"))
	channel := requestChannel(c)
	if listing.OS != nil && strings.TrimSpace(*listing.OS) != "" {
		osName = platform.CanonicalOS(nil, *listing.OS)
	}
	if listing.Arch != nil && strings.TrimSpace(*listing.Arch) != "" {
		arch = platform.CanonicalArch(*listing.Arch)
	}
	if listing.Channel != nil && strings.TrimSpace(*listing.Channel) != "" {
		channel = strings.TrimSpace(*listing.Channel)
	}

	req := storesvc.Request{
		Project:         p,
		Channel:         channel,
		OS:              osName,
		Arch:            arch,
		HWRev:           strings.TrimSpace(c.Query("hw_rev")),
		Path:            docPath,
		CurrentVersion:  curVer,
		ChangelogLocale: c.Query("changelog_locale"),
		Locale:          c.Query("locale"),
		AcceptLanguage:  c.GetHeader("Accept-Language"),
		Method:          c.Request.Method,
		RawBody:         rawBody,
		QueryParams:     queryParams,
		Listing:         listing,
	}

	deps := &storesvc.Deps{
		Updates: h.updates, Signer: h.signer, Storage: h.projects.Storage(),
		ObjectURL: h.updates.EnclosureObjectURL(), LocalProxy: h.updates.LocalProxy(),
	}
	resp, err := adapter.Render(c.Request.Context(), deps, req)
	if err != nil {
		// 文档路径未知 / 可见集为空 → 纯文本 404（§9.2：与协议未开启同一
		// 语义，绝不冒充协议产物，也不回退通用 zip）。
		if errors.Is(err, storesvc.ErrUnknownPath) || errors.Is(err, storesvc.ErrNoRelease) {
			writePlainNotFound(c)
			return
		}
		writeStoreError(c, err)
		return
	}

	// 私有存储项目跳过 304 短路：body 内签名 URL 随 TTL 过期，恒回新鲜 200
	//（与原生 check 的 Private 语义一致，C15-1）。
	status := resp.Status
	if status == 0 {
		status = http.StatusOK
	}
	localProxy := h.updates != nil && h.updates.LocalProxy()
	cacheControl := storesvc.CacheControlFor(p, localProxy)
	// 204 No Content（如 Tauri dynamic check 无更新）：无 body、无 ETag、
	// 不写 304，但保留缓存控制头。
	if status == http.StatusNoContent || len(resp.Body) == 0 {
		c.Header("Cache-Control", cacheControl)
		c.Status(status)
		return
	}

	// ETag / 缓存头由框架层统一计算（Adapter 不碰 HTTP 头，§9.2）。
	etag := storesvc.BodyETag(resp.Body)
	c.Header("ETag", etag)
	c.Header("Cache-Control", cacheControl)
	c.Header("Vary", "Accept-Encoding")

	if p.StorageVisibility != model.StorageVisibilityPrivate &&
		update.MatchesETag(c.GetHeader("If-None-Match"), etag) {
		c.Status(http.StatusNotModified)
		return
	}

	c.Data(status, resp.ContentType, resp.Body)
}

// requestChannel 解析渠道 query：缺省 stable（§4.3 分发权威）；未知渠道由
// Adapter 校验（目录加载后才知道项目渠道表）。
func requestChannel(c *gin.Context) string {
	ch := strings.TrimSpace(c.Query("channel"))
	if ch == "" {
		ch = strings.TrimSpace(c.Query("Channel"))
	}
	if ch == "" {
		return "stable"
	}
	return ch
}

// writeStoreError 把 feed 领域错误映射为统一错误包；目录 / 存储等基础设施
// 错误一律 500（错误体不携带内部细节）。
func writeStoreError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, storesvc.ErrMissingParam) || errors.Is(err, storesvc.ErrUnknownChannel):
		response.Error(c, http.StatusBadRequest, "INVALID_QUERY_PARAM", err.Error(), nil)
	default:
		response.Error(c, http.StatusInternalServerError, "INTERNAL_ERROR", "internal server error", nil)
	}
}

// writePlainNotFound 写出纯文本 404：feed 未适配 / 未开启时不得返回任何
// 协议化 body（更不能是 zip，C16-4），也不建议 JSON 错误包误导协议客户端。
func writePlainNotFound(c *gin.Context) {
	c.Header("Cache-Control", "private, no-store")
	c.String(http.StatusNotFound, "404 page not found")
}
