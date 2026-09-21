package service

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strings"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/Kirizu-Official/KiriVers/internal/model"
	"github.com/Kirizu-Official/KiriVers/internal/platform"
	"github.com/Kirizu-Official/KiriVers/pkg/hashutil"
	"github.com/Kirizu-Official/KiriVers/pkg/pathutil"
)

var (
	// ErrArchiveRequired 多文件平台切片就绪或发版前必须具有全量 zip 归档产物（C06-4, C06-6）。
	ErrArchiveRequired = errors.New("full archive is required for multi-file version line")
	// ErrZipManifestMismatch 全量 zip 内部文件清单与登记的 Manifest 不一致（C06-4, §7.5）。
	ErrZipManifestMismatch = errors.New("zip contents do not match manifest")
	// ErrDuplicateManifestPath Manifest 中包含重复的规范化路径。
	ErrDuplicateManifestPath = errors.New("duplicate path in manifest")
	// ErrManifestRequired 多文件平台切片需要 Manifest。
	ErrManifestRequired = errors.New("manifest is required for multi-file version line")
)

var (
	hex64Regex = regexp.MustCompile(`^[a-fA-F0-9]{64}$`)
	hex32Regex = regexp.MustCompile(`^[a-fA-F0-9]{32}$`)
)

// ManifestEntryInput 是提交/写入 Manifest 的单个文件输入项。
type ManifestEntryInput struct {
	Path          string `json:"path"`
	Size          int64  `json:"size"`
	SHA256        string `json:"sha256"`
	MD5           string `json:"md5"`
	InstallPolicy string `json:"install_policy,omitempty"`
}

// ManifestResult 是 Manifest 查询与写入操作的返回结构。
type ManifestResult struct {
	RootHash string                `json:"root_hash"`
	Count    int                   `json:"count"`
	Entries  []model.ManifestEntry `json:"entries"`
}

// SetManifest 为多文件 Version Line 提交或更新 Manifest 条目并重新计算 Root Hash（C06-1, C06-2, C06-3）。
func (s *ProjectService) SetManifest(ctx context.Context, projectRef, versionRef, osSlug, archSlug string, entries []ManifestEntryInput) (*ManifestResult, error) {
	p, err := s.Resolve(ctx, projectRef)
	if err != nil {
		return nil, err
	}
	v, err := s.ResolveVersion(ctx, p.ID, versionRef)
	if err != nil {
		return nil, err
	}
	if v.Status == model.VersionStatusPublished {
		return nil, ErrPublishedVersionImmutable
	}

	canonicalOS := platform.CanonicalOSWrite(osSlug)
	canonicalArch := platform.CanonicalArch(archSlug)
	line, err := s.store.GetVersionLine(ctx, v.ID, canonicalOS, canonicalArch)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrVersionLineNotFound
		}
		return nil, err
	}

	effective, err := s.effectiveForVersion(ctx, p.ID, v.ChannelSlug, canonicalOS, canonicalArch)
	if err != nil {
		return nil, err
	}

	// 校验与规范化每个条目。省略 install_policy 时套用生效模板；显式值原样保留（D2）。
	seenPaths := make(map[string]bool, len(entries))
	modelEntries := make([]model.ManifestEntry, 0, len(entries))

	for _, in := range entries {
		normPath, err := pathutil.NormalizeAndValidatePath(in.Path)
		if err != nil {
			return nil, err
		}
		if seenPaths[normPath] {
			return nil, fmt.Errorf("%w: %s", ErrDuplicateManifestPath, normPath)
		}
		seenPaths[normPath] = true

		sha256Hex := strings.ToLower(strings.TrimSpace(in.SHA256))
		if !hex64Regex.MatchString(sha256Hex) {
			return nil, fmt.Errorf("%w: invalid sha256 for path %s", ErrChecksumMismatch, normPath)
		}

		md5Hex := strings.ToLower(strings.TrimSpace(in.MD5))
		if !hex32Regex.MatchString(md5Hex) {
			return nil, fmt.Errorf("%w: invalid md5 for path %s", ErrChecksumMismatch, normPath)
		}

		if in.Size < 0 {
			return nil, fmt.Errorf("%w: size must be non-negative for path %s", pathutil.ErrInvalidPath, normPath)
		}

		policy := strings.ToUpper(strings.TrimSpace(in.InstallPolicy))
		if policy == "" {
			if stamped, ok := effective[normPath]; ok && stamped != "" {
				policy = stamped
			} else {
				policy = model.InstallPolicyOverwrite
			}
		}
		if policy != model.InstallPolicyOverwrite && policy != model.InstallPolicyKeepIfExists {
			return nil, fmt.Errorf("%w: unsupported install policy %q for path %s", pathutil.ErrInvalidPath, policy, normPath)
		}

		integrity := true
		if policy == model.InstallPolicyKeepIfExists {
			integrity = false
		}

		modelEntries = append(modelEntries, model.ManifestEntry{
			ID:             uuid.New(),
			ProjectID:      p.ID,
			VersionID:      v.ID,
			VersionLineID:  line.ID,
			Path:           normPath,
			Size:           in.Size,
			SHA256:         sha256Hex,
			MD5:            md5Hex,
			InstallPolicy:  policy,
			IntegrityCheck: integrity,
		})
	}

	rootHash, err := pathutil.CalculateRootHash(modelEntries)
	if err != nil {
		return nil, fmt.Errorf("calculate root hash: %w", err)
	}

	if err := s.store.SaveManifestEntries(ctx, line.ID, modelEntries); err != nil {
		return nil, err
	}

	line.RootHash = rootHash
	if err := s.store.SaveVersionLine(ctx, line); err != nil {
		return nil, err
	}

	s.invalidateProject(ctx, p.ID)
	return &ManifestResult{
		RootHash: rootHash,
		Count:    len(modelEntries),
		Entries:  modelEntries,
	}, nil
}

