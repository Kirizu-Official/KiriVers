package service

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/Kirizu-Official/KiriVers/internal/model"
	"github.com/Kirizu-Official/KiriVers/internal/platform"
	"github.com/Kirizu-Official/KiriVers/internal/storage"
	"github.com/Kirizu-Official/KiriVers/pkg/hashutil"
	"github.com/Kirizu-Official/KiriVers/pkg/pathutil"
)

// UploadArtifactInput 包含直接上传产物的输入参数。
type UploadArtifactInput struct {
	Filename          string
	ExpectedSHA256    string
	IdempotencyKey    string
	HwRev             *string
	MinHwRev          *string
	MaxHwRev          *string
	CompatibleHwRevs  []string
	ArtifactSignature string
	Size              int64
	ContentType       string
}

// CreateTusUploadInput 包含初始化 TUS 1.0 上传会话的参数。
type CreateTusUploadInput struct {
	Size              int64
	Filename          string
	ExpectedSHA256    string
	IdempotencyKey    string
	HwRev             *string
	MinHwRev          *string
	MaxHwRev          *string
	CompatibleHwRevs  []string
	ArtifactSignature string
	Metadata          map[string]any
}

// ArtifactDownload 承载客户端下载产物时的元数据。
type ArtifactDownload struct {
	Artifact    *model.Artifact
	StorageKey  string
	FileName    string
	ContentType string
	Size        int64
}

// DetectContentType 根据扩展名映射对应的 MIME 类型。未知扩展名回退至 application/octet-stream。
func DetectContentType(filename string) string {
	ext := strings.ToLower(filepath.Ext(filename))
	switch ext {
	case ".apk":
		return "application/vnd.android.package-archive"
	case ".dmg":
		return "application/x-apple-diskimage"
	case ".zip":
		return "application/zip"
	case ".exe":
		return "application/vnd.microsoft.portable-executable"
	case ".msi":
		return "application/x-msi"
	case ".appimage":
		return "application/vnd.appimage"
	case ".tar.gz", ".tgz":
		return "application/gzip"
	case ".bin", ".hex":
		return "application/octet-stream"
	default:
		return "application/octet-stream"
	}
}

// DefaultExtensionForPackageType 根据平台矩阵的 package_type 给出默认文件扩展名。
func DefaultExtensionForPackageType(pkgType string) string {
	switch strings.ToLower(strings.TrimSpace(pkgType)) {
	case "apk":
		return ".apk"
	case "dmg":
		return ".dmg"
	case "zip":
		return ".zip"
	case "exe":
		return ".exe"
	case "msi":
		return ".msi"
	case "appimage":
		return ".AppImage"
	case "tar.gz":
		return ".tar.gz"
	case "bin":
		return ".bin"
	default:
		return ""
	}
}

// BuildStableFilename 按照 §5.9 规则生成对外暴露的稳定全量文件名：
// {project_slug}-{version_semver|version_integer}-{os}-{arch}[-{hw_rev}][.{ext}]
func BuildStableFilename(projectSlug string, v *model.Version, os, arch string, hwRev *string, ext string) string {
	var verStr string
	if v.VersionSemverCanonical != nil && *v.VersionSemverCanonical != "" {
		verStr = *v.VersionSemverCanonical
	} else if v.VersionInteger != nil {
		verStr = fmt.Sprintf("%d", *v.VersionInteger)
	} else {
		verStr = "0"
	}

	ext = strings.TrimSpace(ext)
	if ext != "" && !strings.HasPrefix(ext, ".") {
		ext = "." + ext
	}

	hwPart := ""
	if hwRev != nil && strings.TrimSpace(*hwRev) != "" {
		hwPart = "-" + strings.TrimSpace(*hwRev)
	}

	return fmt.Sprintf("%s-%s-%s-%s%s%s", projectSlug, verStr, os, arch, hwPart, ext)
}

