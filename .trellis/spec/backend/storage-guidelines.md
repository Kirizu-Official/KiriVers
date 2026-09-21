# Storage Guidelines

> All blob I/O goes through `storage.Backend`. Do not open a second abstraction for uploads or signed URLs.

---

## Overview

Drivers: `local` (default) and `s3` (AWS SDK v2, path-style compatible with MinIO / R2). Factory: `storage.Open(Options)` from process config. This package does not read environment variables.

Remote (`driver=s3`) uses **two** backends: `storage.s3` is the **public** artifact bucket (anonymous GET of `{slug}/{sha256}`); `storage.private` is the **private** backend bucket (GeoIP MMDB, credentialed Get). `driver=local` shares one LocalFS root and isolates families by key prefix (`geoip/` vs `{slug}/…`). Do not Put GeoIP bytes on the public Backend.

---

## Scenario: Backend Range, Head, and Presign

### 1. Scope / Trigger

Uploads, downloads, `/health` `ready` probe, TUS, PresignPut, and private GET URLs (HMAC signing is `pkg/urlsign` at the download handler — not a second storage interface).

### 2. Signatures

```go
const ReadyProbeKey = ".ready"

type Backend interface {
    Put(ctx context.Context, key string, r io.Reader, size int64, contentType string) error
    Get(ctx context.Context, key string) (io.ReadCloser, error)
    Range(ctx context.Context, key string, start, end int64) (io.ReadCloser, error)
    Head(ctx context.Context, key string) (size int64, etag string, err error)
    Delete(ctx context.Context, key string) error
    PresignPut(ctx context.Context, key string, ttl time.Duration, contentType string) (string, error)
    PresignGet(ctx context.Context, key string, ttl time.Duration) (string, error)
}

func Open(opts Options) (Backend, error)
func EnsureProbe(ctx context.Context, b Backend) error
```

Range is a closed HTTP interval `[start, end]`. `end < 0` means through EOF.

### 3. Contracts

- Keys are opaque strings. Reject `..`, absolute paths, and Windows drive letters (`storage.ErrInvalidKey`). LocalFS resolves with `filepath.Rel` against the root (do not use case-sensitive `HasPrefix` on Windows). Canonical **artifact** `StorageKey` is `{slug}/{sha256}` (`storage.ArtifactObjectKey`; lowercase 64-hex; no `artifacts/` prefix, no project UUID, no FileName segment). Incoming bytes write **local disk only** at `{storage.local.root}/{slug}/temp/{uuid}` (`ArtifactTempRel`); hash then promote. Public buckets must never contain `temp/`. The **HTTP** lookup remains `GET /packages/{sha256}` (`ParsePackageRef`) unless cluster S3-direct emits a public object URL (see `client-check-contract.md`). Markdown media stays `GET /media/{uuid}`. HMAC remains `pkg/urlsign` on the **node path**, never on absolute `http(s)` public URLs. Patch cleanup may `DeleteArtifact` a stale `kind=patch` row but must **not** `storage.Delete` the blob when `full` / `store_full` / another kept artifact still shares `StorageKey`. Never delete `kind=full` or `kind=store_full` rows in retention cleanup. Do not dual-read leftover `artifacts/{uuid}/…` keys.
- Missing object → `storage.ErrNotFound`. S3/MinIO often return `smithy.GenericAPIError` with code `NotFound` / `NoSuchKey` instead of `*types.NotFound`; map those to `ErrNotFound` or `EnsureProbe` never writes `.ready` and `/health` reports `ready=false`.
- LocalFS `PresignPut`/`PresignGet` may return a `file://` (or process-local) URL. Private-project URL TTL is `Project.SignedURLTTLSeconds` (0 → 3600); HMAC signing is applied in the download handler (`pkg/urlsign`), not inside LocalFS.
- Probe: on startup call `EnsureProbe` so Head of `.ready` can succeed.
- Config keys: `storage.driver`, `storage.local.root`, `storage.s3.{endpoint,region,bucket,access_key,secret_key,use_path_style,public_base_url}`, `storage.private.*` (same shape as `s3`, no public_base_url). Node identity file: `{storage.local.root}/.kirivers-node-id`.
- Env: `KIRIVERS_STORAGE_*` / `KIRIVERS_STORAGE_PRIVATE_*` after `BindEnv` (see quality-guidelines Viper gotcha). Empty `storage.private.bucket` with `driver=s3`: process still listens; GeoIP upload fails and must not write the public bucket. `/health` `ready` Heads public `.ready` and, when a distinct private Backend is open, private `.ready` too.

