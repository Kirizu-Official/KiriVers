---
title: "Gray rollout"
description: "Allowlist plus coverage knobs. Incomplete gray hides the version from non-members. Critical versions cannot use gray."
---

# Gray rollout

From a version, open **Gray**. Mechanism: device allowlist + **Coverage knobs**, **not** HMAC / `rollout_percent`. Critical copy: “Critical versions cannot use incremental gray.” → <ErrorCode code="GRAY_NOT_ALLOWED_ON_CRITICAL" />.

1. Open **Gray** on a published version.
2. Set **Start %**, **Step %**, **Interval (seconds)**. Changing knobs does **not** reset start time.
3. **Add clients**, tick roster rows, **Add selected**. Table **Allowlisted clients** shows source **Auto** or **Manual**.
4. **Complete now (full push)** sets `gray_completed_at` so anonymous check and store feeds see the version.

| State | Not on the allowlist (including anonymous) |
|-------|--------------------------------------------|
| Incomplete | Still sees the previous version; check/feed `Cache-Control: private` |
| Full push | Visible |

An empty roster completes immediately. `device_id_policy=none` cannot admit by device. Concepts: [Gray rollout](/en/guide/features/gray).
