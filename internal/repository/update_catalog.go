package repository

import (
	"context"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/Kirizu-Official/KiriVers/internal/model"
	"github.com/Kirizu-Official/KiriVers/internal/service/update"
)

// UpdateCatalogRepo 是 update.CatalogLoader 的 PostgreSQL（GORM）实现。
//
// 一次性装配选目标所需的目录快照：项目全部 Version、这些 Version 在
// (os,arch) 与 (os,fallback_arch) 上的 Line、线上的 kind=full Artifact、
// 渠道表、全部矩阵行与硬件代号表。单项目版本量级为数百，配合 CDN
// s-maxage 短缓存，DB 压力有界（task design §2）。
type UpdateCatalogRepo struct {
	db *gorm.DB
	// projects 复用 ProjectStore 的白名单装载方法（ListVersionAllowlist /
	// ListLineAllowlist），避免同一查询在两处重复。
	projects *ProjectRepo
}

// NewUpdateCatalogRepo 构造仓储。
func NewUpdateCatalogRepo(db *gorm.DB) *UpdateCatalogRepo {
	return &UpdateCatalogRepo{db: db, projects: NewProjectRepo(db)}
}

// LoadCatalog 实现 update.CatalogLoader。
func (r *UpdateCatalogRepo) LoadCatalog(ctx context.Context, projectID uuid.UUID, os, arch string) (*update.Catalog, error) {
	db := r.db.WithContext(ctx)

	var proj model.Project
	if err := db.First(&proj, "id = ?", projectID).Error; err != nil {
		return nil, err
	}

	var channels []model.Channel
	if err := db.Where("project_id = ?", projectID).Find(&channels).Error; err != nil {
		return nil, err
	}

	// 全部矩阵行：用于请求行、回退行与 EnabledOS（量级 = 平台数，很小）。
	var rows []model.PlatformMatrix
	if err := db.Where("project_id = ?", projectID).Find(&rows).Error; err != nil {
		return nil, err
	}

	var hwRevs []model.HwRev
	if err := db.Where("project_id = ?", projectID).Find(&hwRevs).Error; err != nil {
		return nil, err
	}

	var versions []model.Version
	if err := db.Where("project_id = ?", projectID).Find(&versions).Error; err != nil {
		return nil, err
	}

	// 请求行声明的 fallback_arch 决定第二平台的切片范围。
	fallbackArch := ""
	for i := range rows {
		if rows[i].OS == os && rows[i].Arch == arch {
			fallbackArch = rows[i].FallbackArch
			break
		}
	}

	var lines []model.VersionLine
	lineQuery := db.Model(&model.VersionLine{}).
		Joins("JOIN versions ON versions.id = version_lines.version_id").
		Where("versions.project_id = ? AND version_lines.os = ?", projectID, os)
	if fallbackArch != "" && fallbackArch != arch {
		lineQuery = lineQuery.Where("version_lines.arch IN ?", []string{arch, fallbackArch})
	} else {
		lineQuery = lineQuery.Where("version_lines.arch = ?", arch)
	}
	if err := lineQuery.Find(&lines).Error; err != nil {
		return nil, err
	}

	// 这些线上的 kind=full / store_full 产物。
	lineIDs := make([]uuid.UUID, 0, len(lines))
	for i := range lines {
		lineIDs = append(lineIDs, lines[i].ID)
	}
	var artifacts []model.Artifact
	if len(lineIDs) > 0 {
		if err := db.Where("kind IN ? AND version_line_id IN ?",
			[]string{model.ArtifactKindFull, model.ArtifactKindStoreFull}, lineIDs).
			Order("created_at ASC").
			Find(&artifacts).Error; err != nil {
			return nil, err
		}
	}

	// 灰度白名单（版本级）。
	versionAllow, err := r.projects.ListVersionAllowlist(ctx, projectID)
	if err != nil {
		return nil, err
	}

	return assembleUpdateCatalog(&proj, channels, rows, hwRevs, versions, lines, artifacts, versionAllow, os, arch), nil
}

