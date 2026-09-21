# Composable Guidelines

This is a Vue 3 app. Shared stateful logic is a **composable** (`useXxx`), not a React hook and not a Pinia store-per-feature.

---

## Existing composables

| Composable | Role | File |
|------------|------|------|
| `useApiResource` | Mount-time list fetch: `items` / `loading` / `error` / `refresh`. Announcement **edit** still `getAnnouncement` locally because list omits `content`. | `composables/useApiResource.ts` |
| `useProjectLanguages` | Project language list unwrap (`data?.languages ?? []`); shared by languages tab, LocaleTabs, announcements, preview | `composables/useProjectLanguages.ts` |
| `useUpload` | Artifact upload state machine: direct PUT, TUS, presign; exposes `loaded` / `total` / `bytesPerSec` (EMA) | `composables/useUpload.ts` |
| `projectNavItems` / `matchProjectNav` | Drawer + project tabs share one list; highlight = longest `to` prefix | `composables/useProjectNav.ts` |
| `geoLabel` | Locale pick for fused GeoIP name maps (not a `useXxx`) | `composables/geoLabel.ts` |
| `useBuildInfo` | Process-wide build stamp: `frontend` (compile-time `__BUILD_INFO__`) + `backend` (`getBuildInfo`, fetched **once**) + `loading` / `failed` / `ensureLoaded()` | `composables/useBuildInfo.ts` |

### Exception: the one-shot build-info snapshot

Line 50 says server data is not a global cache. `useBuildInfo` is the deliberate
exception, and it is module-scope rather than Pinia because
`state-management.md` forbids adding fields to the empty `stores/app.ts` scaffold.

```ts
// state lives at module scope, so route changes and re-mounts reuse one request
const backend = shallowRef<BuildInfo | null>(null)
let requested = false
async function ensureLoaded (): Promise<void> {
  if (requested) return
  requested = true
  …
}
```

Rules that keep it honest:

- `DefaultShell` calls `ensureLoaded()` once on mount; the footer and `AboutDialog`
  only read the cached refs. Never `await` it per component (AC6: one request).
- Failures set `failed = true` and stop. No snackbar, no retry: missing build info
  must not break the console (the admin `getBuildInfo` call can legitimately 401 in
  a `pending` session and 404 while the DB is down).
- Values are opaque strings downstream. `unknown` / `dev` / `ca01b71-dirty` come
  from unstamped builds, so never `new Date(value)` into `Invalid Date` and never
  substitute the current time. Contract: `.trellis/spec/backend/build-info.md`.

Pages that load a **single** resource (project header) fetch in `onMounted` locally (`pages/projects/[projectRef].vue`). Do not force every GET through `useApiResource`.

`useApiResource` already GETs on mount. Do not also `refresh()` from the page’s `load()` on first paint (settings members list). `watch(projectRef)` may `refresh()` when the route param changes.

`geoLabel(names, locale, code?, unknown?)`: `names[locale]` → `names[lang]` → `names.en` → `Intl.DisplayNames([locale], {type:'region'}).of(countryIso)` → code → `unknown`. Only pass a **country** ISO as `code`. Subdivision codes such as `CA` would display as Canada.

Job polling is **not** a composable; it lives in `stores/jobs.ts` (`track` + `schedule`, 2s, session-only ids).

---

## Patterns

**List pages:**

```ts
const resource = useApiResource<Project>(async () => {
  const { data } = await listProjects()
  return data?.projects ?? []
})
```

Fetcher must return `T[]`. Errors stay as `unknown`; pass them to `ErrorAlert`.

**Uploads:** call `useUpload()` inside `UploadDialog` (or a parent that owns the file picker). Artifact `direct_s3` must stay false; throw if the API ever returns true. Details: `quality-guidelines.md` presign scenario.

**New composable:** only when two+ call sites share a state machine (phase, abort, progress). A single page’s `ref` + `try/catch` stays in the SFC. A list search on one unpaginated page is a `computed` filter over `items`, not `useXxxSearch`.

---

## Data fetching

- Admin HTTP imports `@/api/generated`. Check / integrity imports `@/api/generated-client`.
- Transport (Bearer, Idempotency-Key, `ApiError`) lives in `api/client.ts`. There is no `rawRequest` and no `endpoints/` unwrap layer.
- There is no Vue Query / SWR / Pinia Colada. Server data is not a global cache: refresh after mutations (`refresh()` or local `load()`).
- `useApiResource` uses `shallowRef` for the list to avoid deep reactive trees of API objects.

---

## Naming

- Files and exports: `use` + PascalCase (`useApiResource`, `useUpload`).
- Return a plain object of refs/functions (not a class).
- Do not prefix Pinia stores with `use` beyond the generated `useAuthStore` style.

---

## Common mistakes

- Reintroducing `endpoints/` wrappers around generated methods.
- Putting the admin session token or theme in a composable — that is `stores/auth` / `stores/ui`.
- Using `axiosInstance` for a leftover **direct S3** PUT of an unknown artifact key. `useUpload` must throw if `direct_s3` is true.
- Forgetting `{ immediate: true }` on a dialog watcher that the composable/page uses to start work (see `component-guidelines.md`).
- Hardcoding `/api/v1/` in a composable instead of calling the generated SDK.
- Passing `region_code` into `geoLabel` as `code` (subdivision ISO ≠ country ISO).
- Calling `members.refresh()` in `load()` when `useApiResource` already fetched on mount.
- Fetching build info per consumer (`AboutDialog` and the footer each calling `getBuildInfo`). Read `useBuildInfo`'s cached refs; `DefaultShell` owns the single `ensureLoaded()`.
