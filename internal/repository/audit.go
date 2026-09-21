package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/Kirizu-Official/KiriVers/internal/model"
)

// AuditStore 是审计事件（§11.2 / C15-3）的持久化接口：append-only 写入与
// 项目级游标分页查询。PostgreSQL 与内存实现各一份（memory_audit.go）。
type AuditStore interface {
	// Insert 写入一条审计事件（服务端补 ID / CreatedAt）。
	Insert(ctx context.Context, event *model.AuditEvent) error
	// ListByProject 按项目倒序（新→旧）分页返回审计事件。
	// cursor 为上一响应的 next_cursor（空 = 首页）；返回下一页游标（无更多时为空）。
	ListByProject(ctx context.Context, projectID uuid.UUID, cursor string, limit int) ([]model.AuditEvent, string, error)
}

// auditCursorDefaultLimit / auditCursorMaxLimit 分页默认与硬顶（管理端查询
// 防护；与 webhook 投递列表同一量级策略）。
const (
	auditCursorDefaultLimit = 50
	auditCursorMaxLimit     = 500
)

// AuditRepo 是 AuditStore 的 PostgreSQL（GORM）实现。
type AuditRepo struct {
	db *gorm.DB
}

// NewAuditRepo 构造仓储。
func NewAuditRepo(db *gorm.DB) *AuditRepo {
	return &AuditRepo{db: db}
}

// Insert 写入一条审计事件。
func (r *AuditRepo) Insert(ctx context.Context, event *model.AuditEvent) error {
	if err := r.db.WithContext(ctx).Create(event).Error; err != nil {
		return fmt.Errorf("insert audit event: %w", err)
	}
	return nil
}

// ListByProject 项目级倒序分页（命中 idx_audit_project_created）。
// 游标 = "created_at(RFC3339Nano)|id"；元组比较避免同刻事件跨页丢行。
func (r *AuditRepo) ListByProject(ctx context.Context, projectID uuid.UUID, cursor string, limit int) ([]model.AuditEvent, string, error) {
	limit = clampAuditLimit(limit)
	q := r.db.WithContext(ctx).
		Where("project_id = ?", projectID).
		Order("created_at DESC, id DESC").
		Limit(limit + 1) // 多取一条判断有无下一页
	if cursor != "" {
		at, id, err := decodeAuditCursor(cursor)
		if err != nil {
			return nil, "", fmt.Errorf("decode audit cursor: %w", err)
		}
		q = q.Where("(created_at, id) < (?, ?)", at, id)
	}
	var rows []model.AuditEvent
	if err := q.Find(&rows).Error; err != nil {
		return nil, "", fmt.Errorf("list audit events: %w", err)
	}
	var next string
	if len(rows) > limit {
		rows = rows[:limit]
		last := rows[len(rows)-1]
		next = encodeAuditCursor(last.CreatedAt, last.ID)
	}
	return rows, next, nil
}

// clampAuditLimit 钳制分页条数。
func clampAuditLimit(limit int) int {
	if limit <= 0 {
		return auditCursorDefaultLimit
	}
	if limit > auditCursorMaxLimit {
		return auditCursorMaxLimit
	}
	return limit
}

// encodeAuditCursor 游标编码：created_at(RFC3339Nano) + "|" + id。
func encodeAuditCursor(at time.Time, id uuid.UUID) string {
	return at.UTC().Format(time.RFC3339Nano) + "|" + id.String()
}

// decodeAuditCursor 解码游标；非法游标报错（HTTP 400 由上层映射）。
func decodeAuditCursor(cursor string) (time.Time, uuid.UUID, error) {
	sep := -1
	for i := len(cursor) - 1; i >= 0; i-- {
		if cursor[i] == '|' {
			sep = i
			break
		}
	}
	if sep < 0 {
		return time.Time{}, uuid.Nil, errors.New("malformed cursor")
	}
	at, err := time.Parse(time.RFC3339Nano, cursor[:sep])
	if err != nil {
		return time.Time{}, uuid.Nil, err
	}
	id, err := uuid.Parse(cursor[sep+1:])
	if err != nil {
		return time.Time{}, uuid.Nil, err
	}
	return at, id, nil
}
