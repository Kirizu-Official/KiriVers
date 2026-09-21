package update

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/Kirizu-Official/KiriVers/internal/model"
)

// ProjectInfo 是目录快照中与选目标、响应组装相关的项目级配置。
//
// 字段说明：
//   - CompareEngine：semver | integer，比较键选择；
//   - MinimumSupportedVersion：项目级最低支持版本（矩阵行可覆盖）；
//   - Changelog*：changelog 默认范围/排版与客户端覆盖开关；
//   - CacheSMaxageSeconds：GET 响应 CDN s-maxage；
//   - DeviceIDPolicy：hashed | raw | none。灰度白名单比较键据此选择：
//     raw 策略下白名单存原样、比较用原样；其余策略用 HMAC 后的 DeviceHash；
//   - SigningAlgo / SigningPrivateKey：原生 signature 注入所需密钥材料；
//   - StorageVisibility：public | private。private 时下载 URL 走短时签名
//     （§13.7 / C15-1），公开项目直链不变（C15-2）；
//   - SignedURLTTLSeconds：私有签名 URL 项目级 TTL；0 = 实例默认 3600s。
type ProjectInfo struct {
	ID                      uuid.UUID
	Slug                    string
	CompareEngine           string
	MinimumSupportedVersion *string
	RequireClientToken      bool
	DefaultLocale           string
	ChangelogScope          string
	ChangelogLayout         string
	ChangelogClientOverride bool
	ChangelogIncludeRevoked bool
	ChangelogIncludeNotes   bool
	// ChangelogDefaultEntries / ChangelogMaxEntries 是 loadCatalog 已钳制的生效条数。
	ChangelogDefaultEntries int
	ChangelogMaxEntries     int
	CacheSMaxageSeconds     int
	DeviceIDPolicy          string
	SigningAlgo             string
	SigningPrivateKey       string
	StorageVisibility       string
	SignedURLTTLSeconds     int
	// FileListMaxFiles 是已钳制的生效 file_list 上限（平台+项目，Diff 纯函数读此值）。
	FileListMaxFiles int
}

// ChannelInfo 是渠道快照：stability_rank 越大越稳定（§4.4）。
// TokenHash 仅供 hmac 比较，禁止进入 ETag / JSON。
type ChannelInfo struct {
	Slug           string
	StabilityRank  int
	Enabled        bool
	Unlisted       bool
	TokenProtected bool
	TokenHash      string
}

// MatrixInfo 是平台矩阵行快照（键 (os, arch)）。
//
// 字段说明：
//   - PackageType：single_file | multi_file（200 响应 package_type）；
//   - FallbackArch：架构回退声明；
//   - DeltaAlgo：差量算法（delta_available 时下发）；
//   - HwVariantPolicy：independent | higher_compatible_with_lower；
//   - MinimumSupportedVersion：可选覆盖项目级最低支持版本。
type MatrixInfo struct {
	OS                      string
	Arch                    string
	PackageType             string
	FallbackArch            string
	DeltaAlgo               string
	HwVariantPolicy         string
	MinimumSupportedVersion *string
}

// ArtifactInfo 是 kind=full 产物快照（多文件线为预生成 zip），含 hw_rev 变体维度。
type ArtifactInfo struct {
	FileName string
	Size     int64
	SHA256   string
	MD5      string // 兼容旧设备的 MD5（integrity hash_algo=md5 时使用；不参与 ETag）
	// SHA512 是可选的 128 位小写十六进制 SHA-512（上传时随多重哈希一并登记；
	// 旧行可能为空）。供 electron-updater feed 协议使用（其校验 base64(原始
	// 512-bit 摘要)）；为空时由 feed 适配器按需从存储计算，不回填数据库。
	SHA512            string
	ArtifactSignature string
	// ContentType 是产物登记的 MIME（§5.9，按扩展名登记）；商店协议 feed 的
	// enclosure type 使用该值，与下载端点 Content-Type 同源。
	ContentType string
	// StorageKey 是公共对象键 {slug}/{sha256}（小写 64 hex）。
	// 本机代拉按该键读本地副本；S3 直链用同一键拼匿名 GET URL（无 FileName 段）。
	StorageKey       string
	HwRev            *string
	MinHwRev         *string
	MaxHwRev         *string
	CompatibleHwRevs model.StringList
}

// LineState 是 Version 在某一 (os, arch) 上的切片快照。
//
// FullPkgs 携带该线全部 kind=full 产物（含 hw_rev 变体），选目标时按
// 请求 hw_rev 与矩阵策略挑选一个变体作为下发包。
type LineState struct {
	// ID 是 VersionLine 主键；integrity/diff 据此按需读取 Manifest 与
	// delta/file 产物（LineDetailSource），check 路径不使用。
	ID            uuid.UUID
	OS            string
	Arch          string
	Status        string // ready | yanked | disabled | pending | ...
	RootHash      string
	MinOS         *string
	MinAPILevel   *int
	PlatformNotes string
	FullPkgs      []ArtifactInfo
	// StoreFullPkgs 是商店 line_full 用的带路径 zip（kind=store_full）。
	// 多文件线缺省不得回退 FullPkgs（哈希根 kind=full）；单文件线无 store_full
	// 时仍用 FullPkgs。原生 check 永远用 FullPkgs。
	StoreFullPkgs []ArtifactInfo
	// PacksReadyAt 系统预热完成时间；nil 时 check/feed 不可见该线（D13）。
	PacksReadyAt *time.Time
}

