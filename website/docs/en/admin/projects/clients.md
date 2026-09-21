---
title: "Clients"
description: "Roster and stats. Deleting a roster row is not a privacy wipe."
---

# Clients

Nav **Clients**. This is the operational roster (check / report), used for gray admission and overview stats.

1. Open **Clients**. Read KPIs: **Total devices**, **Active 24h**, **Active 7d**.
2. Columns include **Device hash**, **Last IP**, **Last check**, **Country**. **Full JSON** shows `custom`.
3. **Delete client** removes the roster row and that device from this project’s gray allowlist. It does **not** wipe privacy hashes.

Privacy erase is Settings → **Privacy** → **Delete device by hash**, returning `telemetry_deleted` / `allowlist_deleted`. With `device_id_policy=none`, check does not insert a roster row, so gray cannot admit by device. Missing row: 404 <ErrorCode code="CLIENT_NOT_FOUND" />. Concepts: [Telemetry and devices](/en/guide/features/telemetry).
