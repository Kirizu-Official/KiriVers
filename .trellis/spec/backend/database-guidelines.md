# Database Guidelines

> GORM + PostgreSQL. No Redis for jobs. Schema changes go through `model.AutoMigrateModels`.

---

## Overview

- Driver: `gorm.io/driver/postgres`.
- Connection: `internal/database.Open(dsn, log)` only. DSN comes from `internal/config` (`postgres.dsn` / `KIRIVERS_POSTGRES_DSN`). The logger is the process system child with `mod=db` (CLI may pass `zerolog.Nop()`).
- Server **must not listen** if `Open` fails: log `mod=db` then `fatal` (stderr + exit 1), same as Redis `cache.Open`. CLI also requires a live DB.
- Runtime (after a successful Open): keep `*gorm.DB` (pool reconnects). `database.Watch` pings every `DefaultReconnectInterval` (5s). While ping/connect fails, `/api` except `/health` `/openapi.json` returns 404 `NOT_FOUND` (same as NoRoute when routes were never mounted). `/health` stays 200 with `ready=false`. Do not exit the process. Admin UI static files are not 404’d.
- UUID primary keys are generated in `BeforeCreate` (application-side), not via PostgreSQL `uuid-ossp` defaults. Project IDs must be RFC 4122 **v4** (`uuid.NewRandom()`), not a version-unspecified `uuid.New()`.

---

## Scenario: Jobs table + SKIP LOCKED workers

### 1. Scope / Trigger

Any new async work (bundle unpack, delta, webhook) must use the `jobs` row + in-process workers, not Redis or a second queue.

### 2. Signatures

Table `jobs` (`model.Job`):

| Column | Type | Notes |
|--------|------|--------|
| id | uuid PK | app-generated |
| type | text | e.g. `bundle_unpack`; indexed |
| status | text | `queued` \| `running` \| `succeeded` \| `failed` |
| payload | jsonb | handler-owned |
| progress | int | 0–100 |
| error_message | text | empty on success |
| attempts | int | claim count |
| next_attempt_at | timestamptz nullable | delayed requeue for retry backoff (e.g. webhook_deliver 1m/5m); Claim filters `next_attempt_at IS NULL OR <= now` — rows with NULL claim immediately, so legacy job types need no backfill |
| project_id | uuid nullable | **no FK** (keep nullable even though `projects` exists) |
| owner_node_id | uuid nullable | **no FK**; admin jobs stamp the requesting node; `dynamic_pack` stays NULL |
| started_at / finished_at | timestamptz nullable | |
| created_at / updated_at | timestamptz | claim index `idx_jobs_claim` is partial: `(created_at ASC) WHERE status = 'queued'`; terminal states do not enter the btree |

Claim: `SELECT ... FOR UPDATE SKIP LOCKED` in `repository` (not in controller). Filter: `owner_node_id IS NULL OR owner_node_id = this node` **only when `ClaimFilter.NodeID != uuid.Nil`**. A zero NodeID **omits** the owner predicate and can steal another node's `auto_delta`. When `ClaimFilter.Types` is non-empty, `type IN (...)`. Empty Types still claims unknown types so the worker can Release. Client-only processes register only `dynamic_pack` (plus any process-local types). Idempotency lookup is a non-unique composite btree `(project_id, idempotency_key, created_at DESC)` for 24h window lookup, **never** a lifetime UNIQUE constraint.

`cmd/server.go` registers `{local.root}/.kirivers-node-id` and `projects.SetNodeID` whenever `local.root` is set (singleton UI `is_current`). Call `worker.SetNodeID` **only when `ClusterActive`** and **before** `worker.Start`. `SetNodeID` is mutex-guarded; Claim reads it under `RLock`. A singleton worker keeps `NodeID=uuid.Nil` on purpose: it is the only Claim loop on that database. Do not run `ClusterActive=false` workers against a Postgres that other cluster nodes already own.

### 3. Contracts

- Env: `jobs.workers` / `KIRIVERS_JOBS_WORKERS` (default 2).
- Unknown `type`: worker **must Release** the row back to `queued` (or equivalent) and sleep. Never leave a claimed row stuck in `running` with no handler.
- HTTP handlers enqueue rows; they do not unpack zips or hash blobs on the request goroutine.
- Cluster nodes: do not start the Claim loop until this process UUID is in `nodes` and on the worker. Admin jobs stamp `owner_node_id` at enqueue; `dynamic_pack` stays NULL.
- Delayed retry: `JobStore.RequeueAfter(row, d)` sets `next_attempt_at` and returns the row to `queued`; use for bounded-retry delivery jobs (webhook: 3 attempts then terminal `failed` in the delivery table, never in `jobs.status`).

