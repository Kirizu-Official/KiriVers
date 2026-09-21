---
title: "Local development"
description: "dev/docker for Postgres/Redis. go run .. UI loop is yarn dev :3000. Vite proxy order."
---

# Local development

```bash
docker compose -f dev/docker/compose.yml up -d
cp configs/config-example.yaml configs/config.yaml
cp configs/admin-example.yaml configs/admin.yaml
cp configs/client-example.yaml configs/client.yaml
go run .
```

API ports: `:8080` / `:8081`. `dev/docker/compose.yml` does **not** run KiriVers; it is only databases for local `go run .`. To run the full stack in Docker (Hub image + PostgreSQL + Redis) use the operator one-click pack `deploy/`; `dev/build/` keeps only the runtime `Dockerfile` and the release CLI, no Compose file.

## One-file Docker run (Hub image + PostgreSQL + Redis)

For a full local stack use `deploy/` (details: [Docker Compose](/en/guide/install/compose)). Passwords live only in `deploy/.env` (from `.env.example`) and feed Postgres, `redis-server --requirepass`, `KIRIVERS_POSTGRES_DSN`, `KIRIVERS_CACHE_REDIS_*`, and `KIRIVERS_URL_SIGNING_SECRET`. Cache driver is Redis (`KIRIVERS_CACHE_DRIVER=redis`).

```bash
cd deploy
cp .env.example .env
docker compose up -d
```

Persist `deploy/config/` at `/config`; `deploy/data/` for objects, logs and the datastores. `deploy/` publishes only `8080` / `8081` on the host (it is `dev/docker/` that maps 5432/6379), so the two never collide. Do not add a root `compose.yml`. Building the image from source is in [Release and Docker](./release).

Without Docker, keep using `go run .` above (`CGO_ENABLED=1` and a C++ toolchain). Official binaries and images are always CGO; the Releases page ships `KiriVers-<OS>-<Arch>-<sha6>.zip`, and the thing you run after unpacking is `kirivers-<os>-<arch>` (`.exe` on Windows). See [Release and Docker](/en/contribute/release).

## Admin UI

Required:

```bash
cd frontend
yarn
yarn dev
```

Open `http://localhost:3000`. Vite proxies to the real backend. Use this path for daily UI work; `yarn build` then `:8081` is only for verifying production embed.

Vite proxy: `/api/v1/projects` **must precede** `/api` (`:8080` / `:8081`).

Verify production embed: `yarn build` then `go build`. Disk `static_dir` with `index.html` wins on :8081; otherwise embed. `static_dir: ""` disables the UI.

## Checks before you open a PR

The pipeline logic lives in `dev/build/kirivers.py` + `dev/build/kirivers_build/` (standard-library Python, nothing to install), and each gate has an offline self-check. Use `python3` on Linux / macOS and `python` on Windows:

```bash
python3 dev/build/kirivers.py check-workflows              # workflow static gate
python3 dev/build/kirivers.py release-notes --self-check   # changelog format + type-table invariant
python3 dev/build/kirivers.py guard --self-check           # guard decision matrix
python3 dev/build/kirivers.py issue-link --self-check
python3 dev/build/kirivers.py asset-names                  # the 10 archive names and inner binary names
```

- The version is derived only from Conventional Commits; the root `CHANGELOG.md` is written back by the release job — **never hand-edit** it (format and customization: [Release and Docker](/en/contribute/release#changelog-format)).
- To inspect or edit an `sdk/<lang>` branch: `git worktree add .worktrees/<lang> sdk/<lang>`. `.worktrees/` is gitignored; do not check a branch out over the default-branch worktree.
