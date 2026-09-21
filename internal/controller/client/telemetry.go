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
	"github.com/Kirizu-Official/KiriVers/pkg/response"
)

// telemetryHandler 承载 POST /telemetry/report（§10.4 / C11-3）。
//
// 隐私约束（C11-2）：请求体中的原始 device_id 只经 telemetry 服务做内存内
// HMAC/落库策略处理，本文件任何代码路径都不得将其写入日志——需要关联时只
// 允许 service.Fingerprint（哈希前 8 字符）。log buffer 测试锁定该约束。
type telemetryHandler struct {
	projects *service.ProjectService
	// telemetry 为 nil（无数据库装配）时上报返回 503：遥测不阻塞更新协议，
	// 但端点本身无法落库。
	telemetry *service.TelemetryService
	// limiter 是全进程共享限流器；nil = 不限流（仅测试装配省略）。
	limiter *middleware.Limiter
}

// telemetryMaxRequestBodyBytes 请求体读取上限（8 KiB）：上报字段均为短字符串，
// 足够宽裕且防滥用。
const telemetryMaxRequestBodyBytes = 8 << 10

// telemetryRequest 是 POST /telemetry/report 的 JSON 请求体（§10.4）。
type telemetryRequest struct {
	DeviceID     string `json:"device_id"`
	OS           string `json:"os"`
	Arch         string `json:"arch"`
	Channel      string `json:"channel"`
	FromVersion  string `json:"from_version"`
	ToVersion    string `json:"to_version"`
	Status       string `json:"status"`
	ErrorCode    string `json:"error_code"`
	ErrorMessage string `json:"error_message"`
	DiffMode     string `json:"diff_mode"`
}

// report 处理 POST /api/v1/projects/:project_ref/telemetry/report。
//
// 语义（C11-3）：只收不挡——成功恒 202；仅参数非法（400）/项目不存在（404）/
// 限流（429）之外不产生其他客户端可见分支。device_id 缺失仍接受（空哈希）。
func (h *telemetryHandler) report(c *gin.Context) {
	p := middleware.ProjectFrom(c)
	if p == nil {
		checkErrorResponse(c, http.StatusNotFound, "PROJECT_NOT_FOUND", "project not found", nil)
		return
	}
	if h.telemetry == nil {
		checkErrorResponse(c, http.StatusServiceUnavailable, "NOT_READY", "database unavailable", nil)
		return
	}

	raw, err := io.ReadAll(io.LimitReader(c.Request.Body, telemetryMaxRequestBodyBytes+1))
	if err != nil || len(raw) > telemetryMaxRequestBodyBytes {
		checkErrorResponse(c, http.StatusBadRequest, "INVALID_QUERY_PARAM", "invalid request body", nil)
		return
	}
	var req telemetryRequest
	if err := json.Unmarshal(raw, &req); err != nil {
		checkErrorResponse(c, http.StatusBadRequest, "INVALID_QUERY_PARAM", "invalid JSON body", nil)
		return
	}

	// 必填字段与 status 枚举校验（§10.4）；device_id 可选。
	in := service.ReportInput{
		DeviceID:     strings.TrimSpace(req.DeviceID),
		OS:           strings.TrimSpace(req.OS),
		Arch:         strings.TrimSpace(req.Arch),
		Channel:      strings.TrimSpace(req.Channel),
		FromVersion:  strings.TrimSpace(req.FromVersion),
		ToVersion:    strings.TrimSpace(req.ToVersion),
		Status:       strings.TrimSpace(req.Status),
		ErrorCode:    strings.TrimSpace(req.ErrorCode),
		ErrorMessage: strings.TrimSpace(req.ErrorMessage),
		DiffMode:     strings.TrimSpace(req.DiffMode),
	}

	// 限流（§14）：设备维度 30/分（键 dev:{project}:{hash}）；无 device（含
	// none 策略）回退 IP 维度。判定在落库之前。
	deviceHash := resolveDeviceHash(h.telemetry, p, in.DeviceID)
	if h.limiter != nil {
		perMinute := middleware.RateLimitFor(p, model.RateLimitKeyTelemetryPerDevice)
		key := middleware.RateLimitIPKey(c.ClientIP())
		if deviceHash != "" {
			key = middleware.RateLimitDeviceKey(p.Slug, deviceHash)
		}
		if !middleware.AllowKeyOrAbort(c, h.limiter, key, perMinute) {
			return
		}
	}

	if err := h.telemetry.Report(c.Request.Context(), p, in); err != nil {
		if errors.Is(err, service.ErrInvalidTelemetryReport) {
			checkErrorResponse(c, http.StatusBadRequest, "INVALID_QUERY_PARAM", err.Error(), nil)
			return
		}
		checkErrorResponse(c, http.StatusInternalServerError, "INTERNAL_ERROR", "internal server error", nil)
		return
	}
	response.JSON(c, http.StatusAccepted, gin.H{"status": "accepted"})
}
