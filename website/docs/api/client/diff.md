---
title: "单文件差量"
description: "POST /update/diff。local_sha256 只出现在此。不入队 pack。"
---

# 单文件差量

`POST /api/v1/projects/{project_ref}/update/diff`

`local_sha256` **只**出现在本请求，不出现在 check。Lookup-only：不入队 pack。未知 magic 由客户端回退整包。管理生成差量是 Job。`Cache-Control: private, no-store`，无 ETag。

完整字段：[API 参考](/api/reference/) 中的 `openapi.client.json`（<a href="/api/scalar/client" target="_blank" rel="noopener">新窗口打开 Scalar</a>）。概念：[增量更新](/guide/features/incremental)。