### 4. Validation & Error Matrix

| Condition | Error |
|-----------|--------|
| `..` / abs / drive-letter key | `ErrInvalidKey` |
| Missing object | `ErrNotFound` |
| start > end or start < 0 | `ErrInvalidRange` |
| Unknown driver | Open error |
| Nil backend in `/health` `ready` | `ready=false` (HTTP 200) |

### 5. Good/Base/Bad Cases

- Good: Put 8 bytes, Range(2,5) returns exactly those four bytes on LocalFS and on an S3 fake.
- Base: `EnsureProbe` is idempotent if `.ready` already exists.
- Bad: Head treating MinIO `NoSuchKey` as a hard failure (probe never created).

### 6. Tests Required

- LocalFS: Put/Get/Range/Head/Delete/Presign on a temp dir; reject `../x`.
- S3: fake Backend or mock roundtrip that calls Range and Presign (real MinIO: `go test -tags integration ./internal/storage`).
- `/health` `ready=false` when storage Head fails even if DB ping would succeed.

### 7. Wrong vs Correct

#### Wrong

```go
if err := db.Ping(); err != nil {
    return err // storage never probed
}
return store.Head(ctx, ".ready")
```

#### Correct

```go
return service.CheckReady(ctx, db, store) // Ping AND Head; errors.Join
```

---

## Design Decision: One Backend, including Presign

**Context**: Uploads and private download URLs need Presign on the same Backend.

**Options**: (1) Add Presign now on `Backend`; (2) Wait and add a second interface.

**Decision**: Option 1. Product TTL and query-only signature on a stable path live at the download handler (`pkg/urlsign` / client-check-contract), not as a second storage interface.

---

## Scenario: Markdown media (UUID-public GET, S3 replica)

### 1. Scope / Trigger

Vditor uploads images/attachments that must live as durable `<img src>` / download URLs. Artifact GET still requires `?exp=&sig=` on private projects and `ClientProjectAuth`. Markdown media is a separate object family: same `storage.Backend`, different key prefix, no PresignGet in stored Markdown.

### 2. Signatures

```go
type ProjectMedia struct { /* uuid PK, project_id, file_name, storage_key, size, content_type, sha256, md5, sha512 */ }
func (s *MediaService) Put(ctx context.Context, project *model.Project, in MediaPutInput) (*MediaPutResult, error)
func (s *MediaService) GetMeta(ctx context.Context, projectID, id uuid.UUID) (*model.ProjectMedia, error)
func (s *MediaService) Open(ctx context.Context, meta *model.ProjectMedia, start, end int64) (io.ReadCloser, error)
func ExpandSiteURL(markdown, origin string) string
func OriginFromReferer(referer, fallback string) string
func MediaPublicURL(slug string, id uuid.UUID) string // ${site_url}/api/v1/projects/{slug}/media/{id}
```

- Admin: `POST /api/v1/admin/projects/:project_ref/media` (`ProjectAccess` `project:admin`), multipart `file[]`.
- Client: `GET`/`HEAD /api/v1/projects/:project_ref/media/:id` with `ClientProjectResolve` (CORS + `force_https` + IP rate limit; **no** token, **no** urlsign).
- Replica (S3 driver only): `storage.NewLocalFS(storage.local.root)` keys `media-cache/{project_id}/{id}/{file}`. Local driver: replica is nil (skip second write).

### 3. Contracts

- Backend keys: `media/{project_id}/{id}/{safe_filename}`.
- Stored Markdown prefix is the literal `${site_url}` (no `public_url` / `KIRIVERS_CLIENT_PUBLIC_URL`).
- Client announcement/check/changelog JSON: `origin = OriginFromReferer(Referer, RequestOrigin)` then `ExpandSiteURL`. `RequestOrigin` uses the same HTTPS detection as `force_https` (`X-Forwarded-Proto`). Add `Vary: Referer`. ETag hashes **stored** placeholder text.
- Admin CRUD GET keeps `${site_url}`. Vditor preview expands with `window.location.origin` and collapses on emit.
- GET 200: `Content-Type` from metadata; jpeg/png/gif/webp `inline` else `attachment`; `Content-Disposition` filename from `project_media.file_name` (`filename=` ASCII fallback; RFC 5987 `filename*` when the stored name has non-ASCII); `Cache-Control: public, max-age=31536000, immutable`; `Accept-Ranges`. When sha256/md5 are non-empty: `ETag` is the quoted sha256 hex; `Digest: sha-256=<base64>`; `Content-MD5` is raw MD5 base64. Range 416 when unsatisfiable.
- Admin POST JSON keeps Vditor `succMap` and also returns `data.files[]` with `id`, `file_name`, `size`, `content_type`, `sha256`, `md5`, `sha512`, `url`. `MediaService.Put` hashes with `hashutil.NewMultiHasher(true)` before `Backend.Put`. New uploads always fill hashes; do not backfill leftover local rows.
- Max 10 MiB. Reject empty files and MIME `text/html`, `image/svg+xml`, JavaScript types (declared + `DetectContentType`). UUID leak = bytes leak (accepted).

