# KiriVers 管理控制台前端

Vue 3 + Vuetify 4 (MD3) + Pinia + vue-i18n + 文件路由 + Tailwind CSS 4 + TypeScript。
包管理器：**yarn**（Node ≥ 20）。

开发与构建都直连真实后端双平面（admin `:8081` / client `:8080`）。开发期同源 `/api` 经 Vite 代理分流。

## 快速开始

在仓库根目录启动依赖与后端，再启动前端：

```bash
# 1. PostgreSQL / Redis
docker compose -f dev/docker/compose.yml up -d

# 2. 双平面后端（admin :8081，client :8080）
go run .

# 3. 创建本机管理员账号（另开终端）
go run . admin add <user>

# 4. 前端
cd frontend
yarn install
yarn dev          # http://localhost:3000
```

登录页使用上一步创建的管理员账号。

## 开发代理（双平面）

`vite.config.mts` 按路径把同源 `/api` 分流到两个监听端口。Vite 代理按 key 顺序匹配，**更具体的 `/api/v1/projects` 必须排在通用 `/api` 之前**：

| 前缀 | 目标 | 覆盖环境变量 | 默认 |
| --- | --- | --- | --- |
| `/api/v1/projects` | 客户端平面（check / integrity 等公开端点） | `KIRIVERS_CLIENT_API_URL` | `http://127.0.0.1:8080` |
| `/api` | 管理平面（`/api/v1/admin/**`、探活） | `KIRIVERS_API_URL` | `http://127.0.0.1:8081` |

`yarn dev` 必须按上表分流。生产管理平面会把 `/api/v1/projects/**` 反代到本进程客户端平面，托管后的 SPA 不必再单独分流；预览 404 只会出现在未设置反代（单元测试）或 Vite 代理顺序写反时。

## 生产托管（管理平面静态文件）

`yarn build` 产物默认输出到 `frontend/dist`。`go build` / `go run .` 会把该目录 `go:embed` 进二进制。管理平面（默认 `:8081`）优先托管磁盘上的 `static_dir`（默认 `frontend/dist`，环境变量 `KIRIVERS_ADMIN_STATIC_DIR`）；磁盘缺少 `index.html` 时改用内嵌副本。将 `static_dir` 设为空字符串可显式关闭 UI 托管。生产发布应先 `yarn build` 再编译 Go，否则二进制里只有占位文件、没有真正的管理台。

## 脚本

| 命令 | 说明 |
| --- | --- |
| `yarn dev` | 启动开发服务器（端口 3000，经双平面代理打真实后端） |
| `yarn build` | type-check + vite 生产构建 |
| `yarn lint` / `yarn lint:fix` | ESLint 检查 / 自动修复 |
| `yarn type-check` | vue-tsc 全量类型检查 |
| `yarn generate:api` | 从 `internal/controller/openapi.admin.json` / `openapi.client.json` 注入 operationId 并重新生成 hey-api SDK（admin → `generated/`，client → `generated-client/`） |

## 目录结构

- `src/api/` — API 层：`generated/`（管理平面 hey-api SDK，勿手改）、`generated-client/`（客户端平面 SDK，勿手改）、`client.ts`
  （拦截器、`ApiError`、双 SDK 共用 Axios 实例）
- `src/pages/` — 文件路由页面（`projects/[projectRef]/` 下为项目级页面）
- `src/components/` — 通用组件（对话框、状态芯片、LocaleTabs 等）
- `src/composables/` — `useApiResource`（列表 CRUD 状态机）、`useUpload`
  （direct PUT + SHA-256 / TUS 1.0 分块 / presign）
- `src/stores/` — Pinia：auth（会话 Token）、ui（主题+语言）、jobs（任务轮询）、snackbar
- `src/locales/` — zh-CN / en 文案与错误码映射

## 同步 OpenAPI

路径/请求/响应形状以平面契约为唯一事实：管理台用 `internal/controller/openapi.admin.json`，客户端平面用 `internal/controller/openapi.client.json`。

后端若更新了 OpenAPI，先保证对应平面文件字段与 Gin handler JSON 一致，再
`go test ./internal/controller -run "TestOpenAPIRoutesSync|TestPlaneSpecsValid"`，然后在 `frontend/` 运行 `yarn generate:api`。
不要手改生成目录。页面、store、composable 直接调用生成方法，不要再加 `endpoints/`、`paths.ts` 或手写领域 `types.ts`。
