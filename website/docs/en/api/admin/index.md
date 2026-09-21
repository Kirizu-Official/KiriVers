---
title: "Admin automation"
description: "Admin plane mapped by OpenAPI tags. Schemas live in API reference."
---

# Admin automation

Admin plane default `:8081`. Auth: session cookie or Bearer. Schemas: [API reference](/en/api/reference/) (`openapi.admin.json`; <a href="/en/api/scalar/admin" target="_blank" rel="noopener">open Scalar</a>) and [CI](/en/api/reference/).

OpenAPI tags (complete):

| Tag | Role |
|-----|------|
| Auth | Login, logout, TOTP/Passkey/recovery |
| Admins | Instance admins |
| Projects | Project CRUD, tokens, stats |
| Versions | Versions and lines, publish/revoke/promote |
| Line Defaults | Line defaults |
| Artifacts | Upload, TUS, presign (`direct_s3` false), delta jobs, reuse, archives |
| Manifest | Multi-file manifest |
| Gray | Allowlist and knobs |
| Jobs | `GET /jobs/{job_id}` |
| GeoIP | Private databases |
| Nodes | Nodes and node-sync |
| Media | Admin image upload |
| Audit | Audit |
| Webhooks | Deliveries |
| Store Listings | Listing CRUD |
| InstallPolicy | Install-policy rules |
| Telemetry | Privacy delete |
| Channels | Channels |
| Platforms | Matrix, hw-revs, catalog |
| Languages | Project languages |
| Members | Members |
| Clients | Roster |
| Announcements | Announcement CRUD |
| System | `GET /api/v1/health`, `openapi.json` |

CI tokens and `ci/releases`: [CI](/en/api/ci) and [API reference](/en/api/reference/) (`openapi.admin.json`; <a href="/en/api/scalar/admin" target="_blank" rel="noopener">open Scalar</a>).
