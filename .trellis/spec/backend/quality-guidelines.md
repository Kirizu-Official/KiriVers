# Quality Guidelines

> Layering, config, OpenAPI, TLS, tests. Directory layout is in `directory-structure.md`.

---

## Overview

`go test ./...` and `go vet ./...` must pass before a task is committed. New public HTTP routes belong in the owning plane spec: `internal/controller/openapi.admin.json` (`/api/v1/admin/**` plus shared `/health` `/openapi.json`) or `internal/controller/openapi.client.json` (client-plane paths plus the same shared endpoints). Those two files are the only committed OpenAPI JSON; `go:embed` serves them from each plane’s `GET /api/v1/openapi.json`. Do not add `docs/openapi.json`, `internal/controller/openapi.json`, `internal/openapi`, or frontend `OpenApi.json` copies. Docs-site Scalar pages must consume `website/scripts/filter-openapi.mjs` **unfiltered** copies (`website/docs/public/openapi/openapi.client.json` and `openapi.admin.json`, gitignored), not a third committed spec and not four path-sliced files. Ordinary Markdown pages must not embed Scalar.

`TestOpenAPIRoutesSync` only checks HTTP method + path coverage. It does **not** compare schema property names to handler JSON. Field names are gated by `TestOpenAPIForbiddenNames` and `TestOpenAPIFieldTables` (`internal/controller/openapi_fields_test.go`): leftover tokens (`display_version`, `server_protocol`, `min_client_protocol`, `store_protocols`, `protocol_version`, request-body `dirty_paths`, `/feed/` paths) must not appear as OpenAPI `name` / `properties` keys / `paths` keys, and high-churn maps (`publicProject`, listing JSON, client check/diff/pack/changelog 200, pack request) must match live `gin.H` / json tags with `additionalProperties: false`. Success and error bodies in the plane spec must match what Gin actually writes (`response.JSON`, `public*` helpers, bind structs). When names drift (`records` vs `events`, promote wrapper vs naked `Version`, PUT `changelog` map vs `changelog_i18n`), fix the **spec** to the live handler — do not change handler JSON to match a wrong schema. `TestPlaneSpecsValid` checks path partition, shared endpoints, `$ref` closure, and `info.title`. Admin UI files are served from NoRoute only — `gin.Static` / `StaticFS` would register `GET /*filepath` and fail the route test. The route gate is not enough: run the field tests with the route pair.

After a plane-spec edit, run `go test ./internal/controller -run "TestOpenAPIRoutesSync|TestPlaneSpecsValid|TestOpenAPIForbidden"` then `cd frontend && yarn generate:api`.

## Scenario: Per-plane OpenAPI source of truth

### 1. Scope / Trigger

Adding or changing a public HTTP route, request/response JSON field, or the files under `internal/controller/openapi*.json`.

### 2. Signatures

```text
internal/controller/openapi.admin.json   # admin plane + shared probes
internal/controller/openapi.client.json  # client plane + shared probes
//go:embed openapi.admin.json / openapi.client.json
func TestOpenAPIRoutesSync(t *testing.T)
func TestPlaneSpecsValid(t *testing.T)
func TestOpenAPIForbiddenNames(t *testing.T)
func TestOpenAPIFieldTables(t *testing.T)
```

Frontend: `yarn generate:api` → `scripts/fetch-openapi.mjs` injects `operationId` into gitignored `frontend/.openapi/`, then `openapi-ts` writes `src/api/generated/` and `generated-client/`.

### 3. Contracts

- Edit **admin** spec for `/api/v1/admin/**`; edit **client** spec for `/api/v1/projects/**` and other non-admin paths.
- Duplicate only the shared pair in both files: `/api/v1/health`, `/api/v1/openapi.json`.
- Schema property names must match handler JSON (`public*` / bind structs / `response.JSON` keys).
- Admin delta `engines[].implementation` enum is `cgo` | `pure-go` (hdiffpatch is always `cgo`; bsdiff/xdelta3 `pure-go`). Do not restore `official-cli`. This is admin-plane only; do not add it to `openapi.client.json`. After `yarn generate:api`, revert `src/api/generated-client/*` unless the **client** plane spec actually changed (the script rewrites both SDKs).
- Leftover `GET /update/check` and `POST /clients/login` are **unregistered (404)**, not 400. Do **not** list `local_sha256` / `dirty_paths` / `changelog_*` as accepted check parameters. Prefer descriptions that do not re-teach the old name. Create `bootstrap_admin_token` stays a rejected leftover if the handler still 400s it.
- Client OpenAPI store operations and the tags list use `Store`, never `Feed`. `TestOpenAPIForbiddenNames` still bans `/feed/` path keys.
- `TestOpenAPIFieldTables` covers high-churn maps only (`publicProject`, listing JSON, check/diff/pack/changelog 200, pack request). A new `public*` helper with many keys must add a table row. Other `gin.H` routes stay route-gated; `ginHKeys` sees string-literal keys only.
- Conditional keys (`store_token` on rotate) belong in schema `properties` as optional, not omitted.
- Do not recreate a merged master spec. Do not commit frontend OpenAPI JSON snapshots. Official language SDKs on orphan `sdk/<lang>` **may** copy `openapi.client.json` (+ `OPENAPI_REVISION`) at the package root; that copy must not appear on the default branch. After a client-plane spec edit, copy the snapshot into each SDK worktree — do not generate SDK clients from OpenAPI.

