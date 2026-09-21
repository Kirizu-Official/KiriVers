---
title: "设备上报"
description: "POST .../clients/report。200 仅 ip 与 geo。clients/login 为 404。"
---

# 设备上报

`POST /api/v1/projects/{project_ref}/clients/report`

```bash
curl -sS -X POST http://127.0.0.1:8080/api/v1/projects/my-app/clients/report \
  -H 'Content-Type: application/json' \
  -d '{"device_id":"app-stable-id","os":"windows","arch":"x86_64"}'
```

200 体只有 `ip` 与 geo 字段，无 roster 包装。遗留 `POST .../clients/login` 为 **404**。`device_id_policy=none` 时带 device 可能 400 `INVALID_REQUEST`。完整字段：[API 参考](/api/reference/) 中的 `openapi.client.json`（<a href="/api/scalar/client" target="_blank" rel="noopener">新窗口打开 Scalar</a>）。
