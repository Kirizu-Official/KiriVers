package service

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/Kirizu-Official/KiriVers/internal/model"
)

// 本文件实现渠道晋升（C14-2，docs/app-init.md §4.3 / §5.8）：
//
//   - 仅 Published 版本可晋升（Draft 直接用 PUT 改 channel）；
//   - 目标渠道必须存在且启用；
//   - ValidateChannelSuffix（§4.3，单一来源）通过 → 直接改 Version.ChannelSlug/
//     ChannelID（无预发布后缀的 beta 版本升 stable 即此路径，验收 3）；
//   - 后缀冲突（如 1.2.3-beta.1 → stable）→ 400 CHANNEL_SUFFIX_MISMATCH，
//     details 提示「新建更高比较键 Version + artifacts/reuse 复用 blob」（验收 2）；
//   - 目标渠道已存在同比较键 Published Version → 409 CHANNEL_CONFLICT；
//   - 晋升不移动产物行（同 Version 下 Line 不变）；目录 ETag 由既有机制失效。

// ChannelSuffixMismatchError 在渠道晋升场景包装 ErrChannelSuffixMismatch，
// 携带「新建版本 + 复用产物」的修复指引，供 HTTP 层写入响应 details。
type ChannelSuffixMismatchError struct {
	Err error
}

func (e *ChannelSuffixMismatchError) Error() string {
	return e.Err.Error()
}

func (e *ChannelSuffixMismatchError) Unwrap() error {
	return e.Err
}

// PromoteVersionInput 是渠道晋升请求体。
type PromoteVersionInput struct {
	TargetChannel string `json:"target_channel"`
}

// PromoteVersion 把已发布版本晋升到目标渠道（§4.3）。
// 产物行不移动：同 Version 下的 Line / Artifact 保持原样。
func (s *ProjectService) PromoteVersion(ctx context.Context, projectID uuid.UUID, versionRef string, in PromoteVersionInput) (*model.Version, error) {
	v, err := s.ResolveVersion(ctx, projectID, versionRef)
	if err != nil {
		return nil, err
	}
	if v.Status != model.VersionStatusPublished {
		return nil, fmt.Errorf("%w: only published versions can be promoted (current: %s); edit draft channel with PUT", ErrInvalidVersionTransition, v.Status)
	}

	target := strings.TrimSpace(in.TargetChannel)
	if target == "" {
		return nil, fmt.Errorf("%w: target_channel is required", ErrInvalidProjectSettings)
	}
	if v.ChannelSlug == target {
		// 幂等：已在目标渠道，直接返回。
		return v, nil
	}

	ch, err := s.store.GetChannel(ctx, projectID, target)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("%w: channel %q does not exist", ErrChannelNotFound, target)
		}
		return nil, err
	}
	if !ch.Enabled {
		return nil, fmt.Errorf("%w: channel %q is disabled", ErrChannelNotFound, target)
	}

	// 后缀规则（§4.3，单一来源 ValidateChannelSuffix）。整数引擎版本无 SemVer
	// 后缀概念，不适用后缀规则（§4.3：version_integer 不适用后缀规则）。
	if v.VersionSemverCanonical != nil && *v.VersionSemverCanonical != "" {
		if err := ValidateChannelSuffix(target, *v.VersionSemverCanonical); err != nil {
			if errors.Is(err, ErrChannelSuffixMismatch) {
				return nil, &ChannelSuffixMismatchError{Err: err}
			}
			return nil, err
		}
	}

	// 目标渠道同比较键 Published 版本冲突（§4.3：同一比较键不得在两个渠道各建
	// 一条；比较键按项目 compare_engine 取整版本或规范 SemVer）。
	if err := s.checkPromoteConflict(ctx, projectID, v, target); err != nil {
		return nil, err
	}

	v.ChannelSlug = target
	v.ChannelID = &ch.ID
	if err := s.store.SaveVersion(ctx, v); err != nil {
		return nil, err
	}
	s.invalidateProject(ctx, projectID)
	return v, nil
}

// checkPromoteConflict 检查目标渠道是否已存在同比较键的 Published Version（除自身）。
func (s *ProjectService) checkPromoteConflict(ctx context.Context, projectID uuid.UUID, v *model.Version, target string) error {
	if v.VersionSemverCanonical != nil && *v.VersionSemverCanonical != "" {
		existing, err := s.store.GetVersionBySemverCanonical(ctx, projectID, *v.VersionSemverCanonical)
		if err == nil && existing != nil && existing.ID != v.ID &&
			existing.ChannelSlug == target && existing.Status == model.VersionStatusPublished {
			return fmt.Errorf("%w: channel %q already has published version with the same compare key", ErrChannelConflict, target)
		}
		return nil
	}
	if v.VersionInteger != nil {
		existing, err := s.store.GetVersionByInteger(ctx, projectID, *v.VersionInteger)
		if err == nil && existing != nil && existing.ID != v.ID &&
			existing.ChannelSlug == target && existing.Status == model.VersionStatusPublished {
			return fmt.Errorf("%w: channel %q already has published version with the same compare key", ErrChannelConflict, target)
		}
	}
	return nil
}
