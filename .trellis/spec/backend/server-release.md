# Server Release and Runtime Image

> Official binaries, Alpine Hub image, and GitHub Actions. Layout of `dev/build/` vs `deploy/` vs `dev/docker/` is in `directory-structure.md`. CGO/HDiffPatch is in `directory-structure.md` and `client-check-contract.md`. GitHub is the only forge; there is no second pipeline anywhere in this repo.

---

## Scenario: Official runtime image and Conventional Commits release

### 1. Scope / Trigger

Adding or changing `dev/build/Dockerfile`, the release CLI (`dev/build/kirivers.py` + `dev/build/kirivers_build/`), Release asset names, `next-version`, the changelog format or its type table, or any file in `.github/workflows/` (`ci`, `release`, `pr-guard`, `gosec-scan`, `security-gate`, `sdk-automerge`). Infra: image, secrets, env wiring, forge publish, CI sandbox topology.

### 2. Signatures

```text
dev/build/Dockerfile                 # context = repo root; -f this file
dev/build/kirivers.py                # single CLI entry; python3 … <command> on Linux/macOS, python … on Windows
                                     # stdlib only: no install step, no virtualenv, no requirements.txt
deploy/compose.yml                   # operator pack; the only Compose example that runs KiriVers; image name stays kirizuofficial/kirivers
dev/build/kirivers_build/common.py   # MATRIX, asset_name, archive_name, assert_machine, sha256_file (imported, not executed)
dev/build/kirivers_build/build.py
  build-cgo <goos> <goarch> <outfile>
  build-linux-musl <amd64|arm64> [outfile]
  assert-linux-musl <binary>...
  install-cross-toolchains <386|arm|riscv64>       # stdout is exactly CC= / CXX=
  install-llvm-mingw                               # stdout is exactly one bin dir
  frontend-build
dev/build/kirivers_build/version.py
  next-version [--format <plain|env>]
dev/build/kirivers_build/notes.py
  release-notes [options] [prev_tag] [head]        # --version --config --pr-map --date --self-check
  changelog [release-notes options]                # splices CHANGELOG.md (KIRIVERS_CHANGELOG_FILE)
dev/build/kirivers_build/package.py
  asset-names [--binaries] [linux|darwin|windows]
  package-assets <srcdir> <destdir>                # 10 zips + frontend-dist.zip + sums
                                                   # no separate checksums tool: it writes SHA256SUMS.txt
dev/build/kirivers_build/images.py
  docker-image <amd64|arm64> <semver>              # LOCAL-ONLY since the build-push-action migration:
                                                   # one native arch, pushes <semver>_<arch>
  docker-manifest <semver> [arch...]               # LOCAL-ONLY: imagetools create :latest + :<semver>
                                                   # no workflow calls either verb any more — release.yml
                                                   # uses login-action@v3 + build-push-action@v6 and a
                                                   # `docker-manifest` JOB
dev/build/kirivers_build/guards.py
  commitlint [--self-check]                         # COMMITLINT_TITLE / _FROM / _TO
                                                   # exit 0 pass | 1 verdict fail | 2 tooling fault
  issue-link [--record f] [pr] [--self-check]      # ok|fail|skip|error
  guard --record f [--title-ok 1|0|error] [--issue-ok 1|0|error] [--out f] [--self-check]
dev/build/kirivers_build/selfcheck.py
  check-workflows                                  # static regression gate
dev/build/changelog-types.conf                     # display-only type table
# dev/build/ holds no shell script and no Compose file. Retired with no replacement:
# build-darwin / build-windows / ensure-go (the release matrix jobs + setup-go cover them)
# and seed-config (it only seeded the deleted maintainer stack).
.github/workflows/ci.yml             # pull_request + push → main; publishes nothing
.github/workflows/pr-guard.yml       # pull_request_target, base.ref == main only
.github/workflows/gosec-scan.yml     # pull_request + labeled(wait-merge); the UNTRUSTED half
.github/workflows/security-gate.yml  # pull_request_target(synchronize) + workflow_run; the WRITE half
.github/workflows/release.yml        # workflow_dispatch ONLY
.github/workflows/sdk-automerge.yml  # workflow_dispatch ONLY; merge + release + registry push
```

Hub image tags: `kirizuofficial/kirivers:latest`, `:<semver>` (multi-arch manifest), `:<semver>_amd64`, `:<semver>_arm64` (no `v` prefix). Git tag: annotated `v<semver>` on the released `main` commit.

Two name layers, never mixed:

- inner binary (`asset_name`): `kirivers-<os>-<arch>`, Windows adds `.exe` — used by the Dockerfile's `USE_PREBUILT` copy and by anything that execs the file.
- uploaded archive (`archive_name`): `KiriVers-<OS>-<Arch>-<sha6>.zip` where OS ∈ `Linux|macOS|Windows`, Arch ∈ `x86|x86_64|armv7|arm64|riscv64` (darwin uses `x86_64|arm64`), and `<sha6>` is the first 6 hex chars of the packed binary's SHA-256 (a file cannot hash itself, so the name changes every release by design).

