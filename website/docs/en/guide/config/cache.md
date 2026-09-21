---
title: "Cache and Redis"
description: "Default memory. driver=redis is fail-closed at startup Ping; runtime failure falls back to memory and reconnects."
---

# Cache and Redis

Hot-path cache (Resolve / LoadCatalog). Catalog snapshots include signing material. Production Redis **must** set a password and must not be shared with untrusted tenants.

| Key | Type | Default | Meaning | When to change |
|-----|------|---------|---------|----------------|
| `cache.driver` | `memory` \| `redis` | `memory` | In-process or Redis | Multi-process or shared cache → `redis` |
| `cache.redis.addr` | string | `127.0.0.1:6379` | Redis address | Required when `driver=redis`; change for Docker / remote |
| `cache.redis.password` | string | `""` | Required in production; `KIRIVERS_CACHE_REDIS_PASSWORD` | Must set in production; snapshots include signing material |
| `cache.redis.db` | int | `0` | Logical database | Isolate when sharing a Redis instance |
| `cache.redis.reconnect_interval_minutes` | int | `10` | Reconnect interval after runtime fallback; `<1` becomes 1 | Shorter to return to Redis faster; longer to reduce flapping |

## Startup vs runtime

| Phase | `memory` | `redis` |
|-------|----------|---------|
| Startup | No Redis connection | **Ping must succeed** or the process exits non-zero and does not listen |
| Runtime outage | n/a | Fall back to in-process memory, reconnect on the interval, flush the `kirivers:` prefix, then switch back |

`ready` on `GET /api/v1/health` does **not** include Redis. Runtime Redis fallback does not mark health unready.

Skip Redis on a single machine. Multiple KiriVers processes require Redis. [cluster.download](./cluster) is active only when `storage.driver=s3` **and** `cache.driver=redis`.

<<< @/../../configs/config-example.yaml{104-110}
