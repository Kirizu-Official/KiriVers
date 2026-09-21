---
title: "多文件打包"
description: "同 URL 轮询。过大 200 full_package。无管理 job_id。"
---

# 多文件打包

`POST /api/v1/projects/{project_ref}/update/pack`

同一 URL 入队与轮询。不要调用 `pack/status`。不要用管理 `GET /jobs/{id}`。

| 状态 | HTTP | 含义 |
|------|------|------|
| `pending` | 202 | 已入队或已有同 fileset Job |
| `ready` | 200 | 差量 zip 可下载 |
| `full_package` | **200** | 过大、缺路径或 Job 失败后的整包回退 |

超 `dynamic_pack.max_bytes` 或未压缩 Manifest 占比过高 → 200 `full_package`，不是 400。

完整字段：[API 参考](/api/reference/) 中的 `openapi.client.json`（<a href="/api/scalar/client" target="_blank" rel="noopener">新窗口打开 Scalar</a>）。
