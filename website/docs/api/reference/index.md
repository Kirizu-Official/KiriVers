---
title: "API 参考"
description: "两份未过滤平面契约：openapi.client.json（客户端与商店）与 openapi.admin.json（管理后台与 CI）。下载或在新窗口打开 Scalar。"
---

# API 参考

KiriVers 有两个 HTTP 平面。文档站构建时把运行中进程使用的同一份契约文件原样复制到本站，**不**再按路径切成四份。

| 平面 | 契约文件 | 覆盖 |
|------|----------|------|
| 客户端 | `openapi.client.json` | 原生 JSON（check、下载、差量、公告等）**和**商店 `/store/` feed |
| 管理后台 | `openapi.admin.json` | 控制台自动化 **和** CI Token / `ci/releases` |

对照：

- **客户端 / 商店**：`openapi.client.json`
- **管理后台 / CI**：`openapi.admin.json`

运行中各平面 `GET /api/v1/openapi.json` 与下表下载文件来自同一源。字段表以契约为准，教程页不手抄 schema。

## 下载

- <a href="/openapi/openapi.client.json" download="openapi.client.json"><code>openapi.client.json</code></a>
- <a href="/openapi/openapi.admin.json" download="openapi.admin.json"><code>openapi.admin.json</code></a>

## 在新窗口浏览 Scalar

Scalar 是全屏契约浏览器，**不**嵌在带侧栏的文档页里。请用新窗口打开：

- <a href="/api/scalar/client" target="_blank" rel="noopener">客户端 / 商店 Scalar</a>
- <a href="/api/scalar/admin" target="_blank" rel="noopener">管理后台 / CI Scalar</a>

相关教程：[客户端调用](/api/client/)、[管理后台自动化](/api/admin/)、[自动发版](/api/ci)、[调用约定](/api/conventions)。失败码见 [FAQ · 失败代码对照](/faq/errors)。
