---
title: "管理 · 项目"
description: "项目 CRUD、stats、settings 面板对应字段。详细 schema 见 API 参考。"
---

# 管理 · 项目

教程：列表、创建、PATCH 设置。显示名 `name` 不是 `project_ref`。列表/详情 `stats` 含 `storage_bytes`，不含设备哈希。`storage_visibility=private` 会被拒绝（400 `INVALID_REQUEST`）。商店 listing 信封 `{ listings: [] }`；重复 `(protocol, slug)` 400。

安装策略、渠道、矩阵、语言、成员各有独立资源，不要塞进项目 PATCH。完整路径：[API 参考](/api/reference/) 中的 `openapi.admin.json`（<a href="/api/scalar/admin" target="_blank" rel="noopener">新窗口打开 Scalar</a>）。
