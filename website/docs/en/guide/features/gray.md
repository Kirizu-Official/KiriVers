---
title: "Gray rollout"
description: "Allowlist plus start/step/interval knobs and gray_completed_at. Not HMAC or rollout_percent."
---

# Gray rollout

Gray is a **device allowlist** plus coverage knobs (`gray_start_percent` / `gray_step_percent` / `gray_interval_seconds`) and `gray_completed_at`. It is **not** HMAC and not `rollout_percent`.

## When to use

Use gray to show a version to some roster devices before anonymous clients and store feeds. Skip incremental gray for an empty roster or a critical version.

## Configuration entry

Version → **Gray**. Steps: [Gray](/en/admin/projects/gray). Project **Settings → Update Policy** controls admission weight. No YAML percent key.

## Rules and error codes

| State | Not allowlisted (including anonymous) | Allowlisted |
|-------|----------------------------------------|-------------|
| Incomplete | Still sees the previous version | Sees this version |
| `gray_completed_at` set | Visible to anonymous store feeds | Visible |

Incomplete gray forces check/feed `Cache-Control: private`. Starting gray on an empty roster **completes immediately**. `device_id_policy=none` cannot admit by device. Knob PATCH does **not** reset `gray_started_at`. Critical: <ErrorCode code="GRAY_NOT_ALLOWED_ON_CRITICAL" />.

## Client duties

Send a stable `device_id` on check when policy allows. Anonymous clients miss until complete. Store feeds use the same `GrayIsComplete()` gate.

Related: [Update check](./check), [Telemetry](./telemetry), [CDN](./cdn).
