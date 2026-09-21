---
title: "Configuration"
description: "Three YAML files in one directory: config.yaml, admin.yaml, client.yaml. Env prefixes KIRIVERS_ / KIRIVERS_ADMIN_ / KIRIVERS_CLIENT_."
---

# Configuration

Keep three files in **one folder**:

```text
config-dir/
  config.yaml     # system: database, storage, cache, jobs, security
  admin.yaml      # admin plane: addr, TLS, trusted_proxies, static_dir
  client.yaml     # client plane: addr, TLS, trusted_proxies
```

`-config` points at that directory or the system file. Missing files load sibling `*-example.yaml`.

| File | Env prefix | Example |
|------|------------|---------|
| `config.yaml` | `KIRIVERS_` | `postgres.dsn` → `KIRIVERS_POSTGRES_DSN` |
| `admin.yaml` | `KIRIVERS_ADMIN_` | `addr` → `KIRIVERS_ADMIN_ADDR` |
| `client.yaml` | `KIRIVERS_CLIENT_` | `addr` → `KIRIVERS_CLIENT_ADDR` |

`trusted_proxies` and `static_dir` are **not** in `config.yaml`. They belong to the plane YAML files.

If either plane sets `mode=debug`, the whole process uses Gin debug mode; otherwise release.

## Logging

One process system stream plus system+access on each plane (five streams). Each stream must enable console or rolling file (or both). If both sinks are off, startup fails and the process does not listen.

Use environment variables to inject secrets. Treat the three YAML files as the operator-facing source. Child pages:

- [Bootstrap admin](./bootstrap-admin)
- [client.yaml](./client.yaml)
- [config.yaml](./config.yaml)
- [Cache](./cache)
- [S3](./s3)
- [Local disk](./local-storage)
- [admin.yaml](./admin.yaml)
- [Reverse proxy](./reverse-proxy)
- [Cluster YAML](./cluster)
- [Security](./security)