// UploadArtifact 处理单文件一次性上传 (C05-1, C05-4, C05-5, C05-7, C05-11)。
func (s *ProjectService) UploadArtifact(ctx context.Context, projectRef, versionRef, osSlug, archSlug string, in UploadArtifactInput, body io.Reader) (*model.Artifact, error) {
	if s.storage == nil {
		return nil, ErrStorageUnavailable
	}
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
	line, err := s.store.GetVersionLine(ctx, v.ID, canonicalOS, canonicalArch)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			// 若 line 不存在，尝试自动创建
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
		} else {
			return nil, err
		}
	}

	// 校验硬件代号存在性 (C05-11)
	if err := s.validateHwRevs(ctx, p.ID, in.HwRev, in.MinHwRev, in.MaxHwRev, in.CompatibleHwRevs); err != nil {
		return nil, err
	}

	// 幂等键检查 (24h 窗口)
	if in.IdempotencyKey != "" {
		existingSession, err := s.store.GetUploadSessionByIdempotencyKey(ctx, p.ID, in.IdempotencyKey)
		if err == nil && existingSession != nil && existingSession.Status == model.UploadSessionStatusCompleted {
			if existingArt, err := s.store.GetArtifactByLineID(ctx, line.ID); err == nil && existingArt != nil {
				return existingArt, nil
			}
		}
	}

	// 已发布版本不可变检查 (C05-7)
	existingArtifact, _ := s.store.GetArtifactByLineID(ctx, line.ID)
	if v.Status == model.VersionStatusPublished && existingArtifact != nil {
		if in.ExpectedSHA256 != "" && strings.EqualFold(in.ExpectedSHA256, existingArtifact.SHA256) {
			return existingArtifact, nil // 相同 SHA-256 no-op 成功
		}
		if in.ExpectedSHA256 != "" && !strings.EqualFold(in.ExpectedSHA256, existingArtifact.SHA256) {
			return nil, ErrArtifactImmutable
		}
	}

	// 流式写入本机 {root}/{slug}/temp/{uuid} 并同时计算哈希（不得写入公共桶）。
	tempAbs, _, hasher, written, err := s.writeLocalTemp(p.Slug, body)
	if err != nil {
		return nil, fmt.Errorf("write local temp: %w", err)
	}
	if in.Size > 0 && written != in.Size {
		// 允许未知 size（0）；若声明了则不强制失败，以实算为准。
	}

	computedSHA256 := hasher.SHA256()

	// 校验 X-Content-SHA256
	if in.ExpectedSHA256 != "" && !strings.EqualFold(in.ExpectedSHA256, computedSHA256) {
		removeLocal(tempAbs)
		return nil, ErrChecksumMismatch
	}

	// 若在已发布状态且已有产物，比对实算哈希
	if v.Status == model.VersionStatusPublished && existingArtifact != nil {
		removeLocal(tempAbs)
		if strings.EqualFold(existingArtifact.SHA256, computedSHA256) {
			return existingArtifact, nil
		}
		return nil, ErrArtifactImmutable
	}

	// 计算扩展名与稳定文件名
	ext := filepath.Ext(in.Filename)
	if ext == "" {
		if matrixRow, _ := s.store.GetMatrix(ctx, p.ID, canonicalOS, canonicalArch); matrixRow != nil {
			ext = DefaultExtensionForPackageType(matrixRow.PackageType)
		}
	}
	stableFilename := BuildStableFilename(p.Slug, v, canonicalOS, canonicalArch, in.HwRev, ext)
	artifactID := uuid.New()
	contentType := in.ContentType
	if contentType == "" {
		contentType = DetectContentType(stableFilename)
	}

	permanentKey, err := s.promoteLocalFile(ctx, p.Slug, computedSHA256, contentType, tempAbs, hasher.Size())
	if err != nil {
		removeLocal(tempAbs)
		return nil, fmt.Errorf("commit permanent object: %w", err)
	}

	// 创建实体并持久化
	art := &model.Artifact{
		ID:                artifactID,
		ProjectID:         p.ID,
		VersionID:         v.ID,
		VersionLineID:     line.ID,
		Kind:              model.ArtifactKindFull,
		FileName:          stableFilename,
		StorageKey:        permanentKey,
		Size:              hasher.Size(),
		SHA256:            computedSHA256,
		MD5:               hasher.MD5(),
		SHA512:            hasher.SHA512(),
		ContentType:       contentType,
		ArtifactSignature: in.ArtifactSignature,
		HwRev:             in.HwRev,
		MinHwRev:          in.MinHwRev,
		MaxHwRev:          in.MaxHwRev,
		CompatibleHwRevs:  in.CompatibleHwRevs,
	}

	if err := s.store.CreateArtifact(ctx, art); err != nil {
		_ = s.storage.Delete(ctx, permanentKey)
		return nil, err
	}

	if err := s.onArtifactUploaded(ctx, p, v, line, art); err != nil {
		_ = s.storage.Delete(ctx, permanentKey)
		_ = s.store.DeleteArtifact(ctx, art.ID)
		return nil, err
	}

	// 若提供了幂等键，记录完成会话供后续命中
	if in.IdempotencyKey != "" {
		sess := &model.UploadSession{
			ProjectID:      p.ID,
			VersionID:      v.ID,
			VersionLineID:  line.ID,
			StorageKey:     permanentKey,
			Offset:         hasher.Size(),
			Size:           hasher.Size(),
			Status:         model.UploadSessionStatusCompleted,
			IdempotencyKey: &in.IdempotencyKey,
			ExpiresAt:      time.Now().Add(24 * time.Hour),
		}
		_ = s.store.CreateUploadSession(ctx, sess)
	}

	return art, nil
}

