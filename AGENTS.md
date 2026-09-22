<!-- TRELLIS:START -->
# Trellis Instructions

These instructions are for AI assistants working in this project.

This project is managed by Trellis. The working knowledge you need lives under `.trellis/`:

- `.trellis/workflow.md` — development phases, when to create tasks, skill routing
- `.trellis/spec/` — package- and layer-scoped coding guidelines (read before writing code in a given layer)
- `.trellis/workspace/` — per-developer journals and session traces
- `.trellis/tasks/` — active and archived tasks (PRDs, research, jsonl context)

If a Trellis command is available on your platform (e.g. `/trellis:finish-work`, `/trellis:continue`), prefer it over manual steps. Not every platform exposes every command.

If you're using Codex or another agent-capable tool, additional project-scoped helpers may live in:
- `.agents/skills/` — reusable Trellis skills
- `.codex/agents/` — optional custom subagents

Managed by Trellis. Edits outside this block are preserved; edits inside may be overwritten by a future `trellis update`.

<!-- TRELLIS:END -->

# KiriVers
KiriVers 是一个版本控制系统，主要用于管理二进制软件项目的版本（也可用于开源客户端代码分发）。它是一个简单易用的版本控制系统，支持版本控制、版本管理、版本发布、版本回滚等功能。

## 技术栈
* **编程语言**：Go
* **Web 框架**：Gin Web Framework (`github.com/gin-gonic/gin`)
* **ORM 框架**：GORM (`gorm.io/gorm` + `gorm.io/driver/postgres`)
* **数据库**：PostgreSQL (`>= 14`)，支持 JSONB、GIN 索引与 UUID 扩展
* **日志系统**：Zerolog (`github.com/rs/zerolog`)，全局结构化链路跟踪
* **版本解析**：Masterminds SemVer (`github.com/Masterminds/semver/v3`)
* **对象存储**：适配器模式抽象（支持本地磁盘 LocalFS 与 S3 兼容协议，如 MinIO/Cloudflare R2/AWS S3）
* **官方客户端 SDK**：完整包根在孤儿分支 `sdk/<lang>`（独立 worktree，不要 checkout 盖住默认分支）。Go / PHP / Swift 本仓 `sdk/go`、`sdk/php`、`sdk/swift` 只是 README 跳转，源码在 `sdk/go-src` 等，对外发布仓见 `docs/sdk-publish.md`。不要把语言包源码、SDK 用的 `openapi.client.json` 快照或 `sdk/` 目录放进默认分支。
* **服务端差量**：官方服务端 `CGO_ENABLED=1` 链接 HDiffPatch（未压缩 HDIFF13）。Go SDK 仍 `CGO_ENABLED=0`。

