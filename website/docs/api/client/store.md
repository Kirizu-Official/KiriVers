---
title: "商店 feed"
description: "/store/{protocol}/{listing_slug}。缺 listing 纯文本 404。九种 protocol。"
---

# 商店 feed

`GET|POST /api/v1/projects/{project_ref}/store/{protocol}/{listing_slug}`  
可选 `/{doc}`。

protocol：`sparkle`、`electron`、`tauri`、`squirrel`、`clickonce`、`appimage`、`winget`、`msix`、`fdroid`。缺 listing、未知 protocol、旧路径无 slug → **纯文本 404**（无 JSON `code`）。官方 SDK **不**读 feed。

完整契约：[API 参考](/api/reference/) 中的 `openapi.client.json`（<a href="/api/scalar/client" target="_blank" rel="noopener">新窗口打开 Scalar</a>）。创建 listing 在管理后台参考。场景：[Electron](/guide/scenarios/electron)、[Sparkle](/guide/scenarios/sparkle)。
