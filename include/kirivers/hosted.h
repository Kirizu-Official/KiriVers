#ifndef KIRIVERS_HOSTED_H
#define KIRIVERS_HOSTED_H

#include "kirivers/adapters.h"

#ifdef __cplusplus
extern "C" {
#endif

#if defined(KIRIVERS_HOSTED) && KIRIVERS_HOSTED

/** libcurl Transport. Pair with kirivers_transport_curl_deinit. */
int kirivers_transport_curl_init(KiriversTransport *out, KiriversError *err);
void kirivers_transport_curl_deinit(KiriversTransport *t);

/** stdio FileStore rooted at `root_dir`. NFC via utf8proc. */
int kirivers_filestore_stdio_init(KiriversFileStore *out, const char *root_dir, KiriversError *err);
void kirivers_filestore_stdio_deinit(KiriversFileStore *fs);

/** OpenSSL SHA-256 / MD5. */
int kirivers_hasher_openssl_init(KiriversHasher *out, KiriversError *err);
void kirivers_hasher_openssl_deinit(KiriversHasher *h);

/** libzip member extract (name = content hash). */
int kirivers_unpacker_libzip_init(KiriversArchiveUnpacker *out, KiriversError *err);
void kirivers_unpacker_libzip_deinit(KiriversArchiveUnpacker *u);

/**
 * OpenSSL Ed25519 / RSA-SHA256 verifier.
 * Pass PEM public keys (either may be NULL).
 */
int kirivers_verifier_openssl_init(KiriversSignatureVerifier *out, const char *ed25519_pem,
                                   const char *rsa_pem, KiriversError *err);
void kirivers_verifier_openssl_deinit(KiriversSignatureVerifier *v);

/**
 * Fill hosted defaults: curl + stdio + openssl hasher + libzip.
 * Patcher and Replacer stay NULL. `file_root` may be NULL (then FileStore is unset).
 */
int kirivers_hosted_adapters_init(KiriversAdapters *out, const char *file_root, KiriversError *err);
void kirivers_hosted_adapters_deinit(KiriversAdapters *a);

#endif /* KIRIVERS_HOSTED */

#ifdef __cplusplus
}
#endif

#endif
