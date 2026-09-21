---
title: "API reference"
description: "Two unfiltered plane contracts: openapi.client.json (client and store) and openapi.admin.json (admin and CI). Download them or open Scalar in a new window."
---

# API reference

KiriVers exposes two HTTP planes. The docs build copies the same contract files the running process embeds. It does **not** slice them into four filtered specs.

| Plane | Contract file | Covers |
|-------|---------------|--------|
| Client | `openapi.client.json` | Native JSON (check, download, delta, announcements, …) **and** store `/store/` feeds |
| Admin | `openapi.admin.json` | Console automation **and** CI tokens / `ci/releases` |

Mapping:

- **Client / store**: `openapi.client.json`
- **Admin / CI**: `openapi.admin.json`

Each plane’s `GET /api/v1/openapi.json` and the downloads below come from the same source. Tutorials do not hand-copy field tables.

## Download

- <a href="/openapi/openapi.client.json" download="openapi.client.json"><code>openapi.client.json</code></a>
- <a href="/openapi/openapi.admin.json" download="openapi.admin.json"><code>openapi.admin.json</code></a>

## Open Scalar in a new window

Scalar is a full-viewport explorer. It is **not** embedded in a docs page with a sidebar. Open it in a new window:

- <a href="/en/api/scalar/client" target="_blank" rel="noopener">Client / store Scalar</a>
- <a href="/en/api/scalar/admin" target="_blank" rel="noopener">Admin / CI Scalar</a>

Related tutorials: [Client HTTP](/en/api/client/), [Admin automation](/en/api/admin/), [CI releases](/en/api/ci), [Conventions](/en/api/conventions). Error codes: [FAQ](/en/faq/errors).
