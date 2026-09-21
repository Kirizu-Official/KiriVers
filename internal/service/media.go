package service

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"path"
	"path/filepath"
	"strings"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/Kirizu-Official/KiriVers/internal/model"
	"github.com/Kirizu-Official/KiriVers/internal/repository"
	"github.com/Kirizu-Official/KiriVers/internal/storage"
	"github.com/Kirizu-Official/KiriVers/pkg/hashutil"
)

var (
	// ErrMediaNotFound 媒体不存在或不属于该项目。
	ErrMediaNotFound = errors.New("media not found")
	// ErrMediaStorageUnavailable 对象存储未装配。
	ErrMediaStorageUnavailable = errors.New("storage backend unavailable")
)

// MediaService 负责 Markdown 媒体 Put/Get 与 S3 本地副本。不使用 Presign、TUS 或 urlsign。
type MediaService struct {
	store   repository.ProjectMediaStore
	backend storage.Backend
	// replica 在 storage.driver=s3 时为 NewLocalFS(storage.local.root)；local 驱动为 nil（不二次写入）。
	replica storage.Backend
}

// NewMediaService 构造服务。replica 仅在 S3 驱动时非 nil。
func NewMediaService(store repository.ProjectMediaStore, backend, replica storage.Backend) *MediaService {
	return &MediaService{store: store, backend: backend, replica: replica}
}

// MediaPutInput 是单文件上传输入。
type MediaPutInput struct {
	FileName    string
	ContentType string
	Body        io.Reader
}

// MediaPutResult 是成功写入后的元数据与占位 URL。
type MediaPutResult struct {
	Media *model.ProjectMedia
	URL   string
}

// Put 校验 MIME/大小，写入 Backend（S3 时再写 replica），并插入元数据行。
func (s *MediaService) Put(ctx context.Context, project *model.Project, in MediaPutInput) (*MediaPutResult, error) {
	if s == nil || s.store == nil {
		return nil, ErrMediaNotFound
	}
	if s.backend == nil {
		return nil, ErrMediaStorageUnavailable
	}
	if project == nil {
		return nil, ErrProjectNotFound
	}

	raw, err := io.ReadAll(io.LimitReader(in.Body, model.MediaMaxBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read media: %w", err)
	}
	if len(raw) == 0 {
		return nil, ErrInvalidRequest("empty file")
	}
	if int64(len(raw)) > model.MediaMaxBytes {
		return nil, ErrInvalidRequest("file exceeds 10 MiB")
	}

	sniffed := http.DetectContentType(raw)
	declared := strings.TrimSpace(in.ContentType)
	if rejectedMediaMIME(declared) || rejectedMediaMIME(sniffed) {
		return nil, ErrInvalidRequest("file type is not allowed")
	}

	contentType := declared
	if base, _, err := mime.ParseMediaType(declared); err == nil {
		contentType = base
	}
	if contentType == "" || contentType == "application/octet-stream" {
		if base, _, err := mime.ParseMediaType(sniffed); err == nil && base != "" && base != "application/octet-stream" {
			contentType = base
		}
	}
	if contentType == "" {
		contentType = "application/octet-stream"
	}

	id, err := uuid.NewRandom()
	if err != nil {
		return nil, err
	}
	fileName := safeMediaFileName(in.FileName)
	storageKey := path.Join("media", project.ID.String(), id.String(), fileName)

	hasher := hashutil.NewMultiHasher(true)
	if _, err := hasher.Write(raw); err != nil {
		return nil, fmt.Errorf("hash media: %w", err)
	}

	if err := s.backend.Put(ctx, storageKey, bytes.NewReader(raw), int64(len(raw)), contentType); err != nil {
		return nil, fmt.Errorf("put media object: %w", err)
	}
	row := &model.ProjectMedia{
		ID:          id,
		ProjectID:   project.ID,
		FileName:    fileName,
		StorageKey:  storageKey,
		Size:        int64(len(raw)),
		ContentType: contentType,
		SHA256:      hasher.SHA256(),
		MD5:         hasher.MD5(),
		SHA512:      hasher.SHA512(),
	}
	if err := s.writeReplica(ctx, row, bytes.NewReader(raw)); err != nil {
		return nil, err
	}
	if err := s.store.Create(ctx, row); err != nil {
		return nil, err
	}
	return &MediaPutResult{Media: row, URL: MediaPublicURL(project.Slug, row.ID)}, nil
}

// GetMeta 按项目与 ID 读取元数据。
func (s *MediaService) GetMeta(ctx context.Context, projectID, id uuid.UUID) (*model.ProjectMedia, error) {
	if s == nil || s.store == nil {
		return nil, ErrMediaNotFound
	}
	row, err := s.store.GetByProjectAndID(ctx, projectID, id)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrMediaNotFound
		}
		return nil, err
	}
	return row, nil
}

