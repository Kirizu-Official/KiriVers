package client

import (
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/Kirizu-Official/KiriVers/internal/middleware"
	"github.com/Kirizu-Official/KiriVers/internal/model"
	"github.com/Kirizu-Official/KiriVers/internal/service"
	"github.com/Kirizu-Official/KiriVers/pkg/response"
)

type clientReportHandler struct {
	projects  *service.ProjectService
	telemetry *service.TelemetryService
	limiter   *middleware.Limiter
}

type clientReportReq struct {
	DeviceID string           `json:"device_id"`
	Version  string           `json:"version"`
	OS       string           `json:"os"`
	Arch     string           `json:"arch"`
	Channel  string           `json:"channel"`
	Custom   model.JSONObject `json:"custom"`
}

func (h *clientReportHandler) report(c *gin.Context) {
	p := middleware.ProjectFrom(c)
	if p == nil {
		response.Error(c, http.StatusNotFound, "PROJECT_NOT_FOUND", "project not found", nil)
		return
	}
	var req clientReportReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, http.StatusBadRequest, "INVALID_REQUEST", "invalid json", nil)
		return
	}
	hash := resolveDeviceHash(h.telemetry, p, req.DeviceID)
	if h.limiter != nil {
		if hash != "" && !middleware.AllowKeyOrAbort(c, h.limiter,
			middleware.RateLimitDeviceKey(p.Slug, hash),
			middleware.RateLimitFor(p, model.RateLimitKeyCheckPerDevice)) {
			return
		}
		if !middleware.AllowKeyOrAbort(c, h.limiter,
			middleware.RateLimitIPKey(c.ClientIP()),
			middleware.RateLimitFor(p, model.RateLimitKeyCheckPerIP)) {
			return
		}
	}
	ip := c.ClientIP()
	cl, err := h.projects.LoginClient(c.Request.Context(), p, service.ClientLoginInput{
		DeviceID: req.DeviceID,
		Version:  req.Version,
		OS:       req.OS,
		Arch:     req.Arch,
		Channel:  req.Channel,
		IP:       ip,
		Custom:   req.Custom,
	})
	if err != nil {
		writeClientReportErr(c, err)
		return
	}
	response.JSON(c, http.StatusOK, publicReportRow(ip, cl))
}

func writeClientReportErr(c *gin.Context, err error) {
	switch {
	case errors.Is(err, service.ErrClientPolicyNone),
		errors.Is(err, service.ErrClientDeviceRequired),
		errors.Is(err, service.ErrInvalidAllowlistEntry):
		response.Error(c, http.StatusBadRequest, "INVALID_REQUEST", err.Error(), nil)
	case errors.Is(err, service.ErrProjectNotFound):
		response.Error(c, http.StatusNotFound, "PROJECT_NOT_FOUND", err.Error(), nil)
	default:
		msg := err.Error()
		if strings.Contains(msg, "custom") {
			response.Error(c, http.StatusBadRequest, "INVALID_REQUEST", msg, nil)
			return
		}
		response.Error(c, http.StatusBadRequest, "INVALID_REQUEST", msg, nil)
	}
}

func publicReportRow(ip string, cl *model.Client) gin.H {
	geo := cl.GeoI18n
	return gin.H{
		"ip":           ip,
		"country_code": cl.CountryCode,
		"region_code":  cl.RegionCode,
		"geo_i18n":     geo,
	}
}
