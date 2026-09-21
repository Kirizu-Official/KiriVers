# Frontend Development Guidelines

Vue 3 admin console (`frontend/`). Specs describe the repo as it exists: file routes, dual hey-api Axios SDKs, Pinia session/theme/jobs, no MSW.

---

## Overview

Package manager is yarn. UI is Vuetify 4 (MD3) with Pinia and vue-i18n. HTTP is Axios + `@hey-api/openapi-ts` (`@hey-api/client-axios`). Backend is a dual-plane Gin process (admin `:8081`, client `:8080`).

---

## Guidelines Index

| Guide | Description | Status |
|-------|-------------|--------|
| [Directory Structure](./directory-structure.md) | `pages` / `components` / `api` / `stores` layout | Filled |
| [Component Guidelines](./component-guidelines.md) | SFC, Vuetify, dialogs, i18n | Filled |
| [Composable Guidelines](./hook-guidelines.md) | `useApiResource`, `useUpload`, `useProjectNav`, `geoLabel` | Filled |
| [State Management](./state-management.md) | Pinia auth/ui/jobs/snackbar vs page state | Filled |
| [Quality Guidelines](./quality-guidelines.md) | Lint/build, dual-plane proxy, no MSW, presign, changelog/audit/allowlist/announcements | Filled |
| [Type Safety](./type-safety.md) | Generated types, optional fields, route params, ApiError | Filled |

---

## Pre-Development Checklist

Before editing `frontend/src`:

1. [Directory Structure](./directory-structure.md) — where the new file belongs; generated dual SDK vs `client.ts` only.
2. [Quality Guidelines](./quality-guidelines.md) — no MSW; proxy order; `yarn generate:api`.
3. [Type Safety](./type-safety.md) — typed `useRoute`; unwrap list envelopes at the call site.
4. If UI: [Component Guidelines](./component-guidelines.md) and [Composable Guidelines](./hook-guidelines.md).
5. If session/theme/jobs/toasts: [State Management](./state-management.md).
6. Shared: [Thinking Guides](../guides/index.md) (presign, dual-plane, no mock).

---

## Quality Check

- [ ] `yarn lint` and `yarn type-check` (or `yarn build`)
- [ ] No new `src/mocks`, `VITE_USE_MOCK`, or `/api/v1/` literals in pages/stores/composables
- [ ] File-route pages use `useRoute('/typed/path')`
- [ ] New copy in both locale files
- [ ] Unpaginated list search is a page `computed` over `useApiResource.items` (name/slug, unfiltered summaries, distinct no-match empty) — `component-guidelines.md`
- [ ] Presign / client-plane calls follow quality-guidelines
- [ ] List unwraps use live keys + `?? []`; version writes `changelog_i18n`; gray allowlist **has** GET (`client_ids`); `versions.rolloutHint` is allowlist + knobs, not HMAC percent (`quality-guidelines.md`)
- [ ] Did not hand-edit `src/api/generated/` or `src/api/generated-client/`
- [ ] Admin CRUD imports `@/api/generated`; check/integrity/announcements preview imports `@/api/generated-client`. Check preview is POST `checkUpdate({ path, body })`; IntegrityDialog has no limit control. Settings changelog numbers use GET `*_limit` as `:max` and strip those keys on PATCH (`quality-guidelines.md`)
- [ ] Announcements list unwraps `data?.announcements ?? []`; edit uses `getAnnouncement` and keeps Save disabled until GET succeeds; empty client preview JSON is 200 (`quality-guidelines.md`)
- [ ] `/admins` last-login cells use generated fields + `admins.neverLoggedIn` for JSON `null`; platform chip uses `is_platform_admin`; `/nodes` and project node-sync are platform-admin only (`quality-guidelines.md`)
- [ ] Project drawer + tabs share `projectNavItems` / `matchProjectNav`; settings is a separate route; overview country labels use `geoLabel` (`quality-guidelines.md`)
- [ ] `isAuthenticated` is full session only; pending 2FA lives in `sessionStorage` (including `hasPasskey`); login wizard maps `UNAUTHORIZED`+`invalid credentials` to `auth.invalidCredentials`; `second_factor` default tab is Passkey only when `has_passkey === true` and must not auto-start WebAuthn (`state-management.md`)
- [ ] Project list cards render `name` + compact `stats` (`latest_version`, version count, `active_7d`) from the list GET (no per-card version fetch, no Token/24h bars); overview PATCH omits `stats`/`uuid`; drawer caption is `getProject` + provide/inject, not `useProjectStore` (`quality-guidelines.md`, `state-management.md`)
- [ ] Changelog/release/announcement bodies use `MarkdownEditor` (Vditor `cdn` = Vite-copied `/vditor`, not unpkg); announcement windows + token expiry + gray interval use `DateTimeField`; charts use `EChart.vue`; allowlist is a table/GET not a textarea (`quality-guidelines.md`, `component-guidelines.md`)
- [ ] Platform OS/arch comboboxes use `osComboboxItems` / `archComboboxItems` (`slug` ∪ `COMMON_*`), never `entry.name` (`quality-guidelines.md`)
- [ ] Changelog locale editors use project-language `v-select` (`useProjectLanguages`) and do not drop extra map keys; announcement save is one language row; overview has no `default_locale` field
- [ ] No `v-skeleton-loader`; lists use circular or table loading; `release.vue` is batch zip onto an existing version (`quality-guidelines.md`)
- [ ] Do not `refresh()` a `useApiResource` from the same page’s first `load()`; `geoLabel` must not receive subdivision ISO as `code` (`hook-guidelines.md`)
- [ ] Settings store listings: generated SDK CRUD; unwrap `data?.listings ?? []`; pin OS/arch/channel from `listMatrix` / `listChannels` (clearable), not free-text; `watch(projectRef)` must `listings.refresh()`
- [ ] Install policy: `InstallPolicyEditor` in settings + channels wide dialog; Manifest table select + hashes; unwrap `data?.entries ?? []`; `yarn generate:api` after OpenAPI; do not hand-edit generated SDKs (`component-guidelines.md`, `quality-guidelines.md`)
- [ ] Version line chips: `packs_ready_at` → success `versions.packsReady`; ready but unstamped → warning `versions.packsPending`. Matrix `multi_file` rows show `delta_source_count` (preheat N), not `—`. Settings cleanup copy must say native `full` / `store_full` are never deleted. Both locales (`component-guidelines.md`)

---

**Language**: Specs in this folder are English (same as backend specs).
