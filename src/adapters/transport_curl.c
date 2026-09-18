#include "kirivers/hosted.h"
#include "internal.h"

#include <curl/curl.h>
#include <stdlib.h>
#include <string.h>
#include <ctype.h>

typedef struct CurlState {
    int global_ok;
} CurlState;

static int g_curl_refs = 0;

typedef struct CurlBuf {
    uint8_t *data;
    size_t len;
    size_t cap;
} CurlBuf;

typedef struct CurlHdrs {
    char **names;
    char **values;
    size_t count;
} CurlHdrs;

static size_t write_cb(char *ptr, size_t size, size_t nmemb, void *userdata) {
    CurlBuf *b = (CurlBuf *)userdata;
    size_t n = size * nmemb;
    if (b->len + n + 1 > b->cap) {
        size_t cap = b->cap ? b->cap : 4096;
        uint8_t *nd;
        while (cap < b->len + n + 1) {
            cap *= 2;
        }
        nd = (uint8_t *)realloc(b->data, cap);
        if (!nd) {
            return 0;
        }
        b->data = nd;
        b->cap = cap;
    }
    memcpy(b->data + b->len, ptr, n);
    b->len += n;
    b->data[b->len] = 0;
    return n;
}

static size_t hdr_cb(char *buffer, size_t size, size_t nitems, void *userdata) {
    CurlHdrs *h = (CurlHdrs *)userdata;
    size_t n = size * nitems;
    char *line, *colon;
    size_t i;
    if (n < 3) {
        return n;
    }
    line = (char *)malloc(n + 1);
    if (!line) {
        return 0;
    }
    memcpy(line, buffer, n);
    line[n] = 0;
    while (n && (line[n - 1] == '\n' || line[n - 1] == '\r')) {
        line[--n] = 0;
    }
    colon = strchr(line, ':');
    if (!colon) {
        free(line);
        return size * nitems;
    }
    *colon = 0;
    {
        char *name = line;
        char *val = colon + 1;
        char **nn, **vv;
        while (*val == ' ' || *val == '\t') {
            val++;
        }
        nn = (char **)realloc(h->names, (h->count + 1) * sizeof(char *));
        if (!nn) {
            free(line);
            return 0;
        }
        h->names = nn;
        vv = (char **)realloc(h->values, (h->count + 1) * sizeof(char *));
        if (!vv) {
            free(line);
            return 0;
        }
        h->values = vv;
        h->names[h->count] = kirivers_strdup(name);
        h->values[h->count] = kirivers_strdup(val);
        h->count++;
        (void)i;
    }
    free(line);
    return size * nitems;
}

static int curl_request(void *ctx, const KiriversHttpRequest *req, KiriversHttpResponse *out,
                        KiriversError *err) {
    CURL *easy;
    struct curl_slist *hdrs = NULL;
    CurlBuf body = {0};
    CurlHdrs rh = {0};
    long status = 0;
    size_t i;
    CURLcode cr;
    (void)ctx;
    memset(out, 0, sizeof(*out));
    easy = curl_easy_init();
    if (!easy) {
        return kirivers_error_set(err, 0, "TRANSPORT", "curl_easy_init failed", NULL);
    }
    curl_easy_setopt(easy, CURLOPT_URL, req->url);
    curl_easy_setopt(easy, CURLOPT_CUSTOMREQUEST, req->method);
    if (req->method && strcmp(req->method, "HEAD") == 0) {
        curl_easy_setopt(easy, CURLOPT_NOBODY, 1L);
    }
    curl_easy_setopt(easy, CURLOPT_FOLLOWLOCATION, 1L);
    curl_easy_setopt(easy, CURLOPT_WRITEFUNCTION, write_cb);
    curl_easy_setopt(easy, CURLOPT_WRITEDATA, &body);
    curl_easy_setopt(easy, CURLOPT_HEADERFUNCTION, hdr_cb);
    curl_easy_setopt(easy, CURLOPT_HEADERDATA, &rh);
    curl_easy_setopt(easy, CURLOPT_USERAGENT, "kirivers-client-c/0.1.0");
    if (req->body && req->body_len) {
        curl_easy_setopt(easy, CURLOPT_POSTFIELDS, req->body);
        curl_easy_setopt(easy, CURLOPT_POSTFIELDSIZE, (long)req->body_len);
    }
    for (i = 0; i < req->header_count; i++) {
        char line[1024];
        snprintf(line, sizeof(line), "%s: %s", req->headers[i].name, req->headers[i].value);
        hdrs = curl_slist_append(hdrs, line);
    }
    if (hdrs) {
        curl_easy_setopt(easy, CURLOPT_HTTPHEADER, hdrs);
    }
    cr = curl_easy_perform(easy);
    curl_easy_getinfo(easy, CURLINFO_RESPONSE_CODE, &status);
    curl_slist_free_all(hdrs);
    curl_easy_cleanup(easy);
    if (cr != CURLE_OK) {
        free(body.data);
        for (i = 0; i < rh.count; i++) {
            free(rh.names[i]);
            free(rh.values[i]);
        }
        free(rh.names);
        free(rh.values);
        return kirivers_error_set(err, 0, "TRANSPORT", curl_easy_strerror(cr), NULL);
    }
    out->status = (int)status;
    out->body = body.data;
    out->body_len = body.len;
    out->header_names = rh.names;
    out->header_values = rh.values;
    out->header_count = rh.count;
    return KIRIVERS_OK;
}

static void curl_free_response(void *ctx, KiriversHttpResponse *resp) {
    (void)ctx;
    kirivers_response_clear(resp);
}

int kirivers_transport_curl_init(KiriversTransport *out, KiriversError *err) {
    CurlState *st;
    memset(out, 0, sizeof(*out));
    if (g_curl_refs == 0) {
        if (curl_global_init(CURL_GLOBAL_DEFAULT) != 0) {
            return kirivers_error_set(err, 0, "TRANSPORT", "curl_global_init failed", NULL);
        }
    }
    g_curl_refs++;
    st = (CurlState *)calloc(1, sizeof(*st));
    if (!st) {
        g_curl_refs--;
        if (g_curl_refs == 0) {
            curl_global_cleanup();
        }
        return kirivers_error_set(err, 0, "INTERNAL_ERROR", "oom", NULL);
    }
    st->global_ok = 1;
    out->ctx = st;
    out->request = curl_request;
    out->free_response = curl_free_response;
    return KIRIVERS_OK;
}

void kirivers_transport_curl_deinit(KiriversTransport *t) {
    if (!t) {
        return;
    }
    if (t->ctx) {
        CurlState *st = (CurlState *)t->ctx;
        if (st->global_ok && g_curl_refs > 0) {
            g_curl_refs--;
            if (g_curl_refs == 0) {
                curl_global_cleanup();
            }
        }
        free(st);
    }
    memset(t, 0, sizeof(*t));
}
