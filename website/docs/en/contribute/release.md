---
title: "Release and Docker"
description: "Manually triggered server release, Conventional Commits version derivation, the ten-target matrix and libc policy, changelog format and customisation, multi-arch Docker tags."
---

# Release and Docker

Maintainers publish the process image and the official binaries from the default branch. Operator install docs live under [Guide → Install](/en/guide/install/). This page covers the in-repo pipelines and the local `dev/build` toolchain only. The release toolchain has a single entry point, `dev/build/kirivers.py`: `python3 dev/build/kirivers.py <command>` on Linux / macOS and `python dev/build/kirivers.py <command>` on Windows (standard library only, nothing to install). The rest of this page names just the subcommand.

GitHub is the only forge: checks, releases and images all run in GitHub Actions, and the repo holds no second pipeline.

## Paths

| Purpose | Path |
|---------|------|
| Runtime image (Alpine, no database) | `dev/build/Dockerfile`, build context = repo root |
| One-file run (Hub image + Postgres + Redis) | `deploy/`; passwords in `deploy/.env` (operator docs, see [Docker Compose](/en/guide/install/compose)) |
| Release CLI | `dev/build/kirivers.py` + package `dev/build/kirivers_build/` (standard-library Python; workflows only orchestrate, every decision lives in the CLI) |
| GitHub PR checks | `.github/workflows/ci.yml` (`pull_request` + `push` → `main`) |
| PR guard | `.github/workflows/pr-guard.yml` (title and issue link, see [Pull requests](/en/contribute/pull-requests)) |
| Security gate | `.github/workflows/gosec-scan.yml` (the `wait-merge` label starts a sandboxed Gosec scan) + `.github/workflows/security-gate.yml` (a clean scan auto-promotes to `ready-merge`; new commits instantly revoke `wait-merge` / `ready-merge`) |
| GitHub release | `.github/workflows/release.yml` (**only** `workflow_dispatch`; never `on: push`, never `on: tags`) |
| Changelog type table | `dev/build/changelog-types.conf` (display layer; not part of version derivation) |
| Changelog appended by releases | repo-root `CHANGELOG.md` (written back to `main` by `dev/build/kirivers.py changelog`) |
| Official SDK releases | the `.github/workflows/` each `sdk/<lang>` branch carries; maintainers can also run `sdk-automerge.yml` from the default branch (**only** `workflow_dispatch`) to batch-merge `ready-merge` PRs and push to each language registry in the same run, see `docs/sdk-publish.md` |

Do not add `compose.yml` / `docker-compose.yml` at the repo root. The image installs no `hdiffz` / `hpatchz`; official HDIFF13 is produced in-process via CGO.

## Trigger: merging to main does not publish

Server release is **manual-only**: merging a PR into `main` runs checks. To release, open Actions → Release → **Run workflow**.

- The version is derived automatically. There is **no** field to type a version and no "force one level up" switch.
- When the range holds no releasable commits (only `chore:` / `docs:` / `ci:` and friends), the run **finishes green**: no tag, no Release, no image push, and the job summary states the reason.
- `SERVER_PUBLISH` is the manual kill-switch: set to `false`, the run derives the version even when there are releasable commits and produces no external side effect at all.

## Versioning (same rules as the SDKs, different tag prefix)

Server tags are annotated `v<semver>` on an ancestor of `main` (for example `v0.1.0`). The previous-version lookup ignores `sdk-*-v*`.

1. No server `v*` → publish the baseline **`0.1.0`** (even when the tip commit is `ci:` / `chore:`). Historical `feat` commits are not replayed.
2. A previous `v*` exists → `git log <prev>..HEAD` (no merges): `feat` → minor; `fix` / `perf` / `revert` → patch; body `BREAKING CHANGE:` or `type!:` → major. `chore` / `docs` / `ci` / `test` / `style` / `refactor` **do not** release.
3. A breaking change from `0.1.x` jumps to `1.0.0`.
4. A pure `docs:` merge into `main`: checks pass, and a manual run releases nothing — no new tag, no Hub push.
5. There is no server VERSION file, and the server never commits `chore(release)`.

