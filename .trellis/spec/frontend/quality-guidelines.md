# Quality Guidelines

Yarn is the package manager (`frontend/AGENTS.md`). There is no frontend unit-test runner; the gate is lint + `vue-tsc` + production build against the real dual-plane API.

---

## Overview

- ESLint: `eslint-config-vuetify` with `{ ts: true }` (`eslint.config.js`). Ignores `src/api/generated/**`, `src/api/generated-client/**`, `src/typed-router.d.ts`.
- Typecheck: `yarn type-check` → `vue-tsc --build --force`. `yarn build` runs type-check then Vite.
- Format: Prettier via the Vuetify ESLint config (`yarn lint:fix`).
- Sync OpenAPI: `yarn generate:api` (`scripts/fetch-openapi.mjs` reads `internal/controller/openapi.admin.json` / `openapi.client.json`, injects `operationId`s into gitignored `frontend/.openapi/`, then `openapi-ts` two jobs). Do not hand-edit `generated/` or `generated-client/`. If handler JSON drifted, edit the owning plane spec, then `yarn generate:api`.
- Handwritten API layer is only `client.ts` (interceptors, `ApiError`, `registerAuthHooks`). Pages/stores/composables call generated methods.

---

## Verification

```bash
cd frontend
yarn lint
yarn type-check
yarn build
```

No Vitest/Jest suite. Manual checks: `yarn dev` (port 3000) with `go run .` and Docker Postgres from `dev/docker/compose.yml`. Production (`yarn build` then `go build`) embeds `frontend/dist` via `frontend/embed.go`; the admin plane prefers disk `static_dir` (default `frontend/dist`) and falls back to the embedded FS. Hosted SPA client-plane calls go through admin D9 reverse-proxy of `/api/v1/projects/**` (`backend/quality-guidelines.md`). Vite `yarn dev` still splits `/api/v1/projects` before `/api`.

---

## Forbidden Patterns

### Don't reintroduce MSW / mock-first

There is no `src/mocks/`, no `public/mockServiceWorker.js`, no `VITE_USE_MOCK`,
and no `/mock-s3` Vite proxy. The admin UI talks to the real dual-plane backend
(dev: Vite proxy; prod: admin plane hosts `frontend/dist` on the same origin as `/api/v1/admin` and reverse-proxies `/api/v1/projects` to the client listener).

Do not add `msw` / `fflate` (mock-only) dependencies, a DevPanel, or a second
path-construction site. New HTTP calls import `@/api/generated` or
`@/api/generated-client`.

### Don't rebuild a handwritten API wrapper

Do not add `src/api/endpoints/`, `src/api/paths.ts`, handwritten `src/api/types.ts`,
or `rawRequest`. List unwrap (`data?.projects ?? []`) lives at the call site.

### Don't bind platform catalog `name`

`GET .../platforms/catalog` items match Go `CatalogEntry`: `{ slug, aliases }`.
There is no `name`. Combobox items are `osComboboxItems` / `archComboboxItems`
(`catalogSlugs` = `slug ?? name`, union `COMMON_OS` / `COMMON_ARCH` in
`constants/platforms.ts`). Reading `entry.name` empties the dropdown even when
the catalog JSON is correct.

---

### Bare `useRoute()` param access in file-route pages

`useRoute()` returns a union of every route's param shape, so
`route.params.projectRef` fails type-check (`TS2339`). In a page whose route is
known, pass the route name from `src/typed-router.d.ts` to get exact param types:

```ts
const route = useRoute('/projects/[projectRef]/versions/[version]')
const projectRef = computed(() => route.params.projectRef) // typed string
```

Only in components shared across routes (e.g. `layouts/DefaultShell.vue`) narrow
manually: `route.params as { projectRef?: string }`. Fixed across all pages,
2026-09-13.

---

## Required Patterns

### Dual-plane API routing (admin :8081 / client :8080)

`/api/v1/admin/**` is served by the admin plane and `/api/v1/projects/**` by a
separate client-plane listener. The frontend always talks same-origin, so:

- `vite.config.mts` MUST keep the more specific `'/api/v1/projects'` proxy rule
  listed BEFORE the generic `'/api'` rule (first match wins):
  - `'/api/v1/projects'` → `KIRIVERS_CLIENT_API_URL ?? http://127.0.0.1:8080` (client plane)
  - `'/api'` → `KIRIVERS_API_URL ?? http://127.0.0.1:8081` (admin plane)
- Production hosting: the admin process **does** reverse-proxy `/api/v1/projects/**` (D9). Client-plane preview 404s only when `ClientProxyURL` is empty (unit tests) or the Vite proxy order is wrong in `yarn dev`.
- Check / integrity import `@/api/generated-client`. Admin CRUD imports
  `@/api/generated`. Both SDKs share `client.instance` from `api/client.ts`.
  Pages must not contain `/api/v1/` literals.