### 4. Validation & Error Matrix

| Condition | Behavior |
|-----------|----------|
| No queued rows | sleep / idle loop |
| Unknown type | Release; do not mark succeeded |
| DB nil | do not start workers (HTTP tests / memory stores) |
| `worker.Start` before `SetNodeID` on a **cluster** process | Claim omits owner filter; replica can steal `auto_delta` |
| `ClusterActive=false` worker on a shared cluster DB | Same: Nil NodeID omits owner filter |

### 5. Good/Base/Bad Cases

- Good: two server replicas both `SKIP LOCKED`; a row is claimed once. A replica with only `dynamic_pack` cannot claim `auto_delta` owned by another node.
- Base: empty type registry → workers idle without CPU spin.
- Bad: `SELECT * FROM jobs WHERE status='queued' LIMIT 1` without `SKIP LOCKED` (double claim).

### 6. Tests Required

- Unknown type is released, not left `running`.
- Claim uses `SKIP LOCKED` (repository test or SQL assertion).
- Replica with only `dynamic_pack` cannot claim `auto_delta` owned by another node (`jobs_test.go`).
- `database.Open` empty DSN fails. `Watch`: ping fail → `Available=false`; ping ok → true. HTTP: `DBAvailable=false` → business `/api` 404 `NOT_FOUND`, `/health` 200 with `ready=false`.

### 7. Wrong vs Correct

#### Wrong

```go
db.Where("status = ?", "queued").First(&job) // races across replicas
```

#### Correct

```go
if clusterActive {
    worker.SetNodeID(row.ID) // before Start; ClaimFilter.NodeID != uuid.Nil
}
db.Clauses(clause.Locking{Strength: "UPDATE", Options: "SKIP LOCKED"}).
    Where("status = ?", model.JobStatusQueued).
    Where("owner_node_id IS NULL OR owner_node_id = ?", nodeID).
    Order("created_at ASC").
    First(&job)
```

---

## Migrations

Register every new entity in `model.AutoMigrateModels()` **in FK order** (referenced tables first). Current order in `internal/model/migrate.go`: Admin → **ProjectMember** → **GeoipDatabase** → AdminTOTP → AdminPasskey → AdminRecoveryCode → Project → **StoreListing** → **ProjectLanguage** → ProjectSlugAlias → ProjectToken → CIToken → Version → VersionLine → Announcement → ProjectMedia → Channel → PlatformMatrix → **InstallPolicyRule** → HwRev → Job → Artifact → UploadSession → ManifestEntry → TelemetryEvent → **Client** → **ClientDailyStats** → GrayAllowlist → **GrayRolloutSnapshot** → WebhookDelivery → AuditEvent → **Node** → **NodeArtifactSync**. `database.AutoMigrate` is add-column/add-table by default. Column, jsonb key, and `kind` **renames** run as SQL extras **before** `db.AutoMigrate` (`renameStoreSurface`: `projects.feed_token_hash` → `store_token_hash`, `artifacts.kind` `feed_full` → `store_full`, `rate_limit.feed_per_ip_per_minute` → `store_per_ip_per_minute`). GORM does not RENAME; AutoMigrate first would ADD an empty `store_token_hash` and leave the hashed token in the old column. Named `Migrator().DropColumn` after AutoMigrate is allowed when a task forbids a compatibility shim (no unused leftover column). Named drops today: `projects.min_client_protocol`, `projects.default_rollout_percent`, `projects.store_protocols` (replaced by `project_store_listings`; no jsonb backfill); `versions.required_intermediate_version` (hop is `min_source_version` only), `versions.rollout_percent`, `versions.gray_salt`; `platform_matrix.min_os` and `platform_matrix.min_api_level`; `version_lines.rollout_percent_override`; `gray_allowlist.version_line_id`. After AutoMigrate, `alterLeftoverVarcharToText` converts remaining `varchar`/`character` columns to `text` (no dual-read). Application validators still cap runes/bytes (`MaxProjectNameRunes`, `MaxDeviceIDRunes`, login `custom` 16 KiB).

