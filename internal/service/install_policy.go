package service

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/Kirizu-Official/KiriVers/internal/model"
	"github.com/Kirizu-Official/KiriVers/internal/platform"
	"github.com/Kirizu-Official/KiriVers/internal/service/update"
	"github.com/Kirizu-Official/KiriVers/pkg/pathutil"
)

// InstallPolicyEntry 是模板 GET/PUT 中的单条路径策略（不含哈希）。
type InstallPolicyEntry struct {
	Path          string `json:"path"`
	InstallPolicy string `json:"install_policy"`
}

// InstallPolicyReference 是所选渠道上最新非吊销 Version 对应平台 Manifest 的参考勾选源。
type InstallPolicyReference struct {
	Version *string               `json:"version"`
	Status  *string               `json:"status"`
	Entries []model.ManifestEntry `json:"entries"`
}

func overlayInstallPolicy(projectRules, channelRules []model.InstallPolicyRule) map[string]string {
	out := make(map[string]string, len(projectRules)+len(channelRules))
	for _, r := range projectRules {
		if r.Path == "" {
			continue
		}
		out[r.Path] = r.InstallPolicy
	}
	for _, r := range channelRules {
		if r.Path == "" {
			continue
		}
		out[r.Path] = r.InstallPolicy
	}
	return out
}

func entriesFromRules(rules []model.InstallPolicyRule) []InstallPolicyEntry {
	out := make([]InstallPolicyEntry, 0, len(rules))
	for _, r := range rules {
		out = append(out, InstallPolicyEntry{Path: r.Path, InstallPolicy: r.InstallPolicy})
	}
	return out
}

func entriesFromPolicyMap(m map[string]string) []InstallPolicyEntry {
	out := make([]InstallPolicyEntry, 0, len(m))
	for path, policy := range m {
		out = append(out, InstallPolicyEntry{Path: path, InstallPolicy: policy})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out
}

func applyEffectiveInstallPolicy(entries []model.ManifestEntry, effective map[string]string) {
	for i := range entries {
		policy := model.InstallPolicyOverwrite
		if p, ok := effective[entries[i].Path]; ok && p != "" {
			policy = p
		}
		entries[i].InstallPolicy = policy
		entries[i].IntegrityCheck = policy != model.InstallPolicyKeepIfExists
	}
}

func (s *ProjectService) requireMatrixPair(ctx context.Context, projectID uuid.UUID, osRaw, archRaw string) (osSlug, archSlug string, err error) {
	osSlug, archSlug, err = normalizeMatrixOSArch(osRaw, archRaw)
	if err != nil {
		return "", "", ErrInvalidRequest(err.Error())
	}
	_, err = s.store.GetMatrix(ctx, projectID, osSlug, archSlug)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return "", "", ErrInvalidRequest("os/arch is not in the platform matrix")
	}
	if err != nil {
		return "", "", err
	}
	return osSlug, archSlug, nil
}

func (s *ProjectService) listInstallPolicyRules(ctx context.Context, projectID, channelID uuid.UUID, os, arch string) ([]model.InstallPolicyRule, error) {
	if s.installPolicy == nil {
		return []model.InstallPolicyRule{}, nil
	}
	return s.installPolicy.List(ctx, projectID, channelID, os, arch)
}

// EffectiveInstallPolicy 按 D1 叠加：同一 (os, arch) 内渠道覆盖项目；两边都未写则为空（调用方回退 OVERWRITE）。
func (s *ProjectService) EffectiveInstallPolicy(ctx context.Context, projectID, channelID uuid.UUID, osRaw, archRaw string) (map[string]string, error) {
	osSlug := platform.CanonicalOSWrite(osRaw)
	archSlug := platform.CanonicalArch(archRaw)
	projectRules, err := s.listInstallPolicyRules(ctx, projectID, uuid.Nil, osSlug, archSlug)
	if err != nil {
		return nil, err
	}
	var channelRules []model.InstallPolicyRule
	if channelID != uuid.Nil {
		channelRules, err = s.listInstallPolicyRules(ctx, projectID, channelID, osSlug, archSlug)
		if err != nil {
			return nil, err
		}
	}
	return overlayInstallPolicy(projectRules, channelRules), nil
}

