# Project Admin Identity and List Stats

> Executable contract for admin-plane project **display name** and list/get **stats**. `name` is a console title, never a `project_ref`. `stats` is computed in `ProjectService.StatsFor` from batched store queries. Do **not** put `name` on client `ProjectPublic`, check/changelog, or store feeds. Do **not** call `SelectTarget` to pick the list-card latest version. Keep this document and `internal/controller/openapi.admin.json` in sync with handler JSON.

---

## Scenario: Display name vs slug

### 1. Scope / Trigger

Admin create/list/get/patch JSON gained `name`. Operators need a Unicode product title while URL/API identity stays `slug` / UUID / unexpired alias.

### 2. Signatures

```go
// internal/model/project.go
Name string `gorm:"type:text;not null;default:''" json:"name"` // MaxProjectNameRunes=128 is an app-layer cap, not varchar(128)

func (p *Project) DisplayName() string // stored Name, else Slug

// internal/service/project.go
type CreateProjectInput struct {
    Slug *string
    Name *string // create: nil → name=slug; explicit blank → 400
    // …
}

func (s *ProjectService) StatsFor(ctx context.Context, ids []uuid.UUID, engineByID map[uuid.UUID]string, now time.Time) (map[uuid.UUID]ProjectStats, error)
func (s *ProjectService) SetTelemetryStore(store repository.TelemetryStore) // nil → telemetry counts stay 0
```

Column add: GORM AutoMigrate with empty default. Immediately after migrate:

```sql
UPDATE projects SET name = slug WHERE name = '';
```

(`internal/database.AutoMigrate`). Identity cache stores `Project` including `Name`. Do not cache derived `stats` in the identity blob.

### 3. Contracts

| Field | Rule |
|-------|------|
| `slug` | `[a-zA-Z0-9_-]{3,64}`; the only `project_ref` besides UUID/alias |
| `name` | Unicode, trim, 1–128 runes, **not unique**, never used as `project_ref` |
| Create omit/nil `name` | persist `name = slug` (POST `{ "slug": "my-app" }` still 201) |
| Create/patch explicit whitespace-only `name` | 400 `INVALID_REQUEST` |
| Empty stored `name` on read | `DisplayName()` / JSON `name` fallback to slug |
| Client `ProjectPublic` | **no** `name` |

`stats` appears on **list and get** (and patch response). Create 201 may omit `stats`. Stats failure → 500 `INTERNAL_ERROR`; never a partial `projects` array with missing stats.

`latest_version`: max among `published` and `deprecated` using `update.CompareVersions` and the project `compare_engine` (all channels). Payload `{ version, channel, status }` or `null`. Exclude draft/revoked. Display `version` = semver if set else integer string. Tie-break: published over deprecated, then later `publish_time`/`created_at`. Drafts-only project → `null`.

Catalog: `versions` total + by status; `channels`; `matrix_rows`; `hw_revs`.

Tokens: `project_tokens` / `ci_tokens` each `{ total, active }` where active = `ExpiresAt` null or in the future. Counts only — no fingerprints.

Storage: `artifact_count` = row count; `storage_bytes` = sum of `size` **once per distinct `storage_key`** (reuse copies a row that shares `StorageKey`). Do not add manifest-entry sizes.

Telemetry: last `service.TelemetryDowngradeWindow` (24h) **event counts** `installed_24h` / `failed_24h` only on the list/get `stats.telemetry` object. No distinct-device counts, no hash lists (`telemetry-privacy.md`). Overview charts use `GET .../stats/series` daily `installed_count` / `failed_count` (7 calendar days in UTC) — **not** the list-card object.

Registry: `active_7d` = clients with `last_check_at` in the last 7 days, batched. List **cards** render `latest_version` + `versions.total` + `active_7d` only — no Token totals, no 24h install bars, no UUID, no mix bars.

`gray_weight_tenure_activity` is a project bool (default **true**) on get/patch; not part of `stats`.

Project GET/PATCH JSON uses `store_token` (write-only plaintext on create/rotate) and `has_store_token` (no `feed_token` aliases). GET also returns stored `file_list_max_files` (0 = inherit) and read-only `file_list_max_files_limit` (platform ceiling). PATCH `file_list_max_files` above the ceiling is 400 `INVALID_REQUEST`.