`clients` uniqueIndex `idx_clients_project_device` is `(project_id, device_hash)` — `project_id` must be on the same GORM index name. Composite btrees on `(project_id, last_os)`, `(project_id, last_arch)`, `(project_id, last_version)`, and `(project_id, country_code)` support equality filters. Search: GIN `custom jsonb_path_ops`, `to_tsvector(custom::text)`, `pg_trgm` on last_version/os/arch/channel/last_ip. Persist fused GeoIP as `country_code`, `region_code`, and `geo_i18n` JSONB (`{country,region}` locale maps) — **not** `country_name`. `gray_allowlist` unique is `(version_id, device_id)` (version-level only); composite `(project_id, device_id)` (`idx_gray_allowlist_project_device`) supports privacy deletion. `telemetry_events` has `idx_telemetry_privacy (project_id, device_hash)`.

`project_members` unique is `(project_id, admin_id)`; role `owner`|`admin`; no FK. `admins.is_platform_admin` bool NOT NULL DEFAULT true. Platform admins do not need a membership row.

`geoip_databases` is a platform table (no project FK): name, file_name, storage_key, size, enabled, rank. Cap 8 rows / 256 MiB each in the service.

`channels` rows have `name` (required display), `unlisted`, `token_hash` (`json:"-"`), and `token_plain` (admin JSON `token`). System seeds snapshot names at project create. Slug is the identity and is not updated on PATCH.

`project_languages` uniqueIndex `idx_project_languages_project_code` is `(project_id, code)` — `project_id` must be on the same GORM index name. That index is case-sensitive; AutoMigrate also creates `idx_project_languages_project_code_ci` on `(project_id, lower(code))`, and `idx_project_languages_one_default` unique `(project_id) WHERE is_default` in SQL extras to ensure at most one default language per project. Cap 16 rows per project (`ProjectMaxLanguages`). After AutoMigrate, backfill a default row from `projects.default_locale` (invalid/empty → `en` for **legacy** rows only) plus harvested announcement/changelog JSON keys.

`project_store_listings` uniqueIndex `idx_store_listings_project_protocol_slug` is `(project_id, protocol, slug)` — `project_id` must be on the same GORM index name. `slug` uses channel rules (`[a-z0-9-]{3,64}`, no underscore). Duplicate is HTTP 400 `INVALID_REQUEST`, not 409. No production backfill from dropped `store_protocols`.

`projects.name` is `text NOT NULL DEFAULT ''` (console title; not unique; not a resolver key; 128-rune cap in the service). After AutoMigrate, backfill `UPDATE projects SET name = slug WHERE name = ''` so existing rows are not blank in the admin UI. Reads still fall back empty name → slug (`DisplayName()`). Do not add `name` to client `ProjectPublic`. `gray_weight_tenure_activity bool NOT NULL DEFAULT true`.

`Job.ProjectID` stays nullable **without** a FK. `Job.OwnerNodeID` is the same. `AuditEvent` also soft-references Project (no FK).

`install_policy_rules` is a template table (no FK): unique `(project_id, channel_id, os, arch, path)` on GORM index `idx_install_policy_rules_scope_path`. `channel_id` is **NOT NULL**; `uuid.Nil` means project scope (PostgreSQL 14 cannot unique NULL). Cap 256 rows per `(project_id, channel_id, os, arch)`. Rule writes do **not** `InvalidateProject`. Custom channel delete removes rows with that `channel_id`. `project_media` has `sha256`/`md5`/`sha512` `type:text not null default ''` filled on new `Put` only (no backfill).

`nodes` is a platform table (no project FK): UUID PK from `{storage.local.root}/.kirivers-node-id`, optional `display_name`, `admin_enabled`, heartbeat CPU/memory/`last_seen_at`. **Register + heartbeat whenever `storage.local.root` is set**, including singleton; `worker.SetNodeID` and `SetOnBeat` (replica pull) stay **cluster-only** (`ClusterActive` = `storage.driver=s3` AND `cache.driver=redis`). List JSON is `{ cluster_active, nodes }`; each node has `is_current` for this admin process. Page mode is `cluster_active`, never `len(nodes)>1`. Heartbeat `cpu_percent` is currently always `0`; `mem_used_bytes` / `mem_total_bytes` are Go `HeapAlloc` / `Sys`, not OS RSS. Do not treat list JSON as host telemetry until a sampler exists. `node_artifact_sync` unique scope cannot use GORM composite unique tags because `line_id` is nullable (PostgreSQL standard UNIQUE treats multiple NULLs as distinct); uniqueness is enforced in SQL extras via expression index `idx_node_artifact_sync_scope (node_id, version_id, COALESCE(line_id, '00000000-0000-0000-0000-000000000000'::uuid))`. Soft UUID refs, no FK. Offline badge: `now - last_seen_at > 30s`. No auto-delete.