// CreateTusUpload 初始化 TUS 1.0 上传会话 (C05-2, C05-8)。
func (s *ProjectService) CreateTusUpload(ctx context.Context, projectRef, versionRef, osSlug, archSlug string, in CreateTusUploadInput) (*model.UploadSession, error) {
	if s.storage == nil {
		return nil, ErrStorageUnavailable
	}
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
	line, err := s.store.GetVersionLine(ctx, v.ID, canonicalOS, canonicalArch)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
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
		} else {
			return nil, err
		}
	}

	if err := s.validateHwRevs(ctx, p.ID, in.HwRev, in.MinHwRev, in.MaxHwRev, in.CompatibleHwRevs); err != nil {
		return nil, err
	}

	// 幂等重试检查
	if in.IdempotencyKey != "" {
		existing, err := s.store.GetUploadSessionByIdempotencyKey(ctx, p.ID, in.IdempotencyKey)
		if err == nil && existing != nil {
			return existing, nil
		}
	}

	// 已发布版本不可变检查
	existingArtifact, _ := s.store.GetArtifactByLineID(ctx, line.ID)
	if v.Status == model.VersionStatusPublished && existingArtifact != nil {
		if in.ExpectedSHA256 != "" && !strings.EqualFold(in.ExpectedSHA256, existingArtifact.SHA256) {
			return nil, ErrArtifactImmutable
		}
	}

	sessionID := uuid.New()
	metadataObj := model.JSONObject{}
	for k, val := range in.Metadata {
		metadataObj[k] = val
	}
	if in.Filename != "" {
		metadataObj["filename"] = in.Filename
	}
	if in.ExpectedSHA256 != "" {
		metadataObj["sha256"] = in.ExpectedSHA256
	}
	if in.HwRev != nil {
		metadataObj["hw_rev"] = *in.HwRev
	}
	if in.MinHwRev != nil {
		metadataObj["min_hw_rev"] = *in.MinHwRev
	}
	if in.MaxHwRev != nil {
		metadataObj["max_hw_rev"] = *in.MaxHwRev
	}
	if len(in.CompatibleHwRevs) > 0 {
		metadataObj["compatible_hw_revs"] = in.CompatibleHwRevs
	}
	if in.ArtifactSignature != "" {
		metadataObj["artifact_signature"] = in.ArtifactSignature
	}

	var idemKeyPtr *string
	if in.IdempotencyKey != "" {
		idemKeyPtr = &in.IdempotencyKey
	}

	sess := &model.UploadSession{
		ID:             sessionID,
		ProjectID:      p.ID,
		VersionID:      v.ID,
		VersionLineID:  line.ID,
		StorageKey:     storage.ArtifactTempRel(p.Slug, sessionID),
		Offset:         0,
		Size:           in.Size,
		Status:         model.UploadSessionStatusUploading,
		IdempotencyKey: idemKeyPtr,
		Metadata:       metadataObj,
		ExpiresAt:      time.Now().Add(24 * time.Hour),
	}

	if err := s.ensureTusTempFile(sess.StorageKey); err != nil {
		return nil, err
	}

	if err := s.store.CreateUploadSession(ctx, sess); err != nil {
		return nil, err
	}
	return sess, nil
}

// GetTusUpload 查询当前 TUS 上传会话详情与偏移量。
func (s *ProjectService) GetTusUpload(ctx context.Context, projectRef string, uploadID uuid.UUID) (*model.UploadSession, error) {
	p, err := s.Resolve(ctx, projectRef)
	if err != nil {
		return nil, err
	}
	sess, err := s.store.GetUploadSessionByID(ctx, uploadID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrUploadSessionNotFound
		}
		return nil, err
	}
	if sess.ProjectID != p.ID {
		return nil, ErrUploadSessionNotFound
	}
	if abs := s.tusTempAbs(sess.StorageKey); abs != "" {
		if fi, err := os.Stat(abs); err == nil {
			sess.Offset = fi.Size()
		}
	}
	return sess, nil
}

