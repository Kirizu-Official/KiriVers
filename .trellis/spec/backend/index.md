# Backend Development Guidelines

> Best practices for backend development in this project.

---

## Overview

Code-specs for the Go API process: Gin, GORM/PostgreSQL, `storage.Backend`, `cache.Store`, Zerolog, Viper.

---

## Guidelines Index

| Guide | Description | Status |
|-------|-------------|--------|
| [Directory Structure](./directory-structure.md) | Module organization and file layout | Filled |
| [Build Info](./build-info.md) | `internal/buildinfo` -X symbols and `/api/v1/admin/build-info` contract; `KIRIVERS_{VERSION,BUILD_COMMIT,BUILD_TIME}` env, the three places that must spell symbols identically, fallback + injection test points | Filled |
| [Server Release](./server-release.md) | Alpine Hub image, musl linux, D8/D9 tags, manual-only GitHub publish, 10-target zero-QEMU matrix, changelog format + type table, CI sandbox + two-half security gate, registry push routes | Filled |
| [Database Guidelines](./database-guidelines.md) | GORM AutoMigrate, jobs SKIP LOCKED | Filled |
| [Storage Guidelines](./storage-guidelines.md) | LocalFS/S3 Backend, Range, Presign, `/health` `ready` probe | Filled |
| [Cache Guidelines](./cache-guidelines.md) | memory/Redis Store, fail-closed Ping, runtime failover, invalidation | Filled |
| [Error Handling](./error-handling.md) | `pkg/response` envelope and infrastructure codes | Filled |
| [Quality Guidelines](./quality-guidelines.md) | Viper BindEnv, TLS, OpenAPI copies, trusted_proxies, admin last-login JSON | Filled |
| [Client Check Contract](./client-check-contract.md) | POST check + GET changelog/:channel/:os/:arch + POST pack: fileset hash, packs_ready_at, no request-path zip | Filled |
| [Announcement Contract](./announcement-contract.md) | Dedicated admin CRUD + client GET; seven scopes; scheduled; locale leftover; CDN ETag | Filled |
| [Project Admin Identity](./project-admin-contract.md) | Display `name` vs slug; list/get `stats`; store listings CRUD (not `store_protocols`); version list filters vs `latest=true`; line-defaults; channels | Filled |
| [Logging Guidelines](./logging-guidelines.md) | Five streams, ConsoleWriter, lumberjack, `mod`/`cat`/`plane` | Filled |
| [Telemetry Privacy](./telemetry-privacy.md) | device_id hashing, downgrade read model, rate limits | Filled |
| [Store Adapter Contract](./store-adapter-contract.md) | listing-slug feed path; hash enclosures; `store_full` for `line_full`; visibility, 404/auth/ETag rules | Filled |

---

## Pre-Development Checklist

Before editing Go under `cmd/`, `internal/`, or `pkg/`:

1. [Directory Structure](./directory-structure.md) — package boundaries (`controller` → `service` → `repository`). Before editing `dev/build/`, `deploy/compose.yml`, or forge workflows, also read [Server Release](./server-release.md) (musl Alpine, D8/D9, dispatch-only publish, no QEMU, changelog type table).
2. [Quality Guidelines](./quality-guidelines.md) — BindEnv, per-plane OpenAPI, TLS, `trusted_proxies`, admin `static_dir` / NoRoute UI hosting, admin last-login JSON.
3. [Error Handling](./error-handling.md) — `pkg/response` envelope only.
4. [Logging Guidelines](./logging-guidelines.md) — which YAML / stream / `mod`.
5. Domain contracts as needed: [Client Check](./client-check-contract.md), [Announcements](./announcement-contract.md), [Project Admin Identity](./project-admin-contract.md), [Store Adapter](./store-adapter-contract.md), [Storage](./storage-guidelines.md), [Cache](./cache-guidelines.md), [Telemetry](./telemetry-privacy.md), [Database](./database-guidelines.md).
6. Shared: [Thinking Guides](../guides/index.md).

---

## Quality Check

