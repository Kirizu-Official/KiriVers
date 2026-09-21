# Cache Guidelines

> Process read-cache for project identity and update catalogs. All cache I/O goes through `cache.Store`. Do not import the Redis SDK from `service` / `repository` / `controller`. Do not use Redis as a job queue (see `database-guidelines.md`).

---

## Overview

Drivers: `memory` (default) and `redis` (single `addr`, `github.com/redis/go-redis/v9`). Factory: `cache.Open(Options)` from process config. This package does not read environment variables.

Hot paths: `ProjectService.Resolve` (UUID / live slug / unexpired alias) and `update.CatalogLoader` (`kirivers:catalog:{id}:{os}:{arch}`). Writes that change identity or catalog must call `cache.InvalidateProject`. Dynamic-pack **occupancy** uses `SetNX` (`PackOccupancyKey`) plus a done key after the public Put — still not a Redis job queue. Node CPU/memory snapshots and GeoIP readers stay **process-local**; node rows live in PostgreSQL. HTTP CDN headers (ETag / Cache-Control / 304) are a different layer — do not replace them with this package. Announcement list ETags are computed from the `announcements` table (`announcement-contract.md`); announcement-only writes do **not** call `InvalidateProject`.

---

## Scenario: Open, failover, and invalidate

### 1. Scope / Trigger

Infra integration: `config.yaml` `cache.*`, server startup Ping, runtime Redis outage, cache-aside on client check/feed, write-path invalidation.

### 2. Signatures

```go
const KeyPrefix = "kirivers:"
const DefaultReconnectInterval = 10 * time.Minute

type Store interface {
    Get(ctx context.Context, key string) (value []byte, hit bool, err error)
    Set(ctx context.Context, key string, value []byte) error
    SetWithTTL(ctx context.Context, key string, value []byte, ttl time.Duration) error
    SetNX(ctx context.Context, key string, value []byte, ttl time.Duration) (bool, error)
    Delete(ctx context.Context, keys ...string) error
    DeletePrefix(ctx context.Context, prefix string) error
    Close() error
}

func Open(opts Options) (Store, error)
func AdminSessKey(token string) string
func AdminPendKey(token string) string
func AdminWebAuthnKey(adminID uuid.UUID) string
func InvalidateProject(ctx context.Context, store Store, id uuid.UUID) error

func ProjectIDKey(id uuid.UUID) string
func ProjectSlugKey(slug string) string
func ProjectAliasKey(slug string) string
func ProjectLookupsKey(id uuid.UUID) string
func CatalogKey(projectID uuid.UUID, os, arch string) string
func CatalogPrefix(projectID uuid.UUID) string
func PackOccupancyKey(idempotencyKey string) string
func PackDoneKey(idempotencyKey string) string
```

`Get` miss is `(nil, false, nil)`. Redis connection failures are `error`, never a miss (failover wrapper needs the error).

### 3. Contracts

- Config (system YAML): `cache.driver` (`memory` | `redis`), `cache.redis.{addr,password,db,reconnect_interval_minutes}`.
- Env (after `BindEnv`): `KIRIVERS_CACHE_DRIVER`, `KIRIVERS_CACHE_REDIS_ADDR`, `KIRIVERS_CACHE_REDIS_PASSWORD`, `KIRIVERS_CACHE_REDIS_DB`, `KIRIVERS_CACHE_REDIS_RECONNECT_INTERVAL_MINUTES`.
- Default driver is `memory` so example YAML starts without Redis. Default reconnect interval is 10 minutes; `cmd/server.go` clamps YAML `< 1` to 1 minute. `cache.Open` may honor sub-minute `Options.ReconnectInterval` for tests.
- `driver=redis`: startup `Ping` must succeed or `Open` returns error. `cmd/server.go` logs `mod=cache` then `fatal` (stderr + exit 1) **before listen**. Postgres `database.Open` is the same fail-closed-at-start rule; runtime Redis failovers to memory, runtime Postgres returns API 404 and `Watch` reconnects.
- `driver=redis` runtime: operation errors (not miss) switch to an in-process memory standby. Ticker Ping; on success `DeletePrefix("kirivers:")` on Redis, drop the memory map, and switch back **under the same lock**. Do not rewrite `cache.driver`.
- `/health` `ready` is still `service.CheckReady` = DB Ping **AND** storage Head `.ready`. Redis outage / memory fallback does not flip `ready`.
- Callers must clone on read (JSON round-trip or `cloneBytes`). `json:"-"` fields on `model.Project` / `model.Version` (`DeviceSecret`, signing/store/webhook material, `VersionSemverCanonical`) must be copied by the typed envelopes or they vanish on a hit.
- Alias entries store `expires_at`; a hit after expiry is a miss + delete.
- Nil `Store` on `ProjectService` / `InvalidateProject` is a no-op (tests keep DB-only behavior).
- Redis holds catalog JSON that can include `SigningPrivateKey`. Treat Redis like Postgres: password required in production; do not share the DB with untrusted tenants. Do not log the password.
- No Sentinel/Cluster in MVP. No Redis **job queue**. Dynamic-pack occupancy `SetNX` is allowed; Postgres `jobs` + `SKIP LOCKED` remains the execution queue. Do not store node heartbeat or GeoIP readers in Redis.

