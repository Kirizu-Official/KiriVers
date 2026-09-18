#if !defined(_WIN32)
#ifndef _POSIX_C_SOURCE
#define _POSIX_C_SOURCE 200809L
#endif
#endif

#include "internal.h"

#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <stdarg.h>
#include <ctype.h>

#ifdef _WIN32
#include <windows.h>
#else
#include <time.h>
#endif

char *kirivers_strdup(const char *s) {
    size_t n;
    char *d;
    if (!s) {
        return NULL;
    }
    n = strlen(s);
    d = (char *)malloc(n + 1);
    if (!d) {
        return NULL;
    }
    memcpy(d, s, n + 1);
    return d;
}

char *kirivers_strndup(const char *s, size_t n) {
    char *d;
    size_t i;
    if (!s) {
        return NULL;
    }
    d = (char *)malloc(n + 1);
    if (!d) {
        return NULL;
    }
    for (i = 0; i < n && s[i]; i++) {
        d[i] = s[i];
    }
    d[i] = 0;
    return d;
}

void kirivers_free(void *p) { free(p); }

int kirivers_error_set(KiriversError *err, int http_status, const char *code, const char *message,
                       const char *details_json) {
    if (!err) {
        return KIRIVERS_ERR;
    }
    kirivers_error_clear(err);
    err->http_status = http_status;
    err->code = kirivers_strdup(code ? code : "INTERNAL_ERROR");
    err->message = kirivers_strdup(message ? message : "");
    err->details_json = details_json ? kirivers_strdup(details_json) : NULL;
    return KIRIVERS_ERR;
}

void kirivers_error_clear(KiriversError *err) {
    if (!err) {
        return;
    }
    free(err->code);
    free(err->message);
    free(err->details_json);
    memset(err, 0, sizeof(*err));
}

int kirivers_error_from_body(int http_status, const uint8_t *body, size_t len, KiriversError *err) {
    cJSON *root, *error, *details;
    char *details_json = NULL;
    const char *code = "HTTP_ERROR";
    const char *message = "";
    char tmp[64];
    int rc;

    if (!body || len == 0) {
        snprintf(tmp, sizeof(tmp), "HTTP %d", http_status);
        return kirivers_error_set(err, http_status, "HTTP_ERROR", tmp, NULL);
    }
    root = cJSON_ParseWithLength((const char *)body, len);
    if (!root) {
        char *snippet = kirivers_strndup((const char *)body, len > 180 ? 180 : len);
        rc = kirivers_error_set(err, http_status, "HTTP_ERROR", snippet ? snippet : "invalid json",
                                NULL);
        free(snippet);
        return rc;
    }
    error = cJSON_GetObjectItemCaseSensitive(root, "error");
    if (cJSON_IsObject(error)) {
        cJSON *c = cJSON_GetObjectItemCaseSensitive(error, "code");
        cJSON *m = cJSON_GetObjectItemCaseSensitive(error, "message");
        if (cJSON_IsString(c) && c->valuestring) {
            code = c->valuestring;
        }
        if (cJSON_IsString(m) && m->valuestring) {
            message = m->valuestring;
        }
        details = cJSON_GetObjectItemCaseSensitive(error, "details");
        if (details && !cJSON_IsNull(details)) {
            details_json = cJSON_PrintUnformatted(details);
        }
    }
    rc = kirivers_error_set(err, http_status, code, message, details_json);
    if (details_json) {
        cJSON_free(details_json);
    }
    cJSON_Delete(root);
    return rc;
}

int kirivers_buf_append(KiriversBuf *b, const void *p, size_t n) {
    if (!b) {
        return KIRIVERS_ERR;
    }
    if (b->len + n + 1 > b->cap) {
        size_t cap = b->cap ? b->cap : 64;
        char *nd;
        while (cap < b->len + n + 1) {
            cap *= 2;
        }
        nd = (char *)realloc(b->data, cap);
        if (!nd) {
            return KIRIVERS_ERR;
        }
        b->data = nd;
        b->cap = cap;
    }
    if (n && p) {
        memcpy(b->data + b->len, p, n);
    }
    b->len += n;
    b->data[b->len] = 0;
    return KIRIVERS_OK;
}

int kirivers_buf_puts(KiriversBuf *b, const char *s) {
    return kirivers_buf_append(b, s, s ? strlen(s) : 0);
}

int kirivers_buf_printf(KiriversBuf *b, const char *fmt, ...) {
    char tmp[1024];
    va_list ap;
    int n;
    va_start(ap, fmt);
    n = vsnprintf(tmp, sizeof(tmp), fmt, ap);
    va_end(ap);
    if (n < 0) {
        return KIRIVERS_ERR;
    }
    if ((size_t)n < sizeof(tmp)) {
        return kirivers_buf_append(b, tmp, (size_t)n);
    }
    {
        char *big = (char *)malloc((size_t)n + 1);
        if (!big) {
            return KIRIVERS_ERR;
        }
        va_start(ap, fmt);
        vsnprintf(big, (size_t)n + 1, fmt, ap);
        va_end(ap);
        n = kirivers_buf_append(b, big, (size_t)n);
        free(big);
        return n;
    }
}

void kirivers_buf_free(KiriversBuf *b) {
    if (!b) {
        return;
    }
    free(b->data);
    memset(b, 0, sizeof(*b));
}

