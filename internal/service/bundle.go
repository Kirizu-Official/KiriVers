package service

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"slices"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/Kirizu-Official/KiriVers/internal/model"
	"github.com/Kirizu-Official/KiriVers/internal/platform"
	"github.com/Kirizu-Official/KiriVers/pkg/pathutil"
)

type lineKey struct {
	os    string
	arch  string
	hwRev string
}

var (
	// ErrZipLayoutInvalid 归档包布局不合法（如根目录裸文件、缺少 os/arch、路径遍历或未知平台标识）。
	ErrZipLayoutInvalid = errors.New("zip layout invalid: missing os/arch prefix or illegal path")
	// ErrJobNotFound 任务不存在。
	ErrJobNotFound = errors.New("job not found")
	// ErrJobForbidden 无权访问其他项目的任务。
	ErrJobForbidden = errors.New("forbidden: job belongs to another project")
	// ErrJobFailed 任务执行失败。
	ErrJobFailed = errors.New("job failed")
)

// BundleJobPayload 是 bundle_unpack 任务的载荷（C07-2, §11.4）。
type BundleJobPayload struct {
	ProjectID       uuid.UUID              `json:"project_id"`
	VersionRef      string                 `json:"version_ref"`
	Channel         string                 `json:"channel,omitempty"`
	Changelog       *string                `json:"changelog,omitempty"`
	Publish         bool                   `json:"publish"` // ci/releases 默认 true
	AutoPublishWhen *model.AutoPublishRule `json:"auto_publish_when,omitempty"`
	StorageKey      string                 `json:"storage_key"`
	IdempotencyKey  string                 `json:"idempotency_key,omitempty"`
}

// BundleUnpackResult 记录整包拆线的结果详情（C07-4, C07-8）。
type BundleUnpackResult struct {
	Success bool               `json:"success"`
	Version string             `json:"version"`
	Lines   []BundleLineResult `json:"lines"`
	Error   string             `json:"error,omitempty"`
}

// BundleLineResult 记录单条平台切片的拆线与产物结果。
type BundleLineResult struct {
	OS          string `json:"os"`
	Arch        string `json:"arch"`
	HwRev       string `json:"hw_rev,omitempty"`
	PackageType string `json:"package_type"` // "single_file" | "multi_file"
	Status      string `json:"status"`       // "ready" | "failed"
	ArtifactID  string `json:"artifact_id,omitempty"`
	RootHash    string `json:"root_hash,omitempty"`
	Error       string `json:"error,omitempty"`
}

// CreateBundleReleaseInput 是创建 CI Bundle 发版任务的输入。
type CreateBundleReleaseInput struct {
	Version         string
	Channel         string
	Changelog       *string
	Publish         *bool // 若为 nil，ci/releases 默认 true
	AutoPublishWhen *model.AutoPublishRule
	IdempotencyKey  string
	ArchiveData     []byte
	ArchiveFilename string
}