// Open 返回对象流。优先 replica；缺失时从 Backend 拉取并补副本。
// start/end 为 HTTP 闭区间；end < 0 表示读到末尾。
func (s *MediaService) Open(ctx context.Context, meta *model.ProjectMedia, start, end int64) (io.ReadCloser, error) {
	if s == nil || s.backend == nil {
		return nil, ErrMediaStorageUnavailable
	}
	if meta == nil {
		return nil, ErrMediaNotFound
	}
	backend, key := s.backend, meta.StorageKey
	if s.replica != nil {
		if err := s.fillReplica(ctx, meta); err != nil && !errors.Is(err, storage.ErrNotFound) {
			return nil, err
		}
		if _, _, err := s.replica.Head(ctx, replicaKey(meta)); err == nil {
			backend, key = s.replica, replicaKey(meta)
		}
	}
	if start == 0 && end < 0 {
		return backend.Get(ctx, key)
	}
	return backend.Range(ctx, key, start, end)
}

func (s *MediaService) writeReplica(ctx context.Context, row *model.ProjectMedia, r io.Reader) error {
	if s.replica == nil {
		return nil
	}
	if err := s.replica.Put(ctx, replicaKey(row), r, row.Size, row.ContentType); err != nil {
		return fmt.Errorf("put media replica: %w", err)
	}
	return nil
}

func (s *MediaService) fillReplica(ctx context.Context, meta *model.ProjectMedia) error {
	if s.replica == nil {
		return nil
	}
	if _, _, err := s.replica.Head(ctx, replicaKey(meta)); err == nil {
		return nil
	}
	rc, err := s.backend.Get(ctx, meta.StorageKey)
	if err != nil {
		return err
	}
	defer rc.Close()
	return s.replica.Put(ctx, replicaKey(meta), rc, meta.Size, meta.ContentType)
}

func replicaKey(row *model.ProjectMedia) string {
	return path.Join("media-cache", row.ProjectID.String(), row.ID.String(), row.FileName)
}

func safeMediaFileName(name string) string {
	name = strings.ReplaceAll(name, "\\", "/")
	name = path.Base(name)
	name = strings.TrimSpace(name)
	if name == "" || name == "." || name == ".." {
		return "file"
	}
	var b strings.Builder
	for _, r := range name {
		switch {
		case r == '/' || r == '\\' || r == 0:
			b.WriteByte('_')
		default:
			b.WriteRune(r)
		}
	}
	out := strings.TrimSpace(b.String())
	if out == "" || out == "." || out == ".." {
		return "file"
	}
	if filepath.IsAbs(out) {
		return "file"
	}
	return out
}

func rejectedMediaMIME(raw string) bool {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return false
	}
	base, _, err := mime.ParseMediaType(raw)
	if err != nil {
		base = strings.ToLower(strings.TrimSpace(strings.Split(raw, ";")[0]))
	} else {
		base = strings.ToLower(base)
	}
	switch base {
	case "text/html", "application/xhtml+xml", "image/svg+xml",
		"application/javascript", "application/x-javascript", "application/ecmascript",
		"text/javascript", "text/ecmascript", "text/jscript":
		return true
	default:
		return false
	}
}
