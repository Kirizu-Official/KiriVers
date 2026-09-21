---
title: "Jobs"
description: "Header bell plus batch-release session tracking. GET /api/v1/admin/jobs/{job_id}. Client pack does not return job_id."
---

# Jobs

UI: header bell (**Task Center**) plus in-session tracking on **Batch Release**. There is **no** standalone Jobs menu or `/jobs` route.

1. Upload, delta, reuse, archive, and cleanup enqueue jobs.
2. Open the bell **Task Center** and read **Queued / Running / Succeeded / Failed**.
3. HTTP: `GET /api/v1/admin/jobs/{job_id}`. Missing: <ErrorCode code="JOB_NOT_FOUND" />. Failed: <ErrorCode code="JOB_FAILED" />.

Client `POST /update/pack` does **not** return an admin `job_id`. Poll the same pack URL until `ready` or `full_package`. See [Incremental updates](/en/guide/features/incremental) and [Batch release](/en/admin/projects/release).
