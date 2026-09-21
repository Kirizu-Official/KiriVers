---
title: "Rust SDK"
description: "crates.io kirivers-client. CheckOutcome. Blocking reqwest+rustls. No tokio required."
---

# Rust

Blocking API (`reqwest` + `rustls-tls`). No tokio required. Do not add hyper/ureq.

## 1. Install

```toml
[dependencies]
kirivers-client = "REPLACE_WITH_RELEASE"
```

::: warning Not on crates.io yet
Clone `-b sdk/rust` and use a path dependency.
:::

## 2. Quick start

```rust
let client = Client::new(Config::new("http://127.0.0.1:8080", "my-app"));
match client.check(&CheckRequest {
    current_version: "1.0.0".into(),
    os: "linux".into(),
    arch: "x86_64".into(),
    channel: Some("stable".into()),
    device_id: Some("your-stable-device-id".into()),
    ..Default::default()
})? {
    CheckOutcome::NoUpdate | CheckOutcome::NotModified => {}
    CheckOutcome::Update(body) => println!("{}", body.package_url),
}
```

Blocking API; tokio is not required. Pass a caller-owned `device_id`.

## 3. Download and verify

`client.download(&body.package_url, None)` vs `body.sha256`.

## 4. Updater

check → download/delta or pack → SHA-256 → optional replace. No Replacer → stage only.

## 5. Client methods

project, report, check, changelog, integrity, diff, pack (`PackPollOptions`), packages, catalogs, announcements, telemetry, media, health. Not implemented: `/store/`, GET check, `clients/login`, `GET /ready`, manifest, pack/status, artifact filename.

## 6. Config and auth

`Config::new`, project/channel tokens, verify PEM.

## 7. Adapters

Transport reqwest blocking+rustls. JSON serde_json (not an adapter). Hasher sha2. FileStore std::fs + NFC. zip crate. ed25519-dalek + rsa. Replacer `std::fs::rename`. No default Patcher.

## 8. Capabilities

Empty capabilities field sends `full_package` only. Updater expands from live adapters.

## 9. Patcher

`binary_delta` only after inject. Do not cross-decode magics.

## 10. Replacer

Default rename cannot handle busy files or APK. Missing Replacer is not a failure.

## 11. Errors

`kirivers_client::Error` reads `code`. 204/304 are Outcome variants.

## 12. Transport / tests

Inject Transport. `cargo test` without Docker.

## 13. Language notes

Blocking reqwest+rustls; no tokio; do not add hyper/ureq.