| Target | libc | Runner | Notes |
|--------|------|--------|-------|
| linux/amd64 | musl | `ubuntu-latest` | Alpine container build |
| linux/arm64 | musl | `ubuntu-24.04-arm` | native arm64, never QEMU |
| linux/386 | glibc | `ubuntu-latest` | `g++-i686-linux-gnu` |
| linux/arm (GOARM=7) | glibc | `ubuntu-latest` | `g++-arm-linux-gnueabihf`, hard-float |
| linux/riscv64 | glibc | `ubuntu-latest` | `g++-riscv64-linux-gnu` |
| darwin/amd64 | — | `macos-15` | `clang -arch x86_64` |
| darwin/arm64 | — | `macos-15` | native |
| windows/386, amd64, arm64 | — | `windows-latest` | llvm-mingw triplets |

Assets are the 10 archives above plus `frontend-dist.zip` (contents of `frontend/dist`, `index.html` at the archive root) and `SHA256SUMS.txt`. Install tables must use the same strings derived from `kirivers_build/common.py`.

### 3. Contracts

**Image**

- Alpine runtime. Process only: no PostgreSQL/Redis packages, no `hdiffz`/`hpatchz`.
- `EXPOSE 8080 8081`. `ENTRYPOINT ["kirivers"]` (no args → `server`).
- `ENV KIRIVERS_CONFIG=/config`. Do not COPY local `*.yaml` secrets or `.env` into layers.
- `HEALTHCHECK` is busybox `wget` to admin `GET /api/v1/health`. HTTP 200 with `ready=false` is still a live process (DB/storage down). Do not AND Redis into this probe.
- linux binaries copied into Alpine must be musl (prefer static). Ubuntu glibc gcc output is not a Hub runtime artifact. `assert-linux-musl` is the CI gate, and `docker-image` refuses a host architecture that differs from the target.
- Official `go build` is always `CGO_ENABLED=1`. `build-cgo` must reject `CGO_ENABLED=0`. Darwin CGO only on a macOS host.
- Every target is asserted with `file(1)` (`assert_machine` in `kirivers_build/common.py`): once inside `build-cgo`, and again for all ten binaries in `package-assets` on the Linux packaging job. A silent cross-compiler fallback must never reach an archive.

**Compose**

- `deploy/` is the only one-click Compose example in the repo; `dev/build/` ships no Compose file (only the runtime `Dockerfile` and the release CLI). `dev/docker/` stays databases only for `go run .`.
- Passwords live only in `deploy/.env` (from `deploy/.env.example`). Wire **all** of: `POSTGRES_*` → DSN, `REDIS_PASSWORD` → `requirepass` and `KIRIVERS_CACHE_REDIS_PASSWORD`, `URL_SIGNING_SECRET` → `KIRIVERS_URL_SIGNING_SECRET`.
- Do not publish host `5432`/`6379` from `deploy/` (conflicts with `dev/docker`).
- Do not add root `compose.yml` / `docker-compose.yml`. Do not rename the Hub image.

**Maintainer CLI**