// assembleUpdateCatalog 把原始行装配为纯数据目录快照；GORM 实现与内存实现共用。
// os/arch 是请求平台（决定 Matrix 与 FallbackMatrix 行的选取）。
// versionAllow / lineAllow 是灰度白名单条目（Version 级 / per-line）。
func assembleUpdateCatalog(
	proj *model.Project,
	channels []model.Channel,
	rows []model.PlatformMatrix,
	hwRevs []model.HwRev,
	versions []model.Version,
	lines []model.VersionLine,
	artifacts []model.Artifact,
	versionAllow []model.GrayAllowlist,
	os, arch string,
) *update.Catalog {
	cat := &update.Catalog{
		Project: update.ProjectInfo{
			ID:                      proj.ID,
			Slug:                    proj.Slug,
			CompareEngine:           proj.CompareEngine,
			MinimumSupportedVersion: proj.MinimumSupportedVersion,
			RequireClientToken:      proj.RequireClientToken,
			DefaultLocale:           proj.DefaultLocale,
			ChangelogScope:          proj.ChangelogScope,
			ChangelogLayout:         proj.ChangelogLayout,
			ChangelogClientOverride: proj.ChangelogClientOverride,
			ChangelogIncludeRevoked: proj.ChangelogIncludeRevoked,
			ChangelogIncludeNotes:   proj.ChangelogIncludeNotes,
			ChangelogDefaultEntries: proj.ChangelogDefaultEntries,
			ChangelogMaxEntries:     proj.ChangelogMaxEntries,
			CacheSMaxageSeconds:     proj.CacheSMaxageSeconds,
			DeviceIDPolicy:          proj.DeviceIDPolicy,
			SigningAlgo:             proj.SigningAlgo,
			SigningPrivateKey:       proj.SigningPrivateKey,
			StorageVisibility:       proj.StorageVisibility,
			SignedURLTTLSeconds:     proj.SignedURLTTLSeconds,
			FileListMaxFiles:        proj.FileListMaxFiles,
		},
		Channels:   make([]update.ChannelInfo, 0, len(channels)),
		HwRevRanks: make(map[string]int, len(hwRevs)),
		EnabledOS:  make([]string, 0, len(rows)),
		Versions:   make([]update.VersionState, 0, len(versions)),
	}
	for _, ch := range channels {
		cat.Channels = append(cat.Channels, update.ChannelInfo{
			Slug:           ch.Slug,
			StabilityRank:  ch.StabilityRank,
			Enabled:        ch.Enabled,
			Unlisted:       ch.Unlisted,
			TokenProtected: ch.TokenProtected(),
			TokenHash:      ch.TokenHash,
		})
	}
	for _, h := range hwRevs {
		cat.HwRevRanks[h.Slug] = h.Rank
	}

	// 矩阵行 → MatrixInfo；找出请求行与回退行。
	var reqRow *update.MatrixInfo
	matrices := make([]update.MatrixInfo, 0, len(rows))
	for i := range rows {
		row := &rows[i]
		cat.EnabledOS = appendUniqueOS(cat.EnabledOS, row.OS)
		matrices = append(matrices, update.MatrixInfo{
			OS:                      row.OS,
			Arch:                    row.Arch,
			PackageType:             row.PackageType,
			FallbackArch:            row.FallbackArch,
			DeltaAlgo:               row.DeltaAlgo,
			HwVariantPolicy:         row.HwVariantPolicy,
			MinimumSupportedVersion: row.MinimumSupportedVersion,
		})
		if row.OS == os && row.Arch == arch {
			reqRow = &matrices[len(matrices)-1]
		}
	}
	cat.Matrix = reqRow
	if reqRow != nil && reqRow.FallbackArch != "" {
		for i := range matrices {
			if matrices[i].OS == reqRow.OS && matrices[i].Arch == reqRow.FallbackArch {
				cat.FallbackMatrix = &matrices[i]
				break
			}
		}
	}

	// 版本状态表。
	states := make(map[uuid.UUID]*update.VersionState, len(versions))
	stateOrder := make([]uuid.UUID, 0, len(versions))
	// 灰度白名单归组（C12-1）：白名单是 per-Version 的——Version 级条目只对
	// 加入时的那个 Version（VersionID）生效，按版本归组，绝不跨版本泄漏；
	// per-line 条目经 line→version 映射挂到所属版本。装载器已按 device_id
	// 升序返回，归组后各切片保持升序（ETag 序列化依赖稳定顺序）。
	versionAllowByVersion := make(map[uuid.UUID][]string, len(versions))
	allowMaxCreated := make(map[uuid.UUID]*time.Time)
	for i := range versionAllow {
		e := &versionAllow[i]
		versionAllowByVersion[e.VersionID] = append(versionAllowByVersion[e.VersionID], e.DeviceID)
		if prev, ok := allowMaxCreated[e.VersionID]; !ok || e.CreatedAt.After(*prev) {
			t := e.CreatedAt
			allowMaxCreated[e.VersionID] = &t
		}
	}
	for i := range versions {
		v := versions[i]
		states[v.ID] = &update.VersionState{
			Version: v,
			Allowlist: update.Allowlist{
				Version:      versionAllowByVersion[v.ID],
				MaxCreatedAt: allowMaxCreated[v.ID],
			},
		}
		stateOrder = append(stateOrder, v.ID)
	}

	// 产物按线归组（full vs store_full）。
	artsByLine := make(map[uuid.UUID][]update.ArtifactInfo, len(artifacts))
	feedByLine := make(map[uuid.UUID][]update.ArtifactInfo, len(artifacts))
	for i := range artifacts {
		a := &artifacts[i]
		info := update.ArtifactInfo{
			FileName:          a.FileName,
			Size:              a.Size,
			SHA256:            a.SHA256,
			MD5:               a.MD5,
			SHA512:            a.SHA512,
			ArtifactSignature: a.ArtifactSignature,
			ContentType:       a.ContentType,
			StorageKey:        a.StorageKey,
			HwRev:             a.HwRev,
			MinHwRev:          a.MinHwRev,
			MaxHwRev:          a.MaxHwRev,
			CompatibleHwRevs:  a.CompatibleHwRevs,
		}
		switch a.Kind {
		case model.ArtifactKindStoreFull:
			feedByLine[a.VersionLineID] = append(feedByLine[a.VersionLineID], info)
		default:
			artsByLine[a.VersionLineID] = append(artsByLine[a.VersionLineID], info)
		}
	}

	// 线挂到版本。
	for i := range lines {
		l := &lines[i]
		st := states[l.VersionID]
		if st == nil {
			continue
		}
		ls := update.LineState{
			ID:            l.ID,
			OS:            l.OS,
			Arch:          l.Arch,
			Status:        l.Status,
			RootHash:      l.RootHash,
			MinOS:         l.MinOS,
			MinAPILevel:   l.MinAPILevel,
			PlatformNotes: l.PlatformNotes,
			PacksReadyAt:  l.PacksReadyAt,
			FullPkgs:      artsByLine[l.ID],
			StoreFullPkgs:  feedByLine[l.ID],
		}
		if ls.FullPkgs == nil {
			ls.FullPkgs = []update.ArtifactInfo{}
		}
		if ls.StoreFullPkgs == nil {
			ls.StoreFullPkgs = []update.ArtifactInfo{}
		}
		st.Lines = append(st.Lines, ls)
	}

	// 按原查询顺序输出，保证确定性。
	for _, id := range stateOrder {
		cat.Versions = append(cat.Versions, *states[id])
	}
	return cat
}

