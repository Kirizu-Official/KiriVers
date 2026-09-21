---
title: "Desktop apps"
description: "Single-file optional binary delta; directories use pack. Store updaters have their own pages."
---

# Desktop apps

Homegrown desktop clients use native JSON: **POST** `/api/v1/projects/{project_ref}/update/check`. Store updaters (electron-updater, Sparkle, Tauri updater) use listing URLs on their own pages.

## Package shape

| Shape | Matrix `package_type` | Incremental |
|-------|----------------------|-------------|
| Single exe / dmg / AppImage | Single file | May declare `binary_delta` and `POST /update/diff`. Algorithms: [Incremental](/en/guide/features/incremental) |
| Multi-file directory | Multi-file | `GET` integrity, then `POST /update/pack`. No per-dll binary delta. Oversized packs return 200 `full_package` |

Console: [Platform matrix](/en/admin/projects/matrix), [Release](/en/admin/projects/release).

## Client duties

- Method must be **POST**. GET check is not this product’s query.
- Verify SHA-256 after downloading `package_url`. Unknown delta magic falls back to the full package.
- Optional `POST .../telemetry/report` (202); failures must not block install.

Related: [Update check](/en/guide/features/check), [Electron](./electron), [Sparkle](./sparkle), [Tauri](./tauri).
