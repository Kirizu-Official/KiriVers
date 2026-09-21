# Announcement Contract

> Executable contract for project announcements. Admin CRUD lives on the admin plane; software clients use a dedicated GET. Do **not** store announcements on `Version`, embed them in `update/check` / changelog / store feeds, or call `SelectTarget` to match rows. Keep this document and `internal/controller/openapi.admin.json` (admin CRUD) / `openapi.client.json` (client GET + languages catalog) in sync with handler JSON.

HTTP CDN headers (ETag / Cache-Control / 304) are computed in `AnnouncementService.ListClient` from the announcements table. They are not the process catalog cache — announcement writes must not call `cache.InvalidateProject` (catalog is unchanged). See `cache-guidelines.md` for catalog I/O.

One row is one language. Columns are `language`, `title`, `subtitle`, `content` (Markdown). Version scope binds `version_id` UUID with **no FK** and no `version_ref` string.

Media for Markdown images/attachments is shared `project_media` (`uploadProjectMedia` / client GET). There is no announcement-specific blob table or `/announcements/.../files` route.

---

## Scenario: Client GET match, language filter, CDN

### 1. Scope / Trigger

- Trigger: `GET /api/v1/projects/:project_ref/announcements` (client plane, `ClientProjectAuth` + `store_per_ip_per_minute`).
- Rule: match the request identity (`version`, `os`, `arch`) against stored scopes. Do not resolve an update target.

### 2. Signatures

```go
// internal/service — no gin.Context; no SelectTarget.
func (s *AnnouncementService) ListClient(ctx context.Context, project *model.Project, in ClientListInput) (*ClientListResult, error)

func update.BuildLocaleChain(changelogLocale, locale, acceptLanguage, defaultLocale string) []string
func filterAnnouncementLanguage(candidates []ClientAnnouncement, explicit bool, locale, acceptLanguage, defaultLocale string) []ClientAnnouncement
```

Routes register only when `controller.Deps.Announcements != nil` (same nil-skip as telemetry). Client `GET /languages` is independent (catalog handler; see below).

### 3. Contracts

Query (all optional): `version`, `os`, `arch`, `locale`. `Accept-Language` participates **only** when `locale` is empty.

200 body:

```json
{ "announcements": [{ "id": "...", "title": "...", "subtitle": "", "markdown": "", "locale": "zh-CN", "starts_at": null, "ends_at": null }] }
```

`locale` is the row `language`. `markdown` is `content` after `${site_url}` expansion. Array order is display order (`sort_order`, then `created_at`, then id). Client items omit `status`, `sort_order`, `content`, and `version_id`.

Empty match set is **200** `{ "announcements": [] }`, never 204. Hosted admin SPA must reach this GET via D9 proxy (`quality-guidelines.md`); do not treat admin `NOT_FOUND` as an empty announcement list.

Scope match (lookup canonicalize query os/arch with `CanonicalOS` / `CanonicalArch`; empty query stays empty). Seven legal encodings; `announcementMatches` is the only matcher. Version match is UUID only: parse query `version` with `ParseVersionRef`, look up the Version in the project, compare to `VersionID`. Lookup miss skips version-scoped rows (project / OS / Arch rows still return). Deleted Version rows do **not** match leftover SemVer/`version_ref` strings.

| Stored scope | Keep when |
|--------------|-----------|
| Project-wide (version_id/os/arch empty) | always (if visible) |
| Version-only | query `version` resolves to the same Version UUID |
| OS-only | query os (canonical) equals stored os |
| Arch-only | query arch (canonical) equals stored arch |
| Version + OS | UUID and os equal |
| Version + Arch | UUID and arch equal |
| Version + matrix | UUID, os, and arch all equal |

OS+arch without version is **not** a scope (write 400; match never treats it as project-wide).

