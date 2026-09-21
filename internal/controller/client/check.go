package client

import (
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/Kirizu-Official/KiriVers/internal/middleware"
	"github.com/Kirizu-Official/KiriVers/internal/model"
	"github.com/Kirizu-Official/KiriVers/internal/platform"
	"github.com/Kirizu-Official/KiriVers/internal/service"
	"github.com/Kirizu-Official/KiriVers/internal/service/update"
	"github.com/Kirizu-Official/KiriVers/pkg/response"
)

// checkHandler 承载 POST /update/check 与 GET /changelog/:channel/:os/:arch。
//
// 分层约束（task design §1/§11）：本文件只做 JSON/path 绑定、状态码、
// ETag / Cache-Control / Vary 写头；选目标、签名等全部领域规则位于
// internal/service/update。changelog 只走独立 GET。剩余 protocol_version query 忽略。
type checkHandler struct {
	updates *update.Service
	// telemetry 供 device_id 哈希（限流键 + 连续失败降级查询）；可 nil。
	telemetry *service.TelemetryService
	// limiter 是全进程共享限流器；nil = 不限流（仅测试装配省略）。
	limiter *middleware.Limiter
	// projects 供名册 upsert 与灰度懒放号；可 nil（仅纯函数测试）。
	projects *service.ProjectService
}

// resolveDeviceHash 把原始 device_id 按项目策略转为哈希：hashed = HMAC，
// raw = 明文，none / 无 telemetry / 空 device_id = 空串。
// 原始 device_id 只在内存中使用，任何日志只允许 service.Fingerprint。
func resolveDeviceHash(telemetry *service.TelemetryService, p *model.Project, rawDeviceID string) string {
	rawDeviceID = strings.TrimSpace(rawDeviceID)
	if telemetry == nil || p == nil || rawDeviceID == "" || p.DeviceIDPolicy == model.DeviceIDPolicyNone {
		return ""
	}
	return telemetry.HashDeviceID(p, rawDeviceID)
}

// limitCheckRequest 执行 check 的双维度限流（§14）：设备维度（有 device 时，
// 键 dev:{project}:{hash}，默认 60/分）+ 源 IP 维度（键 ip:{addr}，默认
// 120/分）。判定发生在任何业务逻辑（含 304 短路）之前——HTTP 304 仍计数。
func (h *checkHandler) limitCheckRequest(c *gin.Context, p *model.Project, deviceHash string) bool {
	if h.limiter == nil {
		return true
	}
	if deviceHash != "" &&
		!middleware.AllowKeyOrAbort(c, h.limiter,
			middleware.RateLimitDeviceKey(p.Slug, deviceHash),
			middleware.RateLimitFor(p, model.RateLimitKeyCheckPerDevice)) {
		return false
	}
	return middleware.AllowKeyOrAbort(c, h.limiter,
		middleware.RateLimitIPKey(c.ClientIP()),
		middleware.RateLimitFor(p, model.RateLimitKeyCheckPerIP))
}

type checkRequest struct {
	CurrentVersion     string   `json:"current_version"`
	OS                 string   `json:"os"`
	Arch               string   `json:"arch"`
	Channel            string   `json:"channel"`
	HwRev              string   `json:"hw_rev"`
	OSVersion          string   `json:"os_version"`
	DeviceID           string   `json:"device_id"`
	Capabilities       []string `json:"capabilities"`
	AcceptedDeltaAlgos []string `json:"accepted_delta_algos"`
}

