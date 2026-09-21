---
title: "Docker"
description: "Run KiriVers from the multi-arch image kirizuofficial/kirivers against an external PostgreSQL; amd64 and arm64 only."
---

# Docker

::: warning Image name
The published name is **`kirizuofficial/kirivers`**. Which tag to pull follows the current Docker Hub and GitHub Release. Official images already embed the admin console; mounting or copying `frontend/dist` is unnecessary.
:::

| Tag | Meaning |
|-----|---------|
| `latest` | points at the most recent release |
| `<semver>` (e.g. `0.3.0`, no `v` prefix) | that version's multi-arch manifest; `docker pull` picks your architecture |
| `<semver>_amd64` / `<semver>_arm64` | that version's single-architecture images; use them only when you pin an architecture |

The image supports `linux/amd64` and `linux/arm64` (Alpine base, musl builds). Other Linux architectures have no official image — use the [glibc Release archives](/en/guide/install/#releases-files) or build from source.

The image is the published unit; the container is the running unit.

PostgreSQL must already exist (another container, a hosted database, or the host). This `docker run` does not start a database. To bring up the database in one stack, use [Compose](/en/guide/install/compose).

## Run

Mount a host config directory into the container (it must contain `config.yaml`, `admin.yaml`, `client.yaml`) and publish both planes' ports:

```bash
docker run --name kirivers --restart unless-stopped \
  -p 8080:8080 -p 8081:8081 \
  -v /var/kirivers/config:/config \
  -e KIRIVERS_CONFIG=/config \
  -e KIRIVERS_POSTGRES_DSN='host=host.docker.internal user=kirivers password=CHANGE_ME dbname=kirivers port=5432 sslmode=disable' \
  kirizuofficial/kirivers
```

Expect the container to stay running and `docker logs kirivers` to show the `:8080` / `:8081` listeners. Open `http://127.0.0.1:8081` on the host.

The official image includes the admin console. `static_dir` in `admin.yaml` points at the console's static files; the VitePress docs `dist` is not the console. `static_dir: ""` disables the UI.

::: details Container is up but the browser has no login page
Confirm port `8081` is published and `static_dir` in the container's `admin.yaml` is not the empty string. The official image already embeds the UI; do not mount a self-built `frontend/dist` on top of it.
:::

## Logs, upgrade, data

| Task | Action |
|------|--------|
| Logs | `docker logs -f kirivers`; optionally bind-mount `log.file.dir` to a volume |
| Upgrade image | `docker pull kirizuofficial/kirivers` then recreate the container; back up PostgreSQL and `storage.local.root` first |
| Data | Database on the PostgreSQL volume; packages on object storage or a `storage.local.root` volume |

Next: [Compose](/en/guide/install/compose) or [bootstrap the first admin](/en/guide/config/bootstrap-admin).
