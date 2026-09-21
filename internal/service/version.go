package service

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	msemver "github.com/Masterminds/semver/v3"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/Kirizu-Official/KiriVers/internal/model"
	"github.com/Kirizu-Official/KiriVers/internal/platform"
	"github.com/Kirizu-Official/KiriVers/internal/service/update"
	"github.com/Kirizu-Official/KiriVers/pkg/semver"
)

var (
	// ErrVersionNotFound 版本号不存在。
	ErrVersionNotFound = errors.New("version not found")
	// ErrVersionLineNotFound 平台切片不存在。
	ErrVersionLineNotFound = errors.New("version line not found")
	// ErrEngineMismatch 比较键缺失、无法解析或引擎类型不匹配。
	ErrEngineMismatch = errors.New("engine mismatch or invalid version format")
	// ErrChannelSuffixMismatch 语义化版本的预发布后缀与渠道标识不符。
	ErrChannelSuffixMismatch = errors.New("semver suffix does not match channel")
	// ErrVersionAlreadyExists 版本号已存在或已被占用。
	ErrVersionAlreadyExists = errors.New("version already exists")
	// ErrChannelConflict 幂等创建时渠道与已有版本不一致。
	ErrChannelConflict = errors.New("channel conflict with existing version")
	// ErrArtifactRequired 发布版本时至少需要一条已就绪的平台切片。
	ErrArtifactRequired = errors.New("at least one ready version line is required to publish")
	// ErrGrayNotAllowedOnCritical 关键紧急版本禁止灰度放量。
	ErrGrayNotAllowedOnCritical = errors.New("gray rollout is not allowed on critical version")
	// ErrIntermediateUnavailable 中继版本不存在、未发布或已吊销。
	ErrIntermediateUnavailable = errors.New("intermediate version is unavailable")
	// ErrInvalidVersionTransition 非法的版本生命周期状态流转。
	ErrInvalidVersionTransition = errors.New("invalid version status transition")
	// ErrPublishedVersionImmutable 已发布版本的版本号或主 changelog 不可更改。
	ErrPublishedVersionImmutable = errors.New("published version numbers or changelog cannot be modified")
	// ErrVersionLineAlreadyExists 平台切片已存在。
	ErrVersionLineAlreadyExists = errors.New("version line already exists")
	// ErrAutoPublishPending 齐套自动发布所需的平台切片未全部就绪（C07-9）。
	ErrAutoPublishPending = errors.New("auto publish pending: required lines not all ready")
)

// VersionConflictError 携带冲突的字段信息（version_integer 或 version_semver）。
type VersionConflictError struct {
	Field string
}

func (e *VersionConflictError) Error() string {
	return "version already exists: " + e.Field
}

func (e *VersionConflictError) Unwrap() error {
	return ErrVersionAlreadyExists
}

var intVersionRegex = regexp.MustCompile(`^[0-9]+$`)

// ParsedVersionRef 表示从字符串解析出的版本号引用。
type ParsedVersionRef struct {
	IsInteger bool
	Integer   int64
	Semver    string // 规范化 SemVer（去 v、去 +build）
}

// ParseVersionRef 解析 API 传入的 :version 或 current_version 字符串。
//
// 规则：
// 1. 去除首尾空白；
// 2. 整段为十进制正整数（无小圆点，如 "10"、"010"）走 integer 路径，"010" 与 "10" 均解析为 10；
// 3. 否则走 SemVer 2.0 规范化（去 v 前缀、去 +build）。若不合法则返回包含 ErrEngineMismatch 的错误。
func ParseVersionRef(raw string) (*ParsedVersionRef, error) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return nil, fmt.Errorf("%w: empty version string", ErrEngineMismatch)
	}
	if intVersionRegex.MatchString(s) {
		val, err := strconv.ParseInt(s, 10, 64)
		if err != nil || val <= 0 {
			return nil, fmt.Errorf("%w: invalid integer version %q", ErrEngineMismatch, raw)
		}
		return &ParsedVersionRef{
			IsInteger: true,
			Integer:   val,
		}, nil
	}
	canonical, err := semver.Canonical(s)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrEngineMismatch, err)
	}
	return &ParsedVersionRef{
		IsInteger: false,
		Semver:    canonical,
	}, nil
}

// ValidateChannelSuffix 校验 SemVer 2.0 预发布后缀与渠道 slug 的匹配约束（C04-4）。
//
// 规则：
// - 无预发布后缀的版本允许属于任何渠道；
// - stable 渠道禁止带有预发布后缀；
// - 非 stable 渠道若带有预发布后缀，其第一段标识符必须精确等于渠道 slug（如 beta.1 必须属于 beta 渠道）。
func ValidateChannelSuffix(channelSlug, canonicalSemver string) error {
	v, err := msemver.StrictNewVersion(canonicalSemver)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrEngineMismatch, err)
	}
	prerelease := v.Prerelease()
	if prerelease == "" {
		return nil
	}
	if channelSlug == model.ChannelStable {
		return fmt.Errorf("%w: stable channel forbids prerelease suffix %q", ErrChannelSuffixMismatch, prerelease)
	}
	parts := strings.Split(prerelease, ".")
	firstIdent := parts[0]
	if firstIdent != channelSlug {
		return fmt.Errorf("%w: prerelease prefix %q does not match channel %q", ErrChannelSuffixMismatch, firstIdent, channelSlug)
	}
	return nil
}