// GetManifest 查询指定平台切片的 Manifest 条目与 Root Hash（C06-1, C06-3）。
func (s *ProjectService) GetManifest(ctx context.Context, projectRef, versionRef, osSlug, archSlug string) (*ManifestResult, error) {
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
			return nil, ErrVersionLineNotFound
		}
		return nil, err
	}

	entries, err := s.store.ListManifestEntries(ctx, line.ID)
	if err != nil {
		return nil, err
	}

	rootHash := line.RootHash
	if rootHash == "" && len(entries) > 0 {
		rootHash, _ = pathutil.CalculateRootHash(entries)
	} else if rootHash == "" && len(entries) == 0 {
		rootHash = pathutil.EmptyRootHash
	}

	return &ManifestResult{
		RootHash: rootHash,
		Count:    len(entries),
		Entries:  entries,
	}, nil
}

// ParsedZipEntry 是解析 zip 文件内部得到的文件条目。
type ParsedZipEntry struct {
	NormalizedPath string
	Size           int64
	SHA256         string
	MD5            string
	InstallPolicy  string
	IntegrityCheck bool
	// Content 是条目字节，供重建哈希根目录 full 与带路径 store_full。
	Content []byte
}

// ParseZipEntries 扫描 zip 包，提取文件清单并计算哈希。keep sidecar 文件名视为忽略项，不进入清单、也不再据此标 KEEP。
func ParseZipEntries(r io.ReaderAt, size int64) ([]ParsedZipEntry, error) {
	zr, err := zip.NewReader(r, size)
	if err != nil {
		return nil, fmt.Errorf("read zip: %w", err)
	}

	seen := make(map[string]bool)
	var entries []ParsedZipEntry

	for _, f := range zr.File {
		if f.FileInfo().IsDir() {
			continue
		}
		normPath, err := pathutil.NormalizeAndValidatePath(f.Name)
		if err != nil {
			return nil, err
		}
		if isKeepSidecarPath(normPath) {
			continue
		}
		if seen[normPath] {
			return nil, fmt.Errorf("%w: duplicate path in zip: %s", pathutil.ErrInvalidPath, normPath)
		}
		seen[normPath] = true

		rc, err := f.Open()
		if err != nil {
			return nil, fmt.Errorf("open zip entry %s: %w", normPath, err)
		}
		body, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			return nil, fmt.Errorf("read zip entry %s: %w", normPath, err)
		}
		hasher := hashutil.NewMultiHasher(false)
		if _, err := hasher.Write(body); err != nil {
			return nil, fmt.Errorf("hash zip entry %s: %w", normPath, err)
		}

		entries = append(entries, ParsedZipEntry{
			NormalizedPath: normPath,
			Size:           int64(len(body)),
			SHA256:         hasher.SHA256(),
			MD5:            hasher.MD5(),
			InstallPolicy:  model.InstallPolicyOverwrite,
			IntegrityCheck: true,
			Content:        body,
		})
	}

	return entries, nil
}

