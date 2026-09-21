---
title: "Create a project"
description: "Display name is not project_ref. slug, compare_engine, default locale. Engine locks after first publish."
---

# Create a project

Who: accounts that see **Projects**. The create button is platform-admin only (project accounts get 403 <ErrorCode code="FORBIDDEN" />).

1. **Projects** → **New project**.
2. Fill the table, click **Create**.
3. Click **Open project** to reach Overview.

| Field | UI | Meaning | Failure |
|-------|----|---------|---------|
| `name` | **Name** | Console title, **not** `project_ref` | Too long → 400 <ErrorCode code="INVALID_REQUEST" /> |
| Slug | **Slug** | Public URL identity; old slug stays an alias until expiry | Duplicate live slug rejected |
| Compare engine | **Compare engine** | **SemVer** or **Integer build**. **Locked after first publish** | <ErrorCode code="COMPARE_ENGINE_IMMUTABLE" /> |
| Default locale | **Default locale** | Fallback when client copy is missing | Illegal code → 400 |
| Owner username | **Project owner username** | Optional. Binds an existing account | — |
| Owner password | **Owner password** | Only when creating a new account; min 8 chars | — |

Client `project_ref` may be UUID, live slug, or unexpired alias. Expired alias → 404 <ErrorCode code="PROJECT_NOT_FOUND" />. Concepts: [Releases](/en/guide/features/versions).
