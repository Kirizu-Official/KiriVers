#include "kirivers/kirivers.h"
#include "mock_transport.h"
#include "test.h"
#include "cJSON.h"

#include <stdio.h>

#ifndef KIRIVERS_OPENAPI_PATH
#define KIRIVERS_OPENAPI_PATH "openapi.client.json"
#endif

static const char *OK_OBJ = "{\"ok\":true}";
static const char *CHECK_200 =
    "{\"has_update\":true,\"is_mandatory\":false,\"is_downgrade\":false,\"reason\":\"normal\","
    "\"compare_engine\":\"semver\",\"version_integer\":null,\"version_semver\":\"1.1.0\","
    "\"target_channel\":\"stable\",\"target_hw_rev\":null,\"package_type\":\"single_file\","
    "\"root_hash\":\"\",\"package_url\":\"/api/v1/projects/p/packages/abc\",\"file_name\":\"a.bin\","
    "\"size\":3,\"sha256\":\"aaa\",\"delta_available\":false}";
static const char *GEO = "{\"ip\":\"1.1.1.1\",\"country_code\":\"US\",\"region_code\":\"CA\",\"geo_i18n\":{}}";
static const char *HEALTH = "{\"status\":\"ok\",\"ready\":true}";
static const char *PROJECT = "{\"slug\":\"p\",\"uuid\":\"u\",\"compare_engine\":\"semver\"}";
static const char *CHANNELS = "{\"channels\":[{\"slug\":\"stable\",\"name\":\"Stable\",\"stability_rank\":3}]}";
static const char *MATRIX = "{\"matrix\":[{\"os\":\"windows\",\"arch\":\"x86_64\",\"package_type\":\"single_file\"}]}";
static const char *LANGS = "{\"languages\":[{\"code\":\"en\",\"display_name\":\"English\",\"is_default\":true}]}";
static const char *ANNS = "{\"announcements\":[]}";
static const char *CL = "{\"changelog\":\"hi\",\"changelog_versions\":[]}";
static const char *INTEG =
    "{\"version_integer\":null,\"version_semver\":\"1.1.0\",\"channel\":\"stable\","
    "\"package_type\":\"single_file\",\"root_hash\":\"\",\"full_package_url\":\"/x\","
    "\"file_name\":\"a.bin\",\"size\":3,\"sha256\":\"aaa\",\"files\":[]}";
static const char *DIFF =
    "{\"diff_mode\":\"full_package\",\"root_hash\":\"\",\"version_integer\":null,"
    "\"version_semver\":\"1.1.0\",\"channel\":\"stable\",\"compare_engine\":\"semver\"}";
static const char *PACK = "{\"status\":\"ready\",\"package_url\":\"/p\",\"sha256\":\"aaa\"}";
static const char *TEL = "{\"status\":\"accepted\"}";
static const char *ERR404 =
    "{\"error\":{\"code\":\"NOT_FOUND\",\"message\":\"missing\",\"details\":{\"hint\":1}}}";

static int openapi_has_path(cJSON *paths, const char *p) {
    return cJSON_GetObjectItemCaseSensitive(paths, p) != NULL;
}