Instance-admin list (`pages/admins.vue`): show `last_login_at` / `last_login_ip` from generated `Admin`. JSON `null` uses `t('admins.neverLoggedIn')` in both locale files — do not leave the cell blank or reuse `created_at`. Chip `is_platform_admin` with `admins.platformAdmin` / `admins.projectOnly`.

Drawer `/admins` and `/geoip` (and the project-list create button) render only when `auth.isPlatformAdmin` (`admin.is_platform_admin !== false` so old sessions stay superusers). Route guard sends non-platform sessions from those paths to `/projects`.

Shared project nav: `projectNavItems` + `matchProjectNav` in `composables/useProjectNav.ts` — DefaultShell drawer and `[projectRef].vue` tabs must share the same items/order. Highlight is the longest `item.to` prefix of `route.path` (overview vs settings; `/versions/:v/gray` stays on Versions). Settings live at `/projects/:ref/settings`; overview is charts-only (country pie via `geoLabel(names, ui.locale)`, no OS/Arch pies, no settings form). GeoIP labels come from fused `names` + `ui.locale`, not locale files.

## Scenario: Dual hey-api Axios clients

### 1. Scope / Trigger

Admin console talks to two Gin listeners. hey-api must not merge `openapi.admin.json` and `openapi.client.json` (shared `/api/v1/health` would collide). Both SDKs still need one interceptor stack.

### 2. Signatures

- `openapi-ts.config.ts`: `defineConfig([ adminJob, clientJob ])` → `src/api/generated` and `src/api/generated-client`.
- `api/client.ts`: admin `client.setConfig({ baseURL, throwOnError: true })`, then interceptors on `client.instance`, then `clientPlane.setConfig({ axios: client.instance, baseURL, throwOnError: true })`.
- `main.ts` imports `@/api/client` before plugins/stores.

### 3. Contracts

| Call site | Import |
| --- | --- |
| Admin CRUD / upload / jobs | `@/api/generated` |
| Check / integrity / announcements preview | `@/api/generated-client` |
| Transport errors | `ApiError` / `isApiError` from `@/api/client` |

Vite: `/api/v1/projects` proxy rule listed before `/api`.

### 4. Validation & Error Matrix

| Condition | Result |
| --- | --- |
| Two `createClient()` Axios instances, interceptors only on admin | Client-plane check has no Bearer / no `ApiError` |
| Merge both specs into one output | Duplicate `getHealth` / mixed types |
| Check 304 with default Axios `validateStatus` | Interceptor turns it into `ApiError` |
| `direct_s3` true on artifact presign | Frontend throws; never PUT an unknown S3 key |

### 5. Good/Base/Bad Cases

- Good: `clientPlane.setConfig({ axios: client.instance, … })`.
- Base: check preview `validateStatus` allows 204 and 304; empty body → no-update alert.
- Bad: `rawRequest` + `paths.ts` for check because “it is not in the admin SDK”.

### 6. Tests Required

- `yarn type-check` after adding a client-plane call.
- Manual: check preview 204 shows no-update, not `ErrorAlert`.

### 7. Wrong vs Correct

#### Wrong

```ts
const { data } = await axiosInstance.get(`/api/v1/projects/${ref}/update/check`)
```

#### Correct

```ts
import { checkUpdate } from '@/api/generated-client'
await checkUpdate({
  path: { project_ref },
  body: { current_version, os, arch, channel },
  validateStatus: s => (s >= 200 && s < 300) || s === 304,
})
```

S3 object-storage PUT of unknown artifact keys is forbidden; `direct_s3` stays false.

---

## Scenario: Presign upload (API/TUS only)

### 1. Scope / Trigger

Frontend consumes `POST .../artifacts/presign` via generated `presignArtifactUpload`.
The Go handler writes `response.JSON(c, 200, PresignUploadOutput)` — a **naked**
object, not a named envelope. Artifact bytes always land on local temp then
promote; `direct_s3` is always `false`. Do not PUT unknown object keys to S3.

### 2. Signatures

Backend (`internal/service/artifact.go`):

```go
type PresignUploadOutput struct {
    UploadURL string `json:"upload_url"`
    Method    string `json:"method"`
    DirectS3  bool   `json:"direct_s3"` // always false for artifacts
}
```

`useUpload` reads `{ data }` from `presignArtifactUpload` and PUTs to `upload_url` through `axiosInstance`.

### 3. Contracts

| Field | Meaning |
| --- | --- |
| `upload_url` | Relative `/api/v1/admin/.../artifacts` (API proxy / TUS) |
| `method` | `PUT` |
| `direct_s3` | Always `false`. A true value is a contract error — throw, do not PUT. |