// CreateBundleJob 创建 bundle 解压与拆线异步任务（C07-2, C07-10）。
// 若提供了 24 小时内的 IdempotencyKey 且已存在对应任务，直接返回已有任务 ID（created=false）。
func (s *ProjectService) CreateBundleJob(ctx context.Context, projectID uuid.UUID, in CreateBundleReleaseInput) (uuid.UUID, bool, error) {
	if s.jobs == nil {
		return uuid.Nil, false, fmt.Errorf("job store is not configured")
	}
	if s.storage == nil {
		return uuid.Nil, false, ErrStorageUnavailable
	}

	// 1. 24 小时幂等键去重 (C07-10)
	key := strings.TrimSpace(in.IdempotencyKey)
	if key != "" {
		if existing, err := s.jobs.GetByIdempotencyKey(ctx, projectID, key); err == nil && existing != nil {
			return existing.ID, false, nil
		}
	}

	// 2. 确保版本存在或创建草稿版本
	publish := true
	if in.Publish != nil {
		publish = *in.Publish
	}

	versionRef := strings.TrimSpace(in.Version)
	if versionRef == "" {
		return uuid.Nil, false, fmt.Errorf("%w: version is required", ErrInvalidProjectSettings)
	}

	ch := strings.TrimSpace(in.Channel)
	if ch == "" {
		ch = "stable"
	}

	_, _, err := s.PutVersion(ctx, projectID, versionRef, VersionWriteInput{
		Channel:         ch,
		Changelog:       in.Changelog,
		AutoPublishWhen: in.AutoPublishWhen,
	})
	if err != nil {
		return uuid.Nil, false, err
	}

	proj, err := s.store.GetByID(ctx, projectID)
	if err != nil {
		return uuid.Nil, false, err
	}

	jobID := uuid.New()
	tempAbs, rel, _, _, err := s.writeLocalTemp(proj.Slug, bytes.NewReader(in.ArchiveData))
	if err != nil {
		return uuid.Nil, false, fmt.Errorf("store bundle archive: %w", err)
	}

	payload := BundleJobPayload{
		ProjectID:       projectID,
		VersionRef:      versionRef,
		Channel:         ch,
		Changelog:       in.Changelog,
		Publish:         publish,
		AutoPublishWhen: in.AutoPublishWhen,
		StorageKey:      rel,
		IdempotencyKey:  key,
	}
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return uuid.Nil, false, err
	}

	var ik *string
	if key != "" {
		ik = &key
	}

	job := &model.Job{
		ID:             jobID,
		Type:           "bundle_unpack",
		Status:         model.JobStatusQueued,
		Payload:        payloadBytes,
		ProjectID:      &projectID,
		OwnerNodeID:    s.ownerNodePtr(),
		IdempotencyKey: ik,
	}

	if err := s.jobs.Create(ctx, job); err != nil {
		removeLocal(tempAbs)
		return uuid.Nil, false, err
	}

	return jobID, true, nil
}

// GetJob 查询任务状态，并严格实施跨项目隔离（C07-8）。
// 如果 caller 不是实例管理员，且任务所属项目与 callerProjectID 不一致，返回 ErrJobForbidden。
func (s *ProjectService) GetJob(ctx context.Context, callerProjectID *uuid.UUID, isInstanceAdmin bool, jobID uuid.UUID) (*model.Job, error) {
	if s.jobs == nil {
		return nil, fmt.Errorf("job store is not configured")
	}
	job, err := s.jobs.GetByID(ctx, jobID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrJobNotFound
		}
		return nil, err
	}

	if !isInstanceAdmin {
		if callerProjectID == nil {
			return nil, ErrJobForbidden
		}
		if job.ProjectID != nil && *job.ProjectID != *callerProjectID {
			return nil, ErrJobForbidden
		}
	}

	return job, nil
}

// ExecuteBundleJob 直接执行指定 bundle_unpack 任务（供 worker 或直接调用）。
func (s *ProjectService) ExecuteBundleJob(ctx context.Context, jobID uuid.UUID) error {
	if s.jobs == nil {
		return fmt.Errorf("job store is not configured")
	}
	job, err := s.jobs.GetByID(ctx, jobID)
	if err != nil {
		return err
	}
	if job.Type != "bundle_unpack" {
		return fmt.Errorf("unexpected job type: %s", job.Type)
	}

	var payload BundleJobPayload
	if err := json.Unmarshal(job.Payload, &payload); err != nil {
		_ = s.jobs.MarkFailed(ctx, job.ID, "invalid payload json: "+err.Error())
		return err
	}

	// 读取归档包数据
	abs := s.localAbs(payload.StorageKey)
	archiveBytes, err := os.ReadFile(abs)
	if err != nil {
		_ = s.jobs.MarkFailed(ctx, job.ID, "failed to read bundle archive: "+err.Error())
		return err
	}

	// 解压并拆线
	result, unpackErr := s.UnpackAndProcessBundle(ctx, payload.ProjectID, payload.VersionRef, archiveBytes, payload.Publish)

	resultJSON, _ := json.Marshal(result)
	if unpackErr != nil {
		_ = s.jobs.MarkFailedWithResult(ctx, job.ID, unpackErr.Error(), resultJSON)
		return unpackErr
	}

	_ = s.jobs.MarkSucceededWithResult(ctx, job.ID, resultJSON)
	removeLocal(s.localAbs(payload.StorageKey))
	return nil
}

