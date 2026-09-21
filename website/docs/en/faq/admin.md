---
title: "Admin console"
description: "Sign-in, 2FA, Passkey NOT_READY, LAST_OWNER, upload, COMPARE_ENGINE_IMMUTABLE."
---

# Admin console

| Symptom | Meaning |
|---------|---------|
| Password works but 2FA never finishes | Pending TTL expired; sign in again |
| Cannot add a passkey | Empty `webauthn_rp_id` **or** empty `webauthn_origins` → `NOT_READY`; use TOTP |
| Cannot remove a member | `LAST_OWNER` / `LAST_ADMIN` |
| Publish fails | `ARTIFACT_REQUIRED`, `UPLOAD_INCOMPLETE`, `AUTO_PUBLISH_PENDING` |
| Compare engine 409 | `COMPARE_ENGINE_IMMUTABLE` |
| Naked S3 upload fails | `direct_s3` is always false; use the console/TUS |

Watch Jobs on the header bell. There is no standalone jobs route.
