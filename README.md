# kirivers_client

Official **KiriVers** native JSON client SDK for **Dart 3**. It talks to the
client plane (`openapi.client.json`) — check, download, integrity, pack polling,
telemetry, catalogs — and optionally orchestrates an update.

This is **not** a store-feed client (Sparkle / WinGet / electron-updater / Play /
App Store). Browser / `dart:html` is unsupported. The Flutter SDK is **not** a
dependency; Flutter **apps** (VM, Android, iOS, desktop) can use this package.
Web builds cannot: the default file adapters use `dart:io`.

Package coordinate: **pub.dev `kirivers_client`**. Source lives on the
`sdk/dart` branch of [Kirizu-Official/KiriVers](https://github.com/Kirizu-Official/KiriVers)
(package root = repository root on that branch). Tags look like `sdk-dart-v0.1.0`.

OpenAPI snapshot: `openapi.client.json` (info.version `1.0.0`, digest prefix
`B443DEA6`). See `OPENAPI_REVISION`.

## Install

```yaml
dependencies:
  kirivers_client: ^0.1.0
```

```text
dart pub add kirivers_client
```

## Quick start

```dart
import 'package:kirivers_client/kirivers_client.dart';

final client = Client(
  config: ClientConfig(
    baseUrl: 'https://client.example.com',
    projectRef: 'my-project',
    // projectToken: '...',      // Authorization + X-Project-Token
    // channelToken: '...',      // X-Channel-Token (never logged)
    // signingPublicKeyPem: pem, // when responses include signature
  ),
);

final check = await client.check(
  CheckRequest(
    currentVersion: '1.0.0',
    os: 'windows',
    arch: 'x86_64',
    channel: 'stable',
    deviceId: myDeviceId, // caller-supplied; SDK never invents one
  ),
);

if (check.isNoUpdate || check.isNotModified) {
  return;
}

final bytes = await client.downloadUrl(check.body!.packageUrl);
final sha = const CryptoHasher().sha256Hex(bytes.bytes);
```

High-level orchestration (download + SHA-256 verify; replace only if you keep
the default `FileRenameReplacer` **and** pass `installPath`):

```dart
final updater = Updater(client: client);
final result = await updater.run(
  UpdatePlan(
    currentVersion: '1.0.0',
    os: 'windows',
    arch: 'x86_64',
    channel: 'stable',
    deviceId: myDeviceId,
    apply: true,
    installPath: r'C:\App\app.bin',
    stageDir: r'C:\App\stage',
  ),
);
```

Default `Replacer` is **`File.rename`** for app-level files (copy+delete if the
rename crosses volumes). That is **not** one-click install: Android / HarmonyOS
APK sideload, Windows lock-file helpers, and installer relaunch are caller
`Replacer` implementations.

## Native JSON API (`Client`)

| Method | HTTP |
|--------|------|
| `project` | `GET /api/v1/projects/{ref}` |
| `deviceReport` | `POST .../clients/report` |
| `check` | `POST .../update/check` (200 / 204 / 304) |
| `changelog` | `GET .../changelog/{channel}/{os}/{arch}` |
| `integrity` | `GET .../versions/{version}/integrity` |
| `diff` | `POST .../update/diff` |
| `pack` / `packUntilReady` | `POST .../update/pack` (poll the same JSON) |
| `downloadPackage` / `headPackage` / `downloadUrl` | `GET`/`HEAD .../packages/{sha256}` |
| `channels` / `matrix` / `languages` | public catalogs |
| `announcements` | `GET .../announcements` |
| `reportTelemetry` | `POST .../telemetry/report` (202) |
| `media` / `headMedia` | `GET`/`HEAD .../media/{id}` |
| `health` | `GET /api/v1/health` (`ready` is DB + storage) |

Not implemented: `/store/...`, `GET /update/check`, `POST /clients/login`,
`GET .../manifest`, `POST /update/pack/status`, `GET /channels/{slug}`,
`/artifacts/{id}/{filename}`, `GET /ready`.

Private downloads keep `?exp=&sig=` on `package_url`. `Range` is supported.
HTTP 204 check is “no update”, not an error. 304 is an ETag hit.
Errors parse `{ "error": { "code", "message", "details" } }`.

`device_id` is always caller-supplied and is never written to SDK logs.

## Adapters (D18) and capabilities (D13)

| Adapter | Default in this SDK | Caller must implement to… |
|---------|---------------------|---------------------------|
| **Transport** | `package:http` | Use a custom HTTP stack (tests inject a fake) |
| **JSON** | `dart:convert` (not an adapter) | — |
| **Hasher** | `package:crypto` (SHA-256 + MD5) | Swap hash implementation |
| **SignatureVerifier** | `package:cryptography` (Ed25519, RSA-SHA256) | Swap verify; payload is `integer\\nsemver\\nroot_hash\\npackage_url\\nsize\\nsha256` |
| **FileStore** | `dart:io` + `unorm_dart` NFC (`IoFileStore`) | Integrity compare / `file_list` on a custom tree |
| **ArchiveUnpacker** | `package:archive` zip (`ZipArchiveUnpacker`) | Advertise / apply `patch_package` with another unzip |
| **Replacer** | `File.rename` (`FileRenameReplacer`) | APK install, occupied-file replace, flash |
| **Patcher** | **interface only** | Advertise `binary_delta` and apply `hdiffpatch` / `bsdiff` / `xdelta3` |

Hosted runtime dependencies are exactly: `http`, `crypto`, `archive`,
`cryptography`, `unorm_dart`. No second HTTP or JSON stack. No `dart:ffi`
delta engine, no bundled `hpatchz`.

Check `capabilities` follow **attached** adapters:

| Present | Sent on check |
|---------|----------------|
| Transport | `full_package` |
| + ArchiveUnpacker | `patch_package` |
| + FileStore that can write files | `file_list` |
| + Patcher with `supportedAlgos` | `binary_delta` + `accepted_delta_algos` |

Without a `Patcher`, this SDK **does not** send `binary_delta`. Injecting a
Patcher that claims `bsdiff` / `xdelta3` / `hdiffpatch` is what turns that on.
Unknown delta magic (`KVDIFFHP1`, `HDIFF13&`, `BSDIFF40`, VCDIFF `D6 C3 C4`)
fails the patch and the updater falls back to the full package — it never
cross-decodes.

Downgrade targets never request binary delta.

Pack: `needed_paths` are unique NFC `/` paths; local `KEEP_IF_EXISTS` files are
omitted; HTTP 202 / `status=pending` is the same POST again (backoff 1s, cap
15s). `full_package` means download the check `package_url`. Do not call admin
jobs.

## Platforms

| Runtime | Supported | Apply |
|---------|-----------|--------|
| Dart VM / Flutter desktop | Yes | Default `File.rename` |
| Flutter Android / iOS / HarmonyOS | Check + download + verify | Inject `Replacer` (and `Patcher` if you have JNI) |
| Browser / `dart:html` | **No** | — |

## Tests

Contract tests use a fake `Transport` (no Docker):

```text
dart test
```

Live integration (host client plane on `:8080`, fixture
`D:\KiriVers\configs\sdk-fixture.json`): check `1.0.0` → download `1.1.0` →
SHA-256 `7f063a6901ebfd0aa7b1adb8af58f4e792f13310f42213767ced623eb0669969`.
Each run uses a unique `device_id` (`sdk-dart-<random>`).

Docker Compose is only for the KiriVers **server** database/cache. This package
does not depend on Docker.

## Publish (pub.dev)

From this package root (`sdk/dart` branch):

```text
dart pub publish
```

Do not publish from the KiriVers default branch (that tree is the server).
Git tags on this repo for Dart releases are `sdk-dart-v<semver>`.
