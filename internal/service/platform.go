package service

import (
	"context"
	"crypto/hmac"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/Kirizu-Official/KiriVers/internal/model"
	"github.com/Kirizu-Official/KiriVers/internal/platform"
)

// 自定义渠道 slug：小写字母数字与连字符，3–64，不得含 `_`（SemVer 预发布段）。
var channelSlugRE = regexp.MustCompile(`^[a-z0-9-]{3,64}$`)

// ChannelWrite 是创建/PATCH 渠道的输入；指针表示本次是否写入。
// Token：nil=保持，指向空串=清除，非空=SHA-256 替换。永不记录明文。
type ChannelWrite struct {
	Name          *string
	Slug          *string
	StabilityRank *int
	Enabled       *bool
	Unlisted      *bool
	Token         *string
}

// MatrixWrite 是创建/PATCH 平台矩阵的输入。
type MatrixWrite struct {
	OS                      *string
	Arch                    *string
	PackageType             *string
	DeltaAlgo               *string
	DeltaSourceCount        *int
	HwVariantPolicy         *string
	FallbackArch            *string
	MinimumSupportedVersion *string
}

// HwRevWrite 是创建/PATCH 硬件代号的输入。
type HwRevWrite struct {
	Slug  *string
	Rank  *int
	Notes *string
}

// ListChannels 按 stability_rank 升序返回项目渠道。
func (s *ProjectService) ListChannels(ctx context.Context, projectID uuid.UUID) ([]model.Channel, error) {
	return s.store.ListChannels(ctx, projectID)
}

// CreateChannel 创建自定义渠道。系统名已由项目创建时播种，重复 slug → ErrSlugTaken。
func (s *ProjectService) CreateChannel(ctx context.Context, projectID uuid.UUID, in ChannelWrite) (*model.Channel, error) {
	if in.Slug == nil {
		return nil, fmt.Errorf("%w: slug is required", ErrInvalidChannel)
	}
	slug, err := normalizeChannelSlug(*in.Slug)
	if err != nil {
		return nil, err
	}
	if model.IsSystemChannelSlug(slug) {
		// 系统名由项目创建时播种；禁止再建成可删除的自定义行。
		return nil, ErrSlugTaken
	}
	name, err := normalizeChannelName(in.Name, true)
	if err != nil {
		return nil, err
	}
	ch := &model.Channel{
		ProjectID:     projectID,
		Name:          name,
		Slug:          slug,
		StabilityRank: 0,
		Enabled:       true,
		System:        false,
	}
	if in.StabilityRank != nil {
		ch.StabilityRank = *in.StabilityRank
	}
	if in.Enabled != nil {
		ch.Enabled = *in.Enabled
	}
	if in.Unlisted != nil {
		ch.Unlisted = *in.Unlisted
	}
	if err := applyChannelToken(ch, in.Token); err != nil {
		return nil, err
	}
	if err := s.store.CreateChannel(ctx, ch); err != nil {
		if isUniqueViolation(err) {
			return nil, ErrSlugTaken
		}
		return nil, err
	}
	s.invalidateProject(ctx, projectID)
	return ch, nil
}

// PatchChannel 更新 enabled / stability_rank。不可改 slug 与 system。
func (s *ProjectService) PatchChannel(ctx context.Context, projectID uuid.UUID, slug string, in ChannelWrite) (*model.Channel, error) {
	slug, err := normalizeChannelSlug(slug)
	if err != nil {
		return nil, err
	}
	ch, err := s.store.GetChannel(ctx, projectID, slug)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrChannelNotFound
	}
	if err != nil {
		return nil, err
	}
	if in.StabilityRank != nil {
		ch.StabilityRank = *in.StabilityRank
	}
	if in.Enabled != nil {
		ch.Enabled = *in.Enabled
	}
	if in.Name != nil {
		name, err := normalizeChannelName(in.Name, true)
		if err != nil {
			return nil, err
		}
		ch.Name = name
	}
	if in.Unlisted != nil {
		ch.Unlisted = *in.Unlisted
	}
	if err := applyChannelToken(ch, in.Token); err != nil {
		return nil, err
	}
	if err := s.store.SaveChannel(ctx, ch); err != nil {
		return nil, err
	}
	s.invalidateProject(ctx, projectID)
	return ch, nil
}