Request body: `{ filename, size? }`. API-proxy PUT response **is** the Artifact.

TUS and direct artifact PUT use generated `createTusUpload` / `headTusUpload` /
`patchTusUpload` / `putLineArtifact` with `throwOnError: true` when reading headers.

GeoIP upload uses `uploadGeoipDatabase` `onUploadProgress` (cap 99 until 201; keep `ErrorAlert` for 400).

### 4. Validation & Error Matrix

| Condition | Result |
| --- | --- |
| Unwrap `(data as { PresignUploadOutput }).PresignUploadOutput` | `upload_url` is always undefined |
| `direct_s3=true` | throw; do not PUT |
| `direct_s3=false` PUT without Bearer | admin 401 |

### 5. Good/Base/Bad Cases

- Good: `direct_s3=false` → `axiosInstance.put(url, file, { params: filename/hw_rev… })`; `finish(artifact.data)`.
- Base: GeoIP progress never shows 100% on a 400.
- Bad: treating an absolute `http(s)` presign URL as S3 and PUT-ing an unhashed key.

### 6. Tests Required

- `presignArtifactUpload` `data` is `{ upload_url, method, direct_s3 }` (no extra key).
- `yarn type-check` must compile `useUpload.ts` after any import split.

### 7. Wrong vs Correct

#### Wrong

```ts
return (data as { PresignUploadOutput: PresignUploadOutput }).PresignUploadOutput
if (presigned?.direct_s3) await axios.put(url, file) // unknown-key PresignPut
```

#### Correct

```ts
const { data: presigned } = await presignArtifactUpload({ path, body: { filename, size } })
if (presigned?.direct_s3) {
  throw new Error('direct S3 artifact upload is not supported')
}
await axiosInstance.put(url, file, { params, onUploadProgress })
```

---

## Scenario: Version changelog write (`changelog_i18n`)

### 1. Scope / Trigger

PUT/PATCH `/api/v1/admin/projects/:ref/versions/:version` via `putVersion` /
`patchVersion`. GET returns `changelog` as `ChangelogMap` (`{locale: {markdown}}`).
The UI edits `Record<string,string>`. The handler JSON has `changelog` as `*string`
and `changelog_i18n` as the map (`internal/controller/admin/version.go`).
Locale flatten is UI-only in `components/changelogMap.ts`.

### 2. Signatures

```go
Changelog     *string            `json:"changelog"`
ChangelogI18n model.ChangelogMap `json:"changelog_i18n"`
```

### 3. Contracts

| Direction | Field | Shape |
| --- | --- | --- |
| GET | `changelog` | `{ "zh-CN": { "markdown": "…" } }` (also tolerate a plain string value) |
| PUT/PATCH | `changelog_i18n` | same map; omit when empty |
| PUT/PATCH | `changelog` | optional string; do **not** send a locale object here |

Also omit empty strings; gray knobs (`gray_start_percent`, `gray_step_percent`, `gray_interval_seconds`) must be integers (Vuetify sliders yield floats). Do not send `rollout_percent`.

Promote body is `{ target_channel }`. Auto-publish rule is
`{ required_lines: string[], allow_partial?: boolean }`.

### 4. Validation & Error Matrix

| Condition | Result |
| --- | --- |
| PUT `changelog: { "zh-CN": "…" }` | 400 `invalid json` / bind failure |
| PUT `changelog_i18n: {}` | 400 on some binds; omit the key instead |
| GET map used raw in `v-textarea` | `[object Object]` |

### 5. Good/Base/Bad Cases

- Good: flatten GET → LocaleTabs strings; write only non-empty `changelog_i18n`.
- Base: no changelog → omit both keys.
- Bad: `body: { changelog: changelogDraft }` with a locale object.

### 6. Tests Required

- `yarn type-check` after changing changelog helpers or version write bodies.
- Live PUT of a version with LocaleTabs text must 2xx, not 400.

### 7. Wrong vs Correct

#### Wrong

```ts
await putVersion({ body: { changelog: { 'zh-CN': markdown } } })
```

#### Correct

```ts
await putVersion({
  path: { project_ref, version },
  body: { changelog_i18n: changelogDraftToI18n(draft) },
})
```

---

## Scenario: Audit list envelope (`events`)

### 1. Scope / Trigger

`GET /api/v1/admin/projects/:ref/audit` via `listAuditLogs`. Handler
(`audit.go` `listAudit`) writes `{events, next_cursor}` with row field `detail`
(singular) and `actor_fingerprint`. Master OpenAPI `AuditListOutput` matches
those names. Unwrap `data?.events ?? []`.

### 2. Signatures

```go
response.JSON(c, http.StatusOK, gin.H{
    "events":      events,
    "next_cursor": next,
})
```

### 3. Contracts

