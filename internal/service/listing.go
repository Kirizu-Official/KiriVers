package service

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/Kirizu-Official/KiriVers/internal/model"
	"github.com/Kirizu-Official/KiriVers/internal/platform"
)

// knownStoreProtocols 与 feed.Adapter.Protocol() 对齐（不是 UI 旧名 electron-updater）。
var knownStoreProtocols = map[string]struct{}{
	"sparkle": {}, "electron": {}, "tauri": {}, "squirrel": {},
	"clickonce": {}, "appimage": {}, "winget": {}, "msix": {}, "fdroid": {},
}

// StoreListingWrite 是创建/更新商店 listing 的输入（指针字段表示 PATCH 是否写入）。
type StoreListingWrite struct {
	Protocol      *string
	Slug          *string
	Enabled       *bool
	OS            *string
	Arch          *string
	Channel       *string
	ClearOS       bool
	ClearArch     bool
	ClearChannel  bool
	Identifiers   *model.IdentifierMap
	PackageSource *string
	ManifestPath  *string
}

// ListStoreListings 列出项目全部商店上架记录（protocol、slug 升序）。
func (s *ProjectService) ListStoreListings(ctx context.Context, projectID uuid.UUID) ([]model.StoreListing, error) {
	return s.store.ListStoreListings(ctx, projectID)
}

// GetStoreListing 按 protocol+slug 取一条 listing（含停用行）。
func (s *ProjectService) GetStoreListing(ctx context.Context, projectID uuid.UUID, protocol, slug string) (*model.StoreListing, error) {
	row, err := s.store.GetStoreListing(ctx, projectID, strings.ToLower(strings.TrimSpace(protocol)), strings.TrimSpace(slug))
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrStoreListingNotFound
	}
	return row, err
}

// GetEnabledStoreListing 供 feed HTTP：不存在或停用视为未找到。
func (s *ProjectService) GetEnabledStoreListing(ctx context.Context, projectID uuid.UUID, protocol, slug string) (*model.StoreListing, error) {
	row, err := s.GetStoreListing(ctx, projectID, protocol, slug)
	if err != nil {
		return nil, err
	}
	if row == nil || !row.Enabled {
		return nil, ErrStoreListingNotFound
	}
	return row, nil
}

// CreateStoreListing 写入一条商店上架。重复 (protocol, slug) → 400。
func (s *ProjectService) CreateStoreListing(ctx context.Context, projectID uuid.UUID, in StoreListingWrite) (*model.StoreListing, error) {
	if in.Protocol == nil || strings.TrimSpace(*in.Protocol) == "" {
		return nil, fmt.Errorf("%w: protocol is required", ErrInvalidStoreListing)
	}
	if in.Slug == nil {
		return nil, fmt.Errorf("%w: slug is required", ErrInvalidStoreListing)
	}
	protocol, err := normalizeStoreProtocol(*in.Protocol)
	if err != nil {
		return nil, err
	}
	slug, err := normalizeChannelSlug(*in.Slug)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidStoreListing, err)
	}
	row := &model.StoreListing{
		ProjectID:     projectID,
		Protocol:      protocol,
		Slug:          slug,
		Enabled:       true,
		Identifiers:   model.IdentifierMap{},
		PackageSource: model.PackageSourceLineFull,
	}
	if err := applyStoreListingWrite(row, in, true); err != nil {
		return nil, err
	}
	if err := s.validateListingPins(ctx, projectID, row); err != nil {
		return nil, err
	}
	if err := s.store.CreateStoreListing(ctx, row); err != nil {
		if isUniqueViolation(err) {
			return nil, ErrStoreListingTaken
		}
		return nil, err
	}
	s.invalidateProject(ctx, projectID)
	return row, nil
}

// PatchStoreListing 更新 listing。slug / protocol 不可改。
func (s *ProjectService) PatchStoreListing(ctx context.Context, projectID, id uuid.UUID, in StoreListingWrite) (*model.StoreListing, error) {
	row, err := s.store.GetStoreListingByID(ctx, projectID, id)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrStoreListingNotFound
	}
	if err != nil {
		return nil, err
	}
	if err := applyStoreListingWrite(row, in, false); err != nil {
		return nil, err
	}
	if err := s.validateListingPins(ctx, projectID, row); err != nil {
		return nil, err
	}
	if err := s.store.SaveStoreListing(ctx, row); err != nil {
		if isUniqueViolation(err) {
			return nil, ErrStoreListingTaken
		}
		return nil, err
	}
	s.invalidateProject(ctx, projectID)
	return row, nil
}

