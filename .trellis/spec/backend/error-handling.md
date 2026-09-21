# Error Handling

> HTTP errors leave the process only through `pkg/response`. Domain packages return Go `error` values.

---

## Overview

Handlers never assemble `{ "error": ... }` by hand. Success JSON uses snake_case field names. Timestamps in JSON are RFC 3339 UTC (`2006-01-02T15:04:05Z`).

Unknown routes use `engine.NoRoute(response.NotFound)` so Gin cannot return HTML.

Exception — store feeds: missing/disabled listing, unknown protocol, or leftover `/store/{protocol}/{doc}` without a listing slug is **plain-text 404** (`store-adapter-contract.md`). Artifact hash misses still use this JSON envelope.

Exception — admin UI static hosting: `RegisterAdmin` **replaces** NoRoute (never `gin.Static` / `StaticFS`). `/api` (and `/api/**` that are not the D9 `/api/v1/projects/**` proxy) plus non-GET/HEAD unknown paths stay JSON `NOT_FOUND`. Other GET/HEAD paths serve disk `static_dir` when `index.html` is a file; otherwise `frontend.Dist()` when it has `index.html`; explicit `static_dir: ""` is API-only even if embed has `index.html`. Missing hashed assets whose last path segment contains `.` stay JSON `NOT_FOUND` (never HTML). The client engine NoRoute remains JSON-only. The VitePress site in `website/` is not this UI.

---

## Scenario: Unified HTTP error envelope

### 1. Scope / Trigger

Any new Gin handler, middleware abort, or health probe failure.

### 2. Signatures

```go
func JSON(c *gin.Context, status int, payload any)
func Error(c *gin.Context, status int, code, message string, details any)
func NotFound(c *gin.Context)
```

### 3. Contracts

Error body:

```json
{ "error": { "code": "UNAUTHORIZED", "message": "invalid credentials", "details": null } }
```

- `code`: stable machine token (SCREAMING_SNAKE). Infrastructure: `UNAUTHORIZED`, `NOT_FOUND`, `NOT_READY`, `INVALID_REQUEST`, `INTERNAL_ERROR`, `TOTP_RATE_LIMITED`. Project: `PROJECT_NOT_FOUND`, `FORBIDDEN`, `COMPARE_ENGINE_IMMUTABLE`, `SLUG_TAKEN`, `LANGUAGE_TAKEN`, `LANGUAGE_NOT_FOUND`, `LAST_OWNER`, `MEMBER_NOT_FOUND`, `CLIENT_NOT_FOUND`, `GEOIP_NOT_FOUND`. New domain codes (client-check, telemetry, feeds) reuse this same envelope; do not invent a second error shape. Add every new code to both OpenAPI masters’ `Error.code` enum **and** `frontend/src/locales/{zh-CN,en}.ts` `errors.<CODE>`.
- `message`: human-readable; do not leak SQL or stack traces.
- `details`: optional; omit when nil (`omitempty`).
- Env: none. Codes are not configurable.

### 4. Validation & Error Matrix

