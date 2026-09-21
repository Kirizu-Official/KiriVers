---
title: "Settings"
description: "Matches the settings.vue panels from basics through cleanup and members. Install policy has its own page."
---

# Settings

Nav **Settings**. Description: “Project policy, storage, webhooks, privacy, and members.” Click **Save** to PATCH. Install-policy rules are not in this JSON; see [Install policy](./install-policy).

1. **Open project** → **Settings**.
2. Edit a panel, then **Save** (or that panel’s confirm button).

## Basic

| Field | UI | When to change | Failure |
|-------|----|----------------|---------|
| Slug | **Slug** | Public identity rename | Expired alias → client <ErrorCode code="PROJECT_NOT_FOUND" /> |
| Alias retention (days) | **Alias retention (days)** | How long the old slug still resolves | Illegal → 400 <ErrorCode code="INVALID_REQUEST" /> |
| Compare engine | **Compare engine** | Only before first publish | <ErrorCode code="COMPARE_ENGINE_IMMUTABLE" /> |
| Minimum supported version | **Minimum supported version** | Force very old clients up | Compared with the project engine |
| Default locale | **Default locale** | Fallback copy | Must be a registered language |

## Update Policy

Open **Update Policy**.

- **Prefer older, recently active devices in gray admission**: on by default. Off → `created_at`, id ascending.
- **file_list pending-file cap**: `0` inherits the instance ceiling. Over the input max → 400 <ErrorCode code="INVALID_REQUEST" />.

## Install policy templates

Panel title **Install policy templates**. Edit on [Install policy](./install-policy).

## Security

- **Require client token for checks**: missing token → 401 <ErrorCode code="UNAUTHORIZED" />.
- **Store token**: **Generate** or paste; plaintext is shown only on this save. GET returns `has_store_token`. **Clear store token** removes it.
- **Force HTTPS** / **CORS origins (comma separated)**.

## Storage

**Storage visibility** is locked public. Private is rejected (400 <ErrorCode code="INVALID_REQUEST" />). **Storage prefix** / **Bucket** must not equal the project slug. Signing algorithm and public key sign check payloads.

## Store listings

Envelope `{ listings: [] }`.

1. Click **New listing**.
2. Set **Protocol** (`electron`, `sparkle`, `tauri`, … matching `Protocol()`), **Listing slug**, optional OS/arch/channel pins, **Package source**.
3. Copy **Store URL** into the updater. Duplicate `(protocol, slug)` → 400 <ErrorCode code="INVALID_REQUEST" />. After delete the feed is **plaintext 404** (no JSON `code`).

Concepts: [Store listings](/en/guide/features/store).

## Webhook

**Webhook URL**. Events: version published, line ready, version revoked. **View delivery logs**. Do not paste secrets into examples. See [Webhooks](/en/guide/features/webhooks).

## Changelog Defaults

Scope, layout, **Default changelog entries** / **Changelog max entries** are clamped by instance `changelog.*`. Over ceiling → 400 <ErrorCode code="INVALID_REQUEST" />. See [Changelog](/en/guide/features/changelog).

## Rate Limits

Panel **Rate limit**. Excess → 429 <ErrorCode code="RATE_LIMITED" /> with `Retry-After`. `store_per_ip_per_minute` is the shared public-GET IP bucket.

## Privacy

**device_id policy**, **Telemetry retention (days)**, **Delete device by hash**. Erase returns `telemetry_deleted` / `allowlist_deleted`. Deleting a roster row is not this action. See [Clients](./clients).

## Cleanup

**Artifact cleanup**: set **Retention (days)** and confirm **Run artifact cleanup? Purged artifacts cannot be recovered.** Native full packages and store `store_full` are never deleted. Progress: header **Task Center**. Failure: <ErrorCode code="JOB_FAILED" />.

## Project members

1. **Add member**, pick **Role** (**Owner** / **Admin**).
2. **Change role** or **Remove member**. Last owner: <ErrorCode code="LAST_OWNER" />. Unknown: <ErrorCode code="MEMBER_NOT_FOUND" />.