GET also returns `changelog_default_entries` (new-project default 5) and `changelog_max_entries` (0 = inherit instance max) plus read-only `changelog_default_entries_limit` and `changelog_max_entries_limit` (instance YAML ceilings after `<1` fallback). PATCH `changelog_default_entries` must be 1…instance default and ≤ the effective max. PATCH `changelog_max_entries` must be 0 or 1…instance max, and the resulting default must not exceed the effective max. Over the matching instance ceiling, or default > effective max, is 400 `INVALID_REQUEST`. Dirty stored rows are clamped at catalog read (`applyChangelogCeiling`); they must not fail `config.Load` or crash the process.

GET includes read-only `storage_driver` (`local` | `s3`) from the process config, not a per-project column. PATCH `storage_visibility` must be `public`; `private` is 400 `INVALID_REQUEST`. `storage_prefix` (trim, strip leading/trailing `/`, split on `/`) or `storage_bucket` (trim, strip `/`, whole value) must not equal the current or patched project slug case-insensitively; empty values are allowed. Admin list is `created_at DESC, id DESC`. Version create defaults: `gray_start_percent=30`, `gray_step_percent=10` (do not SQL-rewrite existing rows).

Batching: `GROUP BY project_id` (memory-store equivalent scans). Do not `ListVersions` / `ListTokens` / dump telemetry per project in the list handler.

### 4. Validation & Error Matrix

| Condition | Status | Code |
|-----------|--------|------|
| POST `{ slug }` only | 201 | `name` equals slug |
| POST `{ slug, name }` | 201 | stored name |
| PATCH `{ name: "  " }` or over 128 runes | 400 | `INVALID_REQUEST` |
| Duplicate names across projects | 2xx | allowed |
| Stats store error | 500 | `INTERNAL_ERROR` |
| Empty `name` column after migrate | read | JSON `name` = slug |
| PATCH `file_list_max_files` > `file_list_max_files_limit` | 400 | `INVALID_REQUEST` |
| PATCH `changelog_default_entries` > `changelog_default_entries_limit` | 400 | `INVALID_REQUEST` |
| PATCH `changelog_max_entries` > `changelog_max_entries_limit` | 400 | `INVALID_REQUEST` |
| PATCH changelog default > effective max | 400 | `INVALID_REQUEST` |
| PATCH `storage_visibility=private` | 400 | `INVALID_REQUEST` |
| PATCH prefix path segment or bucket equals slug | 400 | `INVALID_REQUEST` |

### 5. Good/Base/Bad Cases

- Good: one `GET /api/v1/admin/projects` fills every card (`name` + `stats`); reused artifacts do not double-count `storage_bytes`.
- Base: new project with no published/deprecated versions → `latest_version: null`; nil telemetry store → zeros.
- Bad: `SUM(artifacts.size)` without distinct `storage_key`; N+1 `listVersions` from Vue; `SelectTarget` as “latest”; returning device hashes on the index; putting `name` on client `ProjectPublic`.

### 6. Tests Required

- Create omit name → stored name equals slug; explicit name stored; whitespace name 400; duplicate names allowed (`TestProjectNameCreateAndPatch`).
- Empty Name → `DisplayName()` is slug.
- Reuse same `storage_key` → `storage_bytes` equals one blob (`TestProjectStatsStorageReuseAndLatest`).
- Drafts only → `latest_version` null; published+deprecated tie-break prefers published.
- Telemetry inside vs outside 24h window for installed/failed only; `active_7d` from registry; `TestStatsSeriesIncludesTelemetryDays`; `TestClientBucketsIncludesChannels`.
- HTTP list/get include `name` and `stats`; blank PATCH name 400 (`internal/controller/admin/project_test.go`).

### 7. Wrong vs Correct

#### Wrong

```go
body["name"] = p.Slug // always; ignores stored title
db.Model(&model.Artifact{}).Select("SUM(size)") // double-counts reuse
```

```ts
for (const p of projects.value) {
  await listVersions({ path: { project_ref: p.slug } }) // N+1
}
```

#### Correct

```go
body["name"] = p.DisplayName()
st, err := svc.StatsFor(ctx, ids, engines, time.Now().UTC())
```

```ts
const { data } = await listProjects()
return data?.projects ?? [] // each item already has stats
```

---

## Common Mistake: PATCH body includes GET `stats` / `uuid`

**Symptom**: Overview save 400 or silently ignores fields after `Object.assign(form, project)`.

**Cause**: Generated `Project` includes read-only `stats` and `uuid`. Spreading the GET payload into the PATCH body is not a settings write.

**Fix**: Overview `save` deletes `payload.stats` and `payload.uuid` before `updateProject`. Do not send `stats` on create/patch request.

**Prevention**: `frontend/quality-guidelines.md`. Keep `stats` response-only in OpenAPI.

## Common Mistake: Pinia current-project store for the drawer caption

