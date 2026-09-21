---
title: "Docker Compose"
description: "拷贝 deploy/ 目录，改一份 .env，即可一键拉起官方镜像 + PostgreSQL + Redis。"
---

# Docker Compose

[`deploy/`](https://github.com/Kirizu-Official/KiriVers/tree/main/deploy) 是给使用者的一键目录：官方镜像 + PostgreSQL + Redis。整目录可以拷到服务器，不必再从仓库其它位置抄配置。

::: info 不要和开发文件搞混
`dev/docker/compose.yml` 只给本机 `go run .` 起 Postgres/Redis，**不含** KiriVers 进程。仓库里给使用者的一键栈只有 `deploy/` 这一份（`dev/build/` 不再放 Compose 文件，只有运行镜像 `Dockerfile` 与发版 CLI）。
:::

## 启动

```bash
cd deploy
cp .env.example .env
# 改 POSTGRES_PASSWORD、REDIS_PASSWORD、URL_SIGNING_SECRET
docker compose up -d
```

`.env` 是唯一口令入口。同一组值会同时进入：

| 写在 `.env` | 用到哪里 |
|-------------|---------|
| `POSTGRES_*` | PostgreSQL 容器，以及进程的 `KIRIVERS_POSTGRES_DSN` |
| `REDIS_PASSWORD` | Redis `requirepass`，以及 `KIRIVERS_CACHE_REDIS_PASSWORD` |
| `URL_SIGNING_SECRET` | `KIRIVERS_URL_SIGNING_SECRET` |

`deploy/config/` 里已有三份 YAML，**不要**再从 `configs/*-example.yaml` 复制。YAML 里的 `CHANGE_ME` 会被 `.env` 覆盖，改 YAML 里的库口令不会生效。

进程固定 `KIRIVERS_CACHE_DRIVER=redis`，地址 `redis:6379`。预期：`docker compose ps` 中 `postgres`、`redis`、`kirivers` 均为 running。浏览器打开 `http://127.0.0.1:8081`。

数据在 `deploy/data/`（Postgres、Redis AOF、安装包、日志）。对外只映射 `8080` / `8081`。

::: danger 示例口令
`.env.example` 里的 `CHANGE_ME` 不能用于生产。
:::

## 用哪个镜像标签

`deploy/compose.yml` 里写的是 `kirizuofficial/kirivers:latest`，即**最近一次发版**。想固定在某个版本，把 `image:` 改成对应标签：

| 标签 | 含义 |
|------|------|
| `kirizuofficial/kirivers:latest` | 最近一次发版 |
| `kirizuofficial/kirivers:<semver>` | 指定版本的多架构 manifest（按宿主架构自动选） |
| `kirizuofficial/kirivers:<semver>_amd64` / `<semver>_arm64` | 该版本的单架构镜像，只在明确锁定架构时用 |

官方镜像只有 `linux/amd64` 与 `linux/arm64` 两种（Alpine 基底、musl 构建）。其他 Linux 架构没有镜像，请改用 [Releases 的 zip 包](/guide/install/manual)。升级前先备份 PostgreSQL 与 `storage.local.root`，再 `docker compose pull && docker compose up -d`。

下一步：[管理员初始化](/guide/config/bootstrap-admin)。
