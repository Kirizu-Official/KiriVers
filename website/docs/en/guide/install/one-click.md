---
title: "One-click dependencies"
description: "Linux script installs PostgreSQL via apt or yum/dnf and optionally Redis. It does not download the KiriVers binary."
---

# One-click dependencies

[`scripts/install-deps.sh`](https://github.com/Kirizu-Official/KiriVers/blob/main/scripts/install-deps.sh) installs PostgreSQL on Linux with **apt or yum/dnf**, and asks whether to install **Redis** from the same package manager.

## What it does

| Does | Does not |
|------|----------|
| Install PostgreSQL as root/sudo and try to enable the service | Download any KiriVers package |
| Optionally install Redis after a prompt | Call Docker |
| Print the next step: open the manual install page, then get the package for your platform from GitHub Releases (`KiriVers-<OS>-<Arch>-<sha6>.zip`) | Support Windows |

::: warning Windows
Do not run this script on Windows. Use [manual install](/en/guide/install/manual).
:::

## Redis choice

- **Skip**: single node, modest traffic. Keep `cache.driver=memory` (default).
- **Install**: multiple KiriVers processes, or you will set `cache.driver=redis`. Startup **must** Ping successfully or the process will not listen. A runtime Redis outage falls back to in-process memory and reconnects; the process does **not** exit immediately. See [Cache and Redis](/en/guide/config/cache).

## Supported systems

Linux with `apt-get` or `yum`/`dnf`, run as root. macOS and distros without those managers: the script exits 1; use manual install or Docker.

## Procedure

1. Run as root (replace the URL if you cloned the repo):

::: code-group

```bash [curl]
curl -fsSL https://raw.githubusercontent.com/Kirizu-Official/KiriVers/main/scripts/install-deps.sh | sudo bash
```

```bash [wget]
wget -qO- https://raw.githubusercontent.com/Kirizu-Official/KiriVers/main/scripts/install-deps.sh | sudo bash
```

```bash [local]
sudo bash scripts/install-deps.sh
```

:::

2. Enter the sudo password if prompted.
3. Answer the Redis prompt: `y` to install, Enter to skip.
4. On success, stdout states PostgreSQL is installed and points at the manual install page for the KiriVers package. Note the default database port (usually `5432`) and the distro postgres user policy.

::: details What success stdout contains
The script prints that PostgreSQL is installed and points at GitHub Releases for the package, then the manual install page (copy three YAML files, create an admin, start the process). If Redis was installed, it notes that a single process may keep `cache.driver=memory`; multiple processes or `cache.driver=redis` require a successful startup Ping.
:::

::: tip Releases ships zip archives
The script only installs databases; it never touches KiriVers. When you do fetch the server, pick `KiriVers-<OS>-<Arch>-<sha6>.zip` on the Release page by **prefix** (the trailing six hex chars are the first six of the packed binary's SHA-256, so the name changes every release). Unpacking gives you `kirivers-<os>-<arch>` (`.exe` on Windows). See [manual install](/en/guide/install/manual).
:::

## Failures

| Symptom | Action |
|---------|--------|
| Not root | Retry with `sudo` |
| No apt/yum/dnf | Exit 1; use manual install or Docker. Missing Docker is **not** a failure of this script |
| Port `5432` in use | Stop the old PostgreSQL or change the DSN port |
| Distro repo has no postgresql package | Enable the distro module stream / official PostgreSQL repo, or install manually |

Next: [manual install](/en/guide/install/manual).
