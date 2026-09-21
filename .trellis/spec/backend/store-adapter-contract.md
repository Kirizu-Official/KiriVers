# Store Adapter Contract

> Executable contract for all store-protocol feed adapters (`internal/service/store`). Sparkle established it; electron/tauri/squirrel/clickonce/appimage/winget/msix/fdroid must follow it.

---

## Framework rules (never leak into adapters)

- `Adapter` interface: `Protocol() string`, `Enabled(*model.Project)` (any enabled listing for that protocol; HTTP primary gate is listing resolve), `Render(ctx, deps, Request) (*Response, error)`. Adapters return Body/ContentType/ETag/Status — they never touch HTTP headers, status codes, or auth. `Render` receives the resolved listing (identifiers, package selector, os/arch/channel pins).
- Controller layer (`internal/controller/client/store`) owns HTTP: `GET /store/:protocol/:listing_slug/*doc`, listing resolve, pin-over-query, plain 404 for missing/disabled listing or unknown protocol (**never a generic zip fallback**, §9.2), applying `middleware.StoreAuth` (401 UNAUTHORIZED), uniform ETag (SHA-256 of body, first 16 hex) + If-None-Match 304 (SKIPPED for private-storage projects so signed URLs stay fresh) + `Cache-Control` from `store.CacheControlFor(project, localProxy)` (`localProxy` **or** private project → `private, no-store`; else `public, s-maxage=<Project.CacheSMaxageSeconds>, stale-while-revalidate=30`) + `Vary: Accept-Encoding`, and feed rate-limit key. Local-proxy must match native check: per-node replica visibility must not enter a shared CDN. Do not reimplement StoreAuth inside an adapter. Old path `/store/{protocol}/{doc}` (no listing slug) is 404.
- os/arch inputs canonicalized via `platform.CanonicalOS/CanonicalArch` (aliases darwin→macos, amd64→x86_64) BEFORE the adapter sees them; listing pins overwrite conflicting query values; identical logical requests must yield identical ETags.

## Visibility rule (§4.4/§9 — shared `AnonymousVisible`)

Public feeds project the ANONYMOUS view: published, channel enabled, line ready with DEFAULT hw variant present, gray hit = `Version.GrayIsComplete()` (`is_critical` OR `gray_completed_at != nil`). Incomplete gray is NEVER visible to anonymous/feed consumers (allowlists need a `device_id`). Empty namebook completes gray, so feeds then include that version.

## Protocol invariants

- Dual-number mapping is FIXED per protocol (§9 table), NOT driven by `compare_engine`. Sparkle: `sparkle:version`=version_integer, `sparkle:shortVersionString`=version_semver (omit-if-null).
- Enclosures/download URLs always use the same helper as native check (`artifactURL` / `update.PackageDownloadURL` of the listing-selected complete package, DEFAULT hw variant). When cluster S3-direct is on, that helper emits the public `{slug}/{sha256}` object URL (no HMAC). `Content-Disposition` still uses the §5.9 stable filename. Private projects get signed URLs via `update.URLSigner` **only on node paths**. Do not fork a third URL builder. electron-updater: `files[].url` is the absolute hash URL; `path` is `{sha256}{ext}` via `service.DecoratedPackageName`; generic provider needs electron-updater ≥6; do not serve downloads under the feed directory.
- Protocol-native signatures differ from the native `signature` bytes (§12.1): Sparkle `edSignature` = Ed25519 over the ENCLOSURE FILE BYTES (LRU-cached per artifact SHA-256, single-flight), NOT over metadata.
- ALL dynamic text must go through `xml.EscapeText` (or equivalent) — changelog is user content; regression test `TestSparkleXMLEscaping` is the template.
- Missing/disabled listing or protocol not adapted → plain 404; never fake the schema with a full zip. `package_source=line_full` uses `kind=store_full` (path zip; SHA distinct from native hash-root `kind=full`). Multi-file lines with no `store_full` are skipped (do **not** serve hash-root `kind=full` as `line_full`). Single-file lines have no hash-root zip and no `store_full`, so they still use `kind=full`. `manifest_path` uses matching file bytes and skips versions with no match; all skipped → `ErrNoRelease` 404.
- Unpinned Sparkle listings project every public channel in one document (`AnonymousVisibleAllPublic`). A listing that pins `channel` uses `AnonymousVisible` for that slug only — `channel=stable` in the query must not resurrect omitted channels.

## Wrong vs Correct

#### Wrong

```go
// adapter sets headers / reads c.Query directly
resp.Header.Set("Cache-Control", "public") // HTTP concerns in adapter
```

