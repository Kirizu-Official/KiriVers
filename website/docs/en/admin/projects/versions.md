---
title: "Versions"
description: "draft → ready → publish. List latest=true semantics. TUS. No bare PUT to unknown S3 keys."
---

# Versions

Nav **Versions**. Lifecycle: **Draft** → **Mark ready** → **Publish** / **Deprecate** / **Revoke**. Lines: **Yank**, **Disable**. **Promote channel** moves to a more stable channel.

## Create and publish

1. Click **Create version**. Fill **Build** and/or **SemVer**, **Channel**. `stable` cannot use a prerelease suffix (<ErrorCode code="CHANNEL_SUFFIX_MISMATCH" />). Duplicate identity: <ErrorCode code="VERSION_ALREADY_EXISTS" />. Engine mismatch: <ErrorCode code="ENGINE_MISMATCH" />.
2. Optional **LTS**, **Critical (mandatory for everyone)**, **Minimum source version**. Critical versions cannot use gray (<ErrorCode code="GRAY_NOT_ALLOWED_ON_CRITICAL" />).
3. Open **Manage artifacts**. Click **Upload artifact**. Channel **Direct PUT** or **TUS chunked (resumable)**. `direct_s3` is always **false**: never PUT unknown S3 keys. Hash mismatch: <ErrorCode code="CHECKSUM_MISMATCH" />. Unregistered hw: <ErrorCode code="HW_REV_UNKNOWN" />. Published artifacts cannot be overwritten (<ErrorCode code="ARTIFACT_IMMUTABLE" />; **Yank** first).
4. **Mark ready** on that platform line. Incomplete upload → <ErrorCode code="UPLOAD_INCOMPLETE" />.
5. **Publish**. No ready line → <ErrorCode code="ARTIFACT_REQUIRED" />. Incomplete `auto_publish_when` → <ErrorCode code="AUTO_PUBLISH_PENDING" />. Copy: “Changelog is immutable after publish”.
6. **Generate delta**, **Reuse existing artifact**, and multi-file archives are async jobs (header **Task Center**). Failure: <ErrorCode code="JOB_FAILED" />.

## List `latest=true`

Admin `GET versions?latest=true&os=&arch=` returns `{ versions, latest }`. `latest` is published + ready via `CompareVersions`, **not** client `SelectTarget`. Without `latest` the response omits that key.

Concepts: [Releases](/en/guide/features/versions). CI: [CI releases](/en/api/ci).
