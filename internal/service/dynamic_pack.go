package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/Kirizu-Official/KiriVers/internal/cache"
	"github.com/Kirizu-Official/KiriVers/internal/config"
	"github.com/Kirizu-Official/KiriVers/internal/model"
	"github.com/Kirizu-Official/KiriVers/internal/service/update"
)

const dynamicPackJobType = "dynamic_pack"

// dynamicPackJobPayload 是客户端 fileset 动态打包任务载荷。
type dynamicPackJobPayload struct {
	ProjectID        uuid.UUID             `json:"project_id"`
	LineID           uuid.UUID             `json:"line_id"`
	VersionID        uuid.UUID             `json:"version_id"`
	OS               string                `json:"os"`
	Arch             string                `json:"arch"`
	Hw               string                `json:"hw"`
	FilesetSHA256    string                `json:"fileset_sha256"`
	Needed           []update.FilesetEntry `json:"needed"`
	SourceFullSHA    string                `json:"source_full_sha256"`
	TargetFullSHA    string                `json:"target_full_sha256"`
	SourceVersionRef string                `json:"source_version_ref"`
	TargetVersionRef string                `json:"target_version_ref"`
}

// SetDynamicPackMaxBytes 注入 D6 硬顶（0 = 默认 512MiB）。
func (s *ProjectService) SetDynamicPackMaxBytes(n int64) {
	s.dynamicPackMaxBytes = n
}

// SetFileListMaxFiles 注入原生 file_list 平台天花板（<1 = 默认 16）。
func (s *ProjectService) SetFileListMaxFiles(n int) {
	s.fileListMaxFiles = n
}

// FileListMaxFilesLimit 返回当前平台天花板（供管理 GET 只读字段）。
func (s *ProjectService) FileListMaxFilesLimit() int {
	if s == nil || s.fileListMaxFiles < 1 {
		return model.DefaultFileListMaxFiles
	}
	return s.fileListMaxFiles
}

// SetChangelogLimits 注入实例 changelog 条数窗口（<1 = 产品缺省 5/50）。
func (s *ProjectService) SetChangelogLimits(defaultEntries, maxEntries int) {
	s.changelogDefaultEntries = defaultEntries
	s.changelogMaxEntries = maxEntries
}

// ChangelogDefaultEntriesLimit 返回实例无 from_version 条数上限。
func (s *ProjectService) ChangelogDefaultEntriesLimit() int {
	if s == nil || s.changelogDefaultEntries < 1 {
		return model.DefaultChangelogDefaultEntries
	}
	return s.changelogDefaultEntries
}

// ChangelogMaxEntriesLimit 返回实例有 from_version 截断上限。
func (s *ProjectService) ChangelogMaxEntriesLimit() int {
	if s == nil || s.changelogMaxEntries < 1 {
		return model.DefaultChangelogMaxEntries
	}
	return s.changelogMaxEntries
}

func (s *ProjectService) packMaxBytes() int64 {
	if s.dynamicPackMaxBytes > 0 {
		return s.dynamicPackMaxBytes
	}
	return config.DefaultDynamicPackMaxBytes
}

// LookupFilesetPatch 实现 update.PackRuntime：按 fileset+hw 查找已落库 patch。
func (s *ProjectService) LookupFilesetPatch(ctx context.Context, lineID uuid.UUID, hw, filesetSHA string) (*update.PackArtifact, error) {
	patches, err := s.store.ListArtifactsByLineAndKind(ctx, lineID, model.ArtifactKindPatch)
	if err != nil {
		return nil, err
	}
	want := strings.ToLower(strings.TrimSpace(filesetSHA))
	hw = strings.TrimSpace(hw)
	for i := range patches {
		p := &patches[i]
		if p.Size <= 0 {
			continue
		}
		if !strings.EqualFold(p.FilesetSHA256, want) {
			continue
		}
		if artifactHwLabel(p.HwRev) != hw {
			continue
		}
		return &update.PackArtifact{FileName: p.FileName, SHA256: p.SHA256, Size: p.Size, StorageKey: p.StorageKey}, nil
	}
	return nil, nil
}