### 4. Validation & Error Matrix

| Condition | Result |
|-----------|--------|
| Gin route missing from the plane spec | `TestOpenAPIRoutesSync` fails |
| Spec path not registered on that plane’s engine | `TestOpenAPIRoutesSync` fails |
| Admin spec contains a non-admin, non-shared path | `TestPlaneSpecsValid` fails |
| Client spec contains `/api/v1/admin/**` | `TestPlaneSpecsValid` fails |
| Unresolvable `#/components/...` `$ref` | `TestPlaneSpecsValid` fails |
| Schema field name ≠ handler JSON | `TestOpenAPIFieldTables` fails; fix the spec (not the handler) |
| Leftover token as property / parameter `name` / `paths` key (`display_version`, `server_protocol`, `min_client_protocol`, `store_protocols`, `protocol_version`, `dirty_paths`, `/feed/`) | `TestOpenAPIForbiddenNames` fails |
| Description re-teaches a leftover name | `TestOpenAPIForbiddenNames` fails unless `forbiddenDescriptionExceptions` (prefer rewrite) |

### 5. Good/Base/Bad Cases

- Good: new `POST /api/v1/admin/projects/{project_ref}/store-listings` is added only to `openapi.admin.json`, then `yarn generate:api`.
- Base: both specs include `GET /api/v1/health` with the same `Health` schema name.
- Bad: adding `docs/openapi.json` or `internal/controller/openapi.json`; hand-editing `src/api/generated/`; documenting a 400 leftover query as an OpenAPI parameter so “clients know we reject it”; putting `official-cli` back on `engines[].implementation`.

### 6. Tests Required

- `TestOpenAPIRoutesSync` bidirectional method+path match per engine.
- `TestPlaneSpecsValid` partition, shared paths, `$ref`, titles.
- `TestOpenAPIForbiddenNames` leftover tokens are not property/parameter/path keys; `TestOpenAPIForbiddenNamesWouldFailOnReinsert` proves the gate trips on a fixture that reinserts them.
- `TestOpenAPIFieldTables` Go keys vs resolved OpenAPI properties (`additionalProperties: false`) for `publicProject`, listing JSON, check/diff/pack/changelog 200, pack request.
- `TestOpenAPIJSON` served title and representative present/absent paths.

### 7. Wrong vs Correct

#### Wrong

```text
# edit a merged master, copy to docs/, then
go test ./internal/controller -run TestSpecFilesUpToDate -update
```

#### Correct

```text
# edit internal/controller/openapi.admin.json (or openapi.client.json)
go test ./internal/controller -run "TestOpenAPIRoutesSync|TestPlaneSpecsValid|TestOpenAPIForbidden"
cd frontend && yarn generate:api
```

### Common Mistake: Document a leftover query as an OpenAPI parameter

**Symptom**: `TestOpenAPIRoutesSync` is green, but `TestOpenAPIForbiddenNames` fails, or CDN keys fragment because clients keep sending `dirty_paths` / `changelog_scope` on check.

**Cause**: Treating “handler 400s this leftover” as “the spec should list it so generators know”. Listed parameters become generated SDK fields and cache-key inputs.

**Fix**: Keep the 400 in the handler. Delete the parameter from the plane spec. Rewrite the 400 description so it does not re-teach the old name (`INVALID_QUERY_PARAM` is enough). Fixture `testdata/openapi_forbidden_reinsert.json` proves the gate trips if the name returns.

**Prevention**: After any plane-spec edit run `TestOpenAPIForbiddenNames` with the route pair. Adding a new leftover token to the product delete list means adding it to `forbiddenOpenAPITokens` in the same change.

### Common Mistake: Admin-only enum dirties the client generated SDK

**Symptom**: `git diff frontend/src/api/generated-client/` after an admin-only OpenAPI enum change (e.g. `engines[].implementation`).

**Cause**: `yarn generate:api` always runs both openapi-ts jobs.