// ListProjectInstallPolicy 返回项目作用域该平台的已存 entries。
func (s *ProjectService) ListProjectInstallPolicy(ctx context.Context, projectID uuid.UUID, osRaw, archRaw string) ([]InstallPolicyEntry, error) {
	osSlug, archSlug, err := s.requireMatrixPair(ctx, projectID, osRaw, archRaw)
	if err != nil {
		return nil, err
	}
	rules, err := s.listInstallPolicyRules(ctx, projectID, uuid.Nil, osSlug, archSlug)
	if err != nil {
		return nil, err
	}
	return entriesFromRules(rules), nil
}

// ListChannelInstallPolicy 返回渠道已存 entries 以及 D1 叠加后的 effective。
func (s *ProjectService) ListChannelInstallPolicy(ctx context.Context, projectID uuid.UUID, slug, osRaw, archRaw string) (entries, effective []InstallPolicyEntry, err error) {
	osSlug, archSlug, err := s.requireMatrixPair(ctx, projectID, osRaw, archRaw)
	if err != nil {
		return nil, nil, err
	}
	ch, err := s.store.GetChannel(ctx, projectID, strings.TrimSpace(slug))
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil, ErrChannelNotFound
	}
	if err != nil {
		return nil, nil, err
	}
	rules, err := s.listInstallPolicyRules(ctx, projectID, ch.ID, osSlug, archSlug)
	if err != nil {
		return nil, nil, err
	}
	eff, err := s.EffectiveInstallPolicy(ctx, projectID, ch.ID, osSlug, archSlug)
	if err != nil {
		return nil, nil, err
	}
	return entriesFromRules(rules), entriesFromPolicyMap(eff), nil
}

func (s *ProjectService) replaceInstallPolicy(ctx context.Context, projectID, channelID uuid.UUID, osSlug, archSlug string, in []InstallPolicyEntry) error {
	if s.installPolicy == nil {
		return ErrInvalidRequest("install policy store is not configured")
	}
	if len(in) > model.MaxInstallPolicyRules {
		return ErrInvalidRequest(fmtInstallPolicyCap())
	}
	seen := make(map[string]struct{}, len(in))
	rules := make([]model.InstallPolicyRule, 0, len(in))
	for _, e := range in {
		normPath, err := pathutil.NormalizeAndValidatePath(e.Path)
		if err != nil {
			return err
		}
		if _, dup := seen[normPath]; dup {
			return ErrInvalidRequest("duplicate path: " + normPath)
		}
		seen[normPath] = struct{}{}
		policy := strings.ToUpper(strings.TrimSpace(e.InstallPolicy))
		if policy == "" {
			policy = model.InstallPolicyOverwrite
		}
		if policy != model.InstallPolicyOverwrite && policy != model.InstallPolicyKeepIfExists {
			return ErrInvalidRequest("unsupported install policy " + policy)
		}
		rules = append(rules, model.InstallPolicyRule{
			ProjectID:     projectID,
			ChannelID:     channelID,
			OS:            osSlug,
			Arch:          archSlug,
			Path:          normPath,
			InstallPolicy: policy,
		})
	}
	return s.installPolicy.Replace(ctx, projectID, channelID, osSlug, archSlug, rules)
}

// PutProjectInstallPolicy 整表替换项目作用域该平台的规则。不 InvalidateProject。
func (s *ProjectService) PutProjectInstallPolicy(ctx context.Context, projectID uuid.UUID, osRaw, archRaw string, in []InstallPolicyEntry) ([]InstallPolicyEntry, error) {
	osSlug, archSlug, err := s.requireMatrixPair(ctx, projectID, osRaw, archRaw)
	if err != nil {
		return nil, err
	}
	if err := s.replaceInstallPolicy(ctx, projectID, uuid.Nil, osSlug, archSlug, in); err != nil {
		return nil, err
	}
	return s.ListProjectInstallPolicy(ctx, projectID, osSlug, archSlug)
}