### 4. Validation & Error Matrix

| Condition | Result |
|-----------|--------|
| Unknown id / id from another project | 404 `NOT_FOUND` |
| Unknown project slug | 404 `PROJECT_NOT_FOUND` |
| `require_client_token` project, no Authorization | 200 bytes (UUID-public) |
| Missing multipart `file[]` | 400 `INVALID_REQUEST` |
| Rejected MIME / empty / >10 MiB | Put error; handler may still 200 `{code:1, errFiles}` |
| `os.Open` / S3 SDK in controller | Forbidden — `Backend` / `NewLocalFS` only |
| `PresignGet` written into Markdown | Forbidden — URLs expire; use `${site_url}` path |

### 5. Good/Base/Bad Cases

- Good: admin upload `succMap` value `${site_url}/api/v1/projects/{slug}/media/{id}`; client GET with `Referer: https://app.example/x` expands to `https://app.example/api/v1/projects/...`.
- Base: missing Referer → request origin; S3 GET miss fills `media-cache/` then serves bytes from KiriVers.
- Bad: mounting media GET under `ClientProjectAuth`; adding `client.public_url`.

### 6. Tests Required

- Service: MIME reject, 10 MiB, replica key on fake S3, Referer origin parse, missing-Referer fallback.
- HTTP: 200 bytes, Range, 404 wrong project, 200 without Authorization when `require_client_token`, `Vary: Referer`, ETag stable across origins, attachment `Content-Disposition` uses stored `file_name` (`filename*` for Unicode).

### 7. Wrong vs Correct

#### Wrong

```go
url, _ := backend.PresignGet(ctx, key, time.Hour)
succMap[name] = url // expires; cannot be <img src>
```

#### Correct

```go
succMap[name] = service.MediaPublicURL(project.Slug, row.ID) // ${site_url}/api/v1/projects/{slug}/media/{id}
```

---

## Scenario: Canonical artifact keys and local temp (D8)

### 1. Scope / Trigger

Admin/CI artifact PUT, TUS, bundle unpack, delta/fileset zip persist. Public Backend (or LocalFS) holds only hashed objects.

### 2. Signatures

```go
func ArtifactObjectKey(slug, sha256 string) string // {slug}/{lowercase-hex}
func ArtifactTempRel(slug string, id uuid.UUID) string // {slug}/temp/{uuid} — local disk only
func PublicObjectURL(cfg S3Settings, key string) string // anonymous GET; public_base_url or endpoint+bucket
```

### 3. Contracts

- Write the inbound stream to `{local.root}/{slug}/temp/{uuid}` (TUS offset = that file size). Do not `Put` `part_*` or `uploads/` keys to the Backend.
- Hash the temp file; if `{slug}/{sha256}` already exists, reuse and delete temp; else Put/rename to canonical and set `StorageKey`.
- `PresignUpload` always returns the API/TUS URL with `direct_s3=false`. Never `PresignPut` an unknown key for artifacts.
- Same project + same SHA-256 reuses the object. Slug PATCH does not rename blobs already stored under the previous slug.
- Local-proxy cluster: copy from **public** Backend into `{local.root}/{slug}/{sha256}`. S3-direct: do not copy whole packages for client visibility.
- Local-proxy `GET/HEAD /packages/:ref` (and Range): `ProjectService.OpenStoredObject` / `OpenStoredRange` **prefer the local replica** when `ReplicaHas(key)`; otherwise fall back to the public Backend. Check visibility still nils `PacksReadyAt` until the replica exists (`client-check-contract.md`). Miss path is for objects this node never pulled (pull covers `full` + `store_full`, not every delta/patch).