// VersionWriteInput 是创建 Draft 版本的输入。
type VersionWriteInput struct {
	Channel                     string
	VersionInteger              *int64
	VersionSemver               *string
	Changelog                   *string
	ChangelogI18n               model.ChangelogMap
	GitTag                      *string
	GitCommit                   *string
	GitLogFrom                  *string
	GitLogTo                    *string
	IsLTS                *bool
	IsCritical           *bool
	GrayStartPercent     *int
	GrayStepPercent      *int
	GrayIntervalSeconds  *int
	MinSourceVersion     *string
	AutoPublishWhen      *model.AutoPublishRule
}

// VersionPatchInput 是修改版本的输入。
type VersionPatchInput struct {
	VersionInteger   *int64
	VersionSemver    *string
	Changelog        *string
	ChangelogI18n    model.ChangelogMap
	GitTag           *string
	GitCommit        *string
	GitLogFrom       *string
	GitLogTo         *string
	IsLTS               *bool
	IsCritical          *bool
	MinSourceVersion    *string
	AutoPublishWhen  *model.AutoPublishRule
}

// PublishVersionInput 提供发版时的即时校验选项（如覆盖 required_lines 或 allow_partial）。
type PublishVersionInput struct {
	RequiredLines []string
	AllowPartial  *bool
}

// VersionLineWriteInput 是创建平台切片的输入。
type VersionLineWriteInput struct {
	OS            string
	Arch          string
	MinOS         *string
	MinAPILevel   *int
	PlatformNotes string
}

// VersionLinePatchInput 是 PATCH 平台切片的输入。*Set 区分省略与清空。
type VersionLinePatchInput struct {
	MinOSSet       bool
	MinOS          *string
	MinAPISet   bool
	MinAPILevel *int
}

// ResolveVersion 根据版本引用（整数、SemVer 或 UUID）在项目内定位唯一的 Version 记录。
func (s *ProjectService) ResolveVersion(ctx context.Context, projectID uuid.UUID, versionRef string) (*model.Version, error) {
	if parsedUUID, err := uuid.Parse(versionRef); err == nil {
		v, err := s.store.GetVersionByID(ctx, parsedUUID)
		if err == nil && v != nil && v.ProjectID == projectID {
			return v, nil
		}
	}
	ref, err := ParseVersionRef(versionRef)
	if err != nil {
		return nil, err
	}
	if ref.IsInteger {
		v, err := s.store.GetVersionByInteger(ctx, projectID, ref.Integer)
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil, ErrVersionNotFound
			}
			return nil, err
		}
		return v, nil
	}
	v, err := s.store.GetVersionBySemverCanonical(ctx, projectID, ref.Semver)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrVersionNotFound
		}
		return nil, err
	}
	return v, nil
}

// CheckVersionExists 检查指定版本号是否存在（C04-9）。
// 如果版本字符串无法解析或记录不存在，返回 exists=false 且无错误。
func (s *ProjectService) CheckVersionExists(ctx context.Context, projectID uuid.UUID, versionRef string) (bool, *model.Version, []model.VersionLine, error) {
	ref, err := ParseVersionRef(versionRef)
	if err != nil {
		return false, nil, nil, nil
	}
	var v *model.Version
	if ref.IsInteger {
		row, err := s.store.GetVersionByInteger(ctx, projectID, ref.Integer)
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return false, nil, nil, nil
			}
			return false, nil, nil, err
		}
		v = row
	} else {
		row, err := s.store.GetVersionBySemverCanonical(ctx, projectID, ref.Semver)
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return false, nil, nil, nil
			}
			return false, nil, nil, err
		}
		v = row
	}
	lines, err := s.store.ListVersionLines(ctx, v.ID)
	if err != nil {
		return false, nil, nil, err
	}
	return true, v, lines, nil
}

