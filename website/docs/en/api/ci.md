---
title: "CI releases"
description: "CI tokens call ci/releases. Job progress is admin jobs."
---

# CI releases

Use a project **CI token** against the admin plane (default `:8081`):

| Method | Path |
|--------|------|
| GET, POST | `/api/v1/admin/projects/{project_ref}/ci-tokens` |
| DELETE | `/api/v1/admin/projects/{project_ref}/ci-tokens/{token_id}` |
| POST | `/api/v1/admin/projects/{project_ref}/ci/releases` |

GitHub Actions (or any CI/CD platform): store the token as a secret, then `POST ci/releases` after the build. Job progress is **not** on this page: `GET /api/v1/admin/jobs/{job_id}` (admin reference).

::: info Not how KiriVers itself is released
A CI token here belongs to **your project's** pipeline, uploading release packages to KiriVers. The build and release pipeline for KiriVers itself is documented in [Contribute → Release and Docker](/en/contribute/release).
:::

Explorer: [API reference](/en/api/reference/) (`openapi.admin.json`; <a href="/en/api/scalar/admin" target="_blank" rel="noopener">open Scalar</a>).