// RegisterBundleJobHandler 为 JobWorker 注册 bundle_unpack 类型的异步任务处理器。
func (s *ProjectService) RegisterBundleJobHandler(w *JobWorker) {
	if w == nil {
		return
	}
	w.RegisterHandler("bundle_unpack", func(ctx context.Context, jobType string, payload []byte) error {
		var p BundleJobPayload
		if err := json.Unmarshal(payload, &p); err != nil {
			return err
		}
		data, err := os.ReadFile(s.localAbs(p.StorageKey))
		if err != nil {
			return err
		}
		_, unpackErr := s.UnpackAndProcessBundle(ctx, p.ProjectID, p.VersionRef, data, p.Publish)
		removeLocal(s.localAbs(p.StorageKey))
		return unpackErr
	})
}

// UnpackAndProcessBundle 解压归档包、拆线、校验形态、上传产物并根据配置发布（C07-2–C07-6, C07-9）。
func (s *ProjectService) UnpackAndProcessBundle(ctx context.Context, projectID uuid.UUID, versionRef string, archiveBytes []byte, publish bool) (*BundleUnpackResult, error) {
	v, err := s.ResolveVersion(ctx, projectID, versionRef)
	if err != nil {
		return nil, err
	}
	proj, err := s.store.GetByID(ctx, projectID)
	if err != nil {
		return nil, err
	}

	verRef := versionRef
	if v.VersionSemver != nil && *v.VersionSemver != "" {
		verRef = *v.VersionSemver
	} else if v.VersionInteger != nil {
		verRef = strconv.FormatInt(*v.VersionInteger, 10)
	}

	// 1. 递归提取归档包文件列表并解除外层嵌套 (C07-2, §11.3.3)
	rawFiles, err := extractArchiveRecursively(archiveBytes, 0)
	if err != nil {
		return nil, err
	}

	// 2. 过滤特殊忽略文件 (__MACOSX/, .DS_Store, Thumbs.db) (C07-3)
	cleanFiles := make(map[string][]byte)
	for p, data := range rawFiles {
		norm, err := pathutil.NormalizeAndValidatePath(p)
		if err != nil {
			return nil, fmt.Errorf("%w: invalid file path %q: %v", ErrZipLayoutInvalid, p, err)
		}
		if isIgnoredPath(norm) {
			continue
		}
		cleanFiles[norm] = data
	}

	if len(cleanFiles) == 0 {
		return nil, fmt.Errorf("%w: archive contains no valid files", ErrZipLayoutInvalid)
	}

	matrixRows, _ := s.store.ListMatrix(ctx, projectID)
	matrixOS := make(map[string]struct{}, len(matrixRows))
	matrixArch := make(map[string]struct{}, len(matrixRows))
	for _, row := range matrixRows {
		matrixOS[row.OS] = struct{}{}
		matrixArch[row.Arch] = struct{}{}
	}

	// 3. 若存在单一顶层包装目录（且不是合法 OS / 本项目矩阵 os），剥离顶层前缀
	cleanFiles = stripWrapperDirectory(cleanFiles, matrixOS)

	// 4. 解析项目已登记的硬件变体
	hwRevs, _ := s.store.ListHwRevs(ctx, projectID)
	hwRevSet := make(map[string]bool)
	for _, hr := range hwRevs {
		hwRevSet[hr.Slug] = true
	}

	// 5. 路径分段并按 Line 分组 (C07-3, C07-4)
	lineFiles := make(map[lineKey]map[string][]byte)
	lineOrder := make([]lineKey, 0)

	for p, data := range cleanFiles {
		parts := strings.Split(p, "/")
		if len(parts) < 3 {
			// 根目录裸文件或缺少 os/arch 结构，整包拒绝 (C07-4)
			return nil, fmt.Errorf("%w: bare file or incomplete layout %q at root level", ErrZipLayoutInvalid, p)
		}

		rawOS := parts[0]
		rawArch := parts[1]

		canOS := platform.CanonicalOSWrite(rawOS)
		canArch := platform.CanonicalArch(rawArch)

		// 校验 OS 与 Arch：预置/别名，或本项目矩阵已登记且 ValidOSArchSlug 的自定义 slug
		if !isKnownOS(canOS, matrixOS) || !isKnownArch(canArch, matrixArch) {
			return nil, fmt.Errorf("%w: unknown os/arch %q in path %q", ErrZipLayoutInvalid, rawOS+"/"+rawArch, p)
		}

		var hwRev string
		var sliceRelPath string

		if hwRevSet[parts[2]] {
			hwRev = parts[2]
			if len(parts) < 4 {
				return nil, fmt.Errorf("%w: missing file under hw_rev %q in %q", ErrZipLayoutInvalid, hwRev, p)
			}
			sliceRelPath = strings.Join(parts[3:], "/")
		} else {
			sliceRelPath = strings.Join(parts[2:], "/")
		}

		key := lineKey{os: canOS, arch: canArch, hwRev: hwRev}
		if _, ok := lineFiles[key]; !ok {
			lineFiles[key] = make(map[string][]byte)
			lineOrder = append(lineOrder, key)
		}
		lineFiles[key][sliceRelPath] = data
	}

	result := &BundleUnpackResult{
		Success: true,
		Version: verRef,
		Lines:   make([]BundleLineResult, 0, len(lineOrder)),
	}

	// 6. 对每条平台切片分别处理单文件或多文件形态并校验矩阵 (C07-5, C07-6)
	hasError := false
	var firstErr error

	for _, key := range lineOrder {
		files := lineFiles[key]
		lineRes := BundleLineResult{
			OS:    key.os,
			Arch:  key.arch,
			HwRev: key.hwRev,
		}

		// 判断单文件 vs 多文件
		isSingle := isSingleFileSlice(files)
		var pkgType string
		if isSingle {
			pkgType = model.PackageTypeSingleFile
		} else {
			pkgType = model.PackageTypeMultiFile
		}
		lineRes.PackageType = pkgType

		// 与平台矩阵比对冲突 (C07-6)
		matrixRow, _ := s.store.GetMatrix(ctx, projectID, key.os, key.arch)
		if matrixRow != nil {
			if matrixRow.PackageType == model.PackageTypeSingleFile && !isSingle {
				err := fmt.Errorf("%w: matrix requires single_file for %s/%s but bundle has multi-file", ErrZipLayoutInvalid, key.os, key.arch)
				lineRes.Status = model.VersionLineStatusFailed
				lineRes.Error = err.Error()
				result.Lines = append(result.Lines, lineRes)
				hasError = true
				if firstErr == nil {
					firstErr = err
				}
				continue
			}
			if matrixRow.PackageType == model.PackageTypeMultiFile && isSingle {
				err := fmt.Errorf("%w: matrix requires multi_file for %s/%s but bundle has single-file", ErrZipLayoutInvalid, key.os, key.arch)
				lineRes.Status = model.VersionLineStatusFailed
				lineRes.Error = err.Error()
				result.Lines = append(result.Lines, lineRes)
				hasError = true
				if firstErr == nil {
					firstErr = err
				}
				continue
			}
		}

		// 确保 VersionLine 存在
		line, err := s.store.GetVersionLine(ctx, v.ID, key.os, key.arch)
		if err != nil {
			line = &model.VersionLine{
				VersionID: v.ID,
				ProjectID: projectID,
				OS:        key.os,
				Arch:      key.arch,
				Status:    model.VersionLineStatusProcessing,
			}
			if err := s.store.CreateVersionLine(ctx, line); err != nil {
				lineRes.Status = model.VersionLineStatusFailed
				lineRes.Error = err.Error()
				result.Lines = append(result.Lines, lineRes)
				hasError = true
				if firstErr == nil {
					firstErr = err
				}
				continue
			}
		} else {
			line.Status = model.VersionLineStatusProcessing
			_ = s.store.SaveVersionLine(ctx, line)
		}

		// 执行单文件或多文件构建
		var hwRevPtr *string
		if key.hwRev != "" {
			hwRevPtr = &key.hwRev
		}

		if isSingle {
			// 单文件上传
			var singleName string
			var singleData []byte
			for fn, fd := range files {
				singleName = fn
				singleData = fd
				break
			}
			art, err := s.UploadArtifact(ctx, proj.Slug, verRef, key.os, key.arch, UploadArtifactInput{
				Filename: singleName,
				Size:     int64(len(singleData)),
				HwRev:    hwRevPtr,
			}, bytes.NewReader(singleData))
			if err != nil {
				lineRes.Status = model.VersionLineStatusFailed
				lineRes.Error = err.Error()
				hasError = true
				if firstErr == nil {
					firstErr = err
				}
			} else {
				lineRes.Status = model.VersionLineStatusReady
				lineRes.ArtifactID = art.ID.String()
			}
		} else {
			// 多文件：打包全量归档；KEEP 来自生效模板，不读 zip sidecar。
			art, mres, err := s.BuildArchiveFromFiles(ctx, proj.Slug, verRef, key.os, key.arch, files)
			if err != nil {
				lineRes.Status = model.VersionLineStatusFailed
				lineRes.Error = err.Error()
				hasError = true
				if firstErr == nil {
					firstErr = err
				}
			} else {
				lineRes.Status = model.VersionLineStatusReady
				lineRes.ArtifactID = art.ID.String()
				lineRes.RootHash = mres.RootHash
			}
		}

		result.Lines = append(result.Lines, lineRes)
	}

	if hasError {
		result.Success = false
		result.Error = firstErr.Error()
		s.invalidateProject(ctx, projectID)
		return result, firstErr
	}

	// 7. 发版逻辑：若 publish 为 true，尝试发布版本 (C07-2, C07-9)
	if publish {
		_, pubErr := s.PublishVersion(ctx, projectID, verRef)
		if pubErr != nil && !errors.Is(pubErr, ErrAutoPublishPending) {
			// 如果不是等待其他切片就绪的 pending，则记录
			result.Error = pubErr.Error()
		}
	}

	s.invalidateProject(ctx, projectID)
	return result, nil
}

