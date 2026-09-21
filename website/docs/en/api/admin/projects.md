---
title: "Admin · Projects"
description: "Project CRUD, stats, settings fields. Schema lives on the API reference."
---

# Admin · Projects

Tutorial: list, create, PATCH settings. Display `name` is not `project_ref`. List/detail `stats` include `storage_bytes` and omit device hashes. `storage_visibility=private` is rejected (400 `INVALID_REQUEST`). Store listings use `{ listings: [] }`; duplicate `(protocol, slug)` is 400.

Install policy, channels, matrix, languages, and members are dedicated resources — do not fold them into project PATCH. Paths: [API reference](/en/api/reference/) (`openapi.admin.json`; <a href="/en/api/scalar/admin" target="_blank" rel="noopener">open Scalar</a>).
