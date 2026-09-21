---
title: "公开目录"
description: "GET channels / matrix / languages。隐藏渠道不出现在列表。空列表 200。"
---

# 公开目录

| 路径 | 信封 |
|------|------|
| `GET .../channels` | `{ "channels": [] }` 公开渠道；省略 unlisted / TokenProtected |
| `GET .../matrix` | `{ "matrix": [] }` 仅 os / arch / package_type |
| `GET .../languages` | `{ "languages": [] }` |

空列表仍是 **200**。没有客户端 `GET /channels/:slug`。隐藏渠道升级靠 check 的 `channel=` + `X-Channel-Token`。
