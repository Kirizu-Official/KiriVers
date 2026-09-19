#ifndef KIRIVERS_UPDATER_H
#define KIRIVERS_UPDATER_H

#include "kirivers/adapters.h"
#include "kirivers/client.h"
#include "kirivers/error.h"
#include "kirivers/types.h"

#include <stddef.h>
#include <stdint.h>

#ifdef __cplusplus
extern "C" {
#endif

typedef struct KiriversUpdater KiriversUpdater;

typedef struct KiriversUpdaterConfig {
    KiriversClient *client; /**< Required; not owned. */
    KiriversAdapters adapters;
    const char *signature_algo; /**< ed25519 or rsa-sha256; used when signature is present. */
} KiriversUpdaterConfig;

typedef struct KiriversUpdateRequest {
    const char *current_version;
    const char *os;
    const char *arch;
    const char *channel;
    const char *hw_rev;
    const char *os_version;
    const char *device_id;
    const char *if_none_match;
    const char *local_sha256; /**< Single-file current package hash for POST /diff. */
    const char *install_dir;  /**< FileStore-relative root for integrity compare. */
    const char *stage_path;   /**< FileStore-relative path for verified bytes. */
    const char *install_path; /**< Passed to Replacer; ignored if Replacer is NULL. */
    int report_device;        /**< If set and device_id present, POST /clients/report first. */
} KiriversUpdateRequest;

#define KIRIVERS_UPDATE_NO_UPDATE 0
#define KIRIVERS_UPDATE_NOT_MODIFIED 1
#define KIRIVERS_UPDATE_STAGED 2
#define KIRIVERS_UPDATE_REPLACED 3

typedef struct KiriversUpdateResult {
    int outcome;
    char *staged_path;
    uint8_t *data; /**< Verified bytes when FileStore is unset; otherwise NULL. */
    size_t len;
    char *sha256;
    char *version_semver;
    char *diff_mode;
    KiriversCheckResult check;
} KiriversUpdateResult;

KiriversUpdater *kirivers_updater_new(const KiriversUpdaterConfig *cfg, KiriversError *err);
void kirivers_updater_free(KiriversUpdater *u);

/**
 * Check → download/diff/pack → hash verify → optional patch/unpack → optional replace.
 * Missing Replacer is not a failure: verified bytes stay at stage_path.
 * Missing FileStore is not a failure: verified bytes are returned in data/len.
 * Telemetry errors never fail the result.
 */
int kirivers_update(KiriversUpdater *u, const KiriversUpdateRequest *req, KiriversUpdateResult *out,
                    KiriversError *err);
void kirivers_update_result_free(KiriversUpdateResult *v);

#ifdef __cplusplus
}
#endif

#endif