// LookupFilesetJob 实现 update.PackRuntime：按幂等键查找 24h 内任务。
func (s *ProjectService) LookupFilesetJob(ctx context.Context, projectID uuid.UUID, key string) (*update.PackJob, error) {
	if s.jobs == nil {
		return nil, nil
	}
	job, err := s.jobs.GetByIdempotencyKey(ctx, projectID, key)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	if job == nil {
		return nil, nil
	}
	return &update.PackJob{Status: job.Status}, nil
}

// EnqueueDynamicPack 实现 update.PackRuntime：GetByIdempotencyKey 后插入一条 Job。
func (s *ProjectService) EnqueueDynamicPack(ctx context.Context, req update.DynamicPackRequest) error {
	if s.jobs == nil {
		return fmt.Errorf("job store is not configured")
	}
	s.packEnqueueMu.Lock()
	defer s.packEnqueueMu.Unlock()
	key := update.DynamicPackIdempotencyKey(req.LineID.String(), req.Hw, req.FilesetSHA256)
	if existing, err := s.jobs.GetByIdempotencyKey(ctx, req.ProjectID, key); err == nil && existing != nil {
		return nil
	}
	if s.cache != nil {
		if _, hit, err := s.cache.Get(ctx, cache.PackDoneKey(key)); err == nil && hit {
			return nil
		}
		ok, err := s.cache.SetNX(ctx, cache.PackOccupancyKey(key), []byte("1"), 24*time.Hour)
		if err == nil && !ok {
			return nil
		}
	}
	payload, err := json.Marshal(dynamicPackJobPayload{
		ProjectID:        req.ProjectID,
		LineID:           req.LineID,
		VersionID:        req.VersionID,
		OS:               req.OS,
		Arch:             req.Arch,
		Hw:               req.Hw,
		FilesetSHA256:    req.FilesetSHA256,
		Needed:           req.Needed,
		SourceFullSHA:    req.SourceFullSHA,
		TargetFullSHA:    req.TargetFullSHA,
		SourceVersionRef: req.SourceVersionRef,
		TargetVersionRef: req.TargetVersionRef,
	})
	if err != nil {
		return err
	}
	job := &model.Job{
		ID:             uuid.New(),
		Type:           dynamicPackJobType,
		Status:         model.JobStatusQueued,
		Payload:        payload,
		ProjectID:      &req.ProjectID,
		IdempotencyKey: &key,
	}
	if err := s.jobs.Create(ctx, job); err != nil {
		if s.cache != nil {
			_ = s.cache.Delete(ctx, cache.PackOccupancyKey(key))
		}
		return err
	}
	return nil
}

// RegisterDynamicPackJobHandler 注册 dynamic_pack worker。
func (s *ProjectService) RegisterDynamicPackJobHandler(w *JobWorker) {
	if w == nil {
		return
	}
	w.RegisterHandler(dynamicPackJobType, func(ctx context.Context, jobType string, payload []byte) error {
		var p dynamicPackJobPayload
		if err := json.Unmarshal(payload, &p); err != nil {
			return err
		}
		_, err := s.runDynamicPack(ctx, p)
		return err
	})
}

// ExecuteDynamicPackJob 同步执行指定 dynamic_pack（测试用）。
func (s *ProjectService) ExecuteDynamicPackJob(ctx context.Context, jobID uuid.UUID) error {
	if s.jobs == nil {
		return fmt.Errorf("job store is not configured")
	}
	job, err := s.jobs.GetByID(ctx, jobID)
	if err != nil {
		return err
	}
	if job.Type != dynamicPackJobType {
		return fmt.Errorf("unexpected job type: %s", job.Type)
	}
	var payload dynamicPackJobPayload
	if err := json.Unmarshal(job.Payload, &payload); err != nil {
		_ = s.jobs.MarkFailed(ctx, job.ID, "invalid payload json: "+err.Error())
		return err
	}
	art, runErr := s.runDynamicPack(ctx, payload)
	if runErr != nil {
		_ = s.jobs.MarkFailed(ctx, job.ID, runErr.Error())
		return runErr
	}
	resultJSON, _ := json.Marshal(art)
	_ = s.jobs.MarkSucceededWithResult(ctx, job.ID, resultJSON)
	return nil
}