// extractArchiveRecursively 解压归档包并递归处理嵌套归档（若外层再套一层 zip/tar，以内层为准，§11.3.3）。
func extractArchiveRecursively(data []byte, depth int) (map[string][]byte, error) {
	if depth > 3 {
		return nil, fmt.Errorf("%w: nested archive too deep", ErrZipLayoutInvalid)
	}

	files := make(map[string][]byte)

	// 判定是 tar.gz 还是 zip
	if isGzip(data) {
		gzr, err := gzip.NewReader(bytes.NewReader(data))
		if err != nil {
			return nil, fmt.Errorf("%w: read gzip archive: %v", ErrZipLayoutInvalid, err)
		}
		defer gzr.Close()
		tr := tar.NewReader(gzr)
		for {
			hdr, err := tr.Next()
			if err == io.EOF {
				break
			}
			if err != nil {
				return nil, fmt.Errorf("%w: read tar entry: %v", ErrZipLayoutInvalid, err)
			}
			if hdr.Typeflag == tar.TypeDir {
				continue
			}
			content, err := io.ReadAll(tr)
			if err != nil {
				return nil, fmt.Errorf("%w: read tar content: %v", ErrZipLayoutInvalid, err)
			}
			files[hdr.Name] = content
		}
	} else if isZip(data) {
		zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
		if err != nil {
			return nil, fmt.Errorf("%w: read zip archive: %v", ErrZipLayoutInvalid, err)
		}
		for _, f := range zr.File {
			if f.FileInfo().IsDir() {
				continue
			}
			rc, err := f.Open()
			if err != nil {
				return nil, fmt.Errorf("%w: open zip file %s: %v", ErrZipLayoutInvalid, f.Name, err)
			}
			content, err := io.ReadAll(rc)
			_ = rc.Close()
			if err != nil {
				return nil, fmt.Errorf("%w: read zip file %s: %v", ErrZipLayoutInvalid, f.Name, err)
			}
			files[f.Name] = content
		}
	} else {
		return nil, fmt.Errorf("%w: unsupported archive format", ErrZipLayoutInvalid)
	}

	// 嵌套归档检查：若外层仅含一个文件且为归档包，解压内层
	if len(files) == 1 {
		for _, content := range files {
			if isGzip(content) || isZip(content) {
				return extractArchiveRecursively(content, depth+1)
			}
		}
	}

	return files, nil
}