**Fix**: Keep admin `src/api/generated/` updates. Revert `generated-client/` when `openapi.client.json` did not change.

**Prevention**: After generate, `git diff -- frontend/src/api/generated-client` and restore if the client plane was untouched.

---

## Scenario: Viper env bind for keys without defaults

### 1. Scope / Trigger

Adding any process config key that may be supplied only via `KIRIVERS_*`.

### 2. Signatures

`config.Load(path string) (*Config, error)` for the HTTP server (three YAML files). `config.LoadSystem(path string) (*SystemConfig, error)` for `kirivers admin` (system file only).

Three Vipers / prefixes: `KIRIVERS_` (system `config.yaml`), `KIRIVERS_ADMIN_` (`admin.yaml`), `KIRIVERS_CLIENT_` (`client.yaml`). Replacer `.` → `_`. Example: `postgres.dsn` → `KIRIVERS_POSTGRES_DSN`; plane `addr` → `KIRIVERS_ADMIN_ADDR` / `KIRIVERS_CLIENT_ADDR`.

`-config` / `KIRIVERS_CONFIG` may be a directory (`config.yaml` + `admin.yaml` + `client.yaml`) or a system file path (siblings `admin.yaml` / `client.yaml`). Empty path searches `configs/` then `.`, each missing file independently falling back to `*-example.yaml`. An explicit path that is missing a required file is a hard error (server needs all three; CLI only system).

### 3. Contracts

Viper `Unmarshal` only walks `AllKeys()`. Nested keys with **no YAML value and no `SetDefault`** never appear in `AllKeys()` unless `BindEnv` was called. `AutomaticEnv` alone is not enough.

Required `BindEnv` set today includes:

- System (`KIRIVERS_`): `log.level`, `log.console.enabled`, `log.console.no_color`, `log.file.enabled`, `log.file.dir`, `log.file.filename`, `log.file.max_size_mb`, `log.file.max_backups`, `log.file.max_age_days`, `log.file.compress`, `log.file.local_time`, `postgres.dsn`, `storage.*`, `storage.private.*`, `node.display_name`, `cluster.download` (`s3` \| `local`; default `s3`; honored only when `storage.driver=s3` and `cache.driver=redis`), `security.session_idle_hours`, `security.login_pending_ttl_seconds`, `security.totp_max_attempts_per_period`, `security.webauthn_rp_id`, `security.webauthn_origins`, `jobs.workers`, `url_signing_secret`, `cache.driver`, `cache.redis.addr`, `cache.redis.password`, `cache.redis.db`, `cache.redis.reconnect_interval_minutes`, `dynamic_pack.max_bytes` (default 512MiB; D6 uncompressed Manifest-size cap), `file_list.max_files` (default 16; native diff `file_list` pending-file ceiling; `<1` → 16), `changelog.default_entries` (default 5; `<1` → 5), `changelog.max_entries` (default 50; `<1` → 50; after fallback, default > max → `config.Load` error, process does not listen).
- Each plane (`KIRIVERS_ADMIN_` / `KIRIVERS_CLIENT_`): `addr`, `mode`, `tls_cert`, `tls_key`, `trusted_proxies`, plus the same stream keys under `log.system.*` and `log.access.*`.
- Admin plane only (`KIRIVERS_ADMIN_`): `enabled` → `KIRIVERS_ADMIN_ENABLED` (default `true`; `false` skips the admin `http.Server` — no API/SPA/proxy; client plane still listens). `static_dir` → `KIRIVERS_ADMIN_STATIC_DIR`. BindEnv and `SetDefault("frontend/dist")` on the admin Viper only (not `newPlaneViper`, so there is no `KIRIVERS_CLIENT_STATIC_DIR`). Omitted YAML key uses the default; explicit `static_dir: ""` disables UI hosting even when `frontend.Dist()` has `index.html`.

There is **no** `bootstrap_admin_token` key. There is **no** process-level `http.*` block and **no** shared TLS fallback.

TLS: a plane enables HTTPS only when that file’s `tls_cert` **and** `tls_key` are both non-empty; otherwise plaintext. Two listeners in one process: client plane `addr` (default `:8080`), admin plane `addr` (default `:8081`). `gin.SetMode` is process-global: if either plane `mode` is `debug` (case-insensitive) the process is debug, otherwise release.

Each plane YAML has `trusted_proxies` (IP or CIDR list). Empty means `SetTrustedProxies(nil)` — do **not** keep Gin’s default trust-all (`0.0.0.0/0`). Env: `KIRIVERS_ADMIN_TRUSTED_PROXIES` / `KIRIVERS_CLIENT_TRUSTED_PROXIES` (comma-separated). Not a system/`config.yaml` key.

---

