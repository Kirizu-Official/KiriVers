---
title: "管理后台自动化"
description: "按 OpenAPI tag 列出管理平面。详细字段以 API 参考为准。"
---

# 管理后台自动化

管理平面默认 `:8081`。鉴权：登录会话 Cookie 或 Bearer。完整字段：[API 参考](/api/reference/) 中的 `openapi.admin.json`（<a href="/api/scalar/admin" target="_blank" rel="noopener">新窗口打开 Scalar</a>） 与 [CI](/api/reference/)。

OpenAPI tags（无遗漏）：

| Tag | 用途 |
|-----|------|
| Auth | 登录、登出、TOTP/Passkey/恢复码 |
| Admins | 实例管理员 |
| Projects | 项目 CRUD、Token、统计 |
| Versions | 版本与版本线、发布/吊销/晋升 |
| Line Defaults | 版本线默认 |
| Artifacts | 上传、TUS、presign（`direct_s3` false）、差量 Job、复用、归档 |
| Manifest | 多文件清单 |
| Gray | 白名单与旋钮 |
| Jobs | `GET /jobs/{job_id}` |
| GeoIP | 私有库 |
| Nodes | 节点与 node-sync |
| Media | 管理端上传配图 |
| Audit | 审计 |
| Webhooks | 投递记录 |
| Store Listings | listing CRUD |
| InstallPolicy | 安装策略规则 |
| Telemetry | 隐私删除 |
| Channels | 渠道 |
| Platforms | 矩阵与硬件代号、catalog |
| Languages | 项目语言 |
| Members | 项目成员 |
| Clients | 名册 |
| Announcements | 公告 CRUD |
| System | `GET /api/v1/health`、`openapi.json` |

CI Token 与 `ci/releases` 见 [自动发版](/api/ci) 与 [API 参考](/api/reference/) 中的 `openapi.admin.json`（<a href="/api/scalar/admin" target="_blank" rel="noopener">新窗口打开 Scalar</a>）。
