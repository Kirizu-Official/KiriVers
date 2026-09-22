# Build Info Injection (server binary + admin UI)

Executable contract for stamping version / commit / build time into a KiriVers build and serving it to the console.

## Scenario: one release stamp reaching Go binary, admin UI, and the About page

### 1. Scope / Trigger

Update this file whenever any of these change:

- `internal/buildinfo/` (symbols, accessors, fallback rules)
- `GET /api/v1/admin/build-info` or its `BuildInfo` schema
- `dev/build/kirivers_build/build.py` ldflags assembly (`_BUILDINFO_PKG`, `_STAMP_VARS`, `_STAMP_VALUE_RE`, `_COMMIT_SHORT_LEN` / `_short_commit`, `_ldflags`)
- `dev/build/Dockerfile` `-X` splicing, or the `KIRIVERS_*` env blocks in `.github/workflows/release.yml`
- `frontend/vite.config.mts` `__BUILD_INFO__`, or `frontend/src/composables/useBuildInfo.ts`

This is cross-layer (build CLI → linker → Go runtime → HTTP JSON → generated SDK → Vue), so code-spec depth is mandatory.

### 2. Signatures

```go
// internal/buildinfo — injected targets are UNEXPORTED package-level strings.
var (version, commit, buildTime string)

func Version() string     // "0.1.0" (never "v0.1.0"); fallback "dev"
func Commit() string      // 7-hex, optional "-dirty"; fallback VCS stamp then "unknown"
func BuildTime() string   // RFC3339 UTC; fallback "unknown" (never process start time)
func GoVersion() string   // runtime.Version()   — runtime fact, not injected
func Platform() string    // goos + "/" + goarch — runtime fact, not injected
func CgoEnabled() bool    // build-tag const (cgo.go / nocgo.go), never -X
```

- Endpoint: `GET /api/v1/admin/build-info`, registered in the `AdminAuth` group of `internal/controller/admin/admin.go` (`Register` → `authed`). Not public, not in the `pending` group.
- Handler payload builder: `buildInfoPayload()` in `internal/controller/admin/buildinfo.go`.

### 3. Contracts

**Injected symbol path** (must match `go.mod:1`):
`github.com/Kirizu-Official/KiriVers/internal/buildinfo.{version,commit,buildTime}`

> **Warning — three copies of these names must agree verbatim**: the Go `var`,
> `build.py`'s `_BUILDINFO_PKG` + `_STAMP_VARS`, and the `-X` strings in
> `dev/build/Dockerfile`. `cmd/link` **silently ignores** a `-X` target it cannot
> resolve, so a typo produces a working build that reports `dev`/`unknown` forever.

**Env keys** (all optional; same trio for the Go build and the Vite build):

| Key | Default in `build.py` | CI source |
|---|---|---|
| `KIRIVERS_VERSION` | `dev` | per build job: `needs.version.outputs.version` |
| `KIRIVERS_BUILD_COMMIT` | unset → best-effort `git rev-parse --short HEAD`; still empty → **omit the `-X`** | workflow level: `github.sha` (full 40 hex) — `build.py` trims it |
| `KIRIVERS_BUILD_TIME` | `unknown` | workflow level: `github.run_started_at` |

One run shares the commit/time at workflow level so all 11 artifacts plus `dist` carry one stamp; per-job clocks would make operators read eleven builds.

> **Warning — never slice the SHA in YAML.** The Actions expression language has no
> string-slicing function, so `${{ substring(github.sha, 0, 7) }}` is not a wrong value,
> it is an unparsable file: GitHub reports `Unrecognized function: 'substring'` and **no
> job in the workflow can start**. `check-workflows` rejects any function name outside
> `ACTIONS_FUNCTIONS` for exactly this reason — in a `${{ … }}` **and** in an `if:`,
> whose value GitHub evaluates as an expression even when nobody writes the braces.
> The width is decided once, by
> `_COMMIT_SHORT_LEN` / `_short_commit()` in `build.py`, which trims both the injected
> value and the `git rev-parse --short` fallback (git picks that abbreviation's length
> from the object count, so it is *not* a reliable 7). This mirrors `commitShortLen` in
> `internal/buildinfo/buildinfo.go`, which trims a VCS stamp the same way.
> `frontend-build` forwards the trimmed value into the Vite environment, because
> `frontend/vite.config.mts:43` uses `KIRIVERS_BUILD_COMMIT` verbatim.
> The charset guard `_STAMP_VALUE_RE` runs **before** the trim: slicing first would let a
> value whose offending character sits past the cut-through reach the linker.

**Response 200** — bare payload, no success envelope (`pkg/response`):

```json
{"version": "0.1.0", "commit": "4f2a1c9", "build_time": "2026-09-21T03:15:02Z",
 "go_version": "go1.27.0", "platform": "windows/amd64", "cgo_enabled": true}
```

Keys are exactly the `BuildInfo` schema properties, `additionalProperties: false`, all six required. `version` carries no `v` prefix (`KIRIVERS_TAG` owns the prefix). Times travel as RFC3339 UTC; display formatting is the UI's job.

**Fallbacks** must be recognizable, never empty, never fabricated:

