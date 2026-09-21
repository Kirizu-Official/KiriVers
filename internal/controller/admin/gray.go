package admin

import (
	"errors"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/Kirizu-Official/KiriVers/internal/model"
	"github.com/Kirizu-Official/KiriVers/internal/service"
	"github.com/Kirizu-Official/KiriVers/pkg/response"
)

type allowlistWriteReq struct {
	ClientIDs []uuid.UUID `json:"client_ids"`
}

type grayPatchReq struct {
	GrayStartPercent    *int `json:"gray_start_percent"`
	GrayStepPercent     *int `json:"gray_step_percent"`
	GrayIntervalSeconds *int `json:"gray_interval_seconds"`
}

func publicAllowlistEntry(e *model.GrayAllowlist) gin.H {
	return gin.H{
		"id":         e.ID,
		"version_id": e.VersionID,
		"device_id":  e.DeviceID,
		"source":     e.Source,
		"created_at": formatTimeUTC(e.CreatedAt),
	}
}

func allowlistEntries(list []model.GrayAllowlist) []gin.H {
	items := make([]gin.H, 0, len(list))
	for i := range list {
		items = append(items, publicAllowlistEntry(&list[i]))
	}
	return items
}

func writeGrayErr(c *gin.Context, err error) {
	switch {
	case errors.Is(err, service.ErrAllowlistPolicyNone),
		errors.Is(err, service.ErrInvalidAllowlistEntry),
		errors.Is(err, service.ErrInvalidRolloutPercent),
		errors.Is(err, service.ErrClientPolicyNone),
		errors.Is(err, service.ErrClientDeviceRequired):
		response.Error(c, http.StatusBadRequest, "INVALID_REQUEST", err.Error(), nil)
	case errors.Is(err, service.ErrClientNotFound):
		response.Error(c, http.StatusNotFound, "NOT_FOUND", err.Error(), nil)
	default:
		writeVersionErr(c, err)
	}
}

func publicGrayStatus(st *service.GrayStatus) gin.H {
	h := publicVersion(st.Version, nil)
	h["n"] = st.N
	h["desired"] = st.Desired
	h["allowlisted"] = st.Allowlisted
	h["updated"] = st.Updated
	h["target_percent"] = st.TargetPercent
	h["actual_percent"] = st.ActualPercent
	h["completed"] = st.Completed
	return h
}

func (h *projectHandler) getVersionGray(c *gin.Context) {
	p, err := h.projects.Resolve(c.Request.Context(), c.Param("project_ref"))
	if err != nil {
		writeProjectErr(c, err)
		return
	}
	st, err := h.projects.GetGrayStatus(c.Request.Context(), p, c.Param("version"))
	if err != nil {
		writeGrayErr(c, err)
		return
	}
	response.JSON(c, http.StatusOK, publicGrayStatus(st))
}

func (h *projectHandler) patchVersionGray(c *gin.Context) {
	p, err := h.projects.Resolve(c.Request.Context(), c.Param("project_ref"))
	if err != nil {
		writeProjectErr(c, err)
		return
	}
	var req grayPatchReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, http.StatusBadRequest, "INVALID_REQUEST", "invalid json", nil)
		return
	}
	v, err := h.projects.PatchGrayKnobs(c.Request.Context(), p.ID, c.Param("version"),
		req.GrayStartPercent, req.GrayStepPercent, req.GrayIntervalSeconds)
	if err != nil {
		writeGrayErr(c, err)
		return
	}
	st, err := h.projects.GetGrayStatus(c.Request.Context(), p, v.ID.String())
	if err != nil {
		writeGrayErr(c, err)
		return
	}
	response.JSON(c, http.StatusOK, publicGrayStatus(st))
	h.audit.recordAudit(c, &p.ID, model.AuditActionGrayAllowlistAdd, "gray", c.Param("version"),
		gin.H{"action": "patch_knobs"})
}

func (h *projectHandler) completeVersionGray(c *gin.Context) {
	p, err := h.projects.Resolve(c.Request.Context(), c.Param("project_ref"))
	if err != nil {
		writeProjectErr(c, err)
		return
	}
	v, err := h.projects.CompleteGray(c.Request.Context(), p.ID, c.Param("version"))
	if err != nil {
		writeGrayErr(c, err)
		return
	}
	st, err := h.projects.GetGrayStatus(c.Request.Context(), p, v.ID.String())
	if err != nil {
		writeGrayErr(c, err)
		return
	}
	response.JSON(c, http.StatusOK, publicGrayStatus(st))
	h.audit.recordAudit(c, &p.ID, model.AuditActionGrayAllowlistAdd, "gray", c.Param("version"),
		gin.H{"action": "complete"})
}

func (h *projectHandler) listVersionGrayClients(c *gin.Context) {
	p, err := h.projects.Resolve(c.Request.Context(), c.Param("project_ref"))
	if err != nil {
		writeProjectErr(c, err)
		return
	}
	rows, err := h.projects.ListGrayClients(c.Request.Context(), p, c.Param("version"))
	if err != nil {
		writeGrayErr(c, err)
		return
	}
	items := make([]gin.H, 0, len(rows))
	for i := range rows {
		item := publicClient(&rows[i].Client)
		item["source"] = rows[i].Source
		item["updated"] = rows[i].Updated
		items = append(items, item)
	}
	response.JSON(c, http.StatusOK, gin.H{"clients": items})
}

