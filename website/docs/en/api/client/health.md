---
title: "Health"
description: "GET /api/v1/health is always 200. ready is DB+storage, not Redis. GET /api/v1/ready is 404."
---

# Health

`GET /api/v1/health` → HTTP **200**. JSON `ready` is database Ping plus storage Head `.ready`. Redis is **not** included.

```bash
curl -sS -D - http://127.0.0.1:8080/api/v1/health
```

Expect `HTTP/1.1 200`. If Postgres is down at runtime the status stays 200 with `ready=false`. `GET /api/v1/ready` is unregistered (404) and is not a readiness probe.

The admin plane exposes the same path. Schema: [API reference](/en/api/reference/) (`openapi.client.json`; <a href="/en/api/scalar/client" target="_blank" rel="noopener">open Scalar</a>).
