---
title: "macOS / Sparkle"
description: "protocol=sparkle. appcast.xml. sparkle:version is the build number; shortVersionString is the marketing version. Official SDKs do not read the appcast."
---

# macOS / Sparkle

Sparkle, WinSparkle, and NetSparkle share protocol `sparkle`. Document name: `appcast.xml`. Official SDKs wrap native JSON check only and do **not** parse the appcast.

## Package shape

A **single-file** macOS (or WinSparkle) installer. Each appcast item has one enclosure: default hardware variant only. `sparkle:edSignature` signs enclosure bytes (isolated from native check signature format). RSA projects omit `sparkle:dsaSignature` (this server does not support DSA).

The two version fields do **not** follow project `compare_engine`:

| Element | Source |
|---------|--------|
| `sparkle:version` | Internal build `version_integer` (omitted if empty) |
| `sparkle:shortVersionString` | Marketing SemVer `version_semver` (omitted if empty) |

The `stable` channel omits `sparkle:channel` (no element means the default channel).

## Console

1. **Settings → Store listings** → **New listing**, protocol `sparkle`.
2. Store URL points at the appcast:

```text
https://updates.example.com/api/v1/projects/myapp/store/sparkle/stable/appcast.xml
```

## Updater config

::: code-group

```xml [Info.plist]
<key>SUFeedURL</key>
<string>https://updates.example.com/api/v1/projects/myapp/store/sparkle/stable/appcast.xml</string>
```

```ini [WinSparkle]
FeedURL=https://updates.example.com/api/v1/projects/myapp/store/sparkle/stable/appcast.xml
```

:::

## Failures

| Symptom | Cause |
|---------|--------|
| Plaintext **404** | Missing listing, unknown path, protocol off |
| HTTP 401 <ErrorCode code="UNAUTHORIZED" /> | Feed auth failed |
| Signature verify failed | Client public key mismatch; RSA projects have no DSA signature |

Related: [Store listings](/en/guide/features/store), [Desktop](./desktop).
