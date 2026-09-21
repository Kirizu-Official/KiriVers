---
title: "C SDK"
description: "git clone -b sdk/c. Document both hosted curl and KIRIVERS_EMBEDDED function pointers."
---

# C

No registry. C11. CMake. JSON is vendored cJSON (not an adapter). Do not add json-c or a second HTTP stack.

## 1. Install

```text
git clone -b sdk/c https://github.com/Kirizu-Official/KiriVers.git kirivers-sdk-c
```

hosted: `cmake --preset hosted`. embedded: `KIRIVERS_EMBEDDED=ON` leaves cJSON plus function pointers.

## 2. Quick start

Hosted: initialize the curl Transport first:

```c
kirivers_transport_curl_init();
KiriversClientConfig cfg = {
  .base_url = "http://127.0.0.1:8080",
  .project_ref = "my-app",
};
KiriversClient *c = kirivers_client_new(&cfg);
KiriversCheckRequest req = {
  .current_version = "1.0.0",
  .os = "linux",
  .arch = "x86_64",
  .channel = "stable",
  .device_id = "your-stable-device-id",
};
KiriversCheckResult result;
int rc = kirivers_client_check(c, &req, &result);
/* KIRIVERS_NO_UPDATE → HTTP 204 */
```

Embedded builds inject a function-pointer Transport; do not link curl.

## 3. Download and verify

GET `package_url`, compare SHA-256, keep `exp`/`sig`, support Range.

## 4. Updater

`kirivers_update` stages verified bytes. No default Replacer.

## 5. Client methods

check, download, integrity, diff, pack poll, catalogs, announcements, telemetry, health. Not implemented: `/store/`, GET check, `clients/login`, `GET /ready`, manifest, pack/status, artifact filename.

## 6. Config and auth

`KiriversClientConfig.base_url`, `project_ref`, project/channel tokens.

## 7. Adapters

| Adapter | hosted | embedded |
|---------|--------|----------|
| Transport | libcurl | **required inject** |
| FileStore | POSIX/stdio | inject |
| Hasher | OpenSSL | inject |
| ArchiveUnpacker | libzip | inject |
| SignatureVerifier | OpenSSL | inject |
| Patcher / Replacer | none | none |

## 8. Capabilities

Default `full_package` only. `kirivers_update` expands from the live vtable. No Patcher → no `binary_delta`.

## 9. Patcher

vtable `supported_algos` + `apply`. Split by magic; unknown → full package.

## 10. Replacer

No default. Firmware uses a flash callback. Not one-click install.

## 11. Errors

`KiriversError` reads code. `KIRIVERS_NO_UPDATE` is not a failure.

## 12. Transport / tests

Embedded injects HTTP function pointers. Tests do not use Docker.

## 13. Language notes

hosted: curl/openssl/libzip/utf8proc. embedded: cJSON + function pointers only. No default Replacer.
