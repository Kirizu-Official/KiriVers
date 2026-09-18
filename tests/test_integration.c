#include "kirivers/kirivers.h"
#include "cJSON.h"
#include "test.h"

#include <stdio.h>
#include <time.h>

#ifndef KIRIVERS_HOSTED
int main(void) {
    printf("test_integration skipped (embedded build has no libcurl Transport)\n");
    return 0;
}
#else

static char *read_file(const char *path) {
    FILE *f = fopen(path, "rb");
    long n;
    char *b;
    if (!f) {
        return NULL;
    }
    fseek(f, 0, SEEK_END);
    n = ftell(f);
    rewind(f);
    b = (char *)malloc((size_t)n + 1);
    if (!b) {
        fclose(f);
        return NULL;
    }
    if (fread(b, 1, (size_t)n, f) != (size_t)n) {
        free(b);
        fclose(f);
        return NULL;
    }
    b[n] = 0;
    fclose(f);
    return b;
}

int main(void) {
    const char *fixture_path = getenv("KIRIVERS_FIXTURE");
    const char *base_override = getenv("KIRIVERS_BASE_URL");
    char *raw;
    cJSON *doc;
    const char *base, *pref, *channel, *os, *arch, *cur, *target, *sha;
    char device_id[80];
    KiriversError err;
    KiriversAdapters adapters;
    KiriversClientConfig cfg;
    KiriversClient *client;
    KiriversCheckRequest creq;
    KiriversCheckResult check;
    KiriversBytes blob;
    KiriversHasher hasher;
    char hex[65];
    int rc;

    if (getenv("KIRIVERS_SKIP_INTEGRATION") && getenv("KIRIVERS_SKIP_INTEGRATION")[0] == '1') {
        printf("test_integration skipped via KIRIVERS_SKIP_INTEGRATION\n");
        return 0;
    }
    if (!fixture_path) {
        const char *try_paths[] = {
            "sdk-fixture.json",
            "D:/KiriVers/configs/sdk-fixture.json",
            "D:\\KiriVers\\configs\\sdk-fixture.json",
        };
        size_t i;
        for (i = 0; i < sizeof(try_paths) / sizeof(try_paths[0]); i++) {
            FILE *tf = fopen(try_paths[i], "rb");
            if (tf) {
                fclose(tf);
                fixture_path = try_paths[i];
                break;
            }
        }
    }
    if (!fixture_path) {
        fixture_path = "sdk-fixture.json";
    }
    raw = read_file(fixture_path);
    if (!raw) {
        fprintf(stderr, "cannot read fixture %s\n", fixture_path);
        return 1;
    }
    doc = cJSON_Parse(raw);
    free(raw);
    EXPECT(doc != NULL);
    base = base_override ? base_override
                         : cJSON_GetStringValue(cJSON_GetObjectItemCaseSensitive(doc, "client_base_url"));
    pref = cJSON_GetStringValue(cJSON_GetObjectItemCaseSensitive(doc, "project_ref"));
    channel = cJSON_GetStringValue(cJSON_GetObjectItemCaseSensitive(doc, "channel"));
    os = cJSON_GetStringValue(cJSON_GetObjectItemCaseSensitive(doc, "os"));
    arch = cJSON_GetStringValue(cJSON_GetObjectItemCaseSensitive(doc, "arch"));
    cur = cJSON_GetStringValue(cJSON_GetObjectItemCaseSensitive(doc, "current_version"));
    target = cJSON_GetStringValue(cJSON_GetObjectItemCaseSensitive(doc, "target_version"));
    {
        cJSON *sha_obj = cJSON_GetObjectItemCaseSensitive(doc, "sha256");
        sha = cJSON_GetStringValue(cJSON_GetObjectItemCaseSensitive(sha_obj, "1.1.0"));
    }
    EXPECT(base && pref && channel && os && arch && cur && target && sha);

    snprintf(device_id, sizeof(device_id), "sdk-c-%08lx%08lx", (unsigned long)time(NULL),
             (unsigned long)clock());

    memset(&err, 0, sizeof(err));
    memset(&adapters, 0, sizeof(adapters));
    EXPECT(kirivers_transport_curl_init(&adapters.transport, &err) == KIRIVERS_OK);
    EXPECT(kirivers_hasher_openssl_init(&hasher, &err) == KIRIVERS_OK);

    memset(&cfg, 0, sizeof(cfg));
    cfg.base_url = base;
    cfg.project_ref = pref;
    cfg.transport = adapters.transport;
    client = kirivers_client_new(&cfg, &err);
    EXPECT(client != NULL);

    memset(&creq, 0, sizeof(creq));
    creq.current_version = cur;
    creq.os = os;
    creq.arch = arch;
    creq.channel = channel;
    creq.device_id = device_id;
    memset(&check, 0, sizeof(check));
    rc = kirivers_client_check(client, &creq, &check, &err);
    if (rc != KIRIVERS_OK) {
        fprintf(stderr,
                "check failed rc=%d http=%d code=%s message=%s (device_id omitted from this log)\n",
                rc, err.http_status, err.code ? err.code : "?", err.message ? err.message : "?");
        return 1;
    }
    if (!check.version_semver || strcmp(check.version_semver, target) != 0) {
        fprintf(stderr, "unexpected target version %s want %s\n",
                check.version_semver ? check.version_semver : "(null)", target);
        return 1;
    }
    EXPECT(check.package_url != NULL);

    memset(&blob, 0, sizeof(blob));
    rc = kirivers_client_download_url(client, check.package_url, NULL, &blob, &err);
    if (rc != KIRIVERS_OK) {
        fprintf(stderr, "download failed code=%s message=%s\n", err.code ? err.code : "?",
                err.message ? err.message : "?");
        return 1;
    }
    memset(hex, 0, sizeof(hex));
    EXPECT(hasher.sha256(hasher.ctx, blob.data, blob.len, hex, &err) == KIRIVERS_OK);
    {
        size_t i;
        int eq = 1;
        for (i = 0; sha[i] && hex[i]; i++) {
            char a = sha[i], b = hex[i];
            if (a >= 'A' && a <= 'F') {
                a = (char)(a - 'A' + 'a');
            }
            if (b >= 'A' && b <= 'F') {
                b = (char)(b - 'A' + 'a');
            }
            if (a != b) {
                eq = 0;
                break;
            }
        }
        if (!eq || sha[i] || hex[i]) {
            fprintf(stderr, "sha256 mismatch got %s want %s\n", hex, sha);
            return 1;
        }
    }
    printf("test_integration ok version=%s bytes=%zu sha256=%s\n", check.version_semver, blob.len,
           hex);
    kirivers_bytes_free(&blob);
    kirivers_check_result_free(&check);
    kirivers_client_free(client);
    kirivers_hasher_openssl_deinit(&hasher);
    kirivers_transport_curl_deinit(&adapters.transport);
    cJSON_Delete(doc);
    return 0;
}
#endif