// LinePacksReady 判定该线是否已对客户端可见：必须 ready 且系统预热已盖戳。
// 当前线 min_os 仍只看 Status==ready；候选/store/指定 target 走本函数。
func LinePacksReady(line *LineState) bool {
	if line == nil {
		return false
	}
	return line.Status == model.VersionLineStatusReady && line.PacksReadyAt != nil
}

// Allowlist 是一个 Version 的灰度白名单快照（存储形态 id 列表）。
//
// 存储值按项目 DeviceIDPolicy 而定：hashed 策略下为 HMAC hex（与遥测
// DeviceHash 同函数），raw 策略下为原样明文。命中判定见 grayHitFor。
// MaxCreatedAt 供 ETag 感知名单编辑，永不把 device_id 写入 ETag。
type Allowlist struct {
	Version      []string
	MaxCreatedAt *time.Time
}

// contains 判断 key 是否在版本级白名单。金丝雀名单量级小，线性扫描即可。
func (a Allowlist) contains(key string) bool {
	if key == "" {
		return false
	}
	for _, id := range a.Version {
		if id == key {
			return true
		}
	}
	return false
}

// VersionState 是目录快照中的一个版本：Version 行 + 该版本在请求平台
// （(os,arch) 与 (os,fallback_arch)）上的切片。包含 revoked/deprecated 版本
// （降级与中继解析需要）。
type VersionState struct {
	Version model.Version
	Lines   []LineState
	// Allowlist 是该 Version 的灰度白名单快照（C12-1/C12-3）。金丝雀名单
	// 量级小，随 Catalog 一次装载，选目标保持纯函数零额外查询。
	Allowlist Allowlist
}

// Line 返回版本在 (os, arch) 上的切片；不存在返回 nil。
func (v *VersionState) Line(os, arch string) *LineState {
	for i := range v.Lines {
		if v.Lines[i].OS == os && v.Lines[i].Arch == arch {
			return &v.Lines[i]
		}
	}
	return nil
}

// Catalog 是一次 check 所需的完整目录快照（纯数据，选目标算法不再触库）。
//
// Matrix 为请求 (os,arch) 行；FallbackMatrix 为 (os, fallback_arch) 行，
// 仅当请求矩阵行声明 FallbackArch 时加载；二者皆可为 nil。
// HwRevRanks 是项目硬件代号 slug → rank 表，供变体兼容比较；
// EnabledOS 是矩阵中出现过的 os 集合（ipados 是否独立登记）。
type Catalog struct {
	Project        ProjectInfo
	Channels       []ChannelInfo
	HwRevRanks     map[string]int
	Matrix         *MatrixInfo
	FallbackMatrix *MatrixInfo
	EnabledOS      []string
	Versions       []VersionState
}

// Channel 返回 slug 对应的渠道；不存在返回 false。
func (c *Catalog) Channel(slug string) (ChannelInfo, bool) {
	for _, ch := range c.Channels {
		if ch.Slug == slug {
			return ch, true
		}
	}
	return ChannelInfo{}, false
}

// channelRank 返回渠道 stability_rank；渠道不存在（如已被删的自定义渠道）按 0 处理。
func (c *Catalog) channelRank(slug string) int {
	if ch, ok := c.Channel(slug); ok {
		return ch.StabilityRank
	}
	return 0
}

// catalogHasTokenProtected 目录是否含有渠道 Token 保护行（决定 Vary 与私有缓存）。
func (c *Catalog) catalogHasTokenProtected() bool {
	for _, ch := range c.Channels {
		if ch.TokenProtected {
			return true
		}
	}
	return false
}

// catalogHasIncompleteGray 目录中是否存在尚未转全量的已发布灰度版本
// （Draft 不计），决定 check 响应必须 private（D4 / AC7）。
func (c *Catalog) catalogHasIncompleteGray() bool {
	for i := range c.Versions {
		v := &c.Versions[i].Version
		if v.Status != model.VersionStatusDraft && !v.GrayIsComplete() {
			return true
		}
	}
	return false
}

// CatalogLoader 一次性装配项目目录快照（§2：整目录加载，纯函数不再触库）。
//
// os/arch 必须已规范化；实现需加载：项目全部 Version、这些 Version 在
// (os,arch) 与 (os,fallback_arch) 上的 Line、线上的 kind=full Artifact、
// 渠道表、矩阵行与硬件代号表。单项目版本量级为数百，配合 CDN 短缓存，
// DB 压力有界。
type CatalogLoader interface {
	LoadCatalog(ctx context.Context, projectID uuid.UUID, os, arch string) (*Catalog, error)
}
