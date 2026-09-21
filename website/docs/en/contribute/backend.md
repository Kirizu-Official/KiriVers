---
title: "Backend layers"
description: "Controllers have no business rules. service includes update/ and store/. Cache is fail-closed at start."
---

# Backend layers

`controller` (Gin handlers, no business rules) → `service` (including `update/`, `store/`) → `repository` → `model`.

Cross-cutting: `middleware`, `storage` (Local/S3), `cache` (memory/Redis; `cache.Open` fail-closed Ping), `internal/delta`, `internal/platform`.

`pkg/`: `hashutil`, `pathutil`, `response`, `semver`, `signature`, `urlsign`, `webhook`, `grayutil`. Check **must not** use HMAC gray; `grayutil` is leftover.

Errors use `pkg/response` only. The admin plane reverse-proxies `/api/v1/projects/**` when `ClientProxyURL` is set.
