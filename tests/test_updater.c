#include "kirivers/kirivers.h"
#include "mock_transport.h"
#include "test.h"
#include "cJSON.h"

#include <string.h>

static int algos(void *ctx, const char *const **out, size_t *n) {
    static const char *a[] = {"bsdiff"};
    (void)ctx;
    *out = a;
    *n = 1;
    return KIRIVERS_OK;
}

static int apply_stub(void *ctx, const uint8_t *old_bytes, size_t old_len, const uint8_t *delta,
                      size_t delta_len, uint8_t **out, size_t *out_len, KiriversError *err) {
    (void)ctx;
    (void)old_bytes;
    (void)old_len;
    (void)delta;
    (void)delta_len;
    (void)out;
    (void)out_len;
    (void)err;
    return KIRIVERS_ERR;
}

int main(void) {
    KiriversMockTransport mock;
    KiriversTransport tr;
    KiriversClientConfig cfg;
    KiriversClient *c;
    KiriversUpdaterConfig uc;
    KiriversUpdater *u;
    KiriversUpdateRequest req;
    KiriversUpdateResult res;
    KiriversError err;
    const char *ck =
        "{\"has_update\":true,\"is_mandatory\":false,\"is_downgrade\":false,\"reason\":\"normal\","
        "\"compare_engine\":\"semver\",\"version_integer\":null,\"version_semver\":\"1.1.0\","
        "\"target_channel\":\"stable\",\"target_hw_rev\":null,\"package_type\":\"single_file\","
        "\"root_hash\":\"\",\"package_url\":\"/api/v1/projects/p/packages/aa\",\"file_name\":\"a.bin\","
        "\"size\":3,\"sha256\":\"ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad\","
        "\"delta_available\":false}";
    /* sha256 of "abc" */

    kirivers_mock_init(&mock);
    kirivers_mock_add(&mock, 200, ck);
    kirivers_mock_add(&mock, 202, "{\"status\":\"accepted\"}"); /* downloading telemetry */
    kirivers_mock_add_bin(&mock, 200, (const uint8_t *)"abc", 3);
    kirivers_mock_add(&mock, 202, "{\"status\":\"accepted\"}"); /* installed telemetry */
    kirivers_mock_bind(&mock, &tr);

    memset(&cfg, 0, sizeof(cfg));
    cfg.base_url = "http://example.invalid";
    cfg.project_ref = "p";
    cfg.transport = tr;
    memset(&err, 0, sizeof(err));
    c = kirivers_client_new(&cfg, &err);
    EXPECT(c);

    memset(&uc, 0, sizeof(uc));
    uc.client = c;
    uc.adapters.transport = tr;
    uc.adapters.patcher.supported_algos = algos;
    uc.adapters.patcher.apply = apply_stub;
    u = kirivers_updater_new(&uc, &err);
    EXPECT(u);

    memset(&req, 0, sizeof(req));
    req.current_version = "1.0.0";
    req.os = "windows";
    req.arch = "x86_64";
    req.channel = "stable";
    /* No hasher / filestore: download still succeeds (R4). */
    EXPECT(kirivers_update(u, &req, &res, &err) == KIRIVERS_OK);
    EXPECT(res.outcome == KIRIVERS_UPDATE_STAGED);
    EXPECT_STREQ(res.version_semver, "1.1.0");

    {
        cJSON *body = cJSON_Parse(mock.calls[0].body);
        cJSON *caps = cJSON_GetObjectItemCaseSensitive(body, "capabilities");
        cJSON *alg = cJSON_GetObjectItemCaseSensitive(body, "accepted_delta_algos");
        int i, saw_delta = 0, saw_full = 0;
        EXPECT(cJSON_IsArray(caps));
        for (i = 0; i < cJSON_GetArraySize(caps); i++) {
            const char *s = cJSON_GetArrayItem(caps, i)->valuestring;
            if (s && strcmp(s, "binary_delta") == 0) {
                saw_delta = 1;
            }
            if (s && strcmp(s, "full_package") == 0) {
                saw_full = 1;
            }
        }
        EXPECT(saw_full && saw_delta);
        EXPECT(cJSON_IsArray(alg));
        EXPECT_STREQ(cJSON_GetArrayItem(alg, 0)->valuestring, "bsdiff");
        cJSON_Delete(body);
    }

    EXPECT(res.len == 3);
    EXPECT(res.data != NULL && memcmp(res.data, "abc", 3) == 0);
    EXPECT(kirivers_mock_leftover(&mock) == 0);

    kirivers_update_result_free(&res);
    kirivers_updater_free(u);
    kirivers_client_free(c);
    printf("test_updater ok\n");
    return 0;
}
