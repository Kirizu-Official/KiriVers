---
title: "SDK concepts"
description: "Native JSON flow, capabilities, adapters, delta magics, non-goals."
---

# SDK concepts

Cross-language reference. Language details live on each language page.

## Scope

Native client JSON: check, download, verify, optional delta/pack, optional replace.

Non-goals: admin-plane SDK; Sparkle/WinGet/electron-updater/Play/App Store; OpenAPI-generated clients; browsers and Deno (TypeScript has no browser export).

## Default flow

1. Caller identity parameters including a stable `device_id`. The SDK does not mint identity and does not log plaintext device ids.
2. Optional device report.
3. POST update check (default capability full package only).
4. Optional changelog (check has no body text).
5. Download `/packages/{sha256}` (keep `exp`/`sig`, support Range).
6. SHA-256; if `signature` is present, verify over the check payload.
7. Replace only with a Replacer; otherwise return the verified staging path.

## Check and HTTP

Check is **POST**; GET is 404. 204 (no update) and 304 (ETag) are not failures. Errors use `{ "error": { "code", "message", "details" } }`.

## Capabilities

Advertise only live adapters: `full_package` / `file_list` / `patch_package` / `binary_delta` plus `accepted_delta_algos`. Do not send `binary_delta` without a Patcher. `local_sha256` is diff-only.

## Adapters

HTTP, files, hashing, zip, and signature verify are injectable. JSON is **not** an adapter (Jackson, kotlinx.serialization, serde_json, cJSON, nlohmann/json, …). Do not add a second HTTP/JSON stack (axios, OkHttp, Guzzle, Gson, RapidJSON, …). Delta apply and in-use/APK install have no default: Patcher / Replacer only (Go may ship an optional pure-Go Patcher).

## Delta magics

Do not cross-decode: `KVDIFFHP1`, `HDIFF13&`, `BSDIFF40`, VCDIFF `D6 C3 C4`. Unknown magic falls back to the full package. See [FAQ · Delta](/en/faq/delta).

## Docker

SDK packages, install steps, and caller apps do not depend on Docker. Contract tests inject a fake Transport. Swift `swift test` needs an Apple toolchain.

## Non-goals

Admin-plane SDK; store-feed clients; OpenAPI Generator / hey-api clients; browser / Deno updaters; treating this server repository’s `main` as the Go/PHP/Swift package root.
