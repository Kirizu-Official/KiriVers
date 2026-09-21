# Type Safety

TypeScript 5.9 (`vue-tsc --build --force`). There is **no** Zod/Yup/io-ts. Runtime checking of API JSON is the backend’s job; the console trusts interceptors + generated types.

---

## Type organization

| Layer | Source of truth |
|-------|-----------------|
| Admin OpenAPI | `internal/controller/openapi.admin.json` |
| Client OpenAPI | `internal/controller/openapi.client.json` |
| Admin SDK types | `src/api/generated/types.gen.ts` — **do not edit** |
| Client SDK types | `src/api/generated-client/types.gen.ts` — **do not edit** |
| Transport errors | `ApiError` / `isApiError` from `api/client.ts` |
| Route params | `src/typed-router.d.ts` via `useRoute('/path/with/[param]')` |

Pages, stores, and composables import entities and `*Input` types from `@/api/generated` or `@/api/generated-client`. Do not reintroduce handwritten `Project` / `Version` / `Artifact` interfaces.

hey-api marks many JSON fields optional. After a successful GET, use optional chaining and `??` (empty string, `0`, `[]`) at the call site or in the template. Do not invent a second required-field entity layer.

---

## Call-site unwrap

Generated calls return `{ data }`. List envelopes match Gin `response.JSON` keys. Unwrap at the call site:

```ts
const { data } = await listProjects()
return data?.projects ?? []
```

```ts
const { data } = await listAuditLogs({ path: { project_ref }, query: { limit: 50 } })
return data?.events ?? []
```

```ts
const { data } = await listAnnouncements({ path: { project_ref } })
return data?.announcements ?? []
```

Always `?? []` on list envelopes so a missing array does not throw in `useApiResource`. Flatten GET `changelog` (`ChangelogMap`) in `components/changelogMap.ts` for LocaleTabs; write `changelog_i18n` on PUT/PATCH. Do not put `whitelist` on `Version` — allowlist is POST/DELETE only. Use generated `AdminAnnouncement` scalars (`language`, `title`, `subtitle`, `content`, `version_id`); do not hand-edit `src/api/generated*`.

Admin login:

```ts
const { data } = await adminLogin({ body: { username, password } })
```

`data` is already `AdminLoginOutput`. There is no `rawRequest`. Binary / TUS / multipart use generated methods (`putLineArtifact`, `createTusUpload`, `createCiRelease`). Direct S3 PUT uses interceptor-free `import axios from 'axios'`.

When a generated call’s TypeScript union still includes `AxiosError` (generic `ThrowOnError` defaults to `false`), pass `throwOnError: true` so `response.headers` is typed. Runtime already throws because `client.setConfig({ throwOnError: true })`.

---

## Route params

Bare `useRoute()` is a union of every page’s params (`TS2339` on `projectRef`).

**In a page SFC** pass the route name from `typed-router.d.ts`:

```ts
const route = useRoute('/projects/[projectRef]/versions/[version]')
const projectRef = computed(() => route.params.projectRef)
```

**In shared layout** (`layouts/DefaultShell.vue`, `App.vue`) keep untyped `useRoute()` and narrow:

```ts
const projectRef = computed(() => (route.params as { projectRef?: string }).projectRef)
```

`login.vue` may use bare `useRoute()` (query `expired` / `redirect` only).

---

## Errors

Catch as `unknown`. Narrow with `isApiError` from `api/client.ts`. Do not type failures as `any`. `ApiError` fields: `status`, `code`, `message`, `details`.

Check preview: that call must `validateStatus` allow 204 and 304. Empty body / 204 / 304 means no update, not `ApiError`.

---

## Forbidden

- `any` on new code (eslint `eslint-config-vuetify` + `ts: true`).
- Editing `generated/`, `generated-client/`, or `typed-router.d.ts` to “fix” a type.
- Unwrapping presign as `(data as { PresignUploadOutput }).PresignUploadOutput` — the SDK `data` **is** the naked object (`quality-guidelines.md`).
- Importing Vue APIs from `'axios'` or axios from `'vue'` (seen as a typecheck failure in upload work).
- Handwritten entity types in `api/types.ts` (that file must not exist).
- Casting list envelopes to a second DTO instead of using generated `*ListOutput` / `data?.versions`.
