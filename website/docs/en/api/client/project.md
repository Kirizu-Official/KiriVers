---
title: "Public project"
description: "GET /api/v1/projects/{project_ref}. Expired aliases are 404 PROJECT_NOT_FOUND."
---

# Public project

```bash
curl -sS http://127.0.0.1:8080/api/v1/projects/my-app
```

`project_ref`: UUID, live slug, or unexpired alias. Expired alias → 404 `PROJECT_NOT_FOUND`. When required, send `X-Project-Token` or `Authorization: Bearer`. Schema: [API reference](/en/api/reference/) (`openapi.client.json`; <a href="/en/api/scalar/client" target="_blank" rel="noopener">open Scalar</a>).
