#include "kirivers/client.h"
#include "internal.h"

#include <stdlib.h>
#include <string.h>
#include <stdio.h>

static void free_resp(KiriversClient *c, KiriversHttpResponse *resp) {
    if (c && c->transport.free_response) {
        c->transport.free_response(c->transport.ctx, resp);
    } else {
        kirivers_response_clear(resp);
    }
}

static int finish_json_ok(KiriversClient *c, KiriversHttpResponse *resp, cJSON **out,
                          KiriversError *err) {
    cJSON *o = kirivers_json_parse_body(resp, err);
    if (!o) {
        free_resp(c, resp);
        return KIRIVERS_ERR;
    }
    *out = o;
    return KIRIVERS_OK;
}

KiriversClient *kirivers_client_new(const KiriversClientConfig *cfg, KiriversError *err) {
    KiriversClient *c;
    size_t n;
    if (!cfg || !cfg->base_url || !cfg->project_ref) {
        kirivers_error_set(err, 0, "INVALID_ARGUMENT", "base_url and project_ref are required",
                           NULL);
        return NULL;
    }
    if (!cfg->transport.request) {
        kirivers_error_set(err, 0, "NO_TRANSPORT", "Transport.request is required", NULL);
        return NULL;
    }
    c = (KiriversClient *)calloc(1, sizeof(*c));
    if (!c) {
        kirivers_error_set(err, 0, "INTERNAL_ERROR", "oom", NULL);
        return NULL;
    }
    c->base_url = kirivers_strdup(cfg->base_url);
    n = strlen(c->base_url);
    while (n && c->base_url[n - 1] == '/') {
        c->base_url[--n] = 0;
    }
    c->project_ref = kirivers_strdup(cfg->project_ref);
    c->project_token = cfg->project_token ? kirivers_strdup(cfg->project_token) : NULL;
    c->channel_token = cfg->channel_token ? kirivers_strdup(cfg->channel_token) : NULL;
    c->transport = cfg->transport;
    c->clock = cfg->clock;
    c->pack_backoff_ms = cfg->pack_backoff_ms ? cfg->pack_backoff_ms : 1000;
    c->pack_backoff_cap_ms = cfg->pack_backoff_cap_ms ? cfg->pack_backoff_cap_ms : 15000;
    c->pack_deadline_ms = cfg->pack_deadline_ms ? cfg->pack_deadline_ms : 120000;
    return c;
}

void kirivers_client_free(KiriversClient *client) {
    if (!client) {
        return;
    }
    free(client->base_url);
    free(client->project_ref);
    free(client->project_token);
    free(client->channel_token);
    free(client);
}

static int json_get(KiriversClient *c, const char *url, const char *if_none_match,
                    KiriversHttpResponse *resp, int *not_modified, KiriversError *err) {
    int rc;
    *not_modified = 0;
    rc = kirivers_http(c, "GET", url, if_none_match, NULL, NULL, resp, err);
    if (rc != KIRIVERS_OK) {
        return rc;
    }
    if (resp->status == 304) {
        *not_modified = 1;
        return KIRIVERS_OK;
    }
    return KIRIVERS_OK;
}

int kirivers_client_health(KiriversClient *c, KiriversHealth *out, KiriversError *err) {
    char *url;
    KiriversHttpResponse resp;
    cJSON *o;
    int rc;
    memset(out, 0, sizeof(*out));
    url = kirivers_join_url(c->base_url, "/api/v1/health");
    if (!url) {
        return kirivers_error_set(err, 0, "INTERNAL_ERROR", "oom", NULL);
    }
    rc = kirivers_http(c, "GET", url, NULL, NULL, NULL, &resp, err);
    free(url);
    if (rc != KIRIVERS_OK) {
        free_resp(c, &resp);
        return rc;
    }
    if (finish_json_ok(c, &resp, &o, err) != KIRIVERS_OK) {
        return KIRIVERS_ERR;
    }
    rc = kirivers_parse_health(o, out);
    cJSON_Delete(o);
    free_resp(c, &resp);
    return rc == KIRIVERS_OK ? KIRIVERS_OK : kirivers_error_set(err, 0, "JSON_PARSE", "health", NULL);
}

