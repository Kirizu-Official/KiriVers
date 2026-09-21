---
title: "Operations"
description: "GET /api/v1/health vs ready. GET /api/v1/ready is 404. Runtime DB. Redis fallback."
---

# Operations

- `GET /api/v1/health`: always 200. `ready` = DB + storage, **not Redis**.
- `GET /api/v1/ready`: **404**. Not a readiness probe.
- Runtime Postgres outage: business `/api` is 404; health stays 200 with `ready=false`.
- Redis: startup Ping is required when `driver=redis`; a later outage falls back to memory and reconnects.
- Reverse proxies must set per-plane `trusted_proxies`; an empty list does not trust all `X-Forwarded-For`.
- Backups: [Backup](/en/guide/backup). There is no `kirivers listen`. No separate migrate CLI (AutoMigrate).
