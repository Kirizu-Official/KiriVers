package admin

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/Kirizu-Official/KiriVers/internal/model"
	"github.com/Kirizu-Official/KiriVers/internal/service"
	"github.com/Kirizu-Official/KiriVers/pkg/response"
)

type installPolicyPutReq struct {
	Entries []service.InstallPolicyEntry `json:"entries"`
}

func requireInstallPolicyOSArch(c *gin.Context) (osSlug, archSlug string, ok bool) {
	osSlug = strings.TrimSpace(c.Query("os"))
	archSlug = strings.TrimSpace(c.Query("arch"))
	if osSlug == "" || archSlug == "" {
		response.Error(c, http.StatusBadRequest, "INVALID_REQUEST", "os and arch are required", nil)
		return "", "", false
	}
	return osSlug, archSlug, true
}

func publicInstallPolicyEntries(entries []service.InstallPolicyEntry) []service.InstallPolicyEntry {
	if entries == nil {
		return []service.InstallPolicyEntry{}
	}
	return entries
}

func (h *projectHandler) getProjectInstallPolicy(c *gin.Context) {
	p, err := h.projects.Resolve(c.Request.Context(), c.Param("project_ref"))
	if err != nil {
		writeProjectErr(c, err)
		return
	}
	osSlug, archSlug, ok := requireInstallPolicyOSArch(c)
	if !ok {
		return
	}
	entries, err := h.projects.ListProjectInstallPolicy(c.Request.Context(), p.ID, osSlug, archSlug)
	if err != nil {
		writeProjectErr(c, err)
		return
	}
	response.JSON(c, http.StatusOK, gin.H{"entries": publicInstallPolicyEntries(entries)})
}

func (h *projectHandler) putProjectInstallPolicy(c *gin.Context) {
	p, err := h.projects.Resolve(c.Request.Context(), c.Param("project_ref"))
	if err != nil {
		writeProjectErr(c, err)
		return
	}
	osSlug, archSlug, ok := requireInstallPolicyOSArch(c)
	if !ok {
		return
	}
	var req installPolicyPutReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, http.StatusBadRequest, "INVALID_REQUEST", "invalid json", nil)
		return
	}
	entries, err := h.projects.PutProjectInstallPolicy(c.Request.Context(), p.ID, osSlug, archSlug, req.Entries)
	if err != nil {
		writeProjectErr(c, err)
		return
	}
	response.JSON(c, http.StatusOK, gin.H{"entries": publicInstallPolicyEntries(entries)})
}

func (h *projectHandler) getChannelInstallPolicy(c *gin.Context) {
	p, err := h.projects.Resolve(c.Request.Context(), c.Param("project_ref"))
	if err != nil {
		writeProjectErr(c, err)
		return
	}
	osSlug, archSlug, ok := requireInstallPolicyOSArch(c)
	if !ok {
		return
	}
	entries, effective, err := h.projects.ListChannelInstallPolicy(c.Request.Context(), p.ID, c.Param("slug"), osSlug, archSlug)
	if err != nil {
		writeProjectErr(c, err)
		return
	}
	response.JSON(c, http.StatusOK, gin.H{
		"entries":   publicInstallPolicyEntries(entries),
		"effective": publicInstallPolicyEntries(effective),
	})
}

func (h *projectHandler) putChannelInstallPolicy(c *gin.Context) {
	p, err := h.projects.Resolve(c.Request.Context(), c.Param("project_ref"))
	if err != nil {
		writeProjectErr(c, err)
		return
	}
	osSlug, archSlug, ok := requireInstallPolicyOSArch(c)
	if !ok {
		return
	}
	var req installPolicyPutReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, http.StatusBadRequest, "INVALID_REQUEST", "invalid json", nil)
		return
	}
	entries, effective, err := h.projects.PutChannelInstallPolicy(c.Request.Context(), p.ID, c.Param("slug"), osSlug, archSlug, req.Entries)
	if err != nil {
		writeProjectErr(c, err)
		return
	}
	response.JSON(c, http.StatusOK, gin.H{
		"entries":   publicInstallPolicyEntries(entries),
		"effective": publicInstallPolicyEntries(effective),
	})
}

func (h *projectHandler) getInstallPolicyReference(c *gin.Context) {
	p, err := h.projects.Resolve(c.Request.Context(), c.Param("project_ref"))
	if err != nil {
		writeProjectErr(c, err)
		return
	}
	osSlug, archSlug, ok := requireInstallPolicyOSArch(c)
	if !ok {
		return
	}
	channel := strings.TrimSpace(c.Query("channel"))
	if channel == "" {
		channel = model.ChannelStable
	}
	ref, err := h.projects.LatestChannelPlatformManifest(c.Request.Context(), p.ID, channel, osSlug, archSlug)
	if err != nil {
		writeProjectErr(c, err)
		return
	}
	entries := ref.Entries
	if entries == nil {
		entries = []model.ManifestEntry{}
	}
	response.JSON(c, http.StatusOK, gin.H{
		"version": ref.Version,
		"status":  ref.Status,
		"entries": publicManifestEntries(entries),
	})
}
