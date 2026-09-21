---
title: "配图"
description: "GET|HEAD .../media/{id}。UUID 公开，无 Token / 无 urlsign。"
---

# 配图

`GET|HEAD /api/v1/projects/{project_ref}/media/{id}`

`id` 为 UUID。公开 GET，无项目 Token、无 `exp`/`sig`。公告 Markdown 中的配图走本路径。完整字段：[API 参考](/api/reference/) 中的 `openapi.client.json`（<a href="/api/scalar/client" target="_blank" rel="noopener">新窗口打开 Scalar</a>）。
