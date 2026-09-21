package service

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/google/uuid"

	"github.com/Kirizu-Official/KiriVers/internal/config"
	"github.com/Kirizu-Official/KiriVers/internal/model"
	"github.com/Kirizu-Official/KiriVers/internal/storage"
	"github.com/Kirizu-Official/KiriVers/pkg/hashutil"
)

func (s *ProjectService) resolvedLocalRoot() string {
	if s == nil {
		return ""
	}
	if s.localRoot != "" {
		return s.localRoot
	}
	if fs, ok := s.storage.(*storage.LocalFS); ok {
		return fs.Root()
	}
	return ""
}

func (s *ProjectService) keepLocalReplica() bool {
	return s != nil && s.clusterActive && s.clusterDownload == config.ClusterDownloadLocal
}

// ReplicaHas 判断本机是否已有 {local.root}/{storageKey} 副本。
func (s *ProjectService) ReplicaHas(storageKey string) bool {
	if s == nil || storageKey == "" {
		return false
	}
	root := s.resolvedLocalRoot()
	if root == "" {
		if s.storage == nil {
			return false
		}
		_, _, err := s.storage.Head(context.Background(), storageKey)
		return err == nil
	}
	_, err := os.Stat(filepath.Join(root, filepath.FromSlash(storageKey)))
	return err == nil
}

func (s *ProjectService) localAbs(rel string) string {
	return filepath.Join(s.resolvedLocalRoot(), filepath.FromSlash(rel))
}

func (s *ProjectService) writeLocalTemp(slug string, body io.Reader) (abs string, rel string, hasher *hashutil.MultiHasher, size int64, err error) {
	root := s.resolvedLocalRoot()
	if root == "" {
		return "", "", nil, 0, ErrStorageUnavailable
	}
	id := uuid.New()
	rel = storage.ArtifactTempRel(slug, id)
	abs = s.localAbs(rel)
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		return "", "", nil, 0, err
	}
	f, err := os.OpenFile(abs, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return "", "", nil, 0, err
	}
	hasher = hashutil.NewMultiHasher(true)
	n, err := io.Copy(io.MultiWriter(f, hasher), body)
	cerr := f.Close()
	if err != nil {
		_ = os.Remove(abs)
		return "", "", nil, 0, err
	}
	if cerr != nil {
		_ = os.Remove(abs)
		return "", "", nil, 0, cerr
	}
	return abs, rel, hasher, n, nil
}

func removeLocal(path string) {
	if path != "" {
		_ = os.Remove(path)
	}
}

// promoteLocalFile 把本机 temp 晋升为公共 Backend 的 {slug}/{sha256}；已存在则复用并删 temp。
func (s *ProjectService) promoteLocalFile(ctx context.Context, slug, sha256, contentType, tempAbs string, size int64) (string, error) {
	if s.storage == nil {
		removeLocal(tempAbs)
		return "", ErrStorageUnavailable
	}
	key := storage.ArtifactObjectKey(slug, sha256)
	if _, _, err := s.storage.Head(ctx, key); err == nil {
		removeLocal(tempAbs)
		return key, nil
	} else if err != nil && !errors.Is(err, storage.ErrNotFound) {
		return "", err
	}
	if fs, ok := s.storage.(*storage.LocalFS); ok {
		dest := filepath.Join(fs.Root(), filepath.FromSlash(key))
		if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
			return "", err
		}
		if err := os.Rename(tempAbs, dest); err != nil {
			if copyErr := copyFile(tempAbs, dest); copyErr != nil {
				return "", copyErr
			}
			removeLocal(tempAbs)
		}
		if s.keepLocalReplica() {
			rep := s.localAbs(key)
			if err := os.MkdirAll(filepath.Dir(rep), 0o755); err == nil {
				_ = copyFile(dest, rep)
			}
		}
		return key, nil
	}
	f, err := os.Open(tempAbs)
	if err != nil {
		return "", err
	}
	putErr := s.storage.Put(ctx, key, f, size, contentType)
	_ = f.Close()
	if putErr != nil {
		return "", putErr
	}
	if s.keepLocalReplica() {
		dest := s.localAbs(key)
		if err := os.MkdirAll(filepath.Dir(dest), 0o755); err == nil {
			_ = copyFile(tempAbs, dest)
		}
	}
	removeLocal(tempAbs)
	return key, nil
}

// putCanonicalBytes 对已在内存中的生成物按内容哈希写入 {slug}/{sha256}（无公共桶 temp）。
func (s *ProjectService) putCanonicalBytes(ctx context.Context, slug, contentType string, body []byte) (key, sha256 string, hasher *hashutil.MultiHasher, err error) {
	if s.storage == nil {
		return "", "", nil, ErrStorageUnavailable
	}
	hasher = hashutil.NewMultiHasher(true)
	if _, err := hasher.Write(body); err != nil {
		return "", "", nil, err
	}
	sha256 = hasher.SHA256()
	key = storage.ArtifactObjectKey(slug, sha256)
	if _, _, err := s.storage.Head(ctx, key); err == nil {
		return key, sha256, hasher, nil
	} else if err != nil && !errors.Is(err, storage.ErrNotFound) {
		return "", "", nil, err
	}
	if err := s.storage.Put(ctx, key, bytes.NewReader(body), int64(len(body)), contentType); err != nil {
		return "", "", nil, err
	}
	if s.keepLocalReplica() {
		dest := s.localAbs(key)
		if err := os.MkdirAll(filepath.Dir(dest), 0o755); err == nil {
			_ = os.WriteFile(dest, body, 0o644)
		}
	}
	return key, sha256, hasher, nil
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(out, in)
	cerr := out.Close()
	if copyErr != nil {
		return copyErr
	}
	return cerr
}

