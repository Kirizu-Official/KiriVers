---
title: "Channels"
description: "System channels alpha/beta/stable cannot be deleted. Slug is immutable. A wrong channel token is not 403."
---

# Channels

Each version belongs to **one** channel. Channels split insider / beta / stable tracks and can pin a store listing.

## When to use

Use channels for multi-track releases, unlisted tracks, or channel tokens. A single-track product can keep only system `stable`.

## Configuration entry

Project → **Channels**. Steps: [Channels](/en/admin/projects/channels). No YAML table.

## Rules and error codes

| Concept | Behavior |
|---------|----------|
| System `alpha` / `beta` / `stable` | Seeded at project create; **cannot delete** (<ErrorCode code="SYSTEM_CHANNEL" />) |
| Custom slug | Immutable after create |
| `stability_rank` | Higher is more stable. Promote from a lower channel |
| `unlisted` | Omitted from public catalogs; not auto-selected unless the client is already on that slug or queries it |
| Channel token / `X-Channel-Token` | Wrong token on check **skips that hidden channel**, not 403. Changelog mismatch is 404 |

SemVer prerelease suffix must match the channel or <ErrorCode code="CHANNEL_SUFFIX_MISMATCH" />. `stable` cannot use a prerelease suffix. Missing channel: <ErrorCode code="CHANNEL_NOT_FOUND" />.

## Client duties

Public `GET /channels` omits unlisted and token-protected rows. Hidden-channel upgrades use check `channel=` + `X-Channel-Token`.

Related: [Releases](./versions), [Install policy](./install-policy), [Store listings](./store).
