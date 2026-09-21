---
title: "Store feeds"
description: "/store/{protocol}/{listing_slug}. Missing listing is plaintext 404. Nine protocols."
---

# Store feeds

`GET|POST /api/v1/projects/{project_ref}/store/{protocol}/{listing_slug}`  
optional `/{doc}`.

Protocols: `sparkle`, `electron`, `tauri`, `squirrel`, `clickonce`, `appimage`, `winget`, `msix`, `fdroid`. Missing listing, unknown protocol, or a slug-less legacy path → **plaintext 404** (no JSON `code`). Official SDKs do **not** read feeds.

Contract: [API reference](/en/api/reference/) (`openapi.client.json`; <a href="/en/api/scalar/client" target="_blank" rel="noopener">open Scalar</a>). Listing CRUD stays on the admin reference. Scenarios: [Electron](/en/guide/scenarios/electron), [Sparkle](/en/guide/scenarios/sparkle).