| Live / OpenAPI key | UI |
| --- | --- |
| `events` | table rows |
| `detail` | detail cell |
| `actor_fingerprint` | actor cell |
| `next_cursor` | pagination |

Use `?? []` so a missing array does not throw in `useApiResource`.

### 4. Validation & Error Matrix

| Condition | Result |
| --- | --- |
| Read `data.records` | empty table (that key is not in the live JSON) |
| Audit store nil | 503 `NOT_READY` |
| Bad cursor | 400 `INVALID_REQUEST` |

### 5. Good/Base/Bad Cases

- Good: `return data?.events ?? []`.
- Base: empty `events` → `EmptyState`, not a crash.
- Bad: `return data.records` with no fallback.

### 6. Tests Required

- Live GET after a publish shows at least one row (manual smoke).

### 7. Wrong vs Correct

#### Wrong

```ts
return data.records
```

#### Correct

```ts
return data?.events ?? []
```

---

## Scenario: Gray allowlist has GET; add by client id

### 1. Scope / Trigger

Version gray is a dedicated page (`versions/[version]/gray.vue`), not a textarea on version detail. Generated `listVersionGrayAllowlist` / `addVersionGrayAllowlist` / `deleteVersionGrayAllowlist`. There are **no** per-line allowlist endpoints. `publicVersion` has no `whitelist` array; list/get attach live `actual_percent`.

### 2. Signatures

```http
GET    /api/v1/admin/projects/{ref}/versions/{version}/gray
GET    /api/v1/admin/projects/{ref}/versions/{version}/gray/clients
GET    /api/v1/admin/projects/{ref}/versions/{version}/gray/series
GET    /api/v1/admin/projects/{ref}/versions/{version}/gray/allowlist
POST   /api/v1/admin/projects/{ref}/versions/{version}/gray/allowlist
DELETE /api/v1/admin/projects/{ref}/versions/{version}/gray/allowlist
POST   /api/v1/admin/projects/{ref}/versions/{version}/gray/complete
PATCH  /api/v1/admin/projects/{ref}/versions/{version}/gray
```

POST/DELETE body: `{ "client_ids": ["uuid", ...] }`. Response: `{ "entries": [...] }` (`device_id` is the stored hash or raw per policy).

### 3. Contracts

- Load members with GET. Pick from the project client registry; do not free-type device ids.
- Version list chip is `actual_percent` (allowlisted/N×100), not `gray_start_percent`.
- Locale `versions.rolloutHint` must describe allowlist + start/step/interval + `gray_completed_at`. Do **not** say HMAC / percent-of-devices / `rollout_percent`. HMAC wording belongs only to `device_id` storage-policy copy (`hashed`).
- Changelog after publish is edited from the versions **list** dialog only; 产物页 has no changelog editor.
- Charts use `EChart.vue` (`echarts/core` + `vue-echarts`), not a full `echarts` default import. No charts on project list cards.
- Uploads bind `useUpload` `bytesPerSec` / `UploadProgress` (including Vditor media).

### 4. Validation & Error Matrix

| Condition | Status | Code |
| --- | --- | --- |
| Empty `client_ids` | 400 | `INVALID_REQUEST` |
| Unknown client | 404 | `NOT_FOUND` |
| Valid POST | 200 | `{entries}` |
| GET allowlist | 200 | `{entries}` (may be empty) |

### 5. Good/Base/Bad Cases

- Good: gray page table with updated flag; POST `client_ids` from a multi-select.
- Base: empty allowlist GET `entries: []`.
- Bad: `device_id: string[]` textarea; per-line allowlist SDK; `import * as echarts from 'echarts'`.

### 6. Tests Required

- `yarn type-check` after generate. Live: GET allowlist 200; POST empty 400.

### 7. Wrong vs Correct

#### Wrong

```ts
await addVersionGrayAllowlist({
  path: { project_ref, version },
  body: { device_id: lines },
})
```

#### Correct

```ts
const { data } = await listVersionGrayAllowlist({ path: { project_ref, version } })
await addVersionGrayAllowlist({
  path: { project_ref, version },
  body: { client_ids: selected },
})
```

`source_version` on `POST …/artifacts/delta` is the **semver or integer**, not a
version UUID (`DeltaDialog.vue`).

---

## Scenario: Check preview 204 / 304 is “no update”

### 1. Scope / Trigger

`CheckPreviewDialog` calls generated-client `checkUpdate` with a **POST JSON body**
(`current_version`, `os`, `arch`, optional `channel`). Do not pass those as
query. Axios defaults treat 304 as an error. Empty/204/304 must show the
no-update alert, not `ApiError`. Integrity preview (`IntegrityDialog`) has no
`limit` / cursor control; leftover query params must not be sent.

### 2. Contracts

