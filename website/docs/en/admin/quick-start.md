---
title: "Quick start"
description: "After sign-in: create a project, add a matrix row, create a version, upload, mark ready, publish."
---

# Quick start

Prerequisites: KiriVers is installed and you can sign in after [bootstrap the first admin](/en/guide/config/bootstrap-admin). This page starts **after a successful sign-in**. Skip other buttons. Success is version status **Published**.

Console map: the global drawer shows **Admin Users / GeoIP / Nodes** only to platform admins; everyone sees **Projects**. Account security is the user-menu item **Account security** (`/security`), not a drawer row. Jobs use the header bell (**Task Center**). There is **no** standalone Jobs route.

1. **Projects** → **New project**. Fill **Name**, **Slug**, **Compare engine** (**SemVer** or **Integer build**).
2. **Open project** → **Platform Matrix**. Click **Register platform**: **OS**, **Arch**, **Package type** (**Single file** or **Multi file**).
3. **Versions** → **Create version**. Fill **Build** or **SemVer**; channel `stable` for the first release.
4. Open **Manage artifacts**, click **Upload artifact**, and choose a local file. `direct_s3` is always false; the console never PUT-s unknown S3 keys.
5. **Mark ready** on that platform line, then **Publish**.
6. The list shows **Published**. Next: [Update check](/en/guide/features/check).

Publish refusals:

- <ErrorCode code="ARTIFACT_REQUIRED" />
- <ErrorCode code="AUTO_PUBLISH_PENDING" />
- <ErrorCode code="UPLOAD_INCOMPLETE" />

Concepts: [Releases](/en/guide/features/versions). Click path: [Versions](/en/admin/projects/versions).
