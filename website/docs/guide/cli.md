---
title: "CLI"
description: "kirivers server 与 admin add|delete|list|reset-password|clear-2fa。没有 listen 子命令。"
---

# CLI

可执行文件入口等价于 `kirivers`。无位置参数时启动 HTTP 服务（与 `kirivers server` 相同）。

```text
KiriVers — unified startup entry

Usage:
  kirivers [server] [-config path]                              start the HTTP servers (default)
                                                                client plane ... on client.yaml addr (:8080)
                                                                admin plane ... on admin.yaml addr (:8081)
  kirivers admin add <username> [-password secret]              create an instance admin
  kirivers admin delete <username>                              delete an instance admin
  kirivers admin list                                           list instance admins
  kirivers admin reset-password <username> [-password secret]   reset an admin password
  kirivers admin clear-2fa <username>                           clear TOTP, passkeys and recovery codes (does not revoke sessions)
```

**没有** `kirivers listen` 子命令。仓库里的 `cmd/listen.go` 只用于 TLS 监听判定，不是 CLI。

## 配置路径

| 来源 | 优先级 |
|------|--------|
| `-config path` | 最高 |
| 环境变量 `KIRIVERS_CONFIG` | `-config` 缺省时 |
| 默认 `configs/` 然后当前目录 | 都未指定时 |

`path` 可以是文件或目录。目录：该文件夹内的 `config.yaml` + `admin.yaml` + `client.yaml`。文件：该系统 YAML，加上同目录的 `admin.yaml` 与 `client.yaml`。缺文件时回退同目录 `*-example.yaml`。

`kirivers admin` **只加载系统 `config.yaml`**（库连接），不读 `admin.yaml`。

`-password` 仅属于 `admin`。传给 `server` 时以退出码 **2** 打印用法。

工作目录：以下示例均假设当前目录已有三份 YAML，或已传 `-config`。

## `kirivers` / `kirivers server`

```bash
kirivers -config ./configs
```

成功：进程不退出；日志出现客户端与管理平面监听地址。失败：配置/日志双 sink 全关、PostgreSQL 连不上、`cache.driver=redis` 且 Ping 失败 → 非 0 退出且不监听。

常见错误：把 `-password` 传给 server → stderr 含 `-password is only valid with the admin command`，退出码 2。

## `kirivers admin add`

```bash
kirivers admin add alice
```

未传 `-password` 时在终端提示输入并确认密码。成功 stdout：

```text
created alice (<uuid>)
```

## `kirivers admin list`

```bash
kirivers admin list
```

```text
USERNAME  ID                                    CREATED_AT
alice     11111111-1111-1111-1111-111111111111  2026-09-19T04:00:00Z
```

## `kirivers admin reset-password`

```bash
kirivers admin reset-password alice -password 'new-secret'
```

成功：`password reset for alice`

## `kirivers admin clear-2fa`

```bash
kirivers admin clear-2fa alice
```

成功：`cleared 2FA for alice`

清除 TOTP、Passkey 与恢复码。**不**吊销已有管理会话。操作者仍须用新密码或重新绑定 2FA 才能新登录。

## `kirivers admin delete`

```bash
kirivers admin delete alice
```

成功：`deleted alice`。删除最后一名管理员会失败（`LAST_ADMIN`）。

未知子命令或缺用户名：退出码 2，并打印用法。
