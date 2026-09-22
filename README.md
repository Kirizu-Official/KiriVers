<p align="center">
  <img src="website/docs/public/logo.png" alt="KiriVers" width="120">
</p>

<h1 align="center">KiriVers</h1>

<p align="center">自托管软件更新服务</p> 

<p align="center">
Language: <b>中文</b> · <a href="README.EN.md">English</a>
</p>

<p align="center">
  文档：<a href="https://kirivers.kirizu.dev">https://kirivers.kirizu.dev</a>  | QQ群: <a href="https://qm.qq.com/q/jlwrQ9zSEw">574240693</a>
</p>

KiriVers 让你 **自己托管软件更新**：把安装包、固件或文件列表发布到本服务，设备上的应用来检查并下载。它不是 git，也不能替代 App Store、Google Play 或 Microsoft Store，也不是 APT / RPM / Flatpak 软件源。

> !!! 警告：该项目大部分代码由 AI Agent 自动生成，由于作者时间不多，暂时没有经过人工审核和充分测试，可能存在风险。

一个进程同时提供两个端口：

| 服务 | 默认地址 | 职责 |
|------|----------|------|
| 客户端 | `:8080` | 检查更新、下载、商店 feed、遥测、公告 |
| 管理端 | `:8081` | 管理后台与 CI Agent |

官方 Releases 的可执行文件已内嵌管理台，通常不必再单独编译或手动部署前端。运行需要 **PostgreSQL**（≥ 14）；**Redis 可选**（单机可用内存缓存）。产物可放本机磁盘，或 S3 兼容存储（MinIO / Cloudflare R2 / AWS S3 等）。

## 主要目录

| 入口 | 说明 |
|------|------|
| [`internal`](internal/) / [`cmd/`](cmd/) | Golang 主要业务逻辑代码 |
| [`configs`](configs/) | 配置文件默认存储目录 |
| [`website/`](website/) | VitePress 文档站 |
| [`frontend/`](frontend/) | Vue 管理台源码（由 `frontend/embed.go` 嵌入二进制） |
| [`.trellis/`](.trellis/) | 基于 trellis 的 AI Agent 持久化记忆 |
| [`dev/`](dev/) | 开发环境和辅助脚本 |

## 主要特性

- **自托管**：不依赖第三方服务，数据和更新完全在自己控制的服务器上。
- **多项目**：单个实例可管理多个软件项目，每个项目的数据相互独立，且可以根据项目创建管理员（项目管理员仅能管理对应项目，无法管理其他项目或修改系统配置）。
- **多平台**：支持 Windows、macOS、Linux、iOS、Android 等平台的应用更新。
- **多架构**：在平台的基础上，进一步支持 x86、amd64、ARM 等不同架构的应用更新。
- **多渠道**：支持不同的发布渠道（如 beta、stable、nightly 等），每个渠道可以有不同的版本和更新策略；渠道支持 Token 认证，可作为私密渠道。
- **差量更新**：单文件（例如 APK）支持二进制差量更新，多文件（例如 Windows 上的程序）支持差异文件动态打包和预打包，减少下载体积和带宽消耗。
- **灰度更新**：支持配置灰度更新策略，基于初始灰度百分比，随时间按指定增量进行灰度发布。
- **更新日志**：支持在管理台为每个版本发布更新日志，客户端在检查更新时可获取更新日志内容。
- **公告**：支持在管理台发布公告，支持基于平台、架构、版本单独发布公告，也可以发布项目级公告。
- **多语言**：支持为不同渠道、不同版本创建不同语言的更新日志；允许创建不同语言的公告，客户端可根据系统语言选择显示。
- **多节点**：支持部署多个 KiriVers 实例，由多个子节点负责分发和下载。

## 安装

