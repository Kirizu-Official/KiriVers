#ifndef KIRIVERS_CLIENT_H
#define KIRIVERS_CLIENT_H

#include "kirivers/adapters.h"
#include "kirivers/error.h"
#include "kirivers/types.h"

#ifdef __cplusplus
extern "C" {
#endif

typedef struct KiriversClient KiriversClient;

typedef struct KiriversClientConfig {
    const char *base_url;     /**< Client plane, no trailing slash. */
    const char *project_ref;  /**< UUID, slug, or unexpired alias. */
    const char *project_token;  /**< Optional Bearer / X-Project-Token. */
    const char *channel_token;  /**< Optional X-Channel-Token; never logged. */
    KiriversTransport transport;
    KiriversClock clock;
    unsigned pack_backoff_ms;     /**< Default 1000. */
    unsigned pack_backoff_cap_ms; /**< Default 15000. */
    unsigned pack_deadline_ms;    /**< 0 means default 120000 ms. Disable polling with PackRequest.poll=0. */
} KiriversClientConfig;

typedef struct KiriversCheckRequest {
    const char *current_version;
    const char *os;
    const char *arch;
    const char *channel;
    const char *hw_rev;
    const char *os_version;
    const char *device_id; /**< Caller-supplied; SDK does not invent one. */
    const char *const *capabilities;
    size_t n_capabilities;
    const char *const *accepted_delta_algos;
    size_t n_accepted_delta_algos;
    const char *if_none_match;
} KiriversCheckRequest;

typedef struct KiriversDeviceReportRequest {
    const char *device_id;
    const char *os;
    const char *arch;
    const char *channel;
    const char *version;
    const char *custom_json; /**< Object JSON or NULL. */
} KiriversDeviceReportRequest;

typedef struct KiriversChangelogQuery {
    const char *from_version;
    const char *to_version;
    const char *changelog_scope;
    const char *changelog_layout;
    const char *changelog_locale;
    const char *locale;
    int changelog_include_revoked; /**< -1 omit, 0 false, 1 true */
    int changelog_include_platform_notes;
    const char *if_none_match;
} KiriversChangelogQuery;

typedef struct KiriversIntegrityQuery {
    const char *os;
    const char *arch;
    const char *version;
    const char *channel;
    const char *hw_rev;
    const char *hash_algo; /**< sha256 / md5 / both */
    int compact;           /**< -1 omit */
    int include_file_urls;
    const char *if_none_match;
} KiriversIntegrityQuery;

typedef struct KiriversDiffRequest {
    const char *source_version;
    const char *target_version;
    const char *os;
    const char *arch;
    const char *channel;
    const char *hw_rev;
    const char *device_id;
    const char *local_sha256;
    int prefer_full;
    const char *const *capabilities;
    size_t n_capabilities;
    const char *const *accepted_delta_algos;
    size_t n_accepted_delta_algos;
} KiriversDiffRequest;

typedef struct KiriversPackRequest {
    const char *source_version;
    const char *target_version;
    const char *os;
    const char *arch;
    const char *channel;
    const char *hw_rev;
    const char *device_id;
    const char *const *needed_paths;
    size_t n_needed_paths;
    int poll; /**< Non-zero: poll the identical JSON until ready/full_package/error. */
} KiriversPackRequest;

typedef struct KiriversTelemetryRequest {
    const char *os;
    const char *arch;
    const char *channel;
    const char *from_version;
    const char *to_version;
    const char *status; /**< downloading / applying / installed / failed / rolled_back */
    const char *device_id;
    const char *diff_mode;
    const char *error_code;
    const char *error_message;
} KiriversTelemetryRequest;

typedef struct KiriversAnnouncementQuery {
    const char *version;
    const char *os;
    const char *arch;
    const char *locale;
    const char *if_none_match;
} KiriversAnnouncementQuery;

KiriversClient *kirivers_client_new(const KiriversClientConfig *cfg, KiriversError *err);
void kirivers_client_free(KiriversClient *client);

int kirivers_client_health(KiriversClient *c, KiriversHealth *out, KiriversError *err);
int kirivers_client_project(KiriversClient *c, KiriversProject *out, KiriversError *err);
int kirivers_client_device_report(KiriversClient *c, const KiriversDeviceReportRequest *req,
                                  KiriversGeoReport *out, KiriversError *err);
int kirivers_client_check(KiriversClient *c, const KiriversCheckRequest *req,
                          KiriversCheckResult *out, KiriversError *err);
int kirivers_client_changelog(KiriversClient *c, const char *channel, const char *os,
                              const char *arch, const KiriversChangelogQuery *query,
                              KiriversChangelog *out, KiriversError *err);
int kirivers_client_integrity(KiriversClient *c, const KiriversIntegrityQuery *query,
                              KiriversIntegrity *out, KiriversError *err);
int kirivers_client_diff(KiriversClient *c, const KiriversDiffRequest *req, KiriversDiffResult *out,
                         KiriversError *err);
int kirivers_client_pack(KiriversClient *c, const KiriversPackRequest *req, KiriversPackResult *out,
                         KiriversError *err);
int kirivers_client_download(KiriversClient *c, const char *content_sha256, const char *query,
                             const char *range, KiriversBytes *out, KiriversError *err);
int kirivers_client_download_url(KiriversClient *c, const char *url_or_path, const char *range,
                                 KiriversBytes *out, KiriversError *err);
int kirivers_client_head_package(KiriversClient *c, const char *content_sha256, const char *query,
                                 KiriversBytes *out, KiriversError *err);
int kirivers_client_channels(KiriversClient *c, KiriversChannelList *out, KiriversError *err);
int kirivers_client_matrix(KiriversClient *c, KiriversMatrixList *out, KiriversError *err);
int kirivers_client_languages(KiriversClient *c, KiriversLanguageList *out, KiriversError *err);
int kirivers_client_announcements(KiriversClient *c, const KiriversAnnouncementQuery *query,
                                  KiriversAnnouncementList *out, KiriversError *err);
int kirivers_client_telemetry(KiriversClient *c, const KiriversTelemetryRequest *req,
                              KiriversError *err);
int kirivers_client_media(KiriversClient *c, const char *media_id, const char *range,
                          KiriversBytes *out, KiriversError *err);
int kirivers_client_head_media(KiriversClient *c, const char *media_id, KiriversBytes *out,
                               KiriversError *err);

#ifdef __cplusplus
}
#endif

#endif
