---
title: "Runtime architecture"
description: "One process, two listeners. Disk static wins over embed. Admin proxies /api/v1/projects. Probe /api/v1/health."
---

# Runtime architecture

`main.go` → `cmd.Run()`. Default `server`: one process, two listeners. Boot: three YAML files → logger → postgres → storage → cache → dual Gin → job workers.

- Client plane default `:8080` (updates / store / telemetry).
- Admin plane default `:8081` (console + CI agent). `admin.enabled=false` skips the **admin listener only**.
- Admin **reverse-proxies** `/api/v1/projects/**` to this process’s client plane (empty `ClientProxyURL` is tests only).

Admin UI: `NoRoute`, never `gin.Static`. `frontend/embed.go` is `//go:embed all:dist`. Priority: non-empty `static_dir` with disk `index.html` → disk; else embedded FS with `index.html` → embed; `static_dir: ""` → UI off.

Probe `/api/v1/health`; `/api/v1/ready` is unregistered. `ready` excludes Redis. Cache: fail-closed at start, runtime fallback. Five log streams; both sinks off refuses listen. `ApplyTrustedProxies`: empty list `SetTrustedProxies(nil)`.

```mermaid
flowchart TB
  subgraph proc [kirivers process]
    C[Gin client :8080]
    A[Gin admin :8081]
    S[service]
    R[repository]
    DB[(PostgreSQL)]
    ST[storage]
    CA[cache memory/Redis]
    J[job workers]
    A -->|proxy /api/v1/projects| C
    C --> S
    A --> S
    S --> R
    R --> DB
    S --> ST
    S --> CA
    J --> S
  end
  UI[Admin SPA disk or embed]
  A -->|NoRoute static| UI
```
