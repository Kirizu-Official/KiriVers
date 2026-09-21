---
title: "Conventions"
description: "Client plane :8080. JSON snake_case. Logic uses error.code. Probe GET /api/v1/health."
---

# Conventions

Base: client plane default `:8080`, paths under `/api/v1/...`. `project_ref`: UUID / live slug / unexpired alias; expired alias → 404 `PROJECT_NOT_FOUND`. POST is never 301.

JSON is snake_case. Timestamps are RFC 3339 UTC. Error envelope:

```json
{ "error": { "code": "UNAUTHORIZED", "message": "…", "details": null } }
```

Branch on `code`. A missing store listing is **plaintext 404** without that envelope.

Probe: `GET /api/v1/health` is always 200; `ready` is DB Ping + storage Head `.ready` (not Redis). `GET /api/v1/ready` is **404**.

This plane’s contract: `GET /api/v1/openapi.json`.

## Auth

| Mechanism | Use | Failure |
|-----------|-----|---------|
| `X-Project-Token` or `Authorization: Bearer` | Project token | 401 |
| `X-Channel-Token` | Hidden channels | Wrong value skips the channel on check; **not** 403 |
| Store listing token | Feeds | StoreAuth; missing listing is plaintext 404 |
| Admin Cookie/Bearer | Admin plane | 401 / 403 |

Full schemas: [API reference](/en/api/reference/).
