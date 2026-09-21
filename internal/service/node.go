package service

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog"
	"gorm.io/gorm"

	"github.com/Kirizu-Official/KiriVers/internal/model"
	"github.com/Kirizu-Official/KiriVers/internal/repository"
	"github.com/Kirizu-Official/KiriVers/internal/storage"
)

const nodeRegisterAttempts = 8

// NodeService 负责本进程 UUID 文件、nodes 表 upsert 与心跳。
type NodeService struct {
	store         repository.NodeStore
	localRoot     string
	displayName   string
	adminEnabled  bool
	log           zerolog.Logger
	newUUID       func() (uuid.UUID, error)
	mu            sync.RWMutex
	id            uuid.UUID
	heartbeatFn   func() (cpu float64, memUsed, memTotal int64)
	onBeat        func(context.Context)
}

// NodeServiceOptions 是 Register 输入。
type NodeServiceOptions struct {
	Store        repository.NodeStore
	LocalRoot    string
	DisplayName  string
	AdminEnabled bool
	Logger       zerolog.Logger
	NewUUID      func() (uuid.UUID, error)
}

// NewNodeService 构造服务。localRoot 为 storage.local.root。
func NewNodeService(opts NodeServiceOptions) *NodeService {
	n := &NodeService{
		store:        opts.Store,
		localRoot:    opts.LocalRoot,
		displayName:  strings.TrimSpace(opts.DisplayName),
		adminEnabled: opts.AdminEnabled,
		log:          opts.Logger,
		newUUID:      opts.NewUUID,
		heartbeatFn:  snapshotProcMem,
	}
	if n.newUUID == nil {
		n.newUUID = uuid.NewRandom
	}
	return n
}

// ID 返回已登记的节点 UUID；未 Register 时为零值。
func (n *NodeService) ID() uuid.UUID {
	if n == nil {
		return uuid.Nil
	}
	n.mu.RLock()
	defer n.mu.RUnlock()
	return n.id
}

// SetOnBeat 在心跳成功后调用（本机代拉补齐副本）。
func (n *NodeService) SetOnBeat(fn func(context.Context)) {
	if n == nil {
		return
	}
	n.onBeat = fn
}

// Store 返回底层仓储（供管理查询）。
func (n *NodeService) Store() repository.NodeStore {
	if n == nil {
		return nil
	}
	return n.store
}

// Register 读取或写入 {local.root}/.kirivers-node-id，并 upsert nodes 行。
func (n *NodeService) Register(ctx context.Context) (*model.Node, error) {
	if n == nil || n.store == nil {
		return nil, fmt.Errorf("node store is nil")
	}
	if strings.TrimSpace(n.localRoot) == "" {
		return nil, fmt.Errorf("storage local root is empty")
	}
	if err := os.MkdirAll(n.localRoot, 0o755); err != nil {
		return nil, fmt.Errorf("mkdir node identity dir: %w", err)
	}
	path := filepath.Join(n.localRoot, storage.NodeIdentityFileName)
	if raw, err := os.ReadFile(path); err == nil {
		if id, perr := uuid.Parse(strings.TrimSpace(string(raw))); perr == nil && id != uuid.Nil {
			return n.upsertExisting(ctx, id)
		}
	}
	for i := 0; i < nodeRegisterAttempts; i++ {
		id, err := n.newUUID()
		if err != nil {
			return nil, err
		}
		now := time.Now().UTC()
		row := &model.Node{
			ID:           id,
			DisplayName:  n.displayName,
			AdminEnabled: n.adminEnabled,
			LastSeenAt:   now,
			CreatedAt:    now,
			UpdatedAt:    now,
		}
		if err := n.store.Create(ctx, row); err != nil {
			if isUniqueViolation(err) {
				continue
			}
			return nil, err
		}
		if err := writeNodeIdentityFile(path, id); err != nil {
			return nil, err
		}
		n.mu.Lock()
		n.id = id
		n.mu.Unlock()
		return row, nil
	}
	return nil, errors.New("unable to allocate unique node id")
}

func (n *NodeService) upsertExisting(ctx context.Context, id uuid.UUID) (*model.Node, error) {
	now := time.Now().UTC()
	row, err := n.store.GetByID(ctx, id)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		row = &model.Node{
			ID:           id,
			DisplayName:  n.displayName,
			AdminEnabled: n.adminEnabled,
			LastSeenAt:   now,
			CreatedAt:    now,
			UpdatedAt:    now,
		}
		if err := n.store.Create(ctx, row); err != nil {
			return nil, err
		}
	} else if err != nil {
		return nil, err
	} else {
		row.DisplayName = n.displayName
		row.AdminEnabled = n.adminEnabled
		row.LastSeenAt = now
		if err := n.store.Save(ctx, row); err != nil {
			return nil, err
		}
	}
	n.mu.Lock()
	n.id = id
	n.mu.Unlock()
	return row, nil
}

// StartHeartbeat 每 10s 写 CPU/内存/last_seen。ctx 取消后退出。
func (n *NodeService) StartHeartbeat(ctx context.Context) {
	if n == nil || n.store == nil {
		return
	}
	go func() {
		ticker := time.NewTicker(model.NodeHeartbeatInterval)
		defer ticker.Stop()
		n.beat(ctx)
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				n.beat(ctx)
			}
		}
	}()
}

func (n *NodeService) beat(ctx context.Context) {
	id := n.ID()
	if id == uuid.Nil {
		return
	}
	cpu, used, total := n.heartbeatFn()
	if err := n.store.UpdateHeartbeat(ctx, id, cpu, used, total, time.Now().UTC()); err != nil && n.log.GetLevel() <= zerolog.DebugLevel {
		n.log.Debug().Err(err).Msg("node heartbeat")
	}
	if n.onBeat != nil {
		n.onBeat(ctx)
	}
}

// List 返回全部节点。
func (n *NodeService) List(ctx context.Context) ([]model.Node, error) {
	if n == nil || n.store == nil {
		return nil, fmt.Errorf("node store is nil")
	}
	return n.store.List(ctx)
}

// ListProjectSync 返回某项目各节点同步行。
func (n *NodeService) ListProjectSync(ctx context.Context, projectID uuid.UUID) ([]model.NodeArtifactSync, error) {
	if n == nil || n.store == nil {
		return nil, fmt.Errorf("node store is nil")
	}
	return n.store.ListSyncByProject(ctx, projectID)
}

func writeNodeIdentityFile(path string, id uuid.UUID) error {
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, []byte(id.String()+"\n"), 0o600); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}

func snapshotProcMem() (cpu float64, memUsed, memTotal int64) {
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	return 0, int64(ms.HeapAlloc), int64(ms.Sys)
}
