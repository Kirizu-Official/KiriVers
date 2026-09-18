#ifndef KIRIVERS_INTERNAL_H
#define KIRIVERS_INTERNAL_H

#include "kirivers/client.h"
#include "kirivers/error.h"
#include "kirivers/types.h"

#include "cJSON.h"

#include <stddef.h>
#include <stdint.h>

struct KiriversClient {
    char *base_url;
    char *project_ref;
    char *project_token;
    char *channel_token;
    KiriversTransport transport;
    KiriversClock clock;
    unsigned pack_backoff_ms;
    unsigned pack_backoff_cap_ms;
    unsigned pack_deadline_ms;
};

char *kirivers_strdup(const char *s);
char *kirivers_strndup(const char *s, size_t n);
void kirivers_free(void *p);
int kirivers_error_set(KiriversError *err, int http_status, const char *code, const char *message,
                       const char *details_json);
int kirivers_error_from_body(int http_status, const uint8_t *body, size_t len, KiriversError *err);

typedef struct KiriversBuf {
    char *data;
    size_t len;
    size_t cap;
} KiriversBuf;

int kirivers_buf_append(KiriversBuf *b, const void *p, size_t n);
int kirivers_buf_puts(KiriversBuf *b, const char *s);
int kirivers_buf_printf(KiriversBuf *b, const char *fmt, ...);
void kirivers_buf_free(KiriversBuf *b);

char *kirivers_url_encode(const char *s);
char *kirivers_join_url(const char *base, const char *path_or_url);
char *kirivers_project_url(const KiriversClient *c, const char *suffix);
void kirivers_sleep_ms(const KiriversClock *clock, unsigned ms);

const char *kirivers_header_get(const KiriversHttpResponse *resp, const char *name);
void kirivers_response_clear(KiriversHttpResponse *resp);

int kirivers_http(KiriversClient *c, const char *method, const char *url, const char *if_none_match,
                  const char *range, const char *json_body, KiriversHttpResponse *out,
                  KiriversError *err);
int kirivers_http_bytes(KiriversClient *c, const char *method, const char *url, const char *range,
                        KiriversBytes *out, KiriversError *err);

cJSON *kirivers_json_parse_body(const KiriversHttpResponse *resp, KiriversError *err);
char *kirivers_json_str(const cJSON *o, const char *key);
int kirivers_json_bool(const cJSON *o, const char *key, int default_value);
int64_t kirivers_json_i64(const cJSON *o, const char *key, int *present);
int kirivers_json_string_array(const cJSON *arr, char ***out, size_t *n);
void kirivers_free_string_array(char **items, size_t n);

int kirivers_parse_check(const cJSON *o, KiriversCheckResult *out);
int kirivers_parse_project(const cJSON *o, KiriversProject *out);
int kirivers_parse_geo(const cJSON *o, KiriversGeoReport *out);
int kirivers_parse_health(const cJSON *o, KiriversHealth *out);
int kirivers_parse_changelog(const cJSON *o, KiriversChangelog *out);
int kirivers_parse_integrity(const cJSON *o, KiriversIntegrity *out);
int kirivers_parse_diff(const cJSON *o, KiriversDiffResult *out);
int kirivers_parse_pack(const cJSON *o, KiriversPackResult *out);
int kirivers_parse_channels(const cJSON *o, KiriversChannelList *out);
int kirivers_parse_matrix(const cJSON *o, KiriversMatrixList *out);
int kirivers_parse_languages(const cJSON *o, KiriversLanguageList *out);
int kirivers_parse_announcements(const cJSON *o, KiriversAnnouncementList *out);

int kirivers_query_add(KiriversBuf *q, const char *key, const char *value);
int kirivers_query_add_bool(KiriversBuf *q, const char *key, int v); /* -1 skip */
void kirivers_copy_etag(const KiriversHttpResponse *resp, char **dst);

#endif