// DeleteChannel 删除自定义渠道。系统渠道返回 ErrSystemChannel。
func (s *ProjectService) DeleteChannel(ctx context.Context, projectID uuid.UUID, slug string) error {
	slug, err := normalizeChannelSlug(slug)
	if err != nil {
		return err
	}
	ch, err := s.store.GetChannel(ctx, projectID, slug)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return ErrChannelNotFound
	}
	if err != nil {
		return err
	}
	if ch.System || model.IsSystemChannelSlug(ch.Slug) {
		return ErrSystemChannel
	}
	if s.installPolicy != nil {
		if err := s.installPolicy.DeleteByChannel(ctx, projectID, ch.ID); err != nil {
			return err
		}
	}
	if err := s.store.DeleteChannel(ctx, projectID, slug); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrChannelNotFound
		}
		return err
	}
	s.invalidateProject(ctx, projectID)
	return nil
}

// ListMatrix 返回项目平台矩阵行。
func (s *ProjectService) ListMatrix(ctx context.Context, projectID uuid.UUID) ([]model.PlatformMatrix, error) {
	return s.store.ListMatrix(ctx, projectID)
}

// CreateMatrix 写入一行；os/arch 存规范名（darwin→macos，ipados 原样以启用独立行）。
func (s *ProjectService) CreateMatrix(ctx context.Context, projectID uuid.UUID, in MatrixWrite) (*model.PlatformMatrix, error) {
	if in.OS == nil || in.Arch == nil {
		return nil, fmt.Errorf("%w: os and arch are required", ErrInvalidMatrix)
	}
	osSlug, archSlug, err := normalizeMatrixOSArch(*in.OS, *in.Arch)
	if err != nil {
		return nil, err
	}
	if in.PackageType == nil {
		return nil, fmt.Errorf("%w: package_type is required", ErrInvalidMatrix)
	}
	pkg, err := normalizePackageType(*in.PackageType)
	if err != nil {
		return nil, err
	}
	row := &model.PlatformMatrix{
		ProjectID:        projectID,
		OS:               osSlug,
		Arch:             archSlug,
		PackageType:      pkg,
		DeltaAlgo:        model.DeltaAlgoHDiffPatch,
		DeltaSourceCount: model.DefaultDeltaSourceCount,
		HwVariantPolicy:  model.HwVariantIndependent,
	}
	if err := applyMatrixWrite(row, in, true); err != nil {
		return nil, err
	}
	if err := s.store.CreateMatrix(ctx, row); err != nil {
		if isUniqueViolation(err) {
			return nil, ErrMatrixExists
		}
		return nil, err
	}
	s.invalidateProject(ctx, projectID)
	return row, nil
}

// PatchMatrix 按规范 (os,arch) 更新。改 package_type 时若已有 Published Version Line 则锁定。
func (s *ProjectService) PatchMatrix(ctx context.Context, projectID uuid.UUID, osRaw, archRaw string, in MatrixWrite) (*model.PlatformMatrix, error) {
	osSlug, archSlug, err := normalizeMatrixOSArch(osRaw, archRaw)
	if err != nil {
		return nil, err
	}
	row, err := s.store.GetMatrix(ctx, projectID, osSlug, archSlug)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrMatrixNotFound
	}
	if err != nil {
		return nil, err
	}
	if in.PackageType != nil {
		pkg, err := normalizePackageType(*in.PackageType)
		if err != nil {
			return nil, err
		}
		if pkg != row.PackageType {
			locked, err := s.store.HasPublishedVersionLine(ctx, projectID, row.OS, row.Arch)
			if err != nil {
				return nil, err
			}
			if locked {
				return nil, ErrPackageTypeImmutable
			}
		}
	}
	if err := applyMatrixWrite(row, in, false); err != nil {
		return nil, err
	}
	if err := s.store.SaveMatrix(ctx, row); err != nil {
		return nil, err
	}
	s.invalidateProject(ctx, projectID)
	return row, nil
}

// ListHwRevs 按 rank 升序返回硬件代号。
func (s *ProjectService) ListHwRevs(ctx context.Context, projectID uuid.UUID) ([]model.HwRev, error) {
	return s.store.ListHwRevs(ctx, projectID)
}

