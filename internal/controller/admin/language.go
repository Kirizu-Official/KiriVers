package admin

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/Kirizu-Official/KiriVers/internal/model"
	"github.com/Kirizu-Official/KiriVers/internal/service"
	"github.com/Kirizu-Official/KiriVers/pkg/response"
)

type languageWriteReq struct {
	Code        *string `json:"code"`
	DisplayName *string `json:"display_name"`
	SortOrder   *int    `json:"sort_order"`
	IsDefault   *bool   `json:"is_default"`
}

func (h *projectHandler) listLanguages(c *gin.Context) {
	p, err := h.projects.Resolve(c.Request.Context(), c.Param("project_ref"))
	if err != nil {
		writeProjectErr(c, err)
		return
	}
	list, err := h.projects.ListLanguages(c.Request.Context(), p.ID)
	if err != nil {
		writeProjectErr(c, err)
		return
	}
	items := make([]gin.H, 0, len(list))
	for i := range list {
		items = append(items, publicLanguage(&list[i]))
	}
	response.JSON(c, http.StatusOK, gin.H{"languages": items})
}

func (h *projectHandler) createLanguage(c *gin.Context) {
	p, err := h.projects.Resolve(c.Request.Context(), c.Param("project_ref"))
	if err != nil {
		writeProjectErr(c, err)
		return
	}
	var req languageWriteReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, http.StatusBadRequest, "INVALID_REQUEST", "invalid json", nil)
		return
	}
	lang, err := h.projects.CreateLanguage(c.Request.Context(), p.ID, service.LanguageWrite{
		Code:        req.Code,
		DisplayName: req.DisplayName,
		SortOrder:   req.SortOrder,
		IsDefault:   req.IsDefault,
	})
	if err != nil {
		writeProjectErr(c, err)
		return
	}
	response.JSON(c, http.StatusCreated, publicLanguage(lang))
	h.audit.recordAudit(c, &p.ID, model.AuditActionLanguageCreate, "language", lang.Code, nil)
}

func (h *projectHandler) patchLanguage(c *gin.Context) {
	p, err := h.projects.Resolve(c.Request.Context(), c.Param("project_ref"))
	if err != nil {
		writeProjectErr(c, err)
		return
	}
	var req languageWriteReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, http.StatusBadRequest, "INVALID_REQUEST", "invalid json", nil)
		return
	}
	lang, err := h.projects.PatchLanguage(c.Request.Context(), p.ID, c.Param("code"), service.LanguageWrite{
		DisplayName: req.DisplayName,
		SortOrder:   req.SortOrder,
		IsDefault:   req.IsDefault,
	})
	if err != nil {
		writeProjectErr(c, err)
		return
	}
	response.JSON(c, http.StatusOK, publicLanguage(lang))
	h.audit.recordAudit(c, &p.ID, model.AuditActionLanguageUpdate, "language", lang.Code, nil)
}

func (h *projectHandler) deleteLanguage(c *gin.Context) {
	p, err := h.projects.Resolve(c.Request.Context(), c.Param("project_ref"))
	if err != nil {
		writeProjectErr(c, err)
		return
	}
	code := c.Param("code")
	if err := h.projects.DeleteLanguage(c.Request.Context(), p.ID, code); err != nil {
		writeProjectErr(c, err)
		return
	}
	c.Status(http.StatusNoContent)
	h.audit.recordAudit(c, &p.ID, model.AuditActionLanguageDelete, "language", code, nil)
}

func publicLanguage(lang *model.ProjectLanguage) gin.H {
	return gin.H{
		"id":           lang.ID,
		"project_id":   lang.ProjectID,
		"code":         lang.Code,
		"display_name": lang.DisplayName,
		"sort_order":   lang.SortOrder,
		"is_default":   lang.IsDefault,
		"created_at":   formatTimeUTC(lang.CreatedAt),
		"updated_at":   formatTimeUTC(lang.UpdatedAt),
	}
}