// PutVersion 幂等创建草稿版本（C04-1–C04-5, C04-10–C04-12）。
// 返回 (*model.Version, created, error)，其中 created=true 表示 201，created=false 表示 200 幂等命中。
func (s *ProjectService) PutVersion(ctx context.Context, projectID uuid.UUID, versionRef string, in VersionWriteInput) (*model.Version, bool, error) {
	proj, err := s.store.GetByID(ctx, projectID)
	if err != nil {
		return nil, false, err
	}

	channelSlug := strings.TrimSpace(in.Channel)
	if channelSlug == "" {
		return nil, false, fmt.Errorf("%w: channel is required", ErrInvalidProjectSettings)
	}
	ch, err := s.store.GetChannel(ctx, projectID, channelSlug)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, false, fmt.Errorf("%w: channel %q does not exist", ErrChannelNotFound, channelSlug)
		}
		return nil, false, err
	}

	ref, err := ParseVersionRef(versionRef)
	if err != nil {
		return nil, false, err
	}

	var targetInt *int64
	var targetSemverCanonical *string

	if ref.IsInteger {
		targetInt = &ref.Integer
		if in.VersionInteger != nil && *in.VersionInteger != *targetInt {
			return nil, false, fmt.Errorf("%w: path integer %d != body integer %d", ErrEngineMismatch, *targetInt, *in.VersionInteger)
		}
		if in.VersionSemver != nil && strings.TrimSpace(*in.VersionSemver) != "" {
			canon, err := semver.Canonical(*in.VersionSemver)
			if err != nil {
				return nil, false, fmt.Errorf("%w: %w", ErrEngineMismatch, err)
			}
			targetSemverCanonical = &canon
		}
	} else {
		targetSemverCanonical = &ref.Semver
		if in.VersionSemver != nil && strings.TrimSpace(*in.VersionSemver) != "" {
			canon, err := semver.Canonical(*in.VersionSemver)
			if err != nil {
				return nil, false, fmt.Errorf("%w: %w", ErrEngineMismatch, err)
			}
			if canon != *targetSemverCanonical {
				return nil, false, fmt.Errorf("%w: path semver %q != body semver %q", ErrEngineMismatch, *targetSemverCanonical, canon)
			}
		}
		if in.VersionInteger != nil {
			if *in.VersionInteger <= 0 {
				return nil, false, fmt.Errorf("%w: version_integer must be positive", ErrEngineMismatch)
			}
			targetInt = in.VersionInteger
		}
	}

	// 比较引擎规则判定 (C04-1)
	if proj.CompareEngine == model.CompareEngineSemver {
		if targetSemverCanonical == nil {
			return nil, false, fmt.Errorf("%w: semver required when compare_engine is semver", ErrEngineMismatch)
		}
	} else if proj.CompareEngine == model.CompareEngineInteger {
		if targetInt == nil {
			maxVal, err := s.store.GetMaxVersionInteger(ctx, projectID)
			if err != nil {
				return nil, false, err
			}
			next := maxVal + 1
			targetInt = &next
		}
	}

	// 渠道后缀约束校验 (C04-4)
	if targetSemverCanonical != nil {
		if err := ValidateChannelSuffix(channelSlug, *targetSemverCanonical); err != nil {
			return nil, false, err
		}
	}

	// 关键紧急版本与灰度互斥：start < 100 视为启用灰度。
	isCritical := in.IsCritical != nil && *in.IsCritical
	start := model.DefaultGrayStartPercent
	if in.GrayStartPercent != nil {
		start = *in.GrayStartPercent
	}
	if start < 0 || start > 100 {
		return nil, false, ErrInvalidRolloutPercent
	}
	step := model.DefaultGrayStepPercent
	if in.GrayStepPercent != nil {
		step = *in.GrayStepPercent
	}
	if step < 0 || step > 100 {
		return nil, false, ErrInvalidRolloutPercent
	}
	interval := model.DefaultGrayIntervalSeconds
	if in.GrayIntervalSeconds != nil {
		interval = *in.GrayIntervalSeconds
	}
	if interval <= 0 {
		return nil, false, ErrInvalidRequest("gray_interval_seconds must be positive")
	}
	if isCritical && start < 100 {
		return nil, false, ErrGrayNotAllowedOnCritical
	}

	// 幂等性与唯一性检查 (C04-5)
	var existingInt *model.Version
	var existingSemver *model.Version

	if targetInt != nil {
		if v, err := s.store.GetVersionByInteger(ctx, projectID, *targetInt); err == nil {
			existingInt = v
		}
	}
	if targetSemverCanonical != nil {
		if v, err := s.store.GetVersionBySemverCanonical(ctx, projectID, *targetSemverCanonical); err == nil {
			existingSemver = v
		}
	}

	if existingInt != nil && existingSemver != nil {
		if existingInt.ID == existingSemver.ID {
			if existingInt.ChannelSlug != channelSlug {
				return nil, false, ErrChannelConflict
			}
			return existingInt, false, nil
		}
		// 指向两个不同的已有版本
		return nil, false, &VersionConflictError{Field: "version_integer"}
	}

	if existingInt != nil && existingSemver == nil {
		if targetSemverCanonical != nil {
			if existingInt.VersionSemverCanonical != nil && *existingInt.VersionSemverCanonical != *targetSemverCanonical {
				return nil, false, &VersionConflictError{Field: "version_integer"}
			}
		}
		if existingInt.ChannelSlug != channelSlug {
			return nil, false, ErrChannelConflict
		}
		return existingInt, false, nil
	}

	if existingSemver != nil && existingInt == nil {
		if targetInt != nil {
			if existingSemver.VersionInteger != nil && *existingSemver.VersionInteger != *targetInt {
				return nil, false, &VersionConflictError{Field: "version_semver"}
			}
		}
		if existingSemver.ChannelSlug != channelSlug {
			return nil, false, ErrChannelConflict
		}
		return existingSemver, false, nil
	}

	// 组装并创建新版本
	changelog := in.ChangelogI18n
	if changelog == nil {
		changelog = model.ChangelogMap{}
	}
	if in.Changelog != nil && strings.TrimSpace(*in.Changelog) != "" {
		loc := proj.DefaultLocale
		if loc == "" {
			loc = "en"
		}
		entry := changelog[loc]
		entry.Markdown = *in.Changelog
		changelog[loc] = entry
	}

	isLTS := false
	if in.IsLTS != nil {
		isLTS = *in.IsLTS
	}

	v := &model.Version{
		ProjectID:                   projectID,
		ChannelID:                   &ch.ID,
		ChannelSlug:                 channelSlug,
		VersionInteger:              targetInt,
		VersionSemver:               targetSemverCanonical,
		VersionSemverCanonical:      targetSemverCanonical,
		Status:                      model.VersionStatusDraft,
		Changelog:                   changelog,
		GitTag:                      in.GitTag,
		GitCommit:                   in.GitCommit,
		GitLogFrom:                  in.GitLogFrom,
		GitLogTo:                    in.GitLogTo,
		IsLTS:                 isLTS,
		IsCritical:            isCritical,
		GrayStartPercent:      start,
		GrayStepPercent:       step,
		GrayIntervalSeconds:   interval,
		MinSourceVersion:      in.MinSourceVersion,
		AutoPublishWhen:       in.AutoPublishWhen,
	}

	if err := s.store.CreateVersion(ctx, v); err != nil {
		if errors.Is(err, gorm.ErrDuplicatedKey) {
			// 并发竞争重试读取
			if targetInt != nil {
				if existing, qerr := s.store.GetVersionByInteger(ctx, projectID, *targetInt); qerr == nil {
					if existing.ChannelSlug == channelSlug {
						return existing, false, nil
					}
					return nil, false, ErrChannelConflict
				}
			}
			if targetSemverCanonical != nil {
				if existing, qerr := s.store.GetVersionBySemverCanonical(ctx, projectID, *targetSemverCanonical); qerr == nil {
					if existing.ChannelSlug == channelSlug {
						return existing, false, nil
					}
					return nil, false, ErrChannelConflict
				}
			}
			return nil, false, &VersionConflictError{Field: "version_semver"}
		}
		return nil, false, err
	}

	s.invalidateProject(ctx, projectID)
	return v, true, nil
}

