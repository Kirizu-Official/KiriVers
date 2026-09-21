---
title: "OpenAPI 契约"
description: "openapi.client.json 与 openapi.admin.json 为源。文档站原样复制两份平面契约，不再切四份。"
---

# OpenAPI 契约

源文件：`internal/controller/openapi.client.json` 与 `openapi.admin.json`。测试：`TestOpenAPIRoutesSync`、`TestPlaneSpecsValid`、`TestOpenAPIForbiddenNames` / `TestOpenAPIFieldTables`。改 handler JSON 必须改对应平面文件。

文档站 `yarn docs:build` 把这两份 JSON **原样复制**为 `openapi.client.json`（客户端 + 商店）与 `openapi.admin.json`（管理后台 + CI）。不再按 `/store/` 或 CI 路径切片。使用者下载与 Scalar 新窗口见 [API 参考](/api/reference/)。不要手抄字段表。
