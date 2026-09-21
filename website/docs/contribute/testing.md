---
title: "测试"
description: "go test ./...。关键 OpenAPI 同步测试。前端 eslint。契约测试不替你启动 Docker。"
---

# 测试

```bash
CGO_ENABLED=1 go test ./...
go test ./internal/controller -run "TestOpenAPIRoutesSync|TestPlaneSpecsValid"
cd frontend && yarn lint
```

服务端测试与官方二进制需要 `CGO_ENABLED=1` 与 C++ 工具链。契约测试使用假 Transport / 夹具，不替你启动 Docker。`dev/docker` 只给本机进程提供 Postgres/Redis。SDK 语言包在独立分支上自测，不在默认分支。PR 流水线见 [发版与 Docker](./release)。
