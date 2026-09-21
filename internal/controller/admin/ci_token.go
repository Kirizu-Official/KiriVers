package admin

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/Kirizu-Official/KiriVers/internal/model"
	"github.com/Kirizu-Official/KiriVers/pkg/response"
)

type createCITokenReq struct {
	Name      string   `json:"name"`
	Scopes    []string `json:"scopes"`
	ExpiresAt *string  `json:"expires_at,omitempty"`
}

func publicCIToken(t *model.CIToken) gin.H {
	h := gin.H{
		"id":          t.ID,
		"project_id":  t.ProjectID,
		"name":        t.Name,
		"scopes":      t.Scopes,
		"fingerprint": t.Fingerprint,
		"created_at":  formatTimeUTC(t.CreatedAt),
		"updated_at":  formatTimeUTC(t.UpdatedAt),
	}
	if t.ExpiresAt != nil {
		h["expires_at"] = formatTimeUTC(*t.ExpiresAt)
	} else {
		h["expires_at"] = nil
	}
	return h
}

// createCIToken 为指定项目创建专供 CI/CD 使用的 API Token（C07-1, §11.1）。
// 明文 Token 仅在此创建响应中返回一次。
func (h *projectHandler) createCIToken(c *gin.Context) {
	var req createCITokenReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, http.StatusBadRequest, "INVALID_REQUEST", "invalid json", nil)
		return
	}
	var exp *time.Time
	if req.ExpiresAt != nil && *req.ExpiresAt != "" {
		t, err := time.Parse(time.RFC3339, *req.ExpiresAt)
		if err != nil {
			response.Error(c, http.StatusBadRequest, "INVALID_REQUEST", "invalid expires_at", nil)
			return
		}
		utc := t.UTC()
		exp = &utc
	}
	issued, err := h.projects.CreateCIToken(c.Request.Context(), c.Param("project_ref"), req.Name, req.Scopes, exp)
	if err != nil {
		writeProjectErr(c, err)
		return
	}
	body := publicCIToken(issued.Token)
	body["token"] = issued.Plaintext
	response.JSON(c, http.StatusCreated, body)
	// 成功后写审计（C15-3）：CI Token 指纹不在此重复（明文也绝不入审计，C15-5）。
	h.audit.recordAudit(c, &issued.Token.ProjectID, model.AuditActionCITokenCreate,
		"ci_token", issued.Token.ID.String(), gin.H{"name": issued.Token.Name, "scopes": issued.Token.Scopes})
}

// listCITokens 列出项目的 CI Token 列表（绝不返回明文与哈希，仅包含指纹）。
func (h *projectHandler) listCITokens(c *gin.Context) {
	list, err := h.projects.ListCITokens(c.Request.Context(), c.Param("project_ref"))
	if err != nil {
		writeProjectErr(c, err)
		return
	}
	items := make([]gin.H, 0, len(list))
	for i := range list {
		items = append(items, publicCIToken(&list[i]))
	}
	response.JSON(c, http.StatusOK, gin.H{"tokens": items, "ci_tokens": items})
}

// deleteCIToken 吊销并删除指定 CI Token。
func (h *projectHandler) deleteCIToken(c *gin.Context) {
	id, err := uuid.Parse(c.Param("token_id"))
	if err != nil {
		response.Error(c, http.StatusBadRequest, "INVALID_REQUEST", "invalid token id", nil)
		return
	}
	if err := h.projects.DeleteCIToken(c.Request.Context(), c.Param("project_ref"), id); err != nil {
		writeProjectErr(c, err)
		return
	}
	c.Status(http.StatusNoContent)
	// 成功后写审计（C15-3）。
	h.audit.recordAudit(c, h.projectIDFromRef(c), model.AuditActionCITokenDelete, "ci_token", id.String(), nil)
}