func (h *projectHandler) listVersionGraySeries(c *gin.Context) {
	p, err := h.projects.Resolve(c.Request.Context(), c.Param("project_ref"))
	if err != nil {
		writeProjectErr(c, err)
		return
	}
	from, to := parseTimeRange(c)
	snaps, err := h.projects.ListGraySnapshots(c.Request.Context(), p.ID, c.Param("version"), from, to)
	if err != nil {
		writeGrayErr(c, err)
		return
	}
	items := make([]gin.H, 0, len(snaps))
	for i := range snaps {
		s := &snaps[i]
		items = append(items, gin.H{
			"taken_at":       formatTimeUTC(s.TakenAt),
			"n":              s.N,
			"desired":        s.Desired,
			"allowlisted":    s.Allowlisted,
			"updated":        s.Updated,
			"target_percent": s.TargetPercent,
		})
	}
	response.JSON(c, http.StatusOK, gin.H{"series": items})
}

func (h *projectHandler) listVersionGrayAllowlist(c *gin.Context) {
	p, err := h.projects.Resolve(c.Request.Context(), c.Param("project_ref"))
	if err != nil {
		writeProjectErr(c, err)
		return
	}
	list, err := h.projects.ListVersionGrayAllowlist(c.Request.Context(), p.ID, c.Param("version"))
	if err != nil {
		writeGrayErr(c, err)
		return
	}
	response.JSON(c, http.StatusOK, gin.H{"entries": allowlistEntries(list)})
}

func (h *projectHandler) addVersionGrayAllowlist(c *gin.Context) {
	p, err := h.projects.Resolve(c.Request.Context(), c.Param("project_ref"))
	if err != nil {
		writeProjectErr(c, err)
		return
	}
	var req allowlistWriteReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, http.StatusBadRequest, "INVALID_REQUEST", "invalid json", nil)
		return
	}
	list, err := h.projects.AddGrayAllowlistByClientIDs(c.Request.Context(), p.ID, c.Param("version"), req.ClientIDs)
	if err != nil {
		writeGrayErr(c, err)
		return
	}
	response.JSON(c, http.StatusOK, gin.H{"entries": allowlistEntries(list)})
	h.audit.recordAudit(c, &p.ID, model.AuditActionGrayAllowlistAdd, "gray_allowlist", c.Param("version"),
		gin.H{"scope": "version", "added": len(list)})
}

func (h *projectHandler) deleteVersionGrayAllowlist(c *gin.Context) {
	p, err := h.projects.Resolve(c.Request.Context(), c.Param("project_ref"))
	if err != nil {
		writeProjectErr(c, err)
		return
	}
	var req allowlistWriteReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, http.StatusBadRequest, "INVALID_REQUEST", "invalid json", nil)
		return
	}
	removed, list, err := h.projects.DeleteGrayAllowlistByClientIDs(c.Request.Context(), p.ID, c.Param("version"), req.ClientIDs)
	if err != nil {
		writeGrayErr(c, err)
		return
	}
	response.JSON(c, http.StatusOK, gin.H{"removed": removed, "entries": allowlistEntries(list)})
	h.audit.recordAudit(c, &p.ID, model.AuditActionGrayAllowlistRemove, "gray_allowlist", c.Param("version"),
		gin.H{"scope": "version", "removed": removed})
}

type linePatchReq struct {
	MinOS       optionalJSON    `json:"min_os"`
	MinAPILevel optionalIntJSON `json:"min_api_level"`
}

func (h *projectHandler) patchVersionLine(c *gin.Context) {
	p, err := h.projects.Resolve(c.Request.Context(), c.Param("project_ref"))
	if err != nil {
		writeProjectErr(c, err)
		return
	}
	var req linePatchReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, http.StatusBadRequest, "INVALID_REQUEST", "invalid json", nil)
		return
	}
	in := service.VersionLinePatchInput{}
	if req.MinOS.set {
		in.MinOSSet = true
		if !req.MinOS.null {
			v := req.MinOS.value
			in.MinOS = &v
		}
	}
	if req.MinAPILevel.set {
		in.MinAPISet = true
		in.MinAPILevel = req.MinAPILevel.value
	}
	line, err := h.projects.PatchVersionLine(c.Request.Context(), p.ID, c.Param("version"), c.Param("os"), c.Param("arch"), in)
	if err != nil {
		writeGrayErr(c, err)
		return
	}
	response.JSON(c, http.StatusOK, publicVersionLine(line, nil))
}

func parseTimeRange(c *gin.Context) (time.Time, time.Time) {
	to := time.Now().UTC()
	from := to.Add(-30 * 24 * time.Hour)
	if s := c.Query("to"); s != "" {
		if t, err := time.Parse(time.RFC3339, s); err == nil {
			to = t.UTC()
		}
	}
	if s := c.Query("from"); s != "" {
		if t, err := time.Parse(time.RFC3339, s); err == nil {
			from = t.UTC()
		}
	}
	return from, to
}
