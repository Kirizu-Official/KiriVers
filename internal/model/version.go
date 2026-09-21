package model

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

const (
	// VersionStatusDraft 草稿，尚未发布；客户端不可见，可继续挂载平台切片与上传产物。
	VersionStatusDraft = "draft"
	// VersionStatusPublished 已发布；进入更新流；至少一条平台切片就绪；compare_engine 与已有平台形态不可改。
	VersionStatusPublished = "published"
	// VersionStatusDeprecated 已弃用；不作为自动更新目标，但仍允许直接查询或下载。
	VersionStatusDeprecated = "deprecated"
	// VersionStatusRevoked 已吊销；禁止下载产物，客户端强制离开并允许安全降级。
	VersionStatusRevoked = "revoked"

	// DefaultGrayStartPercent 创建版本时起始覆盖缺省 30（未传 gray_start_percent）。
	DefaultGrayStartPercent = 30
	// DefaultGrayStepPercent 默认灰度增量百分比。
	DefaultGrayStepPercent = 10
	// DefaultGrayIntervalSeconds 默认时间增量（秒）。
	DefaultGrayIntervalSeconds = 3600
)

// Version 是项目下的版本发布实体。每个 Version 同时记录整数构建号（VersionInteger）与语义化版本（VersionSemver）。
// 项目级 compare_engine 决定哪个作为主比较键（用于检查更新、选目标、中继、最低支持、升降级），另一个仅作显示与辅助查询。
//
// 关系：
//   - 归属 Project（ProjectID）。
//   - 关联 Channel（ChannelID / ChannelSlug）。
//   - 包含多条 VersionLine（不同 os/arch 平台切片）。
//
// 字段：
//   - ID：UUID 主键，应用侧生成。
//   - ProjectID：所属项目 UUID；与 version_integer / version_semver_canonical 组成项目内唯一索引。
//   - ChannelID：关联渠道 UUID（可空）。
//   - ChannelSlug：所属渠道 slug（如 stable, beta），冗余存储加速查询与 exists 返回。
//   - VersionInteger：正整数构建号；在同一项目非空行中唯一。
//   - VersionSemver：SemVer 2.0 规范形式（去 v、去 +build）；在同一项目非空行中唯一。
//   - VersionSemverCanonical：去 +build 的规范 SemVer，供唯一性约束与索引检索。
//   - Status：draft | published | deprecated | revoked。
//   - Changelog：多语言更新日志 jsonb，格式形如 { "en": { "title": "...", "markdown": "..." } }。
//   - GitTag：构建关联的 Git 标签。
//   - GitCommit：构建关联的 Git commit 哈希。
//   - GitLogFrom：Git 日志区间起始 commit/tag。
//   - GitLogTo：Git 日志区间结束 commit/tag。
//   - IsLTS：是否为长期支持（LTS）版本；事后可设；渠道 slug 为 lts 时本字段仍默认 false。
//   - IsCritical：是否为紧急关键版本；为 true 时禁止灰度（强制全员推送）。
//   - GrayStartPercent / GrayStepPercent / GrayIntervalSeconds：增量灰度三旋钮。
//   - GrayStartedAt：进入灰度墙钟；PATCH 旋钮不重置。
//   - GrayCompletedAt：非空 = 全量推送（匿名与后续新设备可见）。
//   - MinSourceVersion：最低直升来源；check 在当前低于该门槛时把目标改写为该版本（中继链）。
//   - CreatedAt / UpdatedAt：时间戳。
type Version struct {
	ID                     uuid.UUID        `gorm:"type:uuid;primaryKey" json:"id"`
	ProjectID              uuid.UUID        `gorm:"type:uuid;not null;index:idx_versions_project_status,priority:1;uniqueIndex:idx_versions_project_integer,where:version_integer IS NOT NULL;uniqueIndex:idx_versions_project_semver,where:version_semver_canonical IS NOT NULL" json:"project_id"`
	ChannelID              *uuid.UUID       `gorm:"type:uuid" json:"channel_id,omitempty"`
	ChannelSlug            string           `gorm:"type:text;not null" json:"channel"`
	VersionInteger         *int64           `gorm:"uniqueIndex:idx_versions_project_integer,where:version_integer IS NOT NULL" json:"version_integer"`
	VersionSemver          *string          `gorm:"type:text" json:"version_semver"`
	VersionSemverCanonical *string          `gorm:"type:text;uniqueIndex:idx_versions_project_semver,where:version_semver_canonical IS NOT NULL" json:"-"`
	Status                 string           `gorm:"type:text;not null;index:idx_versions_project_status,priority:2" json:"status"`
	Changelog              ChangelogMap     `gorm:"type:jsonb" json:"changelog"`
	GitTag                 *string          `gorm:"type:text" json:"git_tag,omitempty"`
	GitCommit              *string          `gorm:"type:text" json:"git_commit,omitempty"`
	GitLogFrom             *string          `gorm:"type:text" json:"git_log_from,omitempty"`
	GitLogTo               *string          `gorm:"type:text" json:"git_log_to,omitempty"`
	IsLTS                  bool             `gorm:"not null;default:false" json:"is_lts"`
	IsCritical             bool             `gorm:"not null;default:false" json:"is_critical"`
	GrayStartPercent       int              `gorm:"not null;default:30" json:"gray_start_percent"`
	GrayStepPercent        int              `gorm:"not null;default:10" json:"gray_step_percent"`
	GrayIntervalSeconds    int              `gorm:"not null;default:3600" json:"gray_interval_seconds"`
	GrayStartedAt          *time.Time       `json:"gray_started_at,omitempty"`
	GrayCompletedAt        *time.Time       `json:"gray_completed_at,omitempty"`
	MinSourceVersion       *string          `gorm:"type:text" json:"min_source_version,omitempty"`
	AutoPublishWhen        *AutoPublishRule `gorm:"type:jsonb" json:"auto_publish_when,omitempty"`
	PublishTime            *time.Time       `json:"publish_time,omitempty"`
	CreatedAt              time.Time        `json:"created_at"`
	UpdatedAt              time.Time        `json:"updated_at"`
}

// AutoPublishRule 描述多切片并行上传时齐套自动发版的判定规则（C07-9, §11.3.4）。
type AutoPublishRule struct {
	RequiredLines StringList `json:"required_lines,omitempty"` // 如 ["windows/x86_64", "linux/x86_64"]
	AllowPartial  bool       `json:"allow_partial"`            // 默认 false
}

// TableName 固定表名 versions，后续任务不得改名。
func (Version) TableName() string {
	return "versions"
}

// GrayIsComplete 全量推送：关键版本或已写入 gray_completed_at。
func (v *Version) GrayIsComplete() bool {
	if v == nil {
		return true
	}
	return v.IsCritical || v.GrayCompletedAt != nil
}

// GrayIsActive 已发布且尚未转全量（非关键）。
func (v *Version) GrayIsActive() bool {
	if v == nil {
		return false
	}
	return v.Status == VersionStatusPublished && !v.GrayIsComplete() && v.GrayStartedAt != nil
}

// BeforeCreate 补 UUID、默认 draft、默认灰度旋钮与空 ChangelogMap。
func (v *Version) BeforeCreate(_ *gorm.DB) error {
	if v.ID == uuid.Nil {
		v.ID = uuid.New()
	}
	if v.Status == "" {
		v.Status = VersionStatusDraft
	}
	if v.GrayIntervalSeconds <= 0 {
		v.GrayIntervalSeconds = DefaultGrayIntervalSeconds
	}
	if v.Changelog == nil {
		v.Changelog = ChangelogMap{}
	}
	return nil
}
