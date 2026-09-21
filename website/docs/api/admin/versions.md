---
title: "管理 · 版本"
description: "draft/ready/publish。latest=true 不是 SelectTarget。direct_s3 恒 false。"
---

# 管理 · 版本

创建版本、上传产物（TUS）、标记就绪、发布/吊销/晋升。`GET versions?latest=true` 的 `latest` 用 `CompareVersions`，不是客户端 `SelectTarget`。`direct_s3` 恒 false。灰度、差量 Job 见参考页 Gray / Artifacts / Jobs。

CI 批量发版走 [自动发版](/api/ci) 的 `ci/releases`。完整路径：[API 参考](/api/reference/) 中的 `openapi.admin.json`（<a href="/api/scalar/admin" target="_blank" rel="noopener">新窗口打开 Scalar</a>）。
