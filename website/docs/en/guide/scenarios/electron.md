---
title: "Electron"
description: "protocol=electron. generic provider reads latest.yml / latest-mac.yml / latest-linux.yml. Official SDKs do not read this feed."
---

# Electron

Electron apps use the **electron-updater generic provider** to fetch YAML feeds. Backend protocol: `electron`.

Official SDKs wrap native JSON check only and do **not** read `latest.yml`. Homegrown non-electron-updater clients should **POST** [update check](/en/guide/features/check).

## Package shape

Ship a **single-file** installer per OS (NSIS / DMG / AppImage, …). The YAML projects only the default hardware variant `kind=full` artifact for that OS. Any document name other than the three files below is plaintext 404.

| Document | OS |
|----------|-----|
| `latest.yml` | windows |
| `latest-mac.yml` | macos |
| `latest-linux.yml` | linux |

generic provider requires **electron-updater ≥ 6** (`files[]`; `path` is `{sha256}{ext}`; downloads use `/packages/` hash URLs, not files next to the YAML).

## Console

1. Project **Settings → Store listings**, click **New listing**.
2. Protocol `electron`. Copy the **Store URL** (includes the listing slug).
3. Pinned os / arch / channel overwrite conflicting query values.

Details: [Settings](/en/admin/projects/settings).

Listing root:

```text
https://updates.example.com/api/v1/projects/myapp/store/electron/stable
```

The updater then requests `{root}/latest.yml` (Windows), `latest-mac.yml`, and `latest-linux.yml`.

## Updater config

Paste the console **Store URL** into generic `url`. Do not patch electron-updater internals.

::: code-group

```yaml [electron-builder]
publish:
  provider: generic
  url: https://updates.example.com/api/v1/projects/myapp/store/electron/stable
```

```js [setFeedURL]
autoUpdater.setFeedURL({
  provider: 'generic',
  url: 'https://updates.example.com/api/v1/projects/myapp/store/electron/stable',
})
```

:::

## Failures

| Symptom | Cause |
|---------|--------|
| Plaintext **404** (no JSON `code`) | Missing listing, protocol off, unknown filename, slug-less legacy path |
| HTTP 401 <ErrorCode code="UNAUTHORIZED" /> | Feed auth failed |
| No update / empty channel | Anonymous feeds include only `GrayIsComplete()` versions; an empty visible set does not emit empty YAML |

Related: [Store listings](/en/guide/features/store), [Store HTTP](/en/api/client/store).