int kirivers_client_project(KiriversClient *c, KiriversProject *out, KiriversError *err) {
    char *url = kirivers_project_url(c, "");
    KiriversHttpResponse resp;
    cJSON *o;
    int rc;
    memset(out, 0, sizeof(*out));
    if (!url) {
        return kirivers_error_set(err, 0, "INTERNAL_ERROR", "oom", NULL);
    }
    rc = kirivers_http(c, "GET", url, NULL, NULL, NULL, &resp, err);
    free(url);
    if (rc != KIRIVERS_OK) {
        free_resp(c, &resp);
        return rc;
    }
    if (finish_json_ok(c, &resp, &o, err) != KIRIVERS_OK) {
        return KIRIVERS_ERR;
    }
    rc = kirivers_parse_project(o, out);
    cJSON_Delete(o);
    free_resp(c, &resp);
    return rc;
}

int kirivers_client_device_report(KiriversClient *c, const KiriversDeviceReportRequest *req,
                                  KiriversGeoReport *out, KiriversError *err) {
    cJSON *body;
    char *json, *url;
    KiriversHttpResponse resp;
    cJSON *o;
    int rc;
    memset(out, 0, sizeof(*out));
    if (!req || !req->device_id || !req->device_id[0]) {
        return kirivers_error_set(err, 0, "INVALID_ARGUMENT", "device_id is required", NULL);
    }
    body = cJSON_CreateObject();
    cJSON_AddStringToObject(body, "device_id", req->device_id);
    if (req->os) {
        cJSON_AddStringToObject(body, "os", req->os);
    }
    if (req->arch) {
        cJSON_AddStringToObject(body, "arch", req->arch);
    }
    if (req->channel) {
        cJSON_AddStringToObject(body, "channel", req->channel);
    }
    if (req->version) {
        cJSON_AddStringToObject(body, "version", req->version);
    }
    if (req->custom_json && req->custom_json[0]) {
        cJSON *custom = cJSON_Parse(req->custom_json);
        if (!custom || !cJSON_IsObject(custom)) {
            cJSON_Delete(custom);
            cJSON_Delete(body);
            return kirivers_error_set(err, 0, "INVALID_ARGUMENT", "custom_json must be an object",
                                      NULL);
        }
        cJSON_AddItemToObject(body, "custom", custom);
    }
    json = cJSON_PrintUnformatted(body);
    cJSON_Delete(body);
    url = kirivers_project_url(c, "/clients/report");
    if (!json || !url) {
        cJSON_free(json);
        free(url);
        return kirivers_error_set(err, 0, "INTERNAL_ERROR", "oom", NULL);
    }
    rc = kirivers_http(c, "POST", url, NULL, NULL, json, &resp, err);
    cJSON_free(json);
    free(url);
    if (rc != KIRIVERS_OK) {
        free_resp(c, &resp);
        return rc;
    }
    if (finish_json_ok(c, &resp, &o, err) != KIRIVERS_OK) {
        return KIRIVERS_ERR;
    }
    rc = kirivers_parse_geo(o, out);
    cJSON_Delete(o);
    free_resp(c, &resp);
    return rc;
}

static void add_string_array(cJSON *obj, const char *key, const char *const *items, size_t n) {
    cJSON *arr;
    size_t i;
    if (!items || n == 0) {
        return;
    }
    arr = cJSON_AddArrayToObject(obj, key);
    for (i = 0; i < n; i++) {
        if (items[i]) {
            cJSON_AddItemToArray(arr, cJSON_CreateString(items[i]));
        }
    }
}