`versions` is a real table named `versions` (not a lock flag on `projects`). It already has dual version numbers, channel, changelog, gray, LTS/critical, and lifecycle fields — do not create a second version table. Composite `idx_versions_project_status (project_id, status)` covers published/ready/status queries. Single-column indexes on `channel_id`, `channel_slug`, `status`, and non-unique version numbers are dropped; unique indexes on integer and semver must always include `project_id`.

`version_lines` is a real table named `version_lines` (`version_id`, canonical `os`/`arch`, status, `min_os`, `min_api_level`, notes, rollout override, root_hash, **`packs_ready_at`**). `packs_ready_at` is set when system `auto_delta` preheat (channel × `delta_source_count`) succeeds for that line (empty sources / skip / discard count as success). Nil stamp → check/store/specified pack cannot see the line. Client `dynamic_pack` Jobs must not write this column. `PACKAGE_TYPE_IMMUTABLE` is a SQL join: `versions.status=published` **and** a line for that canonical `(os,arch)` — not a boolean on `platform_matrix`. Store line os/arch with `CanonicalOSWrite` / `CanonicalArch` (darwin→macos, ipados stays `ipados`). `CanonicalOS` is for check/CI lookup and would rewrite `ipados`→`ios`. Do not rename `version_lines`. Do not put min OS/API back on `platform_matrix`.

`artifacts` hot-path indexes: composite `idx_artifacts_line_kind (version_line_id, kind)`, `idx_artifacts_project_sha256 (project_id, sha256)`, and `idx_artifacts_project_file_name (project_id, file_name)`. Redundant single-column indexes on `file_name`, `sha256`, `fileset_sha256`, `version_line_id`, and `project_id` are dropped (`fileset_sha256` matching narrows by line+kind first and compares in memory). Columns used by multi-file packs: `fileset_sha256` (text; canonical unordered Manifest set), `compression` (`zip`), `kind` including `store_full` (path zip for store `line_full`, SHA distinct from native hash-root `full`). AutoMigrate adds the columns; do not put fileset identity in Redis.

Slug uniqueness is one domain: live `projects.slug` plus **unexpired** `project_slug_aliases.slug`. Resolver: UUID, else live slug, else unexpired alias. Expired alias → `PROJECT_NOT_FOUND`. Changing slug does **not** HTTP 301.

---

## Naming Conventions

- Explicit `TableName()` on every entity (`admins`, `admin_totp`, `admin_passkeys`, `admin_recovery_codes`, `project_members`, `geoip_databases`, `projects`, `project_store_listings`, `project_languages`, `project_slug_aliases`, `project_tokens`, `ci_tokens`, `versions`, `version_lines`, `announcements`, `project_media`, `channels`, `platform_matrix`, `install_policy_rules`, `hw_revs`, `jobs`, `artifacts`, `upload_sessions`, `manifest_entries`, `telemetry_events`, `gray_allowlist`, `webhook_deliveries`, `audit_events`).
- JSON tags snake_case; `PasswordHash`, TOTP `secret` / `pending_secret`, recovery `code_hash`, token hashes, and signing private keys use `json:"-"`.
- Status/type constants live next to the model (`JobStatusQueued`, …).

---

## Common Mistakes

### Common Mistake: Start job workers before the node UUID is set

**Symptom**: A second process claims `auto_delta` / bundle jobs stamped for another node, then Release-spins or runs them without admin handlers.

**Cause**: `Claim` applies `owner_node_id IS NULL OR = NodeID` only when `NodeID != uuid.Nil`. `worker.Start` before `SetNodeID` uses the zero UUID.

**Fix**: In `cmd/server.go`, `Register` the node identity file when `local.root` is set, then `worker.Start`. On **cluster** processes also `worker.SetNodeID` before Start so Claim keeps `owner_node_id IS NULL OR = this node`. Singleton skips `worker.SetNodeID` (UI still gets `projects.SetNodeID` / `is_current`); that process must be the only worker on the database.

