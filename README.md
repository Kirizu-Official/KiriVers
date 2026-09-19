# kirivers-client

Official **KiriVers** native JSON client SDK for Rust (crates.io `kirivers-client`). It talks to the client plane described by `openapi.client.json` in this package root. There is no OpenAPI codegen.

Clone this tree from the KiriVers repository:

```bash
git clone -b sdk/rust https://github.com/Kirizu-Official/KiriVers.git kirivers-client
```

Publish (maintainers): from this branch, `cargo publish`. Git tags on the server repo use `sdk-rust-v<semver>` so they do not collide with server `v*` tags. crates.io does **not** need a separate GitHub repository.

This crate is a **blocking** API (`reqwest` + `rustls-tls`). Callers do not need tokio. Do not add hyper or ureq as a second HTTP stack.

OpenAPI snapshot: `openapi.client.json` (`OPENAPI_REVISION`).

## Install

```toml
[dependencies]
kirivers-client = "0.1"
```

## Example

```rust
use kirivers_client::{CheckRequest, Client, Config};

fn main() -> Result<(), kirivers_client::Error> {
    let client = Client::new(Config::new("http://127.0.0.1:8080", "my-project"))?;
    let outcome = client.check(&CheckRequest {
        current_version: "1.0.0".into(),
        os: "windows".into(),
        arch: "x86_64".into(),
        channel: Some("stable".into()),
        device_id: Some(my_stable_device_id()), // caller-owned; SDK never generates one
        ..CheckRequest::default()
    })?;
    match outcome {
        kirivers_client::CheckOutcome::Update { body, .. } => {
            let bytes = client.download(&body.package_url, None)?;
            println!("got {} bytes, sha256 catalog {}", bytes.body.len(), body.sha256);
        }
        kirivers_client::CheckOutcome::NoUpdate { .. } => println!("up to date"),
        kirivers_client::CheckOutcome::NotModified { .. } => println!("etag hit"),
    }
    Ok(())
}

fn my_stable_device_id() -> String {
    "app-assigned-id".into()
}
```

`device_id`, channel, `custom`, os/arch, and the current version are always caller-supplied. The SDK does not log plaintext `device_id`.

## Native JSON surface

Implemented (store feeds and leftover routes are **not** wrapped):

| Method | Path |
|--------|------|
| GET | `/api/v1/projects/{ref}` |
| POST | `.../clients/report` |
| POST | `.../update/check` |
| GET | `.../changelog/{channel}/{os}/{arch}` |
| GET | `.../versions/{version}/integrity` |
| POST | `.../update/diff` |
| POST | `.../update/pack` (same URL to enqueue and poll; default wait 1s, doubles to 15s, 120s deadline via `PackPollOptions`) |
| GET/HEAD | `.../packages/{sha256}` (`Range`, keep `exp`/`sig`) |
| POST | `.../telemetry/report` (202; Update ignores failures) |
| GET | `.../channels`, `.../matrix`, `.../languages` |
| GET | `.../announcements` |
| GET/HEAD | `.../media/{id}` |
| GET | `/api/v1/health` |

Check HTTP 204 is “no update”, not an error. HTTP 304 is an ETag hit. Errors parse `{ "error": { "code", "message", "details" } }`.

## Adapters (D18)

| Adapter | Default in this crate | Inject to change |
|---------|----------------------|------------------|
| Transport | `reqwest` (`blocking` + `rustls-tls`) | always injectable |
| JSON | `serde` + `serde_json` (not an adapter) | — |
| Hasher | `sha2` (MD5 via `md-5`) | injectable |
| FileStore | `std::fs` + NFC (`unicode-normalization`) | injectable |
| ArchiveUnpacker | `zip` crate | injectable |
| SignatureVerifier | `ed25519-dalek` + `rsa` (RSA-SHA256) | injectable |
| Replacer | `std::fs::rename` | injectable; **required** for locked files / APK / HarmonyOS |
| Patcher | **none** (interface only) | **required** to enable `binary_delta` |

Runtime crates on crates.io must stay on that list (plus their transitive dependencies). Do not add hyper, ureq, serde_yaml, or a second zip/crypto stack.

### Capability matrix (D13)

`Client::check` with an empty `capabilities` field sends `["full_package"]` only.

`Updater` fills check `capabilities` / `accepted_delta_algos` from **live** adapters:

| Adapter present | Check also sends |
|-----------------|------------------|
| Transport | `full_package` |
| ArchiveUnpacker (default zip) | `patch_package` |
| FileStore that can write files (default `std::fs`) and `UpdateRequest.install_dir` is set | `file_list` |
| Patcher with non-empty `supported_algos()` | `binary_delta` + those algo names |

Without an injected `Patcher`, the SDK **never** reports `binary_delta`. Unknown delta magic (`KVDIFFHP1\n`, `HDIFF13&`, `BSDIFF40`, VCDIFF `D6 C3 C4`) is not cross-decoded; Update falls back to the full package.

## Replacer / apply

`Updater` with the default `RenameReplacer` can rename a **unlocked** staged file onto `dest`. That is **not** a one-click installer:

- A running Windows/macOS/Linux binary that holds the destination open needs a caller `Replacer` (or a helper the app owns).
- Android / HarmonyOS APK install is always a caller `Replacer`.
- Missing `Replacer` is not a failure: Update still downloads and SHA-256-verifies to `stage_dir`.

## Tests

```bash
cargo test
```

Contract tests mock `Transport` and do not start Docker. Integration tests (when `http://127.0.0.1:8080` is the live client plane) use `configs/sdk-fixture.json` from a local KiriVers checkout.

Docker Compose is **not** a dependency of this crate. It only runs Postgres/Redis for a host KiriVers server.
