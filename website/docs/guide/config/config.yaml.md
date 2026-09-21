---
title: "服务端配置"
description: "config.yaml：PostgreSQL DSN、五路日志以外的进程日志、jobs、dynamic_pack、file_list、changelog 窗口、缓存。"
---

# 服务端配置

文件：`config.yaml`。前缀 `KIRIVERS_`。不含监听地址。

## PostgreSQL

| 参数 | 类型 | 说明 | 何时改 |
|------|------|------|--------|
| `postgres.dsn` | string | libpq DSN | 生产必须改密码；可用 `KIRIVERS_POSTGRES_DSN` |

启动时必须能打开数据库，否则进程 fatal。运行中数据库不可用时业务 `/api` 返回 404，`GET /api/v1/health` 仍 200 且 `ready=false`。

## 进程日志 `log`

| 参数 | 类型 | 默认倾向 | 说明 | 何时改 |
|------|------|----------|------|--------|
| `log.level` | string | 示例 `debug`，Viper 默认 `info` | 非法级别回退 info | 生产用 `info`/`warn`；排查再开 `debug` |
| `log.console.enabled` | bool | true | stderr ConsoleWriter | 无终端或只收 journal 时可关 |
| `log.file.enabled` | bool | true | JSON 滚动文件 | 只要 stderr、不要落盘时关 |
| `log.file.dir` | string | `./logs` | 相对进程 CWD | 生产用绝对路径并纳入备份 |

管理/客户端四路日志在平面 YAML 中，见 [配置说明](./)。

## 任务与上限

| 参数 | 类型 | 默认 | 说明 | 何时改 |
|------|------|------|------|--------|
| `jobs.workers` | int | `2` | Job worker 数 | 差量/打包/清理并发 |
| `dynamic_pack.max_bytes` | int | `536870912` | 多文件动态打包未压缩硬顶 | 超限客户端得 200 `full_package` |
| `file_list.max_files` | int | `16` | 原生 file_list 条数天花板；加载后 `<1` 回退 16 | 项目可下调，不得超出 |
| `changelog.default_entries` | int | `5` | 无 `from_version` 时条数；`<1` 回退 5 | 缩短默认窗口；必须 ≤ `max_entries` |
| `changelog.max_entries` | int | `50` | 有 `from_version` 时截断；`<1` 回退 50 | **`default_entries > max_entries` 拒绝启动** |

<<< @/../../configs/config-example.yaml{77-95}

## 其它键

- 缓存：见 [缓存与 Redis](./cache)
- 存储：见 [S3](./s3) 与 [本机磁盘](./local-storage)
- `cluster.download` / `node.display_name`：见 [多节点](./cluster)
- `security.*` / `url_signing_secret`：见 [安全](./security)
