---
title: "Channels"
description: "System channels cannot be deleted. A wrong channel token is not 403. Slug is immutable after create."
---

# Channels

Nav **Channels**. System channels `alpha` / `beta` / `stable` cannot be deleted (<ErrorCode code="SYSTEM_CHANNEL" />). Display names are written in the UI language at project create. Custom slugs cannot change after create. Missing channel: <ErrorCode code="CHANNEL_NOT_FOUND" />.

1. Open **Channels**.
2. **New channel**: fill **Channel** (3–64 lowercase letters, digits, hyphen), **Display name**, **Stability rank**.
3. Optionally tick **Unlisted**; set or **Clear channel token**.
4. Promote a published version from a less-stable channel with **Promote channel** on the version page.
5. **Install policy** opens the overlay editor for that channel.

| Field | UI | Meaning |
|-------|----|---------|
| slug | **Channel** | Used by check and listings; immutable after create |
| unlisted | **Unlisted** | Omitted from public catalogs and not auto-selected unless the client is already on that slug or queries it |
| token | **Channel token** | Wrong `X-Channel-Token` on check **skips that hidden channel**, not 403. Changelog mismatch is 404 |

SemVer prerelease suffix must match the channel or <ErrorCode code="CHANNEL_SUFFIX_MISMATCH" />. `stable` cannot use a prerelease suffix. Concepts: [Channels](/en/guide/features/channels).
