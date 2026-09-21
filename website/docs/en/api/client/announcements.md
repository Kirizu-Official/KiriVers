---
title: "Announcements"
description: "GET announcements. Empty list is 200. Explicit locale is strict."
---

# Announcements

`GET /api/v1/projects/{project_ref}/announcements`

Separate from check. Empty match is **200** `{ "announcements": [] }`, not 204. Explicit `locale` is strict; omit locale for leftover. Honor ETag / 304. Schema: [API reference](/en/api/reference/) (`openapi.client.json`; <a href="/en/api/scalar/client" target="_blank" rel="noopener">open Scalar</a>). Scopes: [Announcements](/en/guide/features/announcements).
