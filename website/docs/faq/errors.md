---
title: "失败代码对照"
description: "客户端与管理 OpenAPI Error.code 并集，加上控制台 locales（排除纯前端 NETWORK/HTTP/UNKNOWN）。"
---

# 失败代码对照

逻辑只看 `error.code`。渠道 Token 错误**不进** 403。商店缺 listing **无** JSON `code`。悬停说明与文档站 `ErrorCode` 组件同源。

| code | HTTP（若稳定） | 含义与检查 |
|------|----------------|------------|
| `PROJECT_NOT_FOUND` | 404 | 项目 UUID/slug/别名未命中或别名过期 |
| `VERSION_NOT_FOUND` | 404 | 版本不存在 |
| `VERSION_LINE_NOT_FOUND` | 404 | 该版本没有此 os/arch 线 |
| `VERSION_NOT_VISIBLE` | 404/409 | 版本对客户端不可见（草稿、未预热或灰度未命中） |
| `VERSION_REVOKED` | 409 | 版本已吊销且无安全回落 |
| `NO_SAFE_TARGET` | 409 | 没有安全目标版本 |
| `INTERMEDIATE_UNAVAILABLE` | 409 | 中继版本缺失、未就绪、成环或 hop 超过 8 次 |
| `MIN_OS_NOT_MET` | 409 | 客户端系统/API 低于当前版本线下限 |
| `CHANNEL_CONFLICT` | 409 | 幂等创建时渠道与已有版本不一致 |
| `UNAUTHORIZED` | 401 | 未认证或会话过期 |
| `FORBIDDEN` | 403 | 权限不足（含项目成员访问 /admins、/geoip、/nodes） |
| `PRECONDITION_FAILED` | 412 | If-Match / ETag 失败（不是 POST check） |
| `HW_REV_INCOMPATIBLE` | 409 | 硬件代号不在兼容范围 |
| `RATE_LIMITED` | 429 | 过频；响应带 Retry-After |
| `CHANGELOG_QUERY_INVALID` | 400 | changelog 查询非法 |
| `NOT_FOUND` | 404 | 通用缺失（含未知包哈希） |
| `NOT_READY` | 503/400 | 服务或 Passkey 未就绪。`webauthn_rp_id` 或 `webauthn_origins` 任一为空时 Passkey 接口返回本码（HTTP 503） |
| `INTERNAL_ERROR` | 500 | 服务器内部错误 |
| `INVALID_REQUEST` | 400 | 请求体/字段非法（含非 .mmdb、超上限、未知矩阵对、重复 listing） |
| `INVALID_QUERY_PARAM` | 400 | 查询参数非法（含公告 version、未知 changelog 渠道） |
| `ENGINE_MISMATCH` | 400 | 比较引擎与版本标识不匹配 |
| `ARTIFACT_REQUIRED` | 400 | 发布被拒绝：没有任何就绪的版本线 |
| `CHANNEL_SUFFIX_MISMATCH` | 400 | SemVer 预发布后缀与渠道不一致；stable 不能用预发布后缀 |
| `VERSION_ALREADY_EXISTS` | 409 | 同项目版本号占用 |
| `GRAY_NOT_ALLOWED_ON_CRITICAL` | 400 | 关键版本禁止灰度 |
| `PACKAGE_TYPE_IMMUTABLE` | 409 | 该平台已有已发布版本线后不能改产物形态 |
| `COMPARE_ENGINE_IMMUTABLE` | 409 | 已发布后不能改比较引擎 |
| `INVALID_PATH` | 400 | 非法路径 |
| `ARTIFACT_IMMUTABLE` | 409 | 已发布产物不可覆盖（先 yank 版本线） |
| `HW_REV_UNKNOWN` | 400 | 未登记硬件代号 |
| `DELTA_ALGO_UNSUPPORTED` | 400 | 不支持的差量算法 |
| `DELTA_SAME_VERSION` | 400 | 源与目标版本相同 |
| `TOTP_RATE_LIMITED` | 429 | 同一 30s 周期 TOTP 次数用尽 |
| `UPLOAD_INCOMPLETE` | 400 | 上传未完成，不能发布或标记就绪 |
| `CHANNEL_NOT_FOUND` | 404 | 渠道不存在 |
| `SYSTEM_CHANNEL` | 409 | 系统渠道 alpha/beta/stable 不可删除 |
| `CHECKSUM_MISMATCH` | 400 | 声明 SHA-256 与服务端复算不符 |
| `JOB_NOT_FOUND` | 404 | 任务不存在 |
| `JOB_FAILED` | 409/500 | 异步任务失败 |
| `AUTO_PUBLISH_PENDING` | 409 | auto_publish_when 尚未全部就绪 |
| `ZIP_LAYOUT_INVALID` | 400 | zip 无法解析为 os/arch/…（缺段、未知标识或含路径穿越） |
| `ADMIN_NOT_FOUND` | 404 | 管理员不存在 |
| `USERNAME_TAKEN` | 409 | 用户名占用 |
| `LAST_ADMIN` | 409 | 不能删除最后一名管理员 |
| `LAST_OWNER` | 400 | 不能移除最后一名拥有者 |
| `MEMBER_NOT_FOUND` | 404 | 项目成员不存在 |
| `CLIENT_NOT_FOUND` | 404 | 客户端名册行不存在 |
| `GEOIP_NOT_FOUND` | 404 | GeoIP 库不存在 |
| `LANGUAGE_TAKEN` | 409 | 语言代码已存在 |
| `LANGUAGE_NOT_FOUND` | 404 | 语言不存在 |

`DELTA_SAME_VERSION` 来自控制台 locales，管理 OpenAPI `Error.code` enum 未列入；出现时按「源与目标相同」处理。已排除纯前端 `NETWORK_ERROR` / `HTTP_ERROR` / `UNKNOWN_ERROR`。商店缺 listing **无** JSON `code`。