// WriteTusChunk 追加写入 TUS 上传分片。若上传完成，落地正式产物并返回。
func (s *ProjectService) WriteTusChunk(ctx context.Context, projectRef string, uploadID uuid.UUID, offset int64, chunk io.Reader) (*model.UploadSession, *model.Artifact, error) {
	sess, err := s.GetTusUpload(ctx, projectRef, uploadID)
	if err != nil {
		return nil, nil, err
	}
	if sess.Status != model.UploadSessionStatusUploading {
		return nil, nil, fmt.Errorf("%w: session status is %s", ErrInvalidVersionTransition, sess.Status)
	}
	if sess.Offset != offset {
		return nil, nil, ErrOffsetMismatch
	}

	tempAbs := s.tusTempAbs(sess.StorageKey)
	if err := os.MkdirAll(filepath.Dir(tempAbs), 0o755); err != nil {
		return nil, nil, err
	}
	f, err := os.OpenFile(tempAbs, os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, nil, fmt.Errorf("open local temp: %w", err)
	}
	if _, err := f.Seek(offset, io.SeekStart); err != nil {
		_ = f.Close()
		return nil, nil, err
	}
	n, err := io.Copy(f, chunk)
	_ = f.Close()
	if err != nil {
		return nil, nil, fmt.Errorf("write chunk: %w", err)
	}

	sess.Offset += n
	if sess.Offset > sess.Size {
		return nil, nil, fmt.Errorf("chunk exceeds upload size")
	}

	// 若未写满，保存并返回
	if sess.Offset < sess.Size {
		if err := s.store.UpdateUploadSession(ctx, sess); err != nil {
			return nil, nil, err
		}
		return sess, nil, nil
	}

	p, err := s.store.GetByID(ctx, sess.ProjectID)
	if err != nil {
		return nil, nil, err
	}
	v, err := s.store.GetVersionByID(ctx, sess.VersionID)
	if err != nil {
		return nil, nil, err
	}
	lines, err := s.store.ListVersionLines(ctx, v.ID)
	if err != nil {
		return nil, nil, err
	}
	var line *model.VersionLine
	for i := range lines {
		if lines[i].ID == sess.VersionLineID {
			line = &lines[i]
			break
		}
	}
	if line == nil {
		return nil, nil, ErrVersionLineNotFound
	}

	hasher := hashutil.NewMultiHasher(true)
	rf, err := os.Open(tempAbs)
	if err != nil {
		return nil, nil, err
	}
	if _, err := io.Copy(hasher, rf); err != nil {
		_ = rf.Close()
		return nil, nil, err
	}
	_ = rf.Close()

	computedSHA256 := hasher.SHA256()

	// 检查元数据中声明的 sha256
	var expectedSHA string
	if val, ok := sess.Metadata["sha256"].(string); ok {
		expectedSHA = val
	}
	if expectedSHA != "" && !strings.EqualFold(expectedSHA, computedSHA256) {
		sess.Status = model.UploadSessionStatusAborted
		_ = s.store.UpdateUploadSession(ctx, sess)
		s.cleanupSessionParts(ctx, sess)
		return nil, nil, ErrChecksumMismatch
	}

	// 检查发布后不可变
	existingArtifact, _ := s.store.GetArtifactByLineID(ctx, line.ID)
	if v.Status == model.VersionStatusPublished && existingArtifact != nil {
		if strings.EqualFold(existingArtifact.SHA256, computedSHA256) {
			sess.Status = model.UploadSessionStatusCompleted
			_ = s.store.UpdateUploadSession(ctx, sess)
			s.cleanupSessionParts(ctx, sess)
			return sess, existingArtifact, nil
		}
		sess.Status = model.UploadSessionStatusAborted
		_ = s.store.UpdateUploadSession(ctx, sess)
		s.cleanupSessionParts(ctx, sess)
		return nil, nil, ErrArtifactImmutable
	}

	// 落地正式对象
	var filenameMeta string
	if val, ok := sess.Metadata["filename"].(string); ok {
		filenameMeta = val
	}
	ext := filepath.Ext(filenameMeta)
	if ext == "" {
		if matrixRow, _ := s.store.GetMatrix(ctx, p.ID, line.OS, line.Arch); matrixRow != nil {
			ext = DefaultExtensionForPackageType(matrixRow.PackageType)
		}
	}

	var hwRevPtr *string
	if val, ok := sess.Metadata["hw_rev"].(string); ok && val != "" {
		hwRevPtr = &val
	}
	var minHwPtr *string
	if val, ok := sess.Metadata["min_hw_rev"].(string); ok && val != "" {
		minHwPtr = &val
	}
	var maxHwPtr *string
	if val, ok := sess.Metadata["max_hw_rev"].(string); ok && val != "" {
		maxHwPtr = &val
	}
	var compHws []string
	if val, ok := sess.Metadata["compatible_hw_revs"].([]any); ok {
		for _, item := range val {
			if sItem, ok := item.(string); ok {
				compHws = append(compHws, sItem)
			}
		}
	}
	var signature string
	if val, ok := sess.Metadata["artifact_signature"].(string); ok {
		signature = val
	}

	stableFilename := BuildStableFilename(p.Slug, v, line.OS, line.Arch, hwRevPtr, ext)
	artifactID := uuid.New()
	contentType := DetectContentType(stableFilename)

	permanentKey, err := s.promoteLocalFile(ctx, p.Slug, computedSHA256, contentType, tempAbs, hasher.Size())
	if err != nil {
		return nil, nil, fmt.Errorf("write final object: %w", err)
	}

	art := &model.Artifact{
		ID:                artifactID,
		ProjectID:         p.ID,
		VersionID:         v.ID,
		VersionLineID:     line.ID,
		Kind:              model.ArtifactKindFull,
		FileName:          stableFilename,
		StorageKey:        permanentKey,
		Size:              hasher.Size(),
		SHA256:            computedSHA256,
		MD5:               hasher.MD5(),
		SHA512:            hasher.SHA512(),
		ContentType:       contentType,
		ArtifactSignature: signature,
		HwRev:             hwRevPtr,
		MinHwRev:          minHwPtr,
		MaxHwRev:          maxHwPtr,
		CompatibleHwRevs:  compHws,
	}

	if err := s.store.CreateArtifact(ctx, art); err != nil {
		_ = s.storage.Delete(ctx, permanentKey)
		return nil, nil, err
	}

	if err := s.onArtifactUploaded(ctx, p, v, line, art); err != nil {
		_ = s.storage.Delete(ctx, permanentKey)
		_ = s.store.DeleteArtifact(ctx, art.ID)
		sess.Status = model.UploadSessionStatusAborted
		_ = s.store.UpdateUploadSession(ctx, sess)
		s.cleanupSessionParts(ctx, sess)
		return nil, nil, err
	}

	sess.Status = model.UploadSessionStatusCompleted
	if err := s.store.UpdateUploadSession(ctx, sess); err != nil {
		return nil, nil, err
	}

	s.cleanupSessionParts(ctx, sess)
	return sess, art, nil
}