**Symptom**: Caption desyncs from the route; extra store to invalidate.

**Cause**: Shell needs a title while `projectRef` is a route param.

**Fix**: `DefaultShell` `getProject` when `projectRef` is set; `provide('setProjectCaption')`. Parent `[projectRef].vue` injects and updates after load/save. No `useProjectStore`.

**Prevention**: `frontend/state-management.md`.

---

## Scenario: Per-platform latest vs filtered version list

### 1. Scope / Trigger

Batch release (`release.vue`) needs the latest **published** version that has a **ready** line for `(os, arch)` without N+1 `getVersion`. The versions **index** needs AND filters (`channel`, `status`, `os`, `arch`) over all statuses. List-card `stats.latest_version` stays project-wide and must not call `SelectTarget`.

### 2. Signatures

```go
func (s *ProjectService) ListPublishedReadyVersionsForPlatform(ctx context.Context, p *model.Project, osRaw, archRaw string) ([]model.Version, *ProjectLatestVersion, error)
func (s *ProjectService) ListVersionsFiltered(ctx context.Context, projectID uuid.UUID, filter VersionListFilter) ([]model.Version, error)
```

`GET /api/v1/admin/projects/{project_ref}/versions`

### 3. Contracts

- `latest=true` **or** `latest=1` (and only then) enters the published+ready branch. Both `os` and `arch` are required → else 400 `INVALID_REQUEST`. Canonicalize with `CanonicalOSWrite` / `CanonicalArch`. Envelope `{ "versions": [...], "latest": { version, channel, status } | null }`. `latest` uses the same `CompareVersions` / `newerReleased` / `displayVersion` helpers as list-card stats. Slim `publicVersion` **without** `lines`. Unknown pair → **200** `{ "versions": [], "latest": null }` (not 404).
- Without `latest`: `ListVersionsFiltered` AND of optional `channel`, `status`, `os`, `arch`. `os`+`arch` means “has a line for that pair (any line status)”. Envelope `{ "versions": [...] }` with **no** `latest` key so `versions/index.vue` `data?.versions ?? []` does not break.
- `?os=&arch=` **without** `latest` is the filtered list, **not** the batch-release latest envelope.
- Do **not** call `SelectTarget`.

### 4. Validation & Error Matrix

| Condition | Status | Body |
|-----------|--------|------|
| `latest=true` and both `os` and `arch` | 200 | `{ versions, latest }` |
| `latest=true` missing os or arch | 400 | `INVALID_REQUEST` |
| Filters without `latest` | 200 | `{ versions }` (no `latest`) |
| Pair with no published ready line (`latest`) | 200 | `{ versions: [], latest: null }` |
| Alias `darwin`+`amd64` | 200 | same as `macos`/`x86_64` |

### 5. Good/Base/Bad Cases

- Good: two published ready linux/x86_64 versions + `latest=true` → `latest` is the greater compare key; versions page `?os=windows&arch=x86_64` still includes drafts that have that line.
- Base: unfiltered list still includes drafts and omits `latest`.
- Bad: `SelectTarget` as latest; putting `latest` on every os+arch GET; treating `?os=&arch=` as published-only.

### 6. Tests Required

- Latest ordering, empty pair, unfiltered omits `latest`, `latest` without os/arch is 400 (`internal/controller/admin/version_test.go`).
- `GET ?os=linux` (arch omitted, no `latest`) stays a filtered/unfiltered list without `latest`.

### 7. Wrong vs Correct

#### Wrong

```go
if osQ != "" && archQ != "" {
    list, latest, err := h.projects.ListPublishedReadyVersionsForPlatform(...)
    response.JSON(c, 200, gin.H{"versions": items, "latest": latest})
    return
}
```

#### Correct

```go
if latestQuery == "true" || latestQuery == "1" {
    if osQ == "" || archQ == "" {
        response.Error(c, 400, "INVALID_REQUEST", "latest requires os and arch", nil)
        return
    }
    list, latest, err := h.projects.ListPublishedReadyVersionsForPlatform(...)
    response.JSON(c, 200, gin.H{"versions": items, "latest": latest})
    return
}
list, err := h.projects.ListVersionsFiltered(..., VersionListFilter{Channel, Status, OS: osQ, Arch: archQ})
response.JSON(c, 200, gin.H{"versions": items})
```

---

## Scenario: VersionLine min OS/API prefill

### 1. Scope / Trigger

Min OS and min API hang on each **VersionLine** (not matrix). The version-detail form prefills from the previous same `(os, arch)` line. `SelectTarget` / check uses only those line fields (`lineMeetsOS`).

### 2. Signatures

