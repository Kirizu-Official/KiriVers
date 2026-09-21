---
title: "Testing"
description: "go test ./.... OpenAPI sync tests. Frontend eslint. Contract tests do not start Docker for you."
---

# Testing

```bash
CGO_ENABLED=1 go test ./...
go test ./internal/controller -run "TestOpenAPIRoutesSync|TestPlaneSpecsValid"
cd frontend && yarn lint
```

Server tests and official binaries need `CGO_ENABLED=1` and a C++ toolchain. Contract tests use fake Transports / fixtures and do not start Docker. `dev/docker` only provides Postgres/Redis for the host process. Language SDKs test on their own branches, not the default branch. PR pipelines: [Release and Docker](./release).
