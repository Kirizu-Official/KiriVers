---
title: "反向代理与 TLS"
description: "公网只暴露客户端平面；管理面绑定本机或内网。证书可在平面 YAML 或由反代终止。"
---

# 反向代理与 TLS

建议：公网只反代客户端平面（`:8080`）。管理平面绑定 `127.0.0.1` 或仅内网，再经 VPN / SSH / 内网反代访问。

TLS 有两种做法（可混用两个平面）：

1. 平面 YAML 的 `tls_cert` + `tls_key`（两行同时非空）。
2. 反代终止 TLS，后端明文；此时必须配置该平面 `trusted_proxies`，否则 `X-Forwarded-For` 不会被信任。

空 `trusted_proxies` 调用 `SetTrustedProxies(nil)`，**不会**走 Gin 默认「信任全部」。

生产管理平面会把 `/api/v1/projects/**` 反代到本进程客户端平面，供控制台预览。管理平面 `GET /` 托管管理台 SPA；文档站应单独部署，否则与 SPA 抢路由。