int kirivers_client_check(KiriversClient *c, const KiriversCheckRequest *req,
                          KiriversCheckResult *out, KiriversError *err) {
    cJSON *body;
    char *json, *url;
    KiriversHttpResponse resp;
    int rc;
    memset(out, 0, sizeof(*out));
    if (!req || !req->current_version || !req->os || !req->arch) {
        return kirivers_error_set(err, 0, "INVALID_ARGUMENT",
                                  "current_version, os, and arch are required", NULL);
    }
    body = cJSON_CreateObject();
    cJSON_AddStringToObject(body, "current_version", req->current_version);
    cJSON_AddStringToObject(body, "os", req->os);
    cJSON_AddStringToObject(body, "arch", req->arch);
    if (req->channel) {
        cJSON_AddStringToObject(body, "channel", req->channel);
    }
    if (req->hw_rev) {
        cJSON_AddStringToObject(body, "hw_rev", req->hw_rev);
    }
    if (req->os_version) {
        cJSON_AddStringToObject(body, "os_version", req->os_version);
    }
    if (req->device_id) {
        cJSON_AddStringToObject(body, "device_id", req->device_id);
    }
    if (req->n_capabilities == 0) {
        cJSON *arr = cJSON_AddArrayToObject(body, "capabilities");
        cJSON_AddItemToArray(arr, cJSON_CreateString("full_package"));
    } else {
        add_string_array(body, "capabilities", req->capabilities, req->n_capabilities);
    }
    add_string_array(body, "accepted_delta_algos", req->accepted_delta_algos,
                     req->n_accepted_delta_algos);
    json = cJSON_PrintUnformatted(body);
    cJSON_Delete(body);
    url = kirivers_project_url(c, "/update/check");
    if (!json || !url) {
        cJSON_free(json);
        free(url);
        return kirivers_error_set(err, 0, "INTERNAL_ERROR", "oom", NULL);
    }
    rc = kirivers_http(c, "POST", url, req->if_none_match, NULL, json, &resp, err);
    cJSON_free(json);
    free(url);
    if (rc != KIRIVERS_OK) {
        free_resp(c, &resp);
        return rc;
    }
    {
        int status = resp.status;
        char *etag = NULL;
        kirivers_copy_etag(&resp, &etag);
        if (status == 304) {
            out->http_status = status;
            out->etag = etag;
            free_resp(c, &resp);
            return KIRIVERS_NOT_MODIFIED;
        }
        if (status == 204) {
            out->http_status = status;
            out->etag = etag;
            free_resp(c, &resp);
            return KIRIVERS_NO_UPDATE;
        }
        {
            cJSON *o;
            if (finish_json_ok(c, &resp, &o, err) != KIRIVERS_OK) {
                free(etag);
                kirivers_check_result_free(out);
                return KIRIVERS_ERR;
            }
            rc = kirivers_parse_check(o, out);
            out->http_status = status;
            out->etag = etag;
            cJSON_Delete(o);
            free_resp(c, &resp);
            return rc;
        }
    }
}

int kirivers_client_changelog(KiriversClient *c, const char *channel, const char *os,
                              const char *arch, const KiriversChangelogQuery *query,
                              KiriversChangelog *out, KiriversError *err) {
    char *ech, *eos, *ear, *suffix, *url;
    KiriversBuf q = {0};
    KiriversHttpResponse resp;
    int nm = 0, rc;
    const char *inm = query ? query->if_none_match : NULL;
    memset(out, 0, sizeof(*out));
    if (!channel || !os || !arch) {
        return kirivers_error_set(err, 0, "INVALID_ARGUMENT", "channel, os, arch required", NULL);
    }
    ech = kirivers_url_encode(channel);
    eos = kirivers_url_encode(os);
    ear = kirivers_url_encode(arch);
    if (!ech || !eos || !ear) {
        free(ech);
        free(eos);
        free(ear);
        return kirivers_error_set(err, 0, "INTERNAL_ERROR", "oom", NULL);
    }
    {
        KiriversBuf s = {0};
        kirivers_buf_printf(&s, "/changelog/%s/%s/%s", ech, eos, ear);
        suffix = s.data;
    }
    free(ech);
    free(eos);
    free(ear);
    if (query) {
        kirivers_query_add(&q, "from_version", query->from_version);
        kirivers_query_add(&q, "to_version", query->to_version);
        kirivers_query_add(&q, "changelog_scope", query->changelog_scope);
        kirivers_query_add(&q, "changelog_layout", query->changelog_layout);
        kirivers_query_add(&q, "changelog_locale", query->changelog_locale);
        kirivers_query_add(&q, "locale", query->locale);
        kirivers_query_add_bool(&q, "changelog_include_revoked", query->changelog_include_revoked);
        kirivers_query_add_bool(&q, "changelog_include_platform_notes",
                                query->changelog_include_platform_notes);
    }
    {
        KiriversBuf full = {0};
        kirivers_buf_puts(&full, suffix);
        if (q.data) {
            kirivers_buf_puts(&full, q.data);
        }
        url = kirivers_project_url(c, full.data);
        kirivers_buf_free(&full);
    }
    free(suffix);
    kirivers_buf_free(&q);
    if (!url) {
        return kirivers_error_set(err, 0, "INTERNAL_ERROR", "oom", NULL);
    }
    rc = json_get(c, url, inm, &resp, &nm, err);
    free(url);
    if (rc != KIRIVERS_OK) {
        free_resp(c, &resp);
        return rc;
    }
    {
        int status = resp.status;
        char *etag = NULL;
        kirivers_copy_etag(&resp, &etag);
        if (nm || status == 304) {
            out->http_status = status;
            out->etag = etag;
            free_resp(c, &resp);
            return KIRIVERS_NOT_MODIFIED;
        }
        {
            cJSON *o;
            if (finish_json_ok(c, &resp, &o, err) != KIRIVERS_OK) {
                free(etag);
                kirivers_changelog_free(out);
                return KIRIVERS_ERR;
            }
            rc = kirivers_parse_changelog(o, out);
            out->http_status = status;
            out->etag = etag;
            cJSON_Delete(o);
            free_resp(c, &resp);
            return rc;
        }
    }
}