## Scenario: Per-plane trusted_proxies

### 1. Scope / Trigger

Any use of `c.ClientIP()` (admin last-login IP, check/diff/telemetry/CI rate-limit keys) behind a reverse proxy, or any new Gin engine in `cmd/server.go`.

### 2. Signatures

```go
type PlaneConfig struct {
    TrustedProxies []string `mapstructure:"trusted_proxies"`
    // Addr, TLS, Log …
}
func ApplyTrustedProxies(engine *gin.Engine, proxies []string) error
```

`cmd/server.go` calls `ApplyTrustedProxies` when constructing **each** plane engine, before `Register*`. Invalid CIDR → `fatal` before listen.

### 3. Contracts

- YAML: `trusted_proxies: []` or a list of IPv4/IPv6 addresses or CIDRs.
- Env overrides the whole key: `KIRIVERS_ADMIN_TRUSTED_PROXIES=127.0.0.1,10.0.0.0/8`. Load must split on comma, trim, drop empties (Viper often unmarshals env as one string).
- Empty after normalize → `engine.SetTrustedProxies(nil)` so `ClientIP()` is the `RemoteAddr` host.
- Non-empty → `SetTrustedProxies(list)`; leftmost `X-Forwarded-For` is used only when the direct peer is in the list.
- `config` must not import `gin`.

### 4. Validation & Error Matrix

| Condition | Result |
|-----------|--------|
| Empty YAML list / omitted key (SetDefault `[]`) | No XFF trust; last-login and rate-limit IPs are RemoteAddr |
| Direct peer in list + `X-Forwarded-For: 198.51.100.10, 192.0.2.1` | `ClientIP()` is `198.51.100.10` |
| Illegal CIDR (`hello/world`) | `ApplyTrustedProxies` error; process does not listen |
| `KIRIVERS_ADMIN_TRUSTED_PROXIES` set | Client plane list unchanged |

### 5. Good/Base/Bad Cases

- Good: `admin-example.yaml` / `client-example.yaml` document `trusted_proxies: []` and the matching `KIRIVERS_*_TRUSTED_PROXIES` env name.
- Base: `gin.New()` tests that assert IP **must** call `ApplyTrustedProxies` (default engine still trusts all).
- Bad: putting `trusted_proxies` on `config.yaml`; custom `X-Forwarded-For` parsers in handlers.

### 6. Tests Required

- Config: YAML list; admin env does not leak to client; comma env splits into multiple entries; `config-example.yaml` has no `trusted_proxies`.
- `ApplyTrustedProxies(nil)` and `[]string{}` both ignore XFF; listed proxy records leftmost XFF; illegal CIDR returns error.
- HTTP login: empty trust + XFF → last_login_ip is RemoteAddr host.

### 7. Wrong vs Correct

#### Wrong

```go
e := gin.New() // trusts 0.0.0.0/0; any client can spoof X-Forwarded-For
```

#### Correct

```go
e := gin.New()
if err := middleware.ApplyTrustedProxies(e, cfg.Admin.TrustedProxies); err != nil {
    fatal(err)
}
```

---

## Scenario: Admin UI static hosting (NoRoute)

### 1. Scope / Trigger

Serving the compiled Vue admin console (`frontend/dist`) from the admin Gin plane, adding `admin.yaml` `static_dir`, or changing unknown-route behavior on either plane.

### 2. Signatures

```go
type PlaneConfig struct {
    StaticDir string `mapstructure:"static_dir"` // admin Viper only
}
type Deps struct {
    AdminStaticDir string
    AdminStaticFS  fs.FS // compiled frontend/dist; nil in unit tests
}
func RegisterAdmin(engine *gin.Engine, deps Deps) // ends with mountAdminStatic
func logAdminStatic(log zerolog.Logger, staticDir string, embedded fs.FS)
```

### 3. Contracts