Implementation: the `next-version` subcommand (`dev/build/kirivers_build/version.py`). PRs are validated by `commitlint` against the title and the commits; on a direct push to `main` it degrades to the `before..after` range (checking only the tip commit when `before` is all zeros or unreachable). A non-conforming squash title fails the check.

## Platform matrix: ten targets, zero QEMU

Always `yarn build` first, then **`CGO_ENABLED=1`**. `CGO_ENABLED=0` is not a release path (CI asserts that it fails to compile).

| Target | libc | Host | How it builds |
|--------|------|------|---------------|
| linux/amd64 | **musl** | `ubuntu-latest` | native gcc/g++ inside an Alpine container |
| linux/arm64 | **musl** | `ubuntu-24.04-arm` | same, on a native arm64 host |
| linux/386 | glibc | `ubuntu-latest` | cross from amd64: `g++-i686-linux-gnu` |
| linux/arm (armv7) | glibc | `ubuntu-latest` | cross from amd64: `g++-arm-linux-gnueabihf` |
| linux/riscv64 | glibc | `ubuntu-latest` | cross from amd64: `g++-riscv64-linux-gnu` |
| darwin/amd64 | — | `macos-15` | Apple clang `-arch x86_64` |
| darwin/arm64 | — | `macos-15` | native Apple clang |
| windows/386 | — | `windows-latest` | llvm-mingw `i686-w64-mingw32` |
| windows/amd64 | — | `windows-latest` | llvm-mingw `x86_64-w64-mingw32` |
| windows/arm64 | — | `windows-latest` | llvm-mingw `aarch64-w64-mingw32` |

- **No QEMU user-mode emulation anywhere**: musl is used only on architectures that can build it natively; the remaining Linux targets fall back to glibc cross builds. The cost is stated on [manual install](/en/guide/install/manual): glibc artifacts require glibc ≥ 2.39 on the target machine and armv7 requires hard-float.
- A cross compiler silently falling back to the host binary is the most dangerous failure mode, so `build-cgo` asserts the machine type with `file` after every target, and packaging asserts again over all ten artifacts inside the Linux job.
- darwin artifacts are produced only on a macOS host, **never** fabricated from Linux.
- Build jobs run `fail-fast: false`; but `package` and `github-release` must fail when an artifact is missing. No release beats half a release.

## Artifacts: archive names carry the content hash

Internal binary names stay stable (`kirivers-<os>-<arch>[.exe]`); what is uploaded is one zip per target:

```
KiriVers-<OS>-<Arch>-<sha6>.zip
KiriVers-Linux-x86_64-449c6d.zip     KiriVers-macOS-arm64-ad1a24.zip
KiriVers-Linux-x86-108619.zip        KiriVers-Windows-x86-635890.zip
KiriVers-Linux-armv7-ec6a8c.zip      KiriVers-Windows-x86_64-abb5fc.zip
KiriVers-Linux-arm64-0c690f.zip      KiriVers-Windows-arm64-0f98c4.zip
KiriVers-Linux-riscv64-074587.zip
frontend-dist.zip                    SHA256SUMS.txt
```

`<sha6>` is the first 6 lowercase hex characters of the SHA-256 of the binary inside the archive (a file cannot hash itself, so the name changes on every release) — recognise the archives by prefix on the Release page. OS tokens are `Linux` / `macOS` / `Windows`; Arch tokens are `x86` / `x86_64` / `armv7` / `arm64` / `riscv64` (darwin uses `x86_64` / `arm64`). The mapping is defined only in `dev/build/kirivers_build/common.py` (`MATRIX` / `archive_name`); `asset-names` and the table above derive from it.

