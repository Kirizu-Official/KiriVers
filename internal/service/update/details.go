package update

import (
	"context"

	"github.com/google/uuid"

	"github.com/Kirizu-Official/KiriVers/internal/model"
)

// 本文件定义 integrity / diff 端点按需读取「单条 Version Line 明细」的
// 数据契约（docs/app-init.md §7.1 / §8 / §10.2–10.3）。
//
// 设计要点：
//   - Catalog 快照只为选目标加载 kind=full 产物，Manifest 与 delta/file 产物
//     体积不可控（Manifest 数千行），因此走按需读取接口而非塞进快照；
//   - LineDetailSource 与 CatalogLoader 解耦：check 路径不依赖它，
//     integrity/diff 路径在 Service 未注入时报 ErrLineDetailsUnavailable；
//   - 实现方：repository.UpdateLineDetailRepo（GORM）与
//     repository.MemoryProjectStore（内存测试），均为单 Line 两次有界查询。

// DeltaArtifactInfo 是一条已预生成差量/补丁产物的快照（§7.1/§7.2/§7.5）。
//
// 生成器（binary_delta / patch_package，任务 13）由管理 API Job 与
// 发布后 auto_delta Job 落地；diff 裁决统一以 SHA-256 身份匹配
// （C10-3/C10-6/C13-4）：SourceSHA256 必须等于 source 线官方全量哈希、
// TargetSHA256 必须等于 target 线官方全量哈希——任一端字节变化后旧差量
// 自然不再命中。SourceVersionRef/TargetVersionRef 仅供展示与调试回溯。
type DeltaArtifactInfo struct {
	// Kind 区分消费形态：patch_package（多文件版本对归档）或 binary_delta（单文件二进制差量）。
	Kind string
	// FileName 对外稳定文件名（§5.9，binary_delta 必须含源/目标全量包完整 64 位 SHA-256）。
	FileName string
	// Size / SHA256 产物字节数与哈希（响应 package_url 三元组来源）。
	Size   int64
	SHA256 string
	// Algo 差量算法（hdiffpatch | bsdiff | xdelta3）；binary_delta 响应回填 delta_algo。
	Algo string
	// SourceSHA256 / TargetSHA256 生成时锁定的源/目标全量包 SHA-256（仅 binary_delta；
	// 空串表示未声明 → 不参与匹配，永不命中）。
	SourceSHA256 string
	TargetSHA256 string
	// SourceVersionRef / TargetVersionRef 生成时锁定的源/目标版本引用
	//（十进制整数或规范 SemVer；展示用途，不参与 SHA 身份匹配）。
	SourceVersionRef string
	TargetVersionRef string
	// HwRev 变体维度；nil/空 = 默认变体（对所有客户端可用）。
	HwRev *string
	// ArtifactSignature 开发者随包签名载荷，原样透传。
	ArtifactSignature string
	// StorageKey 是公共对象键 {slug}/{sha256}，供 S3 直链拼绝对 URL。
	StorageKey string
}

// FileArtifactInfo 是 kind=file 产物快照（§7.5.4 file_list 例外的逐文件 URL 来源）。
type FileArtifactInfo struct {
	// Path 对应的 Manifest 相对路径（空表示未建立映射 → file_list 不命中）。
	Path string
	// FileName 对外稳定文件名（展示用；下载 URL 按 SHA-256 拼装）。
	FileName string
	// StorageKey 对象存储键，供商店 feed 对清单文件做字节级签名。
	StorageKey string
	// Size / SHA256 / MD5 文件体元数据。
	Size   int64
	SHA256 string
	MD5    string
	// HwRev 变体维度；nil/空 = 默认变体。
	HwRev *string
}

// LineDetail 是单条 Line 的按需明细集合；任一字段可为空（实现方按可得性返回）。
type LineDetail struct {
	// Manifest 全量 Manifest 条目（调用方自行排序/分页，不依赖实现方顺序）。
	Manifest []model.ManifestEntry
	// Deltas 该线上 kind=delta 的二进制差量产物（binary_delta，§7.2）。
	Deltas []DeltaArtifactInfo
	// Patches 该线上 kind=patch 的多文件版本对增量归档（patch_package，§7.5）。
	// 与 Deltas 分列表承载（C13：patch/binary_delta 身份与消费形态不同，
	// kind 常量区分），避免匹配逻辑互相污染。
	Patches []DeltaArtifactInfo
	// Files 该线上 kind=file 的单文件产物（file_list 例外用）。
	Files []FileArtifactInfo
}

