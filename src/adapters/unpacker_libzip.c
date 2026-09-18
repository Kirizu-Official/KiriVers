#include "kirivers/hosted.h"
#include "internal.h"

#include <zip.h>
#include <stdlib.h>
#include <string.h>

static int extract_member(void *ctx, const uint8_t *zip_bytes, size_t zip_len, const char *member_name,
                          uint8_t **out, size_t *out_len, KiriversError *err) {
    zip_error_t zerr;
    zip_t *za;
    zip_source_t *src;
    zip_stat_t st;
    zip_file_t *zf;
    zip_int64_t n;
    (void)ctx;
    zip_error_init(&zerr);
    src = zip_source_buffer_create(zip_bytes, zip_len, 0, &zerr);
    if (!src) {
        zip_error_fini(&zerr);
        return kirivers_error_set(err, 0, "ZIP", "zip_source_buffer_create failed", NULL);
    }
    za = zip_open_from_source(src, ZIP_RDONLY, &zerr);
    if (!za) {
        zip_source_free(src);
        zip_error_fini(&zerr);
        return kirivers_error_set(err, 0, "ZIP", "zip_open_from_source failed", NULL);
    }
    if (zip_stat(za, member_name, 0, &st) != 0) {
        zip_close(za);
        zip_error_fini(&zerr);
        return kirivers_error_set(err, 0, "ZIP", "zip member not found", NULL);
    }
    zf = zip_fopen(za, member_name, 0);
    if (!zf) {
        zip_close(za);
        zip_error_fini(&zerr);
        return kirivers_error_set(err, 0, "ZIP", "zip_fopen failed", NULL);
    }
    *out = (uint8_t *)malloc((size_t)st.size + 1);
    if (!*out) {
        zip_fclose(zf);
        zip_close(za);
        zip_error_fini(&zerr);
        return kirivers_error_set(err, 0, "INTERNAL_ERROR", "oom", NULL);
    }
    n = zip_fread(zf, *out, st.size);
    zip_fclose(zf);
    zip_close(za);
    zip_error_fini(&zerr);
    if (n != (zip_int64_t)st.size) {
        free(*out);
        *out = NULL;
        return kirivers_error_set(err, 0, "ZIP", "zip_fread short read", NULL);
    }
    (*out)[st.size] = 0;
    *out_len = (size_t)st.size;
    return KIRIVERS_OK;
}

int kirivers_unpacker_libzip_init(KiriversArchiveUnpacker *out, KiriversError *err) {
    (void)err;
    memset(out, 0, sizeof(*out));
    out->extract_member = extract_member;
    return KIRIVERS_OK;
}

void kirivers_unpacker_libzip_deinit(KiriversArchiveUnpacker *u) {
    if (u) {
        memset(u, 0, sizeof(*u));
    }
}
