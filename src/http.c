#include "internal.h"

#include <stdlib.h>
#include <string.h>

int kirivers_http(KiriversClient *c, const char *method, const char *url, const char *if_none_match,
                  const char *range, const char *json_body, KiriversHttpResponse *out,
                  KiriversError *err) {
    KiriversHttpHeader headers[8];
    size_t n = 0;
    KiriversHttpRequest req;
    char *auth = NULL;
    int rc;

    memset(out, 0, sizeof(*out));
    if (!c || !c->transport.request) {
        return kirivers_error_set(err, 0, "NO_TRANSPORT", "Transport.request is required", NULL);
    }
    if (json_body) {
        headers[n].name = "Content-Type";
        headers[n].value = "application/json";
        n++;
        headers[n].name = "Accept";
        headers[n].value = "application/json";
        n++;
    } else {
        headers[n].name = "Accept";
        headers[n].value = "*/*";
        n++;
    }
    if (c->project_token && c->project_token[0]) {
        size_t tlen = strlen(c->project_token);
        auth = (char *)malloc(tlen + 8);
        if (!auth) {
            return kirivers_error_set(err, 0, "INTERNAL_ERROR", "oom", NULL);
        }
        memcpy(auth, "Bearer ", 7);
        memcpy(auth + 7, c->project_token, tlen + 1);
        headers[n].name = "Authorization";
        headers[n].value = auth;
        n++;
        headers[n].name = "X-Project-Token";
        headers[n].value = c->project_token;
        n++;
    }
    if (c->channel_token && c->channel_token[0]) {
        headers[n].name = "X-Channel-Token";
        headers[n].value = c->channel_token;
        n++;
    }
    if (if_none_match && if_none_match[0]) {
        headers[n].name = "If-None-Match";
        headers[n].value = if_none_match;
        n++;
    }
    if (range && range[0]) {
        headers[n].name = "Range";
        headers[n].value = range;
        n++;
    }

    memset(&req, 0, sizeof(req));
    req.method = method;
    req.url = url;
    req.headers = headers;
    req.header_count = n;
    if (json_body) {
        req.body = (const uint8_t *)json_body;
        req.body_len = strlen(json_body);
    }

    rc = c->transport.request(c->transport.ctx, &req, out, err);
    free(auth);
    if (rc != KIRIVERS_OK) {
        if (err && !err->code) {
            kirivers_error_set(err, 0, "TRANSPORT", "transport request failed", NULL);
        }
        return KIRIVERS_ERR;
    }
    if (out->status >= 400) {
        kirivers_error_from_body(out->status, out->body, out->body_len, err);
        return KIRIVERS_ERR;
    }
    return KIRIVERS_OK;
}

static void take_bytes(KiriversClient *c, KiriversHttpResponse *resp, KiriversBytes *out) {
    memset(out, 0, sizeof(*out));
    out->http_status = resp->status;
    out->data = resp->body;
    out->len = resp->body_len;
    resp->body = NULL;
    resp->body_len = 0;
    kirivers_copy_etag(resp, &out->etag);
    {
        const char *ct = kirivers_header_get(resp, "Content-Type");
        if (ct) {
            out->content_type = kirivers_strdup(ct);
        }
    }
    if (c->transport.free_response) {
        c->transport.free_response(c->transport.ctx, resp);
    } else {
        kirivers_response_clear(resp);
    }
}

int kirivers_http_bytes(KiriversClient *c, const char *method, const char *url, const char *range,
                        KiriversBytes *out, KiriversError *err) {
    KiriversHttpResponse resp;
    int rc = kirivers_http(c, method, url, NULL, range, NULL, &resp, err);
    if (rc != KIRIVERS_OK) {
        if (c->transport.free_response) {
            c->transport.free_response(c->transport.ctx, &resp);
        } else {
            kirivers_response_clear(&resp);
        }
        return rc;
    }
    take_bytes(c, &resp, out);
    return KIRIVERS_OK;
}
