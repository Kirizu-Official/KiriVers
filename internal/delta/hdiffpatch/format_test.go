package hdiffpatch

import (
	"bytes"
	"compress/zlib"
	"crypto/sha256"
	"encoding/binary"
	"testing"
)

// TestFallbackRoundTrip 回退容器 roundtrip：含重叠块、纯追加、就地改写与空输入边界。
func TestFallbackRoundTrip(t *testing.T) {
	cases := []struct {
		name string
		old  string
		new  string
	}{
		{"overlap", "the quick brown fox jumps over the lazy dog", "the quick brown fox leaps over the lazy dog today"},
		{"pure append", "AAAABBBBCCCCDDDDEEEEFFFF", "AAAABBBBCCCCDDDDEEEEFFFFGGGGHHHHIIIIJJJJ"},
		{"identical", "same bytes same bytes", "same bytes same bytes"},
		{"empty old", "", "brand new content entirely"},
		{"empty new", "old content that will be dropped", ""},
		{"shrink", "0123456789abcdef0123456789abcdef", "0123"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			d, err := Diff([]byte(tc.old), []byte(tc.new))
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.HasPrefix(d, Magic) {
				t.Fatalf("fallback delta must start with KVDIFFHP1 magic, got % x", d[:len(Magic)])
			}
			got, err := Patch([]byte(tc.old), d)
			if err != nil {
				t.Fatal(err)
			}
			if string(got) != tc.new {
				t.Fatalf("roundtrip mismatch: %q != %q", got, tc.new)
			}
		})
	}
}

// TestFallbackRejectsWrongOld 基线校验：旧数据与容器记录不符必须拒绝（等价官方 old 校验）。
func TestFallbackRejectsWrongOld(t *testing.T) {
	d, err := Diff([]byte("original baseline data here"), []byte("original baseline data here v2"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Patch([]byte("tampered baseline data!!"), d); err == nil {
		t.Fatal("wrong old data must be rejected")
	}
	if _, err := Patch([]byte("original baseline data here"), []byte("garbage")); err == nil {
		t.Fatal("garbage delta must be rejected")
	}
}

// TestFallbackRejectsCraftedHeaders 构造的畸形容器头必须返回错误而非 panic（防溢出回绕）：
//   - zLen 超过 MaxInt64（int 转换回绕为负会绕过边界检查）；
//   - newLen 超大（预分配容量 panic）。
func TestFallbackRejectsCraftedHeaders(t *testing.T) {
	old := []byte("baseline")
	oldSum := sha256.Sum256(old)
	mkHeader := func(oldLen, newLen uint64) []byte {
		h := append([]byte{}, Magic...)
		h = binary.LittleEndian.AppendUint64(h, oldLen)
		h = binary.LittleEndian.AppendUint64(h, newLen)
		h = append(h, oldSum[:]...)
		h = append(h, make([]byte, sha256.Size)...) // newSum 占位
		return h
	}

	// zLen = MaxUint64 → int 转换回绕，必须报 ErrMalformed 而非切片 panic。
	bad := binary.LittleEndian.AppendUint64(mkHeader(uint64(len(old)), 16), ^uint64(0))
	if _, err := Patch(old, bad); err == nil {
		t.Fatal("huge zLen must be rejected")
	}

	// newLen 超大 → 预分配必须被截断并最终报错，而非分配 panic。
	// zlib 流给出一段合法但无法产出 newLen 字节的指令流。
	var zbuf bytes.Buffer
	zw := zlib.NewWriter(&zbuf)
	zw.Write([]byte{opEnd})
	zw.Close()
	craft := binary.LittleEndian.AppendUint64(mkHeader(uint64(len(old)), ^uint64(0)), uint64(zbuf.Len()))
	craft = append(craft, zbuf.Bytes()...)
	if _, err := Patch(old, craft); err == nil {
		t.Fatal("huge newLen with short stream must be rejected")
	}

	// 复制指令 pos+length 溢出回绕（pos=1, length=MaxUint64 → 和为 0），
	// 必须报 ErrMalformed 而非切片 panic。
	var cbuf bytes.Buffer
	cw := zlib.NewWriter(&cbuf)
	cw.Write([]byte{opCopy})
	var u64 [8]byte
	binary.LittleEndian.PutUint64(u64[:], 1)
	cw.Write(u64[:])
	binary.LittleEndian.PutUint64(u64[:], ^uint64(0))
	cw.Write(u64[:])
	cw.Close()
	craft2 := binary.LittleEndian.AppendUint64(mkHeader(uint64(len(old)), 8), uint64(cbuf.Len()))
	craft2 = append(craft2, cbuf.Bytes()...)
	if _, err := Patch(old, craft2); err == nil {
		t.Fatal("overflowing copy instruction must be rejected")
	}
}

// TestFallbackBinaryFixture 二进制夹具（约 64KiB、50% 重叠）roundtrip 与非空差量。
func TestFallbackBinaryFixture(t *testing.T) {
	size := 64 * 1024
	oldData := make([]byte, size)
	newData := make([]byte, size)
	for i := 0; i < size; i++ {
		oldData[i] = byte(i*31 + i/97)
	}
	copy(newData, oldData[:size/2])
	for i := size / 2; i < size; i++ {
		newData[i] = byte(i ^ 0xA5)
	}

	d, err := Diff(oldData, newData)
	if err != nil {
		t.Fatal(err)
	}
	if len(d) == 0 || len(d) >= size*20 {
		t.Fatalf("suspicious delta size: %d", len(d))
	}
	got, err := Patch(oldData, d)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, newData) {
		t.Fatalf("binary roundtrip mismatch")
	}
}