```go
func (s *ProjectService) LineDefaults(ctx context.Context, p *model.Project, versionRef, os, arch string) (*string, *int, error)

GET /api/v1/admin/projects/{project_ref}/versions/{version}/line-defaults?os=&arch=
```

200 body: `{ "min_os": string|null, "min_api_level": int|null }`.

### 3. Contracts

- Source: among versions **below** the path version (compare engine of the project), pick the highest compare-key version that already has a line for that canonical `(os, arch)` and copy that line’s `min_os` / `min_api_level`.
- No prior line → both null (empty prefill).
- Write path: line create/patch stores numeric-ish `min_os` string and non-negative `min_api_level`. Empty means unrestricted.

### 4. Validation & Error Matrix

| Condition | Status | Code |
|-----------|--------|------|
| Missing os or arch | 400 | `INVALID_REQUEST` |
| Unknown version | 404 | `VERSION_NOT_FOUND` |
| No prior line | 200 | both null |

### 5. Good/Base/Bad Cases

- Good: 1.0 windows/x86_64 min_os=10, min_api=28 → 2.0 line-defaults returns those values.
- Base: first version on that pair → nulls.
- Bad: reading matrix `min_os`; prefilling from a **higher** version; using `CanonicalOS` (would rewrite `ipados`).

### 6. Tests Required

- `TestLineDefaultsPrefillsFromHighestBelow` (`internal/service/version_test.go`).
- HTTP line-defaults in `internal/controller/admin/version_test.go`.

### 7. Wrong vs Correct

#### Wrong

```go
minOS = matrixRow.MinOS // column dropped
```

#### Correct

```go
minOS, minAPI, err := h.projects.LineDefaults(ctx, p, version, osQ, archQ)
response.JSON(c, 200, gin.H{"min_os": minOS, "min_api_level": minAPI})
```

---

## Scenario: Channel name, unlisted, optional token

### 1. Scope / Trigger

Admin channel CRUD plus project-create system seeds. Client catalog is in `client-check-contract.md`. Slug is the stable client key; `name` is display-only.

### 2. Signatures

```go
type Channel struct {
    Name, Slug string
    Unlisted bool
    TokenHash string `json:"-"`
}
func SystemChannelSeeds(projectID uuid.UUID, names SystemChannelNames) []Channel
```

Admin JSON: `name`, `slug`, `unlisted`, `token` (plaintext from `token_plain`; empty when unset), `token_required` (bool), never `token_hash`. Write `token` on create/patch to set or rotate; `""` clears both hash and plaintext. Check still hashes `X-Channel-Token` against `token_hash`. Never put plaintext in ETag, logs, or cache keys.

### 3. Contracts

- Create requires non-empty `name` (≤128 runes). System channels: snapshot names from the **admin UI locale at project create** (`CreateProjectInput.SystemChannelNames`). zh-CN defaults: alpha=内测版, beta=公测版, stable=正式版; en keeps alpha/beta/stable; empty field falls back to slug. Names are stored; later locale changes do not rewrite them.
- Slug is immutable after create (system and custom). `PatchChannel` **ignores** body `slug`; the path param is the identity.
- `unlisted` omits from client **list**; get-by-slug and check still work when the slug is known (D7).
- Optional token: `applyChannelToken` — nil keeps, `""` clears `token_hash` and `token_plain`, non-empty stores SHA-256 hex **and** plaintext. `token_required` is `TokenProtected()`. Never log plaintext. List/get always return `token`.
- `enabled=false` hides from all client paths regardless of unlisted/token.

### 4. Validation & Error Matrix

| Condition | Status | Code |
|-----------|--------|------|
| Empty name | 400 | `INVALID_REQUEST` |
| PATCH body `slug` | ignored | slug stays path identity |
| Duplicate slug | 409 | `SLUG_TAKEN` |
| Admin GET / list | 200 | `token` plaintext (empty if unset), `token_required` bool, no hash |

### 5. Good/Base/Bad Cases

- Good: zh project create seeds 内测版/公测版/正式版; rotate token; list/get still show plaintext after refresh; `token_required` true afterwards.
- Base: no token → `token_required` false; listed enabled channels appear on client list.
- Bad: renaming slug; putting TokenHash in JSON/ETag; 403 on check for a wrong channel token.

### 6. Tests Required

- `internal/service/platform_test.go` (seed names, unlisted, token match/trim).
- Admin publicChannel shape; client catalog tests.

### 7. Wrong vs Correct

#### Wrong

```go
ch.Slug = req.Slug // on PATCH
etagChannels = append(..., ch.TokenHash)
```

