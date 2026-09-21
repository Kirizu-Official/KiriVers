---
title: "Contribute"
description: "For default-branch source changes. Self-hosters: use the Guide. Issues then PRs. yarn dev for UI."
---

# Contribute

This section is for people who change **default-branch source**. Operators should use the [Guide](/en/guide/). Repository: <https://github.com/Kirizu-Official/KiriVers> (submit Issues/PRs after it is public).

| Entry | Page |
|-------|------|
| Defects | [Report a bug](./bugs) |
| Patches | [Pull requests](./pull-requests) |
| Tree and process | [Repository layout](./repo-layout), [Architecture](./architecture) |
| Domain whitepaper | [Technical whitepaper](./whitepaper) (object graph, unique owners, leftover 404s before default-branch work) |
| Local loop | [Local development](./dev-setup), [Release and Docker](./release) |

Order is **issue → PR**: open the issue first, then a pull request that links it (`Fixes #<number>` or Development → Link an issue); when an issue genuinely is not needed, add the `skip-issue-check` label. The gate applies only to pull requests into `main`. Merging into `main` does **not** publish the server — releasing is a manual workflow run by a maintainer.

Debug the admin UI with `cd frontend && yarn dev` (`http://localhost:3000`). Use `yarn build` when embedding the production console.