`SHA256SUMS.txt` has two sections: the first lists the 11 uploaded files and is directly verifiable with `sha256sum -c SHA256SUMS.txt`; the second records the hashes of the 10 in-archive binaries as `#` comments (the prefix keeps `-c` from parsing them), so you can compare them after extracting.

`frontend-dist.zip` serves the "I only want to swap the admin console files" case: it holds the contents of `dist/`, so `index.html` sits at the root of the archive. Official binaries already embed the console; operators do not need this file.

## Changelog: one section per type, scope as the prefix {#changelog-format}

The Release body and the repo-root `CHANGELOG.md` are generated by the same `release-notes` subcommand (`dev/build/kirivers_build/notes.py`), in the format `scripts/sdk_release.py` uses on the SDK branches:

```markdown
## v0.3.0 — 2026-10-05

### 💥 Breaking Changes
- **admin** feat: 重写 API Token 轮换，旧接口下线 (8a9b0c1) #31

### ✨ Features
- **delta**: 支持流式源文件 (4f5e6d7) #28
- 直推且没有 scope 的 feat 照进本节 (6bc7e20)

### 🐛 Bug Fixes
- **delta**: 拒绝截断的 HDIFF13 头部 (a1b2c3d) #29
```

- A section is a commit `type`; its heading is `### <emoji> <label>`. The emoji, the label and the **section order** all come from the type table — file order is section order.
- An entry is `- **<scope>**: <description> (<sha7>) #<PR>`. The `scope` is lowercased and stripped of whitespace (`perf(DB)` and `perf(db)` are the same label). With no scope, the `**scope**:` prefix is omitted; the entry is not dumped into any catch-all section.
- Breaking entries (`type!:` or a body `BREAKING CHANGE:` / `BREAKING-CHANGE:`) leave their own type section, gather in the first section `### 💥`, and get the type word restored inline — because the section heading no longer carries it.
- The PR number is **decoration**: the job queries `gh api .../commits/<sha>/pulls` and silently omits it when the lookup fails, `gh` is missing or the network is down. It never blocks a release. A subject that already ends in `(#123)` does not get a second number.
- Subjects that are not Conventional Commits do not appear (the same set that "does not release"). A valid but unregistered type lands in `### 🔖 Other`, so **entries are never dropped silently**. When the range holds no displayable entry at all, the output is `No user-facing Conventional Commits in this range.`
- Without a previous `v*` tag the output is the baseline statement only: no history replay, no sections.

Two runs over the same range produce byte-identical output.

### Custom emoji / labels / hiding a type

The only entry point is `dev/build/changelog-types.conf` (`scripts/changelog-types.conf` on the SDK branches, byte-identical across all twelve copies):

```ini
breaking=💥=Breaking Changes
feat=✨=Features
fix=🐛=Bug Fixes
perf=⚡=Performance
revert=⏪=Reverts
refactor=♻️=Refactoring
# docs=📝=Documentation
# chore=🔧=Chores
```

One entry per line, `type=emoji[=section label]` (`type=emoji label` is accepted as well). A full-line comment of the form `# type=…` means **that type stays out of the changelog**; every other `#` line is prose. Malformed input — a key that does not start with a lowercase letter, a missing emoji, the same type listed twice — exits with an error instead of being ignored. When the file is missing, the script falls back to its built-in default table and asserts that "the parsed result equals the file", so the two copies of the defaults cannot drift.

::: danger Hold this invariant
The type table is **a display layer only**. The bump table in `next-version` and `RELEASE_TYPES` in `sdk_release.py` never read it — commenting out `fix=` only removes the Bug Fixes section; the patch bump still happens. The regression assertion lives in `release-notes --self-check`: `next-version --format env` must print byte-identical output before and after a config change.
:::

## CHANGELOG.md is written back to main

