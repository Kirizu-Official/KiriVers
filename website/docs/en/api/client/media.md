---
title: "Media"
description: "GET|HEAD .../media/{id}. Public UUID. No token, no URL sign."
---

# Media

`GET|HEAD /api/v1/projects/{project_ref}/media/{id}`

`id` is a UUID. Public GET: no project token, no `exp`/`sig`. Announcement Markdown images use this path. Schema: [API reference](/en/api/reference/) (`openapi.client.json`; <a href="/en/api/scalar/client" target="_blank" rel="noopener">open Scalar</a>).
