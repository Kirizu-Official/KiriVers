---
title: "Repository layout"
description: "Live default-branch tree. Includes frontend/embed.go and internal/delta. No language SDK sources on default branch."
---

# Repository layout

This matches the default-branch tree. Language SDKs are **not** here. `.trellis/` is maintainer/AI workflow, not a required contributor path. GitHub is the only forge: checks and releases live in `.github/workflows/`, there is no second pipeline definition in the repo.

```text
main.go                 # delegates to cmd.Run()
CHANGELOG.md            # changelog sections inserted by the release job (appears after the first release); never hand-edit
cmd/                    # server / admin / listen (listen is not a CLI command)
configs/                # *-example.yaml; local yaml gitignored
internal/
  cache/
  config/
  controller/           # admin/ + client/ + openapi.*.json
  database/
  delta/                # delta engines
  logger/
  middleware/
  model/
  platform/
  repository/
  service/              # includes update/, store/
  storage/              # local + s3
pkg/
  hashutil/ pathutil/ response/ semver/
  signature/ urlsign/ webhook/ grayutil/
frontend/               # Vue admin; embed.go embeds dist; daily yarn dev
frontend/embed.go
frontend/dist/.gitkeep
docs/                   # internal notes (not the VitePress root)
dev/docker/             # developer Postgres/Redis
dev/build/              # runtime Dockerfile and the release CLI (no shell scripts, no Compose)
  Dockerfile            # Alpine runtime image, build context = repo root
  changelog-types.conf  # section emoji/labels — display only, never feeds version derivation
  kirivers.py           # single entry point: python3 dev/build/kirivers.py <command> (python on Windows)
  kirivers_build/       # standard-library package, one module per concern; subcommand names keep the old script names
    cli.py              # subcommand registry and dispatch (usage and exit-code conventions)
    common.py           # single definition of the platform matrix, inner binary and archive names, `file` patterns, sha256
    build.py            # build-cgo / build-linux-musl / assert-linux-musl / install-cross-toolchains / install-llvm-mingw / frontend-build
    version.py          # next-version: derive the next version from Conventional Commits
    notes.py            # release-notes (Release body and CHANGELOG section) plus the changelog write-back
    package.py          # asset-names and package-assets (zips + SHA256SUMS.txt)
    images.py           # docker-image (native per-arch build, pushes <semver>_amd64 / _arm64) and docker-manifest
    guards.py           # commitlint / issue-link (PRs into main must link an issue) / guard (decision matrix)
    selfcheck.py        # check-workflows: static gate on the workflows (triggers, matrix, no QEMU)
.github/workflows/      # ci.yml (PR + main push checks), pr-guard.yml, gosec-scan.yml + security-gate.yml (sandboxed Gosec scan and ready-merge state machine), docs-pages.yml (compiles website/** and publishes gh-pages), release.yml (manual workflow_dispatch only), sdk-automerge.yml (manual only; batch-merges sdk/* PRs, releases and pushes to registries)
website/                # official docs site
deploy/                 # operator one-click Compose (.env credentials + bundled YAML)
scripts/install-deps.sh
```