// stripWrapperDirectory 如果所有文件都位于同一个顶层目录且该顶层目录不是合法 OS，则剔除该顶层目录。
func stripWrapperDirectory(files map[string][]byte, matrixOS map[string]struct{}) map[string][]byte {
	if len(files) == 0 {
		return files
	}

	var commonTop string
	first := true
	for p := range files {
		parts := strings.Split(p, "/")
		if len(parts) < 2 {
			return files // 有根目录文件，无需或不能剥离
		}
		top := parts[0]
		if first {
			commonTop = top
			first = false
		} else if commonTop != top {
			return files // 不共享相同的顶层目录
		}
	}

	// 检查 commonTop 是否本身就是一个标准 OS slug 或本项目矩阵 os
	normTop := platform.CanonicalOSWrite(commonTop)
	if isKnownOS(normTop, matrixOS) {
		return files
	}

	// 剥离 commonTop/
	res := make(map[string][]byte, len(files))
	prefix := commonTop + "/"
	for p, data := range files {
		trimmed := strings.TrimPrefix(p, prefix)
		res[trimmed] = data
	}
	return res
}

func isKeepSidecarPath(p string) bool {
	base := path.Base(p)
	return base == "keep_if_exists.txt" || base == "_keep.json"
}

func isIgnoredPath(p string) bool {
	base := path.Base(p)
	if base == ".DS_Store" || base == "Thumbs.db" || isKeepSidecarPath(p) {
		return true
	}
	parts := strings.Split(p, "/")
	for _, part := range parts {
		if part == "__MACOSX" {
			return true
		}
	}
	return false
}

