---
title: "Downloads"
description: "/packages/{ref} is content SHA-256. No /artifacts/{id}/{filename}. Range and exp/sig."
---

# Downloads

`GET|HEAD /api/v1/projects/{project_ref}/packages/{ref}`

`ref` is content SHA-256 (optional `.{ext}` / `.blockmap`). Lookup is hash-only. **No** `/artifacts/{id}/{filename}`. Keep `exp`/`sig`. `Range` is supported. With `cluster.download=s3`, check `package_url` may be a public object URL.

Schema: [API reference](/en/api/reference/) (`openapi.client.json`; <a href="/en/api/scalar/client" target="_blank" rel="noopener">open Scalar</a>).