// PatchVersion 修改版本字段（C04-8, C04-14）。
// 已发布的版本不可修改双版本号或主 changelog；但允许修改 is_lts、git_*、中继与灰度。
func (s *ProjectService) PatchVersion(ctx context.Context, projectID uuid.UUID, versionRef string, in VersionPatchInput) (*model.Version, error) {
	v, err := s.ResolveVersion(ctx, projectID, versionRef)
	if err != nil {
		return nil, err
	}

	if v.Status != model.VersionStatusDraft {
		if in.VersionInteger != nil && (v.VersionInteger == nil || *in.VersionInteger != *v.VersionInteger) {
			return nil, fmt.Errorf("%w: version_integer cannot be modified after publish", ErrPublishedVersionImmutable)
		}
		if in.VersionSemver != nil {
			canon, err := semver.Canonical(*in.VersionSemver)
			if err != nil || v.VersionSemverCanonical == nil || canon != *v.VersionSemverCanonical {
				return nil, fmt.Errorf("%w: version_semver cannot be modified after publish", ErrPublishedVersionImmutable)
			}
		}
	}
	if in.ChangelogI18n != nil {
		v.Changelog = in.ChangelogI18n
	}
	if in.Changelog != nil {
		proj, err := s.store.GetByID(ctx, projectID)
		if err != nil {
			return nil, err
		}
		loc := proj.DefaultLocale
		if loc == "" {
			loc = "en"
		}
		entry := v.Changelog[loc]
		entry.Markdown = *in.Changelog
		v.Changelog[loc] = entry
	}

	if in.IsLTS != nil {
		v.IsLTS = *in.IsLTS
	}
	if in.IsCritical != nil {
		v.IsCritical = *in.IsCritical
	}
	if v.IsCritical {
		now := time.Now().UTC()
		if v.GrayCompletedAt == nil {
			v.GrayCompletedAt = &now
		}
	}

	if in.GitTag != nil {
		v.GitTag = in.GitTag
	}
	if in.GitCommit != nil {
		v.GitCommit = in.GitCommit
	}
	if in.GitLogFrom != nil {
		v.GitLogFrom = in.GitLogFrom
	}
	if in.GitLogTo != nil {
		v.GitLogTo = in.GitLogTo
	}
	if in.MinSourceVersion != nil {
		v.MinSourceVersion = in.MinSourceVersion
	}
	if in.AutoPublishWhen != nil {
		v.AutoPublishWhen = in.AutoPublishWhen
	}

	if err := s.store.SaveVersion(ctx, v); err != nil {
		return nil, err
	}
	s.invalidateProject(ctx, projectID)
	return v, nil
}

// DeleteVersion 删除草稿版本（C04-6）。只有 draft 状态的版本允许删除。
func (s *ProjectService) DeleteVersion(ctx context.Context, projectID uuid.UUID, versionRef string) error {
	v, err := s.ResolveVersion(ctx, projectID, versionRef)
	if err != nil {
		return err
	}
	if v.Status != model.VersionStatusDraft {
		return fmt.Errorf("%w: only draft versions can be deleted (current: %s)", ErrInvalidVersionTransition, v.Status)
	}
	if err := s.store.DeleteVersion(ctx, v.ID); err != nil {
		return err
	}
	s.invalidateProject(ctx, projectID)
	return nil
}

