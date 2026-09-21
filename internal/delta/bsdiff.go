package delta

import (
	"bytes"

	"github.com/gabstv/go-bsdiff/pkg/bsdiff"
	"github.com/gabstv/go-bsdiff/pkg/bspatch"
)

// bsdiffMagic 是标准 bsdiff4 容器的 8 字节 magic（"BSDIFF40"），
// 官方 bspatch / bsdiff 工具产出与识别的头前缀（C10-8：格式可识别）。
var bsdiffMagic = []byte("BSDIFF40")

// bsdiffEngine 基于 github.com/gabstv/go-bsdiff（纯 Go，标准 bsdiff4 格式）。
// 产出的差量可被官方 bspatch 应用（纯 Go，不依赖 CGO）。
type bsdiffEngine struct{}

// newBsdiffEngine 构造 bsdiff 引擎（注册表静态使用）。
func newBsdiffEngine() Engine { return bsdiffEngine{} }

// Algo 返回算法标识 bsdiff。
func (bsdiffEngine) Algo() string { return AlgoBsdiff }

// Diff 计算标准 bsdiff4 差量（BSDIFF40 容器：bzip2 压缩的 ctrl/diff/extra 三段流）。
func (bsdiffEngine) Diff(oldData, newData []byte) ([]byte, error) {
	return bsdiff.Bytes(oldData, newData)
}

// Patch 应用 bsdiff4 差量还原新数据。
func (bsdiffEngine) Patch(oldData, delta []byte) ([]byte, error) {
	return bspatch.Bytes(oldData, delta)
}

// hasBSdiffMagic 判断字节流是否以 BSDIFF40 头开始（测试与文档示例使用）。
func hasBSdiffMagic(b []byte) bool { return bytes.HasPrefix(b, bsdiffMagic) }