- YAML: `static_dir` on `admin.yaml` only. Env: `KIRIVERS_ADMIN_STATIC_DIR`. Do **not** BindEnv on the client Viper.
- Omitted key → default `frontend/dist`. Explicit `static_dir: ""` disables UI hosting **even if** the binary has an embedded UI.
- Relative paths are process CWD (same as `./logs` / `storage.local.root`).
- Compile: `frontend/embed.go` `//go:embed all:dist` (package `frontend`). `cmd` passes `frontend.Dist()` as `Deps.AdminStaticFS`. `dist/.gitkeep` is committed so `go test` / `go build` work without `yarn build`. Vite `closeBundle` rewrites `.gitkeep` after `emptyOutDir`.
- HTTP: after API registration, admin **replaces** `NoRoute`. No `gin.Static` / `StaticFS` (those register `GET /*filepath` and fail `TestOpenAPIRoutesSync`). Do not add GET `/` to OpenAPI.
- `/api` and `/api/**` that are **not** `/api/v1/projects` / `/api/v1/projects/**`, and any non-GET/HEAD unknown path → JSON `NOT_FOUND`. Client plane NoRoute stays JSON-only even if `AdminStaticDir` / `AdminStaticFS` are set.
- GET/HEAD: each request re-Stats disk `index.html`. If it is a file, serve from disk (`http.ServeFile`). Else if `AdminStaticFS` has `index.html`, serve from the FS (`http.ServeFileFS`). Missing last-segment-with-`.` → JSON `NOT_FOUND` (never HTML); other paths → root `index.html`.
- Missing directory or `index.html` on disk **and** no embedded `index.html` (including when it is a directory): process still listens; GET `/` is JSON 404. A later `yarn build` onto `static_dir` does not need a restart. Shipping a real UI inside the binary requires `yarn build` **before** `go build`.
- Hosted SPA client-plane calls (`/api/v1/projects/**`) are reverse-proxied to this process’s client listener when `Deps.ClientProxyURL` is set — see the D9 scenario below. Vite `yarn dev` still splits `/api/v1/projects` before `/api`.

### 4. Validation & Error Matrix

| Condition | Result |
|-----------|--------|
| `index.html` + `/assets/app.js` present | GET `/` and GET `/assets/app.js` → 200 file |
| GET `/login` (no file) | 200 root `index.html` |
| GET `/assets/missing.js` | 404 JSON `NOT_FOUND`, body not HTML |
| GET `/api/v1/no-such` (not `/api/v1/projects/**`) | 404 JSON `NOT_FOUND` |
| GET `/api/v1/projects/...` on admin with empty `ClientProxyURL` | 404 JSON `NOT_FOUND` (unit tests) |
| GET `/api/v1/projects/...` on admin with `ClientProxyURL` set | proxied client-plane status/body (not SPA HTML) |
| Empty `static_dir` | GET `/` JSON 404 even if embed has `index.html`; `/api/v1/health` 200; startup `warn` |
| Disk missing `index.html`, embed has it | GET `/` 200 from embed; startup `info` `embedded files enabled` |
| Disk and embed both missing `index.html` | GET `/` JSON 404; `/api/v1/health` 200; startup `warn` |
| Disk `index.html` and embed both present | GET `/` 200 from **disk** |
| URL `..` / escape from root | 404 JSON; files outside the static root never appear in the body |
| Directory URL (e.g. GET `/assets`) | SPA `index.html`, no listing |

### 5. Good/Base/Bad Cases

- Good: `t.TempDir()` fixture or `fstest.MapFS`; tests never depend on repo `frontend/dist` containing a Vue build (gitignored except `.gitkeep`).
- Base: `fullTestEngines` leaves `AdminStaticDir` empty and `AdminStaticFS` nil so OpenAPI route sync stays exact.
- Bad: `engine.Static("/", dir)` or `engine.StaticFS("/", http.FS(frontend.Dist()))`; proxying `/api/v1/admin`; falling back missing `.js` to `index.html`; omitting `ClientProxyURL` in production `static_dir` hosting.

### 6. Tests Required

- Config: default `frontend/dist`; `static_dir: ""` disables; omitted YAML key still takes `KIRIVERS_ADMIN_STATIC_DIR`; `KIRIVERS_CLIENT_STATIC_DIR` does not bind; `admin-example.yaml` documents the key; client/system examples do not.
- Controller: AC table above plus no `*` in `engine.Routes()`; client engine with a dist fixture still JSON-404s `/` and `/login`; embed MapFS fallback; disk wins over embed; empty `static_dir` disables embed.
- Cmd: `logAdminStatic` Warns on empty dir, missing file, and directory-named `index.html`; Info when a disk file exists; Info `embedded files enabled` when disk is missing and FS has `index.html`.

### 7. Wrong vs Correct

#### Wrong

```go
engine.Static("/", cfg.Admin.StaticDir) // GET /*filepath breaks TestOpenAPIRoutesSync
engine.StaticFS("/", http.FS(frontend.Dist()))
```

#### Correct

```go
RegisterAdmin(engine, deps) // mountClientProjectsProxy then mountAdminStatic NoRoute; deps.AdminStaticFS = frontend.Dist()
```

---

## Scenario: Admin reverse-proxies client `/api/v1/projects` (D9)

### 1. Scope / Trigger

The Vue admin is hosted on the admin plane (`static_dir`). Same-origin `AnnouncementPreviewDialog` / check / integrity / client login call `/api/v1/projects/**`. Without a proxy those paths hit admin NoRoute and return JSON `NOT_FOUND` (“资源不存在”). Do **not** restore the old “must not proxy” rule.

