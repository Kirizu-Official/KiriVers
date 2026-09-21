---
title: "客户端访问配置"
description: "client.yaml 默认 :8080；TLS 两文件同时非空才启用；空 trusted_proxies 不信任 X-Forwarded-For。"
---

# 客户端访问配置

文件：`client.yaml`。环境变量前缀 `KIRIVERS_CLIENT_`。

| 参数 | 类型 | 默认 | 说明 | 何时改 |
|------|------|------|------|--------|
| `addr` | string | `:8080` | 监听地址 | 与反代或冲突端口 |
| `mode` | string | 示例 `debug` | 与管理平面一起决定进程 Gin mode | 生产用 `release`（两平面都不要 debug） |
| `tls_cert` / `tls_key` | path | `""` | **同时非空**才 HTTPS；无进程级 TLS 回退 | 平面自己终止 TLS 时 |
| `trusted_proxies` | string[] | `[]` | 空 = 不信任 `X-Forwarded-For`，ClientIP 为直连地址 | 前面有反代时填反代 IP/CIDR |
| `log.system` / `log.access` | object | 见示例 | 控制台 + 滚动文件 | 至少开一个 sink |

```yaml
addr: ":8080"
mode: release
tls_cert: ""
tls_key: ""
trusted_proxies: []
```

公网通常只暴露本平面。管理面见 [admin.yaml](./admin.yaml) 与 [反向代理](./reverse-proxy)。