// PutChannelInstallPolicy 整表替换渠道该平台的规则（不改项目规则）。
func (s *ProjectService) PutChannelInstallPolicy(ctx context.Context, projectID uuid.UUID, slug, osRaw, archRaw string, in []InstallPolicyEntry) (entries, effective []InstallPolicyEntry, err error) {
	osSlug, archSlug, err := s.requireMatrixPair(ctx, projectID, osRaw, archRaw)
	if err != nil {
		return nil, nil, err
	}
	ch, err := s.store.GetChannel(ctx, projectID, strings.TrimSpace(slug))
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil, ErrChannelNotFound
	}
	if err != nil {
		return nil, nil, err
	}
	if err := s.replaceInstallPolicy(ctx, projectID, ch.ID, osSlug, archSlug, in); err != nil {
		return nil, nil, err
	}
	return s.ListChannelInstallPolicy(ctx, projectID, ch.Slug, osSlug, archSlug)
}

// LatestChannelPlatformManifest 取该渠道最新非吊销 Version 对应平台 Manifest 作参考勾选（D5）。
func (s *ProjectService) LatestChannelPlatformManifest(ctx context.Context, projectID uuid.UUID, channelSlug, osRaw, archRaw string) (*InstallPolicyReference, error) {
	osSlug, archSlug, err := s.requireMatrixPair(ctx, projectID, osRaw, archRaw)
	if err != nil {
		return nil, err
	}
	slug := strings.TrimSpace(channelSlug)
	if slug == "" {
		slug = model.ChannelStable
	}
	ch, err := s.store.GetChannel(ctx, projectID, slug)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrChannelNotFound
	}
	if err != nil {
		return nil, err
	}
	p, err := s.store.GetByID(ctx, projectID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrProjectNotFound
		}
		return nil, err
	}
	versions, err := s.store.ListVersions(ctx, projectID)
	if err != nil {
		return nil, err
	}
	var latest *model.Version
	for i := range versions {
		v := &versions[i]
		if v.ChannelSlug != ch.Slug || v.Status == model.VersionStatusRevoked {
			continue
		}
		if latest == nil {
			cp := *v
			latest = &cp
			continue
		}
		cmp, ok := update.CompareVersions(p.CompareEngine, v, latest)
		if ok && cmp > 0 {
			cp := *v
			latest = &cp
		}
	}
	empty := &InstallPolicyReference{Entries: []model.ManifestEntry{}}
	if latest == nil {
		return empty, nil
	}
	ver := displayVersion(latest)
	st := latest.Status
	empty.Version = &ver
	empty.Status = &st
	line, err := s.store.GetVersionLine(ctx, latest.ID, osSlug, archSlug)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return empty, nil
	}
	if err != nil {
		return nil, err
	}
	entries, err := s.store.ListManifestEntries(ctx, line.ID)
	if err != nil {
		return nil, err
	}
	if entries == nil {
		entries = []model.ManifestEntry{}
	}
	return &InstallPolicyReference{Version: &ver, Status: &st, Entries: entries}, nil
}

func fmtInstallPolicyCap() string {
	return fmt.Sprintf("at most %d install policy rules per platform", model.MaxInstallPolicyRules)
}

// effectiveForVersion 取 Version 所属渠道与平台的 D1 生效策略；渠道缺失时仅用项目作用域。
func (s *ProjectService) effectiveForVersion(ctx context.Context, projectID uuid.UUID, channelSlug, osRaw, archRaw string) (map[string]string, error) {
	channelID := uuid.Nil
	if slug := strings.TrimSpace(channelSlug); slug != "" {
		ch, err := s.store.GetChannel(ctx, projectID, slug)
		if err == nil {
			channelID = ch.ID
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, err
		}
	}
	return s.EffectiveInstallPolicy(ctx, projectID, channelID, osRaw, archRaw)
}