- [ ] `go test ./...` and `go vet ./...`
- [ ] Default branch has no language SDK package tree under `sdk/`; OpenAPI snapshots for SDKs live only on `sdk/<lang>` (`directory-structure.md`, `quality-guidelines.md`)
- [ ] New public routes in the owning plane spec (`openapi.admin.json` or `openapi.client.json`); schema property names match handler JSON (`quality-guidelines.md` — route test is not enough; run `TestOpenAPIForbiddenNames` / `TestOpenAPIFieldTables` with `TestOpenAPIRoutesSync|TestPlaneSpecsValid`)
- [ ] Official user docs stay in `website/` (VitePress); `docs/` is internal; `deploy/compose.yml` is the user Compose; `dev/docker/compose.yml` is developer databases only; `dev/build/` is the Alpine image + the whole release toolchain as one Python CLI (`check-workflows` is its gate) (`directory-structure.md`, `server-release.md`)
- [ ] Server publish: pull requests do not tag or `docker push`; `release.yml` is `workflow_dispatch` only (merging to main publishes nothing); linux Hub binaries are musl; `deploy/compose.yml` (the only pack that runs the process) passes the URL-signing env; no host 5432/6379 (`server-release.md`)
- [ ] CI security: no `pull_request_target`/`workflow_run` job checks out or runs PR code (the Gosec scan lives on `pull_request`, promotion on `workflow_run`); no `continue-on-error` on a blocking gate; no `|| echo` after an attempted registry push; every `run:` block takes requester-controlled values through `env:`; every workflow file has `check-workflows` assertions (`server-release.md`)
- [ ] No `c.JSON` error objects outside `pkg/response`
- [ ] New config keys `BindEnv` or `SetDefault` (`trusted_proxies` is per-plane, not `config.yaml`; admin `static_dir` is admin-Viper-only, never `KIRIVERS_CLIENT_STATIC_DIR`)
- [ ] Admin UI hosting uses NoRoute only (no `gin.Static` / `StaticFS`); `/api` stays JSON `NOT_FOUND` except D9 reverse-proxy of `/api/v1/projects/**` when `ClientProxyURL` is set (`quality-guidelines.md`, `error-handling.md`)
- [ ] Gray gate is version allowlist + `gray_completed_at` (not HMAC/`rollout_percent`); incomplete gray → `Cache-Control: private`; feeds use `GrayIsComplete` (`client-check-contract.md`, `store-adapter-contract.md`)
- [ ] GORM string columns are `type:text`; leftover varchar is ALTERed; unique indexes that should be per-project include `project_id` on the same GORM `uniqueIndex` name (`database-guidelines.md`)
- [ ] New Gin engines call `ApplyTrustedProxies` (empty list → `SetTrustedProxies(nil)`, never Gin default trust-all)
- [ ] Admin last-login fields: OpenAPI `Admin` matches `publicAdmin`; login uses `UpdateLastLogin` map, not `Save`
- [ ] Pending `second_factor` login JSON has boolean `has_passkey`; omit on other stages, complete, and password 401; do not cache the flag (`quality-guidelines.md`)
- [ ] Logs: correct plane file, stream, and `mod`
- [ ] Cache: `cache.Open` fail-closed on redis Ping; `/health` `ready` still DB+storage only; catalog writes call `InvalidateProject`; admin sessions use `SetWithTTL` (`cache-guidelines.md`)
- [ ] Database: `database.Open` fail-closed before listen; runtime Watch 404 + reconnect, not process exit (`database-guidelines.md`)
- [ ] Announcements stay a dedicated resource: seven scopes, UUID version bind, one language per row; explicit locale is strict; omitted locale leftover (`announcement-contract.md`)
- [ ] Admin project `name` is not a `project_ref`; list/get `stats` are batched (no N+1, no device hashes, reuse-safe `storage_bytes`) (`project-admin-contract.md`)
- [ ] `GET .../versions?latest=true&os=&arch=` returns `{ versions, latest }` via `CompareVersions` over published+ready lines, never `SelectTarget`; filtered list without `latest` omits `latest` (`project-admin-contract.md`)
- [ ] Check hop uses only `min_source_version`; wrong `X-Channel-Token` is not 403; line `min_os`/`min_api_level` (no matrix columns) (`client-check-contract.md`)
- [ ] Project languages: `{ languages: [] }` envelope; `LANGUAGE_TAKEN` / `LANGUAGE_NOT_FOUND`; create requires `default_locale`; client `GET /languages`; changelog editors must not drop extra map keys (`project-admin-contract.md`, `announcement-contract.md`)
- [ ] Markdown media: admin POST `file[]` → `storage.Backend` keys `media/…`; client GET is UUID-public (`ClientProjectResolve`, no token/urlsign); stored Markdown keeps `${site_url}`; client JSON expands from Referer (`storage-guidelines.md`, `announcement-contract.md`)
- [ ] GeoIP files use the **private** Backend keys `geoip/{id}/…` (Get → bytes → `geoip2.FromBytes`; Lookup is memory-only; Reload skips unchanged Head); persist `country_code`/`region_code`/`geo_i18n`, not `country_name`; DELETE client ≠ privacy hash wipe (`storage-guidelines.md`, `telemetry-privacy.md`)
- [ ] Native `package_url` is `/packages/{64-hex}` via `PackageDownloadURL` unless cluster S3-direct emits a public `{slug}/{sha256}` object URL; unsigned JSON `file_name`; download lookup is SHA-256 only; `/artifacts/{id}/{filename}` unregistered; media UUID GET unchanged (`client-check-contract.md`)
- [ ] Artifact `StorageKey` is `{slug}/{sha256}`; inbound bytes stay on `{local.root}/{slug}/temp/{uuid}`; `PresignUpload` is `direct_s3=false` (no unknown-key PresignPut); GeoIP never writes the public bucket; local-proxy `/packages` Get prefers the replica (`storage-guidelines.md`)
- [ ] `ClusterActive` is `storage.driver=s3` **and** `cache.driver=redis`; `admin.enabled=false` skips the admin listener only; register node whenever `local.root` is set; `worker.SetNodeID` **before** `worker.Start` **only when cluster** (`quality-guidelines.md`, `database-guidelines.md`)
- [ ] Platform vs project members: `RequireInstanceAdmin` is `is_platform_admin`; foreign project 404 `PROJECT_NOT_FOUND`; `/admins`, `/geoip`, `/nodes`, and project `node-sync` 403 for project-only; last owner 400 `LAST_OWNER` (`project-admin-contract.md`, `error-handling.md`)
- [ ] Multi-file packs: canonical unordered `fileset_sha256`; `POST /update/pack` (enqueue and poll the same URL; no admin `job_id`); D6/D7 are uncompressed Manifest sums → 200 `full_package`; HTTP/`service/update` stay zip-free; `packs_ready_at` gates check/feed (client jobs do not stamp); `line_full` uses `store_full` never hash-root `full` (`client-check-contract.md`, `store-adapter-contract.md`, `directory-structure.md`)
- [ ] Store listings: `{listings:[]}` CRUD under the project; duplicate `(protocol, slug)` is 400; HTTP is `/store/{protocol}/{listing_slug}/{doc}`; listing JSON `store_url`; missing listing is plain 404 (`project-admin-contract.md`, `store-adapter-contract.md`)
- [ ] Native check 200 has no `changelog` / `changelog_versions`; leftover GET `/update/check` is 404; leftover `POST /clients/login` is 404; `POST /clients/report` 200 is only `ip` + geo (no roster wrapper); changelog lives on `GET /changelog/:channel/:os/:arch` (`client-check-contract.md`, `telemetry-privacy.md`)
- [ ] Official server `CGO_ENABLED=1`; `CGO_ENABLED=0 go build .` fails. hdiffpatch Diff is uncompressed `HDIFF13&` via `internal/delta/hdiffc` (not PATH `hdiffz`, not `KVDIFFHP1` generate). Patch by magic; `engines[].implementation` is `cgo` \| `pure-go` (`directory-structure.md`, `client-check-contract.md`)
- [ ] `file_list.max_files` BindEnv default 16; project `file_list_max_files` 0=inherit; Diff uses catalog effective max; PATCH over ceiling is 400 `INVALID_REQUEST` (`quality-guidelines.md`, `client-check-contract.md`, `project-admin-contract.md`)
- [ ] `changelog.default_entries` / `max_entries` BindEnv 5/50; after `<1` fallback, default > max fails `config.Load` (no listen). Project PATCH over the matching instance ceiling or default > effective max is 400 `INVALID_REQUEST`; dirty rows clamp at catalog read (`client-check-contract.md`, `project-admin-contract.md`)
- [ ] Install-policy templates are a dedicated admin resource (not project GET/PATCH). Overlay is path-wise channel-over-project on the same `(os, arch)`. Stamp only **new** Manifests; ignore `_keep.json` / `keep_if_exists.txt`; reference latest is channel-scoped `CompareVersions`, not `versions?latest=true`. Unknown matrix pair is 400 `INVALID_REQUEST` (`project-admin-contract.md`, `database-guidelines.md`, `error-handling.md`)

---

**Language**: All documentation should be written in **English**.
