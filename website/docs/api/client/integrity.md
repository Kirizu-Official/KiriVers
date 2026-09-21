---
title: "完整性清单"
description: "GET .../versions/{version}/integrity。一次返回全部文件。无 cursor。"
---

# 完整性清单

`GET /api/v1/projects/{project_ref}/versions/{version}/integrity`

一次返回全部文件，无 `cursor` / `limit`。`include_file_urls` 仅在总文件数 ≤ 16 时生效。多文件客户端先对照本地再 `POST /update/pack`。ETag 为 line `RootHash`。

完整字段：[API 参考](/api/reference/) 中的 `openapi.client.json`（<a href="/api/scalar/client" target="_blank" rel="noopener">新窗口打开 Scalar</a>）。
