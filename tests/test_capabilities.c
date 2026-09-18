#include "kirivers/kirivers.h"
#include "test.h"

#include <string.h>

static int patcher_algos(void *ctx, const char *const **algos, size_t *n) {
    static const char *names[] = {"bsdiff", "xdelta3"};
    (void)ctx;
    *algos = names;
    *n = 2;
    return KIRIVERS_OK;
}

static int unpack_stub(void *ctx, const uint8_t *z, size_t zl, const char *name, uint8_t **out,
                       size_t *ol, KiriversError *err) {
    (void)ctx;
    (void)z;
    (void)zl;
    (void)name;
    (void)out;
    (void)ol;
    (void)err;
    return KIRIVERS_ERR;
}

static int apply_stub(void *ctx, const uint8_t *old_bytes, size_t old_len, const uint8_t *delta,
                      size_t delta_len, uint8_t **out, size_t *out_len, KiriversError *err) {
    (void)ctx;
    (void)old_bytes;
    (void)old_len;
    (void)delta;
    (void)delta_len;
    (void)out;
    (void)out_len;
    (void)err;
    return KIRIVERS_ERR;
}

static int write_stub(void *ctx, const char *rel, const uint8_t *d, size_t n, KiriversError *err) {
    (void)ctx;
    (void)rel;
    (void)d;
    (void)n;
    (void)err;
    return KIRIVERS_OK;
}

int main(void) {
    KiriversAdapters a;
    KiriversCapabilitySet caps;
    KiriversError err;
    size_t i;
    int saw_full = 0, saw_patch = 0, saw_files = 0, saw_delta = 0;

    memset(&a, 0, sizeof(a));
    memset(&err, 0, sizeof(err));
    EXPECT(kirivers_derive_capabilities(&a, &caps, &err) == KIRIVERS_OK);
    EXPECT(caps.n_capabilities == 1);
    EXPECT_STREQ(caps.capabilities[0], "full_package");
    EXPECT(caps.n_algos == 0);
    kirivers_capability_set_free(&caps);

    a.unpacker.extract_member = unpack_stub;
    a.file_store.write_all = write_stub;
    a.patcher.supported_algos = patcher_algos;
    EXPECT(kirivers_derive_capabilities(&a, &caps, &err) == KIRIVERS_OK);
    EXPECT(caps.n_algos == 0);
    {
        size_t j;
        int saw = 0;
        for (j = 0; j < caps.n_capabilities; j++) {
            if (strcmp(caps.capabilities[j], "binary_delta") == 0) {
                saw = 1;
            }
        }
        EXPECT(!saw);
    }
    kirivers_capability_set_free(&caps);

    a.patcher.apply = apply_stub;
    EXPECT(kirivers_derive_capabilities(&a, &caps, &err) == KIRIVERS_OK);
    for (i = 0; i < caps.n_capabilities; i++) {
        if (strcmp(caps.capabilities[i], "full_package") == 0) {
            saw_full = 1;
        }
        if (strcmp(caps.capabilities[i], "patch_package") == 0) {
            saw_patch = 1;
        }
        if (strcmp(caps.capabilities[i], "file_list") == 0) {
            saw_files = 1;
        }
        if (strcmp(caps.capabilities[i], "binary_delta") == 0) {
            saw_delta = 1;
        }
    }
    EXPECT(saw_full && saw_patch && saw_files && saw_delta);
    EXPECT(caps.n_algos == 2);
    EXPECT_STREQ(caps.accepted_delta_algos[0], "bsdiff");
    EXPECT_STREQ(caps.accepted_delta_algos[1], "xdelta3");
    kirivers_capability_set_free(&caps);
    printf("test_capabilities ok\n");
    return 0;
}