int kirivers_client_integrity(KiriversClient *c, const KiriversIntegrityQuery *query,
                              KiriversIntegrity *out, KiriversError *err) {
    char *ever, *suffix, *url;
    KiriversBuf q = {0};
    KiriversHttpResponse resp;
    int nm = 0, rc;
    memset(out, 0, sizeof(*out));
    if (!query || !query->version || !query->os || !query->arch) {
        return kirivers_error_set(err, 0, "INVALID_ARGUMENT", "version, os, arch required", NULL);
    }
    ever = kirivers_url_encode(query->version);
    {
        KiriversBuf s = {0};
        kirivers_buf_printf(&s, "/versions/%s/integrity", ever);
        suffix = s.data;
    }
    free(ever);
    kirivers_query_add(&q, "os", query->os);
    kirivers_query_add(&q, "arch", query->arch);
    kirivers_query_add(&q, "channel", query->channel);
    kirivers_query_add(&q, "hw_rev", query->hw_rev);
    kirivers_query_add(&q, "hash_algo", query->hash_algo);
    kirivers_query_add_bool(&q, "compact", query->compact);
    kirivers_query_add_bool(&q, "include_file_urls", query->include_file_urls);
    {
        KiriversBuf full = {0};
        kirivers_buf_puts(&full, suffix);
        kirivers_buf_puts(&full, q.data ? q.data : "");
        url = kirivers_project_url(c, full.data);
        kirivers_buf_free(&full);
    }
    free(suffix);
    kirivers_buf_free(&q);
    if (!url) {
        return kirivers_error_set(err, 0, "INTERNAL_ERROR", "oom", NULL);
    }
    rc = json_get(c, url, query->if_none_match, &resp, &nm, err);
    free(url);
    if (rc != KIRIVERS_OK) {
        free_resp(c, &resp);
        return rc;
    }
    {
        int status = resp.status;
        char *etag = NULL;
        kirivers_copy_etag(&resp, &etag);
        if (nm || status == 304) {
            out->http_status = status;
            out->etag = etag;
            free_resp(c, &resp);
            return KIRIVERS_NOT_MODIFIED;
        }
        {
            cJSON *o;
            if (finish_json_ok(c, &resp, &o, err) != KIRIVERS_OK) {
                free(etag);
                kirivers_integrity_free(out);
                return KIRIVERS_ERR;
            }
            rc = kirivers_parse_integrity(o, out);
            out->http_status = status;
            out->etag = etag;
            cJSON_Delete(o);
            free_resp(c, &resp);
            return rc;
        }
    }
}