// ValidateZipAgainstManifestEntries 比对 zip 中的文件列表与既有 Manifest 条目（C06-4, §7.5）。
func ValidateZipAgainstManifestEntries(zipEntries []ParsedZipEntry, manifestEntries []model.ManifestEntry) error {
	if len(zipEntries) != len(manifestEntries) {
		return fmt.Errorf("%w: zip entry count %d != manifest count %d", ErrZipManifestMismatch, len(zipEntries), len(manifestEntries))
	}

	manifestMap := make(map[string]model.ManifestEntry, len(manifestEntries))
	for _, m := range manifestEntries {
		manifestMap[m.Path] = m
	}

	for _, z := range zipEntries {
		m, ok := manifestMap[z.NormalizedPath]
		if !ok {
			return fmt.Errorf("%w: zip entry %s not found in manifest", ErrZipManifestMismatch, z.NormalizedPath)
		}
		if z.Size != m.Size {
			return fmt.Errorf("%w: size mismatch for %s (zip: %d, manifest: %d)", ErrZipManifestMismatch, z.NormalizedPath, z.Size, m.Size)
		}
		if !strings.EqualFold(z.SHA256, m.SHA256) {
			return fmt.Errorf("%w: sha256 mismatch for %s (zip: %s, manifest: %s)", ErrZipManifestMismatch, z.NormalizedPath, z.SHA256, m.SHA256)
		}
	}

	return nil
}

