---
title: "admin.yaml"
description: "Default :8081. Disk static_dir wins over embed; empty string disables the UI. enabled=false skips the admin listener only."
---

# admin.yaml

File: `admin.yaml`. Prefix `KIRIVERS_ADMIN_`.

| Key | Type | Default | Meaning | When to change |
|-----|------|---------|---------|----------------|
| `addr` | string | `:8081` | Listen | Bind `127.0.0.1:8081` or LAN in production |
| `enabled` | bool | `true` | `false` **skips the admin listener only** (no admin API / SPA / proxy). Client plane and health still run | Client-only hosts |
| `mode` | string | example `debug` | Combined Gin mode | Production `release` |
| `tls_cert` / `tls_key` | path | `""` | HTTPS only when both are set | Fill when this plane terminates TLS; otherwise terminate at the reverse proxy |
| `trusted_proxies` | string[] | `[]` | Empty = do not trust `X-Forwarded-For` | After an admin reverse proxy |
| `static_dir` | path | `frontend/dist` | See below | Usually leave default |

## Admin UI files

Official binaries embed the console. Precedence:

1. `static_dir` is the **empty string** → **UI off** even if embed has `index.html`.
2. Non-empty `static_dir` with disk `index.html` → disk (rebuild Go is not required after replacing dist).
3. Else embed FS has `index.html` → embed.
4. Neither → API-only (`GET /` is JSON 404).

`static_dir` is the admin UI tree (or empty to disable the UI). The VitePress docs site is not the console. Official Release binaries already embed the UI; copying `frontend/dist` is not a user install step.

In production the admin plane **reverse-proxies** `/api/v1/projects/**` to this process’s client plane for console check/integrity previews.