| Condition | Status | Code |
|-----------|--------|------|
| Unknown `/api` route, non-GET/HEAD unknown path, or any client-plane unknown path | 404 | `NOT_FOUND` |
| Admin GET/HEAD outside `/api` when UI is mounted | 200 (file or root `index.html`) | — |
| Missing hashed admin asset (last path segment contains `.`) | 404 | `NOT_FOUND` |
| Missing/invalid Bearer on admin CRUD | 401 | `UNAUTHORIZED` |
| Bad login (password step) | 401 | `UNAUTHORIZED` |
| Bad TOTP / recovery code after pending token | 401 | `UNAUTHORIZED` |
| TOTP/recovery attempts exceed `security.totp_max_attempts_per_period` in one 30s period | 429 | `TOTP_RATE_LIMITED` |
| DB ping or storage Head of `.ready` fails | 200 | `/health` body `ready=false` (process stays alive) |
| Runtime Postgres unavailable (after successful start) | 404 | `NOT_FOUND` on `/api` except `/health` `/openapi.json`; `/health` 200 with `ready=false` |
| Panic recovered | 500 | `INTERNAL_ERROR` |
| Malformed JSON body | 400 | `INVALID_REQUEST` |
| Missing Bearer on admin project routes | 401 | `UNAUTHORIZED` |
| Project/CI token creates a project or issues a token for another project | 403 | `FORBIDDEN` |
| Unknown / expired slug alias / soft-deleted project | 404 | `PROJECT_NOT_FOUND` |
| PATCH `compare_engine` after `versions.status=published` | 409 | `COMPARE_ENGINE_IMMUTABLE` |
| Duplicate live slug | 409 | `SLUG_TAKEN` |
| Duplicate project language code (case-insensitive) | 409 | `LANGUAGE_TAKEN` |
| Unknown project language code | 404 | `LANGUAGE_NOT_FOUND` |
| Delete last language or current default; language cap 16; bad code | 400 | `INVALID_REQUEST` |
| DELETE system channel (`alpha`/`beta`/`stable`) | 409 | `SYSTEM_CHANNEL` |
| PATCH matrix `package_type` after a published Version Line for that `(os,arch)` | 409 | `PACKAGE_TYPE_IMMUTABLE` |
| Bind artifact with unregistered `hw_rev` | 400 | `HW_REV_UNKNOWN` |
| Duplicate SemVer/integer in the **same** project (intended) | 409 | `VERSION_ALREADY_EXISTS` |
| Duplicate SemVer/integer in a **second** project (live bug) | 500 | `INTERNAL_ERROR` — unique index is not composite with `project_id`; see `database-guidelines.md` |
| Client announcements: unparsable `version` query | 400 | `INVALID_QUERY_PARAM` — see `announcement-contract.md` |
| Admin announcements: illegal scope / empty title / bad window / partial reorder | 400 | `INVALID_REQUEST` |
| Admin announcements: version scope but Version missing | 404 | `VERSION_NOT_FOUND` (write only; client GET does not 404 the list) |
| Markdown media GET: unknown id / id not in this project | 404 | `NOT_FOUND` |
| Markdown media GET: missing project | 404 | `PROJECT_NOT_FOUND` |
| Markdown media POST: missing multipart `file[]` / empty file | 400 | `INVALID_REQUEST` (bind failures). Vditor per-file failures stay HTTP 200 `{code:1, data.errFiles}` — see `quality-guidelines.md` |
| Project-only session hits `/admins`, `/geoip`, or `/nodes` | 403 | `FORBIDDEN` |
| Human session on a project they do not belong to | 404 | `PROJECT_NOT_FOUND` (same as unknown slug; do not 403) |
| Delete or demote the last project owner | 400 | `LAST_OWNER` |
| Unknown project member | 404 | `MEMBER_NOT_FOUND` |
| DELETE unknown client UUID | 404 | `CLIENT_NOT_FOUND` |
| GET unknown client UUID | 404 | `NOT_FOUND` (via `writeGrayErr`; not `CLIENT_NOT_FOUND`) |
| PATCH/DELETE unknown GeoIP database | 404 | `GEOIP_NOT_FOUND` |
| Duplicate store listing `(protocol, slug)` / illegal listing slug / unknown protocol / missing `manifest_path` | 400 | `INVALID_REQUEST` |
| PATCH `storage_visibility=private` or prefix/bucket overlapping the project slug | 400 | `INVALID_REQUEST` |
| Unknown store `listing_id` | 404 | `NOT_FOUND` |
| `GET /packages/:ref` unknown or non-hex FileName | 404 | `NOT_FOUND` envelope (not a filename index) |
| Missing/disabled store listing or leftover `/store/{protocol}/{doc}` | 404 | **plain text** (feed adapter exception; not `pkg/response`) |
| Pack D6/D7 oversize or unknown Manifest path | 200 | `{ "status": "full_package", ... }` success JSON — **not** 400 `NEEDED_PATHS_TOO_LARGE` (`client-check-contract.md`) |

### 5. Good/Base/Bad Cases

- Good: `GET /api/v1/admin/admins` without token → 401 `{ "error": { "code": "UNAUTHORIZED", ... } }`
- Base: `GET /api/v1/no-such` → 404 `NOT_FOUND` JSON, not HTML
- Bad: handler `c.JSON(401, gin.H{"error": "no"})` — second envelope shape

### 6. Tests Required