// BuildArchiveFromZipBuffer 从 zip 数据创建全量归档包产物并更新平台切片 Manifest 与状态（C06-4, C06-6）。
func (s *ProjectService) BuildArchiveFromZipBuffer(ctx context.Context, projectRef, versionRef, osSlug, archSlug string, zipData []byte) (*model.Artifact, *ManifestResult, error) {
	if s.storage == nil {
		return nil, nil, ErrStorageUnavailable
	}
	p, err := s.Resolve(ctx, projectRef)
	if err != nil {
		return nil, nil, err
	}
	v, err := s.ResolveVersion(ctx, p.ID, versionRef)
	if err != nil {
		return nil, nil, err
	}
	if v.Status == model.VersionStatusPublished {
		return nil, nil, ErrPublishedVersionImmutable
	}

	canonicalOS := platform.CanonicalOSWrite(osSlug)
	canonicalArch := platform.CanonicalArch(archSlug)
	line, err := s.store.GetVersionLine(ctx, v.ID, canonicalOS, canonicalArch)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil, ErrVersionLineNotFound
		}
		return nil, nil, err
	}

	readerAt := bytes.NewReader(zipData)
	zipSize := int64(len(zipData))

	zipEntries, err := ParseZipEntries(readerAt, zipSize)
	if err != nil {
		return nil, nil, err
	}

	existingManifest, err := s.store.ListManifestEntries(ctx, line.ID)
	if err != nil {
		return nil, nil, err
	}

	var finalManifestEntries []model.ManifestEntry
	var rootHash string

	if len(existingManifest) > 0 {
		// 校验 zip 内容是否与现有 Manifest 一致
		if err := ValidateZipAgainstManifestEntries(zipEntries, existingManifest); err != nil {
			return nil, nil, err
		}
		finalManifestEntries = existingManifest
		rootHash = line.RootHash
		if rootHash == "" {
			rootHash, _ = pathutil.CalculateRootHash(existingManifest)
		}
	} else {
		// 根据 zip 生成 Manifest；策略来自生效模板，不读 zip sidecar。
		effective, err := s.effectiveForVersion(ctx, p.ID, v.ChannelSlug, canonicalOS, canonicalArch)
		if err != nil {
			return nil, nil, err
		}
		finalManifestEntries = make([]model.ManifestEntry, 0, len(zipEntries))
		for _, z := range zipEntries {
			finalManifestEntries = append(finalManifestEntries, model.ManifestEntry{
				ID:            uuid.New(),
				ProjectID:     p.ID,
				VersionID:     v.ID,
				VersionLineID: line.ID,
				Path:          z.NormalizedPath,
				Size:          z.Size,
				SHA256:        z.SHA256,
				MD5:           z.MD5,
			})
		}
		applyEffectiveInstallPolicy(finalManifestEntries, effective)
		h, err := pathutil.CalculateRootHash(finalManifestEntries)
		if err != nil {
			return nil, nil, err
		}
		rootHash = h

		if err := s.store.SaveManifestEntries(ctx, line.ID, finalManifestEntries); err != nil {
			return nil, nil, err
		}
	}

	// 原生 full = 哈希根目录 zip；store_full 从解析条目重建，不含 keep sidecar 成员。
	art, err := s.persistHashRootAndStoreFull(ctx, p, v, line, nil, nil, zipEntries, nil)
	if err != nil {
		return nil, nil, err
	}

	line.RootHash = rootHash
	line.Status = model.VersionLineStatusReady
	if err := s.store.SaveVersionLine(ctx, line); err != nil {
		return nil, nil, err
	}
	_, _ = s.CheckAndTriggerAutoPublish(ctx, p.ID, line.VersionID)
	// 多文件全量归档就绪后触发自动增量归档生成（C13-1 补平台场景）。
	s.notifyLineReady(ctx, p.ID, line.VersionID)
	// 已发布版本补平台线就绪 webhook（C14-3）。
	s.notifyLineReadyWebhook(ctx, p.ID, v, line)

	s.invalidateProject(ctx, p.ID)
	return art, &ManifestResult{
		RootHash: rootHash,
		Count:    len(finalManifestEntries),
		Entries:  finalManifestEntries,
	}, nil
}

// BuildArchiveFromFiles 根据给定的文件内容在内存中打包 zip 并生成全量归档包（C06-4, C06-6）。
// 不写入 keep sidecar；安装策略由生效模板在 BuildArchiveFromZipBuffer 新 Manifest 分支套用。
func (s *ProjectService) BuildArchiveFromFiles(ctx context.Context, projectRef, versionRef, osSlug, archSlug string, files map[string][]byte) (*model.Artifact, *ManifestResult, error) {
	buf := new(bytes.Buffer)
	zw := zip.NewWriter(buf)

	for path, content := range files {
		normPath, err := pathutil.NormalizeAndValidatePath(path)
		if err != nil {
			return nil, nil, err
		}
		if isKeepSidecarPath(normPath) {
			continue
		}
		w, err := zw.Create(normPath)
		if err != nil {
			return nil, nil, err
		}
		if _, err := w.Write(content); err != nil {
			return nil, nil, err
		}
	}

	if err := zw.Close(); err != nil {
		return nil, nil, err
	}

	return s.BuildArchiveFromZipBuffer(ctx, projectRef, versionRef, osSlug, archSlug, buf.Bytes())
}
