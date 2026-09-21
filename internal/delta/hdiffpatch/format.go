// Package hdiffpatch 提供历史 KVDIFFHP1 容器的纯 Go 编解码（Patch 兼容已落库对象）。
//
// 服务端 hdiffpatch 引擎的生成路径已改为 CGO（internal/delta/hdiffc，未压缩 HDIFF13）。
// 本包不再被引擎 Diff 调用；Diff 仅保留给解码器单测与历史夹具。容器布局：
//
//	容器布局（自描述，小端）：
//	  magic   10 字节  "KVDIFFHP1\n"（与官方 .hdiff 的 magic 不同，用于区分来源）
//	  oldLen  uint64   旧数据字节数
//	  newLen  uint64   新数据字节数
//	  oldSHA  32 字节  旧数据 SHA-256（Patch 前校验基线，防误用）
//	  newSHA  32 字节  新数据 SHA-256（Patch 后校验输出）
//	  zLen    uint64   zlib 压缩后的指令流字节数
//	  zlib    zLen 字节 压缩的指令流
//
//	指令流（未压缩形态，小端）：
//	  0x01 + uint64 oldPos + uint64 len   复制指令：从旧数据 oldPos 起复制 len 字节
//	  0x02 + uint64 len + len 字节        追加指令：内联 len 字节 extra 流
//	  0x00                                终止符
//
// 差量模型与 HDiffPatch 一致：匹配块（copy）+ extra 流（append）+ zlib 压缩。
// 重要：该容器 **不是** 官方 .hdiff 线格式；新生成一律走 HDIFF13&（见 docs/delta-engines.md）。
package hdiffpatch

import (
	"bytes"
	"compress/zlib"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
)

// Magic 是本回退容器的自描述 magic（10 字节）。
// 官方 .hdiff 以 "HDIFF13&" 开头，两者不会混淆；Patch 按 magic 自动分流。
var Magic = []byte("KVDIFFHP1\n")

// 算法调参常量（§7.2 依赖的回退实现内部参数）。
const (
	// matchBlockSize 是索引与候选匹配的基本窗口（字节）。
	matchBlockSize = 16
	// matchMinCopyLen 是 Emit copy 指令的最短匹配长度：短于该值的匹配并入 extra 流。
	matchMinCopyLen = matchBlockSize
	// maxCandidatesPerHash 是同一块哈希下最多尝试的候选旧位置数（限制病态数据耗时）。
	maxCandidatesPerHash = 8
)

// 容器错误。
var (
	ErrBadMagic      = errors.New("hdiffpatch fallback: bad magic")
	ErrMalformed     = errors.New("hdiffpatch fallback: malformed delta")
	ErrOldMismatch   = errors.New("hdiffpatch fallback: old data mismatch (sha256)")
	ErrOutputCorrupt = errors.New("hdiffpatch fallback: patched output corrupt (sha256)")
)

// Diff 计算旧数据→新数据的回退容器差量。
//
// 算法：对旧数据按 matchBlockSize 窗口建立「块哈希 → 首个位置」索引；
// 线性扫描新数据，命中且可延伸的匹配段（≥ matchMinCopyLen）输出复制指令，
// 其余字节进入 extra 流；最后对指令流做 zlib 压缩并写入自描述头。
// 输出确定性：同输入必产生同输出（无时间戳、无随机性）。
func Diff(oldData, newData []byte) ([]byte, error) {
	stream, err := buildInstructionStream(oldData, newData)
	if err != nil {
		return nil, err
	}

	var zbuf bytes.Buffer
	zw := zlib.NewWriter(&zbuf)
	if _, err := zw.Write(stream.Bytes()); err != nil {
		return nil, fmt.Errorf("hdiffpatch fallback: compress instructions: %w", err)
	}
	if err := zw.Close(); err != nil {
		return nil, fmt.Errorf("hdiffpatch fallback: finish compress: %w", err)
	}

	oldSum := sha256.Sum256(oldData)
	newSum := sha256.Sum256(newData)

	out := make([]byte, 0, len(Magic)+8*2+sha256.Size*2+8+zbuf.Len())
	out = append(out, Magic...)
	out = binary.LittleEndian.AppendUint64(out, uint64(len(oldData)))
	out = binary.LittleEndian.AppendUint64(out, uint64(len(newData)))
	out = append(out, oldSum[:]...)
	out = append(out, newSum[:]...)
	out = binary.LittleEndian.AppendUint64(out, uint64(zbuf.Len()))
	out = append(out, zbuf.Bytes()...)
	return out, nil
}

// Patch 将回退容器差量应用到旧数据，还原新数据。
// 仅接受本包 Magic 的容器（官方 HDIFF13& 由 hdiffc.Apply 处理，见引擎层分流）。
func Patch(oldData, delta []byte) ([]byte, error) {
	if !bytes.HasPrefix(delta, Magic) {
		return nil, ErrBadMagic
	}
	off := len(Magic)
	need := func(n int) error {
		if off+n > len(delta) {
			return ErrMalformed
		}
		return nil
	}
	if err := need(8 + 8 + sha256.Size*2 + 8); err != nil {
		return nil, err
	}
	oldLen := binary.LittleEndian.Uint64(delta[off:])
	off += 8
	newLen := binary.LittleEndian.Uint64(delta[off:])
	off += 8
	oldSum := delta[off : off+sha256.Size]
	off += sha256.Size
	newSum := delta[off : off+sha256.Size]
	off += sha256.Size
	zLen := binary.LittleEndian.Uint64(delta[off:])
	off += 8
	// 以 uint64 比较防 int 溢出（zLen > MaxInt64 时 int 转换回绕为负，会绕过边界检查）。
	if zLen > uint64(len(delta)-off) {
		return nil, ErrMalformed
	}
	zdata := delta[off : off+int(zLen)]

	// 基线校验：差量基线必须与容器记录一致（防旧数据被误传，等价于官方 patch 的 old 校验）。
	if uint64(len(oldData)) != oldLen {
		return nil, ErrOldMismatch
	}
	if sum := sha256.Sum256(oldData); !bytes.Equal(sum[:], oldSum) {
		return nil, ErrOldMismatch
	}

	zr, err := zlib.NewReader(bytes.NewReader(zdata))
	if err != nil {
		return nil, fmt.Errorf("%w: zlib header: %v", ErrMalformed, err)
	}
	defer zr.Close()
	// 解压指令流；以 newLen 导出硬顶，防 zip-bomb 式病态输入。
	stream, err := io.ReadAll(io.LimitReader(zr, int64(streamLimit(newLen))))
	if err != nil {
		return nil, fmt.Errorf("%w: zlib read: %v", ErrMalformed, err)
	}

	out, err := applyInstructionStream(oldData, stream, newLen)
	if err != nil {
		return nil, err
	}
	if uint64(len(out)) != newLen {
		return nil, ErrOutputCorrupt
	}
	if sum := sha256.Sum256(out); !bytes.Equal(sum[:], newSum) {
		return nil, ErrOutputCorrupt
	}
	return out, nil
}

// streamLimit 是解压指令流的硬顶（防 zip-bomb 式病态输入）：
// 指令流上界 ≈ 19 字节/复制指令 + 9+1 字节/extra 字节，放宽到 max(newLen*10, 64KiB)。
func streamLimit(newLen uint64) uint64 {
	limit := newLen*10 + 4096
	if limit < 1<<16 {
		limit = 1 << 16
	}
	return limit
}