int kirivers_client_diff(KiriversClient *c, const KiriversDiffRequest *req, KiriversDiffResult *out,
                         KiriversError *err) {
    cJSON *body;
    char *json, *url;
    KiriversHttpResponse resp;
    cJSON *o;
    int rc;
    memset(out, 0, sizeof(*out));
    if (!req || !req->source_version || !req->target_version || !req->os || !req->arch) {
        return kirivers_error_set(err, 0, "INVALID_ARGUMENT",
                                  "source_version, target_version, os, arch required", NULL);
    }
    body = cJSON_CreateObject();
    cJSON_AddStringToObject(body, "source_version", req->source_version);
    cJSON_AddStringToObject(body, "target_version", req->target_version);
    cJSON_AddStringToObject(body, "os", req->os);
    cJSON_AddStringToObject(body, "arch", req->arch);
    if (req->channel) {
        cJSON_AddStringToObject(body, "channel", req->channel);
    }
    if (req->hw_rev) {
        cJSON_AddStringToObject(body, "hw_rev", req->hw_rev);
    }
    if (req->device_id) {
        cJSON_AddStringToObject(body, "device_id", req->device_id);
    }
    if (req->local_sha256) {
        cJSON_AddStringToObject(body, "local_sha256", req->local_sha256);
    }
    if (req->prefer_full) {
        cJSON_AddBoolToObject(body, "prefer_full", 1);
    }
    add_string_array(body, "capabilities", req->capabilities, req->n_capabilities);
    add_string_array(body, "accepted_delta_algos", req->accepted_delta_algos,
                     req->n_accepted_delta_algos);
    json = cJSON_PrintUnformatted(body);
    cJSON_Delete(body);
    url = kirivers_project_url(c, "/update/diff");
    if (!json || !url) {
        cJSON_free(json);
        free(url);
        return kirivers_error_set(err, 0, "INTERNAL_ERROR", "oom", NULL);
    }
    rc = kirivers_http(c, "POST", url, NULL, NULL, json, &resp, err);
    cJSON_free(json);
    free(url);
    if (rc != KIRIVERS_OK) {
        free_resp(c, &resp);
        return rc;
    }
    if (finish_json_ok(c, &resp, &o, err) != KIRIVERS_OK) {
        return KIRIVERS_ERR;
    }
    rc = kirivers_parse_diff(o, out);
    cJSON_Delete(o);
    free_resp(c, &resp);
    return rc;
}

static char *pack_body_json(const KiriversPackRequest *req) {
    cJSON *body = cJSON_CreateObject();
    char *json;
    cJSON_AddStringToObject(body, "source_version", req->source_version);
    cJSON_AddStringToObject(body, "target_version", req->target_version);
    cJSON_AddStringToObject(body, "os", req->os);
    cJSON_AddStringToObject(body, "arch", req->arch);
    if (req->channel) {
        cJSON_AddStringToObject(body, "channel", req->channel);
    }
    if (req->hw_rev) {
        cJSON_AddStringToObject(body, "hw_rev", req->hw_rev);
    }
    if (req->device_id) {
        cJSON_AddStringToObject(body, "device_id", req->device_id);
    }
    add_string_array(body, "needed_paths", req->needed_paths, req->n_needed_paths);
    json = cJSON_PrintUnformatted(body);
    cJSON_Delete(body);
    return json;
}

