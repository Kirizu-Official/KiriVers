package client

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/Kirizu-Official/KiriVers/internal/middleware"
	"github.com/Kirizu-Official/KiriVers/internal/model"
	"github.com/Kirizu-Official/KiriVers/internal/service"
	"github.com/Kirizu-Official/KiriVers/internal/service/update"
	"github.com/Kirizu-Official/KiriVers/pkg/response"
)

// diffHandler 承载 POST /update/diff（C09-3..C09-8, C09-11）。
//
// 分层与安全约束：本文件只做 JSON 绑定与状态码/缓存头写出；
// 裁决顺序与差量匹配全部位于 internal/service/update。本层禁止出现任何
// zip/打包逻辑；多文件原生路径是 POST /update/pack，diff 只做单文件 binary_delta 查找。
type diffHandler struct {
	updates *update.Service
	// telemetry 供 device_id 哈希（限流键 + 连续失败降级查询）；可 nil。
	telemetry *service.TelemetryService
	// limiter 是全进程共享限流器；nil = 不限流（仅测试装配省略）。
	limiter *middleware.Limiter
}

// diffMaxRequestBodyBytes 请求体读取上限（1 MiB，DoS 防护兜底）。
const diffMaxRequestBodyBytes = 1 << 20

// diffRequest 是 POST /update/diff 的 JSON 请求体（§10.3）。
type diffRequest struct {
	SourceVersion      string   `json:"source_version"`
	TargetVersion      string   `json:"target_version"`
	OS                 string   `json:"os"`
	Arch               string   `json:"arch"`
	Channel            string   `json:"channel"`
	DeviceID           string   `json:"device_id"`
	HwRev              string   `json:"hw_rev"`
	LocalSHA256        string   `json:"local_sha256"`
	AcceptedDeltaAlgos []string `json:"accepted_delta_algos"`
	PreferFull         bool     `json:"prefer_full"`
	Capabilities       []string `json:"capabilities"`
}

// diff 处理 POST /api/v1/projects/:project_ref/update/diff。
// 恒 Cache-Control: private, no-store，无 ETag（§10.3 / §13.2）。
func (h *diffHandler) diff(c *gin.Context) {
	p := middleware.ProjectFrom(c)
	if p == nil {
		writeDiffError(c, http.StatusNotFound, "PROJECT_NOT_FOUND", "project not found", nil)
		return
	}

	// 读取原始体再反序列化。
	raw, err := io.ReadAll(io.LimitReader(c.Request.Body, diffMaxRequestBodyBytes+1))
	if err != nil {
		writeDiffError(c, http.StatusBadRequest, "INVALID_QUERY_PARAM", "invalid request body", nil)
		return
	}
	if len(raw) > diffMaxRequestBodyBytes {
		writeDiffError(c, http.StatusBadRequest, "INVALID_QUERY_PARAM",
			"request body too large", nil)
		return
	}
	var req diffRequest
	if err := json.Unmarshal(raw, &req); err != nil {
		writeDiffError(c, http.StatusBadRequest, "INVALID_QUERY_PARAM", "invalid JSON body", nil)
		return
	}
	if strings.TrimSpace(req.SourceVersion) == "" || strings.TrimSpace(req.TargetVersion) == "" ||
		strings.TrimSpace(req.OS) == "" || strings.TrimSpace(req.Arch) == "" {
		writeDiffError(c, http.StatusBadRequest, "INVALID_QUERY_PARAM",
			"source_version, target_version, os and arch are required", nil)
		return
	}

	osSlug, archSlug := normalizeRequestPlatform(req.OS, req.Arch)

	// device_id 按项目策略哈希（限流键 + 连续失败降级查询）；原始值不进日志（C11-2）。
	deviceHash := resolveDeviceHash(h.telemetry, p, req.DeviceID)
	// 双维度限流（§14）：设备 20/分（有 device 时）+ 源 IP 60/分。
	// 判定在任何业务裁决之前，304 语义不适用于 POST，但仍先于差量计算。
	if h.limiter != nil {
		if deviceHash != "" &&
			!middleware.AllowKeyOrAbort(c, h.limiter,
				middleware.RateLimitDeviceKey(p.Slug, deviceHash),
				middleware.RateLimitFor(p, model.RateLimitKeyDiffPerDevice)) {
			return
		}
		if !middleware.AllowKeyOrAbort(c, h.limiter,
			middleware.RateLimitIPKey(c.ClientIP()),
			middleware.RateLimitFor(p, model.RateLimitKeyDiffPerIP)) {
			return
		}
	}

	res, err := h.updates.Diff(c.Request.Context(), p.ID, osSlug, archSlug, update.DiffInput{
		SourceVersion:      req.SourceVersion,
		TargetVersion:      req.TargetVersion,
		OS:                 osSlug,
		Arch:               archSlug,
		Channel:            strings.TrimSpace(req.Channel),
		DeviceID:           req.DeviceID,
		DeviceHash:         deviceHash,
		HwRev:              strings.TrimSpace(req.HwRev),
		LocalSHA256:        strings.TrimSpace(req.LocalSHA256),
		AcceptedDeltaAlgos: req.AcceptedDeltaAlgos,
		PreferFull:         req.PreferFull,
		Capabilities:       req.Capabilities,
	})
	if err != nil {
		writeDiffDomainError(c, err)
		return
	}

	// 成功响应：private, no-store，无 ETag。
	c.Header("Cache-Control", "private, no-store")
	response.JSON(c, http.StatusOK, res.Body)
}

// writeDiffDomainError 把 diff 领域错误映射为统一错误包。
func writeDiffDomainError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, update.ErrPreconditionFailed):
		// C09-11：非强制且灰度未命中 → 412，不下发任何包。
		writeDiffError(c, http.StatusPreconditionFailed, "PRECONDITION_FAILED",
			"gray rollout miss for the specified target", nil)
	case errors.Is(err, update.ErrInvalidQuery):
		writeDiffError(c, http.StatusBadRequest, "INVALID_QUERY_PARAM", err.Error(), nil)
	case errors.Is(err, update.ErrChannelConflict):
		writeDiffError(c, http.StatusBadRequest, "CHANNEL_CONFLICT", err.Error(), nil)
	case errors.Is(err, update.ErrVersionNotVisible):
		writeDiffError(c, http.StatusNotFound, "VERSION_NOT_VISIBLE", "version not visible", nil)
	case errors.Is(err, update.ErrVersionNotFound):
		writeDiffError(c, http.StatusNotFound, "VERSION_NOT_FOUND", "version not found", nil)
	case errors.Is(err, update.ErrVersionLineNotFound):
		writeDiffError(c, http.StatusNotFound, "VERSION_LINE_NOT_FOUND",
			"version line not found for the requested platform", nil)
	case errors.Is(err, update.ErrVersionRevoked):
		writeDiffError(c, http.StatusConflict, "VERSION_REVOKED", "version has been revoked", nil)
	default:
		writeDiffError(c, http.StatusInternalServerError, "INTERNAL_ERROR", "internal server error", nil)
	}
}

// writeDiffError 错误出口：恒 private, no-store（POST diff 无任何可缓存形态）。
func writeDiffError(c *gin.Context, status int, code, message string, details any) {
	c.Header("Cache-Control", "private, no-store")
	response.Error(c, status, code, message, details)
}
