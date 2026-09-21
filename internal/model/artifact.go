package model

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

const (
	// ArtifactKindFull 全量包/安装包/固件。
	ArtifactKindFull = "full"
	// ArtifactKindDelta 差量包。
	ArtifactKindDelta = "delta"
	// ArtifactKindPatch 多文件版本对增量归档（patch_package，§7.5）：
	// 仅含目标线 Manifest 相对源线新增/替换（SHA-256 不同）且非 KEEP 的完整文件；
	// DeltaAlgo 恒为空，DeltaSourceSHA256/DeltaTargetSHA256 承载源/目标线
	// 全量归档的 SHA-256（§5.9：锁定对象身份，任一端字节变化 → 新对象新路径）。
	ArtifactKindPatch = "patch"
	// ArtifactKindFile 多文件中的单文件。
	ArtifactKindFile = "file"
	// ArtifactKindStoreFull 商店 line_full 用的带路径 zip（与原生哈希根目录 full 分离）。
	ArtifactKindStoreFull = "store_full"

	// ArtifactCompressionZip 原生/store/patch 归档的压缩格式。
	ArtifactCompressionZip = "zip"
)

// Artifact 是 VersionLine 上可供下载的实体产物（单文件全量包、安装包、固件、差异包等）。
//
// 用途：
//
//	存放产物的对象存储键、文件大小、多重哈希（SHA-256、MD5、可选 SHA-512）、对外稳定文件名、
//	MIME 类型（Content-Type）、开发者数字签名载荷（artifact_signature），
//	以及硬件版本（HwRev）与兼容收窄范围字段（min_hw_rev / max_hw_rev / compatible_hw_revs）。
//
// 关系：
//   - 归属于 Project（ProjectID）
//   - 归属于 Version（VersionID）
//   - 归属于 VersionLine（VersionLineID）
//
// 字段含义：
//   - ID: 产物全局唯一 UUID
//   - ProjectID: 所属项目 UUID
//   - VersionID: 所属版本 UUID
//   - VersionLineID: 所属平台切片 UUID
//   - Kind: 产物种类（full 哈希根目录全量包 | store_full 商店路径 zip | delta 二进制差量包 | patch 多文件 fileset 增量归档 | file 单文件），默认 full
//   - FileName: 对外暴露的稳定文件名（含扩展名，如 {slug}-{ver}-{os}-{arch}[-{hw}].{ext}）
//   - StorageKey: 公共对象键，恒为 {slug}/{sha256}（小写 64 hex；FileName 只在 DB）
//   - Size: 产物字节数（非负整数）
//   - SHA256: 64 位小写十六进制 SHA-256 哈希
//   - MD5: 32 位小写十六进制 MD5 哈希（供旧设备兼容）
//   - SHA512: 128 位小写十六进制 SHA-512 哈希（可选，供 Electron 等特定 feed 协议）
//   - ContentType: 文件的 MIME 类型（按扩展名登记或 application/octet-stream）
//   - DeltaAlgo: 差量算法标识（hdiffpatch | bsdiff | xdelta3，§7.2）；仅 Kind=delta 时必填，
//     其余 Kind 恒为空。同一 (源,目标,os,arch,hw_rev) 可并存多算法对象（C10-2）
//   - DeltaSourceSHA256: 作为差量基线的源全量包 SHA-256（仅 Kind=delta）；与目标哈希共同
//     锁定差量对象身份：任一端字节变化 → 哈希变化 → 新对象新文件名（C10-6，§5.9）
//   - DeltaTargetSHA256: 作为差量目标的全量包 SHA-256（仅 Kind=delta），语义同上
//   - ArtifactSignature: 开发者随包附带的原样签名载荷字符串
//   - HwRev: 硬件代号变体 slug（必须已在项目 HwRev 登记），为 nil 表示默认无硬件限制变体
//   - MinHwRev: 兼容的最低硬件代号（可选）
//   - MaxHwRev: 兼容的最高硬件代号（可选）
//   - CompatibleHwRevs: 显式兼容的硬件代号列表（jsonb，可选）
//   - CreatedAt / UpdatedAt: 实体创建与更新时间
type Artifact struct {
	ID            uuid.UUID `gorm:"type:uuid;primaryKey" json:"id"`
	ProjectID     uuid.UUID `gorm:"type:uuid;not null;index:idx_artifacts_project_sha256,priority:1;index:idx_artifacts_project_file_name,priority:1" json:"project_id"`
	VersionID     uuid.UUID `gorm:"type:uuid;not null;index" json:"version_id"`
	VersionLineID uuid.UUID `gorm:"type:uuid;not null;index:idx_artifacts_line_kind,priority:1" json:"version_line_id"`
	Kind          string    `gorm:"type:text;not null;default:full;index:idx_artifacts_line_kind,priority:2" json:"kind"`
	FileName      string    `gorm:"type:text;not null;index:idx_artifacts_project_file_name,priority:2" json:"file_name"`
	StorageKey    string    `gorm:"type:text;not null" json:"storage_key"`
	Size          int64     `gorm:"not null" json:"size"`
	SHA256        string    `gorm:"type:text;not null;index:idx_artifacts_project_sha256,priority:2" json:"sha256"`
	MD5           string    `gorm:"type:text;not null" json:"md5"`
	SHA512        string    `gorm:"type:text" json:"sha512,omitempty"`
	ContentType   string    `gorm:"type:text;not null;default:application/octet-stream" json:"content_type"`
	// DeltaAlgo / DeltaSourceSHA256 / DeltaTargetSHA256 仅在 Kind=delta / patch 时
	// 承载差量元数据（§7.2 / §7.5 / §5.9；AutoMigrate 只加列，幂等）。GORM 用大写
	// 导出字段名蛇形映射：delta_algo / delta_source_sha256 / delta_target_sha256。
	// Kind=delta：DeltaAlgo=差量算法，源/目标 SHA-256 为单文件全量包哈希；
	// Kind=patch：DeltaAlgo 恒为空，源/目标 SHA-256 为双方线全量归档哈希（§5.9）。
	DeltaAlgo         string `gorm:"type:text" json:"delta_algo,omitempty"`
	DeltaSourceSHA256 string `gorm:"type:text" json:"delta_source_sha256,omitempty"`
	DeltaTargetSHA256 string `gorm:"type:text" json:"delta_target_sha256,omitempty"`
	// FilesetSHA256 是规范化 fileset（排序唯一 NFC 路径 + 文件 SHA-256）的哈希；
	// 仅 Kind=patch 时填写，作为动态打包缓存/幂等键的一部分。
	FilesetSHA256 string `gorm:"type:text" json:"fileset_sha256,omitempty"`
	// Compression 归档压缩格式（zip）；full / store_full / patch 填写。
	Compression       string     `gorm:"type:text" json:"compression,omitempty"`
	ArtifactSignature string     `gorm:"type:text" json:"artifact_signature,omitempty"`
	HwRev             *string    `gorm:"type:text" json:"hw_rev,omitempty"`
	MinHwRev          *string    `gorm:"type:text" json:"min_hw_rev,omitempty"`
	MaxHwRev          *string    `gorm:"type:text" json:"max_hw_rev,omitempty"`
	CompatibleHwRevs  StringList `gorm:"type:jsonb" json:"compatible_hw_revs,omitempty"`
	CreatedAt         time.Time  `json:"created_at"`
	UpdatedAt         time.Time  `json:"updated_at"`
}

// TableName 固定表名 artifacts。
func (Artifact) TableName() string {
	return "artifacts"
}

// BeforeCreate 补全 UUID 与默认 Kind。
func (a *Artifact) BeforeCreate(_ *gorm.DB) error {
	if a.ID == uuid.Nil {
		a.ID = uuid.New()
	}
	if a.Kind == "" {
		a.Kind = ArtifactKindFull
	}
	if a.ContentType == "" {
		a.ContentType = "application/octet-stream"
	}
	return nil
}
