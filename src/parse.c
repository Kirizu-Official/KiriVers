#include "internal.h"

#include <stdlib.h>
#include <string.h>

char *kirivers_json_str(const cJSON *o, const char *key) {
    const cJSON *it;
    if (!o || !key) {
        return NULL;
    }
    it = cJSON_GetObjectItemCaseSensitive(o, key);
    if (cJSON_IsString(it) && it->valuestring) {
        return kirivers_strdup(it->valuestring);
    }
    return NULL;
}

int kirivers_json_bool(const cJSON *o, const char *key, int default_value) {
    const cJSON *it;
    if (!o) {
        return default_value;
    }
    it = cJSON_GetObjectItemCaseSensitive(o, key);
    if (cJSON_IsBool(it)) {
        return cJSON_IsTrue(it) ? 1 : 0;
    }
    return default_value;
}

int64_t kirivers_json_i64(const cJSON *o, const char *key, int *present) {
    const cJSON *it;
    if (present) {
        *present = 0;
    }
    if (!o) {
        return 0;
    }
    it = cJSON_GetObjectItemCaseSensitive(o, key);
    if (cJSON_IsNull(it) || !it) {
        return 0;
    }
    if (cJSON_IsNumber(it)) {
        if (present) {
            *present = 1;
        }
        return (int64_t)it->valuedouble;
    }
    if (cJSON_IsString(it) && it->valuestring) {
        if (present) {
            *present = 1;
        }
        return (int64_t)strtoll(it->valuestring, NULL, 10);
    }
    return 0;
}

int kirivers_json_string_array(const cJSON *arr, char ***out, size_t *n) {
    int i, sz;
    char **items;
    if (out) {
        *out = NULL;
    }
    if (n) {
        *n = 0;
    }
    if (!cJSON_IsArray(arr)) {
        return KIRIVERS_OK;
    }
    sz = cJSON_GetArraySize(arr);
    if (sz <= 0) {
        return KIRIVERS_OK;
    }
    items = (char **)calloc((size_t)sz, sizeof(char *));
    if (!items) {
        return KIRIVERS_ERR;
    }
    for (i = 0; i < sz; i++) {
        cJSON *it = cJSON_GetArrayItem(arr, i);
        if (cJSON_IsString(it) && it->valuestring) {
            items[i] = kirivers_strdup(it->valuestring);
        } else {
            items[i] = kirivers_strdup("");
        }
        if (!items[i]) {
            kirivers_free_string_array(items, (size_t)i);
            return KIRIVERS_ERR;
        }
    }
    *out = items;
    *n = (size_t)sz;
    return KIRIVERS_OK;
}

cJSON *kirivers_json_parse_body(const KiriversHttpResponse *resp, KiriversError *err) {
    cJSON *o;
    if (!resp || !resp->body || resp->body_len == 0) {
        kirivers_error_set(err, resp ? resp->status : 0, "JSON_PARSE", "empty body", NULL);
        return NULL;
    }
    o = cJSON_ParseWithLength((const char *)resp->body, resp->body_len);
    if (!o) {
        kirivers_error_set(err, resp->status, "JSON_PARSE", "invalid json", NULL);
        return NULL;
    }
    return o;
}

int kirivers_parse_health(const cJSON *o, KiriversHealth *out) {
    memset(out, 0, sizeof(*out));
    out->status = kirivers_json_str(o, "status");
    out->ready = kirivers_json_bool(o, "ready", 0);
    return KIRIVERS_OK;
}