// PublishVersion 将版本从草稿推进至已发布状态（C04-6, C04-7, C04-12, C04-14, C07-9）。
func (s *ProjectService) PublishVersion(ctx context.Context, projectID uuid.UUID, versionRef string, opts ...PublishVersionInput) (*model.Version, error) {
	v, err := s.ResolveVersion(ctx, projectID, versionRef)
	if err != nil {
		return nil, err
	}
	if v.Status == model.VersionStatusPublished {
		return v, nil
	}
	if v.Status == model.VersionStatusRevoked || v.Status == model.VersionStatusDeprecated {
		return nil, fmt.Errorf("%w: cannot publish from status %s", ErrInvalidVersionTransition, v.Status)
	}

	// 检查是否有未完成的分片上传 (C05-8)
	lines, err := s.store.ListVersionLines(ctx, v.ID)
	if err != nil {
		return nil, err
	}
	for _, l := range lines {
		sessions, err := s.store.ListActiveUploadSessionsByLineID(ctx, l.ID)
		if err != nil {
			return nil, err
		}
		if len(sessions) > 0 {
			return nil, ErrUploadIncomplete
		}
	}

	// 齐套发布校验规则 (C07-9, §11.3.4)
	var rule *model.AutoPublishRule
	if len(opts) > 0 {
		opt := opts[0]
		if len(opt.RequiredLines) > 0 || opt.AllowPartial != nil {
			rule = &model.AutoPublishRule{
				RequiredLines: opt.RequiredLines,
			}
			if opt.AllowPartial != nil {
				rule.AllowPartial = *opt.AllowPartial
			}
		}
	}
	if rule == nil {
		rule = v.AutoPublishWhen
	}

	if rule != nil && len(rule.RequiredLines) > 0 {
		for _, reqLine := range rule.RequiredLines {
			parts := strings.Split(strings.TrimSpace(reqLine), "/")
			if len(parts) < 2 {
				continue
			}
			canOS := platform.CanonicalOSWrite(parts[0])
			canArch := platform.CanonicalArch(parts[1])
			found := false
			for _, l := range lines {
				if l.OS == canOS && l.Arch == canArch && l.Status == model.VersionLineStatusReady {
					found = true
					break
				}
			}
			if !found && !rule.AllowPartial {
				return nil, ErrAutoPublishPending
			}
		}
	}

	// 闸门规则：至少一条就绪平台切片 (C04-7)
	readyCount, err := s.store.CountReadyVersionLines(ctx, v.ID)
	if err != nil {
		return nil, err
	}
	if readyCount == 0 {
		return nil, ErrArtifactRequired
	}

	// 多文件归档包校验 (C06-6)
	for _, l := range lines {
		if l.Status == model.VersionLineStatusReady {
			if matrixRow, _ := s.store.GetMatrix(ctx, projectID, l.OS, l.Arch); matrixRow != nil && matrixRow.PackageType == model.PackageTypeMultiFile {
				art, artErr := s.store.GetArtifactByLineID(ctx, l.ID)
				if artErr != nil || art == nil || art.Kind != model.ArtifactKindFull {
					return nil, ErrArchiveRequired
				}
				if l.RootHash == "" {
					return nil, ErrArchiveRequired
				}
			}
		}
	}

	v.Status = model.VersionStatusPublished
	now := time.Now().UTC()
	v.PublishTime = &now
	if v.IsCritical || v.GrayStartPercent >= 100 {
		if v.GrayStartedAt == nil {
			v.GrayStartedAt = &now
		}
		v.GrayCompletedAt = &now
	} else {
		v.GrayStartedAt = &now
		v.GrayCompletedAt = nil
	}
	if err := s.store.SaveVersion(ctx, v); err != nil {
		return nil, err
	}
	if v.GrayIsActive() {
		if proj, perr := s.store.GetByID(ctx, projectID); perr == nil {
			if _, aerr := s.EnsureGrayAdmission(ctx, proj, v); aerr == nil && v != nil {
				// version timestamps may have been completed (empty namebook)
			}
		}
	}
	s.invalidateProject(ctx, projectID)
	// 发布成功后触发自动差量/增量包生成（C13-1，§7.2）：仅毫秒级入队，
	// 生成全异步；失败不影响已 Published 状态（C13-6），故静默忽略错误。
	s.notifyLineReady(ctx, projectID, v.ID)
	// 发布 webhook（C14-3，§5.8）：仅入队即返回，失败不影响发版（§11.5）。
	s.notifyWebhookEvent(ctx, projectID, model.WebhookEventVersionPublished, v.ID, nil)
	return v, nil
}