// CreateHwRev 登记一条硬件代号。
func (s *ProjectService) CreateHwRev(ctx context.Context, projectID uuid.UUID, in HwRevWrite) (*model.HwRev, error) {
	if in.Slug == nil {
		return nil, fmt.Errorf("%w: slug is required", ErrInvalidHwRev)
	}
	slug, err := normalizeHwRevSlug(*in.Slug)
	if err != nil {
		return nil, err
	}
	hw := &model.HwRev{
		ProjectID: projectID,
		Slug:      slug,
	}
	if in.Rank != nil {
		hw.Rank = *in.Rank
	}
	if in.Notes != nil {
		hw.Notes = strings.TrimSpace(*in.Notes)
	}
	if err := s.store.CreateHwRev(ctx, hw); err != nil {
		if isUniqueViolation(err) {
			return nil, ErrSlugTaken
		}
		return nil, err
	}
	s.invalidateProject(ctx, projectID)
	return hw, nil
}

// PatchHwRev 更新 rank / notes。
func (s *ProjectService) PatchHwRev(ctx context.Context, projectID uuid.UUID, slug string, in HwRevWrite) (*model.HwRev, error) {
	slug, err := normalizeHwRevSlug(slug)
	if err != nil {
		return nil, err
	}
	hw, err := s.store.GetHwRev(ctx, projectID, slug)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrHwRevNotFound
	}
	if err != nil {
		return nil, err
	}
	if in.Rank != nil {
		hw.Rank = *in.Rank
	}
	if in.Notes != nil {
		hw.Notes = strings.TrimSpace(*in.Notes)
	}
	if err := s.store.SaveHwRev(ctx, hw); err != nil {
		return nil, err
	}
	s.invalidateProject(ctx, projectID)
	return hw, nil
}

// DeleteHwRev 删除一条硬件代号。
func (s *ProjectService) DeleteHwRev(ctx context.Context, projectID uuid.UUID, slug string) error {
	slug, err := normalizeHwRevSlug(slug)
	if err != nil {
		return err
	}
	if err := s.store.DeleteHwRev(ctx, projectID, slug); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrHwRevNotFound
		}
		return err
	}
	s.invalidateProject(ctx, projectID)
	return nil
}

// AssertHwRevKnown 校验 slug 已登记。空 slug 表示默认变体，视为已知。未登记 → ErrHwRevUnknown。
func (s *ProjectService) AssertHwRevKnown(ctx context.Context, projectID uuid.UUID, slug string) error {
	slug = platform.Normalize(slug)
	if slug == "" {
		return nil
	}
	if _, err := s.store.GetHwRev(ctx, projectID, slug); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrHwRevUnknown
		}
		return err
	}
	return nil
}

// MatrixEnabledOS 返回矩阵中已登记的 os，供 CanonicalOS 判断 ipados 是否单独启用。
func (s *ProjectService) MatrixEnabledOS(ctx context.Context, projectID uuid.UUID) ([]string, error) {
	rows, err := s.store.ListMatrix(ctx, projectID)
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(rows))
	seen := map[string]struct{}{}
	for i := range rows {
		os := rows[i].OS
		if _, ok := seen[os]; ok {
			continue
		}
		seen[os] = struct{}{}
		out = append(out, os)
	}
	return out, nil
}

func normalizeChannelSlug(raw string) (string, error) {
	slug := platform.Normalize(raw)
	if strings.Contains(slug, "_") {
		return "", fmt.Errorf("%w: slug must not contain underscore", ErrInvalidChannel)
	}
	if !channelSlugRE.MatchString(slug) {
		return "", fmt.Errorf("%w: slug must match [a-z0-9-]{3,64}", ErrInvalidChannel)
	}
	return slug, nil
}

func normalizeChannelName(raw *string, required bool) (string, error) {
	if raw == nil {
		if required {
			return "", fmt.Errorf("%w: name is required", ErrInvalidChannel)
		}
		return "", nil
	}
	name := strings.TrimSpace(*raw)
	if name == "" {
		return "", fmt.Errorf("%w: name is required", ErrInvalidChannel)
	}
	if utf8.RuneCountInString(name) > model.ChannelNameMaxRunes {
		return "", fmt.Errorf("%w: name is too long", ErrInvalidChannel)
	}
	return name, nil
}

