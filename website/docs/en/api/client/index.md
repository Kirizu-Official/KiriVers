---
title: "Client HTTP"
description: "Native JSON order and capabilities. Full contract is the API reference. Plane spec: GET /api/v1/openapi.json."
---

# Client HTTP

Default base `http://127.0.0.1:8080`. Paths under `/api/v1/...`. Plane contract: `GET /api/v1/openapi.json`. Store feeds are not in this group: [Store](./store) and [API reference](/en/api/reference/) (`openapi.client.json`; <a href="/en/api/scalar/client" target="_blank" rel="noopener">open Scalar</a>).

Suggested order: health → optional report → **POST** check → optional changelog / announcements → download `/packages/{sha256}` → single-file diff or multi-file pack. Default `capabilities` is `full_package` only.

Errors: `error.code`. [Conventions](/en/api/conventions). Schema: [API reference](/en/api/reference/) (`openapi.client.json`; <a href="/en/api/scalar/client" target="_blank" rel="noopener">open Scalar</a>).
