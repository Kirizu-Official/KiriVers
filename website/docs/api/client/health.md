---
title: "探活"
description: "GET /api/v1/health 恒 200。ready 为 DB+存储，不含 Redis。GET /api/v1/ready 为 404。"
---

# 探活

`GET /api/v1/health` → HTTP **200**。JSON 含 `ready`：数据库 Ping 与存储 Head `.ready`。**不含 Redis**。

```bash
curl -sS -D - http://127.0.0.1:8080/api/v1/health
```

预期：`HTTP/1.1 200`。运行中 Postgres 不可用时仍 200 且 `ready=false`。`GET /api/v1/ready` 未注册（404），不能作为就绪探针。

管理平面同一路径也提供 health。完整字段：[API 参考](/api/reference/) 中的 `openapi.client.json`（<a href="/api/scalar/client" target="_blank" rel="noopener">新窗口打开 Scalar</a>）。