// applyChannelToken omit=保持、空串=清除哈希与明文、非空=同时写入明文与 SHA-256。
// 永不把明文写入应用日志；check 仍用 TokenHash 常量时间比较。
func applyChannelToken(ch *model.Channel, token *string) error {
	if token == nil {
		return nil
	}
	plain := strings.TrimSpace(*token)
	if plain == "" {
		ch.TokenHash = ""
		ch.TokenPlain = ""
		return nil
	}
	hash, err := hashToken(plain)
	if err != nil {
		return err
	}
	ch.TokenHash = hash
	ch.TokenPlain = plain
	return nil
}

// ChannelTokenMatches 用 hmac.Equal 比较 SHA-256(header) 与存储哈希。
func ChannelTokenMatches(hash, plaintext string) bool {
	if strings.TrimSpace(hash) == "" {
		return false
	}
	got, err := hashToken(strings.TrimSpace(plaintext))
	if err != nil {
		return false
	}
	return hmac.Equal([]byte(hash), []byte(got))
}

func normalizeHwRevSlug(raw string) (string, error) {
	slug := platform.Normalize(raw)
	if !platform.ValidHwRevSlug(slug) {
		return "", fmt.Errorf("%w: invalid slug", ErrInvalidHwRev)
	}
	return slug, nil
}

func normalizeMatrixOSArch(osRaw, archRaw string) (string, string, error) {
	osSlug := platform.CanonicalOSWrite(osRaw)
	archSlug := platform.CanonicalArch(archRaw)
	if !platform.ValidOSArchSlug(osSlug) {
		return "", "", fmt.Errorf("%w: invalid os", ErrInvalidMatrix)
	}
	if !platform.ValidOSArchSlug(archSlug) {
		return "", "", fmt.Errorf("%w: invalid arch", ErrInvalidMatrix)
	}
	return osSlug, archSlug, nil
}

func normalizePackageType(raw string) (string, error) {
	v := platform.Normalize(raw)
	if err := requireEnum(v, model.PackageTypeSingleFile, model.PackageTypeMultiFile); err != nil {
		return "", fmt.Errorf("%w: package_type", ErrInvalidMatrix)
	}
	return v, nil
}

func applyMatrixWrite(row *model.PlatformMatrix, in MatrixWrite, creating bool) error {
	if in.PackageType != nil {
		pkg, err := normalizePackageType(*in.PackageType)
		if err != nil {
			return err
		}
		row.PackageType = pkg
	}
	if in.DeltaAlgo != nil {
		algo := platform.Normalize(*in.DeltaAlgo)
		if algo == "" && creating {
			algo = model.DeltaAlgoHDiffPatch
		}
		if algo != "" {
			if err := requireEnum(algo, model.DeltaAlgoHDiffPatch, model.DeltaAlgoBsdiff, model.DeltaAlgoXdelta3); err != nil {
				return fmt.Errorf("%w: delta_algo", ErrInvalidMatrix)
			}
			row.DeltaAlgo = algo
		}
	}
	if in.DeltaSourceCount != nil {
		if *in.DeltaSourceCount < 0 {
			return fmt.Errorf("%w: delta_source_count", ErrInvalidMatrix)
		}
		row.DeltaSourceCount = *in.DeltaSourceCount
	}
	if in.HwVariantPolicy != nil {
		pol := platform.Normalize(*in.HwVariantPolicy)
		if pol == "" && creating {
			pol = model.HwVariantIndependent
		}
		if pol != "" {
			if err := requireEnum(pol, model.HwVariantIndependent, model.HwVariantHigherCompatible); err != nil {
				return fmt.Errorf("%w: hw_variant_policy", ErrInvalidMatrix)
			}
			row.HwVariantPolicy = pol
		}
	}
	if in.FallbackArch != nil {
		fb := strings.TrimSpace(*in.FallbackArch)
		if fb == "" {
			row.FallbackArch = ""
		} else {
			fb = platform.CanonicalArch(fb)
			if !platform.ValidOSArchSlug(fb) {
				return fmt.Errorf("%w: invalid fallback_arch", ErrInvalidMatrix)
			}
			row.FallbackArch = fb
		}
	}
	if in.MinimumSupportedVersion != nil {
		v := strings.TrimSpace(*in.MinimumSupportedVersion)
		if v == "" {
			row.MinimumSupportedVersion = nil
		} else {
			row.MinimumSupportedVersion = &v
		}
	}
	return nil
}
