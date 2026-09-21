---
title: "客户端调用"
description: "原生 JSON 顺序与能力位。完整契约见 API 参考。本平面 GET /api/v1/openapi.json。"
---

# 客户端调用

基址默认 `http://127.0.0.1:8080`。路径均在 `/api/v1/...`。本平面契约：`GET /api/v1/openapi.json`。商店 feed 不在本分组，见 [商店](./store) 与 [API 参考](/api/reference/) 中的 `openapi.client.json`（<a href="/api/scalar/client" target="_blank" rel="noopener">新窗口打开 Scalar</a>）。

建议顺序：探活 → 可选报到 → **POST** 检查更新 → 可选 changelog / 公告 → 下载 `/packages/{sha256}` → 单文件 diff 或多文件 pack。默认 `capabilities` 仅 `full_package`。

错误看 `error.code`。[调用约定](/api/conventions)。完整 schema：[API 参考](/api/reference/) 中的 `openapi.client.json`（<a href="/api/scalar/client" target="_blank" rel="noopener">新窗口打开 Scalar</a>）。
