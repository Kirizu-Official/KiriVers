#include "internal.h"
#include "kirivers/path.h"

#include <stdlib.h>
#include <string.h>

#ifdef KIRIVERS_HOSTED
#include <utf8proc.h>
#endif

static int is_drive_letter(const char *seg) {
    return seg && ((seg[0] >= 'A' && seg[0] <= 'Z') || (seg[0] >= 'a' && seg[0] <= 'z')) &&
           seg[1] == ':';
}

int kirivers_path_normalize(const char *raw, char **out, KiriversError *err) {
    size_t i, n;
    char *work;
    KiriversBuf b = {0};
    const char *p;
    int first = 1;

    if (!out) {
        return kirivers_error_set(err, 0, "INVALID_ARGUMENT", "path out is NULL", NULL);
    }
    *out = NULL;
    if (!raw) {
        return kirivers_error_set(err, 0, "INVALID_PATH", "path is empty", NULL);
    }
    while (*raw == ' ' || *raw == '\t') {
        raw++;
    }
    n = strlen(raw);
    while (n && (raw[n - 1] == ' ' || raw[n - 1] == '\t')) {
        n--;
    }
    if (n == 0) {
        return kirivers_error_set(err, 0, "INVALID_PATH", "path is empty", NULL);
    }
    for (i = 0; i < n; i++) {
        if ((unsigned char)raw[i] < 0x20) {
            return kirivers_error_set(err, 0, "INVALID_PATH", "contains control character", NULL);
        }
    }
    if (raw[0] == '/' || raw[0] == '\\') {
        return kirivers_error_set(err, 0, "INVALID_PATH", "path cannot start with leading slash",
                                  NULL);
    }
    if (is_drive_letter(raw)) {
        return kirivers_error_set(err, 0, "INVALID_PATH", "path cannot contain drive letter", NULL);
    }

    work = kirivers_strndup(raw, n);
    if (!work) {
        return kirivers_error_set(err, 0, "INTERNAL_ERROR", "oom", NULL);
    }
    for (i = 0; work[i]; i++) {
        if (work[i] == '\\') {
            work[i] = '/';
        }
    }

    p = work;
    while (*p) {
        const char *slash = strchr(p, '/');
        size_t seglen = slash ? (size_t)(slash - p) : strlen(p);
        if (seglen == 0) {
            /* collapse // */
            p = slash ? slash + 1 : p + 1;
            continue;
        }
        if ((seglen == 1 && p[0] == '.') || (seglen == 2 && p[0] == '.' && p[1] == '.')) {
            free(work);
            kirivers_buf_free(&b);
            return kirivers_error_set(err, 0, "INVALID_PATH", "path traversal segment is forbidden",
                                      NULL);
        }
        if (is_drive_letter(p)) {
            free(work);
            kirivers_buf_free(&b);
            return kirivers_error_set(err, 0, "INVALID_PATH", "segment cannot contain drive letter",
                                      NULL);
        }
        if (!first && kirivers_buf_puts(&b, "/") != KIRIVERS_OK) {
            free(work);
            kirivers_buf_free(&b);
            return kirivers_error_set(err, 0, "INTERNAL_ERROR", "oom", NULL);
        }
        first = 0;
        if (kirivers_buf_append(&b, p, seglen) != KIRIVERS_OK) {
            free(work);
            kirivers_buf_free(&b);
            return kirivers_error_set(err, 0, "INTERNAL_ERROR", "oom", NULL);
        }
        if (!slash) {
            break;
        }
        p = slash + 1;
    }
    free(work);
    if (!b.data || b.len == 0) {
        kirivers_buf_free(&b);
        return kirivers_error_set(err, 0, "INVALID_PATH", "path normalized to empty", NULL);
    }

#ifdef KIRIVERS_HOSTED
    {
        utf8proc_uint8_t *nfc = NULL;
        utf8proc_ssize_t rc =
            utf8proc_map((const utf8proc_uint8_t *)b.data, (utf8proc_ssize_t)b.len, &nfc,
                         (utf8proc_option_t)(UTF8PROC_STABLE | UTF8PROC_COMPOSE | UTF8PROC_NULLTERM));
        kirivers_buf_free(&b);
        if (rc < 0 || !nfc) {
            free(nfc);
            return kirivers_error_set(err, 0, "INVALID_PATH", "NFC normalization failed", NULL);
        }
        *out = (char *)nfc;
        return KIRIVERS_OK;
    }
#else
    *out = b.data;
    return KIRIVERS_OK;
#endif
}
