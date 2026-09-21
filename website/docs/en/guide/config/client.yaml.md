---
title: "client.yaml"
description: "client.yaml defaults to :8080; TLS requires both files; empty trusted_proxies does not trust X-Forwarded-For."
---

# client.yaml

File: `client.yaml`. Env prefix `KIRIVERS_CLIENT_`.

| Key | Type | Default | Meaning | When to change |
|-----|------|---------|---------|----------------|
| `addr` | string | `:8080` | Listen address | Reverse proxy or port conflict |
| `mode` | string | example `debug` | Combined with admin plane for process Gin mode | Production: `release` on both planes |
| `tls_cert` / `tls_key` | path | `""` | HTTPS only when **both** are non-empty | Plane-terminated TLS |
| `trusted_proxies` | string[] | `[]` | Empty = do not trust `X-Forwarded-For` | Fill proxy IP/CIDR when a reverse proxy is in front |
| `log.system` / `log.access` | object | see example | Console + rolling file | At least one sink per stream |

```yaml
addr: ":8080"
mode: release
tls_cert: ""
tls_key: ""
trusted_proxies: []
```

Expose this plane on the public network. Keep the admin plane on [admin.yaml](./admin.yaml) and [reverse proxy](./reverse-proxy).
