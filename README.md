# @kirizu/kirivers-client

Official **KiriVers native JSON** client SDK. TypeScript source, one typed npm package.

**Browsers are not supported.** There is no `browser` export, no Deno target, and no renderer-only Electron build. Use **Node.js 20+** or **Electron main / app-level** Node. Do not bundle this package for a web page.

This is not an electron-updater / Sparkle / WinGet client. It speaks only the native client-plane JSON API.

## Install

```bash
npm install @kirizu/kirivers-client
```

Requires Node.js 20+ (`globalThis.fetch`, `node:crypto` Ed25519). Runtime dependency: **yauzl** only (zip). Do not add axios or node-fetch.

Publish later from this repository’s `sdk/typescript` branch (package root = git root). Tags: `sdk-typescript-v<semver>`.

## Quick example

```ts
import { Client, Updater, NodeHasher } from "@kirizu/kirivers-client";

const client = new Client({
  baseUrl: "http://127.0.0.1:8080",
  projectRef: "my-app",
  // projectToken: process.env.KIRIVERS_PROJECT_TOKEN,
  // channelToken: process.env.KIRIVERS_CHANNEL_TOKEN, // never log
});

const check = await client.check({
  current_version: "1.0.0",
  os: "windows",
  arch: "x86_64",
  channel: "stable",
  device_id: myStableDeviceId, // you generate this; the SDK never invents it
});

if (check.kind === "update") {
  const pkg = await client.downloadUrl(check.body.package_url);
  const sha = new NodeHasher().sha256(pkg.body);
  // sha must equal check.body.sha256
}

const updater = new Updater({
  client,
  os: "windows",
  arch: "x86_64",
  currentVersion: "1.0.0",
  channel: "stable",
  deviceId: myStableDeviceId,
  stageDir: "./.kirivers-stage",
  // targetPath omitted → download+verify only (no replace)
});
const result = await updater.run();
```

Callers pass `device_id`, channel, `custom`, os/arch, and the current version. The SDK does not generate device identity and does not write plaintext `device_id` to logs.

## Native JSON surface

| Method | Path | SDK |
|--------|------|-----|
| GET | `/api/v1/health` | `client.health()` |
| GET | `/api/v1/projects/{ref}` | `client.project()` |
| POST | `.../clients/report` | `client.deviceReport()` |
| POST | `.../update/check` | `client.check()` — 200 / 204 (no update, not an error) / 304 |
| GET | `.../changelog/{channel}/{os}/{arch}` | `client.changelog()` |
| GET | `.../versions/{version}/integrity` | `client.integrity()` |
| POST | `.../update/diff` | `client.diff()` |
| POST | `.../update/pack` | `client.pack()` / `packUntilReady()` (same JSON poll) |
| GET/HEAD | `.../packages/{sha256}` | `downloadPackage` / `downloadUrl` / `headPackage` (keeps `exp`/`sig`, `Range`) |
| POST | `.../telemetry/report` | `client.reportTelemetry()` (HTTP 202; Updater never fails apply on telemetry) |
| GET | `.../channels` `.../matrix` `.../languages` | catalog |
| GET | `.../announcements` | `client.announcements()` |
| GET/HEAD | `.../media/{id}` | markdown images |

Not implemented (out of product scope or leftover): `/store/...`, `GET /update/check`, `POST /clients/login`, `GET .../manifest`, `POST /update/pack/status`, `GET /ready`, `/artifacts/{id}/{filename}`.

`Client.check` sends `capabilities: ["full_package"]` unless you pass more. `Updater` derives capabilities from **live adapters** (D13).

## Adapters

| Adapter | Default | Inject to replace |
|---------|---------|-------------------|
| Transport | Node 20 `fetch` | required for tests; always injectable |
| JSON | `JSON` (not an adapter) | — |
| FileStore | `node:fs` / `node:path`; NFC via `String.normalize('NFC')` | yes |
| Hasher | `node:crypto` SHA-256 (MD5 if integrity asks) | yes |
| SignatureVerifier | `node:crypto` Ed25519 + RSA-SHA256 | yes |
| ArchiveUnpacker | **yauzl** (D18) | yes (`null` disables `patch_package`) |
| Patcher | **interface only** | inject to enable `binary_delta` |
| Replacer | `fs.rename` (EXDEV copies). Locked Windows files → inject your own | `null` skips apply |

This is **not a one-click installer**. Default `fs.rename` replaces app-level files you name with `targetPath`. It does not relaunch Electron, unlock a running `.exe`, or sideload APKs. Supply a `Replacer` for those platforms.

### Patcher snippet

```ts
import type { Patcher } from "@kirizu/kirivers-client";

const patcher: Patcher = {
  supportedAlgos: () => ["bsdiff"], // only algos you can really apply
  async apply(algo, oldBytes, delta) {
    return myBsdiffPatch(oldBytes, delta);
  },
};

new Updater({ client, os, arch, currentVersion, patcher, localFile: "./app.bin" });
```

The SDK checks container magic (`KVDIFFHP1\\n`, `HDIFF13&`, `BSDIFF40`, VCDIFF `D6 C3 C4`) and **never cross-decodes**. Unknown magic falls back to the full package. Do not send `accepted_delta_algos` you cannot apply. `local_sha256` is sent only on `POST /update/diff`, never on check.

## Capability matrix (Updater)

| Adapters present | Check `capabilities` |
|------------------|----------------------|
| Transport (always) | `full_package` |
| + yauzl / ArchiveUnpacker | `patch_package` |
| + FileStore that can write files | `file_list` |
| + Patcher with `supportedAlgos()` | those names in `accepted_delta_algos` (implies `binary_delta`) |

No Patcher → **no** `binary_delta`. Empty advertised algos are forbidden.

## Platform notes

| Runtime | Support |
|---------|---------|
| Node.js 20+ | yes |
| Electron **main / app-level** | yes (same Node APIs) |
| Electron renderer without Node | **no** |
| Browser / Deno | **no** |
| Android APK install | caller `Replacer` |
| Windows in-use executable | caller `Replacer` (MoveFileEx / helper BAT you own) |

## Errors

JSON failures parse `{ "error": { "code", "message", "details" } }` into `ApiError`. Unknown `code` values stay opaque strings. HTTP 204 on check is success (already up to date). 304 is an ETag hit.

## OpenAPI snapshot

`openapi.client.json` at the package root is a copy of the client-plane contract. `OPENAPI_REVISION` is `1.0.0` plus a short digest. Contract tests load this file; they do not generate client code.

## License

MIT