// CheckAndTriggerAutoPublish 检查草稿版本是否配置了 AutoPublishWhen，并在全部 RequiredLines 均就绪时自动推进为 Published（C07-9）。
func (s *ProjectService) CheckAndTriggerAutoPublish(ctx context.Context, projectID uuid.UUID, versionID uuid.UUID) (bool, error) {
	v, err := s.store.GetVersionByID(ctx, versionID)
	if err != nil {
		return false, err
	}
	if v.Status != model.VersionStatusDraft || v.AutoPublishWhen == nil || len(v.AutoPublishWhen.RequiredLines) == 0 {
		return false, nil
	}

	lines, err := s.store.ListVersionLines(ctx, v.ID)
	if err != nil {
		return false, err
	}

	for _, reqLine := range v.AutoPublishWhen.RequiredLines {
		parts := strings.Split(strings.TrimSpace(reqLine), "/")
		if len(parts) < 2 {
			continue
		}
		canOS := platform.CanonicalOSWrite(parts[0])
		canArch := platform.CanonicalArch(parts[1])
		found := false
		for _, l := range lines {
			if l.OS == canOS && l.Arch == canArch && l.Status == model.VersionLineStatusReady {
				found = true
				break
			}
		}
		if !found {
			return false, nil
		}
	}

	ref := ""
	if v.VersionSemver != nil && *v.VersionSemver != "" {
		ref = *v.VersionSemver
	} else if v.VersionInteger != nil {
		ref = strconv.FormatInt(*v.VersionInteger, 10)
	}
	_, err = s.PublishVersion(ctx, projectID, ref)
	if err != nil {
		return false, err
	}
	return true, nil
}

// DeprecateVersion 将版本弃用（C04-6, C04-14）。
func (s *ProjectService) DeprecateVersion(ctx context.Context, projectID uuid.UUID, versionRef string) (*model.Version, error) {
	v, err := s.ResolveVersion(ctx, projectID, versionRef)
	if err != nil {
		return nil, err
	}
	if v.Status == model.VersionStatusRevoked {
		return nil, fmt.Errorf("%w: cannot deprecate revoked version", ErrInvalidVersionTransition)
	}
	if v.Status == model.VersionStatusDeprecated {
		return v, nil
	}
	v.Status = model.VersionStatusDeprecated
	if err := s.store.SaveVersion(ctx, v); err != nil {
		return nil, err
	}
	s.invalidateProject(ctx, projectID)
	return v, nil
}

// RevokeVersion 吊销版本（C04-6, C04-14）。
func (s *ProjectService) RevokeVersion(ctx context.Context, projectID uuid.UUID, versionRef string) (*model.Version, error) {
	v, err := s.ResolveVersion(ctx, projectID, versionRef)
	if err != nil {
		return nil, err
	}
	if v.Status == model.VersionStatusRevoked {
		return v, nil
	}
	v.Status = model.VersionStatusRevoked
	if err := s.store.SaveVersion(ctx, v); err != nil {
		return nil, err
	}
	s.invalidateProject(ctx, projectID)
	// 吊销 webhook（C14-3，§5.8）：仅入队即返回，失败不影响吊销（§11.5）。
	s.notifyWebhookEvent(ctx, projectID, model.WebhookEventVersionRevoked, v.ID, nil)
	return v, nil
}

// AddVersionLine 为指定版本添加一条平台切片（C04-13）。
// 平台与架构写入时通过 CanonicalOSWrite / CanonicalArch 规范化。
func (s *ProjectService) AddVersionLine(ctx context.Context, projectID uuid.UUID, versionRef string, in VersionLineWriteInput) (*model.VersionLine, error) {
	v, err := s.ResolveVersion(ctx, projectID, versionRef)
	if err != nil {
		return nil, err
	}

	canonicalOS := platform.CanonicalOSWrite(in.OS)
	canonicalArch := platform.CanonicalArch(in.Arch)

	minOS, err := normalizeLineMinOS(in.MinOS)
	if err != nil {
		return nil, err
	}
	minAPI, err := normalizeLineMinAPILevel(in.MinAPILevel)
	if err != nil {
		return nil, err
	}

	line := &model.VersionLine{
		VersionID:     v.ID,
		ProjectID:     projectID,
		OS:            canonicalOS,
		Arch:          canonicalArch,
		Status:        model.VersionLineStatusPending,
		MinOS:         minOS,
		MinAPILevel:   minAPI,
		PlatformNotes: in.PlatformNotes,
	}

	if err := s.store.CreateVersionLine(ctx, line); err != nil {
		if errors.Is(err, gorm.ErrDuplicatedKey) {
			return nil, ErrVersionLineAlreadyExists
		}
		return nil, err
	}
	s.invalidateProject(ctx, projectID)
	return line, nil
}

