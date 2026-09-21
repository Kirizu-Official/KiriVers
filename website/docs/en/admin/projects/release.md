---
title: "Batch release"
description: "Unpack a zip onto an existing version. Track the job in-session. direct_s3 is always false."
---

# Batch release

Nav **Batch Release**. Description: “Upload a zip bundle onto an existing version, optionally publish after unpack. Track progress in the task center.”

1. Open **Batch Release**. Choose **Single file** or **Layout zip**.
2. **Layout zip**: pick **Bundle zip**, fill **Version** (must already exist), optionally **Publish immediately after unpacking**, optional **auto_publish_when (publish when all ready)** (one `os/arch` per line).
3. **Single file**: **Publish immediately after upload** creates a missing version and that OS/Arch line, then uploads.
4. Click **Submit release job**. `direct_s3` is always false. Bad layout → <ErrorCode code="ZIP_LAYOUT_INVALID" />. Incomplete upload → <ErrorCode code="UPLOAD_INCOMPLETE" />. Incomplete auto_publish → <ErrorCode code="AUTO_PUBLISH_PENDING" />.
5. Track the job on this page and in header **Task Center**. `GET /api/v1/admin/jobs/{job_id}`. Missing: <ErrorCode code="JOB_NOT_FOUND" />. Failed: <ErrorCode code="JOB_FAILED" />.

Concepts: [Releases](/en/guide/features/versions).