- Assert `json.Unmarshal` into `response.Body` and `Error.Code`.
- Recovery middleware: panic → JSON, `Content-Type` contains `json`, body is not HTML.
- Admin UI mounted (`t.TempDir()` fixture, not repo `dist/`): GET `/login` returns `index.html`; GET `/api/v1/no-such` and GET `/assets/missing.js` stay JSON `NOT_FOUND` (body is not HTML). `TestOpenAPIRoutesSync` stays green — do not register `gin.Static` / `StaticFS`.

### 7. Wrong vs Correct

#### Wrong

```go
c.JSON(http.StatusUnauthorized, gin.H{"message": "no token"})
```

#### Correct

```go
response.Error(c, http.StatusUnauthorized, "UNAUTHORIZED", "missing bearer token", nil)
```

---

## Error Types (Go)

Sentinel errors live in the owning package (`service.ErrInvalidToken`, `storage.ErrNotFound`). Controllers map them with `errors.Is` to HTTP status + code. Do not wrap sentinels with `fmt.Errorf` unless you use `%w`.

---

## Common Mistakes

### Common Mistake: Short-circuit health `ready` after DB ping

**Symptom**: Storage is down but `/health` reports `ready=true`.

**Cause**: Returning on first error.

**Fix**: Probe DB **and** storage (`errors.Join`); set `ready=false` if either fails. HTTP stays 200.

**Prevention**: `service.CheckReady` is the only ready implementation; `/health` must call it.

### Common Mistake: 403 on a project the member cannot see

**Symptom**: Project-only account probing `/projects/other-slug` learns that the slug exists.

**Cause**: Treating missing membership like `/admins` (403 `FORBIDDEN`).

**Fix**: Same 404 `PROJECT_NOT_FOUND` as an unknown slug. 403 is for platform-only **routes** (`/admins`, `/geoip`, `/nodes`, `POST /projects`).

**Prevention**: `identity_test.go` — member GET foreign project is 404; member GET `/geoip/databases` is 403.

### Don't: Invent `bootstrap_admin_token`

There is no unauthenticated bootstrap HTTP path. First admin is created by the `kirivers admin` subcommand.

### Common Mistake: 401 vs 403 for the wrong credential

**Symptom**: A valid project token hitting `POST /api/v1/admin/projects` looks like a missing login.

**Cause**: Treating every failed admin session-token lookup (`AdminAuth` cache miss on `sess:`) as `UNAUTHORIZED`.

**Fix**: If a Bearer parses as a project/CI token, return `FORBIDDEN` for instance-admin-only operations. Missing/garbage Bearer stays `UNAUTHORIZED`.

**Prevention**: `middleware.RequireInstanceAdmin` vs `ProjectAccess` — do not reuse `AdminAuth` alone on project-create.

### Gotcha: `RequireInstanceAdmin` is the platform flag

`RequireInstanceAdmin` means `admin.IsPlatformAdmin` (column default true for existing rows). Project owners/admins share the same `/login` session token but must 403 on `/admins`, `/geoip`, `/nodes`, project `node-sync`, and `POST /projects`. `ProjectAccess` after a human session: platform **or** a `project_members` row; otherwise 404 `PROJECT_NOT_FOUND`. Do not 403 on a foreign `project_ref` (existence leak). Platform admins skip membership. CI/project tokens stay on the token branch of `ProjectAccess` and still cannot hit `/admins` or `/geoip`.

### Gotcha: install-policy matrix pair is 400, not 404

Unknown or missing `os`/`arch` on install-policy GET/PUT is 400 `INVALID_REQUEST`. Do not reuse `MATRIX_NOT_FOUND` (that code is for the matrix resource itself). Illegal paths use the same mapping as Manifest (`INVALID_PATH` via `pathutil.ErrInvalidPath`). Unknown channel slug is 404 `CHANNEL_NOT_FOUND`.

### Gotcha: CORS preflight vs `require_client_token` / `force_https`

OPTIONS must succeed (204 + `Access-Control-Allow-*`) **before** token or HTTPS checks. Otherwise browsers never send the real GET. `Access-Control-Allow-Headers` is `Authorization, Content-Type, X-Project-Token, X-Channel-Token` (`middleware.corsAllowHeaders`). Missing `X-Channel-Token` on preflight makes browser check/catalog calls drop the header; it must **not** be treated as a project-token failure (check never 403s for a wrong channel token — see `client-check-contract.md`).