// updateCheck 处理 POST /api/v1/projects/:project_ref/update/check。
func (h *checkHandler) updateCheck(c *gin.Context) {
	p := middleware.ProjectFrom(c)
	if p == nil {
		checkErrorResponse(c, http.StatusNotFound, "PROJECT_NOT_FOUND", "project not found", nil)
		return
	}

	var req checkRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		checkErrorResponse(c, http.StatusBadRequest, "INVALID_REQUEST", "invalid json", nil)
		return
	}

	currentVersion := strings.TrimSpace(req.CurrentVersion)
	if currentVersion == "" || strings.TrimSpace(req.OS) == "" || strings.TrimSpace(req.Arch) == "" {
		checkErrorResponse(c, http.StatusBadRequest, "INVALID_QUERY_PARAM",
			"current_version, os and arch are required", nil)
		return
	}

	deviceHash := resolveDeviceHash(h.telemetry, p, req.DeviceID)
	if !h.limitCheckRequest(c, p, deviceHash) {
		return
	}

	osSlug, archSlug := normalizeRequestPlatform(req.OS, req.Arch)

	if h.projects != nil {
		h.projects.TouchClientCheck(c.Request.Context(), p, req.DeviceID,
			currentVersion, osSlug, archSlug, strings.TrimSpace(req.Channel), c.ClientIP())
	}

	res, err := h.updates.Check(c.Request.Context(), p.ID, osSlug, archSlug, update.CheckInput{
		OS:                 osSlug,
		Arch:               archSlug,
		CurrentVersion:     currentVersion,
		Channel:            strings.TrimSpace(req.Channel),
		HwRev:              strings.TrimSpace(req.HwRev),
		OSVersion:          strings.TrimSpace(req.OSVersion),
		DeviceID:           req.DeviceID,
		DeviceHash:         deviceHash,
		ChannelToken:       strings.TrimSpace(c.GetHeader("X-Channel-Token")),
		Capabilities:       req.Capabilities,
		AcceptedDeltaAlgos: req.AcceptedDeltaAlgos,
	})
	if err != nil {
		writeCheckError(c, err)
		return
	}
	writeCheckResult(c, res)
}

// changelog 处理 GET /api/v1/projects/:project_ref/changelog/:channel/:os/:arch。
func (h *checkHandler) changelog(c *gin.Context) {
	p := middleware.ProjectFrom(c)
	if p == nil {
		checkErrorResponse(c, http.StatusNotFound, "PROJECT_NOT_FOUND", "project not found", nil)
		return
	}

	osSlug, archSlug := normalizeRequestPlatform(c.Param("os"), c.Param("arch"))

	res, err := h.updates.Changelog(c.Request.Context(), p.ID, osSlug, archSlug, update.ChangelogInput{
		Channel:         strings.TrimSpace(c.Param("channel")),
		ChannelToken:    strings.TrimSpace(c.GetHeader("X-Channel-Token")),
		OS:              osSlug,
		Arch:            archSlug,
		FromVersion:     strings.TrimSpace(c.Query("from_version")),
		ToVersion:       strings.TrimSpace(c.Query("to_version")),
		Scope:           strings.TrimSpace(c.Query("changelog_scope")),
		Layout:          strings.TrimSpace(c.Query("changelog_layout")),
		IncludeRevoked:  parseOptionalBool(c.Query("changelog_include_revoked")),
		IncludeNotes:    parseOptionalBool(c.Query("changelog_include_platform_notes")),
		ChangelogLocale: c.Query("changelog_locale"),
		Locale:          c.Query("locale"),
	})
	if err != nil {
		writeCheckError(c, err)
		return
	}

	c.Header("Vary", strings.Join(res.Vary, ", "))
	if update.MatchesETag(c.GetHeader("If-None-Match"), res.ETag) {
		writeCachedHeaders(c, res.ETag, res.CacheControl)
		c.Status(http.StatusNotModified)
		return
	}
	writeCachedHeaders(c, res.ETag, res.CacheControl)
	expandChangelogBody(c, &res.Body)
	response.JSON(c, http.StatusOK, res.Body)
}

// writeCheckResult 写出 check 的 200/204/304 与 ETag / Cache-Control / Vary。
func writeCheckResult(c *gin.Context, res *update.CheckResult) {
	c.Header("Vary", strings.Join(res.Vary, ", "))
	// 私有项目（§13.7 / C15-1）：跳过 If-None-Match 304 短路——响应中的签名
	// URL 随 TTL 过期，必须每次回新鲜 200 携带新签名；仍发 ETag 供参考。
	if !res.Private && update.MatchesETag(c.GetHeader("If-None-Match"), res.ETag) {
		writeCachedHeaders(c, res.ETag, res.CacheControl)
		c.Status(http.StatusNotModified)
		return
	}
	writeCachedHeaders(c, res.ETag, res.CacheControl)
	if res.Status == http.StatusNoContent {
		c.Status(http.StatusNoContent)
		return
	}
	response.JSON(c, res.Status, res.Body)
}