- `dev/build/kirivers.py <subcommand>` is stdlib-only on purpose: the guard job runs under `pull_request_target` with a write token, so `pip`, `setup-python`, `requirements.txt` and any third-party import are machine-refuted by `check-workflows`. The floor is Python 3.9 (macOS runners ship a 3.9 `python3` that brew cannot override), which `check-workflows` also enforces statically.
- **stdout is a wire format.** Only lines a workflow captures may reach stdout (`install-cross-toolchains` CC/CXX, `install-llvm-mingw`'s one bin dir, `next-version --format env`, the `ok|fail|skip|error` and `skip|pass|comment|close` tokens, the release-notes body). Every diagnostic, and **every child process's stdout**, goes to stderr — `common.run()` diverts it, because release.yml `eval`s those captures.
- **Never emit CRLF.** `common.force_lf()` pins both streams and files are written through `common.write_lf()`; a maintainer's own Windows box would otherwise inject `\r\n` into `eval` lines, tokens and the PR comment body. Runners are Linux, so only local runs would show the damage.
- Resolve the repo root from the module's own file (`common.ROOT`), never from `pwd`/`$PWD`/`getcwd`. Deriving it from `$PWD` is what let a Windows-style root path reach `awk -v`, where the backslash is unescaped, the type table silently failed to load and every commit fell into `### 🔖 Other`. Text is not processed through `awk`/`sed`/`grep` subprocesses anymore for the same class of reason.
- Adding a subcommand means registering it in `cli.COMMANDS` **and** asserting its call site in `check-workflows` (`kirivers\.py <name>`, subcommand token included — an assertion on the entry file alone would pass with the wrong verb).

**Cache**

- Cache **download inputs, never build outputs**. In use: `setup-go`'s default (keyed on `go.sum` + Go version + OS/arch), `setup-node` with `cache: yarn` + `cache-dependency-path: frontend/yarn.lock`, and `actions/cache` for `LLVM_MINGW_PREFIX`. `GOCACHE` content addressing is Go's own, so a restored build cache can only hit on genuinely identical inputs — a dependency or compiler change cannot reuse a stale artifact.
- The llvm-mingw key is `llvm-mingw-${{ hashFiles('dev/build/kirivers_build/build.py') }}-<os>-<arch>`: the pinned `LLVM_MINGW_VERSION` lives in that file, so hashing it *is* versioning the key without repeating the number in YAML. Bias is deliberately toward over-invalidation. No prefix-restore fallback — `check-workflows` refutes any `restore-keys:` key in `release.yml`.
- `LLVM_MINGW_PREFIX` is set at the **job** level so the cache step's `with:` and the CLI resolve the same directory; a step-level `env:` is not visible to that same step's `with:` and would silently miss forever.
- Not cached in `actions/cache`, on purpose: apt cross toolchains (system package state), the musl container's module downloads, docker layers (the CI image build only copies a prebuilt binary), and anything at all in `pr-guard.yml`. Commitlint is instead installed by the CLI into a **version-keyed** directory (`<os-cache>/kirivers/commitlint-<version>`), which answers the objection that a cache would pin whatever `@commitlint` was stored first: bumping `COMMITLINT_VERSION` opens a new directory and the old one is simply abandoned. `commitlint --self-check` asserts that keying. Cache quota is **per repository** (10 GB, 7-day LRU eviction), shared with the `sdk/*` branch workflows.

**Version (D8/D9)**

- Consider annotated tags matching `vMAJOR.MINOR.PATCH` that are ancestors of `HEAD`. Ignore `sdk-*`.
- No matching tag → release `0.1.0` even if the triggering commit is `chore:` / `docs:` / `ci:`.
- Else scan `git log <prev>..HEAD` (drop merge subjects). Rank: `feat` → minor; `fix`/`perf`/`revert` → patch; `type!:` or `BREAKING CHANGE:` → major (from `0.1.x` that is `1.0.0`). `chore`/`docs`/`ci`/`test`/`style`/`refactor`/`build` → no release (exit 0).
- Ignore `Merge pull request` / `Merge branch` subjects. Squash: the PR title is the commit to lint.
- The rank table lives in the `next-version` subcommand (server) and `RELEASE_TYPES` (SDK). Neither may read `changelog-types.conf`.

**Forge**

- `ci.yml` on `pull_request.branches:[main]` + `push.branches:[main]`: pre-flight (dependency-review, TruffleHog), commitlint, issue-link (PR into main only), `yarn build`, `CGO_ENABLED=1 go test`, `go build` with the real `frontend/dist`, plus `check-workflows` and the changelog self-check. `dorny/paths-filter` routes `frontend/**` and `**/*.go` + `third_party/hdiffpatch/**` to their own jobs (`website/**` is gated by `docs-pages.yml`). It creates no Release, pushes no image, makes no tag. **No `sdk/**` route belongs here** — see the SDK note below.
- Publishing is **manual-only**: `release.yml` triggers on `workflow_dispatch` with no inputs. Merging into `main` never publishes. There is no forced-bump input; when the range has nothing releasable the run ends **green** with a job-summary reason and produces no tag, Release or image.
- `SERVER_PUBLISH=false` (GitHub repository variable) is the kill-switch: the version is still derived, nothing is published.
- The packaging job fails when any of the ten archives is missing. Never publish a partial release, and never fabricate a platform.
- Concurrency group `server-release`. The annotated tag is created **after** the assets exist, by `softprops/action-gh-release@v2` (it replaces the hand-written `gh release create`), which uploads the 11 archives + `SHA256SUMS.txt` and takes its body from the generated notes.
- After tag + Release, the same job splices the release notes into root `CHANGELOG.md` and `git push origin HEAD:main` as `docs(release): v<semver> changelog`. That commit is made after the tag, so it is not part of the released range, and `docs:` never bumps the next version. If branch protection does not let `github-actions[bot]` write `main`, the step **fails loudly**; do not downgrade it to a skip.
- `pr-guard.yml` uses `pull_request_target` because commenting on and closing a fork PR needs a write token. It checks out `ref: main` only, never `refs/pull/*`, never `head.sha`/`head.ref`, and runs no build or test step: it reads PR metadata through the API and feeds only that to `commitlint` (stdin), `issue-link` and `guard`. Keep it that way; `check-workflows` asserts it.
- **The commit gate never judges on an empty stomach.** `kirivers.py commitlint` installs the
  pinned `@commitlint` pair itself (`npm install --prefix <os-cache>/kirivers/commitlint-<version>
  --no-save --no-package-lock`) and hands `--config` a generated file that
  `require`s the tracked `commitlint.config.cjs`, because `@commitlint/load` resolves a bare
  `extends` name from `dirname(--config)` upward and then only from the **global** npm prefix.
  `npx --package` could never satisfy that: it installs under `~/.npm/_npx/<hash>/node_modules`,
  so on a clean runner the ruleset raised `MODULE_NOT_FOUND` and exited 1 — the same code a real
  rule violation returns, which is how a healthy PR used to get closed. So: `--print-config` is the
  health probe (it runs the whole config load and never judges a commit), a failed probe or install
  exits **2**, and `pr-guard.yml`'s `case` maps 2 to `ok=error` and skips the verdict. Exit 1 always
  means commitlint itself reached a verdict. No `package.json`, lockfile or `node_modules` may appear
  at the repository root for this; the install goes into a temp directory and is published by rename,
  so a concurrent run can never see a half-written tree. That tree carries no
  `preinstall`/`install`/`postinstall` script (only `prepare`, which npm does not run for a registry
  tarball), and `npx` downloaded and ran the same pinned trees before, so this adds no new trust
  boundary — but it does put
  `registry.npmjs.org` on the gate's hot path, which is exactly why the fault must be loud (2) and
  never a verdict.

**CI security gate (two halves, physically split)**

A job that runs contributor code must never hold a write token, and a job that holds a write token must never run contributor code. Gosec is the one gate that has to do both across two workflows:

| Half | File | Trigger | Secrets | `permissions` | Runs PR code? |
|------|------|---------|---------|---------------|---------------|
| Scan | `gosec-scan.yml` | `pull_request` `types:[labeled]` `branches:[main]`, gated on `label.name == 'wait-merge'` **and** sender in `OWNER,MEMBER,COLLABORATOR` | none | `contents: read` | **yes** — plain `actions/checkout@v4` (merge ref) is correct here |
| Verdict | `security-gate.yml` | `pull_request_target` `types:[synchronize]` + `workflow_run` on `workflows: [Gosec Scan]` | write token | `contents: read`, `pull-requests: write` (+ `actions: read` on the promote job) | **never** — no checkout step at all |

- Label state machine: maintainer applies `wait-merge` → scan runs → `workflow_run` promotes to `ready-merge` on `success`, or comments and **keeps `wait-merge`** on `failure` so a maintainer can re-tag. Any `synchronize` revokes **both** labels (`gh pr edit --remove-label "ready-merge,wait-merge" || true`), which is also the TOCTOU close: a push during a scan means the promote job finds no `wait-merge` and stands down.
- The promote job resolves the PR from `workflow_run.head_sha` through `gh api repos/$REPO/commits/$SHA/pulls --jq '[.[]|select(.base.ref=="main")]|.[0].number//empty'`. `github.event.workflow_run.pull_requests` is empty for fork PRs — do not use it.
- `workflow_run.workflows:` matches the other file's top-level `name:` string exactly (`Gosec Scan`). Renaming one without the other silently kills the gate.
- > **Warning — cold start.** `pull_request_target`, `workflow_run` and `issue_comment` triggers only exist once the file is on the **default branch**. A new gate added on a feature branch cannot fire on the PR that introduces it; it starts working the run after merge. Never conclude a gate is broken from one missing run.
- Full-scope scans are slow enough that `-no-fail` plus a grep of `'"level": "error"'` in the SARIF is the pass/fail contract: HIGH/CRITICAL → `error` → blocks; MEDIUM is a warning and does not block.

**Blocking gates must not be advisory**

- No `continue-on-error:` on a step whose purpose is to block (`dependency-review`, `TruffleHog`, gosec). The one intentional exception is `pr-guard.yml`'s title check: it must not abort the job, because the fail-open decision in `guard` still has to run — the Status Check still goes red.
- Never end a real publish/push command with `|| echo "failed"`: it produces a red-in-name-only green run. Absent credential → `::warning::Missing secret <NAME>, skipping registry push` and continue (per design); present credential and a failed push → redden the job.
- `all(...)` in `--jq` is **vacuously true on an empty array**. Any "every check passed" gate needs `length > 0 and all(...)`.
- Request only the scopes you use: `id-token: write` with no OIDC login step, or `contents: write` on a job that only reads, is a finding, not a style nit.
- Every workflow file gets `check-workflows` assertions from day one. An unasserted YAML is an unguarded invariant — the split above was broken precisely because the new gate had no assertions yet.
- `sdk-automerge.yml` is the only consumer of `ready-merge`: it merges `--squash` into the `sdk/*` orphan branch and then, in the same run, tags, releases and pushes registries. `GITHUB_TOKEN` merges do not broadcast events, which is what keeps this single-workflow shape free of a double release. Its `registry-publish` job is a separate `permissions: contents: read` matrix.
- Each `sdk/<lang>` branch owns its SDK gate. GitHub resolves a pull request's workflows from its **base** branch (and a push's from the pushed branch), so `main`'s `ci.yml` structurally cannot run for a pull request into `sdk/<lang>`: a trigger, a path filter or a job for those branches here is inert, and reads as coverage that never happens. A branch holding `scripts/sdk_release.py` must therefore carry its own `ci.yml` (`pull_request` naming **that** branch + `commitlint`) and `release.yml`; a README-only pointer branch (`go`, `php`, `swift`) must carry none. `check-workflows` walks `refs/remotes/origin/sdk/*` — CI checkouts never have `refs/heads/sdk/*` — and asserts both halves of that contract.

