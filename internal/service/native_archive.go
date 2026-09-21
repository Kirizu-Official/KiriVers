package service

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/Kirizu-Official/KiriVers/internal/model"
)

// persistHashRootAndStoreFull 把路径 zip 拆成原生哈希根目录 full + 带路径 store_full。
// existing 非空时就地改写该 kind=full 行（上传闸门路径）；否则新建。
func (s *ProjectService) persistHashRootAndStoreFull(ctx context.Context, p *model.Project, v *model.Version, line *model.VersionLine, hw *string, pathZip []byte, entries []ParsedZipEntry, existing *model.Artifact) (*model.Artifact, error) {
	members := make([]ArchiveMember, 0, len(entries))
	for _, e := range entries {
		members = append(members, ArchiveMember{Path: e.NormalizedPath, SHA256: e.SHA256, Body: e.Content})
	}
	hashZip, err := BuildHashRootZip(members)
	if err != nil {
		return nil, err
	}
	if len(pathZip) == 0 {
		pathZip, err = BuildPathZip(members)
		if err != nil {
			return nil, err
		}
	}

	fullArt, err := s.putArchiveArtifact(ctx, p, v, line, hw, existing, model.ArtifactKindFull, BuildStableFilename(p.Slug, v, line.OS, line.Arch, hw, ".zip"), hashZip)
	if err != nil {
		return nil, err
	}

	feedName := BuildStableFilename(p.Slug, v, line.OS, line.Arch, hw, "") + "-store.zip"
	var feedExisting *model.Artifact
	feeds, err := s.store.ListArtifactsByLineAndKind(ctx, line.ID, model.ArtifactKindStoreFull)
	if err != nil {
		return nil, err
	}
	wantHw := artifactHwLabel(hw)
	for i := range feeds {
		if artifactHwLabel(feeds[i].HwRev) == wantHw {
			feedExisting = &feeds[i]
			break
		}
	}
	if _, err := s.putArchiveArtifact(ctx, p, v, line, hw, feedExisting, model.ArtifactKindStoreFull, feedName, pathZip); err != nil {
		return nil, err
	}
	return fullArt, nil
}

func (s *ProjectService) putArchiveArtifact(ctx context.Context, p *model.Project, v *model.Version, line *model.VersionLine, hw *string, existing *model.Artifact, kind, fileName string, body []byte) (*model.Artifact, error) {
	id := uuid.New()
	if existing != nil && existing.ID != uuid.Nil {
		id = existing.ID
	}
	key, _, hasher, err := s.putCanonicalBytes(ctx, p.Slug, "application/zip", body)
	if err != nil {
		return nil, fmt.Errorf("write %s archive: %w", kind, err)
	}
	art := existing
	if art == nil {
		art = &model.Artifact{
			ID:            id,
			ProjectID:     p.ID,
			VersionID:     v.ID,
			VersionLineID: line.ID,
		}
	}
	oldKey := art.StorageKey
	art.Kind = kind
	art.FileName = fileName
	art.StorageKey = key
	art.Size = int64(len(body))
	art.SHA256 = hasher.SHA256()
	art.MD5 = hasher.MD5()
	art.SHA512 = hasher.SHA512()
	art.ContentType = "application/zip"
	art.Compression = model.ArtifactCompressionZip
	art.HwRev = hw
	if existing != nil && existing.ID != uuid.Nil {
		if err := s.store.SaveArtifact(ctx, art); err != nil {
			_ = s.storage.Delete(ctx, key)
			return nil, err
		}
		if oldKey != "" && oldKey != key {
			_ = s.storage.Delete(ctx, oldKey)
		}
		return art, nil
	}
	if err := s.store.CreateArtifact(ctx, art); err != nil {
		_ = s.storage.Delete(ctx, key)
		return nil, err
	}
	return art, nil
}
