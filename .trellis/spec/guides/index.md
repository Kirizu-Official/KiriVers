# Thinking Guides

> **Purpose**: Expand your thinking to catch things you might not have considered.

---

## Why Thinking Guides?

**Most bugs and tech debt come from "didn't think of that"**, not from lack of skill:

- Didn't think about what happens at layer boundaries → cross-layer bugs
- Didn't think about code patterns repeating → duplicated code everywhere
- Didn't think about edge cases → runtime errors
- Didn't think about future maintainers → unreadable code

These guides help you **ask the right questions before coding**.

---

## Available Guides

| Guide | Purpose | When to Use |
|-------|---------|-------------|
| [Code Reuse Thinking Guide](./code-reuse-thinking-guide.md) | Identify patterns and reduce duplication | When you notice repeated patterns |
| [Cross-Layer Thinking Guide](./cross-layer-thinking-guide.md) | Think through data flow across layers | Features spanning multiple layers |

---

## Pre-Development Checklist

Before a change that spans packages or repeats an existing pattern:

1. [Cross-Layer Thinking Guide](./cross-layer-thinking-guide.md) — map storage → service → API → UI; `/health` `ready` probes DB and object storage only (not Redis cache); frontend presign / changelog / audit unwrap; no MSW.
2. [Code Reuse Thinking Guide](./code-reuse-thinking-guide.md) — search before adding a helper, Viper key, or payload field cast.
3. Package specs: [backend/index.md](../backend/index.md) and [frontend/index.md](../frontend/index.md).
4. Admin HTTP login: password → pending cache token → Postgres 2FA (TOTP/recovery/passkey) → full cache session. Do not cache 2FA secrets or `has_passkey`. `second_factor` pending JSON carries boolean `has_passkey` for the login default tab (no auto WebAuthn). `last_login_*` only when issuing a full session. Specs: [cache-guidelines](../backend/cache-guidelines.md), [error-handling](../backend/error-handling.md), [quality-guidelines](../backend/quality-guidelines.md), [state-management](../frontend/state-management.md).
5. Multi-node local-proxy: native check **and** store feeds use `private, no-store`; `GET /packages` prefers the local replica; register node UUID before job workers. Specs: [client-check](../backend/client-check-contract.md), [store adapter](../backend/store-adapter-contract.md), [storage](../backend/storage-guidelines.md), [database](../backend/database-guidelines.md).

---

## Quality Check

- [ ] Searched for an existing helper / constant / decoder before adding a new one
- [ ] Cross-layer fields (JSON, OpenAPI, i18n, BindEnv) updated together; admin `engines[].implementation` stays `cgo` \| `pure-go` with generated admin types (`backend/client-check-contract.md`)
- [ ] `/health` `ready` still probes DB and storage only (Redis cache failover is not `ready=false`); frontend still has no `src/mocks` / `VITE_USE_MOCK`
- [ ] New log lines use the correct YAML / stream / `mod` (`backend/logging-guidelines.md`)
- [ ] Frontend list unwrap uses generated envelope keys at the call site (`data?.events ?? []`, `data?.announcements ?? []`, `data?.languages ?? []`, `data?.listings ?? []`, `changelog_i18n`); OpenAPI property names must match the handler (route tests are not enough — `TestOpenAPIForbiddenNames` / `TestOpenAPIFieldTables`)
- [ ] Platform catalog items are `slug` not `name`; OS/arch comboboxes union `COMMON_*` (`frontend/quality-guidelines.md`)
- [ ] Admin batch-release latest for a pair uses `GET /versions?latest=true&os=&arch=` `latest`, not `SelectTarget` or N+1 `getVersion` (`backend/project-admin-contract.md`)
- [ ] Markdown editor assets are same-origin `/vditor` (not unpkg); media bytes go through `storage.Backend`, never a presigned URL in the document
- [ ] Official client SDKs stay on orphan `sdk/<lang>` worktrees; do not add `sdk/` on the default branch or merge those branches into `main` (`backend/directory-structure.md`)
- [ ] Server image/CI: musl linux for Alpine; PR does not publish; both Hub Compose files pass the URL-signing env; no host 5432/6379 (`backend/server-release.md`)

---

## Quick Reference: Thinking Triggers

### When to Think About Cross-Layer Issues

