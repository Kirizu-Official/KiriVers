---
title: "Error codes"
description: "Union of client and admin OpenAPI Error.code plus console locales, excluding frontend NETWORK/HTTP/UNKNOWN."
---

# Error codes

Branch on `error.code`. A wrong channel token is **not** 403. A missing store listing has **no** JSON `code`. Hover copy matches the docs-site `ErrorCode` component.

| code | HTTP (when stable) | Meaning |
|------|--------------------|---------|
| `PROJECT_NOT_FOUND` | 404 | Project identity missed or alias expired |
| `VERSION_NOT_FOUND` | 404 | Version missing |
| `VERSION_LINE_NOT_FOUND` | 404 | No line for that os/arch |
| `VERSION_NOT_VISIBLE` | 404/409 | Version not visible to the client (draft, packs not ready, or gray miss) |
| `VERSION_REVOKED` | 409 | Revoked with no safe target |
| `NO_SAFE_TARGET` | 409 | No safe target |
| `INTERMEDIATE_UNAVAILABLE` | 409 | Min-source hop missing, not ready, cyclic, or more than 8 hops |
| `MIN_OS_NOT_MET` | 409 | OS/API below the current line floor |
| `CHANNEL_CONFLICT` | 409 | Idempotent create channel mismatch |
| `UNAUTHORIZED` | 401 | Missing/invalid credentials |
| `FORBIDDEN` | 403 | Insufficient permission (including project-only access to /admins, /geoip, /nodes) |
| `PRECONDITION_FAILED` | 412 | If-Match / ETag; not POST check |
| `HW_REV_INCOMPATIBLE` | 409 | hw_rev out of range |
| `RATE_LIMITED` | 429 | Too many requests; honor Retry-After |
| `CHANGELOG_QUERY_INVALID` | 400 | Illegal changelog query |
| `NOT_FOUND` | 404 | Generic missing resource |
| `NOT_READY` | 503/400 | Service or Passkey not ready. Either empty `webauthn_rp_id` or `webauthn_origins` makes Passkey routes return this code (HTTP 503) |
| `INTERNAL_ERROR` | 500 | Internal error |
| `INVALID_REQUEST` | 400 | Illegal body/fields (including non-.mmdb, caps, unknown matrix pair, duplicate listing) |
| `INVALID_QUERY_PARAM` | 400 | Illegal query (including announcement version and unknown changelog channel) |
| `ENGINE_MISMATCH` | 400 | Engine vs identity mismatch |
| `ARTIFACT_REQUIRED` | 400 | Publish rejected: no ready version line |
| `CHANNEL_SUFFIX_MISMATCH` | 400 | Prerelease suffix vs channel; stable cannot use a prerelease suffix |
| `VERSION_ALREADY_EXISTS` | 409 | Duplicate version in project |
| `GRAY_NOT_ALLOWED_ON_CRITICAL` | 400 | Gray on critical version |
| `PACKAGE_TYPE_IMMUTABLE` | 409 | package_type locked after a published line exists |
| `COMPARE_ENGINE_IMMUTABLE` | 409 | compare_engine locked after first publish |
| `INVALID_PATH` | 400 | Illegal path |
| `ARTIFACT_IMMUTABLE` | 409 | Published artifact immutable (yank the line first) |
| `HW_REV_UNKNOWN` | 400 | Unregistered hw_rev |
| `DELTA_ALGO_UNSUPPORTED` | 400 | Unknown delta algo |
| `DELTA_SAME_VERSION` | 400 | Delta source equals target |
| `TOTP_RATE_LIMITED` | 429 | TOTP attempts exhausted in one 30s period |
| `UPLOAD_INCOMPLETE` | 400 | Upload incomplete for this operation |
| `CHANNEL_NOT_FOUND` | 404 | Channel missing |
| `SYSTEM_CHANNEL` | 409 | System channels alpha/beta/stable cannot be deleted |
| `CHECKSUM_MISMATCH` | 400 | Declared SHA-256 mismatch |
| `JOB_NOT_FOUND` | 404 | Job missing |
| `JOB_FAILED` | 409/500 | Async job failed |
| `AUTO_PUBLISH_PENDING` | 409 | auto_publish_when is not fully ready yet |
| `ZIP_LAYOUT_INVALID` | 400 | Zip layout is not os/arch/… (missing segments, unknown ids or traversal) |
| `ADMIN_NOT_FOUND` | 404 | Admin missing |
| `USERNAME_TAKEN` | 409 | Username taken |
| `LAST_ADMIN` | 409 | Cannot delete last admin |
| `LAST_OWNER` | 400 | Cannot remove last owner |
| `MEMBER_NOT_FOUND` | 404 | Member missing |
| `CLIENT_NOT_FOUND` | 404 | Roster client missing |
| `GEOIP_NOT_FOUND` | 404 | GeoIP database missing |
| `LANGUAGE_TAKEN` | 409 | Language code taken |
| `LANGUAGE_NOT_FOUND` | 404 | Language missing |

`DELTA_SAME_VERSION` comes from console locales and is not in the admin OpenAPI `Error.code` enum; treat it as “source equals target”. Excluded frontend-only `NETWORK_ERROR` / `HTTP_ERROR` / `UNKNOWN_ERROR`. Missing store listings have **no** JSON `code`.
