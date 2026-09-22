# Directory Structure

Backend layout is defined by `AGENTS.md` and must not grow extra top-level packages without updating that file.

---

## Directory Layout

```
main.go                     # single entry; delegates to cmd.Run()
frontend/embed.go           # go:embed all:dist (package frontend); cmd passes Dist() as Deps.AdminStaticFS
cmd/                        # server / admin / listen (package cmd; tests live here)
configs/                    # Viper YAML; commit *-example.yaml only (config / admin / client)
                            # gitignore: config.yaml, admin.yaml, client.yaml (secrets)
logs/                       # lumberjack JSON (gitignore; example default ./logs)
docs/                       # internal notes (app-init.md, client-sdk.md, app-docs.md); not the public site
website/                    # official VitePress docs (zh root + docs/en); yarn docs:dev / docs:build
scripts/install-deps.sh     # Linux apt/yum/dnf: PostgreSQL always, optional Redis; does not download KiriVers
deploy/                     # one-click operator pack: compose.yml + .env + config YAML; official image + Postgres + Redis
dev/docker/                 # local PostgreSQL/Redis compose for developers (not the API process)
dev/build/                  # Alpine Dockerfile + the release CLI (kirivers.py + the kirivers_build package); no shell script, no compose file
CHANGELOG.md                # appended by the CLI's changelog subcommand at release time; never hand-edited
.worktrees/                 # gitignored local worktrees for sdk/<lang> package roots
third_party/hdiffpatch/     # pinned, trimmed libHDiffPatch + libdivsufsort (VERSION + LICENSE); no CLI/zlib/zstd
internal/config/            # Viper load; only package that reads env/files for config
internal/buildinfo/         # -X-stamped build stamp (version/commit/build time) + runtime facts; CGO via build tags, no env reads
internal/logger/            # Zerolog setup
internal/database/          # GORM postgres connection, ping, runtime Watch
internal/controller/admin/  # management and CI HTTP handlers
internal/controller/client/ # client update HTTP handlers
internal/controller/client/store/ # feed HTTP: dispatch, StoreAuth, ETag, Cache-Control
internal/middleware/        # request-id, access log, recovery, AdminAuth, ProjectAccess, ClientProjectResolve, StoreAuth, ratelimit
internal/model/             # GORM entities
internal/platform/          # Canonical OS/Arch slugs and aliases
internal/cache/             # process cache (memory / Redis); Open(Options), no env reads
internal/repository/        # persistence; no update-selection business rules (project list stats SQL lives in project_stats.go)
internal/service/           # domain logic; no gin.Context. Zip packing for native full/patch and store_full lives here (auto_delta / dynamic_pack Jobs), never on the Gin goroutine
internal/service/update/    # SelectTarget + check/diff/integrity/changelog/pack adjudication; must stay free of archive/zip
internal/service/store/      # store protocol adapters (Sparkle, electron, …); no HTTP headers
internal/delta/             # bsdiff / xdelta3 / hdiffpatch engines; server entry requires CGO_ENABLED=1
internal/delta/hdiffc/      # CGO wrap of libHDiffPatch (uncompressed HDIFF13 Create/Apply)
internal/delta/hdiffpatch/  # KVDIFFHP1 Patch-only decoder (historical objects; engine Diff must not call it)
internal/storage/           # object storage interface (LocalFS / S3)
pkg/hashutil/               # streaming SHA-256 / MD5
pkg/grayutil/               # leftover HMAC bucket; check gray MUST NOT call it (allowlist + gray_completed_at)
pkg/pathutil/               # path helpers
pkg/response/               # only HTTP JSON success/error envelope
pkg/semver/                 # SemVer 2.0 canonicalize/compare helpers
pkg/signature/              # Ed25519 / RSA-SHA256 canonical payload signing
pkg/urlsign/                # HMAC query signatures for private downloads
pkg/webhook/                # webhook HMAC helpers
```

`internal/service/update/` holds the single target-selection implementation (`SelectTarget`); client check and all store feeds must call it — never fork the algorithm. Announcements do not use `SelectTarget`; they live in `service.AnnouncementService` (`announcement-contract.md`). Admin list-card “latest version” also must not call `SelectTarget`; it uses `update.CompareVersions` over published/deprecated rows (`project-admin-contract.md`).

## Module Organization

