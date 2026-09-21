package delta

import (
	"bytes"
	"fmt"

	"github.com/Kirizu-Official/KiriVers/internal/delta/hdiffc"
	"github.com/Kirizu-Official/KiriVers/internal/delta/hdiffpatch"
)

// officialHdiffMagic 是未压缩官方 HDIFF13 容器头（hdiffc / 历史 hdiffz 默认产物）。
var officialHdiffMagic = []byte("HDIFF13&")

// hdiffPatchEngine 通过 CGO 链接 libHDiffPatch：Diff 只生成未压缩 HDIFF13，
// Patch 按 magic 分流——KVDIFFHP1 走纯 Go 历史解码器，HDIFF13& 走 hdiffc.Apply。
type hdiffPatchEngine struct{}

// newHDiffPatchEngine 构造 hdiffpatch 引擎（注册表静态使用）。
func newHDiffPatchEngine() Engine { return hdiffPatchEngine{} }

// Algo 返回算法标识 hdiffpatch。
func (hdiffPatchEngine) Algo() string { return AlgoHDiffPatch }

// Diff 生成未压缩 HDIFF13。失败原样返回，不回退 KVDIFFHP1。
func (hdiffPatchEngine) Diff(oldData, newData []byte) ([]byte, error) {
	return hdiffc.Create(oldData, newData)
}

// Patch 按 magic 分流：KVDIFFHP1 → 纯 Go；HDIFF13& → CGO；其它报错，禁止交叉解码。
func (hdiffPatchEngine) Patch(oldData, delta []byte) ([]byte, error) {
	switch {
	case bytes.HasPrefix(delta, hdiffpatch.Magic):
		return hdiffpatch.Patch(oldData, delta)
	case bytes.HasPrefix(delta, officialHdiffMagic):
		return hdiffc.Apply(oldData, delta)
	default:
		return nil, fmt.Errorf("hdiffpatch: unrecognized delta container (neither %s nor %s)",
			hdiffpatch.Magic, officialHdiffMagic)
	}
}

// 编译期确认 hdiffpatch 引擎满足 Engine 接口。
var _ Engine = hdiffPatchEngine{}
