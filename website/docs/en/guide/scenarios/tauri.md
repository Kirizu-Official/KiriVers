---
title: "Tauri"
description: "protocol=tauri. Static latest.json or dynamic {target}/{arch}/{current_version}. Official SDKs do not read this feed."
---

# Tauri

The Tauri v2 updater reads a JSON feed. Backend protocol: `tauri`. Official SDKs wrap native JSON check only and do **not** read `latest.json`.

## Package shape

Ship per-OS installers (`.msi` / `.dmg`, …). Feeds project the default hardware variant only; the protocol cannot express `hw_rev`. `signature` is a minisign container (not the native check raw signature bytes). RSA projects or a missing private key omit `signature`.

Two endpoints:

| Shape | Path | Behavior |
|-------|------|----------|
| Static | `.../store/tauri/{listing}/latest.json` | Multi-platform `platforms` object. Optional `os` / `arch` / `channel` query; omitted query covers Tauri-capable matrix combos |
| Dynamic | `.../store/tauri/{listing}/{target}/{arch}/{current_version}` | Update → 200 single-platform object; none → **204** empty body. `target` ∈ `darwin` \| `windows` \| `linux` (`macos` alias accepted) |

Unknown `target` / `arch` or any other path → plaintext 404.

## Console

1. **Settings → Store listings** → **New listing**, protocol `tauri`.
2. Copy the **Store URL**. For the dynamic endpoint append `{{target}}/{{arch}}/{{current_version}}`.
3. Put the client public key in listing identifiers `public_key` (this server does not verify on the client’s behalf).

## Updater config

::: code-group

```json [dynamic endpoints]
{
  "plugins": {
    "updater": {
      "endpoints": [
        "https://updates.example.com/api/v1/projects/myapp/store/tauri/stable/{{target}}/{{arch}}/{{current_version}}"
      ],
      "pubkey": "<minisign-public-key>"
    }
  }
}
```

```json [static latest.json]
{
  "plugins": {
    "updater": {
      "endpoints": [
        "https://updates.example.com/api/v1/projects/myapp/store/tauri/stable/latest.json"
      ],
      "pubkey": "<minisign-public-key>"
    }
  }
}
```

:::

A real Tauri client usually requests the static document **without** os/arch query.

## Failures

| Symptom | Cause |
|---------|--------|
| Plaintext **404** | Missing listing, protocol off, unknown path |
| HTTP **204** | Dynamic endpoint: already latest |
| HTTP 401 <ErrorCode code="UNAUTHORIZED" /> | Feed token failed |

Related: [Store listings](/en/guide/features/store), [Update check](/en/guide/features/check).