#### Correct

```go
"token_required": ch.TokenProtected(),
// ETag: Unlisted + TokenProtected bool only
```

---

## Scenario: Project languages admin CRUD

### 1. Scope / Trigger

Per-project locale catalog in `project_languages`. Changelog JSON maps stay keyed by code. Announcements are one language column per row (`announcement-contract.md`). The table is the authoring list and the source of `projects.default_locale`.

### 2. Signatures

```go
type ProjectLanguage struct { /* id, project_id, code, display_name, sort_order, is_default */ }

GET/POST /api/v1/admin/projects/{project_ref}/languages
PATCH/DELETE /api/v1/admin/projects/{project_ref}/languages/{code}
```

Envelope `{ "languages": [...] }`. Create 201. `project:read` lists; `project:admin` writes. Audit `language.create|update|delete`.

### 3. Contracts

- Code: trim, `^[A-Za-z0-9][A-Za-z0-9_-]{1,31}$`, case preserved (`zh-CN`), unique per project **case-insensitive**. Cap 16 (`ProjectMaxLanguages`).
- Exactly one `is_default`. Setting default clears siblings and writes `projects.default_locale`, then `cache.InvalidateProject`.
- POST create project **requires** `default_locale` and seeds one default row. Omit/empty → 400. No hardcoded `en` on new creates. PATCH `default_locale` upserts the row and flips default.
- AutoMigrate backfill: empty projects get `default_locale` (legacy invalid/empty → `en` only here) plus harvested announcement `language` values and changelog JSON keys (valid codes only, cap 16).
- Delete does **not** rewrite announcement `language` or changelog JSON. Cannot delete last language or the current default.
- Client plane: `GET /api/v1/projects/{project_ref}/languages` returns `{languages:[{code,display_name,is_default,sort_order}]}` (omit `project_id`). Changelog leftover still uses `default_locale` + JSON maps. Announcement language filter is D5 (`announcement-contract.md`).

### 4. Validation & Error Matrix

| Condition | Status | Code |
|-----------|--------|------|
| Duplicate code (any case) | 409 | `LANGUAGE_TAKEN` |
| Unknown `:code` | 404 | `LANGUAGE_NOT_FOUND` |
| Delete last or default | 400 | `INVALID_REQUEST` |
| Cap 16 | 400 | `INVALID_REQUEST` |
| POST project without `default_locale` | 400 | `INVALID_REQUEST` |

### 5. Good/Base/Bad Cases

- Good: zh-CN console create → one `zh-CN` default; add `ja`; deleting `ja` leaves existing announcement `language` values unchanged.
- Base: existing project after migrate has at least `default_locale` plus any stored map keys.
- Bad: hardcoded zh-CN/en tabs; rebuilding changelog from only the selected locale; skipping `InvalidateProject` on default change.

### 6. Tests Required

- Create seed, duplicate 409, delete default/last 400, two projects may share `zh-CN`, harvest skips illegal keys, default-change cache invalidation (`language_test.go`, `language.go` HTTP tests, `database/languages_test.go`).

### 7. Wrong vs Correct

#### Wrong

```ts
content = { 'zh-CN': zh, en } // drops ja on save
```

#### Correct

```ts
content = { ...existing, [selected]: edited } // unselected keys with copy stay
```

---

## Scenario: Overview series, client buckets, gray actual_percent

### 1. Scope / Trigger

List cards stay compact. Overview / clients / gray pages need buckets and time series without N+1 `listProjectClients({ limit: 500 })`. Version list chips need live coverage, not `gray_start_percent`.

### 2. Signatures

```
GET /api/v1/admin/projects/{ref}/clients
GET /api/v1/admin/projects/{ref}/clients/stats     → ClientBuckets (versions, os, arch, channels, check histogram)
GET /api/v1/admin/projects/{ref}/stats/series?from=&to=
GET /api/v1/admin/projects/{ref}/versions/{v}/gray
```

`ClientBuckets.channels` is a full GROUP BY `last_channel` (not a 500-row sample). Series rows include `new_count`, `active_count`, `installed_count`, `failed_count` per UTC day.

### 3. Contracts

- Changelog PATCH is allowed after publish; dual version numbers and artifact bytes stay 409.
- `actual_percent` = `allowlisted/N*100` (N==0 or complete → 100). Attach on version list/get.
- Published changelog editor lives on the versions index dialog, not the artifact page.

### 4–7

Tests: `TestClientBucketsIncludesChannels`, `TestStatsSeriesIncludesTelemetryDays`, `TestVersionListShowsActualGrayPercent`, published changelog PATCH 200 + identity 409.

