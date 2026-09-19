# KiriVers C++ Client SDK

Official C++17 client for the KiriVers **native JSON** plane. Clone this orphan
branch and use it as the package root (there is no language-specific registry):

```text
git clone -b sdk/cpp https://github.com/Kirizu-Official/KiriVers.git kirivers-client-cpp
```

This SDK does **not** one-click install a running application. Hosted builds
can optionally use `MoveFileExW` on Windows if you attach a `Replacer` (immediate
replace only; a locked destination fails instead of scheduling a reboot).
Without a `Replacer`, `Updater` downloads and verifies to `stage_path` and
returns. When `ArchiveUnpacker` and `FileStore` are present and `install_dir`
is set, a verified `patch_package` zip is also unpacked into `install_dir`.

Docker is **not** an SDK runtime or publish dependency. `docker/Dockerfile.build`
is only a one-off compile image for machines without CMake/libcurl.

OpenAPI snapshot: `openapi.client.json` (`OPENAPI_REVISION`).

## Hosted vs embedded

| Preset | CMake | JSON | HTTP | SHA-256 / sign | Zip | NFC | Patcher | Replacer |
|--------|-------|------|------|----------------|-----|-----|---------|----------|
| **Hosted (default)** | `-DKIRIVERS_EMBEDDED=OFF` | nlohmann/json (vendored) | libcurl | OpenSSL | libzip | utf8proc | interface | interface; optional `MoveFileExReplacer` on Windows |
| **Embedded** | `-DKIRIVERS_EMBEDDED=ON` | nlohmann/json only | inject `Transport` | inject `Hasher` / `SignatureVerifier` | inject `ArchiveUnpacker` | inject via `FileStore` | interface | interface |

Embedded **turns off** libcurl, OpenSSL, libzip, and utf8proc. You must inject
at least `Transport`. Inject `Hasher` to verify downloads, `ArchiveUnpacker` to
advertise `patch_package`, `FileStore` (with `can_write_individual_files()`)
to advertise `file_list`, and `Patcher` to advertise `binary_delta` plus
`accepted_delta_algos`.

Do not add cpp-httplib, cpr, RapidJSON, or a second HTTP/JSON stack.

### Hosted packages (Debian/Ubuntu)

```text
sudo apt-get install cmake g++ pkg-config libcurl4-openssl-dev libssl-dev libzip-dev libutf8proc-dev
```

### Hosted packages (Windows / vcpkg)

```text
vcpkg install curl openssl libzip utf8proc
cmake -S . -B build -DCMAKE_TOOLCHAIN_FILE=%VCPKG_ROOT%/scripts/buildsystems/vcpkg.cmake
```

### Build

```text
cmake -S . -B build
cmake --build build
ctest --test-dir build --output-on-failure
```

Embedded:

```text
cmake -S . -B build-embed -DKIRIVERS_EMBEDDED=ON
cmake --build build-embed
ctest --test-dir build-embed --output-on-failure
```

Add the library from another CMake project:

```cmake
add_subdirectory(kirivers-client-cpp)
target_link_libraries(my_app PRIVATE kirivers::client)
```

## Example

```cpp
#include <kirivers/kirivers.hpp>

int main() {
  kirivers::Config cfg;
  cfg.base_url = "http://127.0.0.1:8080";  // no trailing slash
  cfg.project_ref = "sdk-fixture";
  // Hosted fills CurlTransport, OpenSslHasher, FilesystemStore, LibzipUnpacker.
  kirivers::Client client(cfg);

  kirivers::CheckInput in;
  in.current_version = "1.0.0";
  in.os = "windows";
  in.arch = "x86_64";
  in.channel = "stable";
  in.device_id = "your-stable-device-id";  // caller-owned; SDK never invents one
  auto result = client.check(in);
  if (result.status != kirivers::CheckStatus::Update) return 0;

  kirivers::Updater updater(client);
  kirivers::UpdateRequest req;
  req.current_version = in.current_version;
  req.os = in.os;
  req.arch = in.arch;
  req.channel = *in.channel;
  req.device_id = in.device_id;
  req.stage_path = "update.bin";
  auto staged = updater.run(req);  // verified bytes; no Replacer => not installed
  (void)staged;
}
```

Override HTTP or SHA-256 (tests, embedded, or a custom stack):

```cpp
cfg.hosted_defaults = false;
cfg.transport = std::make_shared<kirivers::FunctionTransport>(my_http);
cfg.hasher = std::make_shared<kirivers::OpenSslHasher>();  // hosted only
```

Windows replace (optional; not attached by default):

```cpp
#ifdef _WIN32
cfg.replacer = std::make_shared<kirivers::MoveFileExReplacer>();
#endif
```

## Adapter table (D18)

| Adapter | Hosted default | Caller must implement to enable |
|---------|----------------|----------------------------------|
| `Transport` | libcurl | Always required in embedded; injectable in hosted |
| `Hasher` | OpenSSL SHA-256/MD5 | Verify downloads in embedded |
| `SignatureVerifier` | OpenSSL Ed25519 + RSA-SHA256 | Check `signature` over `integer\\nsemver\\nroot_hash\\npackage_url\\nsize\\nsha256` |
| `FileStore` | `std::filesystem` + utf8proc NFC | Integrity compare / `file_list` |
| `ArchiveUnpacker` | libzip | `patch_package` |
| `Patcher` | **none** | `binary_delta` + `accepted_delta_algos` (hdiffpatch / bsdiff / xdelta3). Unknown magic is not cross-decoded; updater falls back to the full package. |
| `Replacer` | **none** (optional `MoveFileExW`; fails if the destination is locked) | Apply/install. Android/HarmonyOS APK install is always caller-owned. |

## Capability matrix (D13)

Default check body is derived from **live** adapters. Empty claims are never sent.

| Live adapter | Check field |
|--------------|-------------|
| (always, if Client can talk HTTP) | `capabilities: ["full_package"]` |
| `ArchiveUnpacker` | `patch_package` |
| `FileStore` with `can_write_individual_files()` | `file_list` |
| `Patcher` with non-empty `supported_algos()` | `binary_delta` and those names in `accepted_delta_algos` |

`local_sha256` is never sent on check. It is only used on `POST /update/diff`.

## Native JSON API

`Client` wraps every native path (not `/store/...`, not leftover `GET /update/check` or `POST /clients/login`):

Project, DeviceReport, Check (200/204/304), Changelog, Integrity, Diff, Pack
(poll the same POST), Download GET+HEAD (`Range`, keep `exp`/`sig`), Channels,
Matrix, Languages, Announcements, Telemetry (202, must not block apply),
optional Media and Health.

Identity (`device_id`, channel, os/arch, version) is always caller-supplied.
Plaintext `device_id` is never written to SDK logs.

## Tests

Contract and adapter tests use a fake `Transport` and **do not** start Docker.

Integration (`kirivers_integration_test`) talks to a **host** client plane
(typically `http://127.0.0.1:8080`) using `configs/sdk-fixture.json`. Set
`KIRIVERS_CLIENT_BASE_URL` and `KIRIVERS_FIXTURE` when the fixture is not at
the default path. Use a unique `device_id` (`sdk-cpp-<random>`).
