---
title: "管理台前端"
description: "yarn dev 日常调试。yarn build 写入 dist 由 embed.go 打进二进制。生产会反代 projects。"
---

# 管理台前端

Vue 3 + Vite + Vuetify + Pinia + 文件路由。两份 hey-api 客户端：`generated/` 管理平面，`generated-client/` 客户端平面。禁止手改 generated。列表信封在调用点解包（`data?.projects ?? []`）。

日常调试：**`yarn dev`**（:3000）。生产：`yarn build` 写入 `frontend/dist`，由 `frontend/embed.go` 打进二进制。

Vite 开发代理（`frontend/vite.config.mts`）：`/api/v1/projects` 必须排在 `/api` 之前，分别转到 `:8080` / `:8081`。可用 `KIRIVERS_CLIENT_API_URL` 与 `KIRIVERS_API_URL` 覆盖。

生产进程**会**反代 `/api/v1/projects/**`。不要写「管理面不转发客户端路径」。