// AbortTusUpload 中止上传会话并清理临时存储。
func (s *ProjectService) AbortTusUpload(ctx context.Context, projectRef string, uploadID uuid.UUID) error {
	sess, err := s.GetTusUpload(ctx, projectRef, uploadID)
	if err != nil {
		return err
	}
	sess.Status = model.UploadSessionStatusAborted
	_ = s.store.UpdateUploadSession(ctx, sess)
	s.cleanupSessionParts(ctx, sess)
	return nil
}

func (s *ProjectService) cleanupSessionParts(_ context.Context, sess *model.UploadSession) {
	if sess == nil {
		return
	}
	removeLocal(s.tusTempAbs(sess.StorageKey))
}

// GetArtifactDownload 按 SHA-256 返回产物下载元数据。
//
// 查找只认 :ref 前导 64 hex（大小写不敏感）；可选 `.{ext}` / `.blockmap` 装饰（D7）。
// 不再按 FileName 或 artifact UUID 查找。同哈希多行共用 StorageKey，
// Content-Disposition 取 created_at/id 确定性第一行的 FileName。
// 哈希 GET 不做 HW_REV_INCOMPATIBLE 闸（R4）。
func (s *ProjectService) GetArtifactDownload(ctx context.Context, projectRef, artifactRef, _ string) (*ArtifactDownload, error) {
	p, err := s.Resolve(ctx, projectRef)
	if err != nil {
		return nil, err
	}

	sha, suffix, ok := ParsePackageRef(artifactRef)
	if !ok {
		return nil, ErrArtifactNotFound
	}

	arts, err := s.store.ListArtifactsByContentSHA256(ctx, p.ID, sha)
	if err != nil {
		return nil, err
	}
	if len(arts) == 0 {
		return nil, ErrArtifactNotFound
	}

	art := pickDownloadArtifact(arts)
	if art == nil {
		return nil, ErrArtifactNotFound
	}
	if isBlockmapSuffix(suffix) {
		if !isBlockmapFileName(art.FileName) {
			var blockmaps []model.Artifact
			for i := range arts {
				if isBlockmapFileName(arts[i].FileName) {
					blockmaps = append(blockmaps, arts[i])
				}
			}
			if len(blockmaps) > 0 {
				art = pickDownloadArtifact(blockmaps)
			} else {
				sibling, serr := s.findBlockmapSibling(ctx, art)
				if serr != nil {
					return nil, serr
				}
				if sibling == nil {
					return nil, ErrArtifactNotFound
				}
				art = sibling
			}
		}
	}

	return &ArtifactDownload{
		Artifact:    art,
		StorageKey:  art.StorageKey,
		FileName:    art.FileName,
		ContentType: art.ContentType,
		Size:        art.Size,
	}, nil
}

