---
title: "一键安装"
description: "Linux 脚本用 apt 或 yum/dnf 安装 PostgreSQL，并询问是否安装 Redis。不下载 KiriVers 二进制。"
---

# 一键安装

仓库脚本 [`scripts/install-deps.sh`](https://github.com/Kirizu-Official/KiriVers/blob/main/scripts/install-deps.sh) 只在 Linux 上用 **apt 或 yum/dnf** 安装 PostgreSQL，并询问是否同样用系统包装 **Redis**。

## 做什么、不做什么

| 做 | 不做 |
|----|------|
| 以 root/sudo 安装 PostgreSQL 并尝试启用服务 | 下载 KiriVers 安装包 |
| 询问后可选安装 Redis | 调用 Docker |
| 打印下一步：打开手动安装页，去 GitHub Releases 取对应平台的包（`KiriVers-<OS>-<Arch>-<sha6>.zip`） | 支持 Windows |

::: warning Windows
不要在 Windows 上运行此脚本。请使用 [手动安装](/guide/install/manual)。
:::

## Redis 怎么选

- **可以不装**：单节点、流量不大。保持 `cache.driver=memory`（默认）。
- **应当装**：多个 KiriVers 进程，或要求缓存走 Redis。`cache.driver=redis` 时**启动**必须 Ping 成功，否则进程不监听。运行中 Redis 断开后回退进程内内存并按间隔重连，**不会**在运行中一断 Redis 就立刻退出。细节见 [缓存与 Redis](/guide/config/cache)。

## 适用系统

能检测到 `apt-get` 或 `yum`/`dnf` 的 Linux，且需要管理员权限。macOS 与没有上述包管理器的发行版：脚本退出码 1，改用手动安装或 Docker。

## 步骤

1. 以 root 运行（把仓库 URL 换成你实际克隆或下载脚本的位置）：

::: code-group

```bash [curl]
curl -fsSL https://raw.githubusercontent.com/Kirizu-Official/KiriVers/main/scripts/install-deps.sh | sudo bash
```

```bash [wget]
wget -qO- https://raw.githubusercontent.com/Kirizu-Official/KiriVers/main/scripts/install-deps.sh | sudo bash
```

```bash [local]
sudo bash scripts/install-deps.sh
```

:::

2. 如提示，输入本机 sudo 密码。
3. 脚本询问是否安装 Redis：`y` 安装，回车跳过。
4. 成功时 stdout 会提示 PostgreSQL 已安装，并指向手动安装页从 Releases 取 KiriVers 安装包。记下发行版默认的数据库端口（通常 `5432`）与系统 postgres 用户策略。

::: details 成功后 stdout 会写什么
脚本打印 PostgreSQL 已安装，并指向 GitHub Releases 取包，再按手动安装页复制三份 YAML、创建管理员并启动进程。若安装了 Redis，会说明单进程可继续用 `cache.driver=memory`；多进程或 `cache.driver=redis` 时启动必须 Ping 成功。
:::

::: tip Releases 里下的是 zip
脚本只装数据库，不碰 KiriVers。真正取包时按**前缀**在 Release 页挑 `KiriVers-<OS>-<Arch>-<sha6>.zip`（末尾六位数是包内二进制的 SHA-256 前缀，每次发版都会变），解压得到的可执行文件才叫 `kirivers-<os>-<arch>`（Windows 为 `.exe`）。详见 [手动安装](/guide/install/manual)。
:::

## 失败

| 现象 | 处理 |
|------|------|
| 不是 root | `sudo` 重试 |
| 找不到 apt/yum/dnf | 退出码 1；改手动或 Docker。Docker 未安装**不是**本脚本的失败原因 |
| `5432` 已被占用 | 停掉旧 PostgreSQL 或改 DSN 端口 |
| 发行版仓库没有 postgresql 包 | 启用发行版模块流 / 官方 PostgreSQL 仓库后再跑，或改手动安装 |

下一步：[手动安装](/guide/install/manual)。
