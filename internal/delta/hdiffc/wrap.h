// wrap.h 是 libHDiffPatch 的 extern "C" 薄封装：内存缓冲上生成/应用未压缩 HDIFF13。
#ifndef KV_HDIFFC_WRAP_H
#define KV_HDIFFC_WRAP_H

#include <stddef.h>
#include <stdint.h>

#ifdef __cplusplus
extern "C" {
#endif

// kv_hdiff_create 用官方 create_compressed_diff（compressPlugin=NULL）生成未压缩 HDIFF13。
// 成功返回 0，*out_buf 由 malloc 分配，调用方必须 kv_hdiff_free。
// 负值：-1 参数非法；-2 库失败；-3 C++ 异常；-4 分配失败。
int kv_hdiff_create(const uint8_t *old_data, size_t old_len,
                    const uint8_t *new_data, size_t new_len,
                    uint8_t **out_buf, size_t *out_len);

// kv_hdiff_patch 应用未压缩 HDIFF13（patch_decompress，无解压插件）。
// 压缩容器（compressedCount>0 或 compressType 非空）返回 -5，不链接 zlib。
// 负值：-1 参数非法；-2 应用失败；-3 C++ 异常；-4 分配失败；-5 压缩差量；-6 newSize 溢出。
int kv_hdiff_patch(const uint8_t *old_data, size_t old_len,
                   const uint8_t *diff, size_t diff_len,
                   uint8_t **out_buf, size_t *out_len);

// kv_hdiff_free 释放 kv_hdiff_create / kv_hdiff_patch 返回的缓冲；p 可为 NULL。
void kv_hdiff_free(uint8_t *p);

#ifdef __cplusplus
}
#endif

#endif