// LineDetailSource 按需读取单条 Version Line 的 Manifest 与 delta/file 产物。
type LineDetailSource interface {
	// LineDetails 一次取回指定 Line 的全部明细；未知 lineID 返回空明细而非错误。
	LineDetails(ctx context.Context, lineID uuid.UUID) (*LineDetail, error)
}

// DowngradeSource 读取「连续失败降级」状态（C11-8 / §15.2）：由遥测服务
// 实现，check/diff 编排层据此对带 device_id 的请求强制全量。接口在此声明、
// 在 internal/service 落地，保持 update 包不依赖遥测存储。
type DowngradeSource interface {
	// DowngradeActive 返回该设备在 (project, os, arch) 上的降级是否生效。
	DowngradeActive(ctx context.Context, projectID uuid.UUID, os, arch, deviceHash string) (bool, error)
}

// Option 是 Service 的构造选项。
type Option func(*Service)

// WithLineDetails 注入 Line 明细读取源；integrity/diff 依赖它，check 不依赖。
func WithLineDetails(src LineDetailSource) Option {
	return func(s *Service) {
		s.details = src
	}
}

// WithDowngradeSource 注入连续失败降级读取源；注入后 check/diff 编排会对
// DeviceHash 非空的请求查询降级状态（单条索引查询，仅带 device 的请求发生）。
func WithDowngradeSource(src DowngradeSource) Option {
	return func(s *Service) {
		s.downgrade = src
	}
}

// WithURLSigner 注入私有存储短时签名器（§13.7 / C15-1）：私有项目
//（StorageVisibility=private）的 package_url / full_package_url / 逐文件 URL
// / diff details.full_package_url 全部追加 ?exp=&sig= 短时签名；公开项目
// 不受影响（C15-2 直链不变）。nil（默认）时不签名。
func WithURLSigner(signer URLSigner) Option {
	return func(s *Service) {
		s.signer = signer
	}
}

// WithPackRuntime 注入动态打包运行时（查缓存 / 入队 Job）；pack HTTP 依赖它。
func WithPackRuntime(rt PackRuntime) Option {
	return func(s *Service) {
		s.pack = rt
	}
}

// WithDynamicPackMaxBytes 注入 D6 未压缩硬顶（默认 512MiB）。
func WithDynamicPackMaxBytes(n int64) Option {
	return func(s *Service) {
		s.dynamicPackMaxBytes = n
	}
}

// WithFileListMaxFiles 注入原生 file_list 平台天花板（默认 16）。
func WithFileListMaxFiles(n int) Option {
	return func(s *Service) {
		s.fileListMaxFiles = n
	}
}

// WithChangelogLimits 注入实例 changelog 条数窗口（<1 时 Changelog 侧回退 5/50）。
func WithChangelogLimits(defaultEntries, maxEntries int) Option {
	return func(s *Service) {
		s.changelogDefaultEntries = defaultEntries
		s.changelogMaxEntries = maxEntries
	}
}

// WithPublicObjectURL 在多节点 S3 直链时把 check/feed 下载地址写成公共对象绝对 URL。
func WithPublicObjectURL(fn func(slug, sha256, storageKey string) string) Option {
	return func(s *Service) {
		s.objectURL = fn
	}
}

// WithReplicaGate 在本机代拉时按本节点本地副本隐藏尚未就绪的线。
func WithReplicaGate(fn func(storageKey string) bool) Option {
	return func(s *Service) {
		s.replicaReady = fn
		s.localProxy = fn != nil
	}
}

// EnclosureURL 返回 check/feed 同源下载地址（S3 直链为绝对 URL，否则 /packages/{sha256}）。
func (s *Service) EnclosureURL(cat *Catalog, slug, sha256, storageKey string) string {
	return s.signingFor(cat).artifactURL(slug, sha256, storageKey)
}

// EnclosureObjectURL 返回 S3 直链拼装函数（可 nil）。
func (s *Service) EnclosureObjectURL() func(slug, sha256, storageKey string) string {
	if s == nil {
		return nil
	}
	return s.objectURL
}

// LocalProxy 表示本机代拉：check/feed 走节点路径且不得进共享 CDN。
func (s *Service) LocalProxy() bool {
	return s != nil && s.localProxy
}

// LineDetails 按需读取单条 Version Line 的 Manifest / file / delta（feed 完整包选择器用）。
func (s *Service) LineDetails(ctx context.Context, lineID uuid.UUID) (*LineDetail, error) {
	if s == nil || s.details == nil {
		return nil, ErrLineDetailsUnavailable
	}
	return s.details.LineDetails(ctx, lineID)
}