// appendUniqueOS 追加不重复的 os（保持首次出现顺序）。
func appendUniqueOS(list []string, os string) []string {
	for _, v := range list {
		if v == os {
			return list
		}
	}
	return append(list, os)
}

// UpdateLineDetailRepo 是 update.LineDetailSource 的 PostgreSQL（GORM）实现。
//
// integrity / diff 端点按需读取单条 Version Line 的 Manifest 条目与
// delta/file 产物；单 Line 两次有界查询，不进入 Catalog 快照（Manifest
// 量级数千行，快照内加载会拖累 check 路径）。
type UpdateLineDetailRepo struct {
	db *gorm.DB
}

// NewUpdateLineDetailRepo 构造 Line 明细读取仓储。
func NewUpdateLineDetailRepo(db *gorm.DB) *UpdateLineDetailRepo {
	return &UpdateLineDetailRepo{db: db}
}

// LineDetails 实现 update.LineDetailSource：返回该线的 Manifest 条目
// （按 Path 升序）与 kind ∈ {delta, patch, file} 的产物快照；未知 lineID 返回空明细。
//
// delta 产物映射（binary_delta，§7.2）与 patch 产物映射（patch_package，§7.5）
// 均以 SHA-256 身份匹配（C10-6 / C13-4）：Algo 与源/目标全量包 SHA-256 来自
// artifacts 列（DeltaAlgo / DeltaSourceSHA256 / DeltaTargetSHA256；kind=patch
// 的 DeltaAlgo 恒为空）。旧行缺失元数据时字段留空 →
// 匹配函数按「未声明不参与匹配」处理，行为正确。
func (r *UpdateLineDetailRepo) LineDetails(ctx context.Context, lineID uuid.UUID) (*update.LineDetail, error) {
	db := r.db.WithContext(ctx)

	var entries []model.ManifestEntry
	if err := db.Where("version_line_id = ?", lineID).Order("path ASC").Find(&entries).Error; err != nil {
		return nil, err
	}

	var arts []model.Artifact
	if err := db.Where("version_line_id = ? AND kind IN ?", lineID,
		[]string{model.ArtifactKindDelta, model.ArtifactKindPatch, model.ArtifactKindFile}).
		Order("created_at ASC").Find(&arts).Error; err != nil {
		return nil, err
	}

	detail := &update.LineDetail{Manifest: entries}
	for i := range arts {
		// 按 Kind 分流映射：delta → Deltas，patch → Patches，file → Files；
		// 误入对方集合的字段虽留空（匹配函数会静默不命中），此处从源头保证集合纯净。
		switch arts[i].Kind {
		case model.ArtifactKindDelta:
			detail.Deltas = append(detail.Deltas, deltaArtifactFromModel(&arts[i]))
		case model.ArtifactKindPatch:
			detail.Patches = append(detail.Patches, patchArtifactFromModel(&arts[i]))
		case model.ArtifactKindFile:
			detail.Files = append(detail.Files, fileArtifactFromModel(&arts[i]))
		}
	}
	return detail, nil
}