After creating the tag and the Release, the `github-release` job prepends the header section to `CHANGELOG.md` and runs `git push origin HEAD:main` with the commit message `docs(release): v<semver> changelog` (`docs:` does not count toward the next release, so it cannot invent a version delta). That commit is therefore not part of the released commit set and is not an ancestor of the `<semver>` tag.

This requires the repository to let `github-actions[bot]` write `main` with `GITHUB_TOKEN` (the same shape as allowing a bot to merge on the SDK branches). If branch protection does not permit it, the step **fails and states why**; it never skips silently. The SDK branches behave the same way, and `prepend_changelog` creates `# Changelog` when the file is absent.

## Docker: native build per architecture, manifest composed remotely

| Tag | Meaning |
|-----|---------|
| `kirizuofficial/kirivers:latest` | points at the current release |
| `kirizuofficial/kirivers:<semver>` | the release's multi-arch manifest |
| `kirizuofficial/kirivers:<semver>_amd64` | that version's single-architecture amd64 image |
| `kirizuofficial/kirivers:<semver>_arm64` | that version's single-architecture arm64 image |

Each of the two architectures builds on a **native runner** with `docker/login-action@v3` + `docker/build-push-action@v6` (`--build-arg USE_PREBUILT=1` reuses the already-built musl binary) and pushes its per-architecture tag; the `docker-manifest` job then composes `:latest` and `:<semver>` registry-side with `buildx imagetools create`. `imagetools create` executes nothing inside the image, so no QEMU or binfmt is needed. glibc artifacts **never** enter the Alpine image: each per-architecture job COPYs only a `linux-musl` matrix binary (musl assertion passed); the local `docker-image` path likewise accepts only binaries that passed the musl assertion and requires the host architecture to match the target.

Job ordering guarantees the manifest is composed only once both per-architecture tags have been pushed — composing the wrong tag would poison `:latest`.

## Secrets and disabling publish

Document names only, never values:

| Name | Use |
|------|-----|
| `DOCKERHUB_USERNAME` | Docker Hub user |
| `DOCKERHUB_TOKEN` | Docker Hub login |
| `SERVER_PUBLISH` | GitHub repository variable; with `false` the run only derives the version |
| `GITHUB_TOKEN` | provided by the workflow; writes the tag, the Release and `CHANGELOG.md`, reads PR numbers |

Missing Hub credentials fail the **image push** step and name `DOCKERHUB_USERNAME` and/or `DOCKERHUB_TOKEN`; logs must not print the token. Concurrency group: `server-release`.

## Local image build

```bash
docker build --platform linux/amd64 -t kirivers:linux-amd64 -f dev/build/Dockerfile .
```

For day-to-day runs use the Hub image from the operator one-click pack `deploy/` (`image: kirizuofficial/kirivers:latest`, see [Docker Compose](/en/guide/install/compose)). A local `docker build` is only for iterating on the Dockerfile before pushing to Hub. The image defaults `KIRIVERS_STORAGE_LOCAL_ROOT=/data/storage` and the log directory to `/data/logs`. Bind-mount a host directory at `/config` and `/data` to persist the YAML, the full and delta objects, and the logs.

HEALTHCHECK hits the admin plane `GET /api/v1/health` (HTTP 200 even without a database). The container runs as root so bind mounts keep simple ownership.

## Must-test items

After touching this chain, run at least:

```bash
python3 dev/build/kirivers.py check-workflows        # static gate: triggers, matrix, zero QEMU, no second-forge residue
python3 dev/build/kirivers.py release-notes --self-check
python3 dev/build/kirivers.py guard --self-check
python3 dev/build/kirivers.py issue-link --self-check
CGO_ENABLED=1 go build . && CGO_ENABLED=0 go build .   # the second must fail
```

On Windows replace `python3` with `python`. `check-workflows` already encodes assertions such as "only `workflow_dispatch`", "no `setup-qemu-action`" and "`pr-guard.yml` never checks out PR code", and the `changelog` job in CI runs it.