int kirivers_parse_project(const cJSON *o, KiriversProject *out) {
    memset(out, 0, sizeof(*out));
    out->uuid = kirivers_json_str(o, "uuid");
    out->slug = kirivers_json_str(o, "slug");
    out->compare_engine = kirivers_json_str(o, "compare_engine");
    out->default_locale = kirivers_json_str(o, "default_locale");
    out->device_id_policy = kirivers_json_str(o, "device_id_policy");
    out->force_https = kirivers_json_bool(o, "force_https", 0);
    out->require_client_token = kirivers_json_bool(o, "require_client_token", 0);
    out->minimum_supported_version = kirivers_json_str(o, "minimum_supported_version");
    out->storage_visibility = kirivers_json_str(o, "storage_visibility");
    out->created_at = kirivers_json_str(o, "created_at");
    out->updated_at = kirivers_json_str(o, "updated_at");
    return KIRIVERS_OK;
}

int kirivers_parse_geo(const cJSON *o, KiriversGeoReport *out) {
    cJSON *geo;
    memset(out, 0, sizeof(*out));
    out->ip = kirivers_json_str(o, "ip");
    out->country_code = kirivers_json_str(o, "country_code");
    out->region_code = kirivers_json_str(o, "region_code");
    geo = cJSON_GetObjectItemCaseSensitive(o, "geo_i18n");
    if (geo && !cJSON_IsNull(geo)) {
        out->geo_i18n_json = cJSON_PrintUnformatted(geo);
    }
    return KIRIVERS_OK;
}

int kirivers_parse_check(const cJSON *o, KiriversCheckResult *out) {
    memset(out, 0, sizeof(*out));
    out->has_update = kirivers_json_bool(o, "has_update", 0);
    out->is_mandatory = kirivers_json_bool(o, "is_mandatory", 0);
    out->is_downgrade = kirivers_json_bool(o, "is_downgrade", 0);
    out->reason = kirivers_json_str(o, "reason");
    out->compare_engine = kirivers_json_str(o, "compare_engine");
    out->version_integer = kirivers_json_i64(o, "version_integer", &out->has_version_integer);
    out->version_semver = kirivers_json_str(o, "version_semver");
    out->target_channel = kirivers_json_str(o, "target_channel");
    out->target_hw_rev = kirivers_json_str(o, "target_hw_rev");
    out->package_type = kirivers_json_str(o, "package_type");
    out->root_hash = kirivers_json_str(o, "root_hash");
    out->package_url = kirivers_json_str(o, "package_url");
    out->file_name = kirivers_json_str(o, "file_name");
    out->size = kirivers_json_i64(o, "size", NULL);
    out->sha256 = kirivers_json_str(o, "sha256");
    out->signature = kirivers_json_str(o, "signature");
    out->artifact_signature = kirivers_json_str(o, "artifact_signature");
    out->platform_notes = kirivers_json_str(o, "platform_notes");
    out->publish_time = kirivers_json_str(o, "publish_time");
    out->delta_available = kirivers_json_bool(o, "delta_available", 0);
    out->delta_algo = kirivers_json_str(o, "delta_algo");
    return KIRIVERS_OK;
}

int kirivers_parse_changelog(const cJSON *o, KiriversChangelog *out) {
    cJSON *arr, *it;
    int i, sz;
    memset(out, 0, sizeof(*out));
    out->changelog = kirivers_json_str(o, "changelog");
    arr = cJSON_GetObjectItemCaseSensitive(o, "changelog_versions");
    if (!cJSON_IsArray(arr)) {
        return KIRIVERS_OK;
    }
    sz = cJSON_GetArraySize(arr);
    if (sz <= 0) {
        return KIRIVERS_OK;
    }
    out->versions = (KiriversChangelogVersion *)calloc((size_t)sz, sizeof(*out->versions));
    if (!out->versions) {
        return KIRIVERS_ERR;
    }
    out->n_versions = (size_t)sz;
    for (i = 0; i < sz; i++) {
        KiriversChangelogVersion *v = &out->versions[i];
        it = cJSON_GetArrayItem(arr, i);
        v->channel = kirivers_json_str(it, "channel");
        v->status = kirivers_json_str(it, "status");
        v->changelog = kirivers_json_str(it, "changelog");
        v->had_artifact_for_request_platform =
            kirivers_json_bool(it, "had_artifact_for_request_platform", 0);
        v->version_integer = kirivers_json_i64(it, "version_integer", &v->has_version_integer);
        v->version_semver = kirivers_json_str(it, "version_semver");
        v->title = kirivers_json_str(it, "title");
        v->platform_notes = kirivers_json_str(it, "platform_notes");
    }
    return KIRIVERS_OK;
}

