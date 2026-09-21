---
title: "运维"
description: "GET /api/v1/health 与 ready。GET /api/v1/ready 为 404。运行中 DB。Redis 回退。"
---

# 运维

- `GET /api/v1/health`：恒 200。`ready` = DB + 存储，**不含 Redis**。
- `GET /api/v1/ready`：**404**。不要当就绪探针。
- 运行中 Postgres 不可用：业务 `/api` 404，health 仍 200 `ready=false`。
- Redis：`driver=redis` 时**启动**必须 Ping；运行中断开回退内存并重连。
- 反代必须配置该平面 `trusted_proxies`；空列表不会信任全部 `X-Forwarded-For`。
- 备份见 [备份与升级](/guide/backup)。无 `kirivers listen`。无独立 migrate CLI（AutoMigrate）。
