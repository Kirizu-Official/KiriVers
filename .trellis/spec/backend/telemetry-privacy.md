# Telemetry Privacy, Downgrade and Rate Limits

> Executable contract for device_id handling, telemetry reporting, the consecutive-failure downgrade read model, and public API rate limiting (§10.4, §13.11–12, §14).

---

## Admin list-card telemetry and client registry

`GET /api/v1/admin/projects` (and get-by-ref) may include `stats.telemetry.installed_24h` / `failed_24h`: **event counts** in `TelemetryDowngradeWindow` (24h), batched by `project_id`. Also `stats.active_7d` = distinct registry rows with `last_check_at` in 7 days. Do not add `device_id` / hash lists or other statuses to the **list-card** telemetry object. The project **list UI** must not render Token counts or 24h install bars (`project-admin-contract.md`). Admin **clients** list/get may show `device_id` as the stored hash (hashed policy) or raw (raw policy) — still never log the raw query value.

## Privacy invariants (never break these)

1. Raw `device_id` NEVER enters application logs (zerolog). Log association uses `Fingerprint` = first 8 hex chars of the device hash. Regression: `TestTelemetryReportNoPlaintextInLogs` captures the full log buffer. Check/report upserts must follow the same rule.
2. `Project.DeviceSecret` (HMAC key for the hashed policy) is `json:"-"`, generated at creation (BeforeCreate fallback), never returned by ANY admin/public endpoint. Never `c.JSON` a raw `model.Project` — use explicit field lists.
3. Storage policy per project: `hashed` (default; `hex(HMAC-SHA256(secret, raw))`), `raw` (discouraged), `none` (DeviceHash stored empty; device-based telemetry downgrade DISABLED; **do not insert** a `clients` row — same as missing `device_id`; gray cannot admit from an empty namebook and therefore completes).

## Telemetry report

- `POST /projects/:ref/telemetry/report` → always 202 on accepted (missing device_id allowed), 400 on invalid status enum. Telemetry failure must never block the update protocol: a DowngradeSource query error is treated as "no downgrade".
- Retention: `Project.TelemetryRetentionDays` (default 90); lazy cleanup on ~1/50 inserts (`DELETE ... WHERE created_at < cutoff`) — stateless, no timer job.
- Privacy deletion: `DELETE /admin/projects/:ref/telemetry/devices/:device_hash` (project-scoped, idempotent 204). Full audit events belong to the signed-url-audit task.

## Privacy wipe vs registry delete

Do not merge these. Privacy wipe is hash-keyed and **keeps** the `clients` row. Registry delete is UUID-keyed and **drops** the row. Neither should log raw `device_id`.

| | Privacy wipe | Registry delete |
|---|---|---|
| Path | `DELETE /api/v1/admin/projects/:ref/telemetry/devices/:device_hash` | `DELETE /api/v1/admin/projects/:ref/clients/:client_id` |
| Telemetry events | deleted | **kept** |
| Gray allowlist (all versions, this project) | deleted | deleted |
| `clients` row | **kept** | deleted |
| Unknown | 204 idempotent `(0,0)` | 404 `CLIENT_NOT_FOUND` |
| Audit | privacy / telemetry | `client.delete` |

`GET .../clients/:id` unknown is 404 `NOT_FOUND` via `writeGrayErr`. Do not silently unify GET with `CLIENT_NOT_FOUND`.

## GeoIP fields on upsert

Report/check upsert writes `country_code`, `region_code`, `geo_i18n` JSONB only when `last_ip` changes. Private / unspecified / lookup miss → empty codes and `{}`. Admin list/stats return the map (not a single `country_name`). No bulk re-geocode after swapping MMDB files. GeoIP maps are not a substitute for `Fingerprint` in logs.

## Scenario: Client geo report (not login, not registry dump)

### 1. Scope / Trigger

Native clients send presence + platform so the namebook and GeoIP stay current. The HTTP 200 must not leak the registry row. Leftover `POST /clients/login` is unregistered (404, no 301).

### 2. Signatures

