---
title: "缓存与 Redis"
description: "默认 memory。driver=redis 启动 fail-closed Ping；运行中故障回退内存并重连。"
---

# 缓存与 Redis

热路径（Resolve / LoadCatalog）缓存。目录快照含签名材料，生产 Redis **必须设密码**，禁止与不可信租户共享。

| 参数 | 类型 | 默认 | 说明 | 何时改 |
|------|------|------|------|--------|
| `cache.driver` | `memory` \| `redis` | `memory` | 进程内或 Redis | 多进程或要求跨进程缓存时改 `redis` |
| `cache.redis.addr` | string | `127.0.0.1:6379` | Redis 地址 | `driver=redis` 时必填；连 Docker / 远程实例时改 |
| `cache.redis.password` | string | `""` | 生产必填；`KIRIVERS_CACHE_REDIS_PASSWORD` | 生产必须设；目录快照含签名材料 |
| `cache.redis.db` | int | `0` | Redis 逻辑库 | 与其它应用共用实例时隔离库号 |
| `cache.redis.reconnect_interval_minutes` | int | `10` | 运行中回退后的重连间隔；`<1` 按 1 分钟 | 缩短以更快切回 Redis；拉长以减少抖动 |

## 启动 vs 运行时

| 阶段 | `memory` | `redis` |
|------|----------|---------|
| 启动 | 不连 Redis | **必须 Ping 成功**，否则非 0 退出、不监听（与 Postgres 宕机仍可能监听不同：Postgres 连不上是 fatal；Redis 仅在 driver=redis 时 fail-closed） |
| 运行中断开 | 不适用 | 回退进程内内存，按间隔重连；成功后清空 `kirivers:` 前缀再切回 |

`GET /api/v1/health` 的 `ready` **不含** Redis。Redis 运行时回退不会把 health 打成未就绪。

单机安装可跳过 Redis。多 KiriVers 进程必须 Redis，且 [cluster.download](./cluster) 仅在 `storage.driver=s3` **且** `cache.driver=redis` 时生效。

<<< @/../../configs/config-example.yaml{104-110}