**Prevention**: Cluster tests and the jobs scenario Wrong/Correct pair above. Do not add a Redis job queue to paper over this. Do not point a `ClusterActive=false` process at a cluster Postgres.

### Common Mistake: Listen when Postgres is down at startup

**Symptom**: Process is "up" but every API is 404 / `NOT_READY`; orchestrators cannot tell misconfiguration from a running node.

**Fix**: `database.Open` error is `fatal` before listen (`mod=db` + stderr). Runtime outages use `Watch` + 404; do not fatal after listen.

**Prevention**: `cmd/server.go` matches Redis: log then `fatal(err)`. Tests cover empty DSN and HTTP 404 while `DBAvailable` is false.

### Common Mistake: Exit the process when Postgres blips at runtime

**Symptom**: A brief network partition kills the node; `/health` is unreachable so the replica is replaced.

**Cause**: Treating runtime `Ping` failure like startup `Open` failure.

**Fix**: After a successful Open, `Watch` marks unavailable, APIs return 404 `NOT_FOUND`, ticker Ping reconnects. `/health` stays 200 with `ready=false`.

### Common Mistake: Version Line stores alias slugs (`darwin`) or uses `CanonicalOS` on write

**Symptom**: PATCH matrix `package_type` after a published line still succeeds, or an `ipados` matrix row never locks because the line was stored as `ios`.

**Cause**: Lock compares matrix canonical `(os,arch)` to `version_lines`. Raw `darwin`/`amd64` does not match `macos`/`x86_64`. `CanonicalOS` maps `ipados`→`ios` unless the matrix already lists `ipados`.

**Fix**: All Version Line writers use `CanonicalOSWrite` + `CanonicalArch` (same as matrix write). `CreateVersionLine` already does this.

**Prevention**: Tests must lock via aliased writes (`darwin`/`amd64`) and prove draft lines / other `(os,arch)` do not lock.

### Common Mistake: Version unique indexes named `project` that omit `project_id`

**Symptom**: Creating the same SemVer (or integer build) in a **second** project returns 500 `INTERNAL_ERROR` instead of 409 `VERSION_ALREADY_EXISTS`. Confirmed live 2026-09-15.

**Cause**: `model.Version` tags are

```go
VersionInteger *int64 `gorm:"index;uniqueIndex:idx_versions_project_integer,where:version_integer IS NOT NULL"`
VersionSemverCanonical *string `gorm:"size:128;index;uniqueIndex:idx_versions_project_semver,where:version_semver_canonical IS NOT NULL"`
```

GORM unique index names include `project` but **only the version column is in the index**. Uniqueness is instance-global, not per `project_id`. Duplicate-key is not mapped to `VERSION_ALREADY_EXISTS`. Comments on the struct still say “unique within a project”.

**Fix** (follow-up backend task, not frontend): composite unique `(project_id, version_semver_canonical)` / `(project_id, version_integer)` with the same partial `WHERE`, and map unique violation to 409 `VERSION_ALREADY_EXISTS`. Until then, admin-UI smoke must use a unique version string per instance.

**Prevention**: Any new `uniqueIndex:` whose name claims a parent scope must list that parent column on the **same** index name. Add a test that two projects may share `1.0.0`.

### Common Mistake: FK from `announcements.version_id` to `versions`

**Symptom**: Version delete is blocked, or orphaned notices for uninstalled builds disappear from client GET.

**Cause**: Draft Version delete must remain possible after announcements bind that UUID. A FK would block delete or cascade-remove notices. The old denormalized `version_ref` string is dropped after flatten.

**Fix**: Nullable `VersionID` **without** FK. Writes resolve `GetVersionByID` and require `v.ProjectID == projectID`. Client match parses query `version` then compares UUIDs; lookup miss skips version-scoped rows. `dropNamedColumns` removes `version_ref`. Operators unpublish/delete stale rows.

**Prevention**: `announcement-contract.md`. AutoMigrate must not add this FK.

### Common Mistake: `Updates(struct)` skips nil `last_login_ip`

**Symptom**: After a login with a blank ClientIP (or a later login that should clear IP), Postgres still shows the previous IP.

**Cause**: GORM `Updates(struct)` omits zero values. A typed `*string(nil)` inside `map[string]any` can also be skipped depending on driver/GORM version.

