#include "mock_transport.h"
#include "internal.h"

#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <ctype.h>

void kirivers_mock_init(KiriversMockTransport *m) { memset(m, 0, sizeof(*m)); }

void kirivers_mock_add(KiriversMockTransport *m, int status, const char *body) {
    if (m->nscripts >= KIRIVERS_MOCK_MAX_SCRIPTS) {
        return;
    }
    m->scripts[m->nscripts].status = status;
    m->scripts[m->nscripts].body = body ? body : "";
    m->nscripts++;
}

void kirivers_mock_add_bin(KiriversMockTransport *m, int status, const uint8_t *data, size_t n) {
    if (m->nscripts >= KIRIVERS_MOCK_MAX_SCRIPTS) {
        return;
    }
    m->scripts[m->nscripts].status = status;
    m->scripts[m->nscripts].bin = data;
    m->scripts[m->nscripts].bin_len = n;
    m->nscripts++;
}

static int is_leftover(const char *method, const char *url) {
    if (strstr(url, "/clients/login")) {
        return 1;
    }
    if (strstr(url, "/update/pack/status")) {
        return 1;
    }
    if (strstr(url, "/store/")) {
        return 1;
    }
    if (strstr(url, "/artifacts/")) {
        return 1;
    }
    if (strstr(url, "/manifest")) {
        return 1;
    }
    if (strstr(url, "/api/v1/ready")) {
        return 1;
    }
    if (strcmp(method, "GET") == 0 && strstr(url, "/update/check")) {
        return 1;
    }
    return 0;
}

static int mock_request(void *ctx, const KiriversHttpRequest *req, KiriversHttpResponse *out,
                        KiriversError *err) {
    KiriversMockTransport *m = (KiriversMockTransport *)ctx;
    KiriversMockCall *call;
    KiriversMockScript *sc;
    size_t i;
    memset(out, 0, sizeof(*out));
    if (m->ncalls >= KIRIVERS_MOCK_MAX_CALLS) {
        return kirivers_error_set(err, 0, "TRANSPORT", "too many mock calls", NULL);
    }
    call = &m->calls[m->ncalls++];
    memset(call, 0, sizeof(*call));
    snprintf(call->method, sizeof(call->method), "%s", req->method ? req->method : "");
    snprintf(call->url, sizeof(call->url), "%s", req->url ? req->url : "");
    if (req->body && req->body_len) {
        size_t n = req->body_len < sizeof(call->body) - 1 ? req->body_len : sizeof(call->body) - 1;
        memcpy(call->body, req->body, n);
        call->body[n] = 0;
    }
    for (i = 0; i < req->header_count; i++) {
        char line[256];
        snprintf(line, sizeof(line), "%s:%s\n", req->headers[i].name, req->headers[i].value);
        strncat(call->headers, line, sizeof(call->headers) - strlen(call->headers) - 1);
    }
    if (is_leftover(call->method, call->url)) {
        m->leftover_hit = 1;
    }
    if (m->script_i >= m->nscripts) {
        return kirivers_error_set(err, 0, "TRANSPORT", "mock script exhausted", NULL);
    }
    sc = &m->scripts[m->script_i++];
    out->status = sc->status;
    if (sc->bin) {
        out->body = (uint8_t *)malloc(sc->bin_len + 1);
        if (!out->body) {
            return kirivers_error_set(err, 0, "INTERNAL_ERROR", "oom", NULL);
        }
        memcpy(out->body, sc->bin, sc->bin_len);
        out->body[sc->bin_len] = 0;
        out->body_len = sc->bin_len;
    } else if (sc->body) {
        size_t n = strlen(sc->body);
        out->body = (uint8_t *)malloc(n + 1);
        if (!out->body) {
            return kirivers_error_set(err, 0, "INTERNAL_ERROR", "oom", NULL);
        }
        memcpy(out->body, sc->body, n + 1);
        out->body_len = n;
    }
    if (sc->etag) {
        out->header_names = (char **)calloc(1, sizeof(char *));
        out->header_values = (char **)calloc(1, sizeof(char *));
        out->header_names[0] = kirivers_strdup("ETag");
        out->header_values[0] = kirivers_strdup(sc->etag);
        out->header_count = 1;
    }
    return KIRIVERS_OK;
}

static void mock_free(void *ctx, KiriversHttpResponse *resp) {
    (void)ctx;
    kirivers_response_clear(resp);
}

void kirivers_mock_bind(KiriversMockTransport *m, KiriversTransport *t) {
    memset(t, 0, sizeof(*t));
    t->ctx = m;
    t->request = mock_request;
    t->free_response = mock_free;
}

int kirivers_mock_leftover(const KiriversMockTransport *m) { return m->leftover_hit; }
