package hdiffpatch

import (
	"bytes"
	"encoding/binary"
	"fmt"
)

// 指令流操作码（小端二进制，见包注释的容器布局）。
const (
	// opCopy 复制指令：从旧数据 oldPos 起复制 len 字节。
	opCopy byte = 0x01
	// opAppend 追加指令：内联 len 字节 extra 流。
	opAppend byte = 0x02
	// opEnd 终止符。
	opEnd byte = 0x00
)

// blockIndex 是旧数据的块哈希索引：哈希 → 候选旧位置（首个优先，最多 maxCandidatesPerHash 个）。
// 字节级校验在候选扩展时进行，哈希冲突不影响正确性。
type blockIndex struct {
	block   int
	old     []byte
	buckets map[uint64][]int32
}

// blockHash 计算 old/new 共用的块哈希（FNV-1a 64）。
// 只做候选定位，不承担正确性（候选命中后逐字节比对）。
func blockHash(b []byte) uint64 {
	const (
		offset64 uint64 = 14695981039346656037
		prime64  uint64 = 1099511628211
	)
	h := uint64(offset64)
	for _, c := range b {
		h ^= uint64(c)
		h *= prime64
	}
	return h
}

// newBlockIndex 为旧数据建立步长为 1 的块索引（窗口 = matchBlockSize）。
// 内存开销 ≈ 旧数据字节数 × 桶均摊 12 字节，属回退实现可接受量级（文档已注明）。
func newBlockIndex(old []byte, block int) *blockIndex {
	idx := &blockIndex{block: block, old: old, buckets: make(map[uint64][]int32)}
	last := len(old) - block
	for i := 0; i <= last; i++ {
		h := blockHash(old[i : i+block])
		list := idx.buckets[h]
		if len(list) < maxCandidatesPerHash {
			idx.buckets[h] = append(list, int32(i))
		}
	}
	return idx
}

// extend 验证候选位置 p 并向右延伸匹配，返回匹配长度（<block 表示候选无效）。
// 需要保证 i+block ≤ len(new) 且 p+block ≤ len(old)。
func (idx *blockIndex) extend(newData []byte, i, p int) int {
	if p < 0 || p+idx.block > len(idx.old) || i+idx.block > len(newData) {
		return 0
	}
	if !bytes.Equal(idx.old[p:p+idx.block], newData[i:i+idx.block]) {
		return 0
	}
	l := idx.block
	maxLen := len(newData) - i
	if len(idx.old)-p < maxLen {
		maxLen = len(idx.old) - p
	}
	for l < maxLen && idx.old[p+l] == newData[i+l] {
		l++
	}
	return l
}

// buildInstructionStream 扫描新数据生成指令流（copy/append 交错，opEnd 结尾）。
// 贪心策略：当前位置若能从旧数据复制（任一候选延伸 ≥ matchMinCopyLen），优先复制。
func buildInstructionStream(oldData, newData []byte) (*bytes.Buffer, error) {
	stream := &bytes.Buffer{}
	u64 := func(v uint64) {
		var buf [8]byte
		binary.LittleEndian.PutUint64(buf[:], v)
		stream.Write(buf[:])
	}

	// extraBuf 暂存未命中的新数据字节，凑成一个 append 指令一次性输出。
	var extraBuf []byte
	flushExtra := func() {
		if len(extraBuf) == 0 {
			return
		}
		stream.WriteByte(opAppend)
		u64(uint64(len(extraBuf)))
		stream.Write(extraBuf)
		extraBuf = extraBuf[:0]
	}

	// 空旧数据：整段新数据都是 extra（zlib 压缩兜底）。
	if len(oldData) < matchBlockSize {
		extraBuf = append(extraBuf, newData...)
		flushExtra()
		stream.WriteByte(opEnd)
		return stream, nil
	}

	idx := newBlockIndex(oldData, matchBlockSize)
	for i := 0; i < len(newData); {
		remaining := len(newData) - i
		bestLen := 0
		bestPos := 0
		if remaining >= matchBlockSize {
			h := blockHash(newData[i : i+matchBlockSize])
			if candidates, ok := idx.buckets[h]; ok {
				for _, c := range candidates {
					if l := idx.extend(newData, i, int(c)); l > bestLen {
						bestLen = l
						bestPos = int(c)
						if l == remaining {
							break // 已覆盖到新数据末尾，无需更长候选
						}
					}
				}
			}
		}
		if bestLen >= matchMinCopyLen {
			flushExtra()
			stream.WriteByte(opCopy)
			u64(uint64(bestPos))
			u64(uint64(bestLen))
			i += bestLen
			continue
		}
		extraBuf = append(extraBuf, newData[i])
		i++
	}
	flushExtra()
	stream.WriteByte(opEnd)
	return stream, nil
}

// applyInstructionStream 解码指令流并重建新数据（Patch 的执行体）。
// 任何越界/截断均返回 ErrMalformed；结果由调用方做长度与 SHA-256 校验。
func applyInstructionStream(oldData, stream []byte, newLen uint64) ([]byte, error) {
	// 预分配容量做上限截断：newLen 来自容器头（可能被构造为超大值），
	// 直接 make(uint64) 会触发分配 panic；实际输出长度仍由指令流与最终校验保证。
	capHint := newLen
	const maxPrealloc = 1 << 20
	if capHint > maxPrealloc {
		capHint = maxPrealloc
	}
	out := make([]byte, 0, capHint)
	rd := 0
	readU64 := func() (uint64, error) {
		if rd+8 > len(stream) {
			return 0, ErrMalformed
		}
		v := binary.LittleEndian.Uint64(stream[rd:])
		rd += 8
		return v, nil
	}
	for {
		if rd >= len(stream) {
			return nil, fmt.Errorf("%w: missing terminator", ErrMalformed)
		}
		switch op := stream[rd]; op {
		case opEnd:
			return out, nil
		case opCopy:
			rd++
			pos, err := readU64()
			if err != nil {
				return nil, err
			}
			length, err := readU64()
			if err != nil {
				return nil, err
			}
			if pos > uint64(len(oldData)) || length > uint64(len(oldData))-pos {
				return nil, fmt.Errorf("%w: copy out of range", ErrMalformed)
			}
			out = append(out, oldData[pos:pos+length]...)
		case opAppend:
			rd++
			length, err := readU64()
			if err != nil {
				return nil, err
			}
			if length > uint64(len(stream)-rd) {
				return nil, fmt.Errorf("%w: append out of range", ErrMalformed)
			}
			out = append(out, stream[rd:rd+int(length)]...)
			rd += int(length)
		default:
			return nil, fmt.Errorf("%w: unknown op 0x%02x", ErrMalformed, op)
		}
	}
}
