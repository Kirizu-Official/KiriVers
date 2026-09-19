# KiriVers C client SDK

Official handwritten C11 client for the KiriVers **native JSON** plane. Clone this repository branch:

```text
git clone -b sdk/c https://github.com/Kirizu-Official/KiriVers.git kirivers-client-c
```

There is no language-specific package registry for C. The package root **is** the branch root. CMake is the build system. **Docker is not an SDK runtime or publish dependency.**

## Platforms

| Runtime | CMake preset | Notes |
|---------|--------------|--------|
| Windows, macOS, Linux desktop/server | `hosted` (default) | Links libcurl, OpenSSL, libzip, utf8proc |
| MCU / RTOS / other embedded | `embedded` | cJSON only; inject `Transport` and any other adapters |
| Android / HarmonyOS NDK | either | No default APK installer (`Replacer` is yours) |
| iOS | not a target | Use the Swift SDK for App Store / MDM install |

Presets do not pin a generator: CMake uses its platform default (`MinGW Makefiles` if that is what `cmake -G` lists on Windows). Override with `-G Ninja` or `-G "MinGW Makefiles"` as needed. Hosted system packages (Debian/Ubuntu names): `libcurl4-openssl-dev`, `libssl-dev`, `libzip-dev`, `libutf8proc-dev`, plus `cmake`, `pkg-config`, and a C11 compiler. Windows: MSYS2/MinGW or vcpkg equivalents of those libraries.

OpenAPI snapshot: `openapi.client.json` (`OPENAPI_REVISION` = `1.0.0 B443DEA6`). The client is handwritten; do not run OpenAPI Generator.

## CMake presets

| Preset | `KIRIVERS_EMBEDDED` | JSON | HTTP | Hash / sign | Zip | NFC | Patcher / Replacer |
|--------|---------------------|------|------|-------------|-----|-----|--------------------|
| `hosted` (default) | `OFF` | vendored [cJSON](https://github.com/DaveGamble/cJSON) | **libcurl** | **OpenSSL** | **libzip** | **utf8proc** | **interfaces only** |
| `embedded` | `ON` | vendored cJSON | **function pointer** | **function pointer** | **function pointer** | **function pointer** | **interfaces only** |

```sh
cmake --preset embedded
cmake --build --preset embedded
ctest --preset embedded

# desktop / server apps
cmake --preset hosted
cmake --build --preset hosted
ctest --preset hosted
```

Hosted requires the D18 system libraries listed above. Embedded links **only** cJSON. You must inject `Transport` (and any other adapters you need). `KIRIVERS_EMBEDDED=ON` does **not** compile or link curl, OpenSSL, libzip, or utf8proc.

JSON is not an injectable adapter (D16). Do not add json-c or a second HTTP stack.

## Adapters (ctx + vtable)

Every adapter is a `void *ctx` plus function pointers. Hosted constructors fill the vtable; embedded callers assign the pointers.

| Adapter | Hosted default | Embedded | Enables check capability |
|---------|----------------|----------|--------------------------|
| `KiriversTransport` | libcurl | **required injection** | any network call; `full_package` |
| `KiriversFileStore` | POSIX/`stdio` | injection | `file_list` when `write_all` works |
| `KiriversHasher` | OpenSSL SHA-256/MD5 | injection | download / integrity verify |
| `KiriversArchiveUnpacker` | libzip | injection | `patch_package` |
| `KiriversSignatureVerifier` | OpenSSL Ed25519 + RSA-SHA256 | injection | verify `signature` when present |
| `KiriversPatcher` | **none** | **none** | `binary_delta` + `accepted_delta_algos` from `supported_algos()` |
| `KiriversReplacer` | **none** (flash is a callback) | **none** | apply after verify — not a check capability |

Default `POST /update/check` body is `capabilities: ["full_package"]` only. `kirivers_update` derives extra flags from the live vtable (D13). Do not advertise `binary_delta` unless `Patcher.supported_algos` names algorithms `apply` can actually run. Magic split: `HDIFF13&` / `KVDIFFHP1\n` → `hdiffpatch`, `BSDIFF40` → `bsdiff`, `D6 C3 C4` → `xdelta3`. Unknown magic is an error; the updater falls back to the full package. Never cross-decode.

**This SDK does not one-click install.** There is no default `Replacer`. `kirivers_update` downloads, verifies, and stages; replacement is your callback.

### Flash / replace callback

```c
static int flash_replace(void *ctx, const char *staged, const char *install, KiriversError *err) {
    (void)ctx;
    /* Program staged bytes onto flash at install. Return KIRIVERS_OK or fill err. */
    return program_flash(staged, install, err);
}

KiriversReplacer replacer = {.ctx = NULL, .replace = flash_replace};
```

### Embedded Transport

```c
static int my_http(void *ctx, const KiriversHttpRequest *req,
                   KiriversHttpResponse *out, KiriversError *err) {
    /* Issue GET/HEAD/POST. Honor Range. Keep ?exp=&sig= on the URL. */
}

KiriversTransport t = {.ctx = &modem, .request = my_http, .free_response = my_free};
KiriversClientConfig cfg = {
    .base_url = "https://updates.example",
    .project_ref = "my-app",
    .transport = t,
};
KiriversClient *c = kirivers_client_new(&cfg, &err);
```

## Identity

The caller supplies `device_id`, channel, `custom`, `os`, `arch`, and `current_version`. The SDK never invents a device id and never writes raw `device_id` to logs.

## Example (hosted)

```c
#include "kirivers/kirivers.h"

KiriversError err = {0};
KiriversTransport t;
kirivers_transport_curl_init(&t, &err);

KiriversClientConfig cfg = {
    .base_url = "http://127.0.0.1:8080",
    .project_ref = "my-project",
    .transport = t,
};
KiriversClient *c = kirivers_client_new(&cfg, &err);

KiriversCheckRequest req = {
    .current_version = "1.0.0",
    .os = "windows",
    .arch = "x86_64",
    .channel = "stable",
    .device_id = caller_device_id,
};
KiriversCheckResult ck;
int rc = kirivers_client_check(c, &req, &ck, &err);
if (rc == KIRIVERS_NO_UPDATE) { /* already current */ }
```

Native JSON methods: `Project`, `DeviceReport`, `Check`, `Changelog`, `Integrity`, `Diff`, `Pack` (same POST polled until `ready` / `full_package`), `Download` / `HEAD` (Range, keep `exp`/`sig`), `Channels`, `Matrix`, `Languages`, `Announcements`, telemetry `Report` (202; never blocks apply), `Media`, `Health`. Store feeds and leftover paths (`GET /update/check`, `POST /clients/login`, pack/status, `/store/...`) are not implemented.

Errors use `{ "error": { "code", "message", "details" } }`. HTTP 204 on check is `KIRIVERS_NO_UPDATE`, not an error. 304 is `KIRIVERS_NOT_MODIFIED`.

## Tests

Contract tests use a fake `Transport` (no Docker, no listening server). Integration against a live client plane is skipped when `sdk-fixture.json` is absent (`KIRIVERS_SKIP_INTEGRATION=1` also skips):

```sh
cmake --preset hosted && cmake --build --preset hosted
ctest --preset hosted --output-on-failure
# or:
KIRIVERS_FIXTURE=/path/to/sdk-fixture.json KIRIVERS_BASE_URL=http://127.0.0.1:8080 \
  ./build/hosted/test_integration
```

## License

MIT. Vendored cJSON is MIT (see `vendor/cJSON/LICENSE`).