#### Correct

```go
// adapter returns Response{Body, ContentType, ETag}; framework writes headers
return &store.Response{Body: xml, ContentType: "application/xml; charset=utf-8", ETag: store.BodyETag(xml)}, nil
```

---

## Scenario: Listing-scoped feed HTTP and complete-package enclosure

### 1. Scope / Trigger

Hard cutover from `/store/{protocol}/{doc}` plus a project-level protocol map. Each feed URL includes a user-chosen listing slug. Enclosures are SHA-256 package URLs of the listing-selected complete package.

### 2. Signatures

```
GET|POST /api/v1/projects/{project_ref}/store/{protocol}/{listing_slug}
GET|POST /api/v1/projects/{project_ref}/store/{protocol}/{listing_slug}/*doc
```

```go
func (s *ProjectService) GetEnabledStoreListing(ctx, projectID, protocol, slug) (*model.StoreListing, error)
func PackageDownloadURL(slug, sha256 string) string
func DecoratedPackageName(sha256, fileName string) string // {sha256}{ext}
```

`Render` input includes the resolved listing (identifiers, `package_source` / `manifest_path`, os/arch/channel pins). Do not read a project-level protocol map.

### 3. Contracts

- Missing adapter, empty slug, missing/disabled listing, or leftover `/store/sparkle/appcast.xml` (doc name in the slug slot) → **plain-text 404**, not `pkg/response`.
- Canonicalize query `os`/`arch` then **overwrite from listing pins**. Pinned `channel=beta` must not emit stable even if `?channel=stable`.
- Enclosure / `files[].url` = same `artifactURL` as native check. `package_source=line_full` → `kind=store_full` (path zip). Missing `store_full` on a multi-file line skips that version (never hash-root `kind=full`). Single-file listings still use `kind=full`. `manifest_path` → matching kind=file / manifest entry bytes; skip versions with no match; all skipped → `ErrNoRelease` (plain 404).
- electron-updater: `files[].url` is the absolute hash URL; `path` is `DecoratedPackageName` (`{sha256}{ext}`). Generic provider needs electron-updater ≥6. Do **not** register downloads under `/store/.../`.
- Do not fork a third URL builder.

### 4. Validation & Error Matrix

| Condition | Status | Body |
|-----------|--------|------|
| Enabled listing + known protocol | 200 | protocol document |
| Unknown protocol | 404 | plain text |
| Unknown / disabled listing slug | 404 | plain text |
| Old `/store/{protocol}/{doc}` (no listing slug) | 404 | plain text |
| Leftover `/feed/...` prefix (not registered) | 404 | plane NoRoute JSON |
| All versions skipped (`manifest_path` miss) | 404 | `ErrNoRelease` / plain |
| `require_client_token` / store token fail | 401 | `UNAUTHORIZED` envelope |

### 5. Good/Base/Bad Cases

- Good: two Sparkle listings, different slugs and bundle ids, feeds do not mix packages (`TestFeedSparkleTwoListingsDoNotMix`).
- Base: unpinned Sparkle uses `AnonymousVisibleAllPublic` (all public channels in one document).
- Bad: relative electron `path` that makes the updater GET `/store/electron/{slug}/{sha}.exe`; FileName enclosure.

### 6. Tests Required

- `TestFeedOldPathWithoutListingSlug404`
- `TestFeedSparkleTwoListingsDoNotMix`
- `TestFeedSparklePinnedBetaOmitsStableSameOS`
- `TestFeedManifestPathSetupExeSHA256` (+ `TestMatchManifestPathSetupExe`)
- `TestResolveStorePackageLineFullSHASeparated` / `TestStoreLineFullUsesStoreFullSHANotNativeFull` (AC16: no hash-root `kind=full` as `line_full`)
- `TestCacheControlLocalProxy` (`internal/service/store/catalog_test.go`): `localProxy=true` → `private, no-store` even when the project is public.

### 7. Wrong vs Correct

#### Wrong

```go
enc.URL = base + "/packages/" + art.FileName
files[0].Path = art.FileName // electron-updater treats this as the download key
pkg := firstArtifact(line, model.ArtifactKindFull) // hash-root zip as store line_full
```

#### Correct

```go
enc.URL = signing.artifactURL(slug, sha256, storageKey)
files[0].URL = enc.URL
files[0].Path = service.DecoratedPackageName(sha256, art.FileName)
pkg := matchStoreFull(line, full) // kind=store_full; skip multi-file if missing
```