## 推荐项目工程结构
```text
├── main.go                         # 单一启动入口：委托 cmd.Run()；无参数默认启动 server；`admin` 子命令管理本机管理员账号
├── cmd/                            # server / admin / listen（package cmd）
├── docs/                           # 内部说明（app-init / client-sdk / app-docs）；对外站点在 website/
├── website/                        # 官方 VitePress 文档（默认中文 + docs/en）
├── scripts/install-deps.sh         # Linux 装 PostgreSQL（可选 Redis）；不下载 KiriVers 二进制
├── deploy/                         # 使用者一键 Compose（官方镜像 + Postgres + Redis；口令只写 .env）；仓库唯一跑 KiriVers 进程的 Compose 示例
├── dev/build/                      # Alpine 运行镜像 Dockerfile 与发版 CLI（kirivers.py + kirivers_build/，纯标准库 Python 子命令；GitHub Actions 只做编排，判定都在 CLI 里）；不放 Compose，也没有 sh 脚本
├── .github/workflows/              # ci.yml（PR 与 main 的检查）、pr-guard.yml、gosec-scan.yml + security-gate.yml（wait-merge 沙箱扫描与 ready-merge 状态机）、docs-pages.yml（website/ 编译并发布 gh-pages）、release.yml（只手动 workflow_dispatch；合入 main 绝不发布）、sdk-automerge.yml（只手动；批量合并 sdk/* PR 并推 registry）
├── CHANGELOG.md                    # 发版作业写回的更新日志（首次发版后出现），贡献者不要手写
├── third_party/hdiffpatch/         # 钉版本裁剪的 libHDiffPatch（未压缩 HDIFF13；无 CLI/压缩插件）
├── configs/
│   ├── config.yaml                 # 本地系统配置（gitignore，含密钥）
│   ├── admin.yaml                  # 本地管理平面配置（gitignore）
│   ├── client.yaml                 # 本地客户端平面配置（gitignore）
│   ├── config-example.yaml         # 系统配置模板
│   ├── admin-example.yaml          # 管理平面模板（addr / TLS / 日志）
│   └── client-example.yaml         # 客户端平面模板（addr / TLS / 日志）
├── logs/                           # 滚动 JSON 日志（gitignore；示例默认 ./logs）
├── internal/
│   ├── config/                     # 配置加载 (Viper)
│   ├── buildinfo/                  # 编译信息 (-X 注入版本/commit/构建时间；CGO 走构建标签)
│   ├── database/                   # 数据库连接与初始化
│   ├── logger/                     # 日志记录与配置
│   ├── controller/                 # HTTP 接口接入层 (Gin Handler)
│   │   ├── admin/                  # 管理平台接口
│   │   └── client/                 # 客户端更新查询与校验接口
│   ├── middleware/                 # 中间件 (Zerolog, Recovery, Auth, RateLimit)
│   ├── model/                      # GORM 实体定义与数据库迁移模型
│   ├── platform/                   # 预置 OS/Arch 与别名解析
│   ├── cache/                      # 进程缓存（memory / Redis；Open(Options)，不读环境变量）
│   ├── repository/                 # 数据持久层
│   ├── service/                    # 核心业务逻辑 (更新计算、差分算法、校验哈希)
│   ├── delta/                      # 单文件差量引擎；hdiffpatch 需 CGO + C++ 工具链
│   └── storage/                    # 存储层抽象 (Local/S3/MinIO)
├── pkg/
│   ├── hashutil/                   # SHA256/MD5 流式计算工具
│   ├── response/                   # 统一 HTTP JSON 响应封装
│   └── semver/                     # 语义化版本比对扩展
├── go.mod
└── go.sum
```

## 代码要求

* 关键函数、变量、算法等地方需要添加注释说明代码的用途、功能、使用方法等，对于GORM实体必须添加详细的注释，说明实体的用途、字段含义、关系等。
* 做好代码的性能优化，避免不必要的内存分配和拷贝，使用合适的算法和数据结构，避免使用低效的算法和数据结构。
* 做好代码的并发安全，避免使用全局变量和共享资源，使用合适的锁和同步机制，避免使用死锁和竞态条件。
* 做好代码的测试，使用合适的测试框架和测试工具，编写全面的测试用例，覆盖所有功能和边界情况。
* 使用合适的代码规范和代码风格，避免使用不规范的代码风格和代码规范。做好基于模块的分层/分文件，不要将大量代码都放在一个文件中或一个函数中。


## 本地测试

本地已有测试服务器，其docker文件存放于 dev/docker/ 目录下，其中compose.yml中的数据库信息（包括密码）可以直接使用以连接服务器，如果服务器未运行可使用docker-compose up -d 命令启动服务器。

服务端 `go test ./internal/delta/...` 与 `go build .` 需要本机 C++ 工具链（Windows：MinGW `g++`；macOS：Xcode CLT `clang++`；Linux：`g++`/`clang++`）且 `CGO_ENABLED=1`。`CGO_ENABLED=0 go build .` 必须失败。darwin 官方产物须在 macOS 宿主上编，不要从 Linux 无 Apple SDK 交叉。

## 前端
前端位于 frontend/ 目录下，如需修改前端请阅读 frontend/AGENTS.md 中的内容。`go build` / `go run .` 会通过 `frontend/embed.go` 把 `frontend/dist` 打进二进制；发布前先 `yarn build` 再编译 Go。

## 官方文档
对外文档在 `website/`（`cd website && yarn docs:dev` / `yarn docs:build`）。根目录 `docs/` 不是 VitePress 内容根。不要把文档站挂到管理平面 `GET /`，也不要把 `admin.yaml` `static_dir` 指到 VitePress dist。
