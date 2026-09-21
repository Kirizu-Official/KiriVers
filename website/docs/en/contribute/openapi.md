---
title: "OpenAPI contracts"
description: "openapi.client.json and openapi.admin.json are sources. The docs site copies both plane files unfiltered."
---

# OpenAPI contracts

Sources: `internal/controller/openapi.client.json` and `openapi.admin.json`. Tests: `TestOpenAPIRoutesSync`, `TestPlaneSpecsValid`, `TestOpenAPIForbiddenNames` / `TestOpenAPIFieldTables`. Handler JSON changes must update the owning plane file.

`yarn docs:build` **copies** those files as `openapi.client.json` (client + store) and `openapi.admin.json` (admin + CI). It no longer slices by `/store/` or CI paths. User downloads and Scalar new windows: [API reference](/en/api/reference/). Do not hand-copy field tables.