func isSingleFileSlice(files map[string][]byte) bool {
	// 剔除 keep 声明文件后看是否恰好 1 个文件且无子目录
	count := 0
	for fn := range files {
		base := path.Base(fn)
		if base == "keep_if_exists.txt" || base == "_keep.json" {
			continue
		}
		if strings.Contains(fn, "/") {
			return false // 包含子目录，视作多文件结构
		}
		count++
	}
	return count == 1
}

func isGzip(data []byte) bool {
	return len(data) >= 2 && data[0] == 0x1f && data[1] == 0x8b
}

func isZip(data []byte) bool {
	return len(data) >= 4 && data[0] == 0x50 && data[1] == 0x4b
}

func isKnownOS(osSlug string, matrixOS map[string]struct{}) bool {
	canon := platform.CanonicalOSWrite(osSlug)
	if slices.Contains(platform.PresetOS, canon) {
		return true
	}
	if !platform.ValidOSArchSlug(canon) {
		return false
	}
	_, ok := matrixOS[canon]
	return ok
}

func isKnownArch(archSlug string, matrixArch map[string]struct{}) bool {
	canon := platform.CanonicalArch(archSlug)
	if slices.Contains(platform.PresetArch, canon) {
		return true
	}
	if !platform.ValidOSArchSlug(canon) {
		return false
	}
	_, ok := matrixArch[canon]
	return ok
}