int kirivers_client_pack(KiriversClient *c, const KiriversPackRequest *req, KiriversPackResult *out,
                         KiriversError *err) {
    char *json, *url;
    unsigned waited = 0;
    unsigned backoff;
    memset(out, 0, sizeof(*out));
    if (!req || !req->source_version || !req->target_version || !req->os || !req->arch) {
        return kirivers_error_set(err, 0, "INVALID_ARGUMENT",
                                  "source_version, target_version, os, arch required", NULL);
    }
    json = pack_body_json(req);
    url = kirivers_project_url(c, "/update/pack");
    if (!json || !url) {
        cJSON_free(json);
        free(url);
        return kirivers_error_set(err, 0, "INTERNAL_ERROR", "oom", NULL);
    }
    backoff = c->pack_backoff_ms;
    for (;;) {
        KiriversHttpResponse resp;
        cJSON *o;
        int rc = kirivers_http(c, "POST", url, NULL, NULL, json, &resp, err);
        if (rc != KIRIVERS_OK) {
            free_resp(c, &resp);
            cJSON_free(json);
            free(url);
            return rc;
        }
        {
            int status = resp.status;
            if (finish_json_ok(c, &resp, &o, err) != KIRIVERS_OK) {
                kirivers_pack_result_free(out);
                cJSON_free(json);
                free(url);
                return KIRIVERS_ERR;
            }
            kirivers_pack_result_free(out);
            if (kirivers_parse_pack(o, out) != KIRIVERS_OK) {
                cJSON_Delete(o);
                free_resp(c, &resp);
                cJSON_free(json);
                free(url);
                return kirivers_error_set(err, 0, "JSON_PARSE", "pack", NULL);
            }
            out->http_status = status;
            cJSON_Delete(o);
            free_resp(c, &resp);
            if (status != 202 && !(out->status && strcmp(out->status, "pending") == 0)) {
                cJSON_free(json);
                free(url);
                return KIRIVERS_OK;
            }
        }
        if (!req->poll || c->pack_deadline_ms == 0) {
            cJSON_free(json);
            free(url);
            return KIRIVERS_PENDING;
        }
        if (waited >= c->pack_deadline_ms) {
            cJSON_free(json);
            free(url);
            return kirivers_error_set(err, 202, "TIMEOUT", "pack poll deadline exceeded", NULL);
        }
        kirivers_sleep_ms(&c->clock, backoff);
        waited += backoff;
        if (backoff < c->pack_backoff_cap_ms) {
            unsigned next = backoff * 2u;
            backoff = next > c->pack_backoff_cap_ms ? c->pack_backoff_cap_ms : next;
        }
    }
}

static int download_abs(KiriversClient *c, const char *method, const char *url_or_path,
                        const char *range, KiriversBytes *out, KiriversError *err) {
    char *url = kirivers_join_url(c->base_url, url_or_path);
    int rc;
    memset(out, 0, sizeof(*out));
    if (!url) {
        return kirivers_error_set(err, 0, "INTERNAL_ERROR", "oom", NULL);
    }
    rc = kirivers_http_bytes(c, method, url, range, out, err);
    free(url);
    return rc;
}

int kirivers_client_download_url(KiriversClient *c, const char *url_or_path, const char *range,
                                 KiriversBytes *out, KiriversError *err) {
    if (!url_or_path) {
        return kirivers_error_set(err, 0, "INVALID_ARGUMENT", "url required", NULL);
    }
    return download_abs(c, "GET", url_or_path, range, out, err);
}

int kirivers_client_download(KiriversClient *c, const char *content_sha256, const char *query,
                             const char *range, KiriversBytes *out, KiriversError *err) {
    char *eref, *suffix, *url;
    KiriversBuf s = {0};
    int rc;
    memset(out, 0, sizeof(*out));
    if (!content_sha256) {
        return kirivers_error_set(err, 0, "INVALID_ARGUMENT", "content sha256 required", NULL);
    }
    eref = kirivers_url_encode(content_sha256);
    kirivers_buf_printf(&s, "/packages/%s", eref);
    if (query && query[0]) {
        kirivers_buf_puts(&s, query[0] == '?' ? "" : "?");
        kirivers_buf_puts(&s, query);
    }
    suffix = s.data;
    free(eref);
    url = kirivers_project_url(c, suffix);
    free(suffix);
    if (!url) {
        return kirivers_error_set(err, 0, "INTERNAL_ERROR", "oom", NULL);
    }
    rc = kirivers_http_bytes(c, "GET", url, range, out, err);
    free(url);
    return rc;
}

int kirivers_client_head_package(KiriversClient *c, const char *content_sha256, const char *query,
                                 KiriversBytes *out, KiriversError *err) {
    char *eref, *suffix, *url;
    KiriversBuf s = {0};
    int rc;
    memset(out, 0, sizeof(*out));
    if (!content_sha256) {
        return kirivers_error_set(err, 0, "INVALID_ARGUMENT", "content sha256 required", NULL);
    }
    eref = kirivers_url_encode(content_sha256);
    kirivers_buf_printf(&s, "/packages/%s", eref);
    if (query && query[0]) {
        kirivers_buf_puts(&s, query[0] == '?' ? "" : "?");
        kirivers_buf_puts(&s, query);
    }
    suffix = s.data;
    free(eref);
    url = kirivers_project_url(c, suffix);
    free(suffix);
    if (!url) {
        return kirivers_error_set(err, 0, "INTERNAL_ERROR", "oom", NULL);
    }
    rc = kirivers_http_bytes(c, "HEAD", url, NULL, out, err);
    free(url);
    return rc;
}

