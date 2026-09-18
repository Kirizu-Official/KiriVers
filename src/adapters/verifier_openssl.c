#include "kirivers/hosted.h"
#include "internal.h"

#include <openssl/evp.h>
#include <openssl/pem.h>
#include <openssl/err.h>
#include <openssl/bio.h>
#include <openssl/buffer.h>
#include <string.h>
#include <stdlib.h>

typedef struct VerifierState {
    EVP_PKEY *ed25519;
    EVP_PKEY *rsa;
} VerifierState;

static unsigned char *b64_decode(const char *s, size_t *out_len) {
    BIO *b64, *mem;
    unsigned char *buf;
    long n;
    size_t slen = strlen(s);
    buf = (unsigned char *)malloc(slen + 1);
    if (!buf) {
        return NULL;
    }
    mem = BIO_new_mem_buf(s, (int)slen);
    b64 = BIO_new(BIO_f_base64());
    BIO_set_flags(b64, BIO_FLAGS_BASE64_NO_NL);
    mem = BIO_push(b64, mem);
    n = BIO_read(mem, buf, (int)slen);
    BIO_free_all(mem);
    if (n <= 0) {
        free(buf);
        return NULL;
    }
    *out_len = (size_t)n;
    return buf;
}

static EVP_PKEY *load_pem(const char *pem) {
    BIO *b;
    EVP_PKEY *key;
    if (!pem || !pem[0]) {
        return NULL;
    }
    b = BIO_new_mem_buf(pem, -1);
    if (!b) {
        return NULL;
    }
    key = PEM_read_bio_PUBKEY(b, NULL, NULL, NULL);
    BIO_free(b);
    return key;
}

static int verify_fn(void *ctx, const char *algo, const char *payload, const char *sig_b64,
                     KiriversError *err) {
    VerifierState *st = (VerifierState *)ctx;
    unsigned char *sig;
    size_t sig_len = 0;
    EVP_PKEY *key;
    EVP_MD_CTX *mdctx;
    int ok;
    int is_ed = algo && strcmp(algo, "ed25519") == 0;
    int is_rsa = algo && strcmp(algo, "rsa-sha256") == 0;
    if (!is_ed && !is_rsa) {
        return kirivers_error_set(err, 0, "UNSUPPORTED", "unknown signature algorithm", NULL);
    }
    key = is_ed ? st->ed25519 : st->rsa;
    if (!key) {
        return kirivers_error_set(err, 0, "SIGNATURE_MISMATCH", "no public key configured", NULL);
    }
    sig = b64_decode(sig_b64 ? sig_b64 : "", &sig_len);
    if (!sig) {
        return kirivers_error_set(err, 0, "SIGNATURE_MISMATCH", "invalid base64 signature", NULL);
    }
    mdctx = EVP_MD_CTX_new();
    if (!mdctx) {
        free(sig);
        return kirivers_error_set(err, 0, "SIGNATURE_MISMATCH", "EVP_MD_CTX_new failed", NULL);
    }
    if (is_ed) {
        ok = EVP_DigestVerifyInit(mdctx, NULL, NULL, NULL, key) == 1 &&
             EVP_DigestVerify(mdctx, sig, sig_len, (const unsigned char *)payload, strlen(payload)) ==
                 1;
    } else {
        ok = EVP_DigestVerifyInit(mdctx, NULL, EVP_sha256(), NULL, key) == 1 &&
             EVP_DigestVerify(mdctx, sig, sig_len, (const unsigned char *)payload, strlen(payload)) ==
                 1;
    }
    EVP_MD_CTX_free(mdctx);
    free(sig);
    if (!ok) {
        return kirivers_error_set(err, 0, "SIGNATURE_MISMATCH", "signature mismatch", NULL);
    }
    return KIRIVERS_OK;
}

int kirivers_verifier_openssl_init(KiriversSignatureVerifier *out, const char *ed25519_pem,
                                   const char *rsa_pem, KiriversError *err) {
    VerifierState *st;
    memset(out, 0, sizeof(*out));
    st = (VerifierState *)calloc(1, sizeof(*st));
    if (!st) {
        return kirivers_error_set(err, 0, "INTERNAL_ERROR", "oom", NULL);
    }
    st->ed25519 = load_pem(ed25519_pem);
    st->rsa = load_pem(rsa_pem);
    out->ctx = st;
    out->verify = verify_fn;
    return KIRIVERS_OK;
}

void kirivers_verifier_openssl_deinit(KiriversSignatureVerifier *v) {
    if (!v) {
        return;
    }
    if (v->ctx) {
        VerifierState *st = (VerifierState *)v->ctx;
        EVP_PKEY_free(st->ed25519);
        EVP_PKEY_free(st->rsa);
        free(st);
    }
    memset(v, 0, sizeof(*v));
}
