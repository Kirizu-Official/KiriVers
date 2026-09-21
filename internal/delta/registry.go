// Package delta 实现单文件二进制差量的引擎层（docs/app-init.md §7.2）。
//
// 内置三种算法：hdiffpatch、bsdiff、xdelta3；通过静态注册表 Get(algo) 获取。
//   - bsdiff：github.com/gabstv/go-bsdiff（标准 bsdiff4，magic "BSDIFF40"，官方 bspatch 可应用）；
//   - xdelta3：github.com/go-deltasync/vcdiff（RFC 3284，magic "VCDIFF"，xdelta3/open-vcdiff 互操作）；
//   - hdiffpatch：CGO 链接官方 libHDiffPatch，生成未压缩 HDIFF13（magic "HDIFF13&"）。
//     已落库的 KVDIFFHP1 仅由 internal/delta/hdiffpatch 纯 Go 解码器 Patch。
//     服务端入口需要 CGO_ENABLED=1 与 C++ 工具链；详见 docs/delta-engines.md。
//
// 各算法的差量输出由各自的 Patch 还原；Diff/Patch 必须成对使用同一来源。
package delta

import (
	"errors"
	"fmt"
	"sort"
)

// 差量算法标识（§7.2 已拍板：三种算法都支持，用户配置）。
// 与 internal/model 的 DeltaAlgo* 常量保持一致（模型层是 DB 默认值来源，本包是引擎注册来源）。
const (
	// AlgoHDiffPatch HDiffPatch（默认算法，与矩阵 delta_algo 默认值一致）。
	AlgoHDiffPatch = "hdiffpatch"
	// AlgoBsdiff bsdiff4（官方 BSDIFF40 格式）。
	AlgoBsdiff = "bsdiff"
	// AlgoXdelta3 VCDIFF / RFC 3284（xdelta3 兼容）。
	AlgoXdelta3 = "xdelta3"
)

// ErrUnsupported 未知算法：HTTP 层映射为 400 DELTA_ALGO_UNSUPPORTED。
var ErrUnsupported = errors.New("delta algo unsupported")

// Engine 是单一差量算法的 Diff/Patch 能力抽象。
//
// 约定：
//   - Diff(oldData, newData) 返回的差量字节必须能被同一引擎的 Patch(oldData, delta) 还原为 newData；
//   - 实现必须是并发安全的（生成 worker 可能并行调用）；
//   - 输出字节是自描述的（含 magic），Patch 可据此识别来源。
type Engine interface {
	// Algo 返回算法标识（hdiffpatch | bsdiff | xdelta3）。
	Algo() string
	// Diff 由旧数据与新数据计算差量。
	Diff(oldData, newData []byte) ([]byte, error)
	// Patch 将差量应用到旧数据，还原出新数据。
	Patch(oldData, delta []byte) ([]byte, error)
}

// supported 按算法名静态注册引擎（无全局可变状态，天然并发安全）。
var supported = map[string]Engine{
	AlgoHDiffPatch: newHDiffPatchEngine(),
	AlgoBsdiff:     newBsdiffEngine(),
	AlgoXdelta3:    newXdelta3Engine(),
}

// Get 返回指定算法的引擎；未知算法返回 ErrUnsupported（→ 400 DELTA_ALGO_UNSUPPORTED）。
func Get(algo string) (Engine, error) {
	e, ok := supported[algo]
	if !ok {
		return nil, fmt.Errorf("%w: %q", ErrUnsupported, algo)
	}
	return e, nil
}

// Available 列出全部已注册算法名（字典序），供 admin 端点暴露引擎可用性（design §6）。
func Available() []string {
	names := make([]string, 0, len(supported))
	for name := range supported {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// EngineInfo 描述单个算法引擎的当前实现形态（admin 诊断用，design §6）。
type EngineInfo struct {
	// Algo 算法标识。
	Algo string `json:"algo"`
	// Implementation 实现形态："cgo"（链接 libHDiffPatch）或 "pure-go"（内置纯 Go）。
	Implementation string `json:"implementation"`
}

// 实现形态取值。
const (
	// ImplCGO 表示 hdiffpatch 经 CGO 链接官方 libHDiffPatch。
	ImplCGO = "cgo"
	// ImplPureGo 表示内置纯 Go 实现。
	ImplPureGo = "pure-go"
)

// DescribeAvailable 逐算法返回实现形态；hdiffpatch 恒为 cgo，bsdiff/xdelta3 恒为 pure-go。
func DescribeAvailable() []EngineInfo {
	infos := make([]EngineInfo, 0, len(supported))
	for _, algo := range Available() {
		impl := ImplPureGo
		if algo == AlgoHDiffPatch {
			impl = ImplCGO
		}
		infos = append(infos, EngineInfo{Algo: algo, Implementation: impl})
	}
	return infos
}
