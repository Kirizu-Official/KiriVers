---
title: "Multi-file pack"
description: "Poll the same URL. Oversize is 200 full_package. No admin job_id."
---

# Multi-file pack

`POST /api/v1/projects/{project_ref}/update/pack`

Enqueue and poll the **same** URL. There is no `pack/status`. Do not poll admin `GET /jobs/{id}`.

| Status | HTTP | Meaning |
|--------|------|---------|
| `pending` | 202 | Queued or coalesced |
| `ready` | 200 | Patch zip is available |
| `full_package` | **200** | Oversize, missing paths, or failed job → full archive |

Over `dynamic_pack.max_bytes` or a large uncompressed Manifest fraction → 200 `full_package`, not 400.

Schema: [API reference](/en/api/reference/) (`openapi.client.json`; <a href="/en/api/scalar/client" target="_blank" rel="noopener">open Scalar</a>).