- [ ] Feature touches 3+ layers (API, Service, Component, Database)
- [ ] Data format changes between layers
- [ ] Multiple consumers need the same data
- [ ] You're not sure where to put some logic
- [ ] You are adding an event kind, JSONL record, RPC payload, or config field
- [ ] You are adding a log line: which YAML (`config` vs `admin`/`client`), which stream (system vs access), and which `mod` (see `backend/logging-guidelines.md`)
- [ ] UI / command code starts casting raw payload fields directly
- [ ] Readiness / health probes that depend on more than one backend (probe **all** of them; see `backend/storage-guidelines.md` and `backend/error-handling.md`)
- [ ] Frontend upload via presign: unwrap naked `{upload_url, method, direct_s3}`; `direct_s3` must stay false for artifacts — never PUT unknown S3 keys (`frontend/quality-guidelines.md`)
- [ ] Do not add MSW / `src/mocks` / `VITE_USE_MOCK`; the admin UI talks to the real dual-plane backend
- [ ] Admin HTTP: import `@/api/generated` (check/integrity/announcements preview: `@/api/generated-client`); unwrap `?? []` at the call site (`announcements`, `events`, `listings`); write `changelog_i18n`; gray allowlist GET + `{ client_ids }` (`frontend/quality-guidelines.md`)
- [ ] Admin project list: `name` + `stats` come from one GET; overview PATCH omits `stats`/`uuid`; no `useProjectStore`; settings listings are CRUD not a `store_protocols` map (`backend/project-admin-contract.md`, `frontend/quality-guidelines.md`)
- [ ] Native/store download URLs are SHA-256 (`PackageDownloadURL`); do not rebuild `/packages/{file_name}`; listing identity is slug and JSON `store_url` (`backend/client-check-contract.md`, `backend/store-adapter-contract.md`, `backend/project-admin-contract.md`)
- [ ] Leftover `GET /update/check` and `POST /clients/login` are 404 (no 301). Check leftover query is not a CDN key. Changelog `${site_url}` expands from RequestOrigin only (no Referer Vary); announcements still expand from Referer (`backend/client-check-contract.md`, `backend/announcement-contract.md`)
- [ ] Renaming a GORM column/jsonb key/`kind` needs SQL extras **before** AutoMigrate (`backend/database-guidelines.md`)
- [ ] `store_per_ip_per_minute` is the shared public-GET IP bucket, not store-only (`backend/telemetry-privacy.md`)
- [ ] Multi-file packs: hash the canonical unordered fileset (not the JSON array); zip only in `internal/service` Jobs; store `line_full` is `store_full` not hash-root `full`; D6/D7 compare Manifest `size` sums (`backend/client-check-contract.md`)
- [ ] Markdown in changelog/announcements: store `${site_url}`; changelog JSON expands from RequestOrigin; announcements still expand from Referer; media GET is UUID-public (`backend/storage-guidelines.md`, `frontend/quality-guidelines.md`)
- [ ] Two projects sharing SemVer 500: `idx_versions_project_semver` is not composite with `project_id` (`backend/database-guidelines.md`) — do not “fix” it in the Vue layer
- [ ] Official language SDKs: orphan `sdk/<lang>` worktrees + `openapi.client.json` snapshot at the package root; never `sdk/` on the default branch; language agents do not edit `internal/` (`backend/directory-structure.md`)
- [ ] Admin `engines[]` (`algo` + `implementation`) must stay in lockstep: `delta.DescribeAvailable` → handler JSON → `openapi.admin.json` enum `cgo`\|`pure-go` → `yarn generate:api` (revert client SDK if the client plane did not change). Do not restore `official-cli` or PATH `hdiffz` (`backend/client-check-contract.md`)

→ Read [Cross-Layer Thinking Guide](./cross-layer-thinking-guide.md)

### When to Think About Code Reuse

- [ ] You're writing similar code to something that exists
- [ ] You see the same pattern repeated 3+ times
- [ ] You're adding a new field to multiple places
- [ ] **You're modifying any constant or config**
- [ ] **You're creating a new utility/helper function** ← Search first!
- [ ] New Viper/YAML key: also `BindEnv` or `SetDefault` so `KIRIVERS_*` still unmarshals (`backend/quality-guidelines.md`)
- [ ] Serving the Vue admin from the Go process: NoRoute + `static_dir`, never `gin.Static`; **do** reverse-proxy `/api/v1/projects` onto the admin plane (D9, `backend/quality-guidelines.md`)
- [ ] Official docs: edit `website/` (not repo `docs/` as the VitePress root). User Compose is `deploy/compose.yml`; do not add root `compose.yml` or point `static_dir` at the docs-site dist (`backend/directory-structure.md`)
- [ ] Official server image: `dev/build/Dockerfile` Alpine + musl linux artifacts; D8/D9 in the `next-version` subcommand; server publish is manual-only (`workflow_dispatch`), never `on: push` or `on: tags` (`backend/server-release.md`)
- [ ] Plane `trusted_proxies` / `c.ClientIP()`: empty list must `SetTrustedProxies(nil)`; do not add the key to `config.yaml` (`backend/quality-guidelines.md`)
- [ ] Instance admin last-login JSON (`last_login_at` / `last_login_ip`) must stay in lockstep: model → `UpdateLastLogin` → `publicAdmin` → OpenAPI `Admin` → generated type → `/admins` i18n null (`backend/quality-guidelines.md`)
- [ ] Two files read the same untyped payload field with local casts
- [ ] Multiple branches update the same derived state from `kind` / `action`

→ Read [Code Reuse Thinking Guide](./code-reuse-thinking-guide.md)

### When Verifying AI Cross-Review Results

- [ ] Reviewer claims "user input can be malicious" → Check the actual data source (internal manifest? user config? external API?)
- [ ] Reviewer flags "missing validation" → Is the data from a trusted internal source?
- [ ] Reviewer says "behavior change" → Read the code comments — is it intentional design?
- [ ] Reviewer identifies a "bug" in test → Mentally delete the feature being tested — does the test still pass? If yes → tautological test

**Common AI reviewer false-positive patterns**:
1. **Trust boundary confusion**: Treating internal data (bundled JSON manifests) as untrusted external input
2. **Ignoring design comments**: Flagging intentional behavior documented in code comments as bugs
3. **Variable misreading**: Not tracing a variable to its actual definition (e.g., Map keyed by path vs name)

**Verification rule**: Every CRITICAL/WARNING finding must be verified against the actual code before prioritizing. Budget ~35% false-positive rate for AI reviews.

---

## Pre-Modification Rule (CRITICAL)

> **Before changing ANY value, ALWAYS search first!**

```bash
# Search for the value you're about to change
grep -r "value_to_change" .
```

This single habit prevents most "forgot to update X" bugs.

---

## How to Use This Directory

1. **Before coding**: Skim the relevant thinking guide
2. **During coding**: If something feels repetitive or complex, check the guides
3. **After bugs**: Add new insights to the relevant guide (learn from mistakes)

---

## Contributing

Found a new "didn't think of that" moment? Add it to the relevant guide.

---

**Core Principle**: 30 minutes of thinking saves 3 hours of debugging.
