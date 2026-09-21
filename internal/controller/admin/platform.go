package admin

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/Kirizu-Official/KiriVers/internal/model"
	"github.com/Kirizu-Official/KiriVers/internal/platform"
	"github.com/Kirizu-Official/KiriVers/internal/service"
	"github.com/Kirizu-Official/KiriVers/pkg/response"
)

type channelWriteReq struct {
	Name          *string `json:"name"`
	Slug          *string `json:"slug"`
	StabilityRank *int    `json:"stability_rank"`
	Enabled       *bool   `json:"enabled"`
	Unlisted      *bool   `json:"unlisted"`
	Token         *string `json:"token"`
}

type matrixWriteReq struct {
	OS                      *string `json:"os"`
	Arch                    *string `json:"arch"`
	PackageType             *string `json:"package_type"`
	DeltaAlgo               *string `json:"delta_algo"`
	DeltaSourceCount        *int    `json:"delta_source_count"`
	HwVariantPolicy         *string `json:"hw_variant_policy"`
	FallbackArch            *string `json:"fallback_arch"`
	MinimumSupportedVersion *string `json:"minimum_supported_version"`
}

type hwRevWriteReq struct {
	Slug  *string `json:"slug"`
	Rank  *int    `json:"rank"`
	Notes *string `json:"notes"`
}

func (h *projectHandler) listChannels(c *gin.Context) {
	p, err := h.projects.Resolve(c.Request.Context(), c.Param("project_ref"))
	if err != nil {
		writeProjectErr(c, err)
		return
	}
	list, err := h.projects.ListChannels(c.Request.Context(), p.ID)
	if err != nil {
		writeProjectErr(c, err)
		return
	}
	items := make([]gin.H, 0, len(list))
	for i := range list {
		items = append(items, publicChannel(&list[i]))
	}
	response.JSON(c, http.StatusOK, gin.H{"channels": items})
}

func (h *projectHandler) createChannel(c *gin.Context) {
	p, err := h.projects.Resolve(c.Request.Context(), c.Param("project_ref"))
	if err != nil {
		writeProjectErr(c, err)
		return
	}
	var req channelWriteReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, http.StatusBadRequest, "INVALID_REQUEST", "invalid json", nil)
		return
	}
	ch, err := h.projects.CreateChannel(c.Request.Context(), p.ID, service.ChannelWrite{
		Name:          req.Name,
		Slug:          req.Slug,
		StabilityRank: req.StabilityRank,
		Enabled:       req.Enabled,
		Unlisted:      req.Unlisted,
		Token:         req.Token,
	})
	if err != nil {
		writeProjectErr(c, err)
		return
	}
	response.JSON(c, http.StatusCreated, publicChannel(ch))
}

func (h *projectHandler) patchChannel(c *gin.Context) {
	p, err := h.projects.Resolve(c.Request.Context(), c.Param("project_ref"))
	if err != nil {
		writeProjectErr(c, err)
		return
	}
	var req channelWriteReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, http.StatusBadRequest, "INVALID_REQUEST", "invalid json", nil)
		return
	}
	ch, err := h.projects.PatchChannel(c.Request.Context(), p.ID, c.Param("slug"), service.ChannelWrite{
		Name:          req.Name,
		StabilityRank: req.StabilityRank,
		Enabled:       req.Enabled,
		Unlisted:      req.Unlisted,
		Token:         req.Token,
	})
	if err != nil {
		writeProjectErr(c, err)
		return
	}
	response.JSON(c, http.StatusOK, publicChannel(ch))
}

func (h *projectHandler) deleteChannel(c *gin.Context) {
	p, err := h.projects.Resolve(c.Request.Context(), c.Param("project_ref"))
	if err != nil {
		writeProjectErr(c, err)
		return
	}
	if err := h.projects.DeleteChannel(c.Request.Context(), p.ID, c.Param("slug")); err != nil {
		writeProjectErr(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *projectHandler) listMatrix(c *gin.Context) {
	p, err := h.projects.Resolve(c.Request.Context(), c.Param("project_ref"))
	if err != nil {
		writeProjectErr(c, err)
		return
	}
	list, err := h.projects.ListMatrix(c.Request.Context(), p.ID)
	if err != nil {
		writeProjectErr(c, err)
		return
	}
	items := make([]gin.H, 0, len(list))
	for i := range list {
		items = append(items, publicMatrix(&list[i]))
	}
	response.JSON(c, http.StatusOK, gin.H{"matrix": items})
}

func (h *projectHandler) createMatrix(c *gin.Context) {
	p, err := h.projects.Resolve(c.Request.Context(), c.Param("project_ref"))
	if err != nil {
		writeProjectErr(c, err)
		return
	}
	var req matrixWriteReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, http.StatusBadRequest, "INVALID_REQUEST", "invalid json", nil)
		return
	}
	row, err := h.projects.CreateMatrix(c.Request.Context(), p.ID, req.toInput())
	if err != nil {
		writeProjectErr(c, err)
		return
	}
	response.JSON(c, http.StatusCreated, publicMatrix(row))
}