Pass `validateStatus: status => (status >= 200 && status < 300) || status === 304`.
If `status` is 204 or 304, or `data` is null/non-object, set `noUpdate` and skip
assigning `result`.

Integrity preview uses `getIntegrity` from `@/api/generated-client`.

---

## Scenario: Client announcements preview is a real GET

### 1. Scope / Trigger

`AnnouncementPreviewDialog` exercises `GET /api/v1/projects/:project_ref/announcements` through the client plane. Empty `{ "announcements": [] }` is a successful 200, not check’s 204/no-update alert. Locale leftover is a backend contract (`announcement-contract.md`); the tester must not fake matches on the admin SDK.

### 2. Signatures

- CRUD page: `pages/projects/[projectRef]/announcements.vue` — `listAnnouncements` / `getAnnouncement` / create / patch / delete / reorder from `@/api/generated`. Unwrap `data?.announcements ?? []`.
- Tester: `listClientAnnouncements` from `@/api/generated-client`.
- `StatusChip` `kind="announcement"` (`draft` / `published` / UI-only `scheduled` / `expired`).
- Do **not** reuse `LocaleTabs.vue` (changelog markdown-only). Announcement title/subtitle/content are scalar fields on one language row. Edit loads `getAnnouncement` so the list can omit `content`. Keep Save and the editor disabled until that GET succeeds (`bodyReady`); list JSON has no `content` key, so PATCHing the list row would wipe Markdown. Version scope uses Version UUID (`version_id`). Call `MarkdownEditor.flush()` before save. Media still uses `uploadProjectMedia`.

### 3. Contracts

Optional `version` / `os` / `arch` / `locale` (clearable; omit empty keys). `validateStatus` allows 200 and 304. Render HTTP status plus `JSON.stringify` of the envelope. Blank params are allowed so operators can test omitted match fields. Token-required projects reuse the shared Axios instance (not a substitute project token). Vite `/api/v1/projects` before `/api` remains mandatory.

### 4. Validation & Error Matrix

| Condition | Result |
|-----------|--------|
| Import `@/api/generated` for the tester | Hits admin plane / 404 / wrong envelope |
| Treat empty `announcements: []` as no-update like check 204 | False error or empty-state alert |
| Hand-built `/api/v1/projects/.../announcements` string | Dual-SDK / proxy drift |
| Reuse `LocaleTabs` for title+subtitle+markdown | Changelog editor breaks; missing title fields |

### 5. Good/Base/Bad Cases

- Good: published zh-CN-only row appears in tester JSON with `locale: "zh-CN"` when the locale field is blank. Explicit `locale=en` with only zh-CN rows returns `{ "announcements": [] }`.
- Base: draft-only project → tester 200 `{ "announcements": [] }`.
- Bad: admin list unwrap `data?.items`; mixing English title with Chinese subtitle in the editor payload.

### 6. Tests Required

- `yarn type-check` after adding generated-client calls.
- Manual: dedicated `/projects/:ref/announcements` tab; create each scope; tester with/without params; empty JSON is success.

### 7. Wrong vs Correct

#### Wrong

```ts
import { listAnnouncements } from '@/api/generated' // admin CRUD used as “preview”
applyRow(listItem) // list Omit content → PATCH "" wipes the body
await patchAnnouncement({ body: { content: listItem.content } })
```

#### Correct

```ts
import { listClientAnnouncements } from '@/api/generated-client'
const { data } = await getAnnouncement({ path: { project_ref, announcement_id } })
if (!data) return // keep Save disabled until bodyReady
await patchAnnouncement({ body: { content: editorRef.value?.flush() ?? data.content } })
```

### Common Mistake: edit-save from the list row

**Symptom**: After reload, GET-by-id Markdown is empty even though create looked successful.

**Cause**: Admin list/reorder omit `content`. Filling the form from the table row and PATCHing sends an empty string.

**Fix**: `openEdit` sets `bodyReady = false`, GETs the item, then enables Save. `save()` returns immediately when `editing && !bodyReady`.

**Prevention**: Do not treat list `AdminAnnouncement.content` as present. `useApiResource` is list-only; edit is a local `getAnnouncement`.

---

## Scenario: Project list cards consume list `stats` (no N+1)

### 1. Scope / Trigger

`pages/projects/index.vue` must show display name, slug, UUID, compare engine, latest version, catalog counts, token totals/active, physical `storage_bytes`, and 24h installed/failed. Backend `GET /api/v1/admin/projects` already attaches `stats` (`project-admin-contract.md`).

### 2. Signatures

