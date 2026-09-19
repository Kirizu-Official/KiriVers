# KiriVers Java client SDK

Official handwritten client for the KiriVers **native JSON client plane**. Coordinate: **`official.kirizu:kirivers-client`**. JDK **17**. Source lives on the `sdk/java` branch of [KiriVers](https://github.com/Kirizu-Official/KiriVers) (package root = repository root on that branch). Maven Central does **not** need a separate GitHub repository.

This package talks only to the native JSON API. It does **not** implement store feeds (Sparkle, WinGet, Play, App Store, electron-updater).

OpenAPI snapshot: [`openapi.client.json`](openapi.client.json) (`OPENAPI_REVISION`). HTTP is handwritten. There is no OpenAPI Generator output.

## Install

JDK 17+. Runtime dependency: **Jackson databind only** (plus its transitive Jackson core/annotations). No OkHttp, Gson, org.json, or JNI `.so`.

Maven:

```xml
<dependency>
  <groupId>official.kirizu</groupId>
  <artifactId>kirivers-client</artifactId>
  <version>0.1.0</version>
</dependency>
```

Gradle:

```kotlin
implementation("official.kirizu:kirivers-client:0.1.0")
```

Until the artifact is on Maven Central, depend on this branch:

```text
git clone -b sdk/java --single-branch https://github.com/Kirizu-Official/KiriVers.git kirivers-client-java
cd kirivers-client-java
./mvnw install
```

## Example

`device_id`, channel, os/arch, and the current version are always supplied by the caller. The SDK never invents a device identity and never writes a plaintext `device_id` to logs.

```java
import official.kirizu.kirivers.client.Client;
import official.kirizu.kirivers.client.Config;
import official.kirizu.kirivers.client.UpdateRequest;
import official.kirizu.kirivers.client.Updater;
import official.kirizu.kirivers.client.model.UpdateCheckRequest;

Client client = new Client(Config.builder()
    .baseUrl("https://updates.example.com")
    .projectRef("my-app")
    .projectToken("optional-project-token")
    .build());

UpdateCheckRequest check = new UpdateCheckRequest();
check.currentVersion = "1.0.0";
check.os = "windows";
check.arch = "x86_64";
check.channel = "stable";
check.deviceId = appOwnedDeviceId;
client.check(check);

UpdateRequest upd = new UpdateRequest();
upd.currentVersion = "1.0.0";
upd.os = "windows";
upd.arch = "x86_64";
upd.channel = "stable";
upd.deviceId = appOwnedDeviceId;
upd.stageDir = Path.of("stage");
var result = new Updater(client).update(upd);
// Without a Replacer this STAGES verified bytes. It does not install.
```

HTTP 204 on check means **no update** (not an error). HTTP 304 is an ETag hit.

## Native JSON API

| Method | Path |
|--------|------|
| GET | `/api/v1/projects/{project_ref}` |
| POST | `/api/v1/projects/{project_ref}/clients/report` |
| POST | `/api/v1/projects/{project_ref}/update/check` |
| GET | `/api/v1/projects/{project_ref}/changelog/{channel}/{os}/{arch}` |
| GET | `/api/v1/projects/{project_ref}/versions/{version}/integrity` |
| POST | `/api/v1/projects/{project_ref}/update/diff` |
| POST | `/api/v1/projects/{project_ref}/update/pack` (poll the same POST) |
| GET/HEAD | `/api/v1/projects/{project_ref}/packages/{ref}` (`Range`, keep `exp`/`sig`) |
| POST | `/api/v1/projects/{project_ref}/telemetry/report` (202; never blocks apply) |
| GET | `/api/v1/projects/{project_ref}/channels` / `matrix` / `languages` |
| GET | `/api/v1/projects/{project_ref}/announcements` |
| GET/HEAD | `/api/v1/projects/{project_ref}/media/{id}` |
| GET | `/api/v1/health` |

Not implemented: `/store/...`, `GET /update/check`, `POST /clients/login`, `GET .../manifest`, `POST /update/pack/status`, leftover artifact UUID paths.

Errors are `{ "error": { "code", "message", "details" } }` (`ApiException.code()`).

## Adapters (D18)

| Adapter | Default in this SDK | Inject when |
|---------|---------------------|-------------|
| **Transport** | JDK `java.net.http.HttpClient` | Android (no `java.net.http` on older APIs) or tests. **Do not add OkHttp as a second HTTP stack in this library.** |
| **JSON** | Jackson `jackson-databind` (not an adapter; D16) | — (no Gson / org.json) |
| **FileStore** | `java.nio.file` + NFC via `java.text.Normalizer` | Embedded / custom VFS |
| **Hasher** | `MessageDigest` SHA-256 / MD5 | Alternate providers |
| **SignatureVerifier** | JDK Ed25519 (`EdDSA`) + RSA-SHA256 | Alternate keys / providers |
| **ArchiveUnpacker** | `java.util.zip` | Non-zip patch archives |
| **Patcher** | **Interface only** (no JNI, no hpatchz) | You already have a delta engine |
| **Replacer** | **Interface only.** Optional helper: `FilesMoveReplacer` (`Files.move`) for desktop. **Android / HarmonyOS APK install is your `PackageInstaller` (or equivalent).** | You want apply after verify |

`Config` defaults include Transport, FileStore, Hasher, SignatureVerifier, and ArchiveUnpacker. **Patcher and Replacer are unset** until you inject them.

This SDK does **not** perform one-click install. `Updater` downloads and verifies to `stageDir`. It calls `Replacer` only when you set one **and** pass `targetPath`. Desktop apps may use `FilesMoveReplacer`. Android apps must implement `Replacer` themselves.

## Capability matrix (D13)

Check `capabilities` follow **live adapters**, never empty claims:

| Adapter present | Check sends |
|-----------------|-------------|
| Transport (always) | `full_package` |
| ArchiveUnpacker (default zip) | `patch_package` |
| FileStore that can write files (default nio) | `file_list` |
| Patcher with `supportedAlgos()` | `binary_delta` + those names in `accepted_delta_algos` |

Without a Patcher, check **does not** send `binary_delta` or `accepted_delta_algos` (a non-empty algo list would auto-grant delta on the server). `local_sha256` is sent only on `POST /update/diff`, never on check.

Unknown delta magic (`KVDIFFHP1\n` / `HDIFF13&` / `BSDIFF40` / VCDIFF `D6 C3 C4`) is refused. A Patcher that does not advertise the matching wire algo (`hdiffpatch` / `bsdiff` / `xdelta3`) is also refused. `Updater` falls back to the full package and never cross-decodes.

## Platforms

| Runtime | Notes |
|---------|--------|
| Desktop JVM 17+ | Default `HttpClient` + nio FileStore. Optional `FilesMoveReplacer`. Occupied Windows executables may still need a helper the app owns. |
| Android | Same Maven coordinate. Inject `Transport` if `java.net.http` is missing. Install only via your `Replacer`. No JNI delta `.so`. |
| HarmonyOS | Same as Android: caller `Replacer`. |

## Pack polling

`Client.packUntilReady` POSTs the **same JSON** again after HTTP 202 or `status=pending`. Default backoff 1s, cap 15s, deadline 2 minutes (all configurable on `Config`). It never calls admin Job APIs.

## Publishing

Package identity is Maven Central `official.kirizu:kirivers-client`. Git tags on this server repo should be `sdk-java-v<semver>` (not bare `v0.1.0`, which collides with the Go server). This task does not upload to OSSRH.

```text
./mvnw -DskipTests deploy
```

(Configure `distributionManagement` / OSSRH credentials in the publishing environment, not in this tree.)

## Tests

Contract tests use a mock `Transport` (no Docker). Integration tests expect a **already running** client plane at `http://127.0.0.1:8080` and `configs/sdk-fixture.json` (unique `device_id` `sdk-java-<uuid>`). Tests do not start or stop that process.

```text
./mvnw test
```
