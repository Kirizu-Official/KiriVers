---
title: "商店适配器"
description: "service/store.DefaultRegistry 九种协议。"
---

# 商店适配器

`internal/service/store.DefaultRegistry` 注册九种协议：`sparkle`、`electron`、`tauri`、`squirrel`、`clickonce`、`appimage`、`winget`、`msix`、`fdroid`。

HTTP 路径 `/store/{protocol}/{listing_slug}`（可选 `/{doc}`）。缺 listing 纯文本 404。listing 身份是 slug。`line_full` 使用 `store_full`，不是 hash-root `full`。Feed 必须调用与原生 check 相同的 `SelectTarget`（匿名、无 device_id）。
