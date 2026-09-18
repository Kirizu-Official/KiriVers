#ifndef KIRIVERS_ERROR_H
#define KIRIVERS_ERROR_H

#include <stddef.h>

#ifdef __cplusplus
extern "C" {
#endif

/** Success. */
#define KIRIVERS_OK 0
/** HTTP 204 on check: already up to date (not an error). */
#define KIRIVERS_NO_UPDATE 1
/** HTTP 304: caller ETag matched (not an error). */
#define KIRIVERS_NOT_MODIFIED 2
/** HTTP 202 pack: still pending; poll the same JSON. */
#define KIRIVERS_PENDING 3
/** HTTP 202 telemetry: accepted. */
#define KIRIVERS_ACCEPTED 4
/** Failure; inspect KiriversError. */
#define KIRIVERS_ERR (-1)

/**
 * Unified error. Server failures use the envelope
 * `{ "error": { "code", "message", "details" } }`.
 * Local failures use stable codes such as NO_TRANSPORT, INVALID_ARGUMENT.
 * SDK logs never include raw device_id.
 */
typedef struct KiriversError {
    int http_status;   /**< 0 when the failure is local. */
    char *code;        /**< Stable token; never NULL after a KIRIVERS_ERR. */
    char *message;     /**< Human-readable; English; do not branch on this. */
    char *details_json; /**< Optional JSON for `details`; NULL if omitted. */
} KiriversError;

void kirivers_error_clear(KiriversError *err);

#ifdef __cplusplus
}
#endif

#endif