func (s *ProjectService) runDynamicPack(ctx context.Context, p dynamicPackJobPayload) (*model.Artifact, error) {
	if s.storage == nil {
		return nil, ErrStorageUnavailable
	}
	if existing, err := s.LookupFilesetPatch(ctx, p.LineID, p.Hw, p.FilesetSHA256); err != nil {
		return nil, err
	} else if existing != nil {
		s.markPackDone(ctx, p)
		return &model.Artifact{FileName: existing.FileName, SHA256: existing.SHA256, Size: existing.Size}, nil
	}
	fulls, err := s.store.ListArtifactsByLineAndKind(ctx, p.LineID, model.ArtifactKindFull)
	if err != nil {
		return nil, err
	}
	var targetFull *model.Artifact
	for i := range fulls {
		if artifactHwLabel(fulls[i].HwRev) == strings.TrimSpace(p.Hw) {
			targetFull = &fulls[i]
			break
		}
	}
	if targetFull == nil {
		return nil, fmt.Errorf("target full artifact missing")
	}
	needed := make([]model.ManifestEntry, 0, len(p.Needed))
	for _, e := range p.Needed {
		needed = append(needed, model.ManifestEntry{Path: e.Path, SHA256: e.SHA256, Size: e.Size, InstallPolicy: e.InstallPolicy, IntegrityCheck: e.IntegrityCheck})
	}
	patchBytes, err := s.buildPatchZipFromTarget(ctx, targetFull, needed)
	if err != nil {
		return nil, err
	}
	artifactID := uuid.New()
	key, _, hasher, err := s.putCanonicalBytes(ctx, mustProjectSlug(ctx, s, p.ProjectID), "application/zip", patchBytes)
	if err != nil {
		return nil, err
	}
	hwPtr := (*string)(nil)
	if strings.TrimSpace(p.Hw) != "" {
		h := strings.TrimSpace(p.Hw)
		hwPtr = &h
	}
	fileName := BuildPatchStableFilename(
		mustProjectSlug(ctx, s, p.ProjectID),
		p.TargetVersionRef, p.SourceVersionRef,
		p.OS, p.Arch, hwPtr,
		p.SourceFullSHA, p.TargetFullSHA,
	)
	if p.FilesetSHA256 != "" {
		fileName = strings.TrimSuffix(fileName, ".zip") + "-" + p.FilesetSHA256[:min(12, len(p.FilesetSHA256))] + ".zip"
	}
	art := &model.Artifact{
		ID:                artifactID,
		ProjectID:         p.ProjectID,
		VersionID:         p.VersionID,
		VersionLineID:     p.LineID,
		Kind:              model.ArtifactKindPatch,
		FileName:          fileName,
		StorageKey:        key,
		Size:              int64(len(patchBytes)),
		SHA256:            hasher.SHA256(),
		MD5:               hasher.MD5(),
		ContentType:       "application/zip",
		Compression:       model.ArtifactCompressionZip,
		FilesetSHA256:     p.FilesetSHA256,
		HwRev:             hwPtr,
		DeltaSourceSHA256: p.SourceFullSHA,
		DeltaTargetSHA256: p.TargetFullSHA,
	}
	if err := s.store.CreateArtifact(ctx, art); err != nil {
		_ = s.storage.Delete(ctx, key)
		return nil, err
	}
	s.invalidateProject(ctx, p.ProjectID)
	s.markPackDone(ctx, p)
	return art, nil
}

func (s *ProjectService) markPackDone(ctx context.Context, p dynamicPackJobPayload) {
	if s == nil || s.cache == nil {
		return
	}
	idem := update.DynamicPackIdempotencyKey(p.LineID.String(), p.Hw, p.FilesetSHA256)
	_ = s.cache.SetWithTTL(ctx, cache.PackDoneKey(idem), []byte("1"), 24*time.Hour)
	_ = s.cache.Delete(ctx, cache.PackOccupancyKey(idem))
}

func mustProjectSlug(ctx context.Context, s *ProjectService, id uuid.UUID) string {
	p, err := s.store.GetByID(ctx, id)
	if err != nil || p == nil {
		return "project"
	}
	return p.Slug
}