char *kirivers_url_encode(const char *s) {
    static const char hex[] = "0123456789ABCDEF";
    KiriversBuf b = {0};
    size_t i;
    if (!s) {
        return kirivers_strdup("");
    }
    for (i = 0; s[i]; i++) {
        unsigned char c = (unsigned char)s[i];
        if ((c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c == '-' ||
            c == '_' || c == '.' || c == '~') {
            if (kirivers_buf_append(&b, &c, 1) != KIRIVERS_OK) {
                kirivers_buf_free(&b);
                return NULL;
            }
        } else {
            char esc[3] = {'%', hex[c >> 4], hex[c & 15]};
            if (kirivers_buf_append(&b, esc, 3) != KIRIVERS_OK) {
                kirivers_buf_free(&b);
                return NULL;
            }
        }
    }
    return b.data;
}

static int is_absolute_url(const char *s) {
    return s && (strncmp(s, "http://", 7) == 0 || strncmp(s, "https://", 8) == 0);
}

char *kirivers_join_url(const char *base, const char *path_or_url) {
    KiriversBuf b = {0};
    const char *slash;
    if (!path_or_url || !*path_or_url) {
        return kirivers_strdup(base ? base : "");
    }
    if (is_absolute_url(path_or_url)) {
        return kirivers_strdup(path_or_url);
    }
    if (path_or_url[0] == '/') {
        /* origin of base + path */
        const char *p = base ? base : "";
        const char *scheme = strstr(p, "://");
        if (scheme) {
            const char *hostend = strchr(scheme + 3, '/');
            size_t n = hostend ? (size_t)(hostend - p) : strlen(p);
            if (kirivers_buf_append(&b, p, n) != KIRIVERS_OK) {
                kirivers_buf_free(&b);
                return NULL;
            }
        } else if (kirivers_buf_puts(&b, p) != KIRIVERS_OK) {
            kirivers_buf_free(&b);
            return NULL;
        }
        if (kirivers_buf_puts(&b, path_or_url) != KIRIVERS_OK) {
            kirivers_buf_free(&b);
            return NULL;
        }
        return b.data;
    }
    if (kirivers_buf_puts(&b, base ? base : "") != KIRIVERS_OK) {
        kirivers_buf_free(&b);
        return NULL;
    }
    slash = b.len && b.data[b.len - 1] == '/' ? "" : "/";
    if (kirivers_buf_puts(&b, slash) != KIRIVERS_OK ||
        kirivers_buf_puts(&b, path_or_url) != KIRIVERS_OK) {
        kirivers_buf_free(&b);
        return NULL;
    }
    return b.data;
}

char *kirivers_project_url(const KiriversClient *c, const char *suffix) {
    char *enc, *path, *url;
    KiriversBuf b = {0};
    if (!c) {
        return NULL;
    }
    enc = kirivers_url_encode(c->project_ref ? c->project_ref : "");
    if (!enc) {
        return NULL;
    }
    if (kirivers_buf_printf(&b, "/api/v1/projects/%s", enc) != KIRIVERS_OK) {
        free(enc);
        kirivers_buf_free(&b);
        return NULL;
    }
    free(enc);
    if (suffix && kirivers_buf_puts(&b, suffix) != KIRIVERS_OK) {
        kirivers_buf_free(&b);
        return NULL;
    }
    path = b.data;
    url = kirivers_join_url(c->base_url, path);
    free(path);
    return url;
}

void kirivers_sleep_ms(const KiriversClock *clock, unsigned ms) {
    if (clock && clock->sleep_ms) {
        clock->sleep_ms(clock->ctx, ms);
        return;
    }
#ifdef _WIN32
    Sleep(ms);
#else
    {
        struct timespec ts;
        ts.tv_sec = (time_t)(ms / 1000u);
        ts.tv_nsec = (long)(ms % 1000u) * 1000000L;
        nanosleep(&ts, NULL);
    }
#endif
}

const char *kirivers_header_get(const KiriversHttpResponse *resp, const char *name) {
    size_t i;
    if (!resp || !name) {
        return NULL;
    }
    for (i = 0; i < resp->header_count; i++) {
        const char *a = resp->header_names[i];
        const char *b = name;
        if (!a) {
            continue;
        }
        while (*a && *b && tolower((unsigned char)*a) == tolower((unsigned char)*b)) {
            a++;
            b++;
        }
        if (*a == 0 && *b == 0) {
            return resp->header_values[i];
        }
    }
    return NULL;
}

void kirivers_response_clear(KiriversHttpResponse *resp) {
    size_t i;
    if (!resp) {
        return;
    }
    for (i = 0; i < resp->header_count; i++) {
        free(resp->header_names ? resp->header_names[i] : NULL);
        free(resp->header_values ? resp->header_values[i] : NULL);
    }
    free(resp->header_names);
    free(resp->header_values);
    free(resp->body);
    memset(resp, 0, sizeof(*resp));
}

int kirivers_query_add(KiriversBuf *q, const char *key, const char *value) {
    char *ek, *ev;
    int rc;
    if (!value || !key) {
        return KIRIVERS_OK;
    }
    ek = kirivers_url_encode(key);
    ev = kirivers_url_encode(value);
    if (!ek || !ev) {
        free(ek);
        free(ev);
        return KIRIVERS_ERR;
    }
    rc = kirivers_buf_printf(q, "%s%s=%s", q->len ? "&" : "?", ek, ev);
    free(ek);
    free(ev);
    return rc;
}

int kirivers_query_add_bool(KiriversBuf *q, const char *key, int v) {
    if (v < 0) {
        return KIRIVERS_OK;
    }
    return kirivers_query_add(q, key, v ? "true" : "false");
}

void kirivers_free_string_array(char **items, size_t n) {
    size_t i;
    if (!items) {
        return;
    }
    for (i = 0; i < n; i++) {
        free(items[i]);
    }
    free(items);
}
