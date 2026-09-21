# Client Check Endpoint Contract

> Executable contract for `POST /api/v1/projects/:project_ref/update/check` and `GET /api/v1/projects/:project_ref/changelog/:channel/:os/:arch`. Device geo report is `POST .../clients/report` ([telemetry-privacy.md](./telemetry-privacy.md)); leftover `POST .../clients/login` is 404. Any change to target selection, ETag, or cache headers must keep this document and `internal/controller/openapi.client.json` in sync. Project announcements are a **separate** GET — see [announcement-contract.md](./announcement-contract.md). Do not add announcement fields to check/changelog bodies or call `SelectTarget` from announcement matching. Hop is `min_source_version` only; channel tokens use `X-Channel-Token` (never 403 on check mismatch; changelog mismatch is 404). Client channel/matrix catalogs are in this file. Check is **not** a CDN object: anonymous complete gray is `private, max-age=0, must-revalidate`. Changelog and packages carry shared cache.

---

## Scenario: Update check (POST, private cache)

### 1. Scope / Trigger

- Trigger: cross-layer client-facing API contract (Gin handler → `service/update` → repository catalog loader), CDN cache semantics, Ed25519/RSA response signing.
- Rule: `SelectTarget` in `internal/service/update` is the ONLY target-selection implementation. Store feeds (Sparkle, electron-updater, …) must call it with "anonymous, no device_id, default hw_rev" — forking the algorithm is forbidden.

### 2. Signatures

```go
// internal/service/update — pure function over a loaded snapshot; no IO inside.
func SelectTarget(cat *Catalog, req Request) (Result, error)
// Result.Target == nil → 204; ErrIntermediateUnavailable → 409; ErrNoSafeTarget → 409

// grayHitFor — allowlist membership until GrayIsComplete (is_critical or gray_completed_at).
// HMAC / gray_salt / rollout_percent are gone. pkg/grayutil is leftover and MUST NOT be called from check.
func grayHitFor(cat *Catalog, cand *VersionState, _ *LineState, dev grayDevice) bool

// pkg/signature — payload fields joined with "\n": integer, semver, root_hash, package_url, size, sha256
func SignPayload(algo, privateKeyPEM, payload string) (string, error) // base64(Ed25519 | RSA-SHA256-PKCS1v15)
```

### 3. Contracts

- JSON body (required): `current_version`, `os`, `arch`; optional: `channel`, `hw_rev`, `os_version`, `device_id` (registry upsert + allowlist gray), `capabilities` and `accepted_delta_algos` as string arrays. Leftover `protocol_version` and leftover `locale` are ignored. Changelog fields are not check inputs. `GET /update/check` is unregistered (404; no 301).
- Optional header `X-Channel-Token` (trim space). Distinct from project `X-Project-Token`. Wrong/missing channel token is **not** 401/403 on check: `channelAllowedForUpgrade` skips that channel; other public-channel candidates still win. Never put the plaintext token in ETag, logs, or cache keys. Admin list/get return `token` from `channels.token_plain`; check still hashes the header against `token_hash`.
- Leftover names `local_sha256`, `dirty_paths`, `changelog_scope`, `changelog_layout`, `changelog_locale` must not appear as accepted OpenAPI parameters (`TestOpenAPIForbiddenNames`). Changelog lives on `GET /changelog/:channel/:os/:arch`.
- Hop: only `Version.MinSourceVersion` (column `required_intermediate_version` is dropped). Winner is the highest compare-key candidate; if current is below that winner’s min_source, rewrite the target along the min_source chain (Published + ready line + hw + D7 channel allowed), max 8 hops. Do **not** skip the blocked newest candidate to pick the next-highest. Missing/not-ready/cycle/>8 → 409 `INTERMEDIATE_UNAVAILABLE`.
- Line min OS/API: `lineMeetsOS` uses **VersionLine** `min_os` / `min_api_level` only (AND if both set). No `platform_matrix` fallback (those columns are dropped). Current line below floor → 409 `MIN_OS_NOT_MET`; candidate skip is silent.
- ETag: SHA-256 over the deterministic sorted projection of the Catalog (versions, line statuses, root_hash, `gray_completed_at`, allowlist **count + max(created_at)** — never member ids, channels including `unlisted`/`token_protected` booleans **not** hashes, matrix, project floor). Check ETag **omits** Version changelog text and project ChangelogScope/Layout/ClientOverride/IncludeRevoked; it **keeps** ChangelogIncludeNotes (gates `platform_notes`). **Never** an input: `device_id`, request body, `X-Channel-Token` plaintext. Completing gray or growing the allowlist must flip the ETag. Editing changelog text must not flip the check ETag.
- Cache-Control matrix:

| Case | Header |
|------|--------|
| 200/204 default (including anonymous complete gray) | `private, max-age=0, must-revalidate` (never public s-maxage; `cache_s_maxage_seconds` does not apply to check) |
| catalog has any non-draft version with `GrayIsComplete()==false` | `private, max-age=0, must-revalidate` (never s-maxage; anonymous and allowlisted differ) |
| `X-Channel-Token` present **or** catalog has any `TokenProtected` channel | `private, max-age=0, must-revalidate` (never s-maxage) |
| `clusterActive && cluster.download=local` (200/204) | `private, no-store` (per-node replica visibility; do not CDN-cache) |
| Any error (404/409/400) | `private, no-store` |

- `Vary`: always `Accept-Encoding`; + `Authorization` when `require_client_token`; + `X-Channel-Token` when the catalog has any TokenProtected channel. Check does not expand changelog and does not Vary on Referer. Changelog `${site_url}` expands from **RequestOrigin only** (ignore Referer / Accept-Language; no Referer Vary). Announcements still expand from Referer (see [announcement-contract.md](./announcement-contract.md)).
- 200 body: no `display_version`, no per-file URL array (multi-file = one pre-generated full zip), no `changelog` / `changelog_versions` (use `GET /changelog/:channel/:os/:arch`), no `hash_tree_url` / `diff_url` (clients build integrity and diff URLs). `platform_notes` remains, gated by project `changelog_include_platform_notes`. `signature` and `artifact_signature` are separate fields.

