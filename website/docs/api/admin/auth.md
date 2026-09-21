---
title: "管理鉴权"
description: "密码登录、pending 第二因素、会话。Passkey 需要 rp_id 与 origins。"
---

# 管理鉴权

管理平面默认 `:8081`。

1. `POST /api/v1/admin/auth/login` 提交用户名与密码。
2. 若需绑定或第二因素，使用 pending token 调用 TOTP / 恢复码 / WebAuthn 系列。
3. 完整会话后，后续请求带 Cookie 或 Bearer。
4. `POST /api/v1/admin/auth/logout` 结束**当前**会话。

`webauthn_rp_id` 或 `webauthn_origins` 任一为空时 Passkey 路由 `NOT_READY`。TOTP 超限 `TOTP_RATE_LIMITED`。CLI `clear-2fa` 不吊销已有会话。完整路径：[API 参考](/api/reference/) 中的 `openapi.admin.json`（<a href="/api/scalar/admin" target="_blank" rel="noopener">新窗口打开 Scalar</a>） Auth 标签。
