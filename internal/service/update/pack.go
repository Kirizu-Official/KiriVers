package update

import (
	"context"
	"strconv"
	"strings"

	"github.com/google/uuid"

	"github.com/Kirizu-Official/KiriVers/internal/model"
	"github.com/Kirizu-Official/KiriVers/pkg/signature"
)

const (
	// PackStatusPending 动态打包 Job 排队或执行中。
	PackStatusPending = "pending"
	// PackStatusReady 缓存命中或 Job 已落库可下载产物。
	PackStatusReady = "ready"
	// PackStatusFullPackage 不入队、立即走原生全量（未知路径 / D6 / D7 / 失败）。
	PackStatusFullPackage = "full_package"
)

// PackInput 是 POST update/pack 的请求参数。
type PackInput struct {
	SourceVersion string
	TargetVersion string
	OS            string
	Arch          string
	Channel       string
	DeviceID      string
	DeviceHash    string
	HwRev         string
	NeededPaths   []string

	signing urlSigning
}

// PackFile 是 ready 响应里的成员元数据（path + sha256，供哈希根目录落盘）。
type PackFile struct {
	Path           string `json:"path"`
	Size           int64  `json:"size"`
	SHA256         string `json:"sha256"`
	InstallPolicy  string `json:"install_policy,omitempty"`
	IntegrityCheck bool   `json:"integrity_check"`
}

// PackResponse 是 POST update/pack 的响应体。
type PackResponse struct {
	Status         string     `json:"status"`
	DiffMode       string     `json:"diff_mode,omitempty"`
	Compression    string     `json:"compression,omitempty"`
	PackageURL     string     `json:"package_url,omitempty"`
	FileName       string     `json:"file_name,omitempty"`
	Size           int64      `json:"size,omitempty"`
	SHA256         string     `json:"sha256,omitempty"`
	Files          []PackFile `json:"files,omitempty"`
	DeletedPaths   []string   `json:"deleted_paths,omitempty"`
	InvalidPaths   []string   `json:"invalid_paths,omitempty"`
	RootHash       string     `json:"root_hash,omitempty"`
	VersionInteger *int64     `json:"version_integer,omitempty"`
	VersionSemver  *string    `json:"version_semver,omitempty"`
	Channel        string     `json:"channel,omitempty"`
	CompareEngine  string     `json:"compare_engine,omitempty"`
	Signature      string     `json:"signature,omitempty"`
}

// PackResult 含 HTTP 状态：200 ready/full_package，202 pending。
type PackResult struct {
	Status int
	Body   *PackResponse
}

// PackArtifact 是 fileset 缓存命中的 patch 产物（无 zip 字节）。
type PackArtifact struct {
	FileName   string
	SHA256     string
	Size       int64
	StorageKey string
}

// PackJob 是动态打包任务的查询投影。
type PackJob struct {
	Status string
}

// DynamicPackRequest 是入队 dynamic_pack Job 的载荷（HTTP 层不打 zip）。
type DynamicPackRequest struct {
	ProjectID        uuid.UUID
	LineID           uuid.UUID
	VersionID        uuid.UUID
	OS               string
	Arch             string
	Hw               string
	FilesetSHA256    string
	Needed           []FilesetEntry
	SourceFullSHA    string
	TargetFullSHA    string
	SourceVersionRef string
	TargetVersionRef string
}

// PackRuntime 由 ProjectService 实现：查缓存、查在途 Job、入队。update 包不导入 archive/zip。
type PackRuntime interface {
	LookupFilesetPatch(ctx context.Context, lineID uuid.UUID, hw, filesetSHA string) (*PackArtifact, error)
	LookupFilesetJob(ctx context.Context, projectID uuid.UUID, key string) (*PackJob, error)
	EnqueueDynamicPack(ctx context.Context, req DynamicPackRequest) error
}

// Pack 执行 fileset 动态打包裁决（HTTP 层便捷入口）。
func (s *Service) Pack(ctx context.Context, projectID uuid.UUID, os, arch string, in PackInput) (*PackResult, error) {
	cat, err := s.loadCatalog(ctx, projectID, os, arch)
	if err != nil {
		return nil, err
	}
	in.signing = s.signingFor(cat)
	return Pack(ctx, cat, s.details, s.pack, s.dynamicPackMaxBytes, in)
}