static int get_list(KiriversClient *c, const char *suffix, KiriversHttpResponse *resp,
                    cJSON **o, KiriversError *err) {
    char *url = kirivers_project_url(c, suffix);
    int rc;
    if (!url) {
        return kirivers_error_set(err, 0, "INTERNAL_ERROR", "oom", NULL);
    }
    rc = kirivers_http(c, "GET", url, NULL, NULL, NULL, resp, err);
    free(url);
    if (rc != KIRIVERS_OK) {
        return rc;
    }
    return finish_json_ok(c, resp, o, err);
}

int kirivers_client_channels(KiriversClient *c, KiriversChannelList *out, KiriversError *err) {
    KiriversHttpResponse resp;
    cJSON *o;
    int rc;
    memset(out, 0, sizeof(*out));
    rc = get_list(c, "/channels", &resp, &o, err);
    if (rc != KIRIVERS_OK) {
        free_resp(c, &resp);
        return rc;
    }
    rc = kirivers_parse_channels(o, out);
    cJSON_Delete(o);
    free_resp(c, &resp);
    return rc;
}

int kirivers_client_matrix(KiriversClient *c, KiriversMatrixList *out, KiriversError *err) {
    KiriversHttpResponse resp;
    cJSON *o;
    int rc;
    memset(out, 0, sizeof(*out));
    rc = get_list(c, "/matrix", &resp, &o, err);
    if (rc != KIRIVERS_OK) {
        free_resp(c, &resp);
        return rc;
    }
    rc = kirivers_parse_matrix(o, out);
    cJSON_Delete(o);
    free_resp(c, &resp);
    return rc;
}

int kirivers_client_languages(KiriversClient *c, KiriversLanguageList *out, KiriversError *err) {
    KiriversHttpResponse resp;
    cJSON *o;
    int rc;
    memset(out, 0, sizeof(*out));
    rc = get_list(c, "/languages", &resp, &o, err);
    if (rc != KIRIVERS_OK) {
        free_resp(c, &resp);
        return rc;
    }
    rc = kirivers_parse_languages(o, out);
    cJSON_Delete(o);
    free_resp(c, &resp);
    return rc;
}

int kirivers_client_announcements(KiriversClient *c, const KiriversAnnouncementQuery *query,
                                  KiriversAnnouncementList *out, KiriversError *err) {
    KiriversBuf q = {0};
    char *url;
    KiriversHttpResponse resp;
    int nm = 0, rc;
    const char *inm = query ? query->if_none_match : NULL;
    memset(out, 0, sizeof(*out));
    if (query) {
        kirivers_query_add(&q, "version", query->version);
        kirivers_query_add(&q, "os", query->os);
        kirivers_query_add(&q, "arch", query->arch);
        kirivers_query_add(&q, "locale", query->locale);
    }
    {
        KiriversBuf s = {0};
        kirivers_buf_puts(&s, "/announcements");
        if (q.data) {
            kirivers_buf_puts(&s, q.data);
        }
        url = kirivers_project_url(c, s.data);
        kirivers_buf_free(&s);
    }
    kirivers_buf_free(&q);
    if (!url) {
        return kirivers_error_set(err, 0, "INTERNAL_ERROR", "oom", NULL);
    }
    rc = json_get(c, url, inm, &resp, &nm, err);
    free(url);
    if (rc != KIRIVERS_OK) {
        free_resp(c, &resp);
        return rc;
    }
    {
        int status = resp.status;
        char *etag = NULL;
        kirivers_copy_etag(&resp, &etag);
        if (nm || status == 304) {
            out->http_status = status;
            out->etag = etag;
            free_resp(c, &resp);
            return KIRIVERS_NOT_MODIFIED;
        }
        {
            cJSON *o;
            if (finish_json_ok(c, &resp, &o, err) != KIRIVERS_OK) {
                free(etag);
                kirivers_announcement_list_free(out);
                return KIRIVERS_ERR;
            }
            rc = kirivers_parse_announcements(o, out);
            out->http_status = status;
            out->etag = etag;
            cJSON_Delete(o);
            free_resp(c, &resp);
            return rc;
        }
    }
}

