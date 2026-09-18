# KiriVers Kotlin client SDK

Official Kotlin client for the KiriVers **native JSON** client plane (`openapi.client.json`).

This is a **separate implementation** from the Java SDK (`official.kirizu:kirivers-client`). It is not a Kotlin wrapper around the Java artifact, does not depend on that JAR, and does not use Jackson, Gson, OkHttp, or Ktor Client.

| | Kotlin SDK | Java SDK |
|---|---|---|
| Maven coordinate | `official.kirizu:kirivers-client-kotlin` | `official.kirizu:kirivers-client` |
| JSON | `kotlinx-serialization-json` only | Jackson |
| HTTP | JDK 17 `HttpClient` | JDK `HttpClient` |
| Source | independent Kotlin, branch `sdk/kotlin` | independent Java, branch `sdk/java` |

Package identity is this repository's `sdk/kotlin` orphan branch (Maven Central, not a second GitHub repo).

## Requirements

- JDK 17 or newer
- Kotlin 2.1+ (stdlib is a runtime dependency)

Runtime third-party libraries: **kotlin-stdlib** and **kotlinx-serialization-json**. Nothing else.

## Install

Gradle (Kotlin DSL):

```kotlin
dependencies {
    implementation("official.kirizu:kirivers-client-kotlin:0.1.0")
}
```

Maven:

```xml
<dependency>
  <groupId>official.kirizu</groupId>
  <artifactId>kirivers-client-kotlin</artifactId>
  <version>0.1.0</version>
</dependency>
```

Until the artifact is on Maven Central, depend on this branch:

```text
git clone -b sdk/kotlin https://github.com/Kirizu-Official/KiriVers.git kirivers-client-kotlin
```

then `./gradlew publishToMavenLocal`.

## Quick start

The SDK never invents a `device_id`. Pass identity from the app. Do not log the raw device id.

```kotlin
import official.kirizu.kirivers.client.*

val client = Client(
    ClientConfig(
        baseUrl = "https://client.example.com",
        projectRef = "my-app",
        projectToken = null,      // Authorization: Bearer / X-Project-Token
        channelToken = null,      // X-Channel-Token; check never 403s on mismatch
    ),
)

when (val result = client.check(CheckRequest(
    currentVersion = "1.0.0",
    os = "windows",
    arch = "x86_64",
    channel = "stable",
    deviceId = appDeviceId,
))) {
    is CheckResult.UpToDate -> { /* HTTP 204 */ }
    is CheckResult.NotModified -> { /* ETag 304 */ }
    is CheckResult.Update -> {
        val bytes = client.download(result.update.packageUrl).body
        val hex = JdkHasher().sha256Hex(bytes)
        check(hex.equals(result.update.sha256, ignoreCase = true))
    }
}
```

Typed JSON coverage: `project()`, `reportDevice()`, `check()`, `changelog()`, `integrity()`, `diff()`, `pack()` / `packUntilReady()`, `download` / `head` (packages, keep `exp`/`sig`, `Range`), `channels()`, `matrix()`, `languages()`, `announcements()`, `reportTelemetry()`, `downloadMedia()` / `headMedia()`, `health()`.

Store feeds, leftover `GET /update/check`, `POST /clients/login`, `GET .../manifest`, `POST /update/pack/status`, and admin routes are **not** implemented.

## High-level updater

`Updater` orchestrates check → download / diff / pack → SHA-256 → optional patch/zip → optional replace. Missing `Replacer` is **not** a failure: you get verified staged bytes.

This SDK has **no default Replacer**. It does **not** one-click install an APK, replace a locked Windows executable, or relaunch the app. Supply your own `Replacer` if you want apply.

```kotlin
val outcome = Updater(
    client = client,
    fileStore = JdkFileStore(stagingDir),
    // patcher = myBsdiffPatcher,   // required before check advertises binary_delta
    // replacer = MyInstaller(),    // required before this is an "install"
).run(
    UpdateIdentity(
        currentVersion = "1.0.0",
        os = "windows",
        arch = "x86_64",
        channel = "stable",
        deviceId = appDeviceId,
    ),
)
```

