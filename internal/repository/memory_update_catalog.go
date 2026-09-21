package repository

import (
	"context"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/Kirizu-Official/KiriVers/internal/model"
	"github.com/Kirizu-Official/KiriVers/internal/service/update"
)

// MemoryUpdateCatalog 是 update.CatalogLoader 的进程内实现，
// 直接从 MemoryProjectStore 读取，供单元测试与 handler 集成测试使用
// （与 GORM 实现共享 assembleUpdateCatalog 装配逻辑，保证两侧行为一致）。
type MemoryUpdateCatalog struct {
	store *MemoryProjectStore
}

// NewMemoryUpdateCatalog 构造内存目录加载器。
func NewMemoryUpdateCatalog(store *MemoryProjectStore) *MemoryUpdateCatalog {
	return &MemoryUpdateCatalog{store: store}
}

// LoadCatalog 实现 update.CatalogLoader。
func (m *MemoryUpdateCatalog) LoadCatalog(_ context.Context, projectID uuid.UUID, os, arch string) (*update.Catalog, error) {
	m.store.mu.Lock()
	defer m.store.mu.Unlock()

	proj, ok := m.store.projects[projectID]
	if !ok || proj.DeletedAt.Valid {
		return nil, gorm.ErrRecordNotFound
	}

	channels := make([]model.Channel, 0, len(m.store.channels))
	for _, ch := range m.store.channels {
		if ch.ProjectID == projectID {
			channels = append(channels, *cloneChannel(ch))
		}
	}

	rows := make([]model.PlatformMatrix, 0, len(m.store.matrix))
	for _, row := range m.store.matrix {
		if row.ProjectID == projectID {
			rows = append(rows, *cloneMatrix(row))
		}
	}

	hwRevs := make([]model.HwRev, 0, len(m.store.hwRevs))
	for _, hw := range m.store.hwRevs {
		if hw.ProjectID == projectID {
			hwRevs = append(hwRevs, *hw)
		}
	}

	versions := make([]model.Version, 0, len(m.store.versions))
	for _, v := range m.store.versions {
		if v.ProjectID == projectID {
			versions = append(versions, *cloneVersion(v))
		}
	}

	// fallback_arch 决定第二平台切片范围。
	fallbackArch := ""
	for i := range rows {
		if rows[i].OS == os && rows[i].Arch == arch {
			fallbackArch = rows[i].FallbackArch
			break
		}
	}
	archSet := map[string]bool{arch: true}
	if fallbackArch != "" {
		archSet[fallbackArch] = true
	}

	lines := make([]model.VersionLine, 0, len(m.store.versionLines))
	lineIDs := make([]uuid.UUID, 0, len(m.store.versionLines))
	for _, l := range m.store.versionLines {
		if l.ProjectID != projectID || l.OS != os || !archSet[l.Arch] {
			continue
		}
		lines = append(lines, *cloneVersionLine(l))
		lineIDs = append(lineIDs, l.ID)
	}

	artifacts := make([]model.Artifact, 0, len(m.store.artifacts))
	for _, a := range m.store.artifacts {
		if a.Kind != model.ArtifactKindFull && a.Kind != model.ArtifactKindStoreFull {
			continue
		}
		for _, id := range lineIDs {
			if a.VersionLineID == id {
				artifacts = append(artifacts, *a)
				break
			}
		}
	}

	// 灰度白名单（§5.4 / C12）：Version 级全量 + 已装载线的 per-line 条目。
	// 本函数已持有 m.store.mu，使用无锁内核（与 GORM 装载同口径）；
	// 金丝雀名单量级小，随快照一次装载，选目标保持纯函数零额外查询。
	versionAllow := sortAllowlist(m.store.listVersionAllowlistLocked(projectID))

	return assembleUpdateCatalog(proj, channels, rows, hwRevs, versions, lines, artifacts, versionAllow, os, arch), nil
}

// cloneChannel 渠道浅拷贝（无可变引用类型字段）。
func cloneChannel(ch *model.Channel) *model.Channel {
	c := *ch
	return &c
}
