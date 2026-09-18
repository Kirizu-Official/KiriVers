#include "kirivers/kirivers.h"
#include "mock_transport.h"
#include "test.h"

static void sleep_noop(void *ctx, unsigned ms) {
    (void)ctx;
    (void)ms;
}

int main(void) {
    KiriversMockTransport mock;
    KiriversTransport tr;
    KiriversClientConfig cfg;
    KiriversClient *c;
    KiriversError err;
    KiriversPackRequest req;
    KiriversPackResult res;
    const char *pending = "{\"status\":\"pending\"}";
    const char *ready = "{\"status\":\"ready\",\"package_url\":\"/pkg\",\"sha256\":\"aa\"}";

    kirivers_mock_init(&mock);
    kirivers_mock_add(&mock, 202, pending);
    kirivers_mock_add(&mock, 200, ready);
    kirivers_mock_bind(&mock, &tr);

    memset(&cfg, 0, sizeof(cfg));
    cfg.base_url = "http://example.invalid";
    cfg.project_ref = "p";
    cfg.transport = tr;
    cfg.clock.sleep_ms = sleep_noop;
    cfg.pack_backoff_ms = 1;
    cfg.pack_backoff_cap_ms = 1;
    cfg.pack_deadline_ms = 1000;
    memset(&err, 0, sizeof(err));
    c = kirivers_client_new(&cfg, &err);
    EXPECT(c);

    memset(&req, 0, sizeof(req));
    req.source_version = "1.0.0";
    req.target_version = "1.1.0";
    req.os = "windows";
    req.arch = "x86_64";
    req.poll = 1;
    EXPECT(kirivers_client_pack(c, &req, &res, &err) == KIRIVERS_OK);
    EXPECT_STREQ(res.status, "ready");
    EXPECT(mock.ncalls == 2);
    EXPECT(strcmp(mock.calls[0].body, mock.calls[1].body) == 0);
    EXPECT_CONTAINS(mock.calls[0].url, "/update/pack");
    EXPECT_STREQ(mock.calls[0].method, "POST");
    EXPECT_STREQ(mock.calls[1].method, "POST");
    kirivers_pack_result_free(&res);
    kirivers_client_free(c);
    printf("test_pack_poll ok\n");
    return 0;
}