**Changelog format (shared by server and SDK)**

- Section = commit `type`, heading = `### <emoji> <label>`; emoji, label and **section order** come from the type table (file order wins). Empty sections are omitted; two runs are byte-identical.
- Entry = `- **<scope>**: <subject> (<sha7>) #<PR>`. Scope is trimmed and lowercased before it is written; an entry with no scope drops the `**scope**:` prefix rather than falling into a catch-all section.
- Breaking entries (`type!:` or a `BREAKING CHANGE:` / `BREAKING-CHANGE:` footer) leave their type section and collect in the first section `### 💥 Breaking Changes`, where the type word is written back into the line because the heading no longer carries it.
- Non-Conventional subjects are not listed (same set that yields rank `none`); an unregistered but valid type lands in `### 🔖 Other` so nothing is dropped silently; an empty range prints `No user-facing Conventional Commits in this range.`; a baseline (no ancestor tag) states the baseline and replays no history.
- The `#<PR>` tail is decoration: `gh api .../commits/<sha>/pulls` per commit, and any failure (no token, no `gh`, offline) omits it. A subject already ending in `(#123)` is not suffixed twice.
- Type table line format is `type=emoji[=label]` (a space instead of the second `=` is accepted). A `#`-prefixed line shaped like `# type=…` hides that type; other `#` lines are prose. Invalid rows (key not `^[a-z][a-z0-9-]*$`, missing emoji, duplicate key, key both active and hidden) **exit non-zero**. A missing file falls back to the built-in default, and both self-checks assert the shipped file parses identically to that default.
- Commenting a type out hides it from the changelog only. `release-notes --self-check` asserts `next-version --format env` is byte-identical before and after such an edit.

