---
title: "CDN and downloads"
description: "public_base_url. Check must not be a public CDN object. storage_visibility=private is rejected."
---

# CDN and downloads

Point `storage.s3.public_base_url` at a CDN. Object key `{slug}/{sha256}`.

## When to use

Use when packages are large or you need edge cache. Local disk is enough for a small fleet.

## Configuration entry

YAML `storage.s3.public_base_url`: [S3](/en/guide/config/s3). Production must pin `url_signing_secret`: [Security](/en/guide/config/security). Console **Storage visibility** is locked public.

## Rules

<Badge type="warning" text="check is not public cache" />

Safe to edge-cache: public artifact GET, store feeds (**complete gray**, not private/local-proxy), announcement GET, media (immutable).

Not public cache: check (including incomplete gray), signed URLs, local-proxy downloads, check with a channel token.

Admin PATCH **rejects** `storage_visibility=private` (400 <ErrorCode code="INVALID_REQUEST" />). Signed URLs still apply to local-proxy. Align Range support with the CDN. Unknown hash → 404 <ErrorCode code="NOT_FOUND" />. Bad signature → 403 <ErrorCode code="FORBIDDEN" />.

## Client duties

Download `package_url` and verify SHA-256. Keep `exp`/`sig` query params. Support Range.

Related: [Update check](./check), [Multi-node](./cluster).
