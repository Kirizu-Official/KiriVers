# Component Guidelines

Console UI is Vue 3 `<script setup>` + Vuetify 4 (MD3). Prefer Vuetify components and utility classes over custom CSS.

---

## Component Structure

Typical SFC order in this repo: template → `<script lang="ts" setup>`. File-top comments name the component and its job (`ConfirmDialog.vue`, `PageHeader.vue`).

Reuse before inventing:

| Need | Existing component |
|------|--------------------|
| Page title + right-side actions | `PageHeader.vue` (`#actions` slot) |
| API failure on a list/detail page | `ErrorAlert.vue` (`error` prop) |
| Empty list | `EmptyState.vue` |
| Status badge | `StatusChip.vue` (`kind`: `'version' \| 'line' \| 'announcement'`) |
| Line packs-ready | Extra `v-chip` next to `StatusChip` (not a new `kind`): `packs_ready_at` → success `versions.packsReady`; `status===ready` without stamp → warning `versions.packsPending`. Copy in `en.ts` + `zh-CN.ts`. |
| Copyable id / token | `CopyField.vue` |
| TOTP otpauth QR | `OtpQr.vue` (`value` = `otpauth_url`) |
| Destructive confirm (optional typed word) | `ConfirmDialog.vue` (`v-model` + `confirm` emit) |
| Artifact upload | `UploadDialog.vue` (`useUpload`) |
| Upload percent / bytes / speed | `UploadProgress.vue` |
| Admin charts | `EChart.vue` (`echarts/core` + `vue-echarts`; never default `echarts` import) |
| Manifest editor | `ManifestDialog.vue` (per-row policy select + size/sha256/md5; watch open with `{ immediate: true }`) |
| Install policy templates | `InstallPolicyEditor.vue` (project settings expansion panel; channels **wide** dialog ≥ 860, not the 480px create/edit card). Unwrap `data?.entries ?? []`. Matrix slugs from `listMatrix`, never catalog `entry.name`. |
| Integrity preview | `IntegrityDialog.vue` |
| Client-plane check preview | `CheckPreviewDialog.vue` |
| Client-plane announcements preview | `AnnouncementPreviewDialog.vue` |
| Delta job dialog | `DeltaDialog.vue` |
| Changelog locale editor | `LocaleTabs.vue` (project-language `v-select` + one `MarkdownEditor`; call `flush()` on locale switch and changelog save; not announcement title/subtitle) |
| Markdown body (changelog / release / announcement) | `MarkdownEditor.vue` (Vditor `ir`; `projectRef` for upload). Cache `modelValue` as pending until Vditor `after()`; do not call `setValue` on an instance that is not ready. Expose `flush()` so parents persist the last toolbar/IME stroke before save. |
| Announcement window / token expiry | `DateTimeField.vue` (`includeTime` true = date+24h time; false = date-only) |
| Toast | `GlobalSnackbar` in `App.vue` via `useSnackbarStore` |

Reference: `components/ConfirmDialog.vue`, `components/PageHeader.vue`, `pages/admins.vue`.

---

## Props and `v-model`

- Props: `defineProps<{ ... }>()` with optional fields (`description?: string`).
- Boolean dialogs: `defineModel<boolean>({ default: false })` (see `ConfirmDialog.vue`).
- Parent that mounts a dialog with `v-if="target"` **and** opens it in the same tick must use `watch(model, ..., { immediate: true })` inside the dialog if the watcher loads data. A non-immediate watch misses the initial `true`.

### Common Mistake: dialog `v-if` + `v-model` without `immediate`

Found in `ManifestDialog.vue` (2026-09-13): parent sets `target` and `open` together; child mounts with `model === true`; `watch(model)` never fires.

```ts
watch(model, open => {
  if (open) void load()
  else editing.value = false
}, { immediate: true })
```

---

## Styling