### 4. Validation & Error Matrix

| Condition | Result |
|-----------|--------|
| `driver` empty / `memory` | In-process map; never dials Redis |
| `driver=redis` and Ping fails | `Open` error → process `fatal` before listen |
| Unknown `driver` | `Open` error → `fatal` |
| Redis `GET` nil | miss (`ok=false`), not error |
| Redis GET/SET/DEL/SCAN error at runtime | failover to memory; log `error` `mod=cache`; request continues (miss → DB) |
| Reconnect Ping ok but prefix flush fails | stay on memory; try next tick |
| `InvalidateProject` with nil store or nil UUID | no-op |
| Invalidation error after a successful DB write | log `error` `mod=cache`; do **not** fail the admin write |

### 5. Good/Base/Bad Cases

- Good: second `Resolve` / `LoadCatalog` with memory or miniredis does not call the inner repository; Publish then LoadCatalog sees the new version.
- Base: `cache.driver=memory` in `config-example.yaml`; process starts with docker Redis down.
- Bad: treating Redis Ping failure at start like a runtime blip (listen anyway); AND-ing Redis into `/health` `ready`; returning Redis errors as cache miss; mutating a cached `*model.Project` / `*update.Catalog` without clone; skipping `InvalidateProject` on a new catalog write (including auto-created version lines, **`packs_ready_at`**, dynamic-pack persist, patch cleanup, telemetry privacy allowlist delete, `EnsureGrayAdmission`, gray complete, and allowlist POST).

### 6. Tests Required

- Memory Store: Get/Set/Delete/DeletePrefix; Get returns a cloned buffer.
- Redis Store: same contract via `miniredis` (CI must not require docker Redis). `SetNX` returns false on a second writer (pack occupancy).
- `Open` redis Ping failure returns error.
- Failover: Redis error → memory serves; after Ping, prefix flushed, new writes land in Redis; reconnect flush + memory drop are atomic.
- `Resolve`: slug/alias hit; alias past `expires_at` is miss; `json:"-"` secrets survive.
- `CatalogLoader`: second load no inner call; `Allowlist.Lines` `map[uuid.UUID]` round-trip; signing material preserved.
- Config: default driver `memory`, interval 10; `KIRIVERS_CACHE_*` BindEnv when YAML omits keys.
- Invalidation: Patch slug / Publish / allowlist mutate / gray complete / `EnsureGrayAdmission` drop identity and catalog keys.

### 7. Wrong vs Correct

#### Wrong

```go
db, err := database.Open(dsn, dbLog)
if err != nil {
    fatal(err) // postgres down at start → no listen
}
store, _ := cache.Open(redisOpts)  // redis down → still listen
return service.CheckReady(ctx, db, objectStore, cacheStore)
```

#### Correct

```go
cacheStore, err := cache.Open(opts)
if err != nil {
    cacheLog.Error().Err(err).Msg("open cache")
    fatal(err) // stderr + exit 1; no listen
}
return service.CheckReady(ctx, db, objectStore) // DB Ping AND storage Head only
```

---

## Design Decision: Fail-closed Redis at start, memory fallback at runtime

**Context**: Operators who set `cache.driver=redis` expect a shared cache. A dead Redis at boot is a misconfiguration. A blip later should not take the node out of the load balancer.

**Options**: (1) Always listen and AND Redis into `/health` `ready`; (2) Fatal on startup Ping, runtime failover to memory + timed reconnect, `/health` `ready` unchanged (DB+storage only).

**Decision**: Option 2. Default `driver=memory` so local example YAML does not depend on docker Redis. On reconnect, flush `kirivers:` then drop memory so Redis cannot serve pre-outage keys.

**Extensibility**: Sentinel/Cluster would be a new driver, not a silent change to `addr`.

---

## Don't: Cache-aside without invalidation

Any service method that persists catalog-visible rows (version lifecycle, lines including **`packs_ready_at`**, artifacts including dynamic-pack `kind=patch` persist and patch cleanup, manifest, gray allowlist, channel/matrix/hw_rev, **project languages / `default_locale`**, telemetry privacy allowlist delete) must call `InvalidateProject` after a successful write. Token/job/audit/telemetry event writes do not. Language writes sync `projects.default_locale` (used by `BuildLocaleChain` and the catalog ETag projection) so skipping invalidation leaves check/changelog on a stale locale.

---

## Don't: Immortal `Set` for admin sessions

Catalog identity uses `Set` (no TTL). Admin login uses `SetWithTTL`:

- Full session: `AdminSessKey` (`kirivers:admin:sess:{token}`), idle TTL, slide on every `AdminAuth` hit.
- Pending 2FA: `AdminPendKey`, `login_pending_ttl_seconds`, **do not slide**.
- WebAuthn ceremony: `AdminWebAuthnKey`, short TTL.

Never cache TOTP secrets, recovery plaintext, passkey private keys, or the pending-login `has_passkey` flag. Compute `has_passkey` from Postgres at password login; the frontend may keep a copy in `sessionStorage`. Redis reconnect `DeletePrefix("kirivers:")` logs every admin out; that is expected, not a reason to persist sessions in Postgres.
