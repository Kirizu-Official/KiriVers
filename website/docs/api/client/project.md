---
title: "项目公开信息"
description: "GET /api/v1/projects/{project_ref}。过期别名 404 PROJECT_NOT_FOUND。"
---

# 项目公开信息

```bash
curl -sS http://127.0.0.1:8080/api/v1/projects/my-app
```

`project_ref`：UUID、活 slug、未过期别名。过期别名 → 404 `PROJECT_NOT_FOUND`。需要项目 Token 时带 `X-Project-Token` 或 `Authorization: Bearer`。完整字段见 [API 参考](/api/reference/) 中的 `openapi.client.json`（<a href="/api/scalar/client" target="_blank" rel="noopener">新窗口打开 Scalar</a>）。