// pickDownloadArtifact 同哈希多行取 created_at 升序、并列 id 升序的第一行。
func pickDownloadArtifact(arts []model.Artifact) *model.Artifact {
	if len(arts) == 0 {
		return nil
	}
	sort.SliceStable(arts, func(i, j int) bool {
		if arts[i].CreatedAt.Equal(arts[j].CreatedAt) {
			return arts[i].ID.String() < arts[j].ID.String()
		}
		return arts[i].CreatedAt.Before(arts[j].CreatedAt)
	})
	return &arts[0]
}

// findBlockmapSibling 在安装器所在线上查找 FileName == installer.FileName+".blockmap" 的 kind=file。
func (s *ProjectService) findBlockmapSibling(ctx context.Context, installer *model.Artifact) (*model.Artifact, error) {
	if installer == nil {
		return nil, nil
	}
	files, err := s.store.ListArtifactsByLineAndKind(ctx, installer.VersionLineID, model.ArtifactKindFile)
	if err != nil {
		return nil, err
	}
	want := installer.FileName + ".blockmap"
	var matches []model.Artifact
	for i := range files {
		if files[i].FileName == want || strings.EqualFold(files[i].FileName, want) || isBlockmapFileName(files[i].FileName) {
			matches = append(matches, files[i])
		}
	}
	if len(matches) == 0 {
		return nil, nil
	}
	// 优先精确 FileName 匹配，否则取确定性第一行 .blockmap。
	var exact []model.Artifact
	for i := range matches {
		if matches[i].FileName == want || strings.EqualFold(matches[i].FileName, want) {
			exact = append(exact, matches[i])
		}
	}
	if len(exact) > 0 {
		return pickDownloadArtifact(exact), nil
	}
	return pickDownloadArtifact(matches), nil
}

// CleanupArtifacts 清理超过保留期或已中止的未完成上传会话 (C05-10)。
func (s *ProjectService) CleanupArtifacts(ctx context.Context, projectRef string, retentionDays int) (int, error) {
	var projectID uuid.UUID
	if projectRef != "" {
		p, err := s.Resolve(ctx, projectRef)
		if err != nil {
			return 0, err
		}
		projectID = p.ID
	}
	if retentionDays <= 0 {
		retentionDays = 90
	}
	before := time.Now().Add(-time.Duration(retentionDays) * 24 * time.Hour)
	expired, err := s.store.ListExpiredUploadSessions(ctx, projectID, before)
	if err != nil {
		return 0, err
	}

	cleanedCount := 0
	for _, sess := range expired {
		s.cleanupSessionParts(ctx, &sess)
		_ = s.store.DeleteUploadSession(ctx, sess.ID)
		cleanedCount++
	}
	if projectID != uuid.Nil {
		n, err := s.cleanupStalePatches(ctx, projectID, before)
		if err != nil {
			return cleanedCount, err
		}
		cleanedCount += n
	}
	return cleanedCount, nil
}