static int parse_integrity_files(const cJSON *arr, KiriversIntegrityFile **out, size_t *n) {
    int i, sz;
    KiriversIntegrityFile *files;
    if (!cJSON_IsArray(arr)) {
        return KIRIVERS_OK;
    }
    sz = cJSON_GetArraySize(arr);
    if (sz <= 0) {
        return KIRIVERS_OK;
    }
    files = (KiriversIntegrityFile *)calloc((size_t)sz, sizeof(*files));
    if (!files) {
        return KIRIVERS_ERR;
    }
    for (i = 0; i < sz; i++) {
        cJSON *it = cJSON_GetArrayItem(arr, i);
        files[i].path = kirivers_json_str(it, "path");
        files[i].size = kirivers_json_i64(it, "size", NULL);
        files[i].sha256 = kirivers_json_str(it, "sha256");
        files[i].md5 = kirivers_json_str(it, "md5");
        files[i].install_policy = kirivers_json_str(it, "install_policy");
        files[i].integrity_check = kirivers_json_bool(it, "integrity_check", 0);
        files[i].url = kirivers_json_str(it, "url");
    }
    *out = files;
    *n = (size_t)sz;
    return KIRIVERS_OK;
}

int kirivers_parse_integrity(const cJSON *o, KiriversIntegrity *out) {
    memset(out, 0, sizeof(*out));
    out->version_integer = kirivers_json_i64(o, "version_integer", &out->has_version_integer);
    out->version_semver = kirivers_json_str(o, "version_semver");
    out->channel = kirivers_json_str(o, "channel");
    out->package_type = kirivers_json_str(o, "package_type");
    out->root_hash = kirivers_json_str(o, "root_hash");
    out->full_package_url = kirivers_json_str(o, "full_package_url");
    out->file_name = kirivers_json_str(o, "file_name");
    out->size = kirivers_json_i64(o, "size", NULL);
    out->sha256 = kirivers_json_str(o, "sha256");
    out->signature = kirivers_json_str(o, "signature");
    return parse_integrity_files(cJSON_GetObjectItemCaseSensitive(o, "files"), &out->files,
                                 &out->n_files);
}

int kirivers_parse_diff(const cJSON *o, KiriversDiffResult *out) {
    cJSON *files;
    int i, sz;
    memset(out, 0, sizeof(*out));
    out->diff_mode = kirivers_json_str(o, "diff_mode");
    out->root_hash = kirivers_json_str(o, "root_hash");
    out->version_integer = kirivers_json_i64(o, "version_integer", &out->has_version_integer);
    out->version_semver = kirivers_json_str(o, "version_semver");
    out->channel = kirivers_json_str(o, "channel");
    out->compare_engine = kirivers_json_str(o, "compare_engine");
    out->package_url = kirivers_json_str(o, "package_url");
    out->file_name = kirivers_json_str(o, "file_name");
    out->size = kirivers_json_i64(o, "size", NULL);
    out->sha256 = kirivers_json_str(o, "sha256");
    out->signature = kirivers_json_str(o, "signature");
    out->delta_algo = kirivers_json_str(o, "delta_algo");
    kirivers_json_string_array(cJSON_GetObjectItemCaseSensitive(o, "deleted_paths"),
                               &out->deleted_paths, &out->n_deleted_paths);
    kirivers_json_string_array(cJSON_GetObjectItemCaseSensitive(o, "invalid_paths"),
                               &out->invalid_paths, &out->n_invalid_paths);
    files = cJSON_GetObjectItemCaseSensitive(o, "files");
    if (cJSON_IsArray(files) && (sz = cJSON_GetArraySize(files)) > 0) {
        out->files = (KiriversDiffFile *)calloc((size_t)sz, sizeof(*out->files));
        if (!out->files) {
            return KIRIVERS_ERR;
        }
        out->n_files = (size_t)sz;
        for (i = 0; i < sz; i++) {
            cJSON *it = cJSON_GetArrayItem(files, i);
            out->files[i].path = kirivers_json_str(it, "path");
            out->files[i].size = kirivers_json_i64(it, "size", NULL);
            out->files[i].sha256 = kirivers_json_str(it, "sha256");
            out->files[i].md5 = kirivers_json_str(it, "md5");
            out->files[i].url = kirivers_json_str(it, "url");
        }
    }
    return KIRIVERS_OK;
}

