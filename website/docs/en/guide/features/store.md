---
title: "Store listings"
description: "Listing-slug URLs. Missing listing is plaintext 404. Official SDKs do not read store feeds."
---

# Store listings

A store listing lets Electron, Sparkle, Tauri, and similar **built-in updaters** fetch a protocol document. Identity is the listing slug. URL:

`/api/v1/projects/{project_ref}/store/{protocol}/{listing_slug}`, optional `/{doc}`.

## When to use

Use when the app already ships a store updater. Homegrown clients should **POST** [update check](./check). Official SDKs do **not** read store feeds.

## Configuration entry

Project **Settings → Store listings** (**New listing**). Steps: [Settings](/en/admin/projects/settings). No YAML protocol map.

## Rules and error codes

Pinned os/arch/channel overwrite conflicting query values. Missing listing, unknown protocol, or a slug-less legacy path → **plaintext 404** (no JSON `code`). Feed auth failure → 401 <ErrorCode code="UNAUTHORIZED" />. Duplicate `(protocol, slug)` → 400 <ErrorCode code="INVALID_REQUEST" />. Anonymous feeds include only `GrayIsComplete()` versions.

Protocols: `sparkle`, `electron`, `tauri`, `squirrel`, `clickonce`, `appimage`, `winget`, `msix`, `fdroid`.

## Updater duties

Paste the console **Store URL** into the updater. Do not patch application source to rewrite feed paths.

Related: [Electron](/en/guide/scenarios/electron), [Sparkle](/en/guide/scenarios/sparkle), [Tauri](/en/guide/scenarios/tauri), [Store HTTP](/en/api/client/store).
