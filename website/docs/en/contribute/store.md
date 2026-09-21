---
title: "Store adapters"
description: "Nine protocols in service/store.DefaultRegistry."
---

# Store adapters

`internal/service/store.DefaultRegistry` registers nine protocols: `sparkle`, `electron`, `tauri`, `squirrel`, `clickonce`, `appimage`, `winget`, `msix`, `fdroid`.

HTTP path `/store/{protocol}/{listing_slug}` (optional `/{doc}`). Missing listing is plaintext 404. Listing identity is the slug. `line_full` uses `store_full`, not hash-root `full`. Feeds must call the same `SelectTarget` as native check (anonymous, no device_id).
