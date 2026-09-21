---
title: "Manual install"
description: "Install PostgreSQL on Windows, macOS and Linux, download the matching Release archive by prefix, configure the three YAML files, start on boot."
---

# Manual install

This section goes from "PostgreSQL is installed (Redis optional)" to "the process is listening and the browser can open the admin console". Official Releases **do not** require copying `frontend/dist`.

::: warning Binary source
Open <https://github.com/Kirizu-Official/KiriVers/releases> and download the archive matching your **prefix** as listed on [Choose an install path](/en/guide/install/#releases-files) — for example `KiriVers-Windows-x86_64-<hash>.zip`, `KiriVers-Linux-arm64-<hash>.zip`. Extracting gives `kirivers-<os>-<arch>` (`.exe` on Windows). Follow the current Release and do not use guessed direct links: the hash at the end of the filename changes every release.
:::

## Windows

1. Install the [PostgreSQL Windows installer](https://www.postgresql.org/download/windows/). Record the superuser password and port you chose (default `5432`). Create a database and a role, for example database `kirivers` and user `kirivers`.
2. Redis: on a single machine you can skip it and use `cache.driver=memory`. If you do need Redis, install a Windows Redis build or run only Redis in Docker.
3. Download the matching prefix from Releases (`KiriVers-Windows-x86_64-`, `-x86-` or `-arm64-`) and put the extracted `.exe` in a stable directory, for example `C:\KiriVers\`.
4. Copy the repository's `configs/config-example.yaml`, `admin-example.yaml` and `client-example.yaml` into that same directory, renaming them `config.yaml`, `admin.yaml` and `client.yaml`. Edit the keys that **must** change before it will start — Notepad or VS Code is fine:
   - `postgres.dsn`: host, user, password, database, port
   - `url_signing_secret` in production (otherwise signed download URLs die after a restart)
   Remaining keys: [Configuration](/en/guide/config/).
5. Open a terminal in that directory:

```bat
kirivers.exe
```

No arguments equals `kirivers server`. Expect the process to stay running and the logs to show the client `:8080` and admin `:8081` listeners.

6. Open `http://127.0.0.1:8081` in a browser. You should see the login page (“Sign in to KiriVers”). If you only see JSON: check whether `static_dir` in `admin.yaml` is the empty string (that **disables the UI**), and whether you are running a self-built binary produced without building the admin console. Official Releases already embed the console.

::: details :8081 shows JSON only
`static_dir: ""` disables the UI even when the binary embeds `index.html`. A self-built binary that never ran `yarn build` in `frontend/` may have no `index.html` in the embedded FS. Official Releases do **not** require copying `frontend/dist`.
:::

7. Start on boot: Task Scheduler → Create Basic Task → trigger "When the computer starts" → action "Start a program" pointing at `kirivers.exe`, started in the directory holding the configuration files.

## macOS

PostgreSQL: Homebrew `brew install postgresql@17` or [Postgres.app](https://postgresapp.com/). Redis optional with `brew install redis`. Download the `darwin` archive from Releases (prefix `KiriVers-macOS-`), copy the three YAML files, run `./kirivers` in a terminal. Open `http://127.0.0.1:8081`.

## Linux (when you did not use the one-click script)

::: code-group

```bash [apt]
sudo apt-get update
sudo apt-get install -y postgresql postgresql-contrib
```

```bash [dnf]
sudo dnf install -y postgresql-server postgresql
sudo postgresql-setup --initdb
sudo systemctl enable --now postgresql
```

:::

Download the Linux executable, copy the three YAML files, run `./kirivers`.

### systemd example

Replace `User`, `WorkingDirectory` and `ExecStart` with your real paths:

```ini
[Unit]
Description=KiriVers
After=network.target postgresql.service

[Service]
Type=simple
User=kirivers
WorkingDirectory=/opt/kirivers
ExecStart=/opt/kirivers/kirivers -config /opt/kirivers
Restart=on-failure

[Install]
WantedBy=multi-user.target
```

- `WorkingDirectory`: the directory holding the three YAML files (or point `-config` at it).
- `ExecStart`: the binary path. No extra arguments starts both planes.
- `After=postgresql.service`: best-effort wait for the database; PostgreSQL must be reachable at startup or the process fatals.

Enable with `sudo systemctl enable --now kirivers`.

Next: [bootstrap the first admin](/en/guide/config/bootstrap-admin).
