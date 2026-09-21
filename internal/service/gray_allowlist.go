package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/Kirizu-Official/KiriVers/internal/model"
	"github.com/Kirizu-Official/KiriVers/internal/platform"
)

const (
	// maxAllowlistDeviceIDLen 白名单条目（原始 device_id）最大长度。
	maxAllowlistDeviceIDLen = 128
)

var (
	// ErrAllowlistPolicyNone 项目 device_id 策略为 none：拒绝白名单写入 → HTTP 400。
	ErrAllowlistPolicyNone = errors.New("device_id policy 'none' does not support gray allowlist")
	// ErrInvalidAllowlistEntry 白名单条目非法（空串或超长）→ HTTP 400。
	ErrInvalidAllowlistEntry = errors.New("invalid allowlist device entry")
	// ErrInvalidRolloutPercent 灰度百分比越界（合法 0–100）→ HTTP 400。
	ErrInvalidRolloutPercent = errors.New("gray percent must be between 0 and 100")
)

// AddGrayAllowlistByClientIDs 从名册 UUID 勾选加入版本白名单（立即生效）。
func (s *ProjectService) AddGrayAllowlistByClientIDs(ctx context.Context, projectID uuid.UUID, versionRef string, clientIDs []uuid.UUID) ([]model.GrayAllowlist, error) {
	if len(clientIDs) == 0 {
		return nil, fmt.Errorf("%w: client_ids is required", ErrInvalidAllowlistEntry)
	}
	proj, err := s.store.GetByID(ctx, projectID)
	if err != nil {
		return nil, err
	}
	if proj.DeviceIDPolicy == model.DeviceIDPolicyNone {
		return nil, ErrAllowlistPolicyNone
	}
	v, err := s.ResolveVersion(ctx, projectID, versionRef)
	if err != nil {
		return nil, err
	}
	clients, err := s.store.ListClientsByIDs(ctx, projectID, clientIDs)
	if err != nil {
		return nil, err
	}
	if len(clients) == 0 {
		return nil, fmt.Errorf("%w: no matching clients", ErrInvalidAllowlistEntry)
	}
	now := time.Now().UTC()
	entries := make([]model.GrayAllowlist, 0, len(clients))
	seen := map[string]bool{}
	for i := range clients {
		h := clients[i].DeviceHash
		if h == "" || seen[h] {
			continue
		}
		seen[h] = true
		entries = append(entries, model.GrayAllowlist{
			ProjectID: projectID,
			VersionID: v.ID,
			DeviceID:  h,
			Source:    model.GraySourceManual,
			CreatedAt: now,
		})
	}
	if len(entries) == 0 {
		return nil, fmt.Errorf("%w: no matching clients", ErrInvalidAllowlistEntry)
	}
	if err := s.store.InsertAllowlist(ctx, entries); err != nil {
		return nil, err
	}
	if v.GrayIsActive() {
		if _, err := s.EnsureGrayAdmission(ctx, proj, v); err != nil {
			return nil, err
		}
	} else {
		s.invalidateProject(ctx, projectID)
	}
	return s.store.ListAllowlist(ctx, projectID, v.ID)
}

// DeleteGrayAllowlistByClientIDs 按名册 UUID 删除白名单（幂等）。
func (s *ProjectService) DeleteGrayAllowlistByClientIDs(ctx context.Context, projectID uuid.UUID, versionRef string, clientIDs []uuid.UUID) (int64, []model.GrayAllowlist, error) {
	if len(clientIDs) == 0 {
		return 0, nil, fmt.Errorf("%w: client_ids is required", ErrInvalidAllowlistEntry)
	}
	v, err := s.ResolveVersion(ctx, projectID, versionRef)
	if err != nil {
		return 0, nil, err
	}
	clients, err := s.store.ListClientsByIDs(ctx, projectID, clientIDs)
	if err != nil {
		return 0, nil, err
	}
	stored := make([]string, 0, len(clients))
	for i := range clients {
		if clients[i].DeviceHash != "" {
			stored = append(stored, clients[i].DeviceHash)
		}
	}
	removed, err := s.store.DeleteAllowlist(ctx, projectID, v.ID, stored)
	if err != nil {
		return 0, nil, err
	}
	s.invalidateProject(ctx, projectID)
	remaining, err := s.store.ListAllowlist(ctx, projectID, v.ID)
	if err != nil {
		return 0, nil, err
	}
	return removed, remaining, nil
}

// ListVersionGrayAllowlist 列出 Version 级白名单条目。
func (s *ProjectService) ListVersionGrayAllowlist(ctx context.Context, projectID uuid.UUID, versionRef string) ([]model.GrayAllowlist, error) {
	v, err := s.ResolveVersion(ctx, projectID, versionRef)
	if err != nil {
		return nil, err
	}
	return s.store.ListAllowlist(ctx, projectID, v.ID)
}