**Secrets (names only)**

| Name | Use |
|------|-----|
| `DOCKERHUB_USERNAME` | Hub user |
| `DOCKERHUB_TOKEN` | Hub login (stdin, then unset) |
| `SERVER_PUBLISH` | `false` disables Release+Hub for that run |
| `GITHUB_TOKEN` | tag, Release, `CHANGELOG.md` push, PR-number reads, `sdk-automerge` merge+release (no external PAT) |
| `CARGO_REGISTRY_TOKEN` | `sdk/rust` → crates.io via `katyo/publish-crates@v2` |
| `PYPI_TOKEN` | `sdk/python` → PyPI via `pypa/gh-action-pypi-publish@release/v1` |
| `NPM_TOKEN` | `sdk/typescript` → npm via `JS-DevTools/npm-publish@v3` |
| `NUGET_API_KEY` | `sdk/csharp` → NuGet (`setup-dotnet@v4` + `dotnet nuget push`) |
| `PUB_CREDENTIALS` | `sdk/dart` → pub.dev (`dart-lang/setup-dart@v1` + `dart pub publish --force`) |
| `MAVEN_CENTRAL_USERNAME` / `_PASSWORD` | `sdk/java` (`./mvnw deploy`) and `sdk/kotlin` (`./gradlew publish`) → Central; both must be present or the pair is skipped |

Registry push routes are the table in `docs/sdk-publish.md`; `sdk/c` and `sdk/cpp` ship a `git archive` source zip on the Release, and `go-src`/`php-src`/`swift-src` are tag-driven (Go modules, Packagist hook, SPM) with no key.

Missing Hub creds fail the image step naming those vars. Logs must not print token values.

**Publish kill-switches** are repository **variables**, not secrets, read as `${{ vars.* }}`, and each one means "derive the version, publish nothing": `SERVER_PUBLISH=false` gates `release.yml`, `SDK_PUBLISH=false` gates the per-branch `sdk/<lang>` release workflows. `sdk-automerge.yml` deliberately reads **neither** — the batch channel's brake is the maintainer's manual dispatch plus whether the PR carries `ready-merge`. Do not add an `SDK_PUBLISH` check there, and do not let a doc imply the batch channel honors it (`docs/sdk-publish.md` scopes it explicitly).

### 4. Validation & Error Matrix

