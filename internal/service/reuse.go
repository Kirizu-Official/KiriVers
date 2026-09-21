package service

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/Kirizu-Official/KiriVers/internal/model"
	"github.com/Kirizu-Official/KiriVers/internal/platform"
	"github.com/Kirizu-Official/KiriVers/pkg/pathutil"
)

// 本文件实现产物复用（C14-1，docs/app-init.md §5.8 / §4.3）：
//
//   - 端点语义：目标 Version（Draft 线或 Published Version 的新增线）的目标
//     (os,arch[,hw]) 线上新建 Artifact 行，StorageKey/Size/SHA256/MD5/SHA512/
//     ContentType/ArtifactSignature 全部引用源对象——零字节拷贝，仅增引用行；
//   - 单文件：一行新引用，FileName 按新版本重算稳定文件名（§5.9；同一存储对象
//     多文件名共存，下载 handler 按行解析）；
//   - 多文件：克隆源线 Manifest 条目（同文件对象 StorageKey）+ 新全量 zip 引用行
//     （同预生成 zip 对象）；RootHash 重算——内容一致则哈希相同；
//   - 校验：源版本存在且未 Revoked（吊销对象禁复用，§5.1）；目标形态与源一致
//     （single/multi），否则 409 PACKAGE_TYPE_IMMUTABLE；sha256 引用项目内
//     唯一命中，多个命中 400 要求 artifact_id；已发布线的既有产物不可变
//     （ARTIFACT_IMMUTABLE 既有语义）。

// ReuseSource 描述复用源的定位方式：artifact_id 或 sha256 二选一。
type ReuseSource struct {
	// ArtifactID 源产物 UUID（优先；与 SHA256 同时提供时以本字段为准）。
	ArtifactID *uuid.UUID `json:"artifact_id,omitempty"`
	// SHA256 源产物 SHA-256（项目内必须唯一命中 kind=full 产物）。
	SHA256 string `json:"sha256,omitempty"`
}

// ReuseArtifactsInput 是产物复用请求体。
type ReuseArtifactsInput struct {
	OS     string      `json:"os"`
	Arch   string      `json:"arch"`
	HwRev  *string     `json:"hw_rev,omitempty"`
	Source ReuseSource `json:"source"`
}

// ReuseArtifactsResult 是产物复用的返回：新建的引用行（单文件 1 行；多文件为
// 全量归档 1 行）与可选的 Manifest 结果（多文件克隆时返回）。
type ReuseArtifactsResult struct {
	Artifacts []model.Artifact `json:"artifacts"`
	Manifest  *ManifestResult  `json:"manifest,omitempty"`
}

// ReuseArtifacts 把源产物的存储对象引用到目标 Version 的目标平台切片上（C14-1）。
// 不做任何 storage.Put：存储对象数不变，仅新增 Artifact 行（必要时新增 Version Line）。
func (s *ProjectService) ReuseArtifacts(ctx context.Context, projectRef, versionRef, osSlug, archSlug string, in ReuseArtifactsInput) (*ReuseArtifactsResult, error) {
	p, err := s.Resolve(ctx, projectRef)
	if err != nil {
		return nil, err
	}
	v, err := s.ResolveVersion(ctx, p.ID, versionRef)
	if err != nil {
		return nil, err
	}

	canonicalOS := platform.CanonicalOSWrite(osSlug)
	canonicalArch := platform.CanonicalArch(archSlug)

	// 目标线：存在则复用；不存在则自动创建（与上传路径语义一致）。
	line, err := s.store.GetVersionLine(ctx, v.ID, canonicalOS, canonicalArch)
	if err != nil {
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, err
		}
		line = &model.VersionLine{
			ProjectID: p.ID,
			VersionID: v.ID,
			OS:        canonicalOS,
			Arch:      canonicalArch,
			Status:    model.VersionLineStatusPending,
		}
		if err := s.store.CreateVersionLine(ctx, line); err != nil {
			return nil, err
		}
		s.invalidateProject(ctx, p.ID)
	}

	// 已发布线不可变（ARTIFACT_IMMUTABLE 既有语义）：Published Version 的已存在
	// 线若已有全量产物则禁止再复用；新线（或尚无产物的线）允许。
	if v.Status == model.VersionStatusPublished {
		if existing, _ := s.store.GetArtifactByLineID(ctx, line.ID); existing != nil {
			return nil, ErrArtifactImmutable
		}
	}

	// 硬件代号存在性校验（与上传一致，C05-11）。
	if err := s.validateHwRevs(ctx, p.ID, in.HwRev, nil, nil, nil); err != nil {
		return nil, err
	}

	// 定位源产物（§5.1：吊销版本的对象禁止复用）。
	src, err := s.resolveReuseSource(ctx, p.ID, in.Source)
	if err != nil {
		return nil, err
	}
	srcVersion, err := s.store.GetVersionByID(ctx, src.VersionID)
	if err != nil {
		return nil, err
	}
	if srcVersion.Status == model.VersionStatusRevoked {
		return nil, fmt.Errorf("%w: source version %s is revoked", ErrReuseSourceRevoked, srcVersion.ID)
	}

	// 形态一致性：源线与目标线的 package_type 必须同形态（single/multi），
	// 否则 409 PACKAGE_TYPE_IMMUTABLE（形态属于 Version Line，§6）。
	srcLine, err := s.findLineByID(ctx, src.VersionID, src.VersionLineID)
	if err != nil {
		return nil, err
	}
	srcMulti := isMultiFileLine(ctx, s, p.ID, srcLine.OS, srcLine.Arch)
	targetMulti := isMultiFileLine(ctx, s, p.ID, canonicalOS, canonicalArch)
	if srcMulti != targetMulti {
		return nil, fmt.Errorf("%w: source line %s/%s package_type does not match target %s/%s",
			ErrPackageTypeImmutable, srcLine.OS, srcLine.Arch, canonicalOS, canonicalArch)
	}

	if srcMulti {
		return s.reuseMultiFileArtifacts(ctx, p, v, line, src)
	}
	return s.reuseSingleFileArtifact(ctx, p, v, line, in.HwRev, src)
}

