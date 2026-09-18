#include "kirivers/adapters.h"
#include "internal.h"

#include <stdlib.h>
#include <string.h>

void kirivers_capability_set_free(KiriversCapabilitySet *set) {
    if (!set) {
        return;
    }
    kirivers_free_string_array(set->capabilities, set->n_capabilities);
    kirivers_free_string_array(set->accepted_delta_algos, set->n_algos);
    memset(set, 0, sizeof(*set));
}

static int push_str(char ***arr, size_t *n, const char *s) {
    char **next = (char **)realloc(*arr, (*n + 1) * sizeof(char *));
    if (!next) {
        return KIRIVERS_ERR;
    }
    next[*n] = kirivers_strdup(s);
    if (!next[*n]) {
        free(next);
        return KIRIVERS_ERR;
    }
    *arr = next;
    (*n)++;
    return KIRIVERS_OK;
}

int kirivers_derive_capabilities(const KiriversAdapters *adapters, KiriversCapabilitySet *out,
                                 KiriversError *err) {
    int can_files = 0;
    memset(out, 0, sizeof(*out));
    if (push_str(&out->capabilities, &out->n_capabilities, "full_package") != KIRIVERS_OK) {
        kirivers_capability_set_free(out);
        return kirivers_error_set(err, 0, "INTERNAL_ERROR", "oom", NULL);
    }
    if (adapters && adapters->unpacker.extract_member) {
        if (push_str(&out->capabilities, &out->n_capabilities, "patch_package") != KIRIVERS_OK) {
            kirivers_capability_set_free(out);
            return kirivers_error_set(err, 0, "INTERNAL_ERROR", "oom", NULL);
        }
    }
    if (adapters && adapters->file_store.write_all) {
        can_files = 1;
        if (adapters->file_store.can_write_files) {
            can_files = adapters->file_store.can_write_files(adapters->file_store.ctx) ? 1 : 0;
        }
        if (can_files &&
            push_str(&out->capabilities, &out->n_capabilities, "file_list") != KIRIVERS_OK) {
            kirivers_capability_set_free(out);
            return kirivers_error_set(err, 0, "INTERNAL_ERROR", "oom", NULL);
        }
    }
    /* D13: advertise binary_delta only when apply() exists and lists real algos. */
    if (adapters && adapters->patcher.apply && adapters->patcher.supported_algos) {
        const char *const *algos = NULL;
        size_t n = 0, i;
        if (adapters->patcher.supported_algos(adapters->patcher.ctx, &algos, &n) == KIRIVERS_OK &&
            n > 0 && algos) {
            for (i = 0; i < n; i++) {
                if (algos[i] &&
                    push_str(&out->accepted_delta_algos, &out->n_algos, algos[i]) != KIRIVERS_OK) {
                    kirivers_capability_set_free(out);
                    return kirivers_error_set(err, 0, "INTERNAL_ERROR", "oom", NULL);
                }
            }
            if (out->n_algos > 0 &&
                push_str(&out->capabilities, &out->n_capabilities, "binary_delta") != KIRIVERS_OK) {
                kirivers_capability_set_free(out);
                return kirivers_error_set(err, 0, "INTERNAL_ERROR", "oom", NULL);
            }
        }
    }
    return KIRIVERS_OK;
}

const char *kirivers_delta_algo_from_magic(const uint8_t *buf, size_t len) {
    static const uint8_t vcdiff[3] = {0xD6, 0xC3, 0xC4};
    if (!buf || len == 0) {
        return NULL;
    }
    if (len >= 8 && memcmp(buf, "HDIFF13&", 8) == 0) {
        return "hdiffpatch";
    }
    if (len >= 10 && memcmp(buf, "KVDIFFHP1\n", 10) == 0) {
        return "hdiffpatch";
    }
    if (len >= 8 && memcmp(buf, "BSDIFF40", 8) == 0) {
        return "bsdiff";
    }
    if (len >= 3 && memcmp(buf, vcdiff, 3) == 0) {
        return "xdelta3";
    }
    return NULL;
}

char *kirivers_build_check_payload(const char *version_integer, const char *version_semver,
                                   const char *root_hash, const char *package_url,
                                   const char *size, const char *sha256_hex) {
    KiriversBuf b = {0};
    const char *a = version_integer ? version_integer : "";
    const char *b1 = version_semver ? version_semver : "";
    const char *c = root_hash ? root_hash : "";
    const char *d = package_url ? package_url : "";
    const char *e = size ? size : "";
    const char *f = sha256_hex ? sha256_hex : "";
    if (kirivers_buf_printf(&b, "%s\n%s\n%s\n%s\n%s\n%s", a, b1, c, d, e, f) != KIRIVERS_OK) {
        kirivers_buf_free(&b);
        return NULL;
    }
    return b.data;
}
