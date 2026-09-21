---
title: "Releases"
description: "Version vs Version Line. draft → ready → publish. Critical versions cannot use gray. Compare engine locks after publish."
---

# Releases

A **Version** is a version identity on a channel. A **Version Line** is that version’s artifact line for one `(os,arch)` (and hw).

## When to use

Every build you want clients to install needs a Version plus ready Version Lines on matrix platforms. Store feeds and native check share those published, ready lines.

## Configuration entry

- Console: **Versions**, **Manage artifacts**, **Batch Release**. Click-path: [Versions](/en/admin/projects/versions).
- YAML: no dedicated release file. Compare engine is chosen at project create; see [Create a project](/en/admin/projects/create).

## Rules and error codes

Lifecycle: **Draft** → **Mark ready** → **Publish**. Also **Deprecate** / **Revoke**; lines **Yank** / **Disable**. Less-stable channels can **Promote channel**.

Publish refusals:

- <ErrorCode code="ARTIFACT_REQUIRED" />
- <ErrorCode code="AUTO_PUBLISH_PENDING" />
- <ErrorCode code="UPLOAD_INCOMPLETE" />

Compare engine is immutable after first publish (<ErrorCode code="COMPARE_ENGINE_IMMUTABLE" />). Critical versions cannot use gray (<ErrorCode code="GRAY_NOT_ALLOWED_ON_CRITICAL" />). Published artifacts cannot be overwritten (<ErrorCode code="ARTIFACT_IMMUTABLE" />).

CI uses `ci/releases` ([CI releases](/en/api/ci)); the browser uses artifacts / TUS. `direct_s3` is always false. Artifacts can be reused.

## Client duties

Clients only see **Published** versions whose line is ready (`packs_ready_at` stamped). Drafts and un-preheated lines are invisible (<ErrorCode code="VERSION_NOT_VISIBLE" />).

Related: [Gray rollout](./gray), [Incremental updates](./incremental), [Channels](./channels).
