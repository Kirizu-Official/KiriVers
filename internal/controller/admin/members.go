package admin

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/Kirizu-Official/KiriVers/internal/middleware"
	"github.com/Kirizu-Official/KiriVers/internal/service"
	"github.com/Kirizu-Official/KiriVers/pkg/response"
)

type memberWriteReq struct {
	Username string  `json:"username"`
	Password *string `json:"password"`
	Role     string  `json:"role"`
}

type memberPatchReq struct {
	Role string `json:"role"`
}

func publicMember(m service.MemberView) gin.H {
	return gin.H{
		"admin_id":   m.AdminID,
		"username":   m.Username,
		"role":       m.Role,
		"created_at": formatTimeUTC(m.CreatedAt),
	}
}

func (h *projectHandler) listMembers(c *gin.Context) {
	if middleware.AdminFrom(c) == nil {
		response.Error(c, http.StatusForbidden, "FORBIDDEN", "human admin session required", nil)
		return
	}
	p, err := h.projects.Resolve(c.Request.Context(), c.Param("project_ref"))
	if err != nil {
		writeProjectErr(c, err)
		return
	}
	list, err := h.projects.ListMembers(c.Request.Context(), p.ID)
	if err != nil {
		writeProjectErr(c, err)
		return
	}
	items := make([]gin.H, 0, len(list))
	for _, m := range list {
		items = append(items, publicMember(m))
	}
	response.JSON(c, http.StatusOK, gin.H{"members": items})
}

func (h *projectHandler) createMember(c *gin.Context) {
	actor := middleware.AdminFrom(c)
	if actor == nil {
		response.Error(c, http.StatusForbidden, "FORBIDDEN", "human admin session required", nil)
		return
	}
	p, err := h.projects.Resolve(c.Request.Context(), c.Param("project_ref"))
	if err != nil {
		writeProjectErr(c, err)
		return
	}
	var req memberWriteReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, http.StatusBadRequest, "INVALID_REQUEST", "invalid json", nil)
		return
	}
	m, err := h.projects.AddMember(c.Request.Context(), p.ID, actor, service.MemberWrite{
		Username: req.Username, Password: req.Password, Role: req.Role,
	})
	if err != nil {
		writeProjectErr(c, err)
		return
	}
	response.JSON(c, http.StatusCreated, publicMember(*m))
}

func (h *projectHandler) patchMember(c *gin.Context) {
	actor := middleware.AdminFrom(c)
	if actor == nil {
		response.Error(c, http.StatusForbidden, "FORBIDDEN", "human admin session required", nil)
		return
	}
	p, err := h.projects.Resolve(c.Request.Context(), c.Param("project_ref"))
	if err != nil {
		writeProjectErr(c, err)
		return
	}
	adminID, err := uuid.Parse(c.Param("admin_id"))
	if err != nil {
		response.Error(c, http.StatusBadRequest, "INVALID_REQUEST", "invalid admin id", nil)
		return
	}
	var req memberPatchReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, http.StatusBadRequest, "INVALID_REQUEST", "invalid json", nil)
		return
	}
	m, err := h.projects.PatchMember(c.Request.Context(), p.ID, adminID, actor, req.Role)
	if err != nil {
		writeProjectErr(c, err)
		return
	}
	response.JSON(c, http.StatusOK, publicMember(*m))
}

func (h *projectHandler) deleteMember(c *gin.Context) {
	actor := middleware.AdminFrom(c)
	if actor == nil {
		response.Error(c, http.StatusForbidden, "FORBIDDEN", "human admin session required", nil)
		return
	}
	p, err := h.projects.Resolve(c.Request.Context(), c.Param("project_ref"))
	if err != nil {
		writeProjectErr(c, err)
		return
	}
	adminID, err := uuid.Parse(c.Param("admin_id"))
	if err != nil {
		response.Error(c, http.StatusBadRequest, "INVALID_REQUEST", "invalid admin id", nil)
		return
	}
	if err := h.projects.RemoveMember(c.Request.Context(), p.ID, adminID, actor); err != nil {
		writeProjectErr(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}