// cleanupStalePatches 删除不在当前 D3 预热窗口内、且超过留存期的 kind=patch。
// 永不删除 full / store_full / file / delta。
func (s *ProjectService) cleanupStalePatches(ctx context.Context, projectID uuid.UUID, before time.Time) (int, error) {
	proj, err := s.store.GetByID(ctx, projectID)
	if err != nil {
		return 0, err
	}
	versions, err := s.store.ListVersions(ctx, projectID)
	if err != nil {
		return 0, err
	}
	keep := make(map[string]struct{})
	for i := range versions {
		v := &versions[i]
		if v.Status != model.VersionStatusPublished {
			continue
		}
		lines, err := s.store.ListVersionLines(ctx, v.ID)
		if err != nil {
			return 0, err
		}
		for j := range lines {
			line := &lines[j]
			fulls, err := s.store.ListArtifactsByLineAndKind(ctx, line.ID, model.ArtifactKindFull)
			if err != nil {
				return 0, err
			}
			sourceCount := model.DefaultDeltaSourceCount
			if row, err := s.store.GetMatrix(ctx, projectID, line.OS, line.Arch); err == nil && row != nil && row.DeltaSourceCount > 0 {
				sourceCount = row.DeltaSourceCount
			}
			for k := range fulls {
				hw := fulls[k].HwRev
				sources, err := s.selectAutoDeltaSources(ctx, proj, v, line, hw, sourceCount)
				if err != nil {
					continue
				}
				tgtSHA := strings.ToLower(fulls[k].SHA256)
				for _, src := range sources {
					key := strings.ToLower(src.full.SHA256) + "|" + tgtSHA
					keep[key] = struct{}{}
				}
			}
		}
	}
	keepBlob := make(map[string]struct{})
	var stale []model.Artifact
	for i := range versions {
		arts, err := s.store.ListArtifactsByVersionID(ctx, versions[i].ID)
		if err != nil {
			return 0, err
		}
		for _, a := range arts {
			if a.Kind != model.ArtifactKindPatch {
				if a.StorageKey != "" {
					keepBlob[a.StorageKey] = struct{}{}
				}
				continue
			}
			pair := strings.ToLower(a.DeltaSourceSHA256) + "|" + strings.ToLower(a.DeltaTargetSHA256)
			keepPatch := false
			if _, ok := keep[pair]; ok {
				keepPatch = true
			}
			if !a.CreatedAt.Before(before) {
				keepPatch = true
			}
			if keepPatch {
				if a.StorageKey != "" {
					keepBlob[a.StorageKey] = struct{}{}
				}
				continue
			}
			stale = append(stale, a)
		}
	}
	cleaned := 0
	for _, a := range stale {
		if a.StorageKey != "" && s.storage != nil {
			if _, shared := keepBlob[a.StorageKey]; !shared {
				_ = s.storage.Delete(ctx, a.StorageKey)
			}
		}
		if err := s.store.DeleteArtifact(ctx, a.ID); err != nil {
			return cleaned, err
		}
		cleaned++
	}
	if cleaned > 0 {
		s.invalidateProject(ctx, projectID)
	}
	return cleaned, nil
}

// PresignUploadOutput 包含产物上传的预签名 URL 或 API 代传 URL (C05-3)。
type PresignUploadOutput struct {
	UploadURL string `json:"upload_url"`
	Method    string `json:"method"`
	DirectS3  bool   `json:"direct_s3"`
}

// PresignUpload 生成 S3 预签名直传 URL 或 API 代传 URL (C05-3)。
func (s *ProjectService) PresignUpload(ctx context.Context, projectRef, versionRef, osSlug, archSlug string, in UploadArtifactInput) (*PresignUploadOutput, error) {
	if s.storage == nil {
		return nil, ErrStorageUnavailable
	}
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
	if _, err := s.store.GetVersionLine(ctx, v.ID, canonicalOS, canonicalArch); err != nil {
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, err
		}
	}

	apiURL := fmt.Sprintf("/api/v1/admin/projects/%s/versions/%s/lines/%s/%s/artifacts", p.Slug, versionRef, canonicalOS, canonicalArch)
	return &PresignUploadOutput{
		UploadURL: apiURL,
		Method:    "PUT",
		DirectS3:  false,
	}, nil
}

func (s *ProjectService) validateHwRevs(ctx context.Context, projectID uuid.UUID, mainRev, minRev, maxRev *string, compRevs []string) error {
	if mainRev != nil && strings.TrimSpace(*mainRev) != "" {
		if _, err := s.store.GetHwRev(ctx, projectID, *mainRev); err != nil {
			return ErrHwRevUnknown
		}
	}
	if minRev != nil && strings.TrimSpace(*minRev) != "" {
		if _, err := s.store.GetHwRev(ctx, projectID, *minRev); err != nil {
			return ErrHwRevUnknown
		}
	}
	if maxRev != nil && strings.TrimSpace(*maxRev) != "" {
		if _, err := s.store.GetHwRev(ctx, projectID, *maxRev); err != nil {
			return ErrHwRevUnknown
		}
	}
	for _, c := range compRevs {
		if strings.TrimSpace(c) != "" {
			if _, err := s.store.GetHwRev(ctx, projectID, c); err != nil {
				return ErrHwRevUnknown
			}
		}
	}
	return nil
}

