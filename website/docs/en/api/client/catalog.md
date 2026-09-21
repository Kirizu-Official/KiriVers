---
title: "Catalogs"
description: "GET channels / matrix / languages. Hidden channels are omitted. Empty lists are 200."
---

# Catalogs

| Path | Envelope |
|------|----------|
| `GET .../channels` | `{ "channels": [] }` public only; omits unlisted / TokenProtected |
| `GET .../matrix` | `{ "matrix": [] }` os / arch / package_type only |
| `GET .../languages` | `{ "languages": [] }` |

Empty lists are still **200**. There is no client `GET /channels/:slug`. Hidden-channel upgrades use check `channel=` plus `X-Channel-Token`.