### 4. Validation & Error Matrix

- unknown `current_version` → 404 `VERSION_NOT_FOUND` (never treated as 0)
- revoked version / yanked current line → downgrade branch; no safe target → 409 `NO_SAFE_TARGET`
- min_source hop missing/not-ready/cycle/>8 hops → 409 `INTERMEDIATE_UNAVAILABLE`
- `os_version` below current line's own `min_os`/`min_api_level` → 409 `MIN_OS_NOT_MET`; candidate-level min_os/min_api failure silently skips the candidate
- wrong/missing `X-Channel-Token` → **not** 403; skip token-protected channels; other public-channel updates still 200, else 204
- leftover `changelog_scope` / `changelog_layout` / `changelog_locale` on **GET** check → 404 (method unregistered). POST leftover query is ignored (not bound).
- POST check never emits `PRECONDITION_FAILED` (that belongs to specified-target integrity/diff flows)

### 5. Good/Base/Bad Cases

- Good: anonymous check, stable channel, published higher version with ready line → 200 + private cache + ETag; body has `platform_notes` and no `changelog` / `changelog_versions`.
- Base: no update → 204 with ETag + Cache-Control (still 304-able). Editing Version changelog text does not flip the check ETag.
- Bad: putting changelog query params on GET check (404); hashing Version changelog into the check catalog ETag.

### 6. Tests Required

- Table-driven `select_test.go`: cross-channel-only-up, revoke/yank downgrade, min_source hops/cycle, token-protected skip/fallback, platform isolation, LTS→2.x, hw variants, allowlist gray hit/miss, anonymous miss until `gray_completed_at`.
- Handler tests: 304 on both 200 and 204 base states; Cache-Control private / incomplete-gray-private / token-private / local-proxy `private, no-store` / error; Vary composition including `X-Channel-Token`; full 200 snapshot (assert absence of `display_version`/`files`/`server_protocol`/`changelog`/`changelog_versions` and of `X-KiriVers-Protocol`); leftover GET check → 404; `TestCheckEndpointTokenFallbackNo403`.
- ETag trigger test: publish/revoke/yank/gray complete/allowlist grow/min_source change each flips ETag (`TestCatalogETagTriggers`). Changelog text edits must not.
- Parity test when replicating `service.ParseVersionRef` locally (`parse_parity_test.go`).

### 7. Wrong vs Correct

#### Wrong

```go
body["changelog"] = BuildChangelog(...)
etag := hashOf(cat, version.Changelog) // check 304 dies on every changelog save
```

#### Correct

```go
// POST /update/check: no changelog fields; ETag omits Version changelog text.
// GET /changelog/:channel/:os/:arch owns scope/layout/locale and its own ETag.
```

---

## Scenario: Changelog GET (not check)

### 1. Scope / Trigger

Native client changelog is only `GET /api/v1/projects/{project_ref}/changelog/{channel}/{os}/{arch}`. Pathless `GET /changelog` is 404 (no 301). Check, integrity, and diff must not embed `changelog` / `changelog_versions`. Store-protocol documents still project Version changelog (Sparkle `<description>`, electron notes, …).

### 2. Signatures

```
GET /api/v1/projects/{project_ref}/changelog/{channel}/{os}/{arch}
```

Query: `from_version` optional, `to_version`, `changelog_scope`, `changelog_layout`, `changelog_locale`. No client `limit` query. Project defaults apply when override is allowed. Path `channel` is required.

### 3. Contracts

- Unknown path channel → 400 `INVALID_QUERY_PARAM`. Token-protected channel with missing/wrong `X-Channel-Token` → 404 `NOT_FOUND` (not 403). Output is filtered to the path channel. Comparison bounds may use other-channel version refs.
- No `from_version` → newest `default_entries` after channel / include_revoked / range_platform filters (product default 5; `target_only` without `to_version` → 1 latest of the path channel). With `from_version` → `(from, to]` newest-first truncated to `max_entries` (instance default 50). Overflow is HTTP 200, never 400.
- Instance YAML `changelog.default_entries` / `max_entries`: after `<1` fallback, if default > max then `config.Load` returns error (cmd/server fatal, do not listen). Do not clamp or swap. Project columns `changelog_default_entries` (new default 5) and `changelog_max_entries` (0 = inherit). PATCH above instance ceiling or project default > effective max → 400 `INVALID_REQUEST`. Admin GET includes readonly `*_limit`.
- 200 body may include `changelog` / `changelog_versions` per scope/layout. Markdown `${site_url}` expands from RequestOrigin only. Do **not** Vary on Referer. ETag hashes the **truncated** entry set that is returned.
- Check does not Vary on Referer and does not expand changelog placeholders.

### 4. Validation & Error Matrix

| Condition | Status | Code |
|-----------|--------|------|
| Bad scope/layout | 400 | `CHANGELOG_QUERY_INVALID` |
| Unknown path channel | 400 | `INVALID_QUERY_PARAM` |
| Token mismatch | 404 | `NOT_FOUND` |
| Pathless `GET /changelog` | 404 | `NOT_FOUND` |
| Leftover GET check | 404 | `NOT_FOUND` |
| Illegal `from_version` | 400 | `CHANGELOG_QUERY_INVALID` |
| Unknown `from_version` / `to_version` | 404 | `VERSION_NOT_FOUND` |
| YAML `changelog.default_entries` > `max_entries` after `<1` fallback | process | `config.Load` error; do not listen |

### 5. Good/Base/Bad Cases

