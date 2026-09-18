#include "kirivers/kirivers.h"
#include "mock_transport.h"
#include "test.h"

int main(void) {
    KiriversMockTransport mock;
    KiriversTransport tr;
    KiriversClientConfig cfg;
    KiriversClient *c;
    KiriversError err;
    KiriversCheckRequest req;
    KiriversCheckResult ck;

    kirivers_mock_init(&mock);
    kirivers_mock_add(&mock, 204, "");
    kirivers_mock_add(&mock, 304, "");
    kirivers_mock_bind(&mock, &tr);
    memset(&cfg, 0, sizeof(cfg));
    cfg.base_url = "http://example.invalid";
    cfg.project_ref = "p";
    cfg.transport = tr;
    memset(&err, 0, sizeof(err));
    c = kirivers_client_new(&cfg, &err);
    EXPECT(c);

    memset(&req, 0, sizeof(req));
    req.current_version = "1.0.0";
    req.os = "linux";
    req.arch = "x86_64";
    EXPECT(kirivers_client_check(c, &req, &ck, &err) == KIRIVERS_NO_UPDATE);
    kirivers_check_result_free(&ck);

    req.if_none_match = "\"abc\"";
    EXPECT(kirivers_client_check(c, &req, &ck, &err) == KIRIVERS_NOT_MODIFIED);
    kirivers_check_result_free(&ck);

    EXPECT(kirivers_client_new(NULL, &err) == NULL);
    EXPECT_STREQ(err.code, "INVALID_ARGUMENT");
    kirivers_error_clear(&err);

    kirivers_client_free(c);
    printf("test_error ok\n");
    return 0;
}
