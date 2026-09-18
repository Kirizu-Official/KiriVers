#ifndef KIRIVERS_ADAPTERS_H
#define KIRIVERS_ADAPTERS_H

#include "kirivers/error.h"

#include <stddef.h>
#include <stdint.h>

#ifdef __cplusplus
extern "C" {
#endif

typedef struct KiriversHttpHeader {
    const char *name;
    const char *value;
} KiriversHttpHeader;

typedef struct KiriversHttpRequest {
    const char *method;
    const char *url;
    const KiriversHttpHeader *headers;
    size_t header_count;
    const uint8_t *body;
    size_t body_len;
} KiriversHttpRequest;

typedef struct KiriversHttpResponse {
    int status;
    char **header_names;
    char **header_values;
    size_t header_count;
    uint8_t *body;
    size_t body_len;
} KiriversHttpResponse;

/**
 * HTTP adapter (ctx + vtable). Must support Range and preserve query strings
 * (`exp`/`sig` on private downloads). Required for every network method.
 */
typedef struct KiriversTransport {
    void *ctx;
    int (*request)(void *ctx, const KiriversHttpRequest *req, KiriversHttpResponse *out,
                   KiriversError *err);
    void (*free_response)(void *ctx, KiriversHttpResponse *resp);
} KiriversTransport;

/**
 * Local tree adapter. Paths exchanged with the API use `/` + Unicode NFC.
 * Hosted default: POSIX/stdio. Embedded: caller function pointers.
 */
typedef struct KiriversFileStore {
    void *ctx;
    int (*normalize)(void *ctx, const char *raw, char **out, KiriversError *err);
    int (*exists)(void *ctx, const char *rel_path, int *out_exists, KiriversError *err);
    int (*read_all)(void *ctx, const char *rel_path, uint8_t **data, size_t *len,
                    KiriversError *err);
    int (*write_all)(void *ctx, const char *rel_path, const uint8_t *data, size_t len,
                     KiriversError *err);
    int (*remove_path)(void *ctx, const char *rel_path, KiriversError *err);
    /** Non-zero when individual files can be written (enables `file_list`). */
    int (*can_write_files)(void *ctx);
} KiriversFileStore;

/** SHA-256 (and MD5 when integrity asks). Hosted default: OpenSSL. */
typedef struct KiriversHasher {
    void *ctx;
    int (*sha256)(void *ctx, const uint8_t *data, size_t len, char out_hex[65],
                  KiriversError *err);
    int (*md5)(void *ctx, const uint8_t *data, size_t len, char out_hex[33],
               KiriversError *err);
} KiriversHasher;

/**
 * Apply one binary delta. No default implementation.
 * `supported_algos` must name only algorithms this apply() can actually run
 * (`bsdiff`, `xdelta3`, `hdiffpatch`). Unknown magic must fail; never cross-decode.
 */
typedef struct KiriversPatcher {
    void *ctx;
    int (*supported_algos)(void *ctx, const char *const **algos, size_t *count);
    int (*apply)(void *ctx, const uint8_t *old_bytes, size_t old_len, const uint8_t *delta,
                 size_t delta_len, uint8_t **out, size_t *out_len, KiriversError *err);
} KiriversPatcher;

/**
 * Replace the running install. No default: desktop/APK/flash is caller-owned.
 * Embedded flash programming is this callback.
 */
typedef struct KiriversReplacer {
    void *ctx;
    int (*replace)(void *ctx, const char *staged_path, const char *install_path,
                   KiriversError *err);
} KiriversReplacer;

/** Native zip unpack (hash-named members). Hosted default: libzip. */
typedef struct KiriversArchiveUnpacker {
    void *ctx;
    int (*extract_member)(void *ctx, const uint8_t *zip, size_t zip_len, const char *member_name,
                          uint8_t **out, size_t *out_len, KiriversError *err);
} KiriversArchiveUnpacker;

/**
 * Verify `signature` over BuildCheckPayload
 * (`integer\\nsemver\\nroot_hash\\npackage_url\\nsize\\nsha256`).
 * Algorithms: `ed25519`, `rsa-sha256` (standard base64). Hosted default: OpenSSL.
 */
typedef struct KiriversSignatureVerifier {
    void *ctx;
    int (*verify)(void *ctx, const char *algo, const char *payload, const char *sig_b64,
                  KiriversError *err);
} KiriversSignatureVerifier;

/** Optional clock for pack polling. NULL sleep uses libc Sleep/nanosleep when available. */
typedef struct KiriversClock {
    void *ctx;
    void (*sleep_ms)(void *ctx, unsigned milliseconds);
} KiriversClock;

typedef struct KiriversAdapters {
    KiriversTransport transport;
    KiriversFileStore file_store;
    KiriversHasher hasher;
    KiriversPatcher patcher;
    KiriversReplacer replacer;
    KiriversArchiveUnpacker unpacker;
    KiriversSignatureVerifier verifier;
    KiriversClock clock;
} KiriversAdapters;

/**
 * D13: `full_package` always. `patch_package` iff unpacker.extract_member.
 * `file_list` iff FileStore can write files. `binary_delta` + accepted_delta_algos
 * iff Patcher.apply is set and supported_algos names at least one algo.
 */
typedef struct KiriversCapabilitySet {
    char **capabilities;
    size_t n_capabilities;
    char **accepted_delta_algos;
    size_t n_algos;
} KiriversCapabilitySet;

int kirivers_derive_capabilities(const KiriversAdapters *adapters, KiriversCapabilitySet *out,
                                 KiriversError *err);
void kirivers_capability_set_free(KiriversCapabilitySet *set);

/**
 * Classify delta container magic. Returns `hdiffpatch`, `bsdiff`, `xdelta3`,
 * or NULL when unknown. Callers must not cross-decode.
 */
const char *kirivers_delta_algo_from_magic(const uint8_t *buf, size_t len);

/** BuildCheckPayload string. Caller frees. Empty fields become empty placeholders. */
char *kirivers_build_check_payload(const char *version_integer, const char *version_semver,
                                   const char *root_hash, const char *package_url,
                                   const char *size, const char *sha256_hex);

#ifdef __cplusplus
}
#endif

#endif
