package delta

import (
	"bytes"

	"github.com/go-deltasync/vcdiff"
)

// xdelta3Magic 是 RFC 3284 VCDIFF 容器头前缀（D6 C3 C4 = 'V'|0x80 'C'|0x80 'D'|0x80，
// RFC 3284 §5.1；xdelta3 / open-vcdiff 等互操作实现共用，C10-8：格式可识别）。
var xdelta3Magic = []byte{0xD6, 0xC3, 0xC4}

// xdelta3Engine 基于 github.com/go-deltasync/vcdiff（纯 Go RFC 3284 实现，
// 默认 code table + 独立 section + 无二级压缩，与 xdelta3 默认模式互操作）。
type xdelta3Engine struct{}

// newXdelta3Engine 构造 xdelta3（vcdiff）引擎（注册表静态使用）。
func newXdelta3Engine() Engine { return xdelta3Engine{} }

// Algo 返回算法标识 xdelta3。
func (xdelta3Engine) Algo() string { return AlgoXdelta3 }

// Diff 计算源→目标的 VCDIFF 差量。
// 空旧数据产生自引用（纯压缩）delta，仍可被本引擎 Patch 还原。
func (xdelta3Engine) Diff(oldData, newData []byte) ([]byte, error) {
	return vcdiff.EncodeBytes(oldData, newData), nil
}

// Patch 应用 VCDIFF 差量还原新数据。
func (xdelta3Engine) Patch(oldData, delta []byte) ([]byte, error) {
	var out bytes.Buffer
	if _, err := vcdiff.Decode(oldData, bytes.NewReader(delta), &out); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

// hasVcdiffMagic 判断字节流是否以 VCDIFF 头开始（测试与文档示例使用）。
func hasVcdiffMagic(b []byte) bool { return bytes.HasPrefix(b, xdelta3Magic) }

// 编译期确认 xdelta3 引擎满足 Engine 接口。
var _ Engine = xdelta3Engine{}
