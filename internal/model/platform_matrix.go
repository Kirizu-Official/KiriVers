package model

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

const (
	// PackageTypeSingleFile 该 (os,arch) 线为单文件包。
	PackageTypeSingleFile = "single_file"
	// PackageTypeMultiFile 该 (os,arch) 线为多文件/目录。
	PackageTypeMultiFile = "multi_file"

	// DeltaAlgoHDiffPatch 默认差量算法。
	DeltaAlgoHDiffPatch = "hdiffpatch"
	// DeltaAlgoBsdiff 差量算法 bsdiff。
	DeltaAlgoBsdiff = "bsdiff"
	// DeltaAlgoXdelta3 差量算法 xdelta3。
	DeltaAlgoXdelta3 = "xdelta3"

	// DefaultDeltaSourceCount 新矩阵行默认差量源版本数。
	DefaultDeltaSourceCount = 3

	// HwVariantIndependent 设备 hw_rev 必须与产物精确相等。
	HwVariantIndependent = "independent"
	// HwVariantHigherCompatible 高 rank 产物默认可覆盖更低 rank 设备。
	HwVariantHigherCompatible = "higher_compatible_with_lower"
)

// PlatformMatrix 是项目的平台切片模板，键为 (os, arch)。
//
// 用途：Version Line 实例化时继承 package_type、差量默认、hw 策略；
// 可选覆盖项目级 minimum_supported_version。os/arch 入库为规范名（darwin→macos）。
// 最低 OS / API 在 VersionLine 上，不在矩阵行。
//
// 关系：归属 Project。不自动为每个预置 OS/Arch 插行。
//
// 字段：
//   - ID：UUID 主键。
//   - ProjectID：所属项目。
//   - OS / Arch：规范 slug，项目内唯一组合。
//   - PackageType：single_file | multi_file；该 (os,arch) 首次被 Published Version 使用后不可改。
//   - DeltaAlgo：默认 hdiffpatch。
//   - DeltaSourceCount：默认 3。
//   - HwVariantPolicy：independent | higher_compatible_with_lower。
//   - FallbackArch：架构回退，默认空表示关闭。
//   - MinimumSupportedVersion：可空；空则回退项目字段。
type PlatformMatrix struct {
	ID                      uuid.UUID `gorm:"type:uuid;primaryKey" json:"id"`
	ProjectID               uuid.UUID `gorm:"type:uuid;not null;uniqueIndex:idx_platform_matrix_project_os_arch" json:"project_id"`
	OS                      string    `gorm:"type:text;not null;uniqueIndex:idx_platform_matrix_project_os_arch" json:"os"`
	Arch                    string    `gorm:"type:text;not null;uniqueIndex:idx_platform_matrix_project_os_arch" json:"arch"`
	PackageType             string    `gorm:"type:text;not null" json:"package_type"`
	DeltaAlgo               string    `gorm:"type:text;not null;default:hdiffpatch" json:"delta_algo"`
	DeltaSourceCount        int       `gorm:"not null;default:3" json:"delta_source_count"`
	HwVariantPolicy         string    `gorm:"type:text;not null;default:independent" json:"hw_variant_policy"`
	FallbackArch            string    `gorm:"type:text" json:"fallback_arch"`
	MinimumSupportedVersion *string   `gorm:"type:text" json:"minimum_supported_version"`
	CreatedAt               time.Time `json:"created_at"`
	UpdatedAt               time.Time `json:"updated_at"`
}

// TableName 固定表名 platform_matrix。
func (PlatformMatrix) TableName() string {
	return "platform_matrix"
}

// BeforeCreate 补 UUID 与差量/策略默认值。DeltaSourceCount=0 视为显式写入，不改写成 3。
func (m *PlatformMatrix) BeforeCreate(_ *gorm.DB) error {
	if m.ID == uuid.Nil {
		m.ID = uuid.New()
	}
	if m.DeltaAlgo == "" {
		m.DeltaAlgo = DeltaAlgoHDiffPatch
	}
	if m.HwVariantPolicy == "" {
		m.HwVariantPolicy = HwVariantIndependent
	}
	return nil
}
