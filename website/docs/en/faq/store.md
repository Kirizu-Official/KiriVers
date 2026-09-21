---
title: "Store listings"
description: "Listing slug, plaintext 404, ETag, store_full vs hash-root."
---

# Store listings

Missing listing, unknown protocol, or a slug-less legacy path → **plaintext 404** (no JSON `code`). The URL must include `{listing_slug}`.

Incomplete gray is omitted from anonymous feeds. `line_full` uses `store_full`, not hash-root `full`. Honor ETag. Official SDKs do not read feeds.