KiriVers 每个版本发布时以 zip 压缩包的形式在 [GitHub Releases](https://github.com/Kirizu-Official/KiriVers/releases) 提供可执行文件；同时提供 Linux amd64 和 arm64 的 Docker 镜像 [kirizuofficial/kirivers](https://hub.docker.com/r/kirizuofficial/kirivers)。

| 环境 | 建议 |
|------|------|
| Linux | 使用 Docker Compose 运行 |
| Windows | 手动安装 PostgreSQL，并下载二进制文件运行 |

**Docker Compose 一键部署**：使用目录 [`deploy/`](deploy/)（官方镜像 + PostgreSQL + Redis），复制 `.env.example` 为 `.env` 并修改后通过 `docker compose up -d` 启动即可。

> **！！！不要使用开发测试用的 `dev/docker/compose.yml`，在生产环境中使用开发环境的固定密码会有严重的安全问题！！！**

```bash
cd deploy
cp .env.example .env   # 修改里面的密码和配置
docker compose up -d
```

口令只写在 `.env`，会同时进入 Postgres、Redis 和 KiriVers。示例值 `CHANGE_ME` 不能用于生产。

## 第一次启动

### 配置文件

一键 Docker Compose 部署时，`deploy/` 中的 `.env` 会覆盖配置文件；如果你用其他方式安装，需要自行创建配置文件。

1. 复制 `configs/config-example.yaml`、`admin-example.yaml`、`client-example.yaml` 为同目录的 `config.yaml`、`admin.yaml`、`client.yaml`。
2. 填写 PostgreSQL DSN；生成不低于 32 位的随机 `url_signing_secret`（否则重启后已签发的下载 URL 会失效）。
3. 启动（无参数即启动服务，等价于 `kirivers server`）：

```bash
./kirivers
```

### 创建管理员

**注意：** KiriVers 不会在首次启动时创建管理员账户，也不提供默认账户或网页端注册。首次启动后请务必通过 CLI 创建管理员账户，否则无法登录管理台。

通过命令创建第一个管理员，将 `{username}` 替换为你的用户名，运行命令后要求输入密码（输入时不可见）：

```bash
kirivers admin add {username}
```

## 从源码开发

需要 Go（见 `go.mod`）、Node.js ≥ 20、Yarn、PostgreSQL，以及本机 C++ 工具链（服务端差量链接 HDiffPatch，必须 `CGO_ENABLED=1`）。

```bash
# 启动 PostgreSQL 和 Redis
docker compose -f dev/docker/compose.yml up -d

# 先编译一次前端，go build / go run 会把 frontend/dist 打进二进制
cd frontend && yarn install && yarn build

# 启动服务
CGO_ENABLED=1 go run .                   # 客户端 :8080，管理 :8081
CGO_ENABLED=1 go run . admin add <user>  # 另开终端
```

改前端时用 `cd frontend && yarn install && yarn dev` 起开发服务器（`http://localhost:3000`，接口已代理到 `:8080` / `:8081`），不需要先 `yarn build`。

## 开源协议

KiriVers 服务端（含管理后台的前端）使用 GPLv3 协议开源，客户端 SDK 使用 MIT 协议开源：

**GPLv3 协议**：你可以自由使用、修改和分发 KiriVers 服务端，但是所有的修改和衍生作品必须在同样的 GPLv3 协议下发布，并且必须 **提供源代码或提供公开获取源代码的方式**（简单来说必须开源）。

**MIT 协议**：你可以自由使用、修改和分发客户端 SDK，并且可以用于商业用途，你不需要开源自己对客户端 SDK 做的任何修改。

你可以：

- 自由使用、修改和分发客户端 SDK，也可以用于商业用途。
- 自由使用、修改和分发 KiriVers 服务端，也可以用于商业用途。

限制内容：

在分发 KiriVers 服务端时，必须遵守 GPLv3 协议的条款：

- 提供你修改后的源代码或提供公开获取源代码的方式。
- 保留原作者的版权声明和许可证信息。
- 在修改后的版本中注明修改内容。

---

使用规范：

- 禁止违反法律法规的用途。
- 禁止侵犯他人知识产权的用途。
- 禁止传播恶意软件、病毒或其他有害程序的用途。
- 禁止用于任何其他非法或不道德的用途。

**你必须遵守适用的法律法规，并自行承担使用 KiriVers 服务端和客户端 SDK 的风险，所有责任由你自行承担。**