// AddVersionGrayAllowlist 按原始 device_id 写入（测试与兼容内部调用）。
func (s *ProjectService) AddVersionGrayAllowlist(ctx context.Context, projectID uuid.UUID, versionRef string, rawIDs []string) ([]model.GrayAllowlist, error) {
	if len(rawIDs) == 0 {
		return nil, fmt.Errorf("%w: device_id list is required", ErrInvalidAllowlistEntry)
	}
	proj, err := s.store.GetByID(ctx, projectID)
	if err != nil {
		return nil, err
	}
	v, err := s.ResolveVersion(ctx, projectID, versionRef)
	if err != nil {
		return nil, err
	}
	stored, err := prepareAllowlistEntries(proj, rawIDs)
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	entries := make([]model.GrayAllowlist, 0, len(stored))
	for _, id := range stored {
		entries = append(entries, model.GrayAllowlist{
			ProjectID: projectID,
			VersionID: v.ID,
			DeviceID:  id,
			Source:    model.GraySourceManual,
			CreatedAt: now,
		})
	}
	if err := s.store.InsertAllowlist(ctx, entries); err != nil {
		return nil, err
	}
	s.invalidateProject(ctx, projectID)
	return s.store.ListAllowlist(ctx, projectID, v.ID)
}

// DeleteVersionGrayAllowlist 按原始 device_id 删除。
func (s *ProjectService) DeleteVersionGrayAllowlist(ctx context.Context, projectID uuid.UUID, versionRef string, rawIDs []string) (int64, []model.GrayAllowlist, error) {
	if len(rawIDs) == 0 {
		return 0, nil, fmt.Errorf("%w: device_id list is required", ErrInvalidAllowlistEntry)
	}
	proj, err := s.store.GetByID(ctx, projectID)
	if err != nil {
		return 0, nil, err
	}
	v, err := s.ResolveVersion(ctx, projectID, versionRef)
	if err != nil {
		return 0, nil, err
	}
	stored, err := prepareAllowlistEntries(proj, rawIDs)
	if err != nil {
		return 0, nil, err
	}
	removed, err := s.store.DeleteAllowlist(ctx, projectID, v.ID, stored)
	if err != nil {
		return 0, nil, err
	}
	s.invalidateProject(ctx, projectID)
	remaining, err := s.store.ListAllowlist(ctx, projectID, v.ID)
	if err != nil {
		return 0, nil, err
	}
	return removed, remaining, nil
}

func prepareAllowlistEntries(project *model.Project, rawIDs []string) ([]string, error) {
	switch project.DeviceIDPolicy {
	case model.DeviceIDPolicyNone:
		return nil, ErrAllowlistPolicyNone
	case model.DeviceIDPolicyRaw:
		seen := make(map[string]bool, len(rawIDs))
		out := make([]string, 0, len(rawIDs))
		for _, raw := range rawIDs {
			id := strings.TrimSpace(raw)
			if id == "" || utf8.RuneCountInString(id) > maxAllowlistDeviceIDLen {
				return nil, fmt.Errorf("%w: device entry length must be 1..%d", ErrInvalidAllowlistEntry, maxAllowlistDeviceIDLen)
			}
			if seen[id] {
				continue
			}
			seen[id] = true
			out = append(out, id)
		}
		return out, nil
	default:
		seen := make(map[string]bool, len(rawIDs))
		out := make([]string, 0, len(rawIDs))
		for _, raw := range rawIDs {
			id := strings.TrimSpace(raw)
			if id == "" || utf8.RuneCountInString(id) > maxAllowlistDeviceIDLen {
				return nil, fmt.Errorf("%w: device entry length must be 1..%d", ErrInvalidAllowlistEntry, maxAllowlistDeviceIDLen)
			}
			stored := HashDeviceID(project, id)
			if seen[stored] {
				continue
			}
			seen[stored] = true
			out = append(out, stored)
		}
		return out, nil
	}
}

// PatchVersionLine 更新线的 min_os / min_api_level（不再接受灰度覆盖）。
func (s *ProjectService) PatchVersionLine(ctx context.Context, projectID uuid.UUID, versionRef, os, arch string, in VersionLinePatchInput) (*model.VersionLine, error) {
	v, err := s.ResolveVersion(ctx, projectID, versionRef)
	if err != nil {
		return nil, err
	}
	line, err := s.store.GetVersionLine(ctx, v.ID, platform.CanonicalOSWrite(os), platform.CanonicalArch(arch))
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrVersionLineNotFound
		}
		return nil, err
	}
	if in.MinOSSet {
		minOS, err := normalizeLineMinOS(in.MinOS)
		if err != nil {
			return nil, err
		}
		line.MinOS = minOS
	}
	if in.MinAPISet {
		minAPI, err := normalizeLineMinAPILevel(in.MinAPILevel)
		if err != nil {
			return nil, err
		}
		line.MinAPILevel = minAPI
	}
	if err := s.store.SaveVersionLine(ctx, line); err != nil {
		return nil, err
	}
	s.invalidateProject(ctx, projectID)
	return line, nil
}
