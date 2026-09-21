---
title: "管理后台配置"
description: "默认 :8081。static_dir 磁盘优先于 embed；空字符串关闭 UI。enabled=false 只关管理监听。"
---

# 管理后台配置

文件：`admin.yaml`。前缀 `KIRIVERS_ADMIN_`。

| 参数 | 类型 | 默认 | 说明 | 何时改 |
|------|------|------|------|--------|
| `addr` | string | `:8081` | 监听 | 生产绑定 `127.0.0.1:8081` 或内网 |
| `enabled` | bool | `true` | `false` **只关闭管理监听**（无管理 API / SPA / 反代）。客户端平面与 health 仍运行 | 只要客户端平面时 |
| `mode` | string | 示例 `debug` | 与 client 一起决定进程 Gin mode | 生产 `release` |
| `tls_cert` / `tls_key` | path | `""` | 同时非空才 HTTPS | 管理平面自己终止 TLS 时填写；否则由反代终止 |
| `trusted_proxies` | string[] | `[]` | 空则不信任 `X-Forwarded-For` | 管理面反代后 |
| `static_dir` | path | `frontend/dist` | 见下表 | 一般保持默认 |

## 管理台静态文件

官方二进制已嵌入管理台。优先级：

1. `static_dir` **显式空字符串** → **关闭 UI**（即使嵌入了 `index.html`）。
2. 非空 `static_dir` 且磁盘上有 `index.html` → 使用磁盘（改 dist 不必重编 Go）。
3. 否则嵌入 FS 含 `index.html` → embed。
4. 二者都没有 → API-only（`GET /` 为 JSON 404）。

`static_dir` 指向管理台静态文件（或留空关闭 UI）。文档站 VitePress 产物不是管理台；官方 Releases 二进制已嵌入 UI，不必再拷 `frontend/dist`。

生产管理平面会把 `/api/v1/projects/**` **反代**到本进程客户端平面，供控制台预览 check / integrity。