- `listProjects` / `getProject` from `@/api/generated`. Unwrap `data?.projects ?? []`.
- Title: `project.name || project.slug`. Open: `/projects/${slug}/overview`.
- Parent `pages/projects/[projectRef].vue`: `replace` to `/overview` only when `route.name === '/projects/[projectRef]'`. Child routes (`/versions`, …) are not redirected.
- `DefaultShell` fetches caption via `getProject`; no `useProjectStore`.

### 3. Contracts

Format `storage_bytes` on the client. Fleet/summary tiles stay on the **unfiltered** loaded array (search is local). Copy in both locale files (`projects.latestVersion`, `projects.noVersion`, catalog/token/storage/telemetry keys).

Overview save: `Object.assign(form, project)` copies read-only `stats`/`uuid`; delete them before `updateProject`.

### 4. Validation & Error Matrix

| Condition | Result |
|-----------|--------|
| Per-card `listVersions` | N+1; list handler already batched stats |
| PATCH body includes `stats` | bind noise / 400 |
| Redirect every child of `[projectRef]` | versions URL lost |
| Delete confirm types display name | operators cannot match the slug gate |

### 5. Good/Base/Bad Cases

- Good: one list GET renders every card; create dialog name+slug (name may prefill from slug).
- Base: `latest_version` null → `projects.noVersion`.
- Bad: Pinia current-project store; matching UUID in list search (search is name/slug only).

### 6. Tests Required

- `yarn type-check` after using `Project.stats`.
- Manual: create with/without name; open project lands on `/overview`; PATCH name updates header and drawer; delete still requires slug.

### 7. Wrong vs Correct

#### Wrong

```ts
await updateProject({ path, body: { ...project } }) // includes stats + uuid
```

#### Correct

```ts
const payload = { ...form, cors_origins, rate_limit: rate }
delete payload.stats
delete payload.uuid
await updateProject({ path, body: payload as Project })
```

---

## Scenario: Vditor Markdown editor + `${site_url}` media

### 1. Scope / Trigger

Changelog tabs, one-shot release changelog, and announcement bodies are Markdown. Native `v-textarea` and unpkg CDN are forbidden. Images in Markdown cannot use expiring artifact signed URLs or `ClientProjectAuth` (no Bearer on `<img src>`).

### 2. Signatures

- `frontend/src/components/MarkdownEditor.vue` wrapping npm `vditor` (`^4.0.0`). Props: `modelValue`, `disabled?`, `placeholder?`, `minHeight?`, `projectRef`.
- `LocaleTabs` uses a project-language `v-select` plus one `MarkdownEditor` (forwards `disabled` + `projectRef` + `languages`). Start the draft map as `{}` and add a key when that locale is edited; do not initialize `{ 'zh-CN': '', en: '' }` or drop keys absent from the select. Version detail: `:disabled="version.status !== 'draft'"`.
- `DateTimeField.vue`: `includeTime` true → `v-date-input` + `v-time-picker` (24h); false → date-only (token expiry). `v-model` is RFC3339 or `''`.
- Upload: generated `uploadProjectMedia` (`file[]`) from `@/api/generated`.
- Vite plugin copies `node_modules/vditor/dist` → `public/vditor/dist` (dev) and `dist/vditor/dist` (build). `public/vditor` is gitignored.

### 3. Contracts

| Option | Value |
| --- | --- |
| `mode` | `ir` (toolbar keeps `edit-mode`) |
| `cdn` | `` `${import.meta.env.BASE_URL}vditor` `` (Vditor 4 loads `${cdn}/dist/...`) |
| `hint.emojiPath` | `${cdn}/dist/images/emoji` (default is unpkg) |
| `cache.enable` | `false` |
| `icon` | `material` |
| theme / lang | `useUiStore.resolvedName()` / `locale` (`zh_CN` / `en_US`) |
| `upload.max` | 10 MiB |
| Stored `v-model` | `${site_url}/api/v1/projects/{slug}/media/{id}` |
| Preview | replace `${site_url}` with `window.location.origin`; emit collapses origin (and `%24%7Bsite_url%7D`) back |

`plugins/vuetify.ts`: `defaults.VSwitch = { color: 'primary', inset: true }`. `is_critical` **switch** is primary; status **chips** may stay `color="error"`.

Announcement dialog `max-width="960"`. Allowlist, manifest JSON, and auto-publish copy stay `v-textarea`.

### 4. Validation & Error Matrix

| Condition | Result |
|-----------|--------|
| `cdn` omitted / unpkg | Admin console fetches jsdelivr/unpkg at runtime (offline/CDN-blocked fail) |
| `cache.enable` left default | Drafts leak across projects/versions via localStorage |
| Upload via Artifact TUS/presign | Version-line scoped; private signed URLs expire in `<img src>` |
| Non-draft version | `vditor.disabled()` including upload; Save stays disabled |
| `upload.format` missing when handler uses `response.JSON` | Vditor may not parse `{code,msg,data.succMap}` |

