---
title: "管理员初始化"
description: "在运行二进制的机器上用 kirivers admin add 创建第一个能登录管理台的账号。"
---

# 管理员初始化

服务已经能启动之后，在**运行程序的那台电脑**上创建第一个网页账号。这是命令行操作，不是网页注册。做完本页才能进入 [管理 → 快速上手](/admin/quick-start)。

`kirivers admin` 只读系统 YAML（PostgreSQL DSN），不读 `admin.yaml`。

## 创建

1. 打开终端，进入二进制与 YAML 所在目录（或传 `-config`）。
2. 运行：

```bash
kirivers admin add alice
```

3. 按提示设置密码（或 `-password`）。成功：

```text
created alice (<uuid>)
```

4. 浏览器打开管理平面（默认 `http://127.0.0.1:8081`），用该用户名登录。首次登录须绑定 TOTP（见 [登录与两步验证](/admin/security)）。

## 其它动作

| 命令 | 作用 | 成功输出 |
|------|------|----------|
| `kirivers admin list` | 列出实例管理员 | 表头 `USERNAME ID CREATED_AT` |
| `kirivers admin reset-password alice` | 重置密码 | `password reset for alice` |
| `kirivers admin clear-2fa alice` | 清除 TOTP / Passkey / 恢复码 | `cleared 2FA for alice` |
| `kirivers admin delete alice` | 删除账号 | `deleted alice` |

`clear-2fa` **不**踢出现有会话。删除最后一名管理员失败（<ErrorCode code="LAST_ADMIN" />）。