// deltaArtifactFromModel 把 kind=delta 产物行映射为 service 快照（binary_delta，§7.2）。
// Algo 与源/目标 SHA-256 由生成 Job 写入的 artifacts 列承载；未承载时留空 → 永不命中。
func deltaArtifactFromModel(a *model.Artifact) update.DeltaArtifactInfo {
	return update.DeltaArtifactInfo{
		Kind:              update.DeltaKindBinaryDelta,
		FileName:          a.FileName,
		Size:              a.Size,
		SHA256:            a.SHA256,
		Algo:              a.DeltaAlgo,
		SourceSHA256:      a.DeltaSourceSHA256,
		TargetSHA256:      a.DeltaTargetSHA256,
		HwRev:             a.HwRev,
		ArtifactSignature: a.ArtifactSignature,
		StorageKey:        a.StorageKey,
	}
}

// patchArtifactFromModel 把 kind=patch 产物行映射为 service 快照（patch_package，§7.5）。
// 源/目标 SHA-256 为双方线全量归档哈希（§5.9，生成 Job 写入的 artifacts 列）；
// DeltaAlgo 恒空（多文件增量归档无二进制差量算法语义）。
func patchArtifactFromModel(a *model.Artifact) update.DeltaArtifactInfo {
	return update.DeltaArtifactInfo{
		Kind:              update.DeltaKindPatchPackage,
		FileName:          a.FileName,
		Size:              a.Size,
		SHA256:            a.SHA256,
		SourceSHA256:      a.DeltaSourceSHA256,
		TargetSHA256:      a.DeltaTargetSHA256,
		HwRev:             a.HwRev,
		ArtifactSignature: a.ArtifactSignature,
		StorageKey:        a.StorageKey,
	}
}

// fileArtifactFromModel 把 kind=file 产物行映射为 service 快照。
// Path 留空（模型未承载 Manifest 路径映射）→ file_list 例外不会命中，行为正确。
func fileArtifactFromModel(a *model.Artifact) update.FileArtifactInfo {
	return update.FileArtifactInfo{
		FileName:   a.FileName,
		StorageKey: a.StorageKey,
		Size:       a.Size,
		SHA256:     a.SHA256,
		MD5:        a.MD5,
		HwRev:      a.HwRev,
	}
}

// 确认 MemoryProjectStore 实现 update.LineDetailSource（实现在 memory_project.go）。
var _ update.LineDetailSource = (*MemoryProjectStore)(nil)
var _ update.LineDetailSource = (*UpdateLineDetailRepo)(nil)
