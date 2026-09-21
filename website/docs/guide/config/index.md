---
title: "配置说明"
description: "三份 YAML 同目录：config.yaml、admin.yaml、client.yaml。环境变量前缀 KIRIVERS_ / KIRIVERS_ADMIN_ / KIRIVERS_CLIENT_。"
---

# 配置说明

三份文件必须在**同一文件夹**：

```text
config-dir/
  config.yaml     # 系统：数据库、存储、缓存、任务、安全
  admin.yaml      # 管理平面：addr、TLS、trusted_proxies、static_dir
  client.yaml     # 客户端平面：addr、TLS、trusted_proxies
```

`-config` 指向该目录或其中的系统文件。缺文件时加载同目录 `*-example.yaml`。

| 文件 | 环境变量前缀 | 说明 |
|------|----------------|------|
| `config.yaml` | `KIRIVERS_` | 例：`postgres.dsn` → `KIRIVERS_POSTGRES_DSN` |
| `admin.yaml` | `KIRIVERS_ADMIN_` | 例：`addr` → `KIRIVERS_ADMIN_ADDR` |
| `client.yaml` | `KIRIVERS_CLIENT_` | 例：`addr` → `KIRIVERS_CLIENT_ADDR` |

`trusted_proxies` 与 `static_dir` **不在** `config.yaml`。它们分别属于平面 YAML。

任一平面 `mode=debug` 则整个进程 Gin 为 debug（进程全局）；否则 release。

## 日志

进程有一路系统日志；管理平面与客户端平面各有 system + access，共五路。每一路：控制台与滚动文件至少启用一个。两边都关则启动失败、不监听。

环境变量覆盖适合部署注入密钥；日常把三份 YAML 当权威。子页：

- [管理员初始化](./bootstrap-admin)
- [client.yaml](./client.yaml)
- [config.yaml](./config.yaml)
- [缓存](./cache)
- [S3](./s3)
- [本机磁盘](./local-storage)
- [admin.yaml](./admin.yaml)
- [反向代理](./reverse-proxy)
- [多节点 YAML](./cluster)
- [安全](./security)
