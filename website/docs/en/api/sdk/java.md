---
title: "Java SDK"
description: "official.kirizu:kirivers-client. Config.builder(). Optional FilesMoveReplacer. No OkHttp/Gson."
---

# Java

## 1. Install

Maven `official.kirizu:kirivers-client` (JDK 17). Runtime: Jackson databind only.

```xml
<dependency>
  <groupId>official.kirizu</groupId>
  <artifactId>kirivers-client</artifactId>
  <version>REPLACE_WITH_RELEASE</version>
</dependency>
```

::: warning Not on Maven Central yet
```text
git clone -b sdk/java --single-branch https://github.com/Kirizu-Official/KiriVers.git kirivers-client-java
cd kirivers-client-java && ./mvnw install
```
:::

## 2. Quick start

```java
var client = new Client(Config.builder()
    .baseUrl("http://127.0.0.1:8080")
    .projectRef("my-app")
    .build());
var result = client.check(UpdateCheckRequest.builder()
    .currentVersion("1.0.0")
    .os("windows")
    .arch("x86_64")
    .channel("stable")
    .deviceId("your-stable-device-id")
    .build());
if (result.isNoUpdate() || result.isNotModified()) {
    return; // HTTP 204 / 304
}
```

`official.kirizu.kirivers.client.Client` + `Config.builder()`. Pass an app-owned `deviceId`.

## 3. Download and verify

Download `packageUrl`, compare SHA-256, keep `exp`/`sig`.

## 4. Updater

`new Updater(client).update(upd)`. Without a Replacer and `targetPath`, bytes stay in `stageDir`.

## 5. Client methods

project, reportDevice, check, changelog, integrity, diff, pack / packUntilReady, download/head, catalogs, announcements, reportTelemetry, media, health. Not implemented: `/store/`, GET check, `clients/login`, `GET /ready`, manifest, pack/status, artifact filename URLs.

## 6. Config and auth

`baseUrl`, `projectRef`, `projectToken`, channel token, verify PEM. Do not log tokens.

## 7. Adapters

Transport: JDK `HttpClient` (inject on older Android; **do not add OkHttp**). JSON: Jackson (not an adapter; no Gson). FileStore nio+NFC. Hasher MessageDigest. SignatureVerifier JDK Ed25519+RSA. ArchiveUnpacker `java.util.zip`. Patcher **interface only**. Replacer **interface only**, optional `FilesMoveReplacer`.

## 8. Capabilities

Transport → `full_package`; zip → `patch_package`; writable FileStore → `file_list`; Patcher.supportedAlgos → `binary_delta`.

## 9. Patcher

No JNI / no hpatchz. Advertise algos only after inject. Never cross-decode magics.

## 10. Replacer

Desktop may use `FilesMoveReplacer`. Busy Windows exe may still need an app helper. Android/HarmonyOS: `PackageInstaller`. This artifact does not install IPAs.

## 11. Errors

`ApiException.code()` reads `error.code`. 204/304 are success.

## 12. Transport / tests

Inject Transport. Contract tests load packaged `openapi.client.json` without Docker.

## 13. Language notes

Jackson is fixed. Android injects Transport. `FilesMoveReplacer` is optional. No OkHttp/Gson.
