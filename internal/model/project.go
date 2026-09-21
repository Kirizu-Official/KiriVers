package model

import (
	"crypto/rand"
	"encoding/hex"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

const (
	// CompareEngineSemver 按 SemVer 2.0 比较（默认）。
	CompareEngineSemver = "semver"
	// CompareEngineInteger 按 version_integer 比较。
	CompareEngineInteger = "integer"

	// DeviceIDPolicyHashed 入库与灰度使用 HMAC 后的 device_id（默认）。
	DeviceIDPolicyHashed = "hashed"
	// DeviceIDPolicyRaw 明文存储（不推荐）。
	DeviceIDPolicyRaw = "raw"
	// DeviceIDPolicyNone 不按设备做遥测降级。
	DeviceIDPolicyNone = "none"

	// StorageVisibilityPublic 产物可用公开直链。
	StorageVisibilityPublic = "public"
	// StorageVisibilityPrivate 产物走短时 URL。
	StorageVisibilityPrivate = "private"

	// SigningAlgoEd25519 默认签名算法。
	SigningAlgoEd25519 = "ed25519"
	// SigningAlgoRSASHA256 RSA-SHA256 签名。
	SigningAlgoRSASHA256 = "rsa-sha256"

	// ChangelogScopeRangeAll 区间内全部非 Draft Version。
	ChangelogScopeRangeAll = "range_all"
	// ChangelogScopeRangePlatform 仅本 os/arch 当时就绪的 Version。
	ChangelogScopeRangePlatform = "range_platform"
	// ChangelogScopeTargetOnly 只要目标 Version。
	ChangelogScopeTargetOnly = "target_only"

	// ChangelogLayoutAggregated 只返回合并 Markdown。
	ChangelogLayoutAggregated = "aggregated"
	// ChangelogLayoutStructured 只返回分版本数组。
	ChangelogLayoutStructured = "structured"
	// ChangelogLayoutBoth 两者都返回（默认）。
	ChangelogLayoutBoth = "both"

	// DefaultSlugAliasRetentionDays 旧 slug alias 默认保留 365 天；0 表示永不过期。
	DefaultSlugAliasRetentionDays = 365

	// DefaultCacheSMaxageSeconds check / changelog 等 GET 响应的 CDN 共享缓存 s-maxage 默认秒数（§10.1 建议默认 60）。
	DefaultCacheSMaxageSeconds = 60

	// DefaultSignedURLTTLSeconds 私有存储短时签名 URL 的默认有效期秒数
	//（§13.7 建议默认 1h）。Project.SignedURLTTLSeconds 为 0 时回退本值。
	DefaultSignedURLTTLSeconds = 3600

	// DefaultFileListMaxFiles 原生 file_list 待下载条数缺省天花板（与 config 缺省对齐）。
	DefaultFileListMaxFiles = 16

	// DefaultChangelogDefaultEntries 无 from_version 时 changelog 默认条数（与 config 缺省对齐）。
	DefaultChangelogDefaultEntries = 5

	// DefaultChangelogMaxEntries 有 from_version 时 changelog 区间截断上限（与 config 缺省对齐）。
	DefaultChangelogMaxEntries = 50

	// DeviceSecretBytesLen 项目设备密钥原始字节数；落库为 hex（×2 长度）。
	DeviceSecretBytesLen = 32

	// MaxProjectNameRunes 项目显示名最大 Unicode 字符数（应用层上限；列类型为 text）。
	MaxProjectNameRunes = 128
)

// Project.RateLimit jsonb 的标准键（§14 限流表）。值语义：>0 为每分钟次数；
// 0 = 显式关闭该维度（不限）；缺键回退文档默认；负值/非法值回退默认。
const (
	// RateLimitKeyCheckPerDevice GET check 每设备（哈希后）每分钟次数，默认 60。
	RateLimitKeyCheckPerDevice = "check_per_device_per_minute"
	// RateLimitKeyCheckPerIP GET check 每源 IP 每分钟次数，默认 120。
	RateLimitKeyCheckPerIP = "check_per_ip_per_minute"
	// RateLimitKeyStorePerIP 公开 feed + integrity + changelog 每源 IP 每分钟次数，默认 300。
	RateLimitKeyStorePerIP = "store_per_ip_per_minute"
	// RateLimitKeyDiffPerDevice POST diff 每设备每分钟次数，默认 20。
	RateLimitKeyDiffPerDevice = "diff_per_device_per_minute"
	// RateLimitKeyDiffPerIP POST diff 每源 IP 每分钟次数，默认 60。
	RateLimitKeyDiffPerIP = "diff_per_ip_per_minute"
	// RateLimitKeyTelemetryPerDevice 遥测 POST 每设备每分钟次数，默认 30。
	RateLimitKeyTelemetryPerDevice = "telemetry_per_device_per_minute"
	// RateLimitKeyCIToken CI 上传/发版每 Token 每分钟次数，默认 600。
	RateLimitKeyCIToken = "ci_per_token_per_minute"
)

// Project 是实例内的一个软件项目（版本目录的根）。
//
// 用途：隔离比较引擎、存储策略、签名、CORS/HTTPS、Token 开关、
// changelog 默认值、限流袋与商店协议包标识。运营者用实例管理员会话 Token 创建。
//
// 关系：
//   - 拥有 ProjectSlugAlias（旧 slug）、ProjectToken / CIToken、后续 Version / 渠道 / 矩阵。
//   - 不归属某个 Admin；管理员是实例级账号。
//
// 字段：
//   - ID：UUID v4 主键，应用侧生成。
//   - Name：人类可读显示名（Unicode，1–128 字符），不唯一，不能当作 project_ref。
//   - Slug：全局唯一，字符集 [a-zA-Z0-9_-]{3,64}。
//   - CompareEngine：semver（默认）或 integer；存在 Published Version 后不可改。
//   - MinimumSupportedVersion：可空；比较走 CompareEngine（本任务只存字符串）。
//   - DefaultLocale：语言回退链的项目默认；与 project_languages.is_default 同步的反规范化副本。
//   - RequireClientToken：客户端 check/download 是否要求 Token，默认 false。
//   - StoreTokenHash：独立商店 Token 的 SHA-256 hex，明文只在写入响应出现。
//   - ForceHTTPS：非 TLS 且无 X-Forwarded-Proto=https 时拒绝，默认 false。
//   - CORSOrigins：允许的 Origin 列表；空表示不加 CORS 头。
//   - DeviceIDPolicy：hashed | raw | none，默认 hashed。
//   - DeviceSecret：hashed 策略的 HMAC 密钥（32 字节 hex）；创建项目时生成，
//     永不经任何 API 返回、永不写日志。
//   - TelemetryRetentionDays：遥测事件留存天数，默认 90；0 视为 90。
//   - GrayWeightTenureActivity：灰度加权放号，默认开（注册更久且更近活跃优先）。
//   - StorageVisibility：public | private，默认 public。
//   - StoragePrefix / StorageBucket：可空，覆盖实例 storage.*。
//   - WebhookURL：发布 webhook 配置位，可空。
//   - WebhookSecret：Publish webhook 签名密钥（配置 URL 时生成，可重置不回显）。
//   - SigningAlgo：ed25519 | rsa-sha256；SigningPublicKey / SigningPrivateKey 为密钥材料。
//   - Changelog*：默认 range_all / both / 允许客户端覆盖。
//   - RateLimit：限流默认 jsonb，429 由后续任务执行。
//   - CacheSMaxageSeconds：check / changelog 等 GET 响应 CDN 共享缓存 s-maxage 秒数，默认 60。
//   - SlugAliasRetentionDays：改 slug 时旧名保留天数，0=永不过期，默认 365。
//   - DeletedAt：GORM 软删；软删后 slug/uuid 均不可解析。
type Project struct {
	ID uuid.UUID `gorm:"type:uuid;primaryKey" json:"uuid"`
	// Name 是控制台显示标题；空串在读取侧回退为 Slug。不参与 UUID/slug/alias 解析。
	Name                    string     `gorm:"type:text;not null;default:''" json:"name"`
	Slug                    string     `gorm:"type:text;uniqueIndex;not null" json:"slug"`
	CompareEngine           string     `gorm:"type:text;not null;default:semver" json:"compare_engine"`
	MinimumSupportedVersion *string    `gorm:"type:text" json:"minimum_supported_version"`
	DefaultLocale           string     `gorm:"type:text;not null;default:en" json:"default_locale"`
	RequireClientToken      bool       `gorm:"not null;default:false" json:"require_client_token"`
	StoreTokenHash           string     `gorm:"type:text" json:"-"`
	ForceHTTPS              bool       `gorm:"not null;default:false" json:"force_https"`
	CORSOrigins             StringList `gorm:"type:jsonb" json:"cors_origins"`
	DeviceIDPolicy          string     `gorm:"type:text;not null;default:hashed" json:"device_id_policy"`
	DeviceSecret            string     `gorm:"type:text;not null;default:''" json:"-"`
	TelemetryRetentionDays     int    `gorm:"not null;default:90" json:"telemetry_retention_days"`
	GrayWeightTenureActivity   bool   `gorm:"not null;default:true" json:"gray_weight_tenure_activity"`
	StorageVisibility          string `gorm:"type:text;not null;default:public" json:"storage_visibility"`
	StoragePrefix           *string    `gorm:"type:text" json:"storage_prefix"`
	StorageBucket           *string    `gorm:"type:text" json:"storage_bucket"`
	WebhookURL              *string    `gorm:"type:text" json:"webhook_url"`
	// WebhookSecret Publish webhook 的 HMAC-SHA256 签名密钥（C14-3，§5.8）。
	// 首次配置 WebhookURL 时生成，可重置但永不回显（json:"-"）；
	// 投递端用它对请求体做 X-KiriVers-Signature 签名。
	WebhookSecret           string         `gorm:"type:text" json:"-"`
	SigningAlgo             string         `gorm:"type:text;not null;default:ed25519" json:"signing_algo"`
	SigningPublicKey        string         `gorm:"type:text" json:"signing_public_key,omitempty"`
	SigningPrivateKey       string         `gorm:"type:text" json:"-"`
	ChangelogScope          string         `gorm:"type:text;not null;default:range_all" json:"changelog_scope"`
	ChangelogLayout         string         `gorm:"type:text;not null;default:both" json:"changelog_layout"`
	ChangelogClientOverride bool           `gorm:"not null;default:true" json:"changelog_client_override"`
	ChangelogIncludeRevoked bool           `gorm:"not null;default:true" json:"changelog_include_revoked"`
	ChangelogIncludeNotes   bool           `gorm:"not null;default:true" json:"changelog_include_platform_notes"`
	// ChangelogDefaultEntries 无 from_version 时返回的最新条数；新建默认 5。
	// PATCH 必须 1…实例 default_entries，且不超过生效 max。
	ChangelogDefaultEntries int `gorm:"not null;default:5" json:"changelog_default_entries"`
	// ChangelogMaxEntries 跨版本窗口截断上限：0 = 继承实例 max_entries。
	ChangelogMaxEntries int            `gorm:"not null;default:0" json:"changelog_max_entries"`
	RateLimit           JSONObject `gorm:"type:jsonb" json:"rate_limit"`
	CacheSMaxageSeconds int        `gorm:"not null;default:60" json:"cache_s_maxage_seconds"`
	SlugAliasRetentionDays  int            `gorm:"not null;default:365" json:"slug_alias_retention_days"`
	// SignedURLTTLSeconds 私有存储（StorageVisibility=private）短时签名 URL
	// 的项目级有效期秒数（§13.7 / C15-1）。0 = 使用实例默认
	// DefaultSignedURLTTLSeconds（3600s = 1h）；负值非法（写入侧拒绝）。
	SignedURLTTLSeconds int            `gorm:"not null;default:0" json:"signed_url_ttl_seconds"`
	// FileListMaxFiles 原生 file_list 待下载条数项目覆盖：0 = 继承平台天花板；
	// 1…天花板为项目上限。写入侧拒绝负数与超过平台值。
	FileListMaxFiles int `gorm:"not null;default:0" json:"file_list_max_files"`
	CreatedAt           time.Time      `json:"created_at"`
	UpdatedAt           time.Time      `json:"updated_at"`
	DeletedAt           gorm.DeletedAt `gorm:"index" json:"-"`
}

// TableName 固定表名 projects。
func (Project) TableName() string {
	return "projects"
}

// DisplayName 返回控制台标题：已存 Name，否则 Slug。
func (p *Project) DisplayName() string {
	if p == nil {
		return ""
	}
	if p.Name == "" {
		return p.Slug
	}
	return p.Name
}

// BeforeCreate 补 UUID、设备密钥与空 jsonb，避免 NULL 破坏读取方。
func (p *Project) BeforeCreate(_ *gorm.DB) error {
	if p.ID == uuid.Nil {
		id, err := uuid.NewRandom() // RFC 4122 UUID v4
		if err != nil {
			return err
		}
		p.ID = id
	}
	if p.DeviceSecret == "" {
		// 兜底生成：任何创建路径遗漏都会在这里补上 32 字节 hex 密钥。
		// 密钥只在服务端内存中使用（hashed 策略 HMAC），永不返回给任何 API。
		secret, err := NewDeviceSecret()
		if err != nil {
			return err
		}
		p.DeviceSecret = secret
	}
	if p.TelemetryRetentionDays <= 0 {
		p.TelemetryRetentionDays = DefaultTelemetryRetentionDays
	}
	if p.CORSOrigins == nil {
		p.CORSOrigins = StringList{}
	}
	if p.RateLimit == nil {
		p.RateLimit = DefaultRateLimit()
	}
	if p.ChangelogDefaultEntries < 1 {
		p.ChangelogDefaultEntries = DefaultChangelogDefaultEntries
	}
	return nil
}

// NewDeviceSecret 生成 32 字节随机密钥的 hex 字符串（64 字符），作 hashed
// 策略的 HMAC-SHA256 密钥。
func NewDeviceSecret() (string, error) {
	b := make([]byte, DeviceSecretBytesLen)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// NewWebhookSecret 生成 32 字节随机密钥的 hex 字符串，作 Publish webhook 的
// HMAC-SHA256 签名密钥（配置 webhook URL 时生成，可重置，永不回显）。
func NewWebhookSecret() (string, error) {
	return NewDeviceSecret()
}

// HasStoreToken 是否已配置独立商店 Token。
func (p *Project) HasStoreToken() bool {
	return p != nil && p.StoreTokenHash != ""
}

// HasSigningPrivateKey 是否已写入私钥材料。
func (p *Project) HasSigningPrivateKey() bool {
	return p != nil && p.SigningPrivateKey != ""
}

// EffectiveFileListMax 计算 file_list 生效上限：项目≤0 继承平台；否则 min(项目, 平台)。
// 平台 <1 回退 DefaultFileListMaxFiles。脏库值高于天花板时运行时仍钳制。
func EffectiveFileListMax(platform, project int) int {
	if platform < 1 {
		platform = DefaultFileListMaxFiles
	}
	if project < 1 {
		return platform
	}
	if project > platform {
		return platform
	}
	return project
}

// EffectiveChangelogMax 计算 changelog 区间截断生效上限：项目≤0 继承实例；否则 min(项目, 实例)。
// 实例 <1 回退 DefaultChangelogMaxEntries。脏库值高于天花板时运行时仍钳制。
func EffectiveChangelogMax(instance, project int) int {
	if instance < 1 {
		instance = DefaultChangelogMaxEntries
	}
	if project < 1 {
		return instance
	}
	if project > instance {
		return instance
	}
	return project
}

// EffectiveChangelogDefault 计算无 from_version 时的生效条数。
// 项目 <1 继承实例 default；再钳到实例 default 与生效 max。实例 default <1 回退 5。
func EffectiveChangelogDefault(instanceDefault, projectDefault, effectiveMax int) int {
	if instanceDefault < 1 {
		instanceDefault = DefaultChangelogDefaultEntries
	}
	if effectiveMax < 1 {
		effectiveMax = DefaultChangelogMaxEntries
	}
	def := projectDefault
	if def < 1 {
		def = instanceDefault
	}
	if def > instanceDefault {
		def = instanceDefault
	}
	if def > effectiveMax {
		def = effectiveMax
	}
	if def < 1 {
		def = 1
	}
	return def
}
