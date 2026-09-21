package model

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// 审计主体类型（docs/app-init.md §11.2 / C15-3）。
const (
	// AuditActorAdmin 实例管理员（缓存会话 Token）。Fingerprint 记 username。
	AuditActorAdmin = "admin"
	// AuditActorProjectToken 项目 Token。Fingerprint 记 Token 指纹。
	AuditActorProjectToken = "project_token"
	// AuditActorCIToken CI Token。Fingerprint 记 Token 指纹。
	AuditActorCIToken = "ci_token"
)

// 审计动作（§11.2 最低集；命名 <资源>.<动作>）。
const (
	AuditActionVersionPublish      = "version.publish"       // 发布 Version
	AuditActionVersionDeprecate    = "version.deprecate"     // 弃用 Version
	AuditActionVersionRevoke       = "version.revoke"        // 吊销 Version
	AuditActionVersionDelete       = "version.delete"        // 删除 Version
	AuditActionVersionPromote      = "version.promote"       // 渠道晋升（C14-2）
	AuditActionArtifactReuse       = "artifact.reuse"        // 产物复用（C14-1）
	AuditActionArtifactDelete      = "artifact.delete"       // 产物删除（清理 / TUS 会话删除）
	AuditActionCITokenCreate       = "ci_token.create"       // 签发 CI Token
	AuditActionCITokenDelete       = "ci_token.delete"       // 删除 CI Token
	AuditActionProjectTokenCreate  = "project_token.create"  // 签发项目 Token
	AuditActionProjectTokenDelete  = "project_token.delete"  // 删除项目 Token
	AuditActionGrayAllowlistAdd    = "gray.allowlist.add"    // 灰度白名单写入（Version 级 / per-line）
	AuditActionGrayAllowlistRemove = "gray.allowlist.remove" // 灰度白名单删除
	AuditActionProjectUpdate       = "project.update"        // 项目配置变更（含 webhook 配置，C14-3）
	AuditActionTelemetryDelete     = "telemetry.delete"      // 按哈希隐私删除遥测（§11.2 / §13.11）
	AuditActionAnnouncementCreate  = "announcement.create"   // 创建公告
	AuditActionAnnouncementUpdate  = "announcement.update"   // 更新公告（含发布/取消发布/改作用域）
	AuditActionAnnouncementDelete  = "announcement.delete"   // 删除公告
	AuditActionAnnouncementReorder = "announcement.reorder"  // 重排公告顺序
	AuditActionLanguageCreate      = "language.create"       // 创建项目语言
	AuditActionLanguageUpdate      = "language.update"       // 更新项目语言（含设为默认）
	AuditActionLanguageDelete      = "language.delete"       // 删除项目语言
	AuditActionClientDelete        = "client.delete"         // 删除名册行并同步清本项目白名单
)

// AuditEvent 是管理动作审计事件（docs/app-init.md §11.2 / C15-3 / C15-5）。
//
// 用途：记录「谁、何时、对哪个项目、做了什么动作、作用于哪个资源」。
// 写入点在 controller 层成功响应之后（共享 helper），失败只记日志，
// 绝不阻塞业务请求。查询入口：GET /admin/projects/:ref/audit（游标分页）。
//
// 隐私边界（C15-5）：本表**永不存储明文 Token**。CI/项目 Token 主体只记
// Fingerprint（明文前 8 字符，model.CIToken/ProjectToken 落库时生成）；
// 管理员主体记 username。审计行不参与业务判定，仅供合规回溯。
//
// 关系：
//   - ProjectID 可空：实例级动作（未来扩展）为 NULL，当前最低集均带项目。
//   - 不设外键约束：审计是 append-only 观测记录，业务行删除不影响审计留存。
//
// 字段：
//   - ID：UUID v4 主键，应用侧生成。
//   - ActorType：admin | project_token | ci_token（上方常量）。
//   - ActorID：主体 UUID（管理员 / Token 的 ID）；无法解析时可空。
//   - ActorFingerprint：主体可读指纹（admin username / Token 指纹）；
//     明文 Token 永不入库。
//   - ProjectID：动作归属项目（可空）。
//   - Action：动作常量（上方 AuditAction*）。
//   - ResourceType / ResourceID：被作用资源的类型与 ID（如 version / UUID、
//     ci_token / UUID、device_hash / 哈希）。
//   - Detail：附加信息 jsonb（如 target_channel、removed 计数）；可空。
//   - CreatedAt：事件时间（UTC）。
//
// 索引：
//   - (project_id, created_at)：项目级审计查询（管理端点按项目回溯）；
//   - (actor_fingerprint, created_at)：按主体回溯（谁在何时做了什么）。
type AuditEvent struct {
	ID               uuid.UUID  `gorm:"type:uuid;primaryKey" json:"id"`
	ActorType        string     `gorm:"type:text;not null" json:"actor_type"`
	ActorID          *uuid.UUID `gorm:"type:uuid" json:"actor_id,omitempty"`
	ActorFingerprint string     `gorm:"type:text;not null;index:idx_audit_actor_created,priority:1" json:"actor_fingerprint"`
	ProjectID        *uuid.UUID `gorm:"type:uuid;index:idx_audit_project_created,priority:1" json:"project_id,omitempty"`
	Action           string     `gorm:"type:text;not null" json:"action"`
	ResourceType     string     `gorm:"type:text;not null" json:"resource_type"`
	ResourceID       string     `gorm:"type:text;not null" json:"resource_id"`
	Detail           JSONObject `gorm:"type:jsonb" json:"detail,omitempty"`
	CreatedAt        time.Time  `gorm:"index:idx_audit_project_created,priority:2;index:idx_audit_actor_created,priority:2" json:"created_at"`
}

// TableName 固定表名 audit_events，后续任务不得改名。
func (AuditEvent) TableName() string {
	return "audit_events"
}

// BeforeCreate 补 UUID 与创建时间。
func (a *AuditEvent) BeforeCreate(_ *gorm.DB) error {
	if a.ID == uuid.Nil {
		a.ID = uuid.New()
	}
	if a.CreatedAt.IsZero() {
		a.CreatedAt = time.Now().UTC()
	}
	return nil
}
