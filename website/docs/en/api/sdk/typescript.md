---
title: "TypeScript SDK"
description: "@kirizu/kirivers-client. Node 20+ / Electron main. No browser, Deno, or renderer."
---

# TypeScript

::: warning Runtime
**Not for web pages.** No browser export, no Deno, no Electron renderer. Node.js 20+ and Electron **main** only.
:::

Runtime dependency: **yauzl** only. Do not add axios / node-fetch.

## 1. Install

```bash
npm install @kirizu/kirivers-client
```

::: warning Not on npm yet
Clone `-b sdk/typescript` and install locally.
:::

## 2. Quick start

```ts
import { Client } from "@kirizu/kirivers-client";

const client = new Client({
  baseUrl: "http://127.0.0.1:8080",
  projectRef: "my-app",
});
const check = await client.check({
  current_version: "1.0.0",
  os: "windows",
  arch: "x86_64",
  channel: "stable",
  device_id: "your-stable-device-id",
});
if (check.kind === "update") {
  console.log(check.body.package_url);
}
// 204 is not an update; 304 is not modified
```

Node 20+ / Electron main process only.

## 3. Download and verify

`client.downloadUrl(check.body.package_url)` + `NodeHasher().sha256`.

## 4. Updater

Omit `targetPath` to stage. Default `fs.rename` is not a busy-exe / APK installer.

## 5. Client methods

`health()`, `project()`, `deviceReport()`, `check()`, changelog/integrity/diff/pack, download/head, catalogs, announcements, telemetry, media. Not implemented: `/store/`, GET check, `clients/login`, `GET /ready`, manifest, pack/status, artifact filename.

## 6. Config and auth

`baseUrl`, `projectRef`, `projectToken`, `channelToken`. Do not log tokens.

## 7. Adapters

Transport: Node 20 `fetch`. JSON: `JSON`. FileStore `node:fs`. Hasher/SignatureVerifier `node:crypto`. ArchiveUnpacker yauzl. Patcher interface only. Replacer `fs.rename` (copy on EXDEV).

## 8. Capabilities

Updater advertises live adapters. No Patcher → no `binary_delta`.

## 9. Patcher

Inject `supportedAlgos` + `apply`. Do not cross-decode magics.

## 10. Replacer

Busy Windows exe and APK need a caller Replacer. Default rename only replaces `targetPath`.

## 11. Errors

`ApiError` reads `error.code`. 204/304 are success.

## 12. Transport / tests

Inject Transport. No Docker.

## 13. Language notes

Node 20+ / Electron main; no browser/Deno/renderer; runtime yauzl only.