// Pack 是纯函数裁决：身份/灰度与 diff 同规则，然后缓存 → D6/D7 → 入队或查询。
func Pack(ctx context.Context, cat *Catalog, details LineDetailSource, runtime PackRuntime, maxBytes int64, in PackInput) (*PackResult, error) {
	if cat == nil {
		return nil, ErrInvalidQuery
	}
	if details == nil {
		return nil, ErrLineDetailsUnavailable
	}

	srcRef, ok := parseVersionRef(in.SourceVersion)
	if !ok {
		return nil, ErrVersionNotFound
	}
	tgtRef, ok := parseVersionRef(in.TargetVersion)
	if !ok {
		return nil, ErrVersionNotFound
	}
	source := findVersionState(cat, srcRef)
	target := findVersionState(cat, tgtRef)
	if source == nil || target == nil {
		return nil, ErrVersionNotFound
	}
	switch target.Version.Status {
	case model.VersionStatusDraft:
		return nil, ErrVersionNotVisible
	case model.VersionStatusRevoked:
		return nil, ErrVersionRevoked
	}
	if ch := strings.TrimSpace(in.Channel); ch != "" && ch != target.Version.ChannelSlug {
		return nil, ErrChannelConflict
	}

	srcLine := source.Line(in.OS, in.Arch)
	tgtLine := target.Line(in.OS, in.Arch)
	escape := source.Version.Status == model.VersionStatusRevoked ||
		(srcLine != nil && srcLine.Status == model.VersionLineStatusYanked)
	if !escape && !diffGrayHit(cat, source, tgtLine, target, DiffInput{
		OS: in.OS, Arch: in.Arch, HwRev: in.HwRev, DeviceID: in.DeviceID, DeviceHash: in.DeviceHash,
	}) {
		return nil, ErrPreconditionFailed
	}

	if tgtLine == nil || tgtLine.Status != model.VersionLineStatusReady {
		return nil, ErrVersionLineNotFound
	}
	if tgtLine.PacksReadyAt == nil {
		return nil, ErrVersionNotVisible
	}
	tgtPkg := matchHwVariant(cat, cat.Matrix, tgtLine, strings.TrimSpace(in.HwRev))
	if tgtPkg == nil {
		return nil, ErrVersionLineNotFound
	}
	fullURL := in.signing.artifactURL(cat.Project.Slug, tgtPkg.SHA256, tgtPkg.StorageKey)

	fullResp := func() *PackResult {
		body := &PackResponse{
			Status:         PackStatusFullPackage,
			DiffMode:       DiffModeFullPackage,
			Compression:    model.ArtifactCompressionZip,
			PackageURL:     fullURL,
			FileName:       tgtPkg.FileName,
			Size:           tgtPkg.Size,
			SHA256:         tgtPkg.SHA256,
			RootHash:       tgtLine.RootHash,
			VersionInteger: target.Version.VersionInteger,
			VersionSemver:  target.Version.VersionSemverCanonical,
			Channel:        target.Version.ChannelSlug,
			CompareEngine:  cat.Project.CompareEngine,
		}
		signPack(cat, target, tgtLine, body)
		return &PackResult{Status: 200, Body: body}
	}

	matrix := matrixOrEmpty(cat.Matrix)
	if matrix.PackageType == model.PackageTypeSingleFile {
		return fullResp(), nil
	}

	td, err := details.LineDetails(ctx, tgtLine.ID)
	if err != nil {
		return nil, err
	}
	fileset := ResolveNeededFileset(in.NeededPaths, td.Manifest)
	baseMeta := func(status, mode string) *PackResponse {
		return &PackResponse{
			Status:         status,
			DiffMode:       mode,
			InvalidPaths:   fileset.InvalidPaths,
			RootHash:       tgtLine.RootHash,
			VersionInteger: target.Version.VersionInteger,
			VersionSemver:  target.Version.VersionSemverCanonical,
			Channel:        target.Version.ChannelSlug,
			CompareEngine:  cat.Project.CompareEngine,
		}
	}

	if fileset.Unknown {
		body := fullResp().Body
		body.InvalidPaths = fileset.InvalidPaths
		return &PackResult{Status: 200, Body: body}, nil
	}

	var deleted []string
	srcHealthy := srcLine != nil && srcLine.Status == model.VersionLineStatusReady
	if srcHealthy {
		sd, err := details.LineDetails(ctx, srcLine.ID)
		if err != nil {
			return nil, err
		}
		deleted = diffDeletedPaths(sd.Manifest, td.Manifest)
	}

	if len(fileset.Entries) == 0 {
		body := baseMeta(PackStatusReady, DiffModePatchPackage)
		body.Compression = model.ArtifactCompressionZip
		body.DeletedPaths = deleted
		body.Files = []PackFile{}
		signPack(cat, target, tgtLine, body)
		return &PackResult{Status: 200, Body: body}, nil
	}

	if maxBytes <= 0 {
		maxBytes = 512 * 1024 * 1024
	}
	if ExceedsUncompressedGates(fileset.NeededSum, ManifestUncompressedSum(td.Manifest), maxBytes) {
		body := fullResp().Body
		body.InvalidPaths = fileset.InvalidPaths
		body.DeletedPaths = deleted
		return &PackResult{Status: 200, Body: body}, nil
	}

	hw := strings.TrimSpace(in.HwRev)
	files := make([]PackFile, 0, len(fileset.Entries))
	for _, e := range fileset.Entries {
		files = append(files, PackFile{
			Path: e.Path, Size: e.Size, SHA256: e.SHA256,
			InstallPolicy: e.InstallPolicy, IntegrityCheck: e.IntegrityCheck,
		})
	}

	readyFromArt := func(art *PackArtifact) *PackResult {
		body := baseMeta(PackStatusReady, DiffModePatchPackage)
		body.Compression = model.ArtifactCompressionZip
		body.PackageURL = in.signing.artifactURL(cat.Project.Slug, art.SHA256, art.StorageKey)
		body.FileName = art.FileName
		body.Size = art.Size
		body.SHA256 = art.SHA256
		body.Files = files
		body.DeletedPaths = deleted
		signPack(cat, target, tgtLine, body)
		return &PackResult{Status: 200, Body: body}
	}

	if runtime != nil {
		if art, err := runtime.LookupFilesetPatch(ctx, tgtLine.ID, hw, fileset.SHA256); err != nil {
			return nil, err
		} else if art != nil && art.Size > 0 {
			return readyFromArt(art), nil
		}
	}

	key := DynamicPackIdempotencyKey(tgtLine.ID.String(), hw, fileset.SHA256)
	if runtime != nil {
		job, err := runtime.LookupFilesetJob(ctx, cat.Project.ID, key)
		if err != nil {
			return nil, err
		}
		if job != nil {
			switch job.Status {
			case model.JobStatusQueued, model.JobStatusRunning:
				body := baseMeta(PackStatusPending, "")
				body.DeletedPaths = deleted
				return &PackResult{Status: 202, Body: body}, nil
			case model.JobStatusFailed:
				return fullResp(), nil
			case model.JobStatusSucceeded:
				if art, err := runtime.LookupFilesetPatch(ctx, tgtLine.ID, hw, fileset.SHA256); err != nil {
					return nil, err
				} else if art != nil && art.Size > 0 {
					return readyFromArt(art), nil
				}
				return fullResp(), nil
			}
		}
	}

	if runtime == nil {
		return fullResp(), nil
	}

	srcSHA, tgtSHA := "", tgtPkg.SHA256
	srcRefStr, tgtRefStr := "", versionRefString(target)
	if srcHealthy {
		if p := matchHwVariant(cat, cat.Matrix, srcLine, hw); p != nil {
			srcSHA = p.SHA256
		}
		srcRefStr = versionRefString(source)
	}
	if err := runtime.EnqueueDynamicPack(ctx, DynamicPackRequest{
		ProjectID:        cat.Project.ID,
		LineID:           tgtLine.ID,
		VersionID:        target.Version.ID,
		OS:               in.OS,
		Arch:             in.Arch,
		Hw:               hw,
		FilesetSHA256:    fileset.SHA256,
		Needed:           fileset.Entries,
		SourceFullSHA:    srcSHA,
		TargetFullSHA:    tgtSHA,
		SourceVersionRef: srcRefStr,
		TargetVersionRef: tgtRefStr,
	}); err != nil {
		return nil, err
	}
	body := baseMeta(PackStatusPending, "")
	body.DeletedPaths = deleted
	return &PackResult{Status: 202, Body: body}, nil
}

func signPack(cat *Catalog, target *VersionState, line *LineState, body *PackResponse) {
	if cat.Project.SigningPrivateKey == "" || body.PackageURL == "" {
		return
	}
	payload := signature.BuildCheckPayload(
		int64OrEmpty(target.Version.VersionInteger),
		stringOrEmpty(target.Version.VersionSemverCanonical),
		line.RootHash,
		body.PackageURL,
		strconv.FormatInt(body.Size, 10),
		body.SHA256,
	)
	if sig, err := signature.SignPayload(cat.Project.SigningAlgo, cat.Project.SigningPrivateKey, payload); err == nil {
		body.Signature = sig
	}
}