// DeleteStoreListing 删除一条上架。
func (s *ProjectService) DeleteStoreListing(ctx context.Context, projectID, id uuid.UUID) error {
	err := s.store.DeleteStoreListing(ctx, projectID, id)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return ErrStoreListingNotFound
	}
	if err != nil {
		return err
	}
	s.invalidateProject(ctx, projectID)
	return nil
}

func normalizeStoreProtocol(raw string) (string, error) {
	p := strings.ToLower(strings.TrimSpace(raw))
	if _, ok := knownStoreProtocols[p]; !ok {
		return "", fmt.Errorf("%w: unknown protocol %q", ErrInvalidStoreListing, raw)
	}
	return p, nil
}

func applyStoreListingWrite(row *model.StoreListing, in StoreListingWrite, creating bool) error {
	if in.Enabled != nil {
		row.Enabled = *in.Enabled
	}
	if in.Identifiers != nil {
		row.Identifiers = cloneIdentifierMap(*in.Identifiers)
	} else if creating && row.Identifiers == nil {
		row.Identifiers = model.IdentifierMap{}
	}
	if in.PackageSource != nil {
		src := strings.ToLower(strings.TrimSpace(*in.PackageSource))
		switch src {
		case model.PackageSourceLineFull, model.PackageSourceManifestPath:
			row.PackageSource = src
		default:
			return fmt.Errorf("%w: package_source must be line_full or manifest_path", ErrInvalidStoreListing)
		}
	}
	if in.ManifestPath != nil {
		row.ManifestPath = strings.TrimSpace(*in.ManifestPath)
	}
	if creating && row.PackageSource == "" {
		row.PackageSource = model.PackageSourceLineFull
	}
	if row.PackageSource == model.PackageSourceManifestPath && strings.TrimSpace(row.ManifestPath) == "" {
		return fmt.Errorf("%w: manifest_path is required when package_source is manifest_path", ErrInvalidStoreListing)
	}

	if in.ClearOS {
		row.OS = nil
	} else if in.OS != nil {
		osName := strings.TrimSpace(*in.OS)
		if osName == "" {
			row.OS = nil
		} else {
			canon := platform.CanonicalOSWrite(osName)
			row.OS = &canon
		}
	}
	if in.ClearArch {
		row.Arch = nil
	} else if in.Arch != nil {
		arch := strings.TrimSpace(*in.Arch)
		if arch == "" {
			row.Arch = nil
		} else {
			canon := platform.CanonicalArch(arch)
			row.Arch = &canon
		}
	}
	if in.ClearChannel {
		row.Channel = nil
	} else if in.Channel != nil {
		ch := strings.TrimSpace(*in.Channel)
		if ch == "" {
			row.Channel = nil
		} else {
			slug, err := normalizeChannelSlug(ch)
			if err != nil {
				return fmt.Errorf("%w: channel: %v", ErrInvalidStoreListing, err)
			}
			row.Channel = &slug
		}
	}
	return nil
}

func (s *ProjectService) validateListingPins(ctx context.Context, projectID uuid.UUID, row *model.StoreListing) error {
	if row.Channel != nil && *row.Channel != "" {
		if _, err := s.store.GetChannel(ctx, projectID, *row.Channel); err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return fmt.Errorf("%w: unknown channel %q", ErrInvalidStoreListing, *row.Channel)
			}
			return err
		}
	}
	osName, arch := "", ""
	if row.OS != nil {
		osName = *row.OS
	}
	if row.Arch != nil {
		arch = *row.Arch
	}
	if osName != "" && arch != "" {
		if _, err := s.store.GetMatrix(ctx, projectID, osName, arch); err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return fmt.Errorf("%w: os/arch pin is not on the platform matrix", ErrInvalidStoreListing)
			}
			return err
		}
	} else if osName != "" || arch != "" {
		// 只钉死一维时仍要求该维出现在矩阵中（任一配对）。
		rows, err := s.store.ListMatrix(ctx, projectID)
		if err != nil {
			return err
		}
		found := false
		for i := range rows {
			if osName != "" && rows[i].OS != osName {
				continue
			}
			if arch != "" && rows[i].Arch != arch {
				continue
			}
			found = true
			break
		}
		if !found {
			return fmt.Errorf("%w: os/arch pin is not on the platform matrix", ErrInvalidStoreListing)
		}
	}
	return nil
}

func cloneIdentifierMap(in model.IdentifierMap) model.IdentifierMap {
	out := model.IdentifierMap{}
	for k, v := range in {
		out[k] = v
	}
	return out
}
