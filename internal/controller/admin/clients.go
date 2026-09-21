package admin

import (
	"errors"
	"net/http"
	"sort"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/Kirizu-Official/KiriVers/internal/model"
	"github.com/Kirizu-Official/KiriVers/internal/service"
	"github.com/Kirizu-Official/KiriVers/pkg/response"
)

func publicClient(cl *model.Client) gin.H {
	h := gin.H{
		"id":           cl.ID,
		"project_id":   cl.ProjectID,
		"device_hash":  cl.DeviceHash,
		"last_version": cl.LastVersion,
		"last_os":      cl.LastOS,
		"last_arch":    cl.LastArch,
		"last_channel": cl.LastChannel,
		"last_ip":      cl.LastIP,
		"country_code": cl.CountryCode,
		"region_code":  cl.RegionCode,
		"geo_i18n":     cl.GeoI18n,
		"custom":       cl.Custom,
		"created_at":   formatTimeUTC(cl.CreatedAt),
		"updated_at":   formatTimeUTC(cl.UpdatedAt),
	}
	if cl.LastCheckAt != nil {
		h["last_check_at"] = formatTimeUTC(*cl.LastCheckAt)
	}
	return h
}

func (h *projectHandler) listClients(c *gin.Context) {
	p, err := h.projects.Resolve(c.Request.Context(), c.Param("project_ref"))
	if err != nil {
		writeProjectErr(c, err)
		return
	}
	limit, _ := strconv.Atoi(c.Query("limit"))
	offset, _ := strconv.Atoi(c.Query("offset"))
	q := service.ClientListQuery{
		Q: c.Query("q"), OS: c.Query("os"), Arch: c.Query("arch"),
		Version: c.Query("version"), Limit: limit, Offset: offset,
	}
	if s := c.Query("active_since"); s != "" {
		if t, err := time.Parse(time.RFC3339, s); err == nil {
			utc := t.UTC()
			q.ActiveSince = &utc
		}
	}
	list, total, err := h.projects.ListClients(c.Request.Context(), p.ID, q)
	if err != nil {
		writeGrayErr(c, err)
		return
	}
	items := make([]gin.H, 0, len(list))
	for i := range list {
		items = append(items, publicClient(&list[i]))
	}
	response.JSON(c, http.StatusOK, gin.H{"clients": items, "total": total})
}

func (h *projectHandler) getClient(c *gin.Context) {
	p, err := h.projects.Resolve(c.Request.Context(), c.Param("project_ref"))
	if err != nil {
		writeProjectErr(c, err)
		return
	}
	id, err := uuid.Parse(c.Param("client_id"))
	if err != nil {
		response.Error(c, http.StatusBadRequest, "INVALID_REQUEST", "invalid client id", nil)
		return
	}
	cl, err := h.projects.GetClient(c.Request.Context(), p.ID, id)
	if err != nil {
		writeGrayErr(c, err)
		return
	}
	response.JSON(c, http.StatusOK, publicClient(cl))
}

func (h *projectHandler) deleteClient(c *gin.Context) {
	p, err := h.projects.Resolve(c.Request.Context(), c.Param("project_ref"))
	if err != nil {
		writeProjectErr(c, err)
		return
	}
	id, err := uuid.Parse(c.Param("client_id"))
	if err != nil {
		response.Error(c, http.StatusBadRequest, "INVALID_REQUEST", "invalid client id", nil)
		return
	}
	if err := h.projects.DeleteClient(c.Request.Context(), p.ID, id); err != nil {
		if errors.Is(err, service.ErrClientNotFound) {
			response.Error(c, http.StatusNotFound, "CLIENT_NOT_FOUND", err.Error(), nil)
			return
		}
		writeGrayErr(c, err)
		return
	}
	c.Status(http.StatusNoContent)
	h.audit.recordAudit(c, &p.ID, model.AuditActionClientDelete, "client", id.String(), nil)
}

func (h *projectHandler) clientStats(c *gin.Context) {
	p, err := h.projects.Resolve(c.Request.Context(), c.Param("project_ref"))
	if err != nil {
		writeProjectErr(c, err)
		return
	}
	buckets, err := h.projects.ClientBucketStats(c.Request.Context(), p.ID)
	if err != nil {
		writeGrayErr(c, err)
		return
	}
	response.JSON(c, http.StatusOK, buckets)
}

func (h *projectHandler) statsSeries(c *gin.Context) {
	p, err := h.projects.Resolve(c.Request.Context(), c.Param("project_ref"))
	if err != nil {
		writeProjectErr(c, err)
		return
	}
	from, to := parseTimeRange(c)
	rows, err := h.projects.ClientDailySeries(c.Request.Context(), p.ID, from, to)
	if err != nil {
		writeGrayErr(c, err)
		return
	}
	tel, err := h.projects.TelemetryDailySeries(c.Request.Context(), p.ID, from, to)
	if err != nil {
		writeGrayErr(c, err)
		return
	}
	byDay := map[string]gin.H{}
	for i := range rows {
		r := &rows[i]
		day := r.Day.UTC().Format("2006-01-02")
		item := gin.H{
			"day":             day,
			"new_count":       r.NewCount,
			"active_count":    r.ActiveCount,
			"installed_count": 0,
			"failed_count":    0,
		}
		if t, ok := tel[day]; ok {
			item["installed_count"] = t.Installed
			item["failed_count"] = t.Failed
		}
		byDay[day] = item
	}
	for day, t := range tel {
		if _, ok := byDay[day]; ok {
			continue
		}
		byDay[day] = gin.H{
			"day":             day,
			"new_count":       0,
			"active_count":    0,
			"installed_count": t.Installed,
			"failed_count":    t.Failed,
		}
	}
	days := make([]string, 0, len(byDay))
	for day := range byDay {
		days = append(days, day)
	}
	sort.Strings(days)
	items := make([]gin.H, 0, len(days))
	for _, day := range days {
		items = append(items, byDay[day])
	}
	response.JSON(c, http.StatusOK, gin.H{"series": items})
}
