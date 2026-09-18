#include "kirivers/hosted.h"
#include "kirivers/path.h"
#include "internal.h"

#include <errno.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <sys/stat.h>

#ifdef _WIN32
#include <direct.h>
#define kirivers_mkdir(p) _mkdir(p)
#else
#include <unistd.h>
#define kirivers_mkdir(p) mkdir((p), 0755)
#endif

typedef struct StdioStore {
    char *root;
} StdioStore;

static void to_os_sep(char *p) {
#ifdef _WIN32
    size_t i;
    for (i = 0; p[i]; i++) {
        if (p[i] == '/') {
            p[i] = '\\';
        }
    }
#else
    (void)p;
#endif
}

static char *join_root(const StdioStore *st, const char *rel, KiriversError *err) {
    char *norm = NULL, *full, *os;
    if (kirivers_path_normalize(rel, &norm, err) != KIRIVERS_OK) {
        return NULL;
    }
    {
        KiriversBuf b = {0};
        kirivers_buf_puts(&b, st->root);
        if (b.len && b.data[b.len - 1] != '/' && b.data[b.len - 1] != '\\') {
#ifdef _WIN32
            kirivers_buf_puts(&b, "\\");
#else
            kirivers_buf_puts(&b, "/");
#endif
        }
        kirivers_buf_puts(&b, norm);
        free(norm);
        full = b.data;
    }
    if (!full) {
        kirivers_error_set(err, 0, "INTERNAL_ERROR", "oom", NULL);
        return NULL;
    }
    os = full;
    to_os_sep(os);
    return os;
}

static int ensure_parent(const char *path) {
    char *dup = kirivers_strdup(path);
    char *p;
    if (!dup) {
        return KIRIVERS_ERR;
    }
    for (p = dup + 1; *p; p++) {
#ifdef _WIN32
        if (*p == '\\' || *p == '/') {
#else
        if (*p == '/') {
#endif
            char save = *p;
            *p = 0;
            kirivers_mkdir(dup);
            *p = save;
        }
    }
    free(dup);
    return KIRIVERS_OK;
}

static int fs_normalize(void *ctx, const char *raw, char **out, KiriversError *err) {
    (void)ctx;
    return kirivers_path_normalize(raw, out, err);
}

static int fs_exists(void *ctx, const char *rel, int *out_exists, KiriversError *err) {
    StdioStore *st = (StdioStore *)ctx;
    char *path = join_root(st, rel, err);
    struct stat stbuf;
    if (!path) {
        return KIRIVERS_ERR;
    }
    *out_exists = (stat(path, &stbuf) == 0) ? 1 : 0;
    free(path);
    return KIRIVERS_OK;
}

static int fs_read(void *ctx, const char *rel, uint8_t **data, size_t *len, KiriversError *err) {
    StdioStore *st = (StdioStore *)ctx;
    char *path = join_root(st, rel, err);
    FILE *f;
    long sz;
    if (!path) {
        return KIRIVERS_ERR;
    }
    f = fopen(path, "rb");
    free(path);
    if (!f) {
        return kirivers_error_set(err, 0, "IO", "open for read failed", NULL);
    }
    if (fseek(f, 0, SEEK_END) != 0) {
        fclose(f);
        return kirivers_error_set(err, 0, "IO", "seek failed", NULL);
    }
    sz = ftell(f);
    if (sz < 0) {
        fclose(f);
        return kirivers_error_set(err, 0, "IO", "tell failed", NULL);
    }
    rewind(f);
    *data = (uint8_t *)malloc((size_t)sz + 1);
    if (!*data) {
        fclose(f);
        return kirivers_error_set(err, 0, "INTERNAL_ERROR", "oom", NULL);
    }
    if (sz && fread(*data, 1, (size_t)sz, f) != (size_t)sz) {
        free(*data);
        *data = NULL;
        fclose(f);
        return kirivers_error_set(err, 0, "IO", "read failed", NULL);
    }
    (*data)[sz] = 0;
    *len = (size_t)sz;
    fclose(f);
    return KIRIVERS_OK;
}

static int fs_write(void *ctx, const char *rel, const uint8_t *data, size_t len, KiriversError *err) {
    StdioStore *st = (StdioStore *)ctx;
    char *path = join_root(st, rel, err);
    FILE *f;
    if (!path) {
        return KIRIVERS_ERR;
    }
    ensure_parent(path);
    f = fopen(path, "wb");
    free(path);
    if (!f) {
        return kirivers_error_set(err, 0, "IO", "open for write failed", NULL);
    }
    if (len && fwrite(data, 1, len, f) != len) {
        fclose(f);
        return kirivers_error_set(err, 0, "IO", "write failed", NULL);
    }
    fclose(f);
    return KIRIVERS_OK;
}

static int fs_remove(void *ctx, const char *rel, KiriversError *err) {
    StdioStore *st = (StdioStore *)ctx;
    char *path = join_root(st, rel, err);
    if (!path) {
        return KIRIVERS_ERR;
    }
    if (remove(path) != 0 && errno != ENOENT) {
        free(path);
        return kirivers_error_set(err, 0, "IO", "remove failed", NULL);
    }
    free(path);
    return KIRIVERS_OK;
}

static int fs_can_write(void *ctx) {
    (void)ctx;
    return 1;
}

int kirivers_filestore_stdio_init(KiriversFileStore *out, const char *root_dir, KiriversError *err) {
    StdioStore *st;
    memset(out, 0, sizeof(*out));
    if (!root_dir || !root_dir[0]) {
        return kirivers_error_set(err, 0, "INVALID_ARGUMENT", "file store root required", NULL);
    }
    st = (StdioStore *)calloc(1, sizeof(*st));
    if (!st) {
        return kirivers_error_set(err, 0, "INTERNAL_ERROR", "oom", NULL);
    }
    st->root = kirivers_strdup(root_dir);
    out->ctx = st;
    out->normalize = fs_normalize;
    out->exists = fs_exists;
    out->read_all = fs_read;
    out->write_all = fs_write;
    out->remove_path = fs_remove;
    out->can_write_files = fs_can_write;
    return KIRIVERS_OK;
}

void kirivers_filestore_stdio_deinit(KiriversFileStore *fs) {
    if (!fs) {
        return;
    }
    if (fs->ctx) {
        StdioStore *st = (StdioStore *)fs->ctx;
        free(st->root);
        free(st);
    }
    memset(fs, 0, sizeof(*fs));
}
