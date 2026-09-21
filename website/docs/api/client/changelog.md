---
title: "更新说明"
description: "GET changelog/{channel}/{os}/{arch}。check 不含正文。Token 不匹配 404。"
---

# 更新说明

`GET /api/v1/projects/{project_ref}/changelog/{channel}/{os}/{arch}`

check 响应**不含** changelog 正文。未知渠道 400 `INVALID_QUERY_PARAM`。渠道 Token 不匹配 404 `NOT_FOUND`（不是 403）。查询：`from_version`、`changelog_scope`（`range_all` / `range_platform` / `target_only`）、`changelog_layout`。无客户端 `limit`。溢出截断仍是 200。

完整字段：[API 参考](/api/reference/) 中的 `openapi.client.json`（<a href="/api/scalar/client" target="_blank" rel="noopener">新窗口打开 Scalar</a>）。概念：[功能指南 · 更新说明](/guide/features/changelog)。
