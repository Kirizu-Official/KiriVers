package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/Kirizu-Official/KiriVers/internal/model"
)

// NodeStore 是节点身份与本机代拉进度的持久化合同。
type NodeStore interface {
	Create(ctx context.Context, node *model.Node) error
	GetByID(ctx context.Context, id uuid.UUID) (*model.Node, error)
	Save(ctx context.Context, node *model.Node) error
	List(ctx context.Context) ([]model.Node, error)
	UpdateHeartbeat(ctx context.Context, id uuid.UUID, cpu float64, memUsed, memTotal int64, lastSeen time.Time) error
	ListSyncByProject(ctx context.Context, projectID uuid.UUID) ([]model.NodeArtifactSync, error)
	UpsertSync(ctx context.Context, row *model.NodeArtifactSync) error
}

// NodeRepo 是 PostgreSQL 实现。
type NodeRepo struct {
	db *gorm.DB
}

// NewNodeRepo 构造仓储。
func NewNodeRepo(db *gorm.DB) *NodeRepo {
	return &NodeRepo{db: db}
}

// Create 插入节点行；主键冲突由调用方重试。
func (r *NodeRepo) Create(ctx context.Context, node *model.Node) error {
	if r == nil || r.db == nil {
		return fmt.Errorf("node repo is nil")
	}
	return r.db.WithContext(ctx).Create(node).Error
}

// GetByID 按主键读取。
func (r *NodeRepo) GetByID(ctx context.Context, id uuid.UUID) (*model.Node, error) {
	if r == nil || r.db == nil {
		return nil, fmt.Errorf("node repo is nil")
	}
	var n model.Node
	if err := r.db.WithContext(ctx).Where("id = ?", id).Take(&n).Error; err != nil {
		return nil, err
	}
	return &n, nil
}

// Save 更新展示字段与心跳快照。
func (r *NodeRepo) Save(ctx context.Context, node *model.Node) error {
	if r == nil || r.db == nil {
		return fmt.Errorf("node repo is nil")
	}
	return r.db.WithContext(ctx).Save(node).Error
}

// List 返回全部节点，按 last_seen_at 降序。
func (r *NodeRepo) List(ctx context.Context) ([]model.Node, error) {
	if r == nil || r.db == nil {
		return nil, fmt.Errorf("node repo is nil")
	}
	var list []model.Node
	if err := r.db.WithContext(ctx).Order("last_seen_at DESC").Find(&list).Error; err != nil {
		return nil, err
	}
	return list, nil
}

// UpdateHeartbeat 只写 CPU/内存/last_seen，避免覆盖其它列。
func (r *NodeRepo) UpdateHeartbeat(ctx context.Context, id uuid.UUID, cpu float64, memUsed, memTotal int64, lastSeen time.Time) error {
	if r == nil || r.db == nil {
		return fmt.Errorf("node repo is nil")
	}
	res := r.db.WithContext(ctx).Model(&model.Node{}).Where("id = ?", id).Updates(map[string]any{
		"cpu_percent":     cpu,
		"mem_used_bytes":  memUsed,
		"mem_total_bytes": memTotal,
		"last_seen_at":    lastSeen,
	})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

// ListSyncByProject 列出某项目全部节点同步行。
func (r *NodeRepo) ListSyncByProject(ctx context.Context, projectID uuid.UUID) ([]model.NodeArtifactSync, error) {
	if r == nil || r.db == nil {
		return nil, fmt.Errorf("node repo is nil")
	}
	var list []model.NodeArtifactSync
	if err := r.db.WithContext(ctx).Where("project_id = ?", projectID).Order("updated_at DESC").Find(&list).Error; err != nil {
		return nil, err
	}
	return list, nil
}

// UpsertSync 按 (node_id, version_id, line_id) 插入或更新进度。
func (r *NodeRepo) UpsertSync(ctx context.Context, row *model.NodeArtifactSync) error {
	if r == nil || r.db == nil {
		return fmt.Errorf("node repo is nil")
	}
	if row == nil {
		return errors.New("sync row is nil")
	}
	return r.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "node_id"}, {Name: "version_id"}, {Name: "line_id"}},
		DoUpdates: clause.AssignmentColumns([]string{
			"status", "bytes_done", "bytes_total", "error_message", "project_id", "updated_at",
		}),
	}).Create(row).Error
}