// writeCachedHeaders 统一写 ETag 与 Cache-Control。
func writeCachedHeaders(c *gin.Context, etag, cacheControl string) {
	if etag != "" {
		c.Header("ETag", etag)
	}
	if cacheControl != "" {
		c.Header("Cache-Control", cacheControl)
	}
}

// writeCheckError 把 service/update 的领域错误映射为统一错误包。
// POST check 永不发出 PRECONDITION_FAILED（C08-16，该码属于指定 target 的
// integrity/diff，由调用方触发）；integrity/diff 复用本映射（新增领域错误
// 在 check 路径不会出现，映射共享保证出口一致）。
func writeCheckError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, update.ErrInvalidQuery):
		checkErrorResponse(c, http.StatusBadRequest, "INVALID_QUERY_PARAM", err.Error(), nil)
	case errors.Is(err, update.ErrEngineMismatch):
		checkErrorResponse(c, http.StatusBadRequest, "ENGINE_MISMATCH", err.Error(), nil)
	case errors.Is(err, update.ErrChannelConflict):
		checkErrorResponse(c, http.StatusBadRequest, "CHANNEL_CONFLICT", err.Error(), nil)
	case errors.Is(err, update.ErrVersionNotVisible):
		checkErrorResponse(c, http.StatusNotFound, "VERSION_NOT_VISIBLE", "version not visible", nil)
	case errors.Is(err, update.ErrVersionNotFound):
		checkErrorResponse(c, http.StatusNotFound, "VERSION_NOT_FOUND", "version not found", nil)
	case errors.Is(err, update.ErrVersionLineNotFound):
		checkErrorResponse(c, http.StatusNotFound, "VERSION_LINE_NOT_FOUND", "version line not found for the requested platform", nil)
	case errors.Is(err, update.ErrVersionRevoked):
		// 吊销版本：409，且错误体禁止出现任何下载 URL（C09-2）。
		checkErrorResponse(c, http.StatusConflict, "VERSION_REVOKED", "version has been revoked", nil)
	case errors.Is(err, update.ErrNoSafeTarget):
		checkErrorResponse(c, http.StatusConflict, "NO_SAFE_TARGET", "no safe update target", nil)
	case errors.Is(err, update.ErrIntermediateUnavailable):
		checkErrorResponse(c, http.StatusConflict, "INTERMEDIATE_UNAVAILABLE", err.Error(), nil)
	case errors.Is(err, update.ErrMinOSNotMet):
		checkErrorResponse(c, http.StatusConflict, "MIN_OS_NOT_MET", err.Error(), nil)
	case errors.Is(err, update.ErrInvalidChangelogQuery):
		checkErrorResponse(c, http.StatusBadRequest, "CHANGELOG_QUERY_INVALID", err.Error(), nil)
	case errors.Is(err, update.ErrNotFound):
		checkErrorResponse(c, http.StatusNotFound, "NOT_FOUND", "not found", nil)
	default:
		checkErrorResponse(c, http.StatusInternalServerError, "INTERNAL_ERROR", "internal server error", nil)
	}
}

// checkErrorResponse 错误出口：409 依 §10.1 禁止 public 缓存；其余错误
// 同样 private, no-store，避免错误响应被 CDN 长缓存。
func checkErrorResponse(c *gin.Context, status int, code, message string, details any) {
	c.Header("Cache-Control", "private, no-store")
	c.Header("Vary", "Accept-Encoding")
	response.Error(c, status, code, message, details)
}

// normalizeRequestPlatform 规范化请求 os/arch（§4.1–4.2 别名表）。
// 说明：CanonicalOS 的「项目是否登记 ipados」需要矩阵启用集；控制器在
// 加载目录前无法获得，统一按默认检索口径（ipados→ios），独立 ipados
// 矩阵行的项目属边缘场景，由 loader 按登记名直查兜底。
func normalizeRequestPlatform(rawOS, rawArch string) (string, string) {
	return platform.CanonicalOS(nil, rawOS), platform.CanonicalArch(rawArch)
}

// parseOptionalBool 解析可选布尔 query；未传返回 nil。
func parseOptionalBool(raw string) *bool {
	raw = strings.TrimSpace(strings.ToLower(raw))
	if raw == "" {
		return nil
	}
	v := raw == "true" || raw == "1" || raw == "yes"
	return &v
}
