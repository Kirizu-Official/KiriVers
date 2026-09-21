---
title: "本地开发"
description: "dev/docker 起 Postgres/Redis。go run .。改 UI 用 yarn dev :3000。Vite 代理顺序。提交前跑 dev/build CLI 自检。"
---

# 本地开发

```bash
docker compose -f dev/docker/compose.yml up -d
cp configs/config-example.yaml configs/config.yaml
cp configs/admin-example.yaml configs/admin.yaml
cp configs/client-example.yaml configs/client.yaml
go run .
```

API 双端口：`:8080` / `:8081`。`dev/docker/compose.yml` **不含** KiriVers 进程，只给本机 `go run .` 起库。要在 Docker 里跑完整栈（Hub 镜像 + PostgreSQL + Redis）用使用者一键目录 `deploy/`；`dev/build/` 只剩运行镜像 `Dockerfile` 与发版 CLI，不再放 Compose 文件。

## Docker 一键运行（Hub 镜像 + PostgreSQL + Redis）

本机跑完整栈就用 `deploy/`（详见 [Docker Compose](/guide/install/compose)）。口令只写在 `deploy/.env`（模板 `.env.example`），同时供给 Postgres、`redis-server --requirepass`、以及 `KIRIVERS_POSTGRES_DSN` / `KIRIVERS_CACHE_REDIS_*` / `KIRIVERS_URL_SIGNING_SECRET`。缓存走 Redis（`KIRIVERS_CACHE_DRIVER=redis`）。

```bash
cd deploy
cp .env.example .env
docker compose up -d
```

持久化：`deploy/config/` → `/config`；`deploy/data/` → 对象、日志与库。`deploy/` 只在宿主机映射 `8080` / `8081`（映射 5432/6379 的是 `dev/docker/`），两者不会端口冲突。不要在仓库根添加 `compose.yml`。从源码编镜像见 [发版与 Docker](./release)。

本地无 Docker 时仍用上面的 `go run .`（需要 `CGO_ENABLED=1` 与 C++ 工具链）。官方发版二进制与镜像一律 CGO；Releases 上传的是 `KiriVers-<OS>-<Arch>-<sha6>.zip`，解压出来才是要运行的 `kirivers-<os>-<arch>`（Windows 为 `.exe`），见 [发版与 Docker](./release)。

## 改管理台 UI

必须：

```bash
cd frontend
yarn
yarn dev
```

浏览器打开 `http://localhost:3000`。Vite 把请求代理到真实后端。日常改 UI 用这条路径；`yarn build` 后打开 `:8081` 只用于验证生产内嵌。

Vite 代理：`/api/v1/projects` **必须排在** `/api` 之前（分别打 :8080 / :8081）。

验证生产内嵌：`yarn build` 后 `go build`。磁盘 `static_dir` 有 `index.html` 则 :8081 走磁盘，否则走 embed。`static_dir: ""` 关闭 UI。

## 提交前本地校验

流水线的判定都写在 `dev/build/kirivers.py` + `dev/build/kirivers_build/`（纯标准库 Python，无需安装依赖），可以离线跑一遍再开 PR。Linux / macOS 用 `python3`，Windows 用 `python`：

```bash
python3 dev/build/kirivers.py check-workflows              # 工作流静态门
python3 dev/build/kirivers.py release-notes --self-check   # 更新日志格式 + 类型表不变量
python3 dev/build/kirivers.py guard --self-check           # PR 守卫判定矩阵
python3 dev/build/kirivers.py issue-link --self-check
python3 dev/build/kirivers.py asset-names                  # 打印 10 个归档名与包内二进制名
```

- 版本号只由 Conventional Commits 推导；仓库根 `CHANGELOG.md` 由发版作业写回，**不要手改**（格式与自定义见 [发版与 Docker](/contribute/release#changelog-format)）。
- 要看或改 `sdk/<语言>` 分支：`git worktree add .worktrees/<lang> sdk/<lang>`。`.worktrees/` 已在 `.gitignore` 里，不要用 checkout 盖掉默认分支工作树。
