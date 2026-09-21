package admin

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/Kirizu-Official/KiriVers/internal/middleware"
	"github.com/Kirizu-Official/KiriVers/internal/model"
	"github.com/Kirizu-Official/KiriVers/internal/service"
	"github.com/Kirizu-Official/KiriVers/pkg/response"
)

// 审计接入（docs/app-init.md §11.2 / C15-3 / C15-5）。
//
// 约定：
//   - 仅在**成功响应之后**调用 recordAudit——失败路径不产生审计行；
//   - 主体身份来自既有中间件注入的 context 值（AdminAuth 的 admin id+username、
//     ProjectAccess 的 project_token_id+fingerprint、CIRateLimit 键来源
//     ContextCITokenFingerprint）；无法识别主体（未认证）不记审计；
//   - **永不接收明文 Token**：本文件只消费 Fingerprint / username（C15-5，
//     集成测试断言审计行内容不含明文）；
//   - Record 失败在 service 层吞掉并只记日志，绝不阻塞业务响应。

// auditHelper 包装 AuditService，供各 projectHandler 复用（nil 安全）。
type auditHelper struct {
	svc *service.AuditService
}

// auditActor 是从请求上下文解析出的审计主体。
type auditActor struct {
	actorType        string     // model.AuditActor*
	actorID          *uuid.UUID // 主体 UUID（可空）
	actorFingerprint string     // username 或 Token 指纹
}

// auditActorFromContext 依据中间件注入的 context 值解析审计主体。
// 优先级：实例管理员 > 项目 Token > CI Token。三者皆缺（未认证）返回 false，
// 调用方跳过审计（未认证身份不产生审计行）。
func auditActorFromContext(c *gin.Context) (auditActor, bool) {
	// 1. 实例管理员会话 Token（AdminAuth / RequireInstanceAdmin / ProjectAccess 注入）。
	if idRaw := c.GetString(middleware.ContextAdminID); idRaw != "" {
		actor := auditActor{
			actorType:        model.AuditActorAdmin,
			actorFingerprint: c.GetString(middleware.ContextAdminUsername),
		}
		if id, err := uuid.Parse(idRaw); err == nil {
			actor.actorID = &id
		}
		return actor, true
	}
	// 2. 项目 Token（ProjectAccess / AnyAdminOrProjectAccess 注入）。
	if idRaw := c.GetString(middleware.ContextProjectTokenID); idRaw != "" {
		actor := auditActor{
			actorType:        model.AuditActorProjectToken,
			actorFingerprint: c.GetString(middleware.ContextProjectTokenFingerprint),
		}
		if id, err := uuid.Parse(idRaw); err == nil {
			actor.actorID = &id
		}
		return actor, true
	}
	// 3. CI Token（ProjectAccess 注入指纹；任务查询注入 ci_token_id）。
	if fp := c.GetString(middleware.ContextCITokenFingerprint); fp != "" {
		actor := auditActor{
			actorType:        model.AuditActorCIToken,
			actorFingerprint: fp,
		}
		if idRaw := c.GetString("ci_token_id"); idRaw != "" {
			if id, err := uuid.Parse(idRaw); err == nil {
				actor.actorID = &id
			}
		}
		return actor, true
	}
	return auditActor{}, false
}

// recordAudit 组装并落库一条审计事件（成功响应后调用；nil 安全）。
// projectID 为动作归属项目（nil = 实例级 / 项目未解析）。主体不可识别时
// 静默跳过；写入失败由 service 层吞掉（只记日志）。
func (h *auditHelper) recordAudit(c *gin.Context, projectID *uuid.UUID, action, resourceType, resourceID string, detail gin.H) {
	if h == nil || h.svc == nil {
		return
	}
	actor, ok := auditActorFromContext(c)
	if !ok {
		return
	}
	h.svc.Record(c.Request.Context(), service.AuditEntry{
		ActorType:        actor.actorType,
		ActorID:          actor.actorID,
		ActorFingerprint: actor.actorFingerprint,
		ProjectID:        projectID,
		Action:           action,
		ResourceType:     resourceType,
		ResourceID:       resourceID,
		Detail:           detail,
	})
}

// auditListDefaultLimit / auditListMaxLimit 管理端审计查询分页默认与硬顶。
const (
	auditListDefaultLimit = 50
	auditListMaxLimit     = 500
)

// listAudit 处理 GET /api/v1/admin/projects/:project_ref/audit?cursor=&limit=
// （C15-3）：项目级审计事件倒序游标分页。audit 服务未装配时返回 503。
func (h *projectHandler) listAudit(c *gin.Context) {
	p, err := h.projects.Resolve(c.Request.Context(), c.Param("project_ref"))
	if err != nil {
		writeProjectErr(c, err)
		return
	}
	if h.audit == nil || h.audit.svc == nil {
		response.Error(c, http.StatusServiceUnavailable, "NOT_READY", "audit store unavailable", nil)
		return
	}
	limit := 0
	if raw := c.Query("limit"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n <= 0 {
			response.Error(c, http.StatusBadRequest, "INVALID_REQUEST", "limit must be a positive integer", nil)
			return
		}
		limit = n
	}
	events, next, err := h.audit.svc.ListByProject(c.Request.Context(), p.ID, c.Query("cursor"), limit)
	if err != nil {
		response.Error(c, http.StatusBadRequest, "INVALID_REQUEST", "invalid cursor", nil)
		return
	}
	response.JSON(c, http.StatusOK, gin.H{
		"events":      events,
		"next_cursor": next,
	})
}
