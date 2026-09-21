---
title: "手动安装"
description: "Windows、macOS 与 Linux 上安装 PostgreSQL、下载 Releases 二进制、配置三份 YAML 并开机自启。"
---

# 手动安装

本节从「已有 PostgreSQL（可选 Redis）」走到「进程在听、浏览器能打开管理台」。官方 Releases **不必**再拷贝 `frontend/dist`。

::: warning 二进制来源
打开 <https://github.com/Kirizu-Official/KiriVers/releases>，按 [选哪种安装方式](/guide/install/#releases-files) 中的**前缀**下载对应 zip（例如 `KiriVers-Windows-x86_64-<哈希>.zip`、`KiriVers-Linux-arm64-<哈希>.zip`）。解压得到 `kirivers-<os>-<arch>`（Windows 为 `.exe`）。以当前 Release 为准，不要使用猜测的直链——文件名末尾的哈希每次发版都变。
:::

## Windows

1. 安装 [PostgreSQL Windows 安装包](https://www.postgresql.org/download/windows/)。记住你设置的超级用户密码与端口（默认 `5432`）。创建数据库与角色，例如数据库名 `kirivers`、用户 `kirivers`。
2. Redis：单机可跳过，使用 `cache.driver=memory`。若需要 Redis，安装 Windows 可用的 Redis 发行或改用 Docker 只跑 Redis。
3. 从 Releases 下载匹配的前缀（`KiriVers-Windows-x86_64-`、`-x86-` 或 `-arm64-`），解压出的 `.exe` 放到固定目录，例如 `C:\KiriVers\`。
4. 将仓库 `configs/config-example.yaml`、`admin-example.yaml`、`client-example.yaml` 复制到同一目录，分别改名为 `config.yaml`、`admin.yaml`、`client.yaml`。用记事本或 VS Code 修改**必须改才能启动**的项：
   - `postgres.dsn`：主机、用户、密码、库名、端口
   - 生产环境设置 `url_signing_secret`（否则重启后已签发下载 URL 失效）
   其余键见 [配置](/guide/config/)。
5. 在该目录打开终端：

```bat
kirivers.exe
```

无参数等价于 `kirivers server`。预期：进程保持运行；日志出现客户端 `:8080` 与管理 `:8081` 监听。

6. 浏览器打开 `http://127.0.0.1:8081`。应看到登录页（「登录 KiriVers」）。若只看到 JSON：检查 `admin.yaml` 的 `static_dir` 是否被设成空字符串（空则**关闭 UI**），以及是否使用了未先构建管理台的自编译占位二进制。官方 Releases 已内嵌管理台。

::: details 打开 :8081 只看到 JSON
`static_dir: ""` 会关闭 UI（即使二进制已嵌入 `index.html`）。自编译且从未在 `frontend/` 执行 `yarn build` 时，嵌入 FS 可能没有 `index.html`。官方 Releases **不要**再拷贝 `frontend/dist`。
:::
7. 开机自启：任务计划程序 → 创建基本任务 → 触发器「计算机启动」→ 操作「启动程序」指向 `kirivers.exe`，起始于配置文件所在目录。

## macOS

PostgreSQL：Homebrew `brew install postgresql@17` 或 [Postgres.app](https://postgresapp.com/)。可选 `brew install redis`。从 Releases 下载 `KiriVers-macOS-x86_64-`（Intel）或 `KiriVers-macOS-arm64-`（Apple Silicon），解压后复制三份 YAML，在终端运行 `./kirivers`。浏览器打开 `http://127.0.0.1:8081`。

## Linux（未走一键脚本）

::: code-group

```bash [apt]
sudo apt-get update
sudo apt-get install -y postgresql postgresql-contrib
```

```bash [dnf]
sudo dnf install -y postgresql-server postgresql
sudo postgresql-setup --initdb
sudo systemctl enable --now postgresql
```

:::

下载 Linux 可执行文件，复制三份 YAML，执行 `./kirivers`。

### systemd 示例

将 `User`、`WorkingDirectory`、`ExecStart` 换成实际路径：

```ini
[Unit]
Description=KiriVers
After=network.target postgresql.service

[Service]
Type=simple
User=kirivers
WorkingDirectory=/opt/kirivers
ExecStart=/opt/kirivers/kirivers -config /opt/kirivers
Restart=on-failure

[Install]
WantedBy=multi-user.target
```

- `WorkingDirectory`：三份 YAML 所在目录（或用 `-config` 指向该目录）。
- `ExecStart`：二进制路径。无参数即启动双平面。
- `After=postgresql.service`：尽量等数据库起来；数据库在启动时必须可连，否则进程 fatal。

启用：`sudo systemctl enable --now kirivers`。

下一步：[管理员初始化](/guide/config/bootstrap-admin)。
