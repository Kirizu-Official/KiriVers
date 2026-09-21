---
title: "Integrity"
description: "GET .../versions/{version}/integrity. All files in one response. No cursor."
---

# Integrity

`GET /api/v1/projects/{project_ref}/versions/{version}/integrity`

All files in one response; no `cursor` / `limit`. `include_file_urls` is honored only when total files ≤ 16. Multi-file clients compare locally then `POST /update/pack`. ETag is the line `RootHash`.

Schema: [API reference](/en/api/reference/) (`openapi.client.json`; <a href="/en/api/scalar/client" target="_blank" rel="noopener">open Scalar</a>).
