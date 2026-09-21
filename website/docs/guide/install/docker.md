---
title: "Docker 部署"
description: "使用镜像 kirizuofficial/kirivers 运行 KiriVers，并连接外部 PostgreSQL。"
---

# Docker 部署

::: warning 镜像名称
镜像名是 **`kirizuofficial/kirivers`**。拉取哪一个标签以当前 Docker Hub 与 GitHub Release 为准。官方镜像已内嵌管理台，不必挂载或拷贝 `frontend/dist`。
:::

| 标签 | 含义 |
|------|------|
| `latest` | 指向最近一次发版 |
| `<semver>`（如 `0.3.0`，无 `v` 前缀） | 该版本的多架构 manifest，`docker pull` 自动挑你所在架构 |
| `<semver>_amd64` / `<semver>_arm64` | 该版本的单架构镜像，只在明确指定架构时用 |

镜像支持 `linux/amd64` 与 `linux/arm64`（Alpine 基底、musl 构建）。其他 Linux 架构没有官方镜像，请用 [Releases 的 glibc 产物](/guide/install/#releases-files) 或自编译。

镜像是发布单位；容器是运行单位。

PostgreSQL 必须先存在（另一容器、托管库或宿主机）。本页的 `docker run` 不附带数据库。需要一键起库时用 [Compose](/guide/install/compose)。

## 运行

把宿主机配置目录挂到容器内（目录中需有 `config.yaml`、`admin.yaml`、`client.yaml`），并映射双平面端口：

```bash
docker run --name kirivers --restart unless-stopped \
  -p 8080:8080 -p 8081:8081 \
  -v /var/kirivers/config:/config \
  -e KIRIVERS_CONFIG=/config \
  -e KIRIVERS_POSTGRES_DSN='host=host.docker.internal user=kirivers password=CHANGE_ME dbname=kirivers port=5432 sslmode=disable' \
  kirizuofficial/kirivers
```

预期：容器保持运行；`docker logs kirivers` 出现 `:8080` / `:8081` 监听。浏览器打开宿主机 `http://127.0.0.1:8081`。

官方镜像已含管理台。`admin.yaml` 的 `static_dir` 指向管理台静态文件；文档站 VitePress `dist` 不是管理台。`static_dir: ""` 会关闭 UI。

::: details 容器起来了但浏览器没有登录页
确认映射了 `8081`，且容器内 `admin.yaml` 的 `static_dir` 不是空字符串。官方镜像已嵌入 UI，不要再挂载一份自建的 `frontend/dist`。
:::

## 日志、升级、数据

| 任务 | 做法 |
|------|------|
| 日志 | `docker logs -f kirivers`；同时可把 YAML 里的 `log.file.dir` 挂到卷 |
| 升级镜像 | `docker pull kirizuofficial/kirivers` 后重建容器；先备份 PostgreSQL 与 `storage.local.root` |
| 数据 | 数据库在 PostgreSQL 卷；安装包在对象存储或本地 `storage.local.root` 卷 |

下一步：[Compose](/guide/install/compose) 或 [管理员初始化](/guide/config/bootstrap-admin)。