// resolveReuseSource 按 artifact_id（优先）或 sha256 定位源产物。
// sha256 路径要求项目内 kind=full 唯一命中，多个命中返回 ErrReuseAmbiguousSHA256。
func (s *ProjectService) resolveReuseSource(ctx context.Context, projectID uuid.UUID, src ReuseSource) (*model.Artifact, error) {
	if src.ArtifactID != nil && *src.ArtifactID != uuid.Nil {
		art, err := s.store.GetArtifactByID(ctx, *src.ArtifactID)
		if err != nil || art == nil || art.ProjectID != projectID {
			return nil, ErrArtifactNotFound
		}
		return art, nil
	}
	sha := strings.ToLower(strings.TrimSpace(src.SHA256))
	if sha == "" {
		return nil, ErrReuseSourceRequired
	}
	matches, err := s.store.ListArtifactsBySHA256(ctx, projectID, sha)
	if err != nil {
		return nil, err
	}
	switch len(matches) {
	case 0:
		return nil, ErrArtifactNotFound
	case 1:
		return &matches[0], nil
	default:
		return nil, fmt.Errorf("%w: %d artifacts share sha256 %s", ErrReuseAmbiguousSHA256, len(matches), sha)
	}
}

// reuseSingleFileArtifact 在目标线上新建一条引用源对象的 kind=full Artifact 行。
// StorageKey 与哈希/签名载荷原样引用；FileName 按新版本与 hw 变体重算（§5.9）。
func (s *ProjectService) reuseSingleFileArtifact(ctx context.Context, p *model.Project, v *model.Version, line *model.VersionLine, hwRev *string, src *model.Artifact) (*ReuseArtifactsResult, error) {
	ext := filepath.Ext(src.FileName)
	stableFilename := BuildStableFilename(p.Slug, v, line.OS, line.Arch, hwRev, ext)

	art := &model.Artifact{
		ProjectID:         p.ID,
		VersionID:         v.ID,
		VersionLineID:     line.ID,
		Kind:              model.ArtifactKindFull,
		FileName:          stableFilename,
		StorageKey:        src.StorageKey, // 零拷贝：引用同一存储对象
		Size:              src.Size,
		SHA256:            src.SHA256,
		MD5:               src.MD5,
		SHA512:            src.SHA512,
		ContentType:       src.ContentType,
		Compression:       src.Compression,
		ArtifactSignature: src.ArtifactSignature,
		HwRev:             hwRev,
		MinHwRev:          src.MinHwRev,
		MaxHwRev:          src.MaxHwRev,
		CompatibleHwRevs:  src.CompatibleHwRevs,
	}
	if err := s.store.CreateArtifact(ctx, art); err != nil {
		return nil, err
	}
	if err := s.markLineReadyAfterReuse(ctx, p, v, line); err != nil {
		return nil, err
	}
	return &ReuseArtifactsResult{Artifacts: []model.Artifact{*art}}, nil
}