- New HTTP endpoints go in `controller/admin` or `controller/client`, then call `service`. Client pack handlers enqueue Jobs; they must not import `archive/zip`.
- New tables go in `model` + `repository`.
- Store feed adapters live in `internal/service/store` (return Body/ContentType/ETag only). Feed HTTP lives in `internal/controller/client/store`. Do not create `internal/feed` or `internal/openapi`.
- OpenAPI 3 JSON lives at `internal/controller/openapi.admin.json` and `openapi.client.json` (`go:embed`). Edit the plane that owns the route. `TestOpenAPIRoutesSync` matches Gin to the plane spec; `TestPlaneSpecsValid` checks path partition and `$ref` closure. Do not add `docs/openapi.json`, `internal/controller/openapi.json`, or `internal/openapi`. The docs site copies the **unfiltered** plane files at `yarn docs:dev` / `yarn docs:build` via `website/scripts/filter-openapi.mjs` into gitignored `website/docs/public/openapi/openapi.client.json` and `openapi.admin.json` (same bytes as `internal/controller/openapi.*.json`: client includes `/store/`, admin includes CI tokens / `ci/releases`). Do not slice four specs. Scalar is two `layout: false` pages opened in a new window from `/api/reference/`; do not embed Scalar in ordinary doc pages. Do not hand-copy field tables into Markdown.
- Local databases for contributors (`go run .`): `docker compose -f dev/docker/compose.yml up -d`. Operator one-click pack is `deploy/` (`cd deploy && cp .env.example .env && docker compose up -d`): Hub image `kirizuofficial/kirivers:latest` + Postgres + Redis. One `.env` feeds Postgres, Redis `requirepass`, and `KIRIVERS_*` (DSN, Redis, URL signing). `deploy/` is the **only** one-click Compose example in the repo; `dev/build/` ships no Compose file, just the runtime `Dockerfile` (`dev/build/Dockerfile`) and the release CLI. Do **not** add `docker-compose.yml` / `compose.yml` at the repo root. Do not point `admin.yaml` `static_dir` at `website/` dist, and do not host the VitePress site on admin `GET /`.
- Official client SDKs live on orphan branches `sdk/<lang>` in **separate git worktrees** (do not check them out over the default-branch worktree). Each worktree root **is** the package root. Go / PHP / Swift **publish** remotes are `Kirizu-Official/KiriVers-SDK-{Go,PHP,Swift}`; this repo’s `sdk/go` / `sdk/php` / `sdk/swift` are **pointer-only** (README jump). Complete sources stay on `sdk/go-src` / `sdk/php-src` / `sdk/swift-src` until copied to those remotes — do not merge those trees into `main` or into the pointer branches. Never merge any `sdk/*` into the default branch. Never add `sdk/` on the default branch. Package-manager SDK worktrees carry their own `.github/workflows/` (ci + merged-PR-only release) and `scripts/sdk_release.py` + `scripts/changelog-types.conf`; none of those may appear on the default branch. `sdk/c` and `sdk/cpp` are archive-shipping package roots (CMake `project(... VERSION ...)` is the manifest). See `docs/client-sdk.md` and `docs/sdk-publish.md`.
- Language agents must not edit this default-branch tree (including `internal/`). If a live SDK test proves the server wrong, they write `BACKEND_ISSUE.md` in the language worktree; the session that owns `main` applies the fix.
- After `openapi.client.json` changes, copy that file into each SDK worktree root and refresh `OPENAPI_REVISION` (`info.version` + short SHA-256 of the snapshot). Those SDK snapshots must not be committed on the default branch (the only committed client spec remains `internal/controller/openapi.client.json`). Do not OpenAPI-codegen SDK clients.
- Official **server** binaries and `go test` of `internal/delta/hdiffc` need `CGO_ENABLED=1` plus a C++ toolchain (Windows MinGW `g++`, macOS `clang++`, Linux `g++`/`clang++`). `CGO_ENABLED=0 go build .` must fail. Official **Go SDK** stays `CGO_ENABLED=0` (no libHDiffPatch). Darwin CGO release builds must run on a macOS host (`clang -arch` for the other Darwin arch); Linux→darwin CGO without an Apple SDK is not a release path. See `docs/delta-engines.md` and `internal/delta/hdiffc`.
- `third_party/hdiffpatch/` is a **trimmed** copy of a pinned tag (`VERSION` file). Do not vendor `hdiffz.cpp`, dir_diff, or zlib/zstd/lzma plugins. Compile flags keep dir/bsdiff/vcdiff/MT/compress plugins off. The CGO flags declare **no `-I` include root**: every vendored header is included relative to the including file (`wrap.cpp`, `vendor_*.c/.cpp`). An include root holding a dot-less file is matched case-insensitively on Windows/macOS, and `VERSION` there answered libc++'s `#include <version>` — that broke every non-Linux CGO build. `check-workflows` fails if any CGO `-I` root names a directory that holds a dot-less file.

## Naming Conventions

- Go packages: lowercase, singular (`service`, `model`).
- JSON: snake_case.
- Config keys: snake_case in YAML; env override prefixes `KIRIVERS_` / `KIRIVERS_ADMIN_` / `KIRIVERS_CLIENT_`.

## Forbidden

- Business SQL inside `controller`.
- `c.JSON` error objects outside `pkg/response`.
- Reading `os.Getenv` for DSN outside `internal/config`.
- Import cycle `internal/repository → internal/service/…`: `internal/service` already imports `internal/repository`, so a new sub-package under `service` whose loader implementation lives in `repository` must not import `service` helpers (e.g. `ParseVersionRef`). Either move the helper to `pkg/`, or replicate it locally and lock semantics with a parity test against the original (see `internal/service/update/parse_parity_test.go`).
- `os/exec` of `hdiffz` / `hpatchz` from the server. Engine Diff must not generate `KVDIFFHP1`. Do not restore `official-cli` as `engines[].implementation`.
- Root `compose.yml`. Host `5432`/`6379` on `deploy/` Compose. A Compose file or shell script under `dev/build/` (it holds only the runtime Dockerfile and the Python release CLI). COPY of Ubuntu glibc linux binaries into the Alpine runtime. `on: push` or `on: tags` for server Release (publishing is manual-only). A second forge's pipeline. Hand-editing root `CHANGELOG.md`. See `server-release.md`.
