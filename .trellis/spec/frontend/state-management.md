# State Management

Pinia 3 setup stores (`defineStore('id', () => { ... })`) plus local SFC `ref`/`computed`. No Vuex.

---

## State categories

| Category | Where | Persistence |
|----------|--------|-------------|
| Full admin session | `stores/auth.ts` | `localStorage` key `kirivers.auth` (`accessToken`, `expiresAt`, `idleMs`, `admin`). `isAuthenticated` is true only for this token. |
| Pending 2FA login | `stores/auth.ts` | `sessionStorage` key `kirivers.auth.pending` (`pendingToken`, `stage`, `expiresAt`, enrollment secrets, `hasPasskey`). Not authenticated. |
| Theme + locale | `stores/ui.ts` | `localStorage` key `kirivers.ui` |
| Toasts | `stores/snackbar.ts` | memory only |
| In-flight admin jobs | `stores/jobs.ts` | memory only (backend has no job list) |
| List/detail server data | page / `useApiResource` | none — refetch |
| Form drafts, dialog open, tab | SFC `ref` | none |
| TUS resume cursor | `localStorage` key from `useUpload` (`kirivers.tus\|…`) | until success/cancel |
| Empty scaffold | `stores/app.ts` | unused — do not add fields |

URL is the project/version identity: `projectRef` and `version` come from the file route, not from a “current project” store.

---

## When to use a global store

Promote to Pinia only if **two of these** are true:

- More than one layout/page reads it (shell badge, snackbar, auth guard).
- It must survive leaving the page (full session, theme).
- A module without a component needs it (`api/client.ts` token via `registerAuthHooks`, not via importing the store).

Do **not** put `Project` / `Version` in Pinia. The parent page `pages/projects/[projectRef].vue` loads the project for the header; children call generated SDK methods again. That duplication is accepted. Do **not** add a Pinia store for Vditor or datetime pickers — theme/locale come from `useUiStore`; `v-model` stays on the page.

Drawer caption: `DefaultShell` may `getProject` when `projectRef` is set and `provide('setProjectCaption')`. The parent page injects that setter after load/save. `overview.vue` may `provide`/`inject` `projectRecord` so a name PATCH updates the header without a Pinia store.

---

## Auth and the API client

`auth` must not import `router` at top level; `client.ts` must not import `auth`. The handshake is:

1. `useAuthStore` construction calls `registerAuthHooks({ getToken, onUnauthorized, onSuccess })`.
2. Request interceptor attaches `Bearer` when `getToken(url)` is non-null. Pending tokens are attached only to `/auth/2fa` URLs.
3. 401 on a **full session** → `onUnauthorized` → `clear()` → dynamic `import('@/router')` → `/login?expired=1`.
4. 401 while pending: wrong TOTP/recovery (`invalid credentials`) keeps the wizard; `invalid token` (expired pending) clears `sessionStorage` only. Login page copy: `UNAUTHORIZED` + `invalid credentials` → `auth.invalidCredentials`, not `errors.UNAUTHORIZED` (that string means session expired).
5. Successful admin responses call `onSuccess` → slide local `expiresAt` by the idle TTL stored at login.

`isAuthenticated` treats the full session token as expired `EXPIRY_SLACK_MS` (30s) early. Guard: `router/index.ts` (`/login` is the only guest page; `/` redirects to `/projects`; `/admins` and `/geoip` redirect to `/projects` when `isPlatformAdmin` is false). `isPlatformAdmin` is `admin.is_platform_admin !== false` so sessions persisted before the flag still act as platform admins. Pending login is not authenticated.

`applyAuthResult` sets `hasPasskey` only when `status=pending`, `stage=second_factor`, and `has_passkey === true`. `login.vue` defaults the second-factor tab from that flag (Passkey vs TOTP) on password success and on restored pending. Do not auto-start WebAuthn. Do not `watch(stage)` in a way that resets a tab the user already chose this visit. Refresh reloads the default from persisted `hasPasskey`, not the last clicked tab. Missing `hasPasskey` on old blobs is TOTP.

---

## Jobs

`track(jobId, label, type, projectId)` after bundle/delta/CI release returns `{ job_id }`. `App.vue` `onMounted` calls `jobs.schedule()`. Poll with generated `getAdminJob` every 2s while status is `queued` or `running`. The bell in `DefaultShell.vue` reads `runningCount`.

---

## UI store

`mode`: `'light' | 'dark' | 'system'`. `bind()` (from `App.vue`) applies Vuetify theme names from `plugins/vuetify.ts` and listens to `prefers-color-scheme` when mode is `system`. Locale default `zh-CN`; `DefaultShell` writes both `ui.setLocale` and `i18n.global.locale`.

---

## Server state

Pages keep `Project` / `Version` as refs of generated types. Unwrap list envelopes at the call site (`data?.projects ?? []`). After PATCH/POST/DELETE, call `refresh()` or `load()` — there is no optimistic cache layer.

---

## Forbidden

- Extending `useAppStore`.
- Persisting jobs or list caches in `localStorage`.
- Importing `useAuthStore` inside `api/client.ts` (circular).
- A `useProjectStore` that shadows the route param.