| Condition | Result |
|-----------|--------|
| No ancestor `v*` tag | `KIRIVERS_RELEASE=yes` `KIRIVERS_VERSION=0.1.0` `KIRIVERS_TAG=v0.1.0`; notes are the baseline text, no sections |
| Only `docs:` / `ci:` / `chore:` since last `v*` | `KIRIVERS_RELEASE=no`; manual run ends green, nothing published |
| `feat:` since last `v*` | minor bump; tag + Release + CHANGELOG + images |
| `feat!:` or `BREAKING CHANGE:` on `0.1.x` | `1.0.0` |
| `SERVER_PUBLISH=false` | version derived, `publish=no`, every downstream job skipped |
| Squash PR title not Conventional Commits | `commitlint` fails the PR (exit 1); `pr-guard.yml` comments and closes it for non-members |
| commitlint cannot install, or its ruleset cannot load | exit **2**, never 1: `pr-guard.yml` records `ok=error` and skips the verdict instead of closing the PR, and `ci.yml` shows the npm failure |
| commitlint's tool directory is mid-install for another run | that run sees nothing or a complete tree (rename publish); a torn tree fails the `--print-config` probe and is discarded, not cached |
| Push event with `before` all zeros or unreachable | `commitlint` degrades to the tip commit and says so |
| PR into `main` with no linked issue | `issue-link` exits 1 printing the fix; `sdk/*` PRs and direct pushes are not gated |
| `CGO_ENABLED=0` official build | `build-cgo` / `go build .` fail |
| glibc linux binary offered to Alpine | `assert-linux-musl` fails; `docker-image` refuses |
| Cross binary is actually the host arch | `assert_machine` fails in `build-cgo` and again in `package-assets` |
| A matrix target failed | that job is red; `package`/`github-release` fail on the missing archive (fail-fast off, so the rest still report) |
| No `ubuntu-24.04-arm` runner available | the arm64 job fails — do not substitute a QEMU build |
| `changelog-types.conf` row malformed | `release-notes` exits non-zero naming the line |
| `CHANGELOG.md` push rejected by protection | the step fails with the reason; the Release stays published |
| `on: push` or `on: tags` added to release workflow | **forbidden** — `check-workflows` fails |
| `wait-merge` label applied by a non-OWNER/MEMBER/COLLABORATOR | `gosec-scan.yml` job `if` is false — no scan, no promotion |
| Gosec SARIF contains `"level": "error"` (HIGH/CRITICAL) | scan job red → gate comments the block reason and **keeps `wait-merge`**; no `ready-merge` |
| Gosec job `skipped`/`cancelled`, or PR lost `wait-merge` mid-scan | promote job's default branch: leaves labels untouched, exits 0 |
| New commit pushed while a scan is running | `synchronize` revokes both labels; promote then finds no `wait-merge` and stands down |
| `sdk-automerge` PR with **zero** checks | not merged — `length > 0 and all(...)`; an empty check list is never a green light |
| Registry secret absent | `::warning::Missing secret <NAME>, skipping registry push`, run stays green |
| Registry secret present but push fails | job reddens and names the language; merge+Release already done upstream, nothing rolls back |

### 5. Good/Base/Bad Cases

- Good: maintainer runs Release after a `feat:` merge → `v0.2.0`, ten archives + `frontend-dist.zip` + `SHA256SUMS.txt`, notes grouped by type, `CHANGELOG.md` commit on `main`, images `0.2.0` / `0.2.0_amd64` / `0.2.0_arm64` / `latest`.
- Base: first server tag is `v0.1.0` even if the last merge subject is `ci:`; notes carry the baseline sentence only.
- Bad: `GOOS=linux go build` on Ubuntu then `COPY` into Alpine; adding `on: push` back to `release.yml`; publishing six of ten targets because one job was red; building linux/armv7 under QEMU to "complete" the matrix; fabricating darwin files on Linux; letting `pr-guard.yml` check out the PR merge ref to run its tests; reading `changelog-types.conf` from `next-version`; hand-editing root `CHANGELOG.md`; running Gosec over PR code inside the write-token workflow instead of on `pull_request`; `continue-on-error: true` on TruffleHog or dependency-review; `... || echo failed` after a real `cargo publish`; treating `all(.state=="SUCCESS")` over an empty check list as a green light; merging `ready-merge` PRs from a workflow that has no `check-workflows` assertions yet.

### 6. Tests Required