### 2. Signatures

```go
// internal/controller/proxy.go
func ClientProxyURLFromAddr(addr string, useTLS bool) string
func mountClientProjectsProxy(engine *gin.Engine, target string)

type Deps struct {
    AdminStaticDir string
    ClientProxyURL string // empty → do not mount (tests keep JSON 404)
}
```

`cmd/server.go` sets `ClientProxyURL` from `config.Client.Addr` + client TLS via `ClientProxyURLFromAddr` (wildcard/`0.0.0.0`/`::` → `127.0.0.1`).

### 3. Contracts

- Match **only** path `== /api/v1/projects` or prefix `/api/v1/projects/`. Never `/api/v1/admin`.
- Empty / unparsable `target` → skip mount. `fullTestEngines` leaves it empty.
- `httputil.NewSingleHostReverseProxy`; set `X-Forwarded-For` / `X-Forwarded-Host` / `X-Forwarded-Proto`. HTTPS loopback may skip TLS verify.
- Middleware `Abort` after `ServeHTTP` so NoRoute cannot rewrite the proxied body to SPA HTML.
- Vite still lists `'/api/v1/projects'` before `'/api'` for `yarn dev`.

### 4. Validation & Error Matrix

| Condition | Result |
|-----------|--------|
| `ClientProxyURL` empty | GET `/api/v1/projects/x/announcements` on admin → JSON `NOT_FOUND` |
| `ClientProxyURL` set, client 200 `{announcements:[]}` | admin GET same path → 200 same body |
| Path `/api/v1/admin/projects` | admin handler, not proxy |
| Missing hashed `/assets/*.js` | JSON `NOT_FOUND`, not proxy |

### 5. Good/Base/Bad Cases

- Good: `TestAdminProxiesClientProjects` with an httptest client origin.
- Base: OpenAPI route-sync tests with empty `ClientProxyURL`.
- Bad: proxying all `/api`; treating hosted SPA `NOT_FOUND` as a missing announcement resource.

### 6. Tests Required

- `internal/controller/static_test.go`: proxy forwards; empty URL 404; `/api/v1/admin` unproxied.

### 7. Wrong vs Correct

#### Wrong

```go
// Admin NoRoute JSON 404 for /api/v1/projects/** so planes never mix.
```

#### Correct

```go
mountClientProjectsProxy(engine, deps.ClientProxyURL) // before NoRoute
```

---

## Scenario: Admin last-login snapshot

### 1. Scope / Trigger

Changing instance-admin JSON, `POST /api/v1/admin/auth/login` success path, or `admins` columns.

### 2. Signatures

```go
LastLoginAt *time.Time
LastLoginIP *string `gorm:"size:64"`
func (s *AdminService) Login(ctx context.Context, username, password, clientIP string) (*LoginResult, error)
func (r AdminStore) UpdateLastLogin(ctx context.Context, id uuid.UUID, at time.Time, ip *string) error
```

JSON (`publicAdmin`): always `last_login_at` and `last_login_ip`. Time is RFC 3339 UTC `2006-01-02T15:04:05Z`; never-logged-in is JSON `null`. OpenAPI `Admin` schema in `openapi.admin.json` must match; then `yarn generate:api`.

### 3. Contracts

- Only issuing a **full** admin session updates the columns (after 2FA / forced enrollment). Password success, pending tokens, and failed second-factor must not write. Persist error → `INTERNAL_ERROR`, no session token.
- TOTP is not “enrolled” until recovery codes are acknowledged. Write `AdminTOTP.ConfirmedAt` in `AckRecovery`, not in `ConfirmTOTP`. Relogin after confirm-but-before-ack must restart TOTP setup (`enroll_totp`), not skip to `second_factor`.
- Admin HTTP auth is one opaque session token (`Authorization: Bearer <random>`, resolved in the cache under `sess:` / `pend:`, sliding TTL): there is **no** JWT config key, and `AdminAuth` never verifies a signed token. `github.com/golang-jwt/jwt/v5` still appears in `go.mod` as `// indirect` only because `internal/service → go-webauthn/webauthn/protocol` pulls it (verify with `go mod why github.com/golang-jwt/jwt/v5`); it is a WebAuthn/passkey dependency, not an auth path, and not residue to clean up.
- IP is `c.ClientIP()` (after `ApplyTrustedProxies`). Blank → SQL/JSON `null`; cap 64 bytes.
- Point-update only (`Updates(map)`), never full `Save`. Map must use untyped `nil` for a NULL IP so GORM does not skip the column.
- Frontend `/admins`: generated `Admin` type; `null` → `t('admins.neverLoggedIn')` in **both** locales. Do not hand-edit `generated/`.

