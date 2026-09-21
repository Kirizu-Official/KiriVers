// Package hdiffc 通过 CGO 链接官方 libHDiffPatch，在进程内生成/应用未压缩 HDIFF13。
//
// 生成对齐无参数 `hdiffz old new diff`：create_compressed_diff(compressPlugin=NULL)，
// magic 为 HDIFF13&。压缩差量不支持（不链接 zlib/zstd）。空 old/new 用非 NULL 占位。
package hdiffc

/*
#cgo CFLAGS: -I${SRCDIR}/../../../third_party/hdiffpatch -D_IS_NEED_DIR_DIFF_PATCH=0 -D_IS_NEED_BSDIFF=0 -D_IS_NEED_VCDIFF=0 -D_IS_USED_MULTITHREAD=0 -D_IS_NEED_DEFAULT_CompressPlugin=0 -D_IS_NEED_ALL_CompressPlugin=0 -D_IS_NEED_DEFAULT_ChecksumPlugin=0 -D_IS_NEED_ALL_ChecksumPlugin=0 -D_IS_OUT_DIFF_INFO=0 -DNDEBUG
#cgo CXXFLAGS: -std=c++11 -I${SRCDIR}/../../../third_party/hdiffpatch -D_IS_NEED_DIR_DIFF_PATCH=0 -D_IS_NEED_BSDIFF=0 -D_IS_NEED_VCDIFF=0 -D_IS_USED_MULTITHREAD=0 -D_IS_NEED_DEFAULT_CompressPlugin=0 -D_IS_NEED_ALL_CompressPlugin=0 -D_IS_NEED_DEFAULT_ChecksumPlugin=0 -D_IS_NEED_ALL_ChecksumPlugin=0 -D_IS_OUT_DIFF_INFO=0 -DNDEBUG
#cgo linux LDFLAGS: -lstdc++
#cgo windows LDFLAGS: -lstdc++
#cgo darwin LDFLAGS: -lc++
#include "wrap.h"
*/
import "C"

import (
	"fmt"
	"runtime"
	"unsafe"
)

// Magic 是未压缩官方 HDIFF13 容器头（与 hdiffz 默认产物一致）。
var Magic = []byte("HDIFF13&")

// Create 由 old→new 生成未压缩 HDIFF13。失败不回退其它格式。
func Create(oldData, newData []byte) ([]byte, error) {
	var out *C.uint8_t
	var n C.size_t
	oldp, oldn := cbuf(oldData)
	newp, newn := cbuf(newData)
	rc := C.kv_hdiff_create(oldp, oldn, newp, newn, &out, &n)
	runtime.KeepAlive(oldData)
	runtime.KeepAlive(newData)
	if rc != 0 {
		return nil, fmt.Errorf("hdiffc create: %s", errText(int(rc)))
	}
	defer C.kv_hdiff_free(out)
	return goBytes(out, n), nil
}

// Apply 将未压缩 HDIFF13 应用到 old。压缩容器返回错误。
func Apply(oldData, diff []byte) ([]byte, error) {
	var out *C.uint8_t
	var n C.size_t
	oldp, oldn := cbuf(oldData)
	diffp, diffn := cbuf(diff)
	rc := C.kv_hdiff_patch(oldp, oldn, diffp, diffn, &out, &n)
	runtime.KeepAlive(oldData)
	runtime.KeepAlive(diff)
	if rc != 0 {
		return nil, fmt.Errorf("hdiffc apply: %s", errText(int(rc)))
	}
	defer C.kv_hdiff_free(out)
	return goBytes(out, n), nil
}

func cbuf(b []byte) (*C.uint8_t, C.size_t) {
	if len(b) == 0 {
		// wrap.cpp 对 0 长度改用静态占位；这里仍传 NULL+0。
		return nil, 0
	}
	return (*C.uint8_t)(unsafe.Pointer(&b[0])), C.size_t(len(b))
}

func goBytes(p *C.uint8_t, n C.size_t) []byte {
	if p == nil || n == 0 {
		return []byte{}
	}
	if uint64(n) > uint64(^uint(0)>>1) {
		// 无法在本机 int 范围内切片；视为失败路径不应到达。
		return []byte{}
	}
	src := unsafe.Slice((*byte)(unsafe.Pointer(p)), int(n))
	out := make([]byte, len(src))
	copy(out, src)
	return out
}

func errText(rc int) string {
	switch rc {
	case -1:
		return "invalid argument"
	case -2:
		return "libHDiffPatch failed"
	case -3:
		return "c++ exception"
	case -4:
		return "out of memory"
	case -5:
		return "compressed HDIFF13 is unsupported"
	case -6:
		return "new size overflows addressable memory"
	default:
		return fmt.Sprintf("error %d", rc)
	}
}