| Build path | version | commit | build_time |
|---|---|---|---|
| release job | `0.1.0` | 7-hex | run start |
| local `go build .` | `dev` | `ca01b71-dirty` (VCS stamp) | `unknown` |
| `dev/build/Dockerfile` source build | `--build-arg` | `unknown` (`.dockerignore` drops `.git`) | `--build-arg` |
| CI Docker path | — | uses `USE_PREBUILT=1` and copies the already-stamped binary; `images.py` injects nothing | |

**Frontend**: `frontend/vite.config.mts` injects `__BUILD_INFO__` via `define`
(`{version, commit: string | null, buildTime}`), typed in `frontend/env.d.ts`.
`commit` is `null` when git is unavailable (e.g. the Docker `frontend` stage), so
that fallback stays reachable. `useBuildInfo` is a module-scope singleton — not Pinia.

### 4. Validation & Error Matrix

| Condition | Behavior |
|---|---|
| stamp value outside `^[A-Za-z0-9._:/+,-]*$` | `build.py` logs the refusal to stderr and exits non-zero **before** creating the output dir; no artifact (`cmd/go` splits the `-ldflags` value on whitespace, so a space would corrupt the flag stream) |
| `KIRIVERS_VERSION` unset locally | injects `dev`; commit `-X` omitted so the Go VCS fallback runs |
| build job missing `KIRIVERS_VERSION` in CI | silent version drift to `dev` — guarded only by review + AC11, so check all five build jobs when editing `release.yml` |
| anonymous or `pending` session | `401 UNAUTHORIZED` (route lives in the `AdminAuth` group) |
| DB unavailable | `middleware.DBUnavailableNotFound` 404s `/api/**` → the About page/footer show the unavailable placeholder; by design |
| `-buildvcs=false` (all official builds) | `debug.ReadBuildInfo()` has no `vcs.revision`; commit comes from `-X` only |

### 5. Good / Base / Bad

- **Good**: `python dev/build/kirivers.py build-cgo linux amd64 …` with the trio set → endpoint reports the exact injected values verbatim.
- **Base**: contributor `go build .` → `dev` / `ca01b71-dirty` / `unknown`; nothing errors, About page renders.
- **Bad**: `-X …/buildinfo.Version=…` (exported name, capital `V`) → build succeeds, binary reports `dev` forever, no test catches it unless the injected-values case runs against that exact flag string.

### 6. Tests Required

| Test | Assertion point |
|---|---|
| `internal/buildinfo/buildinfo_test.go` | fallbacks `dev`/`unknown`; `Platform()` matches `os/arch` |
| `TestInjectedValuesVerbatim` | skips when the raw var is empty; otherwise the accessor returns the injected string unchanged. Run it with `go test -ldflags "<the string build.py emitted>" ./internal/buildinfo` to prove symbol paths |
| `internal/controller/admin/buildinfo_test.go` | 200 body is the bare six-key map; anonymous and pending tokens both 401 |
| `TestOpenAPIFieldTables` | `BuildInfo` schema keys == `ginHKeys("admin/buildinfo.go", "buildInfoPayload")`; requires exactly **one** `gin.H` literal with literal string keys |
| `TestOpenAPIRoutesSync` / `TestPlaneSpecsValid` / `TestOpenAPIForbiddenNames` | route↔spec both directions, `$ref` closure, path stays under `/api/v1/admin` |
| AC3 negative | `KIRIVERS_VERSION="1.0 0" build-cgo …` → non-zero exit, empty stdout, no output file |

> **Warning**: `go version -m` is **not** proof of injection. With `-trimpath` on
> go1.27 the build settings record `-buildmode/-compiler/-trimpath/CGO_ENABLED/GO*`
> but not `-ldflags`. Prove it by scanning the produced binary for the sentinel
> string (and a negative control from a different sentinel) plus the injected test.

### 7. Wrong vs Correct

#### Wrong

```python
# breaks the single-argv contract documented at build.py:29-32
subprocess.run(["go", "build", f"-ldflags=-s -w -X {pkg}.version={ver}", "."], shell=True)
```

```go
var Version string          // exported …
func Version() string {}    // …and an accessor of the same name: does not compile
```

#### Correct

```python
# LDFLAGS_* constants stay untouched; values validated; one argv, never a shell
ldflags = _ldflags(LDFLAGS_RELEASE)   # "-s -w -X …buildinfo.version=0.1.0 -X …"
_go_build(ldflags, out, env)          # passed as a single list element
```

```go
var version string            // -X target
func Version() string { … }   // fallback resolves once via sync.OnceValue
```

## Common Mistake: stamping only one build path

`build-cgo`, `build-linux-musl` (which re-enters this CLI **inside** Alpine, so the
stamp must be forwarded with `docker run -e KIRIVERS_*`), and the `Dockerfile`
source build are three separate call sites. Missing the musl one ships the only
two Docker Hub binaries as `dev` while every archive reports the release version.

## Common Mistake: exposing build info anonymously

The About dialog and footer render only inside `DefaultShell`, which the router
mounts exclusively for authenticated sessions, so the endpoint stays behind
`AdminAuth`. Do not add build info to `/api/v1/health` or to `sharedSpecPaths`:
commit plus an exact build time is a reconnaissance gift for a server that ships
binaries.
