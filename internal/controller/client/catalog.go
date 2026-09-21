package client

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/Kirizu-Official/KiriVers/internal/middleware"
	"github.com/Kirizu-Official/KiriVers/internal/model"
	"github.com/Kirizu-Official/KiriVers/internal/service"
	"github.com/Kirizu-Official/KiriVers/pkg/response"
)

// catalogHandler 承载公开渠道目录与矩阵目录 GET。
type catalogHandler struct {
	projects *service.ProjectService
}

// listChannels 返回 enabled && !unlisted && 无 Token 的渠道（空列表仍 200）。
func (h *catalogHandler) listChannels(c *gin.Context) {
	p := middleware.ProjectFrom(c)
	if p == nil {
		response.Error(c, http.StatusNotFound, "PROJECT_NOT_FOUND", "project not found", nil)
		return
	}
	list, err := h.projects.ListChannels(c.Request.Context(), p.ID)
	if err != nil {
		response.Error(c, http.StatusInternalServerError, "INTERNAL_ERROR", "internal server error", nil)
		return
	}
	items := make([]gin.H, 0, len(list))
	for i := range list {
		ch := &list[i]
		if !ch.Enabled || ch.Unlisted || ch.TokenProtected() {
			continue
		}
		items = append(items, publicClientChannel(ch))
	}
	response.JSON(c, http.StatusOK, gin.H{"channels": items})
}

// listMatrix 返回全部矩阵行的 os/arch/package_type。
func (h *catalogHandler) listMatrix(c *gin.Context) {
	p := middleware.ProjectFrom(c)
	if p == nil {
		response.Error(c, http.StatusNotFound, "PROJECT_NOT_FOUND", "project not found", nil)
		return
	}
	rows, err := h.projects.ListMatrix(c.Request.Context(), p.ID)
	if err != nil {
		response.Error(c, http.StatusInternalServerError, "INTERNAL_ERROR", "internal server error", nil)
		return
	}
	items := make([]gin.H, 0, len(rows))
	for i := range rows {
		items = append(items, gin.H{
			"os":           rows[i].OS,
			"arch":         rows[i].Arch,
			"package_type": rows[i].PackageType,
		})
	}
	response.JSON(c, http.StatusOK, gin.H{"matrix": items})
}

// listLanguages 返回项目语言目录（含默认行）。空列表仍 200。不含 project_id。
func (h *catalogHandler) listLanguages(c *gin.Context) {
	p := middleware.ProjectFrom(c)
	if p == nil {
		response.Error(c, http.StatusNotFound, "PROJECT_NOT_FOUND", "project not found", nil)
		return
	}
	list, err := h.projects.ListLanguages(c.Request.Context(), p.ID)
	if err != nil {
		response.Error(c, http.StatusInternalServerError, "INTERNAL_ERROR", "internal server error", nil)
		return
	}
	items := make([]gin.H, 0, len(list))
	for i := range list {
		items = append(items, gin.H{
			"code":         list[i].Code,
			"display_name": list[i].DisplayName,
			"is_default":   list[i].IsDefault,
			"sort_order":   list[i].SortOrder,
		})
	}
	response.JSON(c, http.StatusOK, gin.H{"languages": items})
}

func publicClientChannel(ch *model.Channel) gin.H {
	return gin.H{
		"name":           ch.Name,
		"slug":           ch.Slug,
		"stability_rank": ch.StabilityRank,
	}
}
