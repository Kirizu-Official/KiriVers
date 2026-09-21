package admin

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/Kirizu-Official/KiriVers/internal/model"
	"github.com/Kirizu-Official/KiriVers/pkg/response"
)

// 本文件提供 Publish webhook 投递记录的管理端查询（C14-4，§5.8）：
// GET /admin/projects/:ref/webhook/deliveries?cursor=&limit= ，游标分页排障用。
// WebhookSecret 永不出现在任何响应中（json:"-" + 手工字段白名单）。

// publicDelivery 组装投递记录 JSON。payload 原样回显便于核对事件体。
func publicDelivery(d *model.WebhookDelivery) gin.H {
	h := gin.H{
		"id":               d.ID,
		"project_id":       d.ProjectID,
		"event":            d.Event,
		"status":           d.Status,
		"attempts":         d.Attempts,
		"last_status_code": d.LastStatusCode,
		"created_at":       formatTimeUTC(d.CreatedAt),
	}
	if d.VersionID != nil {
		h["version_id"] = *d.VersionID
	}
	if d.LineID != nil {
		h["line_id"] = *d.LineID
	}
	if len(d.Payload) > 0 {
		h["payload"] = d.Payload
	}
	if d.LastError != "" {
		h["last_error"] = d.LastError
	}
	if d.DeliveredAt != nil {
		h["delivered_at"] = formatTimeUTC(*d.DeliveredAt)
	}
	return h
}

// listWebhookDeliveries 处理 GET /projects/:ref/webhook/deliveries（C14-4）。
// cursor 为上一页最后一条的投递 ID；limit 默认 50，上限 200。
func (h *projectHandler) listWebhookDeliveries(c *gin.Context) {
	p, err := h.projects.Resolve(c.Request.Context(), c.Param("project_ref"))
	if err != nil {
		writeProjectErr(c, err)
		return
	}

	var cursor *uuid.UUID
	if raw := strings.TrimSpace(c.Query("cursor")); raw != "" {
		id, parseErr := uuid.Parse(raw)
		if parseErr != nil {
			response.Error(c, http.StatusBadRequest, "INVALID_REQUEST", "invalid cursor", nil)
			return
		}
		cursor = &id
	}
	limit := 0
	if raw := strings.TrimSpace(c.Query("limit")); raw != "" {
		n, parseErr := strconv.Atoi(raw)
		if parseErr != nil || n < 1 {
			response.Error(c, http.StatusBadRequest, "INVALID_REQUEST", "invalid limit", nil)
			return
		}
		limit = n
	}
	if limit <= 0 {
		limit = 50
	}
	if limit > 200 {
		limit = 200
	}

	deliveries, err := h.projects.ListWebhookDeliveries(c.Request.Context(), p.ID, cursor, limit)
	if err != nil {
		response.Error(c, http.StatusInternalServerError, "INTERNAL_ERROR", "list webhook deliveries failed", nil)
		return
	}
	items := make([]gin.H, 0, len(deliveries))
	for i := range deliveries {
		items = append(items, publicDelivery(&deliveries[i]))
	}
	var nextCursor string
	if len(deliveries) == limit {
		nextCursor = deliveries[len(deliveries)-1].ID.String()
	}
	response.JSON(c, http.StatusOK, gin.H{
		"deliveries":  items,
		"next_cursor": nextCursor,
	})
}