// ReadyVersionLine 将指定平台切片置为 ready 就绪状态（测试夹具 API，C04-7）。
func (s *ProjectService) ReadyVersionLine(ctx context.Context, projectID uuid.UUID, versionRef, os, arch string) (*model.VersionLine, error) {
	v, err := s.ResolveVersion(ctx, projectID, versionRef)
	if err != nil {
		return nil, err
	}
	canonicalOS := platform.CanonicalOSWrite(os)
	canonicalArch := platform.CanonicalArch(arch)
	line, err := s.store.GetVersionLine(ctx, v.ID, canonicalOS, canonicalArch)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrVersionLineNotFound
		}
		return nil, err
	}
	sessions, err := s.store.ListActiveUploadSessionsByLineID(ctx, line.ID)
	if err != nil {
		return nil, err
	}
	if len(sessions) > 0 {
		return nil, ErrUploadIncomplete
	}

	// 多文件归档包校验 (C06-4, C06-6)
	if matrixRow, _ := s.store.GetMatrix(ctx, projectID, canonicalOS, canonicalArch); matrixRow != nil && matrixRow.PackageType == model.PackageTypeMultiFile {
		art, artErr := s.store.GetArtifactByLineID(ctx, line.ID)
		if artErr != nil || art == nil || art.Kind != model.ArtifactKindFull {
			return nil, ErrArchiveRequired
		}
		if line.RootHash == "" {
			return nil, ErrArchiveRequired
		}
	}

	line.Status = model.VersionLineStatusReady
	if err := s.store.SaveVersionLine(ctx, line); err != nil {
		return nil, err
	}
	s.invalidateProject(ctx, projectID)
	_, _ = s.CheckAndTriggerAutoPublish(ctx, projectID, v.ID)
	// Line 就绪后触发自动差量（C13-1 补平台场景）：版本已 Published 时生效；
	// 尚为 Draft 时由 CheckAndTriggerAutoPublish → PublishVersion 的触发点接管。
	s.notifyLineReady(ctx, projectID, v.ID)
	// 已发布版本的补平台线就绪 webhook（C14-3，§5.8）；Draft 不发。
	s.notifyLineReadyWebhook(ctx, projectID, v, line)
	return line, nil
}

// YankVersionLine 将指定平台切片撤回（C04-15）。被 yank 的线对客户端不可见，但不会吊销整次发布。
func (s *ProjectService) YankVersionLine(ctx context.Context, projectID uuid.UUID, versionRef, os, arch string) (*model.VersionLine, error) {
	v, err := s.ResolveVersion(ctx, projectID, versionRef)
	if err != nil {
		return nil, err
	}
	canonicalOS := platform.CanonicalOSWrite(os)
	canonicalArch := platform.CanonicalArch(arch)
	line, err := s.store.GetVersionLine(ctx, v.ID, canonicalOS, canonicalArch)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrVersionLineNotFound
		}
		return nil, err
	}
	line.Status = model.VersionLineStatusYanked
	if err := s.store.SaveVersionLine(ctx, line); err != nil {
		return nil, err
	}
	s.invalidateProject(ctx, projectID)
	return line, nil
}

// DisableVersionLine 将指定平台切片临时停用（C04-15）。
func (s *ProjectService) DisableVersionLine(ctx context.Context, projectID uuid.UUID, versionRef, os, arch string) (*model.VersionLine, error) {
	v, err := s.ResolveVersion(ctx, projectID, versionRef)
	if err != nil {
		return nil, err
	}
	canonicalOS := platform.CanonicalOSWrite(os)
	canonicalArch := platform.CanonicalArch(arch)
	line, err := s.store.GetVersionLine(ctx, v.ID, canonicalOS, canonicalArch)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrVersionLineNotFound
		}
		return nil, err
	}
	line.Status = model.VersionLineStatusDisabled
	if err := s.store.SaveVersionLine(ctx, line); err != nil {
		return nil, err
	}
	s.invalidateProject(ctx, projectID)
	return line, nil
}

// GetVersionLine 获取指定平台切片详情。
func (s *ProjectService) GetVersionLine(ctx context.Context, projectID uuid.UUID, versionRef, os, arch string) (*model.VersionLine, error) {
	v, err := s.ResolveVersion(ctx, projectID, versionRef)
	if err != nil {
		return nil, err
	}
	canonicalOS := platform.CanonicalOSWrite(os)
	canonicalArch := platform.CanonicalArch(arch)
	line, err := s.store.GetVersionLine(ctx, v.ID, canonicalOS, canonicalArch)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrVersionLineNotFound
		}
		return nil, err
	}
	return line, nil
}

// ListVersionLines 列出指定版本下的全部平台切片。
func (s *ProjectService) ListVersionLines(ctx context.Context, projectID uuid.UUID, versionRef string) ([]model.VersionLine, error) {
	v, err := s.ResolveVersion(ctx, projectID, versionRef)
	if err != nil {
		return nil, err
	}
	return s.store.ListVersionLines(ctx, v.ID)
}

// VersionLineWithArtifacts 是管理端产物列表行。
type VersionLineWithArtifacts struct {
	Line      model.VersionLine
	Artifacts []model.Artifact
}

// ListVersionLinesWithArtifacts 列出线并附带该版本全部产物（按线分组）。
func (s *ProjectService) ListVersionLinesWithArtifacts(ctx context.Context, projectID uuid.UUID, versionRef string) ([]VersionLineWithArtifacts, error) {
	v, err := s.ResolveVersion(ctx, projectID, versionRef)
	if err != nil {
		return nil, err
	}
	lines, err := s.store.ListVersionLines(ctx, v.ID)
	if err != nil {
		return nil, err
	}
	arts, err := s.store.ListArtifactsByVersionID(ctx, v.ID)
	if err != nil {
		return nil, err
	}
	byLine := make(map[uuid.UUID][]model.Artifact, len(lines))
	for i := range arts {
		byLine[arts[i].VersionLineID] = append(byLine[arts[i].VersionLineID], arts[i])
	}
	out := make([]VersionLineWithArtifacts, 0, len(lines))
	for i := range lines {
		out = append(out, VersionLineWithArtifacts{Line: lines[i], Artifacts: byLine[lines[i].ID]})
	}
	return out, nil
}

