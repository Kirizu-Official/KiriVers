---
title: "安全相关配置"
description: "会话滑动 TTL、登录 pending、TOTP 速率、WebAuthn rp_id、url_signing_secret。"
---

# 安全相关配置

均在 `config.yaml` 的 `security` 与顶层 `url_signing_secret`。

| 参数 | 类型 | 默认 | 说明 | 何时改 |
|------|------|------|------|--------|
| `security.session_idle_hours` | int | `72` | 完整管理会话**空闲滑动 TTL**；管理员鉴权查缓存中的会话 Token（不透明随机串） | 缩短空闲登出 |
| `security.login_pending_ttl_seconds` | int | `600` | 密码成功后的受限 token（强制绑定 / 第二因素） | 缩短绑定窗口；到期须重新密码登录 |
| `security.totp_max_attempts_per_period` | int | `10` | 同一 30s 时间步内校验次数；超限 <ErrorCode code="TOTP_RATE_LIMITED" /> | 收紧暴力尝试时降低 |
| `security.webauthn_rp_id` | string | `""` | 主机名，无 scheme/端口。与 `webauthn_origins` **任一为空**则 Passkey 接口 <ErrorCode code="NOT_READY" />（HTTP 503）；TOTP 仍可用 | 启用 Passkey 时填写 |
| `security.webauthn_origins` | string[] | `[]` | 完整 Origin 列表（含 scheme 与端口，如 `http://localhost:8081`） | 必须与浏览器地址栏 Origin 一致；改 YAML 后重启 |
| `url_signing_secret` | string | `""` | 私有存储短时签名 HMAC。未配置则启动生成临时随机值并告警——**重启后已签发 URL 全部失效** | 生产必须显式配置或 `KIRIVERS_URL_SIGNING_SECRET` |

Passkey 步骤与 Origin 示例：[登录与两步验证](/admin/security)。

<<< @/../../configs/config-example.yaml{59-70}
