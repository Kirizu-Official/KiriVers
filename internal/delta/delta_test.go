package delta

import (
	"bytes"
	"errors"
	"math/rand/v2"
	"testing"

	"github.com/Kirizu-Official/KiriVers/internal/delta/hdiffpatch"
)

// fixtureSize 是引擎测试夹具尺寸（约 64KiB，任务 design §1：小 fixture）。
const fixtureSize = 64 * 1024

// makeOverlapFixture 生成 old/new 各 64KiB 且部分重叠的字节串：
// new = old 前半（重叠 50%）+ 随机新内容 + 少量就地改写，模拟真实二进制版本的常见形态
// （共享段、追加段、局部修改）。
func makeOverlapFixture(seed uint64) (oldData, newData []byte) {
	rng := rand.New(rand.NewPCG(seed, seed^0x9e3779b9))
	oldData = make([]byte, fixtureSize)
	for i := range oldData {
		oldData[i] = byte(rng.IntN(256))
	}
	newData = make([]byte, fixtureSize)
	copy(newData, oldData[:fixtureSize/2])           // 前半与旧版本重叠
	for i := fixtureSize / 2; i < fixtureSize; i++ { // 后半全新
		newData[i] = byte(rng.IntN(256))
	}
	for i := 0; i < 16; i++ { // 就地改写若干处
		pos := rng.IntN(fixtureSize - 32)
		for j := 0; j < 8; j++ {
			newData[pos+j] ^= byte(0x5A + j)
		}
	}
	return oldData, newData
}

// TestRegistryGetUnknown 验收项：未知算法 → ErrUnsupported（映射 400 DELTA_ALGO_UNSUPPORTED）。
func TestRegistryGetUnknown(t *testing.T) {
	if _, err := Get("zstd-dict"); !errors.Is(err, ErrUnsupported) {
		t.Fatalf("unknown algo must return ErrUnsupported, got %v", err)
	}
	for _, algo := range Available() {
		if _, err := Get(algo); err != nil {
			t.Fatalf("registered algo %s must resolve: %v", algo, err)
		}
	}
	// 三种算法齐备（§7.2 已拍板）。
	if got := Available(); len(got) != 3 {
		t.Fatalf("expected 3 registered algos, got %v", got)
	}
}

// TestEngineRoundTrip 验收项（C10-8）：三种算法各对重叠 fixture 生成非空差量，
// Patch(old, Diff(old,new)) == new；hdiffpatch 必须是官方 HDIFF13&（不得再生成 KVDIFFHP1）。
func TestEngineRoundTrip(t *testing.T) {
	oldData, newData := makeOverlapFixture(1)

	for _, algo := range Available() {
		t.Run(algo, func(t *testing.T) {
			e, err := Get(algo)
			if err != nil {
				t.Fatal(err)
			}
			diffBytes, err := e.Diff(oldData, newData)
			if err != nil {
				t.Fatalf("diff: %v", err)
			}
			if len(diffBytes) == 0 {
				t.Fatal("delta output must be non-empty")
			}

			// magic 断言（C10-8：格式可识别）。
			switch algo {
			case AlgoBsdiff:
				if !hasBSdiffMagic(diffBytes) {
					t.Fatalf("bsdiff output must start with BSDIFF40 magic, got % x", diffBytes[:8])
				}
			case AlgoXdelta3:
				if !hasVcdiffMagic(diffBytes) {
					t.Fatalf("xdelta3 output must start with RFC 3284 magic, got % x", diffBytes[:6])
				}
			case AlgoHDiffPatch:
				if !bytes.HasPrefix(diffBytes, officialHdiffMagic) {
					t.Fatalf("hdiffpatch Diff must start with HDIFF13&, got % x", diffBytes[:min(10, len(diffBytes))])
				}
			}

			// roundtrip：Patch(old, diff) == new。
			restored, err := e.Patch(oldData, diffBytes)
			if err != nil {
				t.Fatalf("patch: %v", err)
			}
			if !bytes.Equal(restored, newData) {
				t.Fatalf("patched output differs from new data (%d vs %d bytes)", len(restored), len(newData))
			}
		})
	}
}