// onArtifactUploaded 在产物上传成功后触发：针对多文件切片执行 zip 内容解析、安全校验与 Manifest 校验/提取；
// 若为多文件但非全量 zip，切片状态保持 pending（C06-4, C06-6）。
func (s *ProjectService) onArtifactUploaded(ctx context.Context, p *model.Project, v *model.Version, line *model.VersionLine, art *model.Artifact) error {
	matrixRow, _ := s.store.GetMatrix(ctx, p.ID, line.OS, line.Arch)
	if matrixRow != nil && matrixRow.PackageType == model.PackageTypeMultiFile {
		if strings.HasSuffix(strings.ToLower(art.FileName), ".zip") || art.ContentType == "application/zip" {
			rc, err := s.storage.Get(ctx, art.StorageKey)
			if err != nil {
				return fmt.Errorf("read uploaded zip: %w", err)
			}
			zipBytes, err := io.ReadAll(rc)
			rc.Close()
			if err != nil {
				return fmt.Errorf("read zip content: %w", err)
			}

			zipEntries, err := ParseZipEntries(bytes.NewReader(zipBytes), int64(len(zipBytes)))
			if err != nil {
				return err
			}

			existingManifest, err := s.store.ListManifestEntries(ctx, line.ID)
			if err != nil {
				return err
			}

			if len(existingManifest) > 0 {
				if err := ValidateZipAgainstManifestEntries(zipEntries, existingManifest); err != nil {
					return err
				}
			} else {
				modelEntries := make([]model.ManifestEntry, 0, len(zipEntries))
				for _, z := range zipEntries {
					modelEntries = append(modelEntries, model.ManifestEntry{
						ID:             uuid.New(),
						ProjectID:      p.ID,
						VersionID:      v.ID,
						VersionLineID:  line.ID,
						Path:           z.NormalizedPath,
						Size:           z.Size,
						SHA256:         z.SHA256,
						MD5:            z.MD5,
						InstallPolicy:  z.InstallPolicy,
						IntegrityCheck: z.IntegrityCheck,
					})
				}
				rootHash, err := pathutil.CalculateRootHash(modelEntries)
				if err != nil {
					return err
				}
				if err := s.store.SaveManifestEntries(ctx, line.ID, modelEntries); err != nil {
					return err
				}
				line.RootHash = rootHash
			}

			if _, err := s.persistHashRootAndStoreFull(ctx, p, v, line, art.HwRev, zipBytes, zipEntries, art); err != nil {
				return err
			}

			line.Status = model.VersionLineStatusReady
			if err := s.store.SaveVersionLine(ctx, line); err != nil {
				return err
			}
			_, _ = s.CheckAndTriggerAutoPublish(ctx, p.ID, line.VersionID)
			// 多文件线 zip 就绪后触发自动增量归档生成（C13-1 补平台场景）。
			s.notifyLineReady(ctx, p.ID, line.VersionID)
			// 已发布版本补平台线就绪 webhook（C14-3）。
			s.notifyLineReadyWebhook(ctx, p.ID, v, line)
			s.invalidateProject(ctx, p.ID)
			return nil
		}

		// 上传的文件不是全量 zip：多文件切片在全量归档包上传前保持 pending 状态 (C06-4, C06-6)
		line.Status = model.VersionLineStatusPending
		if err := s.store.SaveVersionLine(ctx, line); err != nil {
			return err
		}
		s.invalidateProject(ctx, p.ID)
		return nil
	}

	line.Status = model.VersionLineStatusReady
	if err := s.store.SaveVersionLine(ctx, line); err != nil {
		return err
	}
	_, _ = s.CheckAndTriggerAutoPublish(ctx, p.ID, line.VersionID)
	// 单文件线产物就绪后触发自动差量生成（C13-1 补平台场景）。
	s.notifyLineReady(ctx, p.ID, line.VersionID)
	// 已发布版本补平台线就绪 webhook（C14-3）。
	s.notifyLineReadyWebhook(ctx, p.ID, v, line)
	s.invalidateProject(ctx, p.ID)
	return nil
}
