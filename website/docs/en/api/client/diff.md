---
title: "Single-file diff"
description: "POST /update/diff. local_sha256 only here. Does not enqueue pack."
---

# Single-file diff

`POST /api/v1/projects/{project_ref}/update/diff`

`local_sha256` appears **only** here, never on check. Lookup-only: does not enqueue pack. Unknown magic: client falls back to full package. Admin generation is a Job. `Cache-Control: private, no-store`, no ETag.

Schema: [API reference](/en/api/reference/) (`openapi.client.json`; <a href="/en/api/scalar/client" target="_blank" rel="noopener">open Scalar</a>). Concepts: [Incremental updates](/en/guide/features/incremental).
