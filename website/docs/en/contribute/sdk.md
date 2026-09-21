---
title: "SDK source branches"
description: "Languages live on sdk/<lang> orphan branches. Go/PHP/Swift sources are *-src plus dedicated repos."
---

# SDK source branches

Each language is an orphan branch `sdk/<lang>`. Go/PHP/Swift full sources are `sdk/go-src`, `sdk/php-src`, `sdk/swift-src`; `sdk/go` and siblings are pointer READMEs. Do not draw language package roots on the default-branch tree.

Publish coordinates, dedicated-repo steps, and the GitHub Actions auto-release (Conventional Commits, `sdk-<lang>-v*` tags, secret names) live in `docs/sdk-publish.md`. User install docs: [API → SDK](/en/api/sdk/). Do not put maintainer `twine` / `cargo publish` or CI secrets on user SDK pages.

## Only a merged pull request publishes

- Each pipelined `sdk/<lang>` branch publishes **only** when a pull request targeting that branch is merged (`pull_request: types: [closed]` + `merged == true`). Direct pushes, and closed-but-unmerged pull requests, stay silent.
- Merging into `main` never publishes an SDK; the server release is a separate manual `workflow_dispatch`, see [Release and Docker](/en/contribute/release).
- `sdk/c` and `sdk/cpp` have their own pipelines but ship **no registry package**: the release attaches a source archive zip (`kirivers-<lang>-<semver>.zip`) to the GitHub Release.
- The pointer branches `sdk/go`, `sdk/php`, `sdk/swift` have **no** workflows; the `sdk/*-src` pipelines only run tests inside this repository (the plan `skip`s when the repository identity is not the publish remote), and the real tag / publish happens in the copied-out dedicated repo — see `docs/sdk-publish.md`.
- The changelog is written back to each branch's own `CHANGELOG.md` (created if missing); do not hand-edit it.