int main(void) {
    KiriversMockTransport mock;
    KiriversTransport tr;
    KiriversClientConfig cfg;
    KiriversClient *c;
    KiriversError err;
    cJSON *doc, *paths;
    FILE *f;
    long sz;
    char *buf;
    size_t i;

    kirivers_mock_init(&mock);
    /* Order matches the calls below. */
    kirivers_mock_add(&mock, 200, HEALTH);
    kirivers_mock_add(&mock, 200, PROJECT);
    kirivers_mock_add(&mock, 200, GEO);
    kirivers_mock_add(&mock, 200, CHECK_200);
    kirivers_mock_add(&mock, 200, CL);
    kirivers_mock_add(&mock, 200, INTEG);
    kirivers_mock_add(&mock, 200, DIFF);
    kirivers_mock_add(&mock, 200, PACK);
    kirivers_mock_add_bin(&mock, 200, (const uint8_t *)"abc", 3);
    kirivers_mock_add(&mock, 200, "");
    kirivers_mock_add(&mock, 200, CHANNELS);
    kirivers_mock_add(&mock, 200, MATRIX);
    kirivers_mock_add(&mock, 200, LANGS);
    kirivers_mock_add(&mock, 200, ANNS);
    kirivers_mock_add(&mock, 202, TEL);
    kirivers_mock_add_bin(&mock, 200, (const uint8_t *)"img", 3);
    kirivers_mock_add(&mock, 200, "");
    kirivers_mock_add(&mock, 404, ERR404);
    kirivers_mock_bind(&mock, &tr);

    memset(&cfg, 0, sizeof(cfg));
    cfg.base_url = "http://127.0.0.1:8080";
    cfg.project_ref = "sdk-fixture";
    cfg.transport = tr;
    memset(&err, 0, sizeof(err));
    c = kirivers_client_new(&cfg, &err);
    EXPECT(c != NULL);

    {
        KiriversHealth h;
        EXPECT(kirivers_client_health(c, &h, &err) == KIRIVERS_OK);
        EXPECT_STREQ(h.status, "ok");
        EXPECT(h.ready == 1);
        kirivers_health_free(&h);
    }
    {
        KiriversProject p;
        EXPECT(kirivers_client_project(c, &p, &err) == KIRIVERS_OK);
        kirivers_project_free(&p);
    }
    {
        KiriversDeviceReportRequest r = {0};
        KiriversGeoReport g;
        r.device_id = "dev-1";
        r.os = "windows";
        r.arch = "x86_64";
        EXPECT(kirivers_client_device_report(c, &r, &g, &err) == KIRIVERS_OK);
        EXPECT_STREQ(g.ip, "1.1.1.1");
        kirivers_geo_report_free(&g);
    }
    {
        KiriversCheckRequest r = {0};
        KiriversCheckResult ck;
        r.current_version = "1.0.0";
        r.os = "windows";
        r.arch = "x86_64";
        r.channel = "stable";
        EXPECT(kirivers_client_check(c, &r, &ck, &err) == KIRIVERS_OK);
        EXPECT_STREQ(ck.version_semver, "1.1.0");
        EXPECT(ck.has_update == 1);
        kirivers_check_result_free(&ck);
    }
    {
        KiriversChangelog cl;
        EXPECT(kirivers_client_changelog(c, "stable", "windows", "x86_64", NULL, &cl, &err) ==
               KIRIVERS_OK);
        kirivers_changelog_free(&cl);
    }
    {
        KiriversIntegrityQuery q = {0};
        KiriversIntegrity in;
        q.version = "1.1.0";
        q.os = "windows";
        q.arch = "x86_64";
        q.compact = -1;
        q.include_file_urls = -1;
        EXPECT(kirivers_client_integrity(c, &q, &in, &err) == KIRIVERS_OK);
        kirivers_integrity_free(&in);
    }
    {
        KiriversDiffRequest d = {0};
        KiriversDiffResult dr;
        d.source_version = "1.0.0";
        d.target_version = "1.1.0";
        d.os = "windows";
        d.arch = "x86_64";
        EXPECT(kirivers_client_diff(c, &d, &dr, &err) == KIRIVERS_OK);
        kirivers_diff_result_free(&dr);
    }
    {
        KiriversPackRequest p = {0};
        KiriversPackResult pr;
        p.source_version = "1.0.0";
        p.target_version = "1.1.0";
        p.os = "windows";
        p.arch = "x86_64";
        EXPECT(kirivers_client_pack(c, &p, &pr, &err) == KIRIVERS_OK);
        kirivers_pack_result_free(&pr);
    }
    {
        KiriversBytes b;
        EXPECT(kirivers_client_download(c, "abc", "exp=1&sig=2", "bytes=0-1", &b, &err) ==
               KIRIVERS_OK);
        EXPECT(b.len == 3);
        kirivers_bytes_free(&b);
    }
    {
        KiriversBytes b;
        EXPECT(kirivers_client_head_package(c, "abc", NULL, &b, &err) == KIRIVERS_OK);
        kirivers_bytes_free(&b);
    }
    {
        KiriversChannelList l;
        EXPECT(kirivers_client_channels(c, &l, &err) == KIRIVERS_OK);
        kirivers_channel_list_free(&l);
    }
    {
        KiriversMatrixList l;
        EXPECT(kirivers_client_matrix(c, &l, &err) == KIRIVERS_OK);
        kirivers_matrix_list_free(&l);
    }
    {
        KiriversLanguageList l;
        EXPECT(kirivers_client_languages(c, &l, &err) == KIRIVERS_OK);
        kirivers_language_list_free(&l);
    }
    {
        KiriversAnnouncementList l;
        EXPECT(kirivers_client_announcements(c, NULL, &l, &err) == KIRIVERS_OK);
        kirivers_announcement_list_free(&l);
    }
    {
        KiriversTelemetryRequest t = {0};
        t.os = "windows";
        t.arch = "x86_64";
        t.channel = "stable";
        t.from_version = "1.0.0";
        t.to_version = "1.1.0";
        t.status = "installed";
        EXPECT(kirivers_client_telemetry(c, &t, &err) == KIRIVERS_ACCEPTED);
    }
    {
        KiriversBytes b;
        EXPECT(kirivers_client_media(c, "mid", NULL, &b, &err) == KIRIVERS_OK);
        kirivers_bytes_free(&b);
    }
    {
        KiriversBytes b;
        EXPECT(kirivers_client_head_media(c, "mid", &b, &err) == KIRIVERS_OK);
        kirivers_bytes_free(&b);
    }
    {
        KiriversProject p;
        kirivers_error_clear(&err);
        EXPECT(kirivers_client_project(c, &p, &err) == KIRIVERS_ERR);
        EXPECT_STREQ(err.code, "NOT_FOUND");
        EXPECT_STREQ(err.message, "missing");
        EXPECT_CONTAINS(err.details_json, "hint");
        kirivers_error_clear(&err);
        kirivers_project_free(&p);
    }

    EXPECT(kirivers_mock_leftover(&mock) == 0);
    {
        size_t k;
        for (k = 0; k < mock.ncalls; k++) {
            EXPECT(!strstr(mock.calls[k].url, "/clients/login"));
            EXPECT(!strstr(mock.calls[k].url, "/update/pack/status"));
            EXPECT(!strstr(mock.calls[k].url, "/store/"));
            EXPECT(!strstr(mock.calls[k].url, "/artifacts/"));
            EXPECT(!strstr(mock.calls[k].url, "/manifest"));
            EXPECT(!strstr(mock.calls[k].url, "/api/v1/ready"));
            if (strcmp(mock.calls[k].method, "GET") == 0) {
                EXPECT(!strstr(mock.calls[k].url, "/update/check"));
            }
        }
    }

    /* Path + method contract. */
    {
        struct {
            const char *method;
            const char *frag;
        } want[] = {
            {"GET", "/api/v1/health"},
            {"GET", "/api/v1/projects/sdk-fixture"},
            {"POST", "/clients/report"},
            {"POST", "/update/check"},
            {"GET", "/changelog/stable/windows/x86_64"},
            {"GET", "/versions/1.1.0/integrity"},
            {"POST", "/update/diff"},
            {"POST", "/update/pack"},
            {"GET", "/packages/abc"},
            {"HEAD", "/packages/abc"},
            {"GET", "/channels"},
            {"GET", "/matrix"},
            {"GET", "/languages"},
            {"GET", "/announcements"},
            {"POST", "/telemetry/report"},
            {"GET", "/media/mid"},
            {"HEAD", "/media/mid"},
        };
        for (i = 0; i < sizeof(want) / sizeof(want[0]); i++) {
            int found = 0;
            size_t k;
            for (k = 0; k < mock.ncalls; k++) {
                if (strcmp(mock.calls[k].method, want[i].method) == 0 &&
                    strstr(mock.calls[k].url, want[i].frag)) {
                    found = 1;
                    break;
                }
            }
            if (!found) {
                fprintf(stderr, "missing %s %s\n", want[i].method, want[i].frag);
            }
            EXPECT(found);
        }
    }

    /* Check body required fields; default capability; no leftover check params. */
    {
        cJSON *body = NULL;
        size_t k;
        for (k = 0; k < mock.ncalls; k++) {
            if (strcmp(mock.calls[k].method, "POST") == 0 && strstr(mock.calls[k].url, "/update/check")) {
                body = cJSON_Parse(mock.calls[k].body);
                break;
            }
        }
        EXPECT(body != NULL);
        EXPECT(cJSON_IsString(cJSON_GetObjectItemCaseSensitive(body, "current_version")));
        EXPECT(cJSON_IsString(cJSON_GetObjectItemCaseSensitive(body, "os")));
        EXPECT(cJSON_IsString(cJSON_GetObjectItemCaseSensitive(body, "arch")));
        {
            cJSON *caps = cJSON_GetObjectItemCaseSensitive(body, "capabilities");
            EXPECT(cJSON_IsArray(caps));
            EXPECT(cJSON_GetArraySize(caps) == 1);
            EXPECT_STREQ(cJSON_GetArrayItem(caps, 0)->valuestring, "full_package");
        }
        EXPECT(cJSON_GetObjectItemCaseSensitive(body, "local_sha256") == NULL);
        EXPECT(cJSON_GetObjectItemCaseSensitive(body, "dirty_paths") == NULL);
        EXPECT(cJSON_GetObjectItemCaseSensitive(body, "accepted_delta_algos") == NULL);
        cJSON_Delete(body);
    }

    /* Private download keeps exp/sig query and Range. */
    {
        int saw_query = 0, saw_range = 0;
        size_t k;
        for (k = 0; k < mock.ncalls; k++) {
            if (strcmp(mock.calls[k].method, "GET") == 0 && strstr(mock.calls[k].url, "/packages/abc") &&
                strstr(mock.calls[k].url, "exp=1") && strstr(mock.calls[k].url, "sig=2")) {
                saw_query = 1;
                if (strstr(mock.calls[k].headers, "Range:") &&
                    strstr(mock.calls[k].headers, "bytes=0-1")) {
                    saw_range = 1;
                }
            }
        }
        EXPECT(saw_query);
        EXPECT(saw_range);
    }

    f = fopen(KIRIVERS_OPENAPI_PATH, "rb");
    EXPECT(f != NULL);
    fseek(f, 0, SEEK_END);
    sz = ftell(f);
    rewind(f);
    buf = (char *)malloc((size_t)sz + 1);
    EXPECT(buf != NULL);
    EXPECT(fread(buf, 1, (size_t)sz, f) == (size_t)sz);
    buf[sz] = 0;
    fclose(f);
    doc = cJSON_Parse(buf);
    free(buf);
    EXPECT(doc != NULL);
    paths = cJSON_GetObjectItemCaseSensitive(doc, "paths");
    EXPECT(paths != NULL);
    EXPECT(openapi_has_path(paths, "/api/v1/health"));
    EXPECT(openapi_has_path(paths, "/api/v1/projects/{project_ref}"));
    EXPECT(openapi_has_path(paths, "/api/v1/projects/{project_ref}/clients/report"));
    EXPECT(openapi_has_path(paths, "/api/v1/projects/{project_ref}/update/check"));
    EXPECT(openapi_has_path(paths, "/api/v1/projects/{project_ref}/changelog/{channel}/{os}/{arch}"));
    EXPECT(openapi_has_path(paths, "/api/v1/projects/{project_ref}/versions/{version}/integrity"));
    EXPECT(openapi_has_path(paths, "/api/v1/projects/{project_ref}/update/diff"));
    EXPECT(openapi_has_path(paths, "/api/v1/projects/{project_ref}/update/pack"));
    EXPECT(openapi_has_path(paths, "/api/v1/projects/{project_ref}/packages/{ref}"));
    EXPECT(openapi_has_path(paths, "/api/v1/projects/{project_ref}/channels"));
    EXPECT(openapi_has_path(paths, "/api/v1/projects/{project_ref}/matrix"));
    EXPECT(openapi_has_path(paths, "/api/v1/projects/{project_ref}/languages"));
    EXPECT(openapi_has_path(paths, "/api/v1/projects/{project_ref}/announcements"));
    EXPECT(openapi_has_path(paths, "/api/v1/projects/{project_ref}/telemetry/report"));
    EXPECT(openapi_has_path(paths, "/api/v1/projects/{project_ref}/media/{id}"));
    /* Leftover paths must not be implemented; they may still exist as store docs. */
    {
        cJSON *check = cJSON_GetObjectItemCaseSensitive(
            paths, "/api/v1/projects/{project_ref}/update/check");
        EXPECT(cJSON_GetObjectItemCaseSensitive(check, "get") == NULL);
        EXPECT(cJSON_GetObjectItemCaseSensitive(check, "post") != NULL);
    }
    cJSON_Delete(doc);

    kirivers_client_free(c);
    printf("test_contract ok\n");
    (void)OK_OBJ;
    return 0;
}
