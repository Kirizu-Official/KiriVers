---
title: "Changelog"
description: "GET changelog/{channel}/{os}/{arch}. Check has no body text. Token mismatch is 404."
---

# Changelog

`GET /api/v1/projects/{project_ref}/changelog/{channel}/{os}/{arch}`

Check responses have **no** changelog text. Unknown channel → 400 `INVALID_QUERY_PARAM`. Channel-token mismatch → 404 `NOT_FOUND` (not 403). Query: `from_version`, `changelog_scope` (`range_all` / `range_platform` / `target_only`), `changelog_layout`. No client `limit`. Overflow truncates at HTTP 200.

Schema: [API reference](/en/api/reference/) (`openapi.client.json`; <a href="/en/api/scalar/client" target="_blank" rel="noopener">open Scalar</a>). Concepts: [Changelog](/en/guide/features/changelog).
