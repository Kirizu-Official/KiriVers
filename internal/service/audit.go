package service

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog"

	"github.com/Kirizu-Official/KiriVers/internal/model"
	"github.com/Kirizu-Official/KiriVers/internal/repository"
)

// 审计服务（docs/app-init.md §11.2 / C15-3 / C15-5）。
//
// 写入点约定：controller 层在**成功响应之后**调用 Record（共享 helper
// auditFromContext），任何写库失败只记日志、绝不阻塞业务请求——审计是
// 观测数据，可用性低于主业务。进程崩溃时最后一条可能丢失（design §5
// trade-off，可接受）。
//
// 隐私边界（C15-5）：AuditEntry 只携带 Fingerprint（Token 明文前 8 字符）
// 或管理员 username，永不接收明文 Token；helper 层亦无明文可传。

// AuditEntry 是一次审计写入的请求描述（controller 层组装，service 层落库）。
type AuditEntry struct {
	// ActorType 主体类型：model.AuditActorAdmin | AuditActorProjectToken | AuditActorCIToken。
	ActorType string
	// ActorID 主体 UUID（管理员 / Token 的 ID）；nil 表示未解析。
	ActorID *uuid.UUID
	// ActorFingerprint 主体可读指纹：admin = username；Token = 指纹。
	// 永不接受明文 Token（C15-5，测试断言锁定）。
	ActorFingerprint string
	// ProjectID 动作归属项目；nil = 实例级。
	ProjectID *uuid.UUID
	// Action 动作常量（model.AuditAction*，如 version.publish）。
	Action string
	// ResourceType / ResourceID 被作用资源（如 version / UUID）。
	ResourceType string
	ResourceID   string
	// Detail 附加信息（如 removed 计数、target_channel）；可空。
	Detail map[string]any
}

// AuditService 承载审计事件的写入与项目级查询。
type AuditService struct {
	store repository.AuditStore
	log   zerolog.Logger
	// now 可注入时钟（测试用）；nil 时用 time.Now。
	now func() time.Time
}

// NewAuditService 构造审计服务。log 使用进程系统 logger 的 mod=audit 子 logger；测试传 zerolog.Nop()。
func NewAuditService(store repository.AuditStore, log zerolog.Logger) *AuditService {
	return &AuditService{store: store, log: log}
}

// SetClock 注入时钟（测试用）。
func (s *AuditService) SetClock(now func() time.Time) { s.now = now }

// Record 落库一条审计事件。任何失败只记日志（结构化 warn），不向调用方
// 返回错误——审计绝不阻塞业务请求（§11.2，design §5）。
func (s *AuditService) Record(ctx context.Context, entry AuditEntry) {
	event := &model.AuditEvent{
		ActorType:        entry.ActorType,
		ActorID:          entry.ActorID,
		ActorFingerprint: entry.ActorFingerprint,
		ProjectID:        entry.ProjectID,
		Action:           entry.Action,
		ResourceType:     entry.ResourceType,
		ResourceID:       entry.ResourceID,
		Detail:           cloneJSONObjectPtr(entry.Detail),
	}
	if s.now != nil {
		event.CreatedAt = s.now().UTC()
	}
	if err := s.store.Insert(ctx, event); err != nil {
		s.log.Warn().
			Str("action", entry.Action).
			Str("actor_fingerprint", entry.ActorFingerprint).
			Err(err).
			Msg("audit record write failed (business request unaffected)")
	}
}

// ListByProject 项目级审计查询（GET /admin/projects/:ref/audit）：
// 倒序游标分页，透传仓储结果。
func (s *AuditService) ListByProject(ctx context.Context, projectID uuid.UUID, cursor string, limit int) ([]model.AuditEvent, string, error) {
	return s.store.ListByProject(ctx, projectID, cursor, limit)
}

// cloneJSONObjectPtr 把 map 转为 model.JSONObject 并拷贝，避免调用方后续
// 修改影响已落库内容；空 map 归一为 nil（列保持 NULL）。
func cloneJSONObjectPtr(m map[string]any) model.JSONObject {
	if len(m) == 0 {
		return nil
	}
	out := make(model.JSONObject, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}