// TestEnginePatchHistoricalKVDIFFHP1 历史纯 Go 容器仍可由引擎 Patch，且不得走进 HDIFF13 解码器。
func TestEnginePatchHistoricalKVDIFFHP1(t *testing.T) {
	oldData, newData := makeOverlapFixture(4)
	legacy, err := hdiffpatch.Diff(oldData, newData)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(legacy, hdiffpatch.Magic) {
		t.Fatalf("fixture must be KVDIFFHP1, got % x", legacy[:min(10, len(legacy))])
	}
	e, err := Get(AlgoHDiffPatch)
	if err != nil {
		t.Fatal(err)
	}
	got, err := e.Patch(oldData, legacy)
	if err != nil {
		t.Fatalf("engine must patch KVDIFFHP1: %v", err)
	}
	if !bytes.Equal(got, newData) {
		t.Fatal("KVDIFFHP1 patch mismatch")
	}
}

// TestEngineOfficialNotCrossDecoded 官方 HDIFF13& 不得进入纯 Go 解码器；引擎 Patch 仍走 CGO。
func TestEngineOfficialNotCrossDecoded(t *testing.T) {
	oldData, newData := makeOverlapFixture(5)
	e, err := Get(AlgoHDiffPatch)
	if err != nil {
		t.Fatal(err)
	}
	official, err := e.Diff(oldData, newData)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(official, officialHdiffMagic) {
		t.Fatalf("want HDIFF13&, got % x", official[:min(10, len(official))])
	}
	if _, err := hdiffpatch.Patch(oldData, official); !errors.Is(err, hdiffpatch.ErrBadMagic) {
		t.Fatalf("official container must not enter pure-Go decoder, got %v", err)
	}
	got, err := e.Patch(oldData, official)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, newData) {
		t.Fatal("engine must still patch official HDIFF13")
	}
}

// TestDescribeAvailableCGO hdiffpatch 恒为 cgo，bsdiff/xdelta3 恒为 pure-go；无 official-cli。
func TestDescribeAvailableCGO(t *testing.T) {
	got := DescribeAvailable()
	if len(got) != 3 {
		t.Fatalf("expected 3 engines, got %v", got)
	}
	byAlgo := map[string]string{}
	for _, info := range got {
		byAlgo[info.Algo] = info.Implementation
		if info.Implementation == "official-cli" {
			t.Fatalf("official-cli must not appear: %+v", info)
		}
	}
	if byAlgo[AlgoHDiffPatch] != ImplCGO {
		t.Fatalf("hdiffpatch implementation=%q, want %q", byAlgo[AlgoHDiffPatch], ImplCGO)
	}
	if byAlgo[AlgoBsdiff] != ImplPureGo || byAlgo[AlgoXdelta3] != ImplPureGo {
		t.Fatalf("bsdiff/xdelta3 must stay pure-go: %v", byAlgo)
	}
}

// TestEngineEdgeCases 边界：空旧数据、空新数据、完全相同（零差量仍可 roundtrip）、
// 新数据远大于旧数据。
func TestEngineEdgeCases(t *testing.T) {
	oldData, newData := makeOverlapFixture(2)

	cases := []struct {
		name string
		old  []byte
		new  []byte
	}{
		{"empty old", nil, newData[:1024]},
		{"empty new", oldData[:1024], nil},
		{"identical", oldData, oldData},
		{"grow 10x", oldData[:4096], append(append([]byte{}, oldData[:4096]...), newData...)},
	}
	for _, algo := range Available() {
		e, _ := Get(algo)
		for _, tc := range cases {
			t.Run(algo+"/"+tc.name, func(t *testing.T) {
				diffBytes, err := e.Diff(tc.old, tc.new)
				if err != nil {
					t.Fatalf("diff: %v", err)
				}
				restored, err := e.Patch(tc.old, diffBytes)
				if err != nil {
					t.Fatalf("patch: %v", err)
				}
				if !bytes.Equal(restored, tc.new) {
					t.Fatalf("roundtrip mismatch: %d vs %d bytes", len(restored), len(tc.new))
				}
			})
		}
	}
}

// TestPatchRejectsGarbage Patch 对乱码差量必须报错而非 panic。
func TestPatchRejectsGarbage(t *testing.T) {
	oldData, _ := makeOverlapFixture(3)
	for _, algo := range Available() {
		e, _ := Get(algo)
		if _, err := e.Patch(oldData, []byte("not a delta")); err == nil {
			t.Fatalf("%s: garbage delta must be rejected", algo)
		}
	}
}
