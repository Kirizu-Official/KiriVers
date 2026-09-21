---
title: "C++ SDK"
description: "kirivers::Client. CheckStatus::Update. hosted vs KIRIVERS_EMBEDDED. nlohmann/json is fixed."
---

# C++

C++17. No registry. `git clone -b sdk/cpp`. JSON: vendored nlohmann/json (not an adapter). No cpp-httplib / RapidJSON.

## 1. Install

```text
git clone -b sdk/cpp https://github.com/Kirizu-Official/KiriVers.git kirivers-client-cpp
cmake -S . -B build && cmake --build build
```

Consumers: `target_link_libraries(my_app PRIVATE kirivers::client)`.

embedded: `-DKIRIVERS_EMBEDDED=ON` turns off curl/openssl/libzip/utf8proc.

## 2. Quick start

```cpp
kirivers::Client client({.base_url = "http://127.0.0.1:8080", .project_ref = "my-app"});
auto st = client.check({
    .current_version = "1.0.0",
    .os = "linux",
    .arch = "x86_64",
    .channel = "stable",
    .device_id = "your-stable-device-id",
});
if (st.status == kirivers::CheckStatus::NoUpdate ||
    st.status == kirivers::CheckStatus::NotModified) {
  return; // HTTP 204 / 304
}
```

Without a Replacer, `Updater.run` stages only.

## 3. Download and verify

Download `package_url` and compare SHA-256.

## 4. Updater

`Updater.run` without a Replacer stages only.

## 5. Client methods

Same native JSON set as C. Not implemented: `/store/`, GET check, `clients/login`, `GET /ready`, manifest, pack/status, artifact filename.

## 6. Config and auth

`kirivers::Config.base_url`, `project_ref`, tokens, PEM.

## 7. Adapters

hosted: CurlTransport, OpenSslHasher, FilesystemStore, LibzipUnpacker. embedded: inject Transport at least. Optional Windows `MoveFileExReplacer`. Patcher interface only.

## 8. Capabilities

From live adapters. No Patcher → no `binary_delta`.

## 9. Patcher

Inject apply. Do not cross-decode magics. Official `HDIFF13&` needs a caller Patcher.

## 10. Replacer

Optional `MoveFileExReplacer`. APK/IPA are caller-owned.

## 11. Errors

Read envelope code. CheckStatus distinguishes no-update.

## 12. Transport / tests

Inject Transport. `ctest` without Docker.

## 13. Language notes

hosted vs `KIRIVERS_EMBEDDED`; nlohmann/json is fixed; optional Windows `MoveFileExReplacer`.
