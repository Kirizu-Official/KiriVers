package client

import (
	"encoding/json"
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

// packHandler 承载 POST /update/pack（首次入队与轮询同一 URL）。
// HTTP 层不打 zip；裁决与入队在 service/update + Jobs。
type packHandler struct {
	updates   *update.Service
	telemetry *service.TelemetryService
	limiter   *middleware.Limiter
}

type packRequest struct {
	SourceVersion string   `json:"source_version"`
	TargetVersion string   `json:"target_version"`
	OS            string   `json:"os"`
	Arch          string   `json:"arch"`
	Channel       string   `json:"channel"`
	DeviceID      string   `json:"device_id"`
	HwRev         string   `json:"hw_rev"`
	NeededPaths   []string `json:"needed_paths"`
}

func (h *packHandler) pack(c *gin.Context) {
	p := middleware.ProjectFrom(c)
	if p == nil {
		writeDiffError(c, http.StatusNotFound, "PROJECT_NOT_FOUND", "project not found", nil)
		return
	}
	raw, err := io.ReadAll(io.LimitReader(c.Request.Body, diffMaxRequestBodyBytes+1))
	if err != nil {
		writeDiffError(c, http.StatusBadRequest, "INVALID_QUERY_PARAM", "invalid request body", nil)
		return
	}
	if len(raw) > diffMaxRequestBodyBytes {
		writeDiffError(c, http.StatusBadRequest, "INVALID_QUERY_PARAM", "request body too large", nil)
		return
	}
	var req packRequest
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
	deviceHash := resolveDeviceHash(h.telemetry, p, req.DeviceID)
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
	res, err := h.updates.Pack(c.Request.Context(), p.ID, osSlug, archSlug, update.PackInput{
		SourceVersion: req.SourceVersion,
		TargetVersion: req.TargetVersion,
		OS:            osSlug,
		Arch:          archSlug,
		Channel:       strings.TrimSpace(req.Channel),
		DeviceID:      req.DeviceID,
		DeviceHash:    deviceHash,
		HwRev:         strings.TrimSpace(req.HwRev),
		NeededPaths:   req.NeededPaths,
	})
	if err != nil {
		writeDiffDomainError(c, err)
		return
	}
	c.Header("Cache-Control", "private, no-store")
	status := res.Status
	if status == 0 {
		status = http.StatusOK
	}
	response.JSON(c, status, res.Body)
}
