package admin

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/Kirizu-Official/KiriVers/internal/middleware"
	"github.com/Kirizu-Official/KiriVers/internal/model"
	"github.com/Kirizu-Official/KiriVers/internal/service"
	"github.com/Kirizu-Official/KiriVers/pkg/response"
)

// telemetryHandler 承载遥测隐私删除入口（§11.2 / §13.11 / C15-4）。
// 删除同时覆盖遥测事件与灰度白名单行（Version 级 + per-line），并写审计。
type telemetryHandler struct {
	projects  *service.ProjectService
	telemetry *service.TelemetryService
	audit     *auditHelper
}

// deleteTelemetryDevice 处理
// DELETE /api/v1/admin/projects/:project_ref/telemetry/devices/:device_hash：
// 按哈希删除该设备在本项目内的全部遥测事件与灰度白名单行（§11.2 / C15-4）。
// 项目不存在 → 404；未知哈希为幂等成功。响应体附删除计数
// {telemetry_deleted, allowlist_deleted}（200；204 无法携带响应体，
// 故以 200 表达幂等成功并交付计数）。device_hash 本身已是哈希形态，可安全入日志。
func (h *telemetryHandler) deleteTelemetryDevice(c *gin.Context) {
	p, err := h.projects.Resolve(c.Request.Context(), c.Param("project_ref"))
	if err != nil {
		writeProjectErr(c, err)
		return
	}
	if h.telemetry == nil {
		response.Error(c, http.StatusServiceUnavailable, "NOT_READY", "database unavailable", nil)
		return
	}
	telemetryDeleted, allowlistDeleted, err := h.telemetry.DeleteDeviceData(c.Request.Context(), p.ID, c.Param("device_hash"))
	if err != nil {
		response.Error(c, http.StatusInternalServerError, "INTERNAL_ERROR", "internal server error", nil)
		return
	}
	response.JSON(c, http.StatusOK, gin.H{
		"telemetry_deleted": telemetryDeleted,
		"allowlist_deleted": allowlistDeleted,
	})
	// 成功响应后写审计（§11.2）：主体来自中间件上下文，失败不阻塞。
	h.audit.recordAudit(c, &p.ID, model.AuditActionTelemetryDelete, "device_hash", c.Param("device_hash"), gin.H{
		"telemetry_deleted": telemetryDeleted,
		"allowlist_deleted": allowlistDeleted,
	})
}

// RegisterTelemetry 在管理端挂载遥测隐私删除路由（需 project:admin scope；
// 管理 API 不参与公开/CI 限流，C11-6）。telemetry 为 nil 时仍注册路由，
// 由 handler 返回 503（与无数据库语义一致）。audit 可 nil（不写审计）。
func RegisterTelemetry(rg *gin.RouterGroup, admins *service.AdminService, projects *service.ProjectService, telemetry *service.TelemetryService, audit *service.AuditService) {
	if projects == nil {
		return
	}
	h := &telemetryHandler{projects: projects, telemetry: telemetry, audit: &auditHelper{svc: audit}}
	g := rg.Group("/projects/:project_ref")
	g.Use(middleware.ProjectAccess(admins, projects, model.ScopeProjectAdmin))
	g.DELETE("/telemetry/devices/:device_hash", h.deleteTelemetryDevice)
}