### 5. Good/Base/Bad Cases

- Good: create-version / draft version detail / release changelog / announcement body all show `.vditor-ir { display:block }`; resources load from `/vditor/dist/...`.
- Base: empty changelog still omits `changelog_i18n` on write (`changelogMap.ts` unchanged).
- Bad: second `MarkdownEditor` implementation; `new Pinia` for the editor; `LocaleTabs` reused for announcement title+subtitle.

### 6. Tests Required

- `yarn lint` / `yarn type-check`. `yarn build-only` leaves `dist/vditor/dist/js/lute/lute.min.js`.
- Manual: locale tab switch leaves one Vditor instance; published changelog `contenteditable=false`; DevTools has no unpkg/jsdelivr.

### 7. Wrong vs Correct

#### Wrong

```ts
new Vditor(el, { /* cdn defaults to https://unpkg.com/vditor@… */ })
```

#### Correct

```ts
new Vditor(el, {
  mode: 'ir',
  cache: { enable: false },
  cdn: `${import.meta.env.BASE_URL}vditor`.replace(/\/$/, ''),
  hint: { emojiPath: `${cdn}/dist/images/emoji` },
})
```

---

## Code Review Checklist

- `yarn lint` and `yarn type-check` (or `yarn build`) pass.
- No MSW / `VITE_USE_MOCK` / `src/mocks` / `/mock-s3`.
- New HTTP: generated SDK only; no `endpoints/` / `paths.ts` / `rawRequest`.
- File-route pages use typed `useRoute('/…/[param]')`; only layouts use a param cast.
- User strings in both `locales/zh-CN.ts` and `locales/en.ts`; API codes under `errors.<CODE>`.
- Presign: naked `{upload_url, method, direct_s3}` with `direct_s3` always false; never PUT unknown S3 keys.
- Vite `/api/v1/projects` proxy rule listed before `/api`. Hosted SPA uses admin D9 proxy, not a second origin.
- Dialogs that fetch on open: `watch(..., { immediate: true })` if mounted with `v-if` + already-true `v-model`.
- Unwrap generated envelopes (`?? []` on lists; audit `events`; version writes `changelog_i18n`).
- Gray allowlist GET exists; POST `{ client_ids }`; version chip is `actual_percent`. Delta `source_version` is semver/integer, not UUID.
- Check preview allows 204/304 as no update.
- Announcements preview uses generated-client combobox (versions + matrix, clearable); empty list is 200 JSON; omitted locale leftover is backend D5; explicit locale is strict.
- Project list cards use `data?.projects ?? []` (`name`, `latest_version`, version count, `active_7d`); no Token, no 24h telemetry bars, no UUID/mix, no ECharts on the card. Overview PATCH deletes `stats` and `uuid`. Bare `/projects/:ref` replaces to `/overview`. Delete confirm still types **slug**.
- Charts: `EChart.vue` only. Markdown bodies use `MarkdownEditor` (same-origin `/vditor`). `DateTimeField` for announcement windows, token date-only, and gray interval. Do not replace manifest/auto-publish textareas.
- Platform OS/arch on **new artifact lines** are matrix `v-select` only. Combobox helpers remain for filters (`osComboboxItems` / `archComboboxItems`).
- Changelog locale editors use project languages (`v-select`); save the full map and do not drop extra locale keys. Announcement save is one language row (`language`/`title`/`subtitle`/`content` + `version_id`). Preview locale is a clearable select (blank omits the query). Overview has no `default_locale` editor. Published changelog is editable from the versions list dialog.
- Page loading: `v-progress-circular` (block) or table `loading` / `UploadProgress` (upload speed). Do **not** add `v-skeleton-loader`.
- `release.vue` is 批量发版 (existing version + zip), not a create-version workbench. Version list row actions: changelog dialog + 产物管理 + 灰度管理. Version **detail** has no 产物/灰度 switcher.
- Switches/sliders/radios/checkboxes inherit `color: 'primary'` from `plugins/vuetify.ts` defaults so light/dark tracks use the theme, not a hardcoded hue.
- Settings store listings: generated `listStoreListings` / create / patch / delete; unwrap `data?.listings ?? []`; table column `store_url` (not `feed_url`); protocol keys are backend `Protocol()` (`electron`, not `electron-updater`); pins are clearable `v-select` from `listMatrix` / `listChannels`; `watch(projectRef)` must `listings.refresh()`.
- Install policy: `InstallPolicyEditor.vue` on settings (project scope) and a **wide** channels dialog (not the 480px form). Manifest dialog edits per-row `install_policy` with size/sha256/md5 visible. Unwrap `data?.entries ?? []`. Matrix slugs from `listMatrix`. On `projectRef` change, reset the matrix pair and reference channel to values that exist on the new project (do not keep the previous pair). `yarn generate:api` after OpenAPI; never hand-edit `src/api/generated/`.
- Settings PATCH: number `file_list_max_files` (0 = inherit; `:max` from GET `file_list_max_files_limit`); number `changelog_default_entries` / `changelog_max_entries` (`:max` from the matching `*_limit`); strip read-only `file_list_max_files_limit`, `changelog_default_entries_limit`, `changelog_max_entries_limit`, `storage_driver`, `stats`, and `uuid`. Include `store_token` **only** when the operator typed/generated a value or checked clear (empty string). Do not strip it unconditionally or every save will skip rotate/clear. After a rotate, show the response plaintext once (`CopyField`); GET never redisplays it. Do not render a public/private visibility control; always PATCH `public`. Hide `storage_bucket` when `storage_driver` is `local`.

