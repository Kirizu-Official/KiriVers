package model

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

const (
	// NodeIdentityFileName 是本地数据目录下的节点 UUID 文件名（0600）。
	NodeIdentityFileName = ".kirivers-node-id"
	// NodeOfflineAfter 超过该空闲时间则管理台标为离线（不自动删行）。
	NodeOfflineAfter = 30 * time.Second
	// NodeHeartbeatInterval 是本进程写 CPU/内存/last_seen 的周期。
	NodeHeartbeatInterval = 10 * time.Second

	// NodeSyncStatusSyncing 本机代拉进行中。
	NodeSyncStatusSyncing = "syncing"
	// NodeSyncStatusReady 本节点对该版本线已就绪（本机代拉副本完成，或 S3 直链无需拷贝）。
	NodeSyncStatusReady = "ready"
	// NodeSyncStatusError 拉取失败。
	NodeSyncStatusError = "error"
)

// Node 是共享 Postgres 上的进程身份行。连库后由本节点 upsert，管理台读库，不经 HTTP 汇报。
//
// 用途：多节点时展示 UUID、显示名、最后在线、CPU/内存；心跳由本进程 UPDATE。
//
// 关系：无 FK。Job.OwnerNodeID 与 NodeArtifactSync.NodeID 软引用本表主键。
//
// 字段：
//   - ID：节点 UUID，来自身份文件或新生成；主键冲突则重试。
//   - DisplayName：仅 UI，可空。
//   - AdminEnabled：本进程是否监听管理平面。
//   - CPUPercent / MemUsedBytes / MemTotalBytes：心跳快照。
//   - LastSeenAt：最近一次心跳。
type Node struct {
	ID            uuid.UUID `gorm:"type:uuid;primaryKey" json:"id"`
	DisplayName   string    `gorm:"type:text;not null;default:''" json:"display_name"`
	AdminEnabled  bool      `gorm:"not null;default:true" json:"admin_enabled"`
	CPUPercent    float64   `gorm:"not null;default:0" json:"cpu_percent"`
	MemUsedBytes  int64     `gorm:"not null;default:0" json:"mem_used_bytes"`
	MemTotalBytes int64     `gorm:"not null;default:0" json:"mem_total_bytes"`
	LastSeenAt    time.Time `json:"last_seen_at"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

// TableName 固定表名 nodes。
func (Node) TableName() string {
	return "nodes"
}

// BeforeCreate 在插入前补 UUID（RFC 4122 v4）。
func (n *Node) BeforeCreate(_ *gorm.DB) error {
	if n.ID == uuid.Nil {
		id, err := uuid.NewRandom()
		if err != nil {
			return err
		}
		n.ID = id
	}
	return nil
}

// Offline 表示心跳已超过 NodeOfflineAfter。
func (n *Node) Offline(now time.Time) bool {
	if n == nil || n.LastSeenAt.IsZero() {
		return true
	}
	return now.Sub(n.LastSeenAt) > NodeOfflineAfter
}

// NodeArtifactSync 记录某节点对某版本线产物的本机副本进度。
//
// 用途：本机代拉时管理台项目页展示各节点同步状态；S3 直链可直接标 ready 而不拷贝字节。
//
// 关系：软引用 nodes / projects / versions / version_lines，无 FK。
//
// 字段：
//   - NodeID / ProjectID / VersionID / LineID：同步范围；LineID 可空表示版本级。
//   - Status：syncing | ready | error。
//   - BytesDone / BytesTotal：拉取进度。
//   - ErrorMessage：失败原因。
//   - 唯一约束由 SQL extras idx_node_artifact_sync_scope (COALESCE line_id) 保证，避免多行 NULL 互异。
type NodeArtifactSync struct {
	ID           uuid.UUID  `gorm:"type:uuid;primaryKey" json:"id"`
	NodeID       uuid.UUID  `gorm:"type:uuid;not null" json:"node_id"`
	ProjectID    uuid.UUID  `gorm:"type:uuid;not null;index" json:"project_id"`
	VersionID    uuid.UUID  `gorm:"type:uuid;not null" json:"version_id"`
	LineID       *uuid.UUID `gorm:"type:uuid" json:"line_id,omitempty"`
	Status       string     `gorm:"type:text;not null;default:syncing" json:"status"`
	BytesDone    int64      `gorm:"not null;default:0" json:"bytes_done"`
	BytesTotal   int64      `gorm:"not null;default:0" json:"bytes_total"`
	ErrorMessage string     `gorm:"type:text" json:"error_message,omitempty"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
}

// TableName 固定表名 node_artifact_sync。
func (NodeArtifactSync) TableName() string {
	return "node_artifact_sync"
}

// BeforeCreate 补 UUID。
func (n *NodeArtifactSync) BeforeCreate(_ *gorm.DB) error {
	if n.ID == uuid.Nil {
		id, err := uuid.NewRandom()
		if err != nil {
			return err
		}
		n.ID = id
	}
	if n.Status == "" {
		n.Status = NodeSyncStatusSyncing
	}
	return nil
}