- **Default:** Vuetify components (`v-card`, `v-btn`, `v-data-table`, …) plus Vuetify helper classes (`d-flex`, `ga-4`, `mb-6`, `text-h5`, `hidden-md-and-up`). Theme tokens live in `plugins/vuetify.ts` (`LIGHT_THEME` / `DARK_THEME`).
- **Tailwind** is on via `@tailwindcss/vite` (`styles/tailwind.css`). Do not build a second spacing/color system; new screens should look like `pages/projects/index.vue`, not like a Tailwind marketing page.
- **Exception:** `pages/login.vue` uses a scoped dark “glass” layout and custom CSS. Do not copy that pattern onto console pages.

TOTP enrollment (`login.vue` `enroll_totp`, `security.vue` rotate dialog) must render `OtpQr` from the API `otpauth_url`. Encode the QR in the browser; do not fetch a third-party chart image. Keep a white quiet zone so the code stays scannable on the dark login canvas. Keep `CopyField` for the secret (and otpauth URL) as a no-camera fallback.

`second_factor` keeps three tabs (TOTP / recovery / Passkey). Default to Passkey only when the pending result’s `has_passkey` is true; otherwise TOTP. Do not call `adminWebauthnLoginBegin` until the user clicks Use passkey.

`frontend/AGENTS.md`: follow Material Design 3; use Vuetify rather than hand-rolled widgets.

Page and table loading use `v-progress-circular` or `v-data-table` `:loading` / `v-progress-linear`. Do not add `v-skeleton-loader`. Selection controls (`VSwitch`, `VSlider`, `VRadioGroup`, `VCheckbox`) default to `color: 'primary'` in `plugins/vuetify.ts` so they follow the active theme.

---

## List search (unpaginated pages)

Admin `GET /api/v1/admin/projects` returns the full array. Search is a page-local `computed` over `useApiResource.items`, not a `q` query param, not a one-page composable, and not a Pinia store.

- Field: compact clearable `v-text-field` + `mdi-magnify` above the card grid. Leave PageHeader `#actions` for create.
- Match: trim; empty/whitespace shows all; case-insensitive substring on **name or slug**. Do not match UUID.
- Fleet / page-level summary tiles stay on the **unfiltered** loaded array. Only the card `v-for` (or table rows) uses the filtered list.
- Zero resources: existing `EmptyState` + create CTA; hide the search field.
- Non-empty query with zero hits: keep search and summaries; `EmptyState` **without** a create CTA (`projects.searchNoMatch`). Reuse `common.search` as the field label.

Reference: `pages/projects/index.vue`.

---

## i18n and errors

Every user-visible string goes through `useI18n()` / `t('…')`. Add keys to **both** `locales/zh-CN.ts` and `locales/en.ts`.

`ErrorAlert` maps `ApiError.code` → `errors.<CODE>` (`te` then `t`), else `err.message`. Mutation failures on pages often also `snackbar.show(..., 'error')` with the same `errors.<CODE>` lookup (`pages/admins.vue`). Independent forms on one page (changelog vs allowlist) each need their own error ref; sharing `saveError` hides the other failure.

Login wizard is special: password and second-factor failures are `UNAUTHORIZED` + `invalid credentials`. Do **not** map that to `errors.UNAUTHORIZED` (“session expired”). Use `auth.invalidCredentials`. `invalid token` / missing Bearer still use `errors.UNAUTHORIZED`.

---

## Accessibility (current bar)

- Icon-only controls use Vuetify `icon="mdi-…"` buttons (login, shell).
- Confirm destructive actions with `ConfirmDialog`; irreversible ones pass `confirmWord`.
- Drawer collapses below md (`DefaultShell.vue`, ~840px).
- Do not add `aria-*` theatre that the surrounding Vuetify component already provides.

---

## Forbidden

- Class components / Options API for new files.
- A one-off `v-dialog` that duplicates `ConfirmDialog` for “type the name to delete”.
- Hardcoded Chinese or English in templates (login feature bullets still go through `t()`).