**Fix**: `AdminStore.UpdateLastLogin` uses `Updates(map[string]any)` with untyped `nil` for SQL NULL, and only puts a string when `ip != nil`. Do not `Save` the whole admin row on login.

**Prevention**: Memory + HTTP tests that persist empty IP as JSON `null`; never assert last-login by loading an in-memory struct that was `Save`d.

### Common Mistake: Rename a GORM field then AutoMigrate first

**Symptom**: After a Go field rename (`FeedTokenHash` → `StoreTokenHash`), existing rows have an empty `store_token_hash` and the old `feed_token_hash` still holds the hash. StoreAuth rejects previously configured tokens. AutoMigrate did not fail.

**Cause**: GORM AutoMigrate only adds tables/columns. It never `ALTER … RENAME COLUMN`. The new struct tag is a new column with the Go zero value.

**Fix**: In `database.AutoMigrate`, run idempotent SQL extras **before** `db.AutoMigrate`: `tableHasColumn` old && !new → `RENAME COLUMN`; then rewrite jsonb keys / `kind` values the same way. `dropNamedColumns` stays **after** AutoMigrate and only for columns the product is deleting, not for renames.

**Prevention**: Any task that renames a persisted identifier must add a before-AutoMigrate extra with existence checks (`renameStoreSurface` in `migrate_extras.go`). Do not rely on AutoMigrate + a later DropColumn of the old name (that drops the data).

### Common Mistake: Memory-store `LoadCatalog` re-locks a held mutex

**Symptom**: A test hangs (e.g. snapshot assembly) with no failure output.

**Cause**: `MemoryUpdateCatalog.LoadCatalog` holds the store mutex for the whole snapshot and delegated to public list methods that lock the same non-reentrant mutex.

**Fix / Pattern**: For every public memory-store method that may be needed during snapshot assembly, extract a lock-free `...Locked` core; the public method locks + delegates, and `LoadCatalog` calls the locked cores under its existing lock. Regression context: version-level gray allowlists are scoped per `VersionID` — assembling them project-wide into every `VersionState` was a real leak caught by an e2e test asserting a device allowlisted on 1.1.0 does not target 1.2.0.

### Common Mistake: GORM unique index on nullable columns (e.g. nullable UUID line_id)

**Symptom**: Inserting multiple rows with `line_id = NULL` (e.g. version-level proxy pull status) succeeds repeatedly without unique constraint violation, resulting in duplicate scope entries.

**Cause**: PostgreSQL standard `UNIQUE` treats `NULL` values as distinct from each other (`NULL != NULL`). GORM's `uniqueIndex` struct tag generates standard `CREATE UNIQUE INDEX ... (node_id, version_id, line_id)` which does not treat multiple `NULL`s as equal.

**Fix**: Remove the GORM `uniqueIndex` tag from the model struct so AutoMigrate does not rebuild the leaky index. Add a SQL expression unique index in `internal/database/migrate_extras.go` with a sentinel UUID:

```sql
CREATE UNIQUE INDEX IF NOT EXISTS idx_node_artifact_sync_scope
  ON node_artifact_sync (node_id, version_id, COALESCE(line_id, '00000000-0000-0000-0000-000000000000'::uuid));
```

**Prevention**: Never use GORM `uniqueIndex` on a composite key where one of the columns can be NULL if business logic requires uniqueness among NULLs; use SQL expression indexes with `COALESCE` in `migrate_extras.go`.

### Common Mistake: Creating full-table index for Queue Claim or lifetime UNIQUE for sliding-window idempotency

**Symptom**: Jobs table bloats index size on finished jobs, or jobs idempotency fails when attempting to reuse an idempotency key outside the 24h window.

**Cause**: GORM struct tags cannot specify partial index `WHERE status = 'queued'`, and making `idempotency_key` unique breaks sliding-window deduplication.

**Fix**: Remove `index:idx_jobs_claim` from GORM model struct; create partial btree index `CREATE INDEX IF NOT EXISTS idx_jobs_claim ON jobs (created_at ASC) WHERE status = 'queued'` in `migrate_extras.go`. For idempotency, use non-unique composite btree `(project_id, idempotency_key, created_at DESC)`.

**Prevention**: Claim indexes must be partial on queued rows; idempotency with a time window must be a normal btree, not UNIQUE.