Visibility: `status=published` **or** `scheduled` whose `starts_at` is due (`now >= starts_at`) AND window contains request UTC (`starts_at` nil = already started; `ends_at` nil = open-ended). Lazy persist `status=published` on admin/client list (`flipDueScheduled`); `announcementVisible` must treat due scheduled as published even if the write fails. Drafts and future-`scheduled` / out-of-window rows stay on the admin list only.

Language (D5; feed-level because rows are independent):

- Candidate set = visible + scope match + non-empty title.
- Non-empty `locale` query: keep candidates whose `language` EqualFold the first tag. **No** default, **no** `Accept-Language`, **no** leftover. Empty set is 200 `[]`.
- Omitted/empty `locale`: `chain := BuildLocaleChain("", "", Accept-Language, project.DefaultLocale)`. First chain tag that appears on any candidate wins; if the chain misses entirely, leftover to the lexicographically smallest `language` among candidates. Return every candidate of that one language.
- `Vary`: include `Accept-Language` only when the `locale` query is empty.

Unknown but parsable `version`: skip version-scoped rows; still return project-wide, OS-only, and arch-only matches.

ETag: SHA-256 of a deterministic projection of **returned** items (id, updated_at, language, title, subtitle, **stored** placeholder markdown) plus project id. Expand `${site_url}` only on the HTTP body after the ETag is computed so origin churn does not flip ETags. `If-None-Match` → 304.

`Cache-Control` 200/304: `public, s-maxage=<cap>, stale-while-revalidate=30`. `cap = min(Project.CacheSMaxageSeconds, seconds until next future starts_at/ends_at among **published and scheduled** rows in the project)` (not only the matched subset). Minimum 1s if a boundary is sooner. Errors: `private, no-store`.

`Vary`: always `Accept-Encoding`; `Authorization` when `require_client_token`; `Accept-Language` when query `locale` is empty; `Referer` (markdown `${site_url}` is expanded from the Referer origin, falling back to the request origin; ETag still hashes stored placeholder markdown).

No `device_id`. Do not set `X-KiriVers-Protocol`.

### 4. Validation & Error Matrix

| Condition | Status | Code |
|-----------|--------|------|
| Unparsable `version` | 400 | `INVALID_QUERY_PARAM` |
| Missing/expired project | 404 | `PROJECT_NOT_FOUND` |
| Token required and missing/wrong | same as other client project routes | `UNAUTHORIZED` / `FORBIDDEN` |
| Empty matching set | 200 | envelope with `[]` |
| If-None-Match hits | 304 | empty body |

### 5. Good/Base/Bad Cases

- Good: published project-wide + matching version/os/arch/matrix rows, ordered by admin `sort_order`; due `scheduled` appears on client GET; zh-CN-only rows still returned when `locale` is omitted and the chain is `en` (leftover); explicit `locale=en` with only zh-CN rows returns `[]`.
- Base: no published visible rows → 200 `{ "announcements": [] }` + ETag.
- Bad: leftover on an explicit `locale` miss; mix zh subtitle onto an en title (impossible once rows are independent; do not invent maps); 204 for empty list; call `SelectTarget` to decide which notices apply; match a deleted Version via leftover SemVer string.

### 6. Tests Required

