package admin

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/Kirizu-Official/KiriVers/internal/model"
	"github.com/Kirizu-Official/KiriVers/pkg/response"
)

func publicNode(n model.Node, now time.Time, currentID uuid.UUID) gin.H {
	return gin.H{
		"id":              n.ID,
		"display_name":    n.DisplayName,
		"admin_enabled":   n.AdminEnabled,
		"cpu_percent":     n.CPUPercent,
		"mem_used_bytes":  n.MemUsedBytes,
		"mem_total_bytes": n.MemTotalBytes,
		"last_seen_at":    formatTimeUTC(n.LastSeenAt),
		"offline":         n.Offline(now),
		"is_current":      currentID != uuid.Nil && n.ID == currentID,
		"created_at":      formatTimeUTC(n.CreatedAt),
		"updated_at":      formatTimeUTC(n.UpdatedAt),
	}
}

func publicNodeSync(row model.NodeArtifactSync, displayName string) gin.H {
	body := gin.H{
		"id":            row.ID,
		"node_id":       row.NodeID,
		"display_name":  displayName,
		"project_id":    row.ProjectID,
		"version_id":    row.VersionID,
		"status":        row.Status,
		"bytes_done":    row.BytesDone,
		"bytes_total":   row.BytesTotal,
		"error_message": row.ErrorMessage,
		"updated_at":    formatTimeUTC(row.UpdatedAt),
	}
	if row.LineID != nil {
		body["line_id"] = *row.LineID
	} else {
		body["line_id"] = nil
	}
	return body
}

func (h *projectHandler) listNodes(c *gin.Context) {
	list, err := h.projects.ListNodes(c.Request.Context())
	if err != nil {
		writeProjectErr(c, err)
		return
	}
	now := time.Now().UTC()
	currentID := h.projects.NodeID()
	items := make([]gin.H, 0, len(list))
	for i := range list {
		items = append(items, publicNode(list[i], now, currentID))
	}
	response.JSON(c, http.StatusOK, gin.H{
		"cluster_active": h.projects.ClusterActive(),
		"nodes":          items,
	})
}

func (h *projectHandler) listNodeSync(c *gin.Context) {
	list, err := h.projects.ListNodeSync(c.Request.Context(), c.Param("project_ref"))
	if err != nil {
		writeProjectErr(c, err)
		return
	}
	nodes, _ := h.projects.ListNodes(c.Request.Context())
	names := map[string]string{}
	for i := range nodes {
		names[nodes[i].ID.String()] = nodes[i].DisplayName
	}
	items := make([]gin.H, 0, len(list))
	for i := range list {
		items = append(items, publicNodeSync(list[i], names[list[i].NodeID.String()]))
	}
	response.JSON(c, http.StatusOK, gin.H{"sync": items})
}