`ClientBuckets.countries` is `{ code, count, names }` (fused locale map, **not** `country_name`). Overview country pie uses `geoLabel(names, ui.locale, code)`. OS/Arch pies stay on the clients page, not overview.

---

## Scenario: Platform admin vs project members

### 1. Scope / Trigger

Same `admins` table and `/login`. `is_platform_admin` default **true** (HTTP `POST /admins` and `kirivers admin`). Project-only accounts are `false` and need `project_members` rows.

### 2. Signatures

```
GET  /api/v1/admin/projects                         → ListVisible (platform: all; else memberships)
POST /api/v1/admin/projects                         → platform only; optional owner_username + owner_password
GET/POST /api/v1/admin/projects/{ref}/members
PATCH/DELETE /api/v1/admin/projects/{ref}/members/{admin_id}
GET/POST/PATCH/DELETE /api/v1/admin/geoip/databases → platform only
GET/POST/PATCH/DELETE /api/v1/admin/admins          → platform only
```

`project_members`: `(project_id, admin_id)` unique; role `owner` | `admin`. No FK. Platform admins skip membership.

### 3. Contracts

- Login JSON includes `is_platform_admin`. Old sessions without the field are treated as platform (`!== false` in the console).
- Human session on a foreign `project_ref` → 404 `PROJECT_NOT_FOUND` (not 403).
- `/admins` and `/geoip` for project-only sessions → 403 `FORBIDDEN`.
- `role=owner` create/PATCH is platform-only. Last owner delete/demote → 400 `LAST_OWNER`.
- Omit `owner_username` on create → no membership row (platform-only access).

### 4. Validation & Error Matrix

| Condition | Status | Code |
|-----------|--------|------|
| Project member GET `/admins` or `/geoip` | 403 | `FORBIDDEN` |
| Project member GET foreign project | 404 | `PROJECT_NOT_FOUND` |
| Remove last owner | 400 | `LAST_OWNER` |
| Unknown member | 404 | `MEMBER_NOT_FOUND` |
| Owner POST `role=owner` | 403 | `FORBIDDEN` |
| Attach existing username without password | 200/201 | membership row; 409 if already on this project |
| Password present, username exists | 409 | `USERNAME_TAKEN` |

### 5. Good/Base/Bad Cases

- Good: platform creates project with `owner_username` + password → project-only account; that login lists only that project; `/geoip` 403.
- Base: omit owner on create → no `project_members` row; only platform admins see it.
- Bad: 403 on a foreign `project_ref`; requiring a membership row for platform admins; treating `/admins` as `AdminAuth` alone.

### 6. Tests Required

- `identity_test.go`: member cannot list/create platform admins; foreign project 404; platform lists all; last owner 400; owner cannot POST `role=owner`; GeoIP 403 for project-only.

### 7. Wrong vs Correct

#### Wrong

```go
if !hasMembership(admin, project) {
    response.Error(c, 403, "FORBIDDEN", "not a member", nil)
}
```

#### Correct

```go
// ProjectAccess: platform OR membership, else 404 PROJECT_NOT_FOUND
// RequireInstanceAdmin: IsPlatformAdmin, else 403 FORBIDDEN
```

---

## Scenario: Fused GeoIP on admin client stats

### 1. Scope / Trigger

Overview country pie and client list country/region columns. Storage + fusion live in `storage-guidelines.md` (GeoIP MMDB). This section is the admin JSON the console must consume.

### 2. Signatures

Admin client row: `country_code`, `region_code`, `geo_i18n` (`{country: map, region: map}`).

`ClientBuckets.countries[]`: `{ code, count, names }` — `names` is the fused **country** locale map. Do not add `country_name`.

### 3. Contracts

Console `geoLabel(names, ui.locale, code)`: `names[locale]` → language prefix → `en` → `Intl.DisplayNames({type:'region'}).of(code)` → code → i18n 「未知」. Chart `computed` depends on `ui.locale` so the language menu relabels without refetch.

Pass **country** ISO into `geoLabel`’s code argument. Do **not** pass `region_code` (e.g. `CA`) into `Intl.DisplayNames({type:'region'})` — that API treats `CA` as Canada, not California.

Do not put world country names in `zh-CN.ts` / `en.ts`.

### 4. Validation & Error Matrix

Unknown / private / no DB → empty `code`, UI unknown copy. Lookup failure must not 500 the upsert.

### 5. Good/Base/Bad Cases

- Good: same stats payload, switch `zh-CN`/`en` in the top bar, pie labels change.
- Base: empty buckets render unknown, not a crash.
- Bad: `country_name` column; hardcoded China/United States in locale files; using region subdivision ISO with `DisplayNames`.

