#include "kirivers/types.h"
#include "internal.h"

#include <stdlib.h>
#include <string.h>

void kirivers_project_free(KiriversProject *v) {
    if (!v) {
        return;
    }
    free(v->uuid);
    free(v->slug);
    free(v->compare_engine);
    free(v->default_locale);
    free(v->device_id_policy);
    free(v->minimum_supported_version);
    free(v->storage_visibility);
    free(v->created_at);
    free(v->updated_at);
    memset(v, 0, sizeof(*v));
}

void kirivers_geo_report_free(KiriversGeoReport *v) {
    if (!v) {
        return;
    }
    free(v->ip);
    free(v->country_code);
    free(v->region_code);
    free(v->geo_i18n_json);
    memset(v, 0, sizeof(*v));
}

void kirivers_health_free(KiriversHealth *v) {
    if (!v) {
        return;
    }
    free(v->status);
    memset(v, 0, sizeof(*v));
}

void kirivers_check_result_free(KiriversCheckResult *v) {
    if (!v) {
        return;
    }
    free(v->etag);
    free(v->reason);
    free(v->compare_engine);
    free(v->version_semver);
    free(v->target_channel);
    free(v->target_hw_rev);
    free(v->package_type);
    free(v->root_hash);
    free(v->package_url);
    free(v->file_name);
    free(v->sha256);
    free(v->signature);
    free(v->artifact_signature);
    free(v->platform_notes);
    free(v->publish_time);
    free(v->delta_algo);
    memset(v, 0, sizeof(*v));
}

void kirivers_changelog_free(KiriversChangelog *v) {
    size_t i;
    if (!v) {
        return;
    }
    free(v->etag);
    free(v->changelog);
    for (i = 0; i < v->n_versions; i++) {
        free(v->versions[i].channel);
        free(v->versions[i].status);
        free(v->versions[i].changelog);
        free(v->versions[i].version_semver);
        free(v->versions[i].title);
        free(v->versions[i].platform_notes);
    }
    free(v->versions);
    memset(v, 0, sizeof(*v));
}

void kirivers_integrity_free(KiriversIntegrity *v) {
    size_t i;
    if (!v) {
        return;
    }
    free(v->etag);
    free(v->version_semver);
    free(v->channel);
    free(v->package_type);
    free(v->root_hash);
    free(v->full_package_url);
    free(v->file_name);
    free(v->sha256);
    free(v->signature);
    for (i = 0; i < v->n_files; i++) {
        free(v->files[i].path);
        free(v->files[i].sha256);
        free(v->files[i].md5);
        free(v->files[i].install_policy);
        free(v->files[i].url);
    }
    free(v->files);
    memset(v, 0, sizeof(*v));
}

void kirivers_diff_result_free(KiriversDiffResult *v) {
    size_t i;
    if (!v) {
        return;
    }
    free(v->diff_mode);
    free(v->root_hash);
    free(v->version_semver);
    free(v->channel);
    free(v->compare_engine);
    free(v->package_url);
    free(v->file_name);
    free(v->sha256);
    free(v->signature);
    free(v->delta_algo);
    kirivers_free_string_array(v->deleted_paths, v->n_deleted_paths);
    kirivers_free_string_array(v->invalid_paths, v->n_invalid_paths);
    for (i = 0; i < v->n_files; i++) {
        free(v->files[i].path);
        free(v->files[i].sha256);
        free(v->files[i].md5);
        free(v->files[i].url);
    }
    free(v->files);
    memset(v, 0, sizeof(*v));
}

void kirivers_pack_result_free(KiriversPackResult *v) {
    size_t i;
    if (!v) {
        return;
    }
    free(v->status);
    free(v->diff_mode);
    free(v->package_url);
    free(v->file_name);
    free(v->sha256);
    free(v->signature);
    free(v->root_hash);
    free(v->channel);
    free(v->compare_engine);
    free(v->compression);
    kirivers_free_string_array(v->deleted_paths, v->n_deleted_paths);
    kirivers_free_string_array(v->invalid_paths, v->n_invalid_paths);
    for (i = 0; i < v->n_files; i++) {
        free(v->files[i].path);
        free(v->files[i].sha256);
        free(v->files[i].install_policy);
    }
    free(v->files);
    memset(v, 0, sizeof(*v));
}

void kirivers_channel_list_free(KiriversChannelList *v) {
    size_t i;
    if (!v) {
        return;
    }
    for (i = 0; i < v->count; i++) {
        free(v->items[i].name);
        free(v->items[i].slug);
    }
    free(v->items);
    memset(v, 0, sizeof(*v));
}

void kirivers_matrix_list_free(KiriversMatrixList *v) {
    size_t i;
    if (!v) {
        return;
    }
    for (i = 0; i < v->count; i++) {
        free(v->items[i].os);
        free(v->items[i].arch);
        free(v->items[i].package_type);
    }
    free(v->items);
    memset(v, 0, sizeof(*v));
}

void kirivers_language_list_free(KiriversLanguageList *v) {
    size_t i;
    if (!v) {
        return;
    }
    for (i = 0; i < v->count; i++) {
        free(v->items[i].code);
        free(v->items[i].display_name);
    }
    free(v->items);
    memset(v, 0, sizeof(*v));
}

void kirivers_announcement_list_free(KiriversAnnouncementList *v) {
    size_t i;
    if (!v) {
        return;
    }
    free(v->etag);
    for (i = 0; i < v->count; i++) {
        free(v->items[i].id);
        free(v->items[i].title);
        free(v->items[i].subtitle);
        free(v->items[i].markdown);
        free(v->items[i].locale);
        free(v->items[i].starts_at);
        free(v->items[i].ends_at);
    }
    free(v->items);
    memset(v, 0, sizeof(*v));
}

void kirivers_bytes_free(KiriversBytes *v) {
    if (!v) {
        return;
    }
    free(v->data);
    free(v->etag);
    free(v->content_type);
    memset(v, 0, sizeof(*v));
}
