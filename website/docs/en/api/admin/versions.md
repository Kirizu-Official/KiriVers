---
title: "Admin · Versions"
description: "draft/ready/publish. latest=true is not SelectTarget. direct_s3 is always false."
---

# Admin · Versions

Create version, upload (TUS), mark ready, publish / revoke / promote. `GET versions?latest=true` uses `CompareVersions`, not client `SelectTarget`. `direct_s3` is always false. Gray, delta jobs: Gray / Artifacts / Jobs on the reference.

CI batch release uses `ci/releases` on [CI](/en/api/ci). Paths: [API reference](/en/api/reference/) (`openapi.admin.json`; <a href="/en/api/scalar/admin" target="_blank" rel="noopener">open Scalar</a>).