int kirivers_client_telemetry(KiriversClient *c, const KiriversTelemetryRequest *req,
                              KiriversError *err) {
    cJSON *body;
    char *json, *url;
    KiriversHttpResponse resp;
    int rc;
    if (!req || !req->os || !req->arch || !req->channel || !req->from_version || !req->to_version ||
        !req->status) {
        return kirivers_error_set(err, 0, "INVALID_ARGUMENT",
                                  "os, arch, channel, from_version, to_version, status required",
                                  NULL);
    }
    body = cJSON_CreateObject();
    cJSON_AddStringToObject(body, "os", req->os);
    cJSON_AddStringToObject(body, "arch", req->arch);
    cJSON_AddStringToObject(body, "channel", req->channel);
    cJSON_AddStringToObject(body, "from_version", req->from_version);
    cJSON_AddStringToObject(body, "to_version", req->to_version);
    cJSON_AddStringToObject(body, "status", req->status);
    if (req->device_id) {
        cJSON_AddStringToObject(body, "device_id", req->device_id);
    }
    if (req->diff_mode) {
        cJSON_AddStringToObject(body, "diff_mode", req->diff_mode);
    }
    if (req->error_code) {
        cJSON_AddStringToObject(body, "error_code", req->error_code);
    }
    if (req->error_message) {
        cJSON_AddStringToObject(body, "error_message", req->error_message);
    }
    json = cJSON_PrintUnformatted(body);
    cJSON_Delete(body);
    url = kirivers_project_url(c, "/telemetry/report");
    if (!json || !url) {
        cJSON_free(json);
        free(url);
        return kirivers_error_set(err, 0, "INTERNAL_ERROR", "oom", NULL);
    }
    rc = kirivers_http(c, "POST", url, NULL, NULL, json, &resp, err);
    cJSON_free(json);
    free(url);
    if (rc != KIRIVERS_OK) {
        free_resp(c, &resp);
        return rc;
    }
    free_resp(c, &resp);
    return KIRIVERS_ACCEPTED;
}

int kirivers_client_media(KiriversClient *c, const char *media_id, const char *range,
                          KiriversBytes *out, KiriversError *err) {
    char *eid, *suffix, *url;
    int rc;
    memset(out, 0, sizeof(*out));
    if (!media_id) {
        return kirivers_error_set(err, 0, "INVALID_ARGUMENT", "media id required", NULL);
    }
    eid = kirivers_url_encode(media_id);
    {
        KiriversBuf s = {0};
        kirivers_buf_printf(&s, "/media/%s", eid);
        suffix = s.data;
    }
    free(eid);
    url = kirivers_project_url(c, suffix);
    free(suffix);
    if (!url) {
        return kirivers_error_set(err, 0, "INTERNAL_ERROR", "oom", NULL);
    }
    rc = kirivers_http_bytes(c, "GET", url, range, out, err);
    free(url);
    return rc;
}

int kirivers_client_head_media(KiriversClient *c, const char *media_id, KiriversBytes *out,
                               KiriversError *err) {
    char *eid, *suffix, *url;
    int rc;
    memset(out, 0, sizeof(*out));
    if (!media_id) {
        return kirivers_error_set(err, 0, "INVALID_ARGUMENT", "media id required", NULL);
    }
    eid = kirivers_url_encode(media_id);
    {
        KiriversBuf s = {0};
        kirivers_buf_printf(&s, "/media/%s", eid);
        suffix = s.data;
    }
    free(eid);
    url = kirivers_project_url(c, suffix);
    free(suffix);
    if (!url) {
        return kirivers_error_set(err, 0, "INTERNAL_ERROR", "oom", NULL);
    }
    rc = kirivers_http_bytes(c, "HEAD", url, NULL, out, err);
    free(url);
    return rc;
}