### 4. Tests Required

- StorageKey is `{slug}/{sha256}`; public Head `{slug}/temp/…` is `ErrNotFound`; TUS offset is local file size; fixtures never `Get` `artifacts/{uuid}/…`.
- `TestOpenStoredObjectPrefersLocalReplica`: after a local file exists, Get does not read the public Backend.

---

## Scenario: Platform GeoIP MMDB files

### 1. Scope / Trigger

Platform admins upload MaxMind/DB-IP `.mmdb` files used to annotate `clients.last_ip`. GeoIP uses the **private** Backend only (`storage.private` when `driver=s3`; shared LocalFS + `geoip/` prefix when `driver=local`). No `config.yaml` path, no second blob API, no licensed MMDB in git.

### 2. Signatures

```
GET/POST    /api/v1/admin/geoip/databases          RequireInstanceAdmin (platform flag)
PATCH/DELETE /api/v1/admin/geoip/databases/{id}
```

Table `geoip_databases`: id, name, file_name, storage_key, size, enabled, rank, timestamps. Object key `geoip/{id}/{safe_filename}` (`GeoipObjectKey`). Upload scratch: `{local.root}/geoip/temp/{uuid}` then private Put.

Lookup: `GeoipLookupFunc(ctx, ip) → { CountryCode, RegionCode, GeoI18n }` where `GeoI18n` is `{ country: {locale: name}, region: {locale: name} }`. Inject a stub in tests; do not vendor a database.

### 3. Contracts

- Validate MMDB, then persist metadata. Runtime: private `Backend.Get` → bytes → `geoip2.FromBytes` (in-memory reader). Lookup is memory only: no `storage.Get`, no disk mmap of the object key, never a second SDK.
- Reload skips `Get` when `Head` size matches the live snapshot. Other nodes: missing local bytes → Get once from private Backend, then memory.
- Fusion: enabled rows by `rank` then `created_at`. Codes: first non-empty. Names: **per locale key**, first non-empty (high-rank missing `zh-CN` can be filled by a later DB).
- Skip invalid / unspecified / loopback / link-local / private IPs (empty result, upsert still 200).
- Corrupt/missing file: skip that row; failed upload deletes temp and does not leave a public-bucket object.
- Reload snapshot after mutate; replicas poll DB at least every 30s (poll must not unconditionally re-Get).
- Cap: 8 databases, 256 MiB each. Non-MMDB upload → 400 `INVALID_REQUEST`.
- Persist on `clients` only when IP changes. No full-table re-geocode.

### 4. Validation & Error Matrix

| Condition | Result |
|-----------|--------|
| Project-only session | 403 `FORBIDDEN` |
| Unknown id | 404 `GEOIP_NOT_FOUND` |
| >8 DBs or >256 MiB | 400 `INVALID_REQUEST` |
| Missing multipart `file` | 400 `INVALID_REQUEST` |
| Private backend nil / unconfigured | upload fails (`ErrGeoipStorage`); public bucket unchanged |

### 5. Good/Base/Bad Cases

- Good: two enabled DBs, rank 1 then 2; country code from DB1, `zh-CN` name from DB2 if DB1 map lacks it; Reload with unchanged Head size does not Get.
- Base: no enabled DB → empty codes, UI 「未知」; private IP never 500.
- Bad: writing a single `country_name` column; putting MMDB path in `config.yaml`; committing a licensed `.mmdb`; Put GeoIP on the public artifact Backend.

### 6. Tests Required

- Fusion + private-IP skip with a stub opener (`geoip_test.go`). HTTP CRUD + 403 for project-only (`identity_test.go`). Private vs public Head (`cluster_geoip_test.go`). Do not require a real MaxMind file in CI.

### 7. Wrong vs Correct

#### Wrong

```go
db, _ := geoip2.Open(cfg.GeoIPPath) // config.yaml; mmap temp file every poll
row.CountryName = rec.Country.Names["zh-CN"]
_ = public.Put(ctx, key, r, size, ct) // MMDB on the anonymous artifact bucket
```

#### Correct

```go
data, _ := io.ReadAll(private.Get(ctx, row.StorageKey))
reader, _ := geoip2.FromBytes(data) // memory snapshot; Lookup does not Get
```

---

## Don't: Direct `os.Open` / S3 SDK in service or controller

Business code takes `storage.Backend` only.
