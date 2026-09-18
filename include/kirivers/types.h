#ifndef KIRIVERS_TYPES_H
#define KIRIVERS_TYPES_H

#include <stddef.h>
#include <stdint.h>

#ifdef __cplusplus
extern "C" {
#endif

typedef struct KiriversProject {
    char *uuid;
    char *slug;
    char *compare_engine;
    char *default_locale;
    char *device_id_policy;
    int force_https;
    int require_client_token;
    char *minimum_supported_version;
    char *storage_visibility;
    char *created_at;
    char *updated_at;
} KiriversProject;

typedef struct KiriversGeoReport {
    char *ip;
    char *country_code;
    char *region_code;
    char *geo_i18n_json;
} KiriversGeoReport;

typedef struct KiriversHealth {
    char *status;
    int ready;
} KiriversHealth;

typedef struct KiriversCheckResult {
    int http_status; /**< 200, 204, or 304. */
    char *etag;
    int has_update;
    int is_mandatory;
    int is_downgrade;
    char *reason;
    char *compare_engine;
    int64_t version_integer;
    int has_version_integer;
    char *version_semver;
    char *target_channel;
    char *target_hw_rev;
    char *package_type;
    char *root_hash;
    char *package_url;
    char *file_name;
    int64_t size;
    char *sha256;
    char *signature;
    char *artifact_signature;
    char *platform_notes;
    char *publish_time;
    int delta_available;
    char *delta_algo;
} KiriversCheckResult;

typedef struct KiriversChangelogVersion {
    char *channel;
    char *status;
    char *changelog;
    int had_artifact_for_request_platform;
    int64_t version_integer;
    int has_version_integer;
    char *version_semver;
    char *title;
    char *platform_notes;
} KiriversChangelogVersion;

typedef struct KiriversChangelog {
    int http_status;
    char *etag;
    char *changelog;
    KiriversChangelogVersion *versions;
    size_t n_versions;
} KiriversChangelog;

typedef struct KiriversIntegrityFile {
    char *path;
    int64_t size;
    char *sha256;
    char *md5;
    char *install_policy;
    int integrity_check;
    char *url;
} KiriversIntegrityFile;

typedef struct KiriversIntegrity {
    int http_status;
    char *etag;
    int64_t version_integer;
    int has_version_integer;
    char *version_semver;
    char *channel;
    char *package_type;
    char *root_hash;
    char *full_package_url;
    char *file_name;
    int64_t size;
    char *sha256;
    char *signature;
    KiriversIntegrityFile *files;
    size_t n_files;
} KiriversIntegrity;

typedef struct KiriversDiffFile {
    char *path;
    int64_t size;
    char *sha256;
    char *md5;
    char *url;
} KiriversDiffFile;

typedef struct KiriversDiffResult {
    char *diff_mode;
    char *root_hash;
    int64_t version_integer;
    int has_version_integer;
    char *version_semver;
    char *channel;
    char *compare_engine;
    char *package_url;
    char *file_name;
    int64_t size;
    char *sha256;
    char *signature;
    char *delta_algo;
    char **deleted_paths;
    size_t n_deleted_paths;
    char **invalid_paths;
    size_t n_invalid_paths;
    KiriversDiffFile *files;
    size_t n_files;
} KiriversDiffResult;

typedef struct KiriversPackFile {
    char *path;
    int64_t size;
    char *sha256;
    char *install_policy;
    int integrity_check;
} KiriversPackFile;

typedef struct KiriversPackResult {
    int http_status;
    char *status;
    char *diff_mode;
    char *package_url;
    char *file_name;
    int64_t size;
    char *sha256;
    char *signature;
    char *root_hash;
    char *channel;
    char *compare_engine;
    char *compression;
    char **deleted_paths;
    size_t n_deleted_paths;
    char **invalid_paths;
    size_t n_invalid_paths;
    KiriversPackFile *files;
    size_t n_files;
} KiriversPackResult;

typedef struct KiriversChannel {
    char *name;
    char *slug;
    int stability_rank;
} KiriversChannel;

typedef struct KiriversChannelList {
    KiriversChannel *items;
    size_t count;
} KiriversChannelList;

typedef struct KiriversMatrixRow {
    char *os;
    char *arch;
    char *package_type;
} KiriversMatrixRow;

typedef struct KiriversMatrixList {
    KiriversMatrixRow *items;
    size_t count;
} KiriversMatrixList;

typedef struct KiriversLanguage {
    char *code;
    char *display_name;
    int is_default;
    int sort_order;
} KiriversLanguage;

typedef struct KiriversLanguageList {
    KiriversLanguage *items;
    size_t count;
} KiriversLanguageList;

typedef struct KiriversAnnouncement {
    char *id;
    char *title;
    char *subtitle;
    char *markdown;
    char *locale;
    char *starts_at;
    char *ends_at;
} KiriversAnnouncement;

typedef struct KiriversAnnouncementList {
    int http_status;
    char *etag;
    KiriversAnnouncement *items;
    size_t count;
} KiriversAnnouncementList;

typedef struct KiriversBytes {
    uint8_t *data;
    size_t len;
    int http_status;
    char *etag;
    char *content_type;
} KiriversBytes;

void kirivers_project_free(KiriversProject *v);
void kirivers_geo_report_free(KiriversGeoReport *v);
void kirivers_health_free(KiriversHealth *v);
void kirivers_check_result_free(KiriversCheckResult *v);
void kirivers_changelog_free(KiriversChangelog *v);
void kirivers_integrity_free(KiriversIntegrity *v);
void kirivers_diff_result_free(KiriversDiffResult *v);
void kirivers_pack_result_free(KiriversPackResult *v);
void kirivers_channel_list_free(KiriversChannelList *v);
void kirivers_matrix_list_free(KiriversMatrixList *v);
void kirivers_language_list_free(KiriversLanguageList *v);
void kirivers_announcement_list_free(KiriversAnnouncementList *v);
void kirivers_bytes_free(KiriversBytes *v);

#ifdef __cplusplus
}
#endif

#endif