- Good: `GET /changelog/:channel/:os/:arch` with project scope/layout still returns logs after check no longer does. No `from_version` returns 5 newest. Overflow truncates at 200.
- Base: store adapter Sparkle `<description>` still contains version changelog.
- Bad: hashing only the aggregated presentation for changelog ETag (nil entries → constant hash → stale 304); returning 400 when the range exceeds `max_entries`.

### 6. Tests Required

- Changelog handler tests keep scope/layout assertions plus no-from default 5, truncate 200 (newest-first, exactly `max` items), token 404, unknown channel 400, illegal from 400, unknown version 404.
- `config.Load` / `TestChangelogLimitsDefaultAndRejectInverted`: default > max fails; `<1` falls back then compares.
- Project PATCH over `*_limit` or default > effective max → 400 `INVALID_REQUEST`.
- Check 200 asserts those keys are absent.
- Sparkle (or any store adapter) still writes changelog into the protocol document.

### 7. Wrong vs Correct

#### Wrong

```go
entries, _ := BuildChangelog(...)   // entries == nil under aggregated layout
etag := hashOf(entries)             // constant ETag → stale-forever 304
```

#### Correct

```go
entries, _ := BuildChangelog(...)   // full set; layout only decides presentation
etag := hashOf(entries) // after newest-first truncate; hash the returned set
body := renderByLayout(entries, layout)
```

---

## Scenario: Incremental allowlist gray (not HMAC)

### 1. Scope / Trigger

Gray is version-level allowlist membership plus `gray_completed_at`. `SelectTarget` stays the only picker; only `grayHitFor` changed. Feeds use `AnonymousVisible` → `GrayIsComplete()` (same as old 100%). Empty client namebook completes immediately (full push, including anonymous).

### 2. Signatures

```go
func (v *Version) GrayIsComplete() bool // is_critical OR gray_completed_at != nil
func (v *Version) GrayIsActive() bool   // published && !complete && gray_started_at != nil

func GrayCoverage(n, start, step, interval int, startedAt, now time.Time) (targetPercent, desired int)
func RankClientsForGray(remaining []Client, weighted bool) []Client

func (s *ProjectService) EnsureGrayAdmission(ctx, project, version) (*repository.GrayAdmitResult, error)
func (s *ProjectService) EnsureProjectGrayAdmission(ctx, project) error // every published incomplete version
```

HTTP (admin): `GET/PATCH .../versions/{v}/gray`, `POST .../gray/complete`, `GET .../gray/clients`, `GET .../gray/series`, `GET/POST/DELETE .../gray/allowlist` with `{ "client_ids": [uuid] }` (not a raw device textarea). There is **no** per-line allowlist and **no** `rollout_percent`.

### 3. Contracts

- Gate: mandatory path skips gray (unchanged caller). Else complete → hit; else anonymous miss; else allowlist contains hashed (or raw-policy plaintext) device → hit.
- Coverage: `k = floor(elapsed/interval)`; `target = min(100, start + k*step)`; `desired = ceil(N * target / 100)`. Only append. Lazy catch-up on check, report, and admin gray GET/PATCH. Knob PATCH does **not** reset `gray_started_at`.
- Complete when `N==0`, `target>=100`, `allowlist_count>=N`, or operator complete. Then anonymous check and public feeds see the version.
- Weight: `projects.gray_weight_tenure_activity` default **true**. On: remaining-set `age_rank * recency_rank` DESC, `id` ASC. Off: `created_at, id` ASC. `NULL last_check_at` recency rank = 1.
- `device_id_policy=none`: do not insert a `clients` row (same as missing device_id).
- Check with `device_id` upserts operational registry fields and does **not** clobber report/registry `custom` JSON.
- “Updated” on the gray page: parse `client.last_version` with `ParseVersionRef` and compare to the gray Version via `compare_engine`. Unparsable → not updated. Not telemetry `installed`.

### 4. Validation & Error Matrix

| Condition | Status / gate |
|-----------|----------------|
| `is_critical` publish | cannot enter gray; `GrayIsComplete` true |
| Empty namebook + start gray | `gray_completed_at` set immediately |
| Anonymous while incomplete | miss / feeds omit |
| Manual `client_ids` POST | 200; members visible on next check |
| Policy none + report | 400 `INVALID_REQUEST` |

### 5. Good/Base/Bad Cases

- Good: t0 `start=10`, N=4 → admit `ceil(0.4)=1`; later ticks catch up; empty N completes.
- Base: publish without gray sets `gray_completed_at` (explicit complete).
- Bad: calling `grayutil.Hit`; putting device ids in ETag; using HMAC percent; keeping `version_line_id` on allowlist.

### 6. Tests Required

- `internal/service/gray_admission_test.go`: t0 percent, tick catch-up, empty→complete, weight order, default weight on.
- `internal/service/update/gray_test.go`: allowlist hit/miss, anonymous miss then hit after complete, critical bypass.
- Admin `TestVersionListShowsActualGrayPercent` (chip is `allowlisted/N*100`, not `gray_start_percent`).

### 7. Wrong vs Correct

#### Wrong

```go
if grayutil.Hit(cand.Version.GraySalt, deviceID, cand.Version.RolloutPercent) { ... }
```

#### Correct

```go
if cand.Version.GrayIsComplete() { return true }
if dev.RawDevice == "" { return false }
return cand.Allowlist.contains(allowlistKey(cat, dev))
```

---

## Scenario: Fileset pack and no request-path zip

### 1. Scope / Trigger

- Trigger: client-plane pack API, `VersionLine.PacksReadyAt` visibility, D6/D7 uncompressed gates, and zip builders that must not run on the Gin goroutine.
- Multi-file clients compare integrity locally, then `POST /update/pack` with `needed_paths`. The same URL is used to enqueue and to poll. `POST /update/diff` is lookup-only (`binary_delta` / `patch_package` / `file_list` / `full_package`) and must not enqueue packs. There is no `dirty_paths` field. There is no `POST /update/pack/status`.

### 2. Signatures

