---
title: "调用约定"
description: "客户端平面 :8080。JSON snake_case。错误看 error.code。探活 GET /api/v1/health。"
---

# 调用约定

基址：客户端平面默认 `:8080`，路径均在 `/api/v1/...`。`project_ref`：UUID / 活 slug / 未过期别名；过期别名 404 `PROJECT_NOT_FOUND`。POST 不 301。

JSON 字段 snake_case。时间 RFC 3339 UTC。错误信封：

```json
{ "error": { "code": "UNAUTHORIZED", "message": "…", "details": null } }
```

逻辑只看 `code`。商店缺 listing 为**纯文本 404**，没有该信封。

探活：`GET /api/v1/health` 恒 200，`ready` = DB Ping + 存储 Head `.ready`（不含 Redis）。`GET /api/v1/ready` 为 **404**，不要当就绪探针。

本平面契约：`GET /api/v1/openapi.json`。

## 鉴权

| 机制 | 用途 | 失败 |
|------|------|------|
| `X-Project-Token` 或 `Authorization: Bearer` | 项目 Token | 401 |
| `X-Channel-Token` | 隐藏渠道 | check 填错则跳过该渠道，**不** 403 |
| 商店 listing Token | feed | 随 StoreAuth；缺 listing 纯文本 404 |
| 管理 Cookie/Bearer | 管理平面 | 401 / 403 |

完整字段以 [API 参考](/api/reference/) 为准。
