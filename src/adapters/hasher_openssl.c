#include "kirivers/hosted.h"
#include "internal.h"

#include <openssl/evp.h>
#include <string.h>

static void to_hex(const unsigned char *in, size_t n, char *out) {
    static const char hex[] = "0123456789abcdef";
    size_t i;
    for (i = 0; i < n; i++) {
        out[i * 2] = hex[in[i] >> 4];
        out[i * 2 + 1] = hex[in[i] & 15];
    }
    out[n * 2] = 0;
}

static int digest(const EVP_MD *md, const uint8_t *data, size_t len, unsigned char *out,
                  unsigned int *out_len, KiriversError *err) {
    EVP_MD_CTX *ctx = EVP_MD_CTX_new();
    if (!ctx) {
        return kirivers_error_set(err, 0, "HASH", "EVP_MD_CTX_new failed", NULL);
    }
    if (EVP_DigestInit_ex(ctx, md, NULL) != 1 || EVP_DigestUpdate(ctx, data, len) != 1 ||
        EVP_DigestFinal_ex(ctx, out, out_len) != 1) {
        EVP_MD_CTX_free(ctx);
        return kirivers_error_set(err, 0, "HASH", "digest failed", NULL);
    }
    EVP_MD_CTX_free(ctx);
    return KIRIVERS_OK;
}

static int sha256_fn(void *ctx, const uint8_t *data, size_t len, char out_hex[65],
                     KiriversError *err) {
    unsigned char raw[32];
    unsigned int n = 0;
    (void)ctx;
    if (digest(EVP_sha256(), data, len, raw, &n, err) != KIRIVERS_OK) {
        return KIRIVERS_ERR;
    }
    to_hex(raw, 32, out_hex);
    return KIRIVERS_OK;
}

static int md5_fn(void *ctx, const uint8_t *data, size_t len, char out_hex[33], KiriversError *err) {
    unsigned char raw[16];
    unsigned int n = 0;
    (void)ctx;
    if (digest(EVP_md5(), data, len, raw, &n, err) != KIRIVERS_OK) {
        return KIRIVERS_ERR;
    }
    to_hex(raw, 16, out_hex);
    return KIRIVERS_OK;
}

int kirivers_hasher_openssl_init(KiriversHasher *out, KiriversError *err) {
    (void)err;
    memset(out, 0, sizeof(*out));
    out->sha256 = sha256_fn;
    out->md5 = md5_fn;
    return KIRIVERS_OK;
}

void kirivers_hasher_openssl_deinit(KiriversHasher *h) {
    if (h) {
        memset(h, 0, sizeof(*h));
    }
}