- Service: seven-scope union, omit `version`/`os`/`arch`, unknown SemVer vs unparsable, window edges, explicit locale miss vs omitted leftover, dual-language rows stay separate, `s-maxage` shrinks toward next published/**scheduled** boundary, `scheduled` past `starts_at` persists published, deleted Version skips version-scoped match.
- HTTP: 200 empty array key present; 304; `Vary` composition; client JSON has no `status`/`sort_order`/`content`/`version_id`.

### 7. Wrong vs Correct

#### Wrong

```go
// Explicit locale=en still leftover to zh-CN.
items := filterAnnouncementLanguage(candidates, false, "en", accept, defaultLocale)
```

#### Correct

```go
// Explicit locale is strict EqualFold. Leftover only when locale is omitted.
items := filterAnnouncementLanguage(candidates, explicitLocale, in.Locale, in.AcceptLanguage, project.DefaultLocale)
```

**Why**: Software clients that send `locale=en` must not receive a Chinese-only notice. Operators who omit `locale` still see zh-only copy when default/`Accept-Language` is `en`.

---

## Scenario: Admin CRUD, seven scopes, scheduled, reorder, audit

### 1. Scope / Trigger

- Trigger: `/api/v1/admin/projects/:project_ref/announcements` (list/get `project:read`; create/patch/delete/reorder `project:admin`).
- `release:publish` / `artifact:write` must not write (403 `FORBIDDEN`).

### 2. Signatures

```go
func (s *AnnouncementService) Create(ctx context.Context, projectID uuid.UUID, in AnnouncementCreateInput) (*model.Announcement, error)
func (s *AnnouncementService) Patch(...) (*model.Announcement, error)
func (s *AnnouncementService) Delete(...) error
func (s *AnnouncementService) Reorder(ctx context.Context, projectID uuid.UUID, ids []uuid.UUID) error // permutation of ALL ids
```

Table `announcements` (`model.Announcement`, `TableName()`). `VersionID` is nullable **without FK**. Limits: title/subtitle ≤ 256 runes; markdown ≤ 64 KiB; language must be a valid project language code; ≤ 500 rows per project. One language per row (no locale map). List/reorder `Omit` `content`; GET/POST/PATCH include it.

### 3. Contracts

Create is always `draft`; `sort_order = max+1`. Publish / schedule is PATCH `status` (`draft` \| `scheduled` \| `published`). `scheduled` **requires** `starts_at`. Past `starts_at` on save → persist `published`. Envelope list key `announcements`. GET by id and POST/PATCH return the item object (not wrapped). DELETE 204.

Admin item JSON: `id`, `status`, `sort_order`, `version_id` (uuid or null), `os`, `arch`, `language`, `title`, `subtitle`, `starts_at`, `ends_at`, `created_at`, `updated_at`. `content` only when `includeContent` (GET/POST/PATCH). Do not emit `version` string or a locale map.

Admin POST: `language` and `title` required; `subtitle`/`content` optional strings; `version_id`/`os`/`arch`/`starts_at`/`ends_at` nullable. PATCH pointers / `optionalJSON` for clearable fields including `version_id` null.

Deleting a `project_languages` row does **not** rewrite `announcements.language`. Admin form: **v-select only** from existing versions (`item-value` = Version UUID) and matrix OS/arch/rows (no combobox free-type). Console `save` must `flush()` MarkdownEditor before POST/PATCH. Edit must `GET` by id and keep Save disabled until that GET succeeds — list JSON has no `content` key, so PATCHing the list row would wipe Markdown.

Scope encoding (write canonicalize os/arch with **`CanonicalOSWrite` / `CanonicalArch`**):

| Scope | version_id | os | arch |
|-------|------------|----|------|
| Project-wide | empty | empty | empty |
| Version-only | set | empty | empty |
| OS-only | empty | set | empty |
| Arch-only | empty | empty | set |
| Version + OS | set | set | empty |
| Version + Arch | set | empty | set |
| Version + matrix | set | set | set |

Any other combo → 400 `INVALID_REQUEST`. Version-scoped writes require an existing Version in **this project** (`GetVersionByID` + `ProjectID` check) or 404 `VERSION_NOT_FOUND`. Version Line may be absent.

PATCH JSON null on `starts_at`/`ends_at`/`version_id`/`os`/`arch` clears the field (`optionalJSON`). `ends_at` before `starts_at` when both set → 400. `scheduled` without `starts_at` → 400.

Reorder: `PUT .../announcements/reorder` body `{ "ids": ["..."] }` must be a permutation of every announcement id in the project; rewrite `sort_order` 0..n-1.

Audit after success only (never block HTTP): `announcement.create` / `announcement.update` / `announcement.delete` / `announcement.reorder`.

### 4. Validation & Error Matrix

| Condition | Status | Code |
|-----------|--------|------|
| Illegal scope combo | 400 | `INVALID_REQUEST` |
| Empty title / invalid language / over limits | 400 | `INVALID_REQUEST` |
| `scheduled` without `starts_at` | 400 | `INVALID_REQUEST` |
| `ends_at` < `starts_at` | 400 | `INVALID_REQUEST` |
| Reorder not a full permutation | 400 | `INVALID_REQUEST` |
| Version scope, Version missing or other project | 404 | `VERSION_NOT_FOUND` |
| Unknown announcement id | 404 | `NOT_FOUND` |
| Token lacks `project:admin` on write | 403 | `FORBIDDEN` |
| Token has `project:read` on GET list | 200 | — |

### 5. Good/Base/Bad Cases

- Good: create one-language draft → 201 with `content`; list omits `content`; GET by id returns body; publish → client GET returns it; reorder permutation flips client order.
- Base: admin empty list is 200 `{ "announcements": [] }`.
- Bad: saving os+arch-without-version as project-wide; unique `sort_order` swaps; embedding notices on Version or check; POSTing a locale map into `content`; emitting `version` string from a dropped `version_ref`; PATCHing `content` from a list row (omitted key → empty wipe).

### 6. Tests Required

- HTTP AC coverage in `internal/controller/admin/announcement_test.go` (draft omission, publish/unpublish, scheduled require `starts_at`, past `starts_at` persists published, window, seven scopes, auth, audit actions, empty 200, UUID bind, foreign Version 404).
- Service illegal-scope table (os+arch without version); repository reorder permutation; `TestAnnouncementScheduledLazyFlip`; D5 language tests.

### 7. Wrong vs Correct

#### Wrong

```go
// Fold notices into check 200 or Version.changelog
result.Announcements = listVisible(...)
```

#### Correct

```go
// Dedicated GET. AnnouncementService does not import SelectTarget.
GET /api/v1/projects/:project_ref/announcements
```

**Why**: Changelog answers “what changed in this release.” Announcements are an independent notice channel; mixing them makes CDN ETags and update payloads change for unrelated copy edits.

---

## Scenario: Client language catalog

### 1. Scope / Trigger

`GET /api/v1/projects/:project_ref/languages` on the client plane (`ClientProjectAuth` + `store_per_ip_per_minute`), same group as channels/matrix.

### 2. Signatures

```go
func (h *catalogHandler) listLanguages(c *gin.Context)
```

Handler: `projects.ListLanguages` → `{ "languages": [{ "code", "display_name", "is_default", "sort_order" }] }`. Omit `project_id`. Register even when `Announcements` is nil.

### 3. Contracts

Empty list is 200. Project GET (`publicProjectSettings`) stays without a languages array. Not merged into announcement list. Auth and rate limit match channels/matrix. Admin language CRUD stays on `GET /api/v1/admin/projects/{project_ref}/languages`.

### 4. Validation & Error Matrix

| Condition | Status | Code |
|-----------|--------|------|
| Missing project | 404 | `PROJECT_NOT_FOUND` |
| Token required and missing/wrong | same as other client project routes | `UNAUTHORIZED` |
| Empty catalog | 200 | `{ "languages": [] }` |

### 5. Good/Base/Bad Cases

- Good: default language plus extra codes, ordered like admin; `require_client_token` accepts the project plaintext token.
- Base: project with only the default locale still returns that row.
- Bad: stuffing `languages` into client project GET; a second admin languages handler; omitting `languages` key on empty.

### 6. Tests Required

- Envelope key present; default row; extra code; `require_client_token` uses plaintext token.

### 7. Wrong vs Correct

#### Wrong

```go
// Fold the catalog into publicProjectSettings or announcement list.
body["languages"] = listLanguages(project)
```

#### Correct

```go
GET /api/v1/projects/:project_ref/languages
// catalogHandler.listLanguages → { "languages": [...] }
```

**Why**: Project GET stays a settings document; announcement GET stays notices. Clients that only need locale codes must not download markdown.
