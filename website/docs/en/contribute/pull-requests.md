---
title: "Pull requests"
description: "Feature-branch PRs: Conventional Commits title, a linked Issue is mandatory, the version and changelog follow automatically, keep OpenAPI in sync, never hand-edit generated clients."
---

# Pull requests

Branch from the default branch and open a PR against <https://github.com/Kirizu-Official/KiriVers>.

## The PR title is where the version comes from

The squash commit message of a merge into `main` is taken from the PR title, and that title decides whether the merge bumps a version:

| Type | Bump |
|------|------|
| `feat` | minor |
| `fix` / `perf` / `revert` | patch |
| `type!:` or a body `BREAKING CHANGE:` | major |
| `docs` / `refactor` / `test` / `style` / `chore` / `ci` / `build` | no release |

Format: `<type>(<optional scope>): <one-line summary>`, for example `fix(client): 修正空响应`. Allowed types: feat | fix | perf | revert | docs | refactor | test | style | chore | ci | build.

- **Use a lowercase scope** (`perf(db)`, not `perf(DB)`). It is also the per-entry attribution label in the changelog.
- Write a breaking change as `type!:` or add a `BREAKING CHANGE:` footer to the body.
- Both the PR title and every commit must conform (`python3 dev/build/kirivers.py commitlint`). A non-conforming squash title fails the check.

## Every PR must link an Issue

Each Pull Request that lands on `main` has to reference an Issue — either way works:

- use the right-hand sidebar **Development → Link an issue**, or
- write one line in the PR description: `Fixes #<number>` (`Closes` / `Resolves` are equally valid, any casing).

When an Issue genuinely is not needed, add the `skip-issue-check` label to that PR.

Scope of the gate: **only PRs targeting `main`**. Direct pushes to `main` (maintainers) and PRs into the `sdk/<lang>` branches are not gated — releasing on an SDK branch is that branch's own pipeline.

### What the guard bot does

`.github/workflows/pr-guard.yml` re-checks the title format and the Issue link when a PR is opened, edited, pushed to, reopened, or marked ready for review, and keeps **one** comment thread updated in the PR (no comment spam):

- An outside contributor (non-organisation member) who fails a check gets a comment and the PR is **closed automatically**. The branch is not deleted and the PR is not locked: once you have fixed things, click **Reopen pull request** and the guard re-checks and updates that same comment to a pass; you do not have to open a new PR.
- An organisation member (OWNER / MEMBER / COLLABORATOR) who fails gets the comment only — no close.
- **Draft PRs and bot authors are exempt entirely** (something still in progress should not be closed). The `skip-issue-check` label exempts the Issue requirement only; the title check still applies.

The guard reads PR metadata only. It never checks out or executes code that comes in with the PR, and it builds and tests nothing.

For the order — open the Issue first, then the PR bound to it — see [Report a bug](/en/contribute/bugs).

## The changelog is generated — do not hand-write it

`CHANGELOG.md` and the GitHub Release body are both generated from commits by the release scripts (format, plus the customisable emoji and labels: [Release and Docker](/en/contribute/release#changelog-format)). Editing `CHANGELOG.md` by hand gets shoved around by the next release's header insert, and it changes no version number. Sections come from `type`, and the `**scope**` in each entry comes from your scope — get the scope right and entries file themselves.

## Other requirements

- Relevant `go test` / frontend lint pass.
- API changes update that plane's `internal/controller/openapi.*.json` and keep `TestOpenAPIRoutesSync` / `TestPlaneSpecsValid` green.
- Frontend contract changes: run `yarn generate:api` in `frontend/`. Do not hand-edit `src/api/generated/` or `generated-client/`.
- Do not push language SDK sources to `main` — they live on the orphan branches `sdk/<lang>`, see [SDK source branches](/en/contribute/sdk).
- Do not commit secret-bearing `configs/*.yaml` (only `*-example.yaml`).
- Merging into `main` does **not** publish the server: release is a workflow a maintainer runs by hand.

## Local self-check

```bash
python3 dev/build/kirivers.py check-workflows      # pipeline static gate
python3 dev/build/kirivers.py guard --self-check   # guard decision matrix
python3 dev/build/kirivers.py issue-link --self-check
CGO_ENABLED=1 go test ./...
cd frontend && yarn lint && yarn test
```

On Windows replace `python3` with `python`.