// ListVersions 列出项目下的全部版本。
func (s *ProjectService) ListVersions(ctx context.Context, projectID uuid.UUID) ([]model.Version, error) {
	return s.store.ListVersions(ctx, projectID)
}

// ListPublishedReadyVersionsForPlatform 列出指定 os/arch 上已发布且该线就绪的版本，
// 并用 update.CompareVersions / newerReleased 选出 latest（不调用 SelectTarget）。
func (s *ProjectService) ListPublishedReadyVersionsForPlatform(ctx context.Context, p *model.Project, os, arch string) ([]model.Version, *ProjectLatestVersion, error) {
	if p == nil {
		return nil, nil, ErrProjectNotFound
	}
	canonOS := platform.CanonicalOSWrite(os)
	canonArch := platform.CanonicalArch(arch)
	list, err := s.store.ListPublishedReadyVersionsForPlatform(ctx, p.ID, canonOS, canonArch)
	if err != nil {
		return nil, nil, err
	}
	latestMap := pickLatestReleased(list, map[uuid.UUID]string{p.ID: p.CompareEngine})
	return list, latestMap[p.ID], nil
}

// VersionListFilter 是管理端版本列表 AND 过滤条件。os+arch 表示「有该线（任意状态）」。
type VersionListFilter struct {
	Channel string
	Status  string
	OS      string
	Arch    string
}

// ListVersionsFiltered 列出项目版本并按 channel/status/os/arch AND 过滤。
// os 与 arch 同时非空时只保留拥有该平台线的版本（任意线状态），不是 published+ready 工作台形状。
func (s *ProjectService) ListVersionsFiltered(ctx context.Context, projectID uuid.UUID, filter VersionListFilter) ([]model.Version, error) {
	list, err := s.store.ListVersions(ctx, projectID)
	if err != nil {
		return nil, err
	}
	channel := strings.TrimSpace(filter.Channel)
	status := strings.TrimSpace(filter.Status)
	osRaw := strings.TrimSpace(filter.OS)
	archRaw := strings.TrimSpace(filter.Arch)
	var canonOS, canonArch string
	filterLine := osRaw != "" && archRaw != ""
	if filterLine {
		canonOS = platform.CanonicalOSWrite(osRaw)
		canonArch = platform.CanonicalArch(archRaw)
	}
	out := make([]model.Version, 0, len(list))
	for i := range list {
		v := list[i]
		if channel != "" && v.ChannelSlug != channel {
			continue
		}
		if status != "" && v.Status != status {
			continue
		}
		if filterLine {
			if _, err := s.store.GetVersionLine(ctx, v.ID, canonOS, canonArch); err != nil {
				if errors.Is(err, gorm.ErrRecordNotFound) {
					continue
				}
				return nil, err
			}
		}
		out = append(out, v)
	}
	return out, nil
}

// LineDefaults 返回当前版本之下、同一 (os,arch) 线的最高比较键版本上的 min_os / min_api_level。
// 没有更低版本时字段为 nil。
func (s *ProjectService) LineDefaults(ctx context.Context, p *model.Project, versionRef, os, arch string) (*string, *int, error) {
	if p == nil {
		return nil, nil, ErrProjectNotFound
	}
	cur, err := s.ResolveVersion(ctx, p.ID, versionRef)
	if err != nil {
		return nil, nil, err
	}
	canonOS := platform.CanonicalOSWrite(os)
	canonArch := platform.CanonicalArch(arch)
	if canonOS == "" || canonArch == "" {
		return nil, nil, ErrInvalidRequest("os and arch are required")
	}
	list, err := s.store.ListVersions(ctx, p.ID)
	if err != nil {
		return nil, nil, err
	}
	var best *model.Version
	var bestLine *model.VersionLine
	for i := range list {
		v := &list[i]
		if v.ID == cur.ID {
			continue
		}
		cmp, ok := update.CompareVersions(p.CompareEngine, v, cur)
		if !ok || cmp >= 0 {
			continue
		}
		line, err := s.store.GetVersionLine(ctx, v.ID, canonOS, canonArch)
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				continue
			}
			return nil, nil, err
		}
		if best == nil {
			best = v
			bestLine = line
			continue
		}
		if c, ok := update.CompareVersions(p.CompareEngine, v, best); ok && c > 0 {
			best = v
			bestLine = line
		}
	}
	if bestLine == nil {
		return nil, nil, nil
	}
	return bestLine.MinOS, bestLine.MinAPILevel, nil
}

var dottedNumericRE = regexp.MustCompile(`^[0-9]+(\.[0-9]+)*$`)

func normalizeLineMinOS(raw *string) (*string, error) {
	if raw == nil {
		return nil, nil
	}
	s := strings.TrimSpace(*raw)
	if s == "" {
		return nil, nil
	}
	if !dottedNumericRE.MatchString(s) {
		return nil, ErrInvalidRequest("min_os must be digits with optional dots")
	}
	return &s, nil
}

func normalizeLineMinAPILevel(raw *int) (*int, error) {
	if raw == nil {
		return nil, nil
	}
	if *raw < 0 {
		return nil, ErrInvalidRequest("min_api_level must be a non-negative integer")
	}
	v := *raw
	return &v, nil
}