- `python3 dev/build/kirivers.py check-workflows` — asserts the dispatch-only trigger, no `tags:`, no QEMU step, ten targets, `frontend-dist.zip`, CHANGELOG + docker jobs, CI publishing nothing, the guard's security invariants, that `dev/build/` holds no shell or compose file, that the workflows call real subcommands (and that the windows job uses `python`, not `python3`), that every expression in a workflow file (braced or bare `if:`) calls only a name in `ACTIONS_FUNCTIONS` — a made-up function is a whole-file parse failure, not a bad value — and (when the refs exist) every `sdk/*` workflow's merged-PR trigger plus the absence of `.gitlab-ci.yml`.
- `python3 dev/build/kirivers.py release-notes --self-check` — §5.1 format, lowercase scope, PR tail, hidden-type behaviour, section order, determinism, embedded-default parity, and the byte-identical `next-version` assertion.
- `python3 dev/build/kirivers.py guard --self-check`, `python3 dev/build/kirivers.py issue-link --self-check` and `python3 dev/build/kirivers.py commitlint --self-check` (all three also run inside `check-workflows`; the commitlint suite is offline — tool-directory keying, the generated config's delegation, and an unwritable config reporting a fault instead of raising).
- `python3 dev/build/kirivers.py next-version --format env` with no `v*` → `0.1.0`.
- `python3 dev/build/kirivers.py asset-names` equals the install-page prefixes (compare after replacing `<sha6>` with `*`).
- `CGO_ENABLED=0 go build .` fails (hdiffc); `CGO_ENABLED=1 go build .` succeeds.
- `package-assets` on ten fixture binaries: 11 `-c`-verifiable lines, 10 commented inner hashes, `<sha6>` equal to each binary's prefix.
- GitHub `ci.yml` has no Release/`docker push`/`git tag` step; `release.yml` has no `push:` or `tags:` trigger.
- `python3 dev/build/kirivers.py check-workflows` also asserts the gate split: `security-gate.yml` checks out nothing and never reads `pull_request.head`, revokes both labels; `gosec-scan.yml` triggers on `pull_request` only and holds no write scope; `sdk-automerge.yml` is dispatch-only, requests no `id-token`, warns-and-skips on a missing registry secret, and uses the action names pinned in this file. Any workflow file without assertions here is unfinished work.
- Image: no `postgres`/`redis`/`hdiffz`/`hpatchz` binaries; HEALTHCHECK URL is `/api/v1/health`.
- Compose: `deploy/` only — no host `5432`/`6379`, URL-signing env wired.

### 7. Wrong vs Correct

#### Wrong

```yaml
# .github/workflows/release.yml — merging must never publish
on:
  push:
    branches: [main]
# linux/arm64 built under QEMU:
  - uses: docker/setup-qemu-action@v3
# and a glibc file copied into Alpine:
  run: CGO_ENABLED=1 go build -o kirivers-linux-arm64 .
```

```yaml
# pr-guard.yml — pull_request_target with a write token must never run PR code
- uses: actions/checkout@v4            # defaults to refs/pull/N/merge here
- run: go test ./...
```

```yaml
# security-gate.yml — ONE workflow doing both halves: the scan executes the
# contributor's Go code inside a pull_request_target job that also holds
# pull-requests:write, so a malicious repo can relabel itself ready-merge
on:
  pull_request_target:
    types: [labeled]
jobs:
  gate:
    permissions: {contents: read, pull-requests: write}
    steps:
      - uses: actions/checkout@v4
        with: {ref: ${{ github.event.pull_request.head.sha }}}   # <-- untrusted code
      - uses: securego/gosec@master
      - run: gh pr edit "$PR" --add-label ready-merge
```

```yaml
# a "block" gate that cannot block, and a publish that cannot fail
- uses: actions/dependency-review-action@v4
  continue-on-error: true
- run: cargo publish && echo "published" || echo "publish failed, continuing"
- run: gh pr checks "$N" --json state --jq 'all(.state == "SUCCESS")'   # true on []
```

```sh
# next-version reading the display table
grep -q '^fix=' dev/build/changelog-types.conf || rank=0
```

#### Correct

```yaml
on:
  workflow_dispatch:
jobs:
  linux-musl:
    strategy:
      matrix:
        include:
          - {goarch: amd64, runner: ubuntu-latest}
          - {goarch: arm64, runner: ubuntu-24.04-arm}
    runs-on: ${{ matrix.runner }}
    steps:
      - run: python3 dev/build/kirivers.py build-linux-musl ${{ matrix.goarch }} dist/kirivers-linux-${{ matrix.goarch }}
```

```yaml
on: pull_request_target
permissions: {contents: read, issues: read, pull-requests: write}
jobs:
  guard:
    if: github.event.pull_request.base.ref == 'main'
    steps:
      - uses: actions/checkout@v4
        with: {ref: main, fetch-depth: 1}
      - run: python3 dev/build/kirivers.py guard --record pr-record.txt --out pr-comment.md
```

```yaml
# gosec-scan.yml — UNTRUSTED half: pull_request carries no secrets and a
# read-only token, so running the gosec toolchain over PR code is safe
on:
  pull_request:
    types: [labeled]
    branches: [main]
permissions: {contents: read}
jobs:
  gosec:
    if: |
      github.event.label.name == 'wait-merge' &&
      contains(fromJSON('["OWNER", "MEMBER", "COLLABORATOR"]'), github.event.sender.association)
    steps:
      - uses: step-security/harden-runner@v2
      - uses: actions/checkout@v4          # merge ref: scans exactly what would land
      - uses: securego/gosec@master
```

```yaml
# security-gate.yml — WRITE half: reacts to the finished run, checks out nothing
on:
  pull_request_target: {types: [synchronize], branches: [main]}
  workflow_run: {workflows: [Gosec Scan], types: [completed]}
permissions: {contents: read, pull-requests: write}
jobs:
  promote-after-scan:
    if: github.event_name == 'workflow_run' && github.event.workflow_run.event == 'pull_request'
    env:
      HEAD_SHA: ${{ github.event.workflow_run.head_sha }}   # never inline in run:
```

```yaml
# images: native per-arch build-push from the staged musl binary, then a
# registry-side manifest — release.yml never shells out to kirivers.py here
- uses: docker/login-action@v3
- uses: docker/build-push-action@v6
  with:
    build-args: USE_PREBUILT=1
    tags: kirizuofficial/kirivers:${{ needs.version.outputs.version }}_amd64
- run: docker buildx imagetools create -t ...:latest -t ...:$V $V_amd64 $V_arm64
```

```sh
# local-only equivalent of the two steps above (not what CI runs)
python3 dev/build/kirivers.py docker-image amd64 "$V"     # -> :${V}_amd64
python3 dev/build/kirivers.py docker-manifest "$V"        # -> :latest and :$V
```

---

## Common Mistake: glibc binary in Alpine

**Symptom**: Hub container exits immediately (`not found` / loader errors) even though `docker build` succeeded.

**Cause**: linux CGO artifact was built on Ubuntu/glibc and COPY’d into `alpine`.

**Fix**: the `build-linux-musl` and `assert-linux-musl` subcommands (`python3 dev/build/kirivers.py …`). Do not switch the runtime to Debian/`gcompat` without a new product decision.

**Prevention**: treat Ubuntu `go test` CGO as a **test** job only; never as the Alpine COPY source. Only musl amd64/arm64 reach the image; the glibc cross builds ship as Release archives and need glibc ≥ 2.39 on the target.

---

## Common Mistake: Hub compose missing URL signing

**Symptom**: the stack comes up healthy but signed downloads fail, because the process never received `URL_SIGNING_SECRET`.

**Cause**: `.env.example` lists the key but the Compose file does not map it into the container's `KIRIVERS_*` environment.

**Fix**: keep the DSN / Redis password / URL-signing env mappings wired in `deploy/compose.yml` (the only Compose that ships the process; `dev/docker/compose.yml` is databases only and runs no KiriVers).

---

## Common Mistake: "one more forge is free"

**Symptom**: a second CI file (`.gitlab-ci.yml`, a mirror job, a second publish path) reappears, and the two sides race the same `:latest` or semver tag.

**Cause**: treating a mirror as a harmless copy instead of a second release entry point.

**Fix**: GitHub only. Delete the second pipeline, and move any capability it had into the `dev/build/kirivers.py` CLI so it stays runnable locally. `check-workflows` fails on a GitLab file, and on the retired `gitlab-release.sh` / `docker-push.sh` / `checksums.sh` paths under `dev/build/`.

---

## Forbidden

- Root `compose.yml` / `docker-compose.yml`.
- Postgres or Redis inside the KiriVers image.
- PATH `hdiffz`/`hpatchz` in the image or as a publish dependency.
- `on: push` or `on: tags` for server Release; publishing is manual-only.
- QEMU / `setup-qemu-action` / `binfmt` anywhere in the build or image path.
- Fabricating darwin assets, or Linux→darwin CGO without an Apple SDK (osxcross is not a release path).
- Letting `pr-guard.yml` check out or execute pull-request code.
- Reading `changelog-types.conf` from any version-bump path.
- Printing `DOCKERHUB_TOKEN` or putting real passwords in committed `.env.example`.
- Changing `deploy/compose.yml` image away from `kirizuofficial/kirivers`.
- Re-adding a second forge's pipeline "for parity".
- A `.sh` or a `compose.yml` under `dev/build/`; the release toolchain is the Python CLI and the only Compose example is `deploy/`.
- Progress or child output on the stdout of a subcommand a workflow captures (`eval`, `$(...)`, `>>$GITHUB_PATH`) — that stream is a wire format.
- Installing a Python dependency for the CLI (stdlib only, so the guard path stays free of a supply chain).
- A `restore-keys:` prefix fallback on a toolchain cache, or any cache inside `pr-guard.yml`.
- Checking out or executing pull-request code in a `pull_request_target` / `workflow_run` job. Untrusted scans go on `pull_request`; the write half only reacts to a finished run.
- `continue-on-error:` on a step whose contract is to block. (`pr-guard.yml`'s title check is the one sanctioned exception, because `guard`'s fail-open verdict must still run.)
- `|| echo`, `|| true`, or `continue-on-error` around a registry push that was actually attempted — a swallowed publish failure is forbidden. (`|| true` on label *removal* is fine: revoking is best-effort.)
- `all(...)` in a jq gate without a preceding `length > 0`.
- An inline `${{ github.event.* }}` (or any requester-controlled expression) inside `run:`. Pass it through `env:`.
- `id-token: write`, `contents: write`, or `pull-requests: write` on a job or workflow that has no step needing it.
- A new `.github/workflows/*.yml` with no `check-workflows` assertions.
- Using `github.event.workflow_run.pull_requests` to find the PR — empty for fork PRs.
