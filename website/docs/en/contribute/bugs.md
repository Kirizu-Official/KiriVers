---
title: "Report a bug"
description: "GitHub Issues. Repro steps, expected vs actual, version, plane, redacted logs. Open the issue first, then a PR that links it."
---

# Report a bug

Open <https://github.com/Kirizu-Official/KiriVers/issues/new>. The issue forms live in `.github/ISSUE_TEMPLATE/`.

Include:

1. Repro steps
2. Expected vs actual
3. Binary version or commit (the `sha6` tail of the `KiriVers-<OS>-<Arch>-<sha6>.zip` asset identifies the exact release; for a local build, give the commit)
4. OS
5. Client plane (`:8080`) vs admin plane (`:8081`)
6. Relevant logs (redact secrets, tokens, DSN passwords)

Do not file security issues publicly. After the repository is public, see SECURITY; until then contact maintainers privately.

## Order: issue first, then the PR

An issue starts the flow, and the pull request that fixes it must point back at it:

1. Open the issue with the repro information above.
2. Once it is worth changing, branch off the default branch and open a PR linked to that issue: **Development → Link an issue** in the sidebar, or `Fixes #<number>` in the PR description (`Closes` / `Resolves` work too).
3. When an issue genuinely is not needed, add the `skip-issue-check` label to the PR.

The gate applies only to pull requests **into `main`**; the guard bot's rules and exemptions are on [Pull requests](/en/contribute/pull-requests).