### 6. Tests Required

- Bucket JSON shape in service/HTTP tests with stub lookup. Frontend: `geoLabel` is a pure helper (no extra store).

### 7. Wrong vs Correct

#### Wrong

```ts
label = t(`countries.${code}`) // locale file as gazetteer
geoLabel(regionNames, locale, regionCode) // CA → Canada
```

#### Correct

```ts
geoLabel(bucket.names, ui.locale, bucket.code)
```

---

## Scenario: Store listings CRUD (replaces `store_protocols` jsonb)

### 1. Scope / Trigger

Hard cutover: drop `projects.store_protocols`. Each feed is a `project_store_listings` row. Admin CRUD is under the project, not a single PATCH map. Writes invalidate the project identity cache.

### 2. Signatures

```go
type StoreListing struct { /* id, project_id, protocol, slug, enabled, os, arch, channel, identifiers, package_source, manifest_path */ }

func (s *ProjectService) ListStoreListings(ctx, projectID) ([]model.StoreListing, error)
func (s *ProjectService) CreateStoreListing(ctx, projectID, StoreListingWrite) (*model.StoreListing, error)
func (s *ProjectService) PatchStoreListing(ctx, projectID, id, StoreListingWrite) (*model.StoreListing, error)
func (s *ProjectService) DeleteStoreListing(ctx, projectID, id) error
```

HTTP: `GET/POST /api/v1/admin/projects/{project_ref}/store-listings`, `PATCH/DELETE .../store-listings/{listing_id}`. List envelope `{listings:[]}`. Each row includes `store_url`.

### 3. Contracts

| Field | Rule |
|-------|------|
| `protocol` | Adapter `Protocol()` (`electron`, `sparkle`, `appimage`, …) — not `electron-updater` |
| `slug` | `[a-z0-9-]{3,64}`, no underscore (same as channel slug) |
| Duplicate `(protocol, slug)` | 400 `INVALID_REQUEST` (not 409) |
| Illegal slug | 400 `INVALID_REQUEST` |
| `package_source` | `line_full` or `manifest_path`; latter requires `manifest_path` |
| Pins | nullable os/arch/channel; must exist on matrix / channel table |

Do not PATCH a `store_protocols` map on the project. `protocol` and `slug` are immutable after create (PATCH ignores them). Empty string on `os`/`arch`/`channel` clears that pin.

Create/patch/delete call `invalidateProject`. Duplicate uniqueness is `(project_id, protocol, slug)` — `idx_store_listings_project_protocol_slug` includes `project_id` on the same GORM index name.

### 4. Validation & Error Matrix

| Condition | Status | Code |
|-----------|--------|------|
| Unknown `protocol` (including `electron-updater`) | 400 | `INVALID_REQUEST` |
| Illegal slug (underscore, too short, uppercase) | 400 | `INVALID_REQUEST` |
| Duplicate `(protocol, slug)` in the same project | 400 | `INVALID_REQUEST` (not 409 `SLUG_TAKEN`) |
| `package_source=manifest_path` with empty `manifest_path` | 400 | `INVALID_REQUEST` |
| Pin channel / os / arch not on that project | 400 | `INVALID_REQUEST` |
| Unknown `listing_id` | 404 | `NOT_FOUND` |
| Invalid UUID `listing_id` | 400 | `INVALID_REQUEST` |
| Two projects may share `sparkle`/`stable` | 2xx | allowed |

### 5. Good/Base/Bad Cases

- Good: two Sparkle rows (`stable`, `beta`) with different identifiers; each `store_url` contains `/store/sparkle/{slug}/appcast.xml`.
- Base: omit pins → protocol enumerates; `package_source` defaults `line_full`.
- Bad: nine-row `store_protocols` PATCH; using project `name` as listing slug; 409 for duplicate listing slug.

### 6. Tests Required

- `internal/controller/admin/listing_test.go`: create, duplicate 400, illegal slug 400, missing `manifest_path` 400, list/patch/delete, `store_url` shape.
- HTTP feed: pinned Sparkle `beta` on the same OS omits stable even when `channel=stable` is in the query (`TestFeedSparklePinnedBetaOmitsStableSameOS`).
- HTTP feed: `manifest_path=Setup.exe` enclosure is that file's sha256, not the kind=full zip (`TestFeedManifestPathSetupExeSHA256`).

### 7. Wrong vs Correct

#### Wrong

