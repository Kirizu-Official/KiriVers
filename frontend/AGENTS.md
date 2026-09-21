# Project Rules

## 通用规则

- 遵循现有的代码风格与设计模式。
- 使用 yarn 运行项目相关命令。
- 除非需要进行代码迁移，否则一律保持使用 TypeScript。
- **不要**启动浏览器截图。

## 技术栈

- 框架：Vue 3 + Vite
- UI 库：Vuetify
- 启用特性：Vuetify MCP、Pinia、Vue I18n、ESLint、文件路由（File Router）、Tailwind CSS

## 代码规范

- 使用 ESLint 进行代码规范检查，确保代码风格一致。
- 使用 Prettier 进行代码格式化，确保代码风格一致。
- 符合Google Material Design 3规范，提供一致的用户体验。
- 使用 Vuetify 提供的组件、样式和工具，尽可能少的手写组件、样式、工具。

## Api 规范

- 使用 `@hey-api/openapi-ts` + `@hey-api/client-axios` 调用后端。管理平面 SDK 在 `src/api/generated/`，客户端平面 SDK 在 `src/api/generated-client/`。两份客户端共用 `src/api/client.ts` 里配置的 Axios 实例（拦截器、`ApiError`、`registerAuthHooks`）。
- 页面、store、composable 直接调用生成方法并导入生成类型。列表信封在调用点解包（`data?.projects ?? []`）。不要手写 `endpoints/`、`paths.ts`、领域 `types.ts` 或 `rawRequest`。
- 不要手改 `src/api/generated/`、`src/api/generated-client/`。平面契约 `internal/controller/openapi.admin.json` / `openapi.client.json` 是唯一源，handler JSON 漂移时改对应平面文件。
- 同步契约：先改本平面 OpenAPI 并保证字段与 Gin handler JSON 一致，再 `go test ./internal/controller -run "TestOpenAPIRoutesSync|TestPlaneSpecsValid"`，然后在 `frontend/` 执行 `yarn generate:api`（`scripts/fetch-openapi.mjs` 注入 operationId 后交给 openapi-ts）。
- 客户端预览（check / integrity）只从 `@/api/generated-client` 导入。产物 `direct_s3` 恒为 false，禁止对未知 S3 键做裸 PUT。Vite 必须保持 `/api/v1/projects` 代理规则在 `/api` 之前。