func (h *projectHandler) patchMatrix(c *gin.Context) {
	p, err := h.projects.Resolve(c.Request.Context(), c.Param("project_ref"))
	if err != nil {
		writeProjectErr(c, err)
		return
	}
	var req matrixWriteReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, http.StatusBadRequest, "INVALID_REQUEST", "invalid json", nil)
		return
	}
	row, err := h.projects.PatchMatrix(c.Request.Context(), p.ID, c.Param("os"), c.Param("arch"), req.toInput())
	if err != nil {
		writeProjectErr(c, err)
		return
	}
	response.JSON(c, http.StatusOK, publicMatrix(row))
}

func (h *projectHandler) listHwRevs(c *gin.Context) {
	p, err := h.projects.Resolve(c.Request.Context(), c.Param("project_ref"))
	if err != nil {
		writeProjectErr(c, err)
		return
	}
	list, err := h.projects.ListHwRevs(c.Request.Context(), p.ID)
	if err != nil {
		writeProjectErr(c, err)
		return
	}
	items := make([]gin.H, 0, len(list))
	for i := range list {
		items = append(items, publicHwRev(&list[i]))
	}
	response.JSON(c, http.StatusOK, gin.H{"hw_revs": items})
}

func (h *projectHandler) createHwRev(c *gin.Context) {
	p, err := h.projects.Resolve(c.Request.Context(), c.Param("project_ref"))
	if err != nil {
		writeProjectErr(c, err)
		return
	}
	var req hwRevWriteReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, http.StatusBadRequest, "INVALID_REQUEST", "invalid json", nil)
		return
	}
	hw, err := h.projects.CreateHwRev(c.Request.Context(), p.ID, service.HwRevWrite{
		Slug:  req.Slug,
		Rank:  req.Rank,
		Notes: req.Notes,
	})
	if err != nil {
		writeProjectErr(c, err)
		return
	}
	response.JSON(c, http.StatusCreated, publicHwRev(hw))
}

func (h *projectHandler) patchHwRev(c *gin.Context) {
	p, err := h.projects.Resolve(c.Request.Context(), c.Param("project_ref"))
	if err != nil {
		writeProjectErr(c, err)
		return
	}
	var req hwRevWriteReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, http.StatusBadRequest, "INVALID_REQUEST", "invalid json", nil)
		return
	}
	hw, err := h.projects.PatchHwRev(c.Request.Context(), p.ID, c.Param("slug"), service.HwRevWrite{
		Rank:  req.Rank,
		Notes: req.Notes,
	})
	if err != nil {
		writeProjectErr(c, err)
		return
	}
	response.JSON(c, http.StatusOK, publicHwRev(hw))
}

func (h *projectHandler) deleteHwRev(c *gin.Context) {
	p, err := h.projects.Resolve(c.Request.Context(), c.Param("project_ref"))
	if err != nil {
		writeProjectErr(c, err)
		return
	}
	if err := h.projects.DeleteHwRev(c.Request.Context(), p.ID, c.Param("slug")); err != nil {
		writeProjectErr(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *projectHandler) platformCatalog(c *gin.Context) {
	if _, err := h.projects.Resolve(c.Request.Context(), c.Param("project_ref")); err != nil {
		writeProjectErr(c, err)
		return
	}
	response.JSON(c, http.StatusOK, gin.H{
		"os":   platform.OSCatalog(),
		"arch": platform.ArchCatalog(),
	})
}

func (r matrixWriteReq) toInput() service.MatrixWrite {
	return service.MatrixWrite{
		OS:                      r.OS,
		Arch:                    r.Arch,
		PackageType:             r.PackageType,
		DeltaAlgo:               r.DeltaAlgo,
		DeltaSourceCount:        r.DeltaSourceCount,
		HwVariantPolicy:         r.HwVariantPolicy,
		FallbackArch:            r.FallbackArch,
		MinimumSupportedVersion: r.MinimumSupportedVersion,
	}
}

func publicChannel(ch *model.Channel) gin.H {
	return gin.H{
		"id":             ch.ID,
		"project_id":     ch.ProjectID,
		"name":           ch.Name,
		"slug":           ch.Slug,
		"stability_rank": ch.StabilityRank,
		"enabled":        ch.Enabled,
		"unlisted":       ch.Unlisted,
		"system":         ch.System,
		"token":          ch.TokenPlain,
		"token_required": ch.TokenProtected(),
		"created_at":     formatTimeUTC(ch.CreatedAt),
		"updated_at":     formatTimeUTC(ch.UpdatedAt),
	}
}

func publicMatrix(row *model.PlatformMatrix) gin.H {
	return gin.H{
		"id":                        row.ID,
		"project_id":                row.ProjectID,
		"os":                        row.OS,
		"arch":                      row.Arch,
		"package_type":              row.PackageType,
		"delta_algo":                row.DeltaAlgo,
		"delta_source_count":        row.DeltaSourceCount,
		"hw_variant_policy":         row.HwVariantPolicy,
		"fallback_arch":             row.FallbackArch,
		"minimum_supported_version": row.MinimumSupportedVersion,
		"created_at":                formatTimeUTC(row.CreatedAt),
		"updated_at":                formatTimeUTC(row.UpdatedAt),
	}
}

func publicHwRev(hw *model.HwRev) gin.H {
	return gin.H{
		"id":         hw.ID,
		"project_id": hw.ProjectID,
		"slug":       hw.Slug,
		"rank":       hw.Rank,
		"notes":      hw.Notes,
		"created_at": formatTimeUTC(hw.CreatedAt),
		"updated_at": formatTimeUTC(hw.UpdatedAt),
	}
}