```go
p.StoreProtocols["sparkle"] = map[string]any{"enabled": true} // column dropped
response.Error(c, 409, "SLUG_TAKEN", err.Error(), nil)       // listing duplicate
```

#### Correct

```go
row, err := h.projects.CreateStoreListing(ctx, p.ID, req.toInput())
// ErrStoreListingTaken / ErrInvalidStoreListing → 400 INVALID_REQUEST
```

---

## Scenario: Install policy templates (project × channel × platform)

### 1. Scope / Trigger

Operators maintain `OVERWRITE` / `KEEP_IF_EXISTS` path lists that seed **new** multi-file Manifests. Dedicated admin resources; do **not** dump the list onto project GET/PATCH. Reference latest is **not** `GET versions?latest=true` (that hop is cross-channel published+ready).

### 2. Signatures

```
GET/PUT /api/v1/admin/projects/{ref}/install-policy-rules?os=&arch=
GET/PUT /api/v1/admin/projects/{ref}/channels/{slug}/install-policy-rules?os=&arch=
GET /api/v1/admin/projects/{ref}/install-policy-rules/reference?channel=&os=&arch=
```

`channel` on reference defaults to `stable`. PUT is replace-all for that scope + pair. Channel GET returns `{ entries, effective }`. Reference returns `{ version, status, entries }` where `entries` use `publicManifestEntry` (path, size, sha256, md5, install_policy, integrity_check).

### 3. Contracts

- Overlay D1: same `(os, arch)` path, channel overrides project; empty channel list inherits project; channel `OVERWRITE` undoes project KEEP.
- `os`/`arch` must already exist on `platform_matrix` → else 400 `INVALID_REQUEST` (not 404 `MATRIX_NOT_FOUND`).
- Cap 256; path via `pathutil.NormalizeAndValidatePath`.
- One-off Manifest PUT does not write templates. Template PUT does not rewrite published Manifests. Rule writes do not `InvalidateProject`.
- Stamp `install_policy` only when creating a **new** Manifest (zip / bundle / omitted PUT). Re-uploading a zip onto a line that already has a Manifest validates bytes/hashes only — do not restamp from the current template.
- `_keep.json` / `keep_if_exists.txt` are ignored members (same class as `.DS_Store`). Do not read KEEP from their contents and do not write them into generated archives. Policy lives on `manifest_entries` / integrity JSON only.
- Reference latest = max `CompareVersions` among that channel’s versions with `status != revoked`. Version exists but no Manifest/line → `version` set, `entries: []`. No non-revoked version → `version` null, 200.

### 4. Validation & Error Matrix

| Condition | HTTP | `error.code` |
| --- | --- | --- |
| Missing/unknown matrix pair | 400 | `INVALID_REQUEST` |
| Unknown channel slug | 404 | `CHANNEL_NOT_FOUND` |
| Illegal path | 400 | `INVALID_PATH` (same as Manifest) or `INVALID_REQUEST` |
| Over cap / duplicate path / bad policy | 400 | `INVALID_REQUEST` |
| Missing `os` or `arch` query | 400 | `INVALID_REQUEST` |

### 5. Good/Base/Bad Cases

- Good: project KEEP `config/user.json` on `windows/x86_64`; stable has no channel rows; new stable zip containing that path stores `KEEP_IF_EXISTS` and `integrity_check=false`; other paths `OVERWRITE`.
- Base: empty `install_policy_rules` → all new paths `OVERWRITE` (today’s default). Channel GET with empty `entries` still returns `effective` from the project list.
- Bad: using `GET .../versions?latest=true` as the reference picker (cross-channel published+ready). Bad: planting KEEP only via zip sidecar. Bad: restamping policy when the line already has a Manifest.

### 6. Tests Required

- Overlay + reference + no restamp on existing zip (`internal/service/install_policy_test.go`).
- HTTP AC1–AC6, AC8–AC9 (`internal/controller/admin/install_policy_test.go`).
- Sidecar ignore / no keep members in generated `store_full` (`internal/service/bundle_test.go`, `manifest_test.go`).

### 7. Wrong vs Correct

#### Wrong

```go
keep := extractKeepRules(zipBytes)          // sidecar is not a policy channel
stampEffective(existingManifest, rules)     // restamp on re-upload
list, _ := versions.ListLatest(os, arch)    // workbench latest=true is cross-channel
```

#### Correct

```go
// New Manifest only:
pol, _ := svc.EffectiveInstallPolicy(ctx, projectID, channelSlug, os, arch)
// Existing Manifest: byte/hash validate; keep stored install_policy.
ref, _ := svc.LatestChannelPlatformManifest(ctx, projectID, channel, os, arch)
```