### 4. Validation & Error Matrix

| Condition | Result |
|-----------|--------|
| CLI-created admin, never HTTP login | both fields JSON `null` |
| Login success | fields are this request’s UTC time and ClientIP |
| Wrong password after a success | previous last-login unchanged |
| `UpdateLastLogin` fails | 500 `INTERNAL_ERROR`, no `access_token` |

### 5. Good/Base/Bad Cases

- Good: login then PATCH username keeps last-login fields.
- Base: empty ClientIP still sets `last_login_at`.
- Bad: login history table; recording failed attempts onto these columns; omitting keys instead of JSON `null`.

### 6. Tests Required

- Service: write/overwrite/empty-IP/failed-password/pending-token-does-not-write.
- HTTP: RemoteAddr round-trip; never-logged-in `null`; PATCH rename; persist failure has no token; AC7 trust list vs XFF.
- OpenAPI: `openapi.admin.json` `Admin` schema includes nullable `last_login_*`.

### 7. Wrong vs Correct

#### Wrong

```go
_ = s.store.Save(ctx, admin) // can clobber a concurrent username/password change
```

#### Correct

```go
updates := map[string]any{"last_login_at": at, "last_login_ip": nil}
if ip != nil {
    updates["last_login_ip"] = *ip
}
db.Model(&model.Admin{}).Where("id = ?", id).Updates(updates)
```

---

## Scenario: Pending login `has_passkey`

### 1. Scope / Trigger

Changing `POST /api/v1/admin/auth/login` pending JSON, `AdminLoginOutput`, or the login wizard default second-factor tab.

### 2. Signatures

```go
func (s *AdminService) Login(ctx context.Context, username, password, clientIP string) (*LoginResult, error)
// LoginResult.HasPasskey is set only when stage == second_factor, from len(ListPasskeys) > 0.
func writeLoginResult(c *gin.Context, out *service.LoginResult)
```

OpenAPI: optional boolean `AdminLoginOutput.has_passkey` in `openapi.admin.json`. Then `yarn generate:api`. Do not hand-edit generated SDKs.

### 3. Contracts

- Password success with `stage=second_factor` always includes JSON `has_passkey` (`true` or `false`). Other pending stages (`enroll_totp`, `ack_recovery`, `optional_passkey`) and `status=complete` omit the key.
- Password `401 UNAUTHORIZED` must not contain `has_passkey`.
- Compute the flag from Postgres at `Login`. Do **not** store it (or passkey names) on cache `pendingPayload`. Frontend may persist `hasPasskey` on `sessionStorage` `kirivers.auth.pending` so a refresh can restore the default tab.
- Login wizard: `has_passkey === true` → default Passkey tab; otherwise TOTP. Do not auto-call WebAuthn begin. Keep TOTP / recovery / Passkey tabs. Do not probe `webauthn/login/begin` to infer the flag.

### 4. Validation & Error Matrix

| Condition | Result |
|-----------|--------|
| Confirmed TOTP, zero passkeys | pending `second_factor`, `has_passkey: false` |
| Confirmed TOTP, ≥1 passkey | pending `second_factor`, `has_passkey: true` |
| Unenrolled admin login | pending `enroll_totp`, no `has_passkey` |
| Wrong password | 401, no `has_passkey` |
| `ListPasskeys` error | login fails (same as other DB errors), no pending token |

### 5. Good/Base/Bad Cases

- Good: second_factor JSON has a boolean; frontend defaults the tab from it; click still starts WebAuthn.
- Base: old pending blobs without `hasPasskey` default to TOTP.
- Bad: putting `has_passkey` on 401 or complete; caching it on the pending token; auto-prompting WebAuthn on mount.

### 6. Tests Required

- Service: zero vs one passkey → `HasPasskey`; pending cache JSON has no `has_passkey`.
- HTTP: 401 and enroll/ack/optional/complete omit the key; `second_factor` is a boolean.
- OpenAPI property exists; `yarn generate:api` yields `has_passkey?: boolean`.

### 7. Wrong vs Correct

#### Wrong

```go
body["has_passkey"] = out.HasPasskey // on every pending stage
```

#### Correct

```go
if out.Stage == service.StageSecondFactor {
    body["has_passkey"] = out.HasPasskey
}
```

### 4. Validation & Error Matrix

