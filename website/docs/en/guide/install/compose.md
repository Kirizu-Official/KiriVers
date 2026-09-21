---
title: "Docker Compose"
description: "Copy the deploy/ folder, edit one .env, and start the official image plus PostgreSQL and Redis."
---

# Docker Compose

[`deploy/`](https://github.com/Kirizu-Official/KiriVers/tree/main/deploy) is the operator one-click pack: official image + PostgreSQL + Redis. Copy the whole folder to a server; you do not need other files from the repo.

::: info Not the developer file
`dev/docker/compose.yml` starts Postgres/Redis for local `go run .` only. It does **not** run KiriVers. `deploy/` is the only operator one-click Compose pack in the repo (`dev/build/` holds no Compose file any more — just the runtime `Dockerfile` and the release CLI).
:::

## Start

```bash
cd deploy
cp .env.example .env
# set POSTGRES_PASSWORD, REDIS_PASSWORD, URL_SIGNING_SECRET
docker compose up -d
```

`.env` is the only password file. The same values feed:

| In `.env` | Applied to |
|-----------|------------|
| `POSTGRES_*` | the PostgreSQL container and `KIRIVERS_POSTGRES_DSN` |
| `REDIS_PASSWORD` | Redis `requirepass` and `KIRIVERS_CACHE_REDIS_PASSWORD` |
| `URL_SIGNING_SECRET` | `KIRIVERS_URL_SIGNING_SECRET` |

`deploy/config/` already has the three YAML files. **Do not** copy `configs/*-example.yaml` again. `CHANGE_ME` in YAML is overridden by `.env`; editing database passwords in YAML has no effect.

The process uses `KIRIVERS_CACHE_DRIVER=redis` at `redis:6379`. Expect `postgres`, `redis`, and `kirivers` in `docker compose ps`. Open `http://127.0.0.1:8081`.

Data lives in `deploy/data/` (Postgres, Redis AOF, packages, logs). Only `8080` and `8081` are published.

::: danger Sample passwords
`CHANGE_ME` in `.env.example` is not a production secret.
:::

## Which image tag

`deploy/compose.yml` pins `kirizuofficial/kirivers:latest`, i.e. the **most recent release**. To stay on a specific version, change the `image:` line:

| Tag | Meaning |
|-----|---------|
| `kirizuofficial/kirivers:latest` | the latest release |
| `kirizuofficial/kirivers:<semver>` | that version's multi-arch manifest (the host architecture picks its own image) |
| `kirizuofficial/kirivers:<semver>_amd64` / `<semver>_arm64` | that version's single-architecture image; use only when you deliberately lock an architecture |

Official images exist for `linux/amd64` and `linux/arm64` only (Alpine base, musl build). Other Linux architectures have no image — use a [Releases zip archive](/en/guide/install/manual) instead. Before upgrading, back up PostgreSQL and `storage.local.root`, then `docker compose pull && docker compose up -d`.

Next: [bootstrap admin](/en/guide/config/bootstrap-admin).