int kirivers_parse_pack(const cJSON *o, KiriversPackResult *out) {
    cJSON *files;
    int i, sz;
    memset(out, 0, sizeof(*out));
    out->status = kirivers_json_str(o, "status");
    out->diff_mode = kirivers_json_str(o, "diff_mode");
    out->package_url = kirivers_json_str(o, "package_url");
    out->file_name = kirivers_json_str(o, "file_name");
    out->size = kirivers_json_i64(o, "size", NULL);
    out->sha256 = kirivers_json_str(o, "sha256");
    out->signature = kirivers_json_str(o, "signature");
    out->root_hash = kirivers_json_str(o, "root_hash");
    out->channel = kirivers_json_str(o, "channel");
    out->compare_engine = kirivers_json_str(o, "compare_engine");
    out->compression = kirivers_json_str(o, "compression");
    kirivers_json_string_array(cJSON_GetObjectItemCaseSensitive(o, "deleted_paths"),
                               &out->deleted_paths, &out->n_deleted_paths);
    kirivers_json_string_array(cJSON_GetObjectItemCaseSensitive(o, "invalid_paths"),
                               &out->invalid_paths, &out->n_invalid_paths);
    files = cJSON_GetObjectItemCaseSensitive(o, "files");
    if (cJSON_IsArray(files) && (sz = cJSON_GetArraySize(files)) > 0) {
        out->files = (KiriversPackFile *)calloc((size_t)sz, sizeof(*out->files));
        if (!out->files) {
            return KIRIVERS_ERR;
        }
        out->n_files = (size_t)sz;
        for (i = 0; i < sz; i++) {
            cJSON *it = cJSON_GetArrayItem(files, i);
            out->files[i].path = kirivers_json_str(it, "path");
            out->files[i].size = kirivers_json_i64(it, "size", NULL);
            out->files[i].sha256 = kirivers_json_str(it, "sha256");
            out->files[i].install_policy = kirivers_json_str(it, "install_policy");
            out->files[i].integrity_check = kirivers_json_bool(it, "integrity_check", 0);
        }
    }
    return KIRIVERS_OK;
}

int kirivers_parse_channels(const cJSON *o, KiriversChannelList *out) {
    cJSON *arr = cJSON_GetObjectItemCaseSensitive(o, "channels");
    int i, sz;
    memset(out, 0, sizeof(*out));
    if (!cJSON_IsArray(arr)) {
        return KIRIVERS_OK;
    }
    sz = cJSON_GetArraySize(arr);
    if (sz <= 0) {
        return KIRIVERS_OK;
    }
    out->items = (KiriversChannel *)calloc((size_t)sz, sizeof(*out->items));
    if (!out->items) {
        return KIRIVERS_ERR;
    }
    out->count = (size_t)sz;
    for (i = 0; i < sz; i++) {
        cJSON *it = cJSON_GetArrayItem(arr, i);
        out->items[i].name = kirivers_json_str(it, "name");
        out->items[i].slug = kirivers_json_str(it, "slug");
        out->items[i].stability_rank = (int)kirivers_json_i64(it, "stability_rank", NULL);
    }
    return KIRIVERS_OK;
}