| Condition | Result |
|-----------|--------|
| YAML omits `url_signing_secret`, env sets `KIRIVERS_URL_SIGNING_SECRET` | struct field populated |
| Same without `BindEnv` | field stays empty; boot generates an ephemeral signing secret and warns — every URL issued before a restart becomes invalid |
| Only one of cert/key set | plaintext listen (not TLS) |
| `KIRIVERS_ADMIN_LOG_SYSTEM_LEVEL=debug` | admin system stream only; client/system `log.level` unchanged |
| Stream `console.enabled` + `file.enabled` both false | `logger.Open` error; listen never starts |
| Stream `file.enabled` with empty `dir` or `filename` | `logger.Open` error (not TempDir) |

### 5. Good/Base/Bad Cases

- Good: example YAML keeps every secret empty — `tls_cert`/`tls_key` (in `admin-example.yaml` / `client-example.yaml`) and `url_signing_secret: ""` (system example), never a bootstrap token.
- Base: missing `configs/config.yaml` falls back to `config-example.yaml`; missing plane files independently fall back to `admin-example.yaml` / `client-example.yaml`.
- Bad: `os.Getenv("KIRIVERS_POSTGRES_DSN")` inside `database` or `storage`.

### 6. Tests Required

- Env override of `postgres.dsn` and of `url_signing_secret` when the YAML key is omitted.
- Prefix isolation: `KIRIVERS_ADMIN_ADDR` must not set client `addr`; `KIRIVERS_ADMIN_LOG_SYSTEM_LEVEL` must not set client/system log level.
- Stream construction: empty level → info; both sinks off and empty file path fail.
- `listenMode(cert, key)` table: both set → `tls`; else `plain`.

### 7. Wrong vs Correct

#### Wrong

```go
v.AutomaticEnv()
v.Unmarshal(&cfg) // url_signing_secret never in AllKeys()
```

#### Correct

```go
_ = v.BindEnv("url_signing_secret")
v.AutomaticEnv()
v.Unmarshal(&cfg)
```

---

## Forbidden Patterns

- `c.JSON` for error objects outside `pkg/response`.
- Reading env/files for DSN or storage secrets outside `internal/config`.
- A second object-storage interface besides `storage.Backend`.
- `internal/openapi`, `internal/feed` as extra top-level packages (adapters: `internal/service/store`; HTTP: `internal/controller/client/store`).
- Root-level `docker-compose.yml` / `compose.yml`. Developer databases: `dev/docker/compose.yml`. Users: `deploy/compose.yml`. Do not host VitePress on admin `GET /` or set `static_dir` to the docs-site dist.
- Redis job queue (jobs are PostgreSQL `SKIP LOCKED`).
- Fatal exit in server mode when Postgres is down (`/health` must still answer).
- Logging password hashes, `url_signing_secret`, admin session tokens, or raw `device_id` (see telemetry-privacy.md).
- `client.public_url` / `KIRIVERS_CLIENT_PUBLIC_URL` for Markdown media hosts — expand `${site_url}` from Referer (fallback: request origin).
- Mounting Markdown media GET behind `ClientProjectAuth` or urlsign.

---

## Required Patterns

- GORM entities: `TableName`, `BeforeCreate` UUID, comments covering purpose/fields/relations (Chinese comments are the project convention in `internal/model`).
- Success JSON snake_case; times RFC 3339 UTC. OpenAPI schema field names for those payloads must match the handler, not a frontend-only alias.
- Exception: `POST .../media` success body is the Vditor envelope `{code, msg, data: {succMap, errFiles}}` via `response.JSON` (HTTP 200 even when `code=1`). Do not invent a second `{error:…}` shape; bind failures still use `response.Error`.
- Admin HTTP: `Authorization: Bearer` comparison is case-insensitive (RFC 6750).
- First instance admin: `kirivers admin` subcommand, not an HTTP bootstrap.

---

## Testing Requirements

- Table tests for LocalFS Range and path rejection.
- HTTP tests with `httptest` + `gin.TestMode` for health/openapi/admin auth.
- Do not require a live Postgres for unit tests (memory admin store, nil DB for ready 503).

---

## Code Review Checklist

- [ ] Errors go through `pkg/response`
- [ ] New config keys have `SetDefault` **or** `BindEnv`
- [ ] New models registered in `AutoMigrateModels`
- [ ] `:project_ref` resolves UUID, live slug, and unexpired alias (no POST 301)
- [ ] Owning plane OpenAPI updated; schema fields match handler JSON (route test is not enough — run `TestOpenAPIForbiddenNames` / `TestOpenAPIFieldTables` with `TestOpenAPIRoutesSync|TestPlaneSpecsValid`)
- [ ] `yarn generate:api` after plane-spec edits (`frontend/AGENTS.md`)
- [ ] `/health` `ready` still means Ping **and** Head `.ready`
- [ ] Markdown media GET uses `ClientProjectResolve`; stored URLs are `${site_url}` not PresignGet (`storage-guidelines.md`)
