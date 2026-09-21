---
title: "Kotlin SDK"
description: "official.kirizu:kirivers-client-kotlin. CheckResult sealed class. No default Replacer. Not a Java wrapper."
---

# Kotlin

Separate artifact `official.kirizu:kirivers-client-kotlin`, **not** a wrapper around the Java SDK. JSON: kotlinx.serialization. No Jackson/Gson/OkHttp/Ktor Client.

## 1. Install

Gradle `implementation("official.kirizu:kirivers-client-kotlin:REPLACE_WITH_RELEASE")` (JDK 17).

::: warning Not published yet
```text
git clone -b sdk/kotlin https://github.com/Kirizu-Official/KiriVers.git kirivers-client-kotlin
./gradlew publishToMavenLocal
```
:::

## 2. Quick start

```kotlin
val client = Client(
    Config(baseUrl = "http://127.0.0.1:8080", projectRef = "my-app"),
)
when (val result = client.check(CheckRequest(
    currentVersion = "1.0.0", os = "windows", arch = "x86_64",
    channel = "stable", deviceId = appDeviceId,
))) {
    is CheckResult.UpToDate -> { }      // 204
    is CheckResult.NotModified -> { }   // 304
    is CheckResult.Update -> { }
}
```

## 3. Download and verify

`client.download(result.update.packageUrl)` + `JdkHasher().sha256Hex`.

## 4. Updater

`Updater(client, fileStore = JdkFileStore(stagingDir)).run(...)`. No default Replacer: staging only.

## 5. Client methods

`project()`, `reportDevice()`, `check()`, `changelog()`, `integrity()`, `diff()`, `pack()` / `packUntilReady()`, download/head, catalogs, `announcements()`, `reportTelemetry()`, media, `health()`. Not implemented: `/store/`, GET check, `clients/login`, `GET /ready`, manifest, pack/status, artifact filename URLs.

## 6. Config and auth

`ClientConfig(baseUrl, projectRef, projectToken, channelToken)`. A wrong channel token is not 403 on check.

## 7. Adapters

Transport: `JdkHttpTransport`. JSON: kotlinx.serialization (not an adapter). Hasher/FileStore/JdkZipUnpacker/JdkSignatureVerifier. Patcher and Replacer **interfaces only**.

## 8. Capabilities

Default `full_package`. Writable FileStore → `file_list`; ArchiveUnpacker → `patch_package`; Patcher → `binary_delta`.

## 9. Patcher

No JNI. Same magic rules as concepts.

## 10. Replacer

No default. Windows `MoveFileEx` and APK install are caller-owned.

## 11. Errors

Read envelope `code`. 204/304 are sealed variants, not failures.

## 12. Transport / tests

Inject Transport. No Docker.

## 13. Language notes

Not a Java wrapper. kotlinx.serialization. No default Replacer.
