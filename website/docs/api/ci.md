---
title: "自动发版"
description: "CI Token 调用 ci/releases。任务进度走管理 jobs。"
---

# 自动发版

使用项目 **CI Token** 调用管理平面（默认 `:8081`）：

| 方法 | 路径 |
|------|------|
| GET, POST | `/api/v1/admin/projects/{project_ref}/ci-tokens` |
| DELETE | `/api/v1/admin/projects/{project_ref}/ci-tokens/{token_id}` |
| POST | `/api/v1/admin/projects/{project_ref}/ci/releases` |

GitHub Actions（或任何 CI/CD 平台）：把 CI Token 放进密钥，构建产物后 `POST ci/releases`。任务进度**不**在本页：`GET /api/v1/admin/jobs/{job_id}`（管理后台参考）。

::: info 别和 KiriVers 自己的发版搞混
这里的 CI Token 是**你项目的流水线**用来向 KiriVers 上传发布包的凭据。KiriVers 服务端自身的构建与发版是另一回事，见 [贡献 → 发版与 Docker](/contribute/release)。
:::

可浏览契约：[API 参考](/api/reference/) 中的 `openapi.admin.json`（<a href="/api/scalar/admin" target="_blank" rel="noopener">新窗口打开 Scalar</a>）。