func (s *ProjectService) replicaLocalFS() (*storage.LocalFS, bool) {
	if !s.keepLocalReplica() {
		return nil, false
	}
	root := s.resolvedLocalRoot()
	if root == "" {
		return nil, false
	}
	fs, err := storage.NewLocalFS(root)
	if err != nil {
		return nil, false
	}
	return fs, true
}

// OpenStoredObject 读取产物字节：本机代拉优先本地副本，否则公共 Backend。
func (s *ProjectService) OpenStoredObject(ctx context.Context, key string) (io.ReadCloser, error) {
	if fs, ok := s.replicaLocalFS(); ok && s.ReplicaHas(key) {
		return fs.Get(ctx, key)
	}
	if s.storage == nil {
		return nil, ErrStorageUnavailable
	}
	return s.storage.Get(ctx, key)
}

// OpenStoredRange 读取产物区间：本机代拉优先本地副本。
func (s *ProjectService) OpenStoredRange(ctx context.Context, key string, start, end int64) (io.ReadCloser, error) {
	if fs, ok := s.replicaLocalFS(); ok && s.ReplicaHas(key) {
		return fs.Range(ctx, key, start, end)
	}
	if s.storage == nil {
		return nil, ErrStorageUnavailable
	}
	return s.storage.Range(ctx, key, start, end)
}

func (s *ProjectService) pullPublicToLocal(ctx context.Context, key string) error {
	root := s.resolvedLocalRoot()
	if root == "" || s.storage == nil || key == "" {
		return ErrStorageUnavailable
	}
	dest := filepath.Join(root, filepath.FromSlash(key))
	if _, err := os.Stat(dest); err == nil {
		return nil
	}
	rc, err := s.storage.Get(ctx, key)
	if err != nil {
		return err
	}
	defer rc.Close()
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return err
	}
	tmp := dest + ".part"
	f, err := os.OpenFile(tmp, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(f, rc)
	cerr := f.Close()
	if copyErr != nil {
		_ = os.Remove(tmp)
		return copyErr
	}
	if cerr != nil {
		_ = os.Remove(tmp)
		return cerr
	}
	return os.Rename(tmp, dest)
}

func (s *ProjectService) markLineSync(ctx context.Context, nodeID, projectID, versionID uuid.UUID, lineID *uuid.UUID, status string, done, total int64, errMsg string) {
	if s == nil || s.nodes == nil || nodeID == uuid.Nil {
		return
	}
	row := &model.NodeArtifactSync{
		NodeID:       nodeID,
		ProjectID:    projectID,
		VersionID:    versionID,
		LineID:       lineID,
		Status:       status,
		BytesDone:    done,
		BytesTotal:   total,
		ErrorMessage: errMsg,
	}
	_ = s.nodes.UpsertSync(ctx, row)
}

func (s *ProjectService) syncLineAfterReady(ctx context.Context, p *model.Project, versionID, lineID uuid.UUID, keys []string, sizes []int64) {
	if s == nil || p == nil {
		return
	}
	nodeID := s.nodeID
	var total int64
	for _, n := range sizes {
		total += n
	}
	linePtr := &lineID
	if !s.clusterActive {
		return
	}
	if s.clusterDownload != config.ClusterDownloadLocal {
		s.markLineSync(ctx, nodeID, p.ID, versionID, linePtr, model.NodeSyncStatusReady, total, total, "")
		return
	}
	s.markLineSync(ctx, nodeID, p.ID, versionID, linePtr, model.NodeSyncStatusSyncing, 0, total, "")
	var done int64
	for i, key := range keys {
		if err := s.pullPublicToLocal(ctx, key); err != nil {
			s.markLineSync(ctx, nodeID, p.ID, versionID, linePtr, model.NodeSyncStatusError, done, total, err.Error())
			return
		}
		if i < len(sizes) {
			done += sizes[i]
		}
		s.markLineSync(ctx, nodeID, p.ID, versionID, linePtr, model.NodeSyncStatusSyncing, done, total, "")
	}
	s.markLineSync(ctx, nodeID, p.ID, versionID, linePtr, model.NodeSyncStatusReady, total, total, "")
}

func (s *ProjectService) tusTempAbs(sessStorageKey string) string {
	return s.localAbs(sessStorageKey)
}

func (s *ProjectService) ensureTusTempFile(rel string) error {
	abs := s.localAbs(rel)
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(abs, os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	return f.Close()
}

func fmtErr(msg string, err error) error {
	return fmt.Errorf("%s: %w", msg, err)
}