int kirivers_parse_matrix(const cJSON *o, KiriversMatrixList *out) {
    cJSON *arr = cJSON_GetObjectItemCaseSensitive(o, "matrix");
    int i, sz;
    memset(out, 0, sizeof(*out));
    if (!cJSON_IsArray(arr)) {
        return KIRIVERS_OK;
    }
    sz = cJSON_GetArraySize(arr);
    if (sz <= 0) {
        return KIRIVERS_OK;
    }
    out->items = (KiriversMatrixRow *)calloc((size_t)sz, sizeof(*out->items));
    if (!out->items) {
        return KIRIVERS_ERR;
    }
    out->count = (size_t)sz;
    for (i = 0; i < sz; i++) {
        cJSON *it = cJSON_GetArrayItem(arr, i);
        out->items[i].os = kirivers_json_str(it, "os");
        out->items[i].arch = kirivers_json_str(it, "arch");
        out->items[i].package_type = kirivers_json_str(it, "package_type");
    }
    return KIRIVERS_OK;
}

int kirivers_parse_languages(const cJSON *o, KiriversLanguageList *out) {
    cJSON *arr = cJSON_GetObjectItemCaseSensitive(o, "languages");
    int i, sz;
    memset(out, 0, sizeof(*out));
    if (!cJSON_IsArray(arr)) {
        return KIRIVERS_OK;
    }
    sz = cJSON_GetArraySize(arr);
    if (sz <= 0) {
        return KIRIVERS_OK;
    }
    out->items = (KiriversLanguage *)calloc((size_t)sz, sizeof(*out->items));
    if (!out->items) {
        return KIRIVERS_ERR;
    }
    out->count = (size_t)sz;
    for (i = 0; i < sz; i++) {
        cJSON *it = cJSON_GetArrayItem(arr, i);
        out->items[i].code = kirivers_json_str(it, "code");
        out->items[i].display_name = kirivers_json_str(it, "display_name");
        out->items[i].is_default = kirivers_json_bool(it, "is_default", 0);
        out->items[i].sort_order = (int)kirivers_json_i64(it, "sort_order", NULL);
    }
    return KIRIVERS_OK;
}

int kirivers_parse_announcements(const cJSON *o, KiriversAnnouncementList *out) {
    cJSON *arr = cJSON_GetObjectItemCaseSensitive(o, "announcements");
    int i, sz;
    memset(out, 0, sizeof(*out));
    if (!cJSON_IsArray(arr)) {
        return KIRIVERS_OK;
    }
    sz = cJSON_GetArraySize(arr);
    if (sz <= 0) {
        return KIRIVERS_OK;
    }
    out->items = (KiriversAnnouncement *)calloc((size_t)sz, sizeof(*out->items));
    if (!out->items) {
        return KIRIVERS_ERR;
    }
    out->count = (size_t)sz;
    for (i = 0; i < sz; i++) {
        cJSON *it = cJSON_GetArrayItem(arr, i);
        out->items[i].id = kirivers_json_str(it, "id");
        out->items[i].title = kirivers_json_str(it, "title");
        out->items[i].subtitle = kirivers_json_str(it, "subtitle");
        out->items[i].markdown = kirivers_json_str(it, "markdown");
        out->items[i].locale = kirivers_json_str(it, "locale");
        out->items[i].starts_at = kirivers_json_str(it, "starts_at");
        out->items[i].ends_at = kirivers_json_str(it, "ends_at");
    }
    return KIRIVERS_OK;
}

/* suppress unused warning: etag helper used from client.c */
void kirivers_copy_etag(const KiriversHttpResponse *resp, char **dst) {
    const char *e;
    if (!resp || !dst) {
        return;
    }
    e = kirivers_header_get(resp, "ETag");
    if (e) {
        *dst = kirivers_strdup(e);
    }
}