## Adapters

| Adapter | Default | Notes |
|---------|---------|--------|
| `Transport` | JDK `HttpClient` (`JdkHttpTransport`) | Injectable. No OkHttp / Ktor. |
| JSON | `kotlinx-serialization-json` | Not an adapter. Do not add Jackson/Gson. |
| `Hasher` | `JdkHasher` (SHA-256, MD5) | MD5 only when integrity asks. |
| `FileStore` | `JdkFileStore` when you pass a root | `/` + Unicode NFC; rejects `..`. Enables integrity compare and `file_list`. |
| `ArchiveUnpacker` | `JdkZipUnpacker` (`java.util.zip`) | Zip members are content hashes mapped to `files[].path`. Enables `patch_package`. |
| `SignatureVerifier` | `JdkSignatureVerifier` | Ed25519 and RSA-SHA256 over `integer\\nsemver\\nroot_hash\\npackage_url\\nsize\\nsha256`. |
| `Patcher` | **interface only** | No JNI, no bundled `hpatchz`. Inject one that can apply `bsdiff` / `xdelta3` / `hdiffpatch`. Unknown magic must fail; never cross-decode (`KVDIFFHP1`, `HDIFF13&`, `BSDIFF40`, VCDIFF `D6 C3 C4`). |
| `Replacer` | **interface only** | Windows `MoveFileEx` / APK sideload / HarmonyOS install are **your** code. |

NFC uses `java.text.Normalizer`. Zip uses `java.util.zip`. Signatures use `java.security`.

## Capability matrix (D13)

Check always includes `full_package` unless you pass an explicit non-empty `capabilities` list on `CheckRequest`.

`Updater.capabilities()` adds more **only** when the matching adapter is present:

| Adapter present | Extra check fields |
|-----------------|--------------------|
| (always) | `capabilities: ["full_package"]` |
| `FileStore.canWriteIndividualFiles()` | `file_list` |
| `ArchiveUnpacker` | `patch_package` |
| `Patcher.supportedAlgos()` non-empty | `binary_delta` and those names in `accepted_delta_algos` |

Never send `local_sha256` on check (only on `POST /update/diff`). Empty capability lists are not sent. Downgrade targets do not apply binary delta.

Pack polling POSTs the **same JSON** to `/update/pack` (backoff 1s, cap 15s). `status=full_package` downloads the check full URL. Do not call admin jobs.

## Platforms

| Runtime | Notes |
|---------|--------|
| Desktop JVM 17+ | Default JDK `HttpClient` + nio `FileStore`. Occupied Windows executables need a `Replacer` the app owns (`MoveFileEx` / helper BAT). |
| macOS / Linux desktop | Atomic rename is your `Replacer`. Document permission and code-sign limits in the app. |
| Android | Same Maven coordinate. Inject `Transport` if `java.net.http` is missing. APK install is your `PackageInstaller` `Replacer`. No JNI delta `.so`. |
| HarmonyOS | Same as Android: caller `Replacer`. |

This SDK does **not** one-click install on any of those platforms.

## OpenAPI snapshot

- `openapi.client.json` — client plane contract copied from the server tree
- `OPENAPI_REVISION` — `1.0.0 B443DEA6` (`info.version` + short SHA-256 of the snapshot)

Contract tests load this file and assert path + method + required field names + the `{error:{code,message,details}}` envelope. They use a mock `Transport` and do not start Docker.

## Build / test

```text
./gradlew test
```

On Windows: `gradlew.bat test`.

Integration tests talk to `http://127.0.0.1:8080` using `D:\KiriVers\configs\sdk-fixture.json` (check `1.0.0` → download `1.1.0`, SHA-256 `7f063a6901ebfd0aa7b1adb8af58f4e792f13310f42213767ced623eb0669969`). Each run uses a unique `device_id` (`sdk-kotlin-<random>`). Docker is not an SDK dependency.

## Publish

See `docs/sdk-publish.md` on the server default branch. Coordinate: `official.kirizu:kirivers-client-kotlin`. Git tags on this repo: `sdk-kotlin-v<semver>`.
