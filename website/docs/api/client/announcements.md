---
title: "公告"
description: "GET announcements。空列表 200。显式 locale 严格匹配。"
---

# 公告

`GET /api/v1/projects/{project_ref}/announcements`

与 check **分开**。空匹配 **200** `{ "announcements": [] }`，不是 204。显式 `locale` 严格匹配；省略才走语言链 + leftover。遵守 ETag / 304。完整字段：[API 参考](/api/reference/) 中的 `openapi.client.json`（<a href="/api/scalar/client" target="_blank" rel="noopener">新窗口打开 Scalar</a>）。七种 scope：[功能指南 · 公告](/guide/features/announcements)。