```
POST /api/v1/projects/{project_ref}/clients/report
```

Body: existing `ClientLoginInput` (`device_id` required; `version` / `os` / `arch` / `channel` / `custom` optional). Handler still calls `ProjectService.LoginClient`.

### 3. Contracts

- 200 is a flat object only: `ip` (`c.ClientIP()`, same value written to `last_ip`), `country_code`, `region_code`, `geo_i18n`. No `client` wrapper. No `id` / `device_hash` / `custom` / other roster fields.
- `device_id_policy=none` → 400 (do not insert a `clients` row).
- Rate-limit key remains device + IP (same as leftover login).
- OpenAPI Auth tag describes report, not login. Client OpenAPI must not `$ref` the admin roster schema for this path.

### 4. Validation & Error Matrix

| Condition | Status | Code |
|-----------|--------|------|
| Valid device + hashed/raw policy | 200 | geo object; upsert still happens |
| `device_id_policy=none` | 400 | `INVALID_REQUEST` (or the live none-policy code; do not 200) |
| Leftover `POST /clients/login` | 404 | unregistered |
| Missing `device_id` | 400 | bind error |

### 5. Good/Base/Bad Cases

- Good: 200 keys are exactly the four geo fields; subsequent admin clients GET shows the upserted row.
- Base: GeoIP miss → `""` / `{}`; `ip` still the request ClientIP.
- Bad: wrapping `{ "client": publicClientRow }`; logging raw `device_id`.

### 6. Tests Required

- `TestReportEndpointGeoOnly`: 200 shape; login 404; none policy 400; upsert/GeoIP still written.

### 7. Wrong vs Correct

#### Wrong

```go
response.JSON(c, 200, gin.H{"client": publicClientRow(row)})
```

#### Correct

```go
response.JSON(c, 200, gin.H{
    "ip": c.ClientIP(), "country_code": row.CountryCode,
    "region_code": row.RegionCode, "geo_i18n": row.GeoI18n,
})
```

## Consecutive-failure downgrade (C11-8)

- `DowngradeActive`: within a 24h window for `(project, os, arch, deviceHash)` — channel EXCLUDED (clients may cross channels): `failed >= 3` AND the latest `installed` (if any) precedes the 3rd-newest `failed`. One `installed` restores normal serving.
- check: `delta_available &&= !DowngradeActive`; diff: short-circuit to `full_package` after the gray gate, before delta selection.
- Downgrade state is DEVICE-SPECIFIC: it must NOT enter the catalog ETag or change Cache-Control (same exclusion rule as gray/device_id).

## Rate limits (§14)

- `internal/middleware/ratelimit.go`: in-process 256-shard sliding window (multi-replica = per-instance quota, documented). Key namespaces are distinct: `ip:{addr}`, `dev:{project}:{deviceHash}`, `ci:{tokenFingerprint}` — CI quota exhaustion must never throttle anonymous public traffic from the same IP, and admin APIs are unlimited.
- Defaults come from `model.DefaultRateLimit()` jsonb keys; project override via `RateLimitFor(project, key)`; **0 = explicitly unlimited**, invalid/negative falls back to default.
- `store_per_ip_per_minute` (default 300, `RateLimitKeyStorePerIP`) is the **shared public-GET IP bucket**: store protocol documents, changelog, integrity, client channel/matrix catalogs, announcements, and media. Lowering it throttles all of those, not only `/store/`. Startup rewrites leftover jsonb `feed_per_ip_per_minute` to this key (`renameStoreSurface`). Do not add a second IP bucket named like “changelog_per_ip” unless the product splits quotas.
- The limit check runs BEFORE the 304 short-circuit — a 304 consumes quota (§14 "HTTP 304 仍计入限额").
- 429 response: `RATE_LIMITED` + integer `Retry-After` + `Cache-Control: private, no-store`.

## Wrong vs Correct

#### Wrong

```go
logger.Info().Str("device_id", raw).Msg("telemetry") // plaintext leak
```

#### Correct

```go
logger.Info().Str("device", telemetry.Fingerprint(hash)).Msg("telemetry")
```