---

## Scenario: Settings store listings CRUD

### 1. Scope / Trigger

Replace the nine-row `store_protocols` table with listing CRUD. Store listing identity is listing `slug`, not project `name`.

### 2. Signatures

```ts
listStoreListings({ path: { project_ref } }) // envelope { listings }
createStoreListing / updateStoreListing / deleteStoreListing
```

### 3. Contracts

- Call-site unwrap: `data?.listings ?? []`.
- Show `store_url` from the API; do not invent `/store/{protocol}/{doc}` without a slug.
- Pin OS/arch from matrix slugs, channel from `listChannels`; empty selection clears the pin (`""` on PATCH).
- `package_source=manifest_path` requires `manifest_path` (form validation + 400).

### 4. Validation & Error Matrix

| Condition | UI |
|-----------|-----|
| Duplicate `(protocol, slug)` | snackbar from `INVALID_REQUEST` |
| Project switch | `listings.refresh()` so the table is not stale |
| Loading with empty list | circular spinner, not `v-skeleton-loader` |

### 5. Good/Base/Bad Cases

- Good: two Sparkle rows, copy each `store_url`, old path 404s.
- Base: `line_full` default; pins optional.
- Bad: free-text os/arch; keeping listings loaded from the previous `projectRef`.

### 6. Tests Required

No frontend unit runner. HTTP: `listing_test.go`. Manual: settings page create two Sparkle listings.

### 7. Wrong vs Correct

#### Wrong

```ts
await updateProject({ body: { store_protocols: { sparkle: { enabled: true } } } })
```

#### Correct

```ts
const { data } = await listStoreListings({ path: { project_ref: projectRef.value } })
return data?.listings ?? []
```

---

## Scenario: Console loading, batch release, version list filters

### 1. Scope / Trigger

Admin UX after D1–D8: no skeleton screens; versions index filters by matrix pair; batch zip upload on `release.vue`; announcement scopes are select-only.

### 2. Signatures

- `listVersions({ query: { channel, status, os, arch } })` — never send `latest` from the versions index.
- Batch latest (if used): `listVersions({ query: { latest: true, os, arch } })`.
- Line prefill: `getVersionLineDefaults` (generated) with `os`+`arch`.
- Channel create: send locale-pack names via project create `system_channel_names` (or equivalent generated field).

### 3. Contracts

| Surface | Behavior |
| --- | --- |
| First paint / list fetch | Centered `v-progress-circular color="primary" indeterminate` or `v-data-table` `:loading` |
| Upload progress | `v-progress-linear` |
| Version row | Changelog dialog on list; 产物管理 routes to `[version].vue` |
| `release.vue` | Pick existing version + zip (+ optional publish). No OS/arch/changelog editors on this page |
| Announcement scope | `v-select` from existing versions and matrix rows only |
| Announcement status | `draft` / `scheduled` / `published`; `starts_at` required only for `scheduled` |

### 4. Validation & Error Matrix

| Condition | UI |
| --- | --- |
| Empty version list | `EmptyState`, not skeleton bones |
| Filter pair with no rows | empty table, still 200 unwrap `?? []` |
| Scheduled without start | do not PATCH; keep `starts_at` visible |

### 5. Good/Base/Bad Cases

- Good: switching light/dark keeps switch track on `primary`; filtering windows/x86_64 hits `listVersions` with os+arch and no `latest`.
- Base: unfiltered list omits query keys (`undefined`).
- Bad: `v-skeleton-loader`; sending `latest=true` from the versions index; free-typing announcement OS.

### 6. Tests Required

- `yarn lint` / `yarn type-check`.
- No `v-skeleton` under `frontend/src`.

### 7. Wrong vs Correct

#### Wrong

```vue
<v-skeleton-loader type="table-tbody" />
```

#### Correct

```vue
<v-progress-circular v-if="resource.loading" color="primary" indeterminate />
<v-data-table v-else :loading="resource.loading" :items="resource.items" />
```
