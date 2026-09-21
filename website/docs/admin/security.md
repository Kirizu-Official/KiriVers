---
title: "登录与两步验证"
description: "密码 → pending → 第二因素。Passkey 必须同时配置 webauthn_rp_id 与 webauthn_origins。clear-2fa 不吊销会话。"
---

# 登录与两步验证

登录页标题 **登录 KiriVers**。流程：密码 → 待绑定或第二因素（pending TTL：`security.login_pending_ttl_seconds`）→ 完整会话。空闲滑动 TTL：`security.session_idle_hours`。管理员鉴权使用**缓存会话 Token**（服务端 `Authorization: Bearer` 携带的不透明随机串）。

谁能打开：任何能访问管理平面（默认 `http://127.0.0.1:8081`）的浏览器。首次账号须先用 [管理员初始化](/guide/config/bootstrap-admin) 创建。

## 首次登录

1. 输入 **用户名**、**密码**，点 **登录**。
2. 首次须 **绑定身份验证器**：扫码或手动输入密钥，填写 **6 位验证码**，点 **确认绑定**。
3. **保存恢复码**（只显示一次）。勾选 **我已安全保存这些恢复码**，再点 **继续**。
4. 出现 **添加 Passkey（可选）**。可点 **注册 Passkey**，或点 **跳过**。之后在右上角 **账号安全** 添加。

同一 30s 周期验证码次数用尽：<ErrorCode code="TOTP_RATE_LIMITED" />。

## 配置 Passkey（WebAuthn）

Passkey 依赖系统 YAML 里**两个**键。`security.webauthn_rp_id` 或 `security.webauthn_origins` **任一为空**，Passkey 接口返回 <ErrorCode code="NOT_READY" />（HTTP 503）。TOTP 与恢复码仍可用。

界面此时显示：「服务器未配置 WebAuthn，暂时无法添加 Passkey。仍可用 TOTP 或恢复码登录。」

| 参数 | 类型 | 默认 | 含义 | 何时改 | 失败 |
|------|------|------|------|--------|------|
| `security.webauthn_rp_id` | string | `""` | 依赖方 ID：主机名，**不含** scheme 与端口 | 启用 Passkey | 空 → <ErrorCode code="NOT_READY" /> |
| `security.webauthn_origins` | string[] | `[]` | 完整 Origin 列表（含 scheme 与端口） | 必须与浏览器地址栏 Origin 一致 | 空或与浏览器不一致 → 浏览器拒绝或 <ErrorCode code="NOT_READY" /> |

示例（本机管理平面 `http://localhost:8081`）：

<<< @/../../configs/config-example.yaml{59-70}

改完后把 `webauthn_rp_id` 设为 `localhost`（或公网主机名 `updates.example.com`），把 `webauthn_origins` 设为完整 Origin，例如 `http://localhost:8081`。**改 YAML 后必须重启进程。**

::: warning HTTPS
非 localhost 需要 HTTPS 安全上下文，否则浏览器不允许 WebAuthn。localhost 可用 `http://localhost:8081`。
:::

::: code-group

```yaml [localhost]
security:
  webauthn_rp_id: localhost
  webauthn_origins:
    - http://localhost:8081
```

```yaml [公网]
security:
  webauthn_rp_id: updates.example.com
  webauthn_origins:
    - https://updates.example.com
```

:::

YAML 细节：[安全相关配置](/guide/config/security)。

## 账号安全（后续添加 / 删除 Passkey）

右上角用户菜单 **账号安全**（`/security`），不是抽屉项。页内说明：**不会踢掉其他已登录会话**。

1. 打开 **账号安全**。
2. 在 **Passkey** 区域点 **添加 Passkey**，填写 **Passkey 名称**，完成浏览器提示。
3. 删除：点 **删除 Passkey**，确认。不会退出已登录会话，也不会移除 TOTP。
4. **轮换 TOTP** / **重新生成** 恢复码同样不踢会话。

若服务器未配置 WebAuthn，**添加 Passkey** 不可用，并显示上面那句界面提示。仍可用 TOTP 或恢复码登录。

## `clear-2fa`

本机 CLI：

```bash
kirivers admin clear-2fa alice
```

清除该账号的 TOTP、Passkey 与恢复码。**不**吊销已有会话。对方下次登录会重新绑定身份验证器。见 [管理员初始化](/guide/config/bootstrap-admin)。
