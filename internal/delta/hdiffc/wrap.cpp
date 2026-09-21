// wrap.cpp 捕获 C++ 异常并拷贝 std::vector 到 malloc 缓冲，避免向 Go 泄漏 C++ 对象。
#include "wrap.h"

#include "libHDiffPatch/HDiff/diff.h"
#include "libHDiffPatch/HPatch/patch.h"

#include <cstdlib>
#include <cstring>
#include <limits>
#include <vector>

namespace {

// kEmpty 在 len==0 时提供非 NULL 占位：官方指针区间 API 在 nullptr+0 上不安全。
const uint8_t kEmpty = 0;

const uint8_t *nonNull(const uint8_t *p, size_t n) {
    if (p == 0 || n == 0) {
        return &kEmpty;
    }
    return p;
}

uint8_t *dupBytes(const unsigned char *src, size_t n) {
    if (n == 0) {
        // malloc(0) 实现定义；始终给出可 free 的非 NULL 指针，Go 侧仍按 0 长度拷贝。
        uint8_t *p = static_cast<uint8_t *>(std::malloc(1));
        return p;
    }
    uint8_t *p = static_cast<uint8_t *>(std::malloc(n));
    if (p == 0) {
        return 0;
    }
    std::memcpy(p, src, n);
    return p;
}

}  // namespace

int kv_hdiff_create(const uint8_t *old_data, size_t old_len,
                    const uint8_t *new_data, size_t new_len,
                    uint8_t **out_buf, size_t *out_len) {
    if (out_buf == 0 || out_len == 0) {
        return -1;
    }
    *out_buf = 0;
    *out_len = 0;
    try {
        const uint8_t *oldp = nonNull(old_data, old_len);
        const uint8_t *newp = nonNull(new_data, new_len);
        std::vector<unsigned char> diff;
        // 对齐无参数 hdiffz old new diff：未压缩 HDIFF13（magic HDIFF13&），
        // 而不是 create_diff() 的无类型头旧格式。threadNum=1。
        create_compressed_diff(newp, newp + new_len, oldp, oldp + old_len, diff,
                               0, kMinSingleMatchScore_default, false, 0, 1);
        uint8_t *buf = dupBytes(diff.empty() ? 0 : &diff[0], diff.size());
        if (buf == 0) {
            return -4;
        }
        *out_buf = buf;
        *out_len = diff.size();
        return 0;
    } catch (...) {
        return -3;
    }
}

int kv_hdiff_patch(const uint8_t *old_data, size_t old_len,
                   const uint8_t *diff, size_t diff_len,
                   uint8_t **out_buf, size_t *out_len) {
    if (out_buf == 0 || out_len == 0) {
        return -1;
    }
    *out_buf = 0;
    *out_len = 0;
    if (diff == 0 || diff_len == 0) {
        return -1;
    }
    try {
        const uint8_t *oldp = nonNull(old_data, old_len);
        const uint8_t *diffp = nonNull(diff, diff_len);
        hpatch_compressedDiffInfo info;
        std::memset(&info, 0, sizeof(info));
        if (!getCompressedDiffInfo_mem(&info, diffp, diffp + diff_len)) {
            return -2;
        }
        // 压缩 HDIFF13 需要 zlib/zstd 插件；本封装明确不链接。
        if (info.compressedCount > 0 || info.compressType[0] != '\0') {
            return -5;
        }
        if (info.newDataSize > (hpatch_StreamPos_t)std::numeric_limits<size_t>::max()) {
            return -6;
        }
        const size_t n = (size_t)info.newDataSize;
        uint8_t *out = static_cast<uint8_t *>(std::malloc(n == 0 ? 1 : n));
        if (out == 0) {
            return -4;
        }
        hpatch_BOOL ok = patch_decompress_mem(out, out + n, oldp, oldp + old_len,
                                              diffp, diffp + diff_len, 0);
        if (!ok) {
            std::free(out);
            return -2;
        }
        *out_buf = out;
        *out_len = n;
        return 0;
    } catch (...) {
        return -3;
    }
}

void kv_hdiff_free(uint8_t *p) {
    std::free(p);
}