```
POST /api/v1/projects/:project_ref/update/pack
```

```go
func CanonicalFilesetSHA256(entries []FilesetEntry) string
func ResolveNeededFileset(raw []string, manifest []model.ManifestEntry) FilesetResult
func Pack(ctx, cat, details, runtime, maxBytes, PackInput) (*PackResult, error)

type PackRuntime interface {
    LookupFilesetPatch(ctx, lineID, hw, filesetSHA) (*PackArtifact, error)
    LookupFilesetJob(ctx, projectID, key) (*PackJob, error)
    EnqueueDynamicPack(ctx, DynamicPackRequest) error
}
```

Idempotency key: `dynamic-pack:{line_id}:{hw}:{fileset_sha256}`. Config: `dynamic_pack.max_bytes` / `KIRIVERS_DYNAMIC_PACK_MAX_BYTES` (default 512MiB). Rate limit: `diff_per_device` / `diff_per_ip`. Do not expose admin `GET /jobs/:id`.

### 3. Contracts

- Body: `source_version`, `target_version`, `os`, `arch` required; `needed_paths`, `channel`, `device_id`, `hw_rev` optional. Same identity/gray/channel rules as diff. `Cache-Control: private, no-store`. No ETag. First POST may enqueue; later POSTs with the same fileset find the job.
- Official SDKs omit local `KEEP_IF_EXISTS` paths from `needed_paths` when the file already exists (OpenAPI). Hash-gated KEEP (omit only if the on-disk SHA-256 matches Manifest) is stricter and optional; the server hashes whatever unique NFC paths it receives.
- Canonical fileset: `\`→`/`, NFC (`pathutil.NormalizeAndValidatePath`), unique by path, then SHA-256 of sorted `{nfc_path}\n{manifest_file_sha256}\n`. JSON array order and Windows vs POSIX separators must not change the hash. os/arch still partitions lines.
- Lookup/queue key = Version Line + hw + `fileset_sha256`. Identical filesets coalesce (GetByIdempotencyKey before insert).
- Catalog ETag includes `packs_ready_at`. Check/store/pack/diff/integrity against a line with nil stamp → `VERSION_NOT_VISIBLE`. Only **system** `auto_delta` (channel × `delta_source_count`) stamps the line. Client pack jobs must not. **Local-proxy** (`clusterActive && cluster.download=local`): this process also nils `PacksReadyAt` for lines whose `{slug}/{sha256}` replica is missing (`WithReplicaGate`); S3-direct does not copy packages and uses the global stamp alone.
- `POST /update/diff`: ALWAYS `private, no-store`, never an ETag. Full-package `url/size/sha256` from the SAME artifact row check 200 serves. No enqueue. No `dirty_paths`. `file_list` is opt-in via `capabilities` and capped by the catalog effective max (next scenario).
- Integrity returns **all** files in one response (no `cursor` / `limit` / `next_cursor`). `include_file_urls` is still only honored when total files ≤ 16. Integrity ETag = `"<line.RootHash>"` (single-file falls back to package SHA-256 when RootHash is empty).
- Native `kind=full` / `kind=patch` zip members are SHA-256 hex (hash-root). Store `line_full` uses `kind=store_full` (path zip). HTTP handlers never import `archive/zip`.

### 4. Validation & Error Matrix

| Condition | HTTP | Body |
|-----------|------|------|
| Draft target or nil `packs_ready_at` | 404 | `VERSION_NOT_VISIBLE` |
| Revoked target | 409 | `VERSION_REVOKED` |
| Gray miss on specified non-mandatory target | 412 | `PRECONDITION_FAILED` |
| Any valid path missing from Manifest | 200 | `status=full_package`, no job |
| Unique needed Manifest `size` sum ≥ `dynamic_pack.max_bytes` (D6) | 200 | `full_package`, no job |
| Same sum ≥ 70% of **all** target Manifest entry sizes including KEEP (D7; never zip `Size`) | 200 | `full_package`, no job |
| Artifact `size>0` for this fileset | 200 | `status=ready` + pack fields |
| Queued/running job, no artifact | 202 | `status=pending` (no second job) |
| Else (no artifact, no in-flight job) | 202 | enqueue one `dynamic_pack` → `pending` |
| Job failed with no artifact | 200 | `full_package` |
| D6/D7 oversize | **not** 400 | do **not** emit `NEEDED_PATHS_TOO_LARGE` |

Ready payload includes `diff_mode` / `compression` / `package_url` / `size` / `sha256` / `file_name` / `files` (path+sha256+size+install_policy) / `deleted_paths` / `signature` so apply can skip a separate diff.

### 5. Good/Base/Bad Cases

- Good: same Manifest set as `foo\\bar` then `foo/bar`, or two shuffled `needed_paths` arrays → one `fileset_sha256`, one Job, one `kind=patch`.
- Base: system preheat (3 channels × `delta_source_count`) stamps `packs_ready_at`; check/feed then see the line; a unique client fileset still returns 202 `pending`.
- Bad: hashing the raw JSON array; comparing compressed zip bytes to `kind=full` zip `Size`; zipping in `controller/client` or `service/update`; stamping `packs_ready_at` from a client pack Job; polling admin `GET /jobs/:id`.

### 6. Tests Required

- `fileset_test.go`: order / `\` vs `/` / NFC vs NFD invariance (AC19).
- Pack HTTP: cache hit `ready`; D6/D7 zero new jobs; concurrent identical filesets one Job; poll the same `POST /update/pack` until pending then ready (no separate status route).
- Visibility + ETag: `packs_ready_at` in catalog hash; unstamped line not in check/feed (`TestCatalogETagTriggers`, select/feed tests).
- Guard: `TestDiffPathHasNoPackingImports` (go/parser over production `controller/client` + `service/update`; forbid `archive/zip`, `archive/tar`, `compress/*`).
- OpenAPI: `POST /update/pack` in `openapi.client.json`; `TestOpenAPIRoutesSync` / `TestPlaneSpecsValid`. No `pack/status` path.

### 7. Wrong vs Correct

#### Wrong

```go
buf := buildZipFromPaths(needed) // FORBIDDEN on the HTTP path
if zipSize >= 0.7*fullZip.Size { return full } // compressed Size is not D7
```

#### Correct

```go
// HTTP returns pending; Job in internal/service writes the zip
res.Status = PackStatusPending
if neededSum >= int64(0.7*float64(fullManifestSum)) { return fullPackage } // uncompressed
```

---

## Scenario: Diff `file_list` cap (platform + project clamp)

### 1. Scope / Trigger

Native `POST /update/diff` may return `diff_mode=file_list` (pending-new-file URL array). The cap is no longer a hardcoded 16: platform YAML is the ceiling; each project may inherit or lower it. Integrity `include_file_urls` page size is a different gate and stays out of this contract.

### 2. Signatures

```
file_list.max_files            # config.yaml / KIRIVERS_FILE_LIST_MAX_FILES
projects.file_list_max_files   # int not null default 0
```

```go
const DefaultFileListMaxFiles = 16
func EffectiveFileListMax(platform, project int) int
func WithFileListMaxFiles(n int) Option // inject into update.Service + ProjectService
```

Catalog `ProjectInfo.FileListMaxFiles` is the **already clamped** effective max. `Diff` is a pure function over that snapshot.

### 3. Contracts

- Platform default 16; load-time `<1` → 16.
- Project `0` (or unset) inherits the platform ceiling. PATCH `1…ceiling` stores an override. PATCH above the current ceiling → 400 `INVALID_REQUEST`. GET returns stored `file_list_max_files` plus read-only `file_list_max_files_limit` (live platform ceiling) for the console.
- Effective = project≤0 → platform; else `min(project, platform)`. Platform later lowered still clamps dirty rows at catalog load (`applyFileListCeiling`).
- `file_list` only if `capabilities` includes `file_list`, upgrade is not a downgrade, source line is healthy, and pending-new-file count ≤ effective max with a `kind=file` artifact per path. Otherwise silent fallback to archive (`full_package` / delta). Undeclared capability never emits `files[]`.

### 4. Validation & Error Matrix

| Condition | Behavior |
|-----------|----------|
| Project 0, platform 16 | file_list allowed up to 16 pending files |
| Project 4, 5 new files | no `files[]`; archive fallback |
| PATCH 32 while platform 16 | 400 `INVALID_REQUEST` |
| Stored 32, platform 16 | Diff uses 16 |
| No `file_list` capability | never `diff_mode=file_list` |

### 5. Good/Base/Bad Cases

- Good: project 8, 3 new files, capability declared → `file_list` with 3 URLs.
- Base: omitted YAML key → 16 via SetDefault/BindEnv.
- Bad: hardcoding `const FileListMaxFiles = 16` in `diff.go`; Diff reading YAML itself; using a new error code instead of `INVALID_REQUEST` for over-ceiling PATCH.

### 6. Tests Required

- `internal/config`: default / env / `<1` clamp (`TestFileListMaxFilesDefaultEnvAndClamp`).
- `model.EffectiveFileListMax` table (`project_filelist_test.go`).
- `diff_test.go`: project cap + dirty catalog clamp; undeclared capability has no `files[]`.
- Admin PATCH over ceiling 400 (`project_test.go`).

### 7. Wrong vs Correct

#### Wrong

```go
const FileListMaxFiles = 16 // process-wide; ignores YAML and project
if pending > 16 { return full }
```

#### Correct

```go
cat.Project.FileListMaxFiles = model.EffectiveFileListMax(platform, stored)
if files, ok := buildFileList(..., fileListMax(cat)); ok {
    mode = DiffModeFileList
}
```

---

## Scenario: Binary delta identity and hdiffpatch CGO (§7.2)

### 1. Scope / Trigger

- Trigger: cross-layer delta objects (storage identity), admin `engines[]` JSON + OpenAPI enum, and CGO libHDiffPatch vs historical `KVDIFFHP1`.
- Identity matching (C10-3/C10-6) is unchanged; only the **hdiffpatch generate/apply implementation** and `engines[].implementation` enum changed.

### 2. Signatures

```go
// internal/delta — Engine stays []byte Diff/Patch (no context; C++ cannot be cancelled).
func Get(algo string) (Engine, error)           // unknown → ErrUnsupported → 400 DELTA_ALGO_UNSUPPORTED
func DescribeAvailable() []EngineInfo           // admin POST delta response engines[]
const ImplCGO = "cgo"
const ImplPureGo = "pure-go"

// internal/delta/hdiffc — CGO wrap; requires CGO_ENABLED=1.
func Create(oldData, newData []byte) ([]byte, error) // uncompressed HDIFF13, magic HDIFF13&
func Apply(oldData, diff []byte) ([]byte, error)

// wrap.cpp (extern "C")
int kv_hdiff_create(...) // create_compressed_diff(..., compressPlugin=NULL, threadNum=1)
int kv_hdiff_patch(...)  // patch_decompress_mem; compressedCount/compressType → -5
void kv_hdiff_free(uint8_t*)
```

Vendor tag is `third_party/hdiffpatch/VERSION` (currently `v4.12.2`). Admin OpenAPI: `engines[].implementation` enum `cgo` | `pure-go` in `openapi.admin.json`.

### 3. Contracts

- Delta objects are matched by **content identity, not version refs**: `local_sha256` must equal the source package SHA-256 AND the delta row's `DeltaSourceSHA256`/`DeltaTargetSHA256` must match the resolved full-package hashes (version refs are display-only for `patch_package`). Multi-file patch zips are kind=patch artifacts matched by fileset SHA (dynamic pack) or by SHA identity of BOTH full packages (preheat). Auto-generation (`auto_delta` job) triggers from PublishVersion and line-ready seams, excludes revoked versions and yanked/disabled lines, skips a source when uncompressed D6/D7 gates fire, and stamps `packs_ready_at` when the line preheat succeeds. Generation and consumption never compare compressed zip Size to the full zip.
- Stable delta filename embeds the FULL 64-hex source and target SHA-256; job idempotency keys on `(target line, algo, srcsha, dstsha)`.
- Algorithm selection: `accepted_delta_algos ∩ available`, preferring `matrix.delta_algo`, deterministic lexicographic fallback. Unknown algos in client requests are tolerated (empty intersection → full); unknown algos in ADMIN generation requests → 400 `DELTA_ALGO_UNSUPPORTED`.
- Magics: bsdiff `BSDIFF40`; xdelta3 RFC 3284 `D6 C3 C4` (not ASCII); hdiffpatch generate = uncompressed `HDIFF13&` via `hdiffc.Create`; historical `KVDIFFHP1\n` is **Patch-only** (`internal/delta/hdiffpatch.Patch`). Unknown magic → error. Never cross-decode (do not feed `HDIFF13&` to the KVDIFFHP1 decoder or the reverse).
- `hdiffPatchEngine.Diff` only calls `hdiffc.Create`. Failures return as-is; **no** `KVDIFFHP1` generate fallback; **no** `os/exec` of `hdiffz`/`hpatchz`.
- Uncompressed HDIFF13 is `create_compressed_diff` with a **NULL** compress plugin (same as unparameterized `hdiffz old new diff`). Raw `create_diff()` is a different header — do not use it. Compressed HDIFF13 (`-c-*` / non-empty `compressType`) is out of scope: `kv_hdiff_patch` returns `-5`; do not link zlib/zstd.
- `DescribeAvailable`: hdiffpatch is always `cgo`; bsdiff and xdelta3 are always `pure-go`. Do not PATH-probe. Do not emit `official-cli`.
- Official server: `CGO_ENABLED=1` + C++ toolchain. `CGO_ENABLED=0 go build .` must fail (`hdiffc` build constraints exclude all Go files). Go SDK stays `CGO_ENABLED=0`. Darwin CGO release builds are macOS-hosted (see `docs/delta-engines.md`).
- Empty old/new must roundtrip (`TestEngineEdgeCases`). Wrap uses a non-NULL placeholder for zero-length buffers. Go copies with `GoBytes`/`copy` then `kv_hdiff_free`. C++ `catch (...)` → `-3`. Job cancel does not abort an in-flight C++ call.
- KVDIFFHP1 decoder treats all lengths as uint64 with subtraction-form bounds checks — crafted headers must error, never panic (`TestFallbackRejectsCraftedHeaders`).

### 4. Validation & Error Matrix

| Condition | Result |
|-----------|--------|
| Admin unknown `delta_algo` | 400 `DELTA_ALGO_UNSUPPORTED` |
| Client unknown algo in `accepted_delta_algos` | ignored; empty intersection → full package |
| Engine Diff CGO failure | error; no KVDIFFHP1 fallback |
| Patch magic `KVDIFFHP1\n` | pure-Go `hdiffpatch.Patch` |
| Patch magic `HDIFF13&` | `hdiffc.Apply` |
| Patch unknown magic | error naming both magics |
| Compressed HDIFF13 (`compressType` set) | `hdiffc apply: compressed HDIFF13 is unsupported` (`-5`) |
| `CGO_ENABLED=0 go build .` | compile failure |
| `engines[].implementation` = `official-cli` | forbidden; OpenAPI enum and `TestDescribeAvailableCGO` / `TestArtifactHTTP_DeltaJob` |

### 5. Good/Base/Bad Cases

- Good: CGO Diff starts with `HDIFF13&`; engine Patch restores `new`; a stored `KVDIFFHP1` fixture still Patches; `hdiffpatch.Patch` on official bytes is `ErrBadMagic`.
- Base: `DescribeAvailable` reports three algos; hdiffpatch `cgo`.
- Bad: `exec.LookPath("hdiffz")`; `create_diff()` instead of `create_compressed_diff(..., NULL)`; generating `KVDIFFHP1` when CGO fails; restoring `official-cli`; Linux→darwin CGO without Apple SDK as a release path.

### 6. Tests Required

- `go test ./internal/delta/hdiffc/...`: `TestCreateApplyRoundTrip` (magic `HDIFF13&`), empty buffers, `TestApplyRejectsCompressedType`, parallel Create.
- `go test ./internal/delta/...`: `TestEngineRoundTrip` **requires** hdiffpatch `HDIFF13&` (KVDIFFHP1-only is not success); `TestEnginePatchHistoricalKVDIFFHP1`; `TestEngineOfficialNotCrossDecoded`; `TestDescribeAvailableCGO`; `TestEngineEdgeCases`; `TestFallbackRejectsCraftedHeaders`.
- Admin: `TestArtifactHTTP_DeltaJob` asserts hdiffpatch `cgo` and no `official-cli`.
- OpenAPI: `openapi.admin.json` enum `cgo` \| `pure-go`; `TestOpenAPIRoutesSync|TestPlaneSpecsValid|TestOpenAPIForbidden|TestOpenAPIFieldTables` then `yarn generate:api`.
- AC6: `CGO_ENABLED=0 go build .` fails.

### 7. Wrong vs Correct

#### Wrong

```go
path, _ := exec.LookPath("hdiffz")
return hdiffpatch.Diff(old, new) // KVDIFFHP1 generate fallback
create_diff(newp, newp+n, oldp, oldp+m, diff) // not HDIFF13&
```

#### Correct

```go
return hdiffc.Create(old, new) // create_compressed_diff(..., compressPlugin=NULL)
// Patch: KVDIFFHP1 → hdiffpatch.Patch; HDIFF13& → hdiffc.Apply; else error
```

**Why**: Official `.hdiff` clients (historical `hpatchz`) need uncompressed HDIFF13. `create_diff()` is a different container. PATH CLI caused generate/apply split-brain. `KVDIFFHP1` remains only so already-stored objects still Patch.


---

## Scenario: Private-project signed download URLs (§13.7)

- Signing happens at OUR download handler layer (LocalFS/S3 uniform): `sig = hex(HMAC-SHA256(secret, "GET\n{path}\n{exp}"))` in query `?exp=&sig=`; stable path/filename unchanged. Secret: instance `url_signing_secret` (startup warning if unset — restart invalidates URLs); TTL `Project.SignedURLTTLSeconds` (0 → 3600).
- For `StorageVisibility=private`: check/integrity SKIP the 304 short-circuit (always fresh 200 — a 304 would pin a client to an expiring URL) and use `Cache-Control: private, no-store`. ALL download URLs are signed (package_url, full_package_url, file_list/integrity per-file URLs, pack ready/full_package URLs); the §12.1 response signature payload is computed AFTER URL signing so it covers the signed URL.
- Public projects: signer nil → URLs byte-identical, direct download, normal 304/public cache semantics.
- Download handlers return a UNIFORM 403 for missing/expired/tampered signatures (no artifact-existence leak), verified before any storage access.

---

## Scenario: min_source hop and channel token (D3 / D7)

### 1. Scope / Trigger

`SelectTarget` in `internal/service/update/select.go` is the only hop and channel-gate implementation. Store feeds stay anonymous (no `X-Channel-Token`, public channels only). Do not restore `required_intermediate_version` or matrix `min_os` / `min_api_level`.

### 2. Signatures

```go
func SelectTarget(cat *Catalog, req Request) (Result, error)
func hopViaMinSource(...) (Result, error) // max 8 hops
func channelAllowedForUpgrade(cat *Catalog, req Request, slug string) bool
func lineMeetsOS(line *LineState, osVersion string) bool
```

`Request.ChannelToken` is the trimmed `X-Channel-Token` header. `Channel.TokenHash` is SHA-256 hex; compare with `hmac.Equal` after hashing the header (`service.ChannelTokenMatches`).

### 3. Contracts

- Winner = highest compare-key upgrade candidate. If `candidateBlockedByMinSource`, rewrite to the first hop on that candidate’s `min_source_version` chain that the client can take (Published, ready line, hw, `channelAllowedForUpgrade`). Example: 3.0 min_source=2.0, 2.0 min_source=1.5, 1.5 unrestricted, client at 1.0 → first check returns 1.5, then 2.0, then 3.0.
- Unlisted: usable if the client’s **current** channel slug equals it **or** query `channel=` equals it. Otherwise skip (do not 404 the whole check).
- TokenProtected: require matching header. Mismatch → skip that channel only.
- Token plaintext never appears in JSON, ETag projection (`token_protected` bool only), or logs.

### 4. Validation & Error Matrix

| Condition | Status | Code |
|-----------|--------|------|
| Hop missing / not ready / cycle / >8 | 409 | `INTERMEDIATE_UNAVAILABLE` |
| Wrong channel token, public channel still newer | 200 | public-channel target |
| Wrong channel token, no other update | 204 | empty |
| Unlisted channel without query/current slug | skip | not 403 |
| Current line min OS/API unmet | 409 | `MIN_OS_NOT_MET` |

### 5. Good/Base/Bad Cases

- Good: 1.0 → 1.5 → 2.0 → 3.0 hop chain; matching token selects insider; wrong token falls back to stable.
- Base: no token, no TokenProtected channels → public cache as before.
- Bad: 403 on wrong `X-Channel-Token`; skipping the blocked newest candidate to serve 2.9 when 3.0 is gated by min_source; reading min OS from matrix.

### 6. Tests Required

- `TestSelectTarget_TokenProtectedFallback`, hop tables in `select_test.go`.
- HTTP `TestCheckEndpointTokenFallbackNo403`.
- Cache-control token branch in `TestCheckCacheControlBranches`.

### 7. Wrong vs Correct

#### Wrong

```go
if ch.TokenProtected && !tokenOK {
    return 403, "FORBIDDEN"
}
// pick next-highest candidate when winner is min_source-blocked
```

#### Correct

```go
if !channelAllowedForUpgrade(cat, req, slug) {
    continue // skip channel; other candidates remain
}
return hopViaMinSource(...) // rewrite winner, do not pick candidates[1]
```

**Why**: Channel tokens are per-channel secrets, not project auth. A browser with a stale insider token must still receive the public stable update. Hop is a rewrite of the newest target, not a search for an older unconstrained release.

---

## Scenario: Client channel and matrix catalogs

### 1. Scope / Trigger

Slim client GETs for software clients (not admin CRUD). Register on the client plane with `ClientProjectAuth`. Do not return token hashes, `gray_salt`, or unused matrix columns.

### 2. Signatures

```
GET /api/v1/projects/:project_ref/channels          → { "channels": [...] }
GET /api/v1/projects/:project_ref/matrix            → { "matrix": [...] }
GET /api/v1/projects/:project_ref/languages         → { "languages": [...] }
```

```go
func publicClientChannel(ch *model.Channel) gin.H // name, slug, stability_rank only
```

### 3. Contracts

- List: `enabled && !unlisted && !TokenProtected()`. Empty list is **200** `{ "channels": [] }`.
- There is no client `GET /channels/:slug`. Hidden-channel upgrades use check `channel=` + `X-Channel-Token`. An unregistered slug path is plane NoRoute JSON 404 (`NOT_FOUND`), not `CHANNEL_NOT_FOUND`.
- Matrix list: every row’s `os`, `arch`, `package_type` only.
- Languages list: `{ "languages": [] }` envelope; omit `project_id`.

### 4. Validation & Error Matrix

| Condition | Status | Code |
|-----------|--------|------|
| Token-protected / unlisted / disabled row on list | omit | — |
| Empty public list | 200 | `{ "channels": [] }` |
| `GET /channels/:slug` | 404 | `NOT_FOUND` (unregistered) |

### 5. Good/Base/Bad Cases

- Good: list omits unlisted and token-protected rows; matrix returns `os`/`arch`/`package_type` only.
- Base: no public channels → 200 empty array.
- Bad: listing TokenProtected channels; 403 on wrong `X-Channel-Token` during check; leaking `token_hash`.

### 6. Tests Required

- `internal/controller/client/catalog_test.go` (list omit, get-by-slug unregistered 404).

### 7. Wrong vs Correct

#### Wrong

```go
response.Error(c, 403, "FORBIDDEN", "bad channel token", nil)
```

#### Correct

```go
if !ch.Enabled || ch.Unlisted || ch.TokenProtected() {
    continue
}
items = append(items, publicClientChannel(ch))
```

**Why**: Catalog list is not an auth gate and does not confirm hidden slugs. Check uses `channel=` + `X-Channel-Token` without 403 on mismatch.

---

## Scenario: SHA-256 package URLs and unsigned `file_name`

### 1. Scope / Trigger

Native check / integrity / package-bearing diff share one URL builder. Download lookup is content SHA-256, not FileName or artifact UUID.

### 2. Signatures

```go
func PackageDownloadURL(slug, sha256 string) string // /api/v1/projects/{slug}/packages/{lowercase-hex}
func ParsePackageRef(ref string) (hex, suffix string, ok bool) // leading 64 hex, optional .{ext}/.blockmap
func ArtifactObjectKey(slug, sha256 string) string // storage: {slug}/{sha256}
```

### 3. Contracts

- Default (single-node, or cluster `download=local`): `package_url` / `full_package_url` / per-file download `url` = `PackageDownloadURL`. Do not append `file_name` to the path. Private signed URLs add `?exp=&sig=` on that same **node path** (`pkg/urlsign`); signing covers the path, not `file_name`.
- Multi-node gate (`storage.driver=s3` **and** `cache.driver=redis`) with `cluster.download=s3`: emit the **public object URL** for `{slug}/{sha256}` (`storage.PublicObjectURL` / `public_base_url`). No PresignGet, no HMAC query. §12.1 signs that absolute URL string. Store feeds must use the same helper (`artifactURL`), not a third builder.
- JSON `file_name` is the §5.9 stable name (unsigned). Multi-file install paths stay `path`. Clients rename locally from metadata.
- §12.1 payload remains `integer\nsemver\nroot_hash\npackage_url\nsize\nsha256` — **no** `file_name`.
- `GET/HEAD /packages/:ref`: `ParsePackageRef` — leading 64 hex (case-insensitive, stored lowercase); optional `.{ext}` / `.blockmap`. Lookup is `ListArtifactsByContentSHA256` only (kind-agnostic: full, delta, patch, file, …). Same hash shares `StorageKey`; `Content-Disposition` uses `pickDownloadArtifact` (`created_at` then `id` ascending). Hash GET skips `HW_REV_INCOMPATIBLE`. `/artifacts/{id}/{filename}` is unregistered. Markdown `/media/:id` is unchanged UUID-public GET. Bytes come from `OpenStoredObject` / `OpenStoredRange`: local-proxy prefers `{local.root}/{slug}/{sha256}` when the replica exists, else the public Backend (`storage-guidelines.md`).
- `StorageKey` is only `{slug}/{sha256}`. Tests must not `Get` leftover `artifacts/{uuid}/…`.

### 4. Validation & Error Matrix

| Condition | Status | Code / body |
|-----------|--------|-------------|
| Valid 64-hex (optional `.{ext}` / `.blockmap`) and row exists | 200 | bytes; Range allowed |
| `:ref` is a FileName / not leading 64 hex | 404 | `pkg/response` `NOT_FOUND` |
| Unknown 64-hex | 404 | `pkg/response` `NOT_FOUND` (`"artifact not found"`) |
| Private project, missing/bad `exp`/`sig` | 403 | `FORBIDDEN` |
| `X-Hw-Rev` / `hw_rev` on hash GET | ignored | never `HW_REV_INCOMPATIBLE` |
| GET `/artifacts/{uuid}/{filename}` | 404 | route not registered |

### 5. Good/Base/Bad Cases

- Good: check 200 `package_url` ends with `/packages/{sha256}` (local-proxy / single-node) **or** `{slug}/{sha256}` on the public bucket (S3-direct); JSON `sha256` matches; `file_name` is the published display name.
- Base: `{sha256}.exe.blockmap` still resolves the same hash; sibling `.blockmap` artifact if the primary row is not a blockmap.
- Bad: `/packages/{FileName}`; putting `file_name` into `BuildCheckPayload`; forking a third URL helper in a feed adapter.

### 6. Tests Required

- `internal/controller/client/artifact_test.go`: hash GET **and HEAD** for `kind=full`, `kind=delta` (bsdiff), and `kind=patch` (multi-file zip); D7 `{sha}.exe`; unknown hex `NOT_FOUND` envelope; filename/UUID 404; hw_rev skip. Lookup stays `ListArtifactsByContentSHA256` (no kind filter, no new path).
- `internal/service/package_ref_test.go`: `ParsePackageRef` / `DecoratedPackageName`.
- `internal/service/update` check/integrity/diff: URL is hash; `file_name` present; payload has no `file_name`.

### 7. Wrong vs Correct

#### Wrong

```go
url := "/api/v1/projects/" + slug + "/packages/" + art.FileName
payload += "\n" + art.FileName // into §12.1
```

#### Correct

```go
url := signing.artifactURL(slug, art.SHA256, art.StorageKey)
// JSON file_name is a sibling field; SignPayload still uses package_url only
// artifactURL skips HMAC when the URL is already http(s)
```