// reuseMultiFileArtifacts 克隆源线 Manifest 条目到目标线并新建一条引用源
// 预生成全量 zip 对象的 kind=full Artifact 行；RootHash 重算（内容一致则相同）。
func (s *ProjectService) reuseMultiFileArtifacts(ctx context.Context, p *model.Project, v *model.Version, line *model.VersionLine, src *model.Artifact) (*ReuseArtifactsResult, error) {
	entries, err := s.store.ListManifestEntries(ctx, src.VersionLineID)
	if err != nil {
		return nil, err
	}
	if len(entries) == 0 {
		return nil, ErrManifestRequired
	}

	// Manifest 条目批量克隆：同文件对象（源条目本身只登记哈希，不持对象），
	// 新 ID 挂到目标线。
	cloned := make([]model.ManifestEntry, 0, len(entries))
	for _, e := range entries {
		cloned = append(cloned, model.ManifestEntry{
			ID:             uuid.New(),
			ProjectID:      p.ID,
			VersionID:      v.ID,
			VersionLineID:  line.ID,
			Path:           e.Path,
			Size:           e.Size,
			SHA256:         e.SHA256,
			MD5:            e.MD5,
			InstallPolicy:  e.InstallPolicy,
			IntegrityCheck: e.IntegrityCheck,
		})
	}
	rootHash, err := pathutil.CalculateRootHash(cloned)
	if err != nil {
		return nil, fmt.Errorf("calculate root hash: %w", err)
	}
	if err := s.store.SaveManifestEntries(ctx, line.ID, cloned); err != nil {
		return nil, err
	}

	// 全量 zip 引用行：同 StorageKey，不复制字节。
	stableFilename := BuildStableFilename(p.Slug, v, line.OS, line.Arch, nil, ".zip")
	art := &model.Artifact{
		ProjectID:         p.ID,
		VersionID:         v.ID,
		VersionLineID:     line.ID,
		Kind:              model.ArtifactKindFull,
		FileName:          stableFilename,
		StorageKey:        src.StorageKey, // 零拷贝：引用同一预生成 zip 对象
		Size:              src.Size,
		SHA256:            src.SHA256,
		MD5:               src.MD5,
		SHA512:            src.SHA512,
		ContentType:       src.ContentType,
		Compression:       src.Compression,
		ArtifactSignature: src.ArtifactSignature,
	}
	if err := s.store.CreateArtifact(ctx, art); err != nil {
		return nil, err
	}
	clonedArts := []model.Artifact{*art}
	feeds, err := s.store.ListArtifactsByLineAndKind(ctx, src.VersionLineID, model.ArtifactKindStoreFull)
	if err != nil {
		return nil, err
	}
	for i := range feeds {
		f := feeds[i]
		feedName := BuildStableFilename(p.Slug, v, line.OS, line.Arch, f.HwRev, "") + "-store.zip"
		feedArt := &model.Artifact{
			ProjectID:         p.ID,
			VersionID:         v.ID,
			VersionLineID:     line.ID,
			Kind:              model.ArtifactKindStoreFull,
			FileName:          feedName,
			StorageKey:        f.StorageKey,
			Size:              f.Size,
			SHA256:            f.SHA256,
			MD5:               f.MD5,
			SHA512:            f.SHA512,
			ContentType:       f.ContentType,
			Compression:       f.Compression,
			ArtifactSignature: f.ArtifactSignature,
			HwRev:             f.HwRev,
		}
		if err := s.store.CreateArtifact(ctx, feedArt); err != nil {
			return nil, err
		}
		clonedArts = append(clonedArts, *feedArt)
	}
	// 目标线携带重算的 RootHash（内容一致则与源线相同）。
	line.RootHash = rootHash
	if err := s.markLineReadyAfterReuse(ctx, p, v, line); err != nil {
		return nil, err
	}

	return &ReuseArtifactsResult{
		Artifacts: clonedArts,
		Manifest: &ManifestResult{
			RootHash: rootHash,
			Count:    len(cloned),
			Entries:  cloned,
		},
	}, nil
}

// markLineReadyAfterReuse 复用落库后把目标线置为 ready（Manifest 与全量对象均已满足），
// 并接续齐套自动发布 / 自动差量 / webhook 触发点。
func (s *ProjectService) markLineReadyAfterReuse(ctx context.Context, p *model.Project, v *model.Version, line *model.VersionLine) error {
	line.Status = model.VersionLineStatusReady
	if err := s.store.SaveVersionLine(ctx, line); err != nil {
		return err
	}
	_, _ = s.CheckAndTriggerAutoPublish(ctx, p.ID, v.ID)
	// 复用产生的线就绪同样触发自动差量（C13-1 补平台场景）与 webhook（C14-3）。
	s.notifyLineReady(ctx, p.ID, v.ID)
	s.notifyLineReadyWebhook(ctx, p.ID, v, line)
	s.invalidateProject(ctx, p.ID)
	return nil
}

// findLineByID 在版本的线列表中按 ID 定位源线。
func (s *ProjectService) findLineByID(ctx context.Context, versionID, lineID uuid.UUID) (*model.VersionLine, error) {
	lines, err := s.store.ListVersionLines(ctx, versionID)
	if err != nil {
		return nil, err
	}
	for i := range lines {
		if lines[i].ID == lineID {
			return &lines[i], nil
		}
	}
	return nil, ErrVersionLineNotFound
}

// isMultiFileLine 判定指定 (os,arch) 平台矩阵是否为多文件形态；无矩阵行视为单文件。
func isMultiFileLine(ctx context.Context, s *ProjectService, projectID uuid.UUID, os, arch string) bool {
	row, _ := s.store.GetMatrix(ctx, projectID, os, arch)
	return row != nil && row.PackageType == model.PackageTypeMultiFile
}
