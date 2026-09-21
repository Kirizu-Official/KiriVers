---
title: "Choose an install path"
description: "Official binaries embed the admin UI and are named by content hash, so pick them by prefix; the one-click script only prepares PostgreSQL on Linux, Windows is manual-only."
---

# Choose an install path

The primary install path is a **GitHub Releases executable** or the Docker image `kirizuofficial/kirivers`. You do not need Node.js or Go, and you do not compile or copy admin UI files. Official executables already embed the admin UI.

Follow the current [GitHub Releases](https://github.com/Kirizu-Official/KiriVers/releases) and Docker Hub tags. Do not use guessed download URLs that 404.

## Matrix

| Environment | Recommendation |
|-------------|----------------|
| Linux (Debian/Ubuntu: apt; RHEL/CentOS/Fedora: yum or dnf) | Optionally run the [one-click script](/en/guide/install/one-click) to install PostgreSQL (Redis optional), then download the program per [manual install](/en/guide/install/manual) |
| Windows | **Do not** run the script. Follow manual install: the official installer for PostgreSQL, Redis optional or the memory cache, then the `.exe` |
| macOS and any Linux without apt/yum/dnf | The script exits. Use manual install, or [Docker](/en/guide/install/docker) |
| Docker already available | [Docker](/en/guide/install/docker) or [Compose](/en/guide/install/compose) to run the program and the database together |

## Picking a Release asset {#releases-files}

Open <https://github.com/Kirizu-Official/KiriVers/releases> and pick the archive by **prefix**: `KiriVers-<OS>-<Arch>-<hash>.zip`. The trailing six characters are the first six hex digits of the SHA-256 of the binary inside, so every filename changes on every release — recognise the prefix, do not bookmark a specific link. Extracting gives you an executable named `kirivers-<os>-<arch>` (`.exe` on Windows).

| Your system | Typical CPU | Download prefix |
|-------------|-------------|-----------------|
| Windows | x64 | `KiriVers-Windows-x86_64-` |
| Windows | 32-bit x86 | `KiriVers-Windows-x86-` |
| Windows | Arm | `KiriVers-Windows-arm64-` |
| Linux | x86_64 | `KiriVers-Linux-x86_64-` |
| Linux | aarch64 | `KiriVers-Linux-arm64-` |
| Linux | 32-bit x86 | `KiriVers-Linux-x86-` |
| Linux | ARMv7 (Raspberry Pi 2/3 and similar) | `KiriVers-Linux-armv7-` |
| Linux | RISC-V 64 | `KiriVers-Linux-riscv64-` |
| macOS | Intel | `KiriVers-macOS-x86_64-` |
| macOS | Apple Silicon | `KiriVers-macOS-arm64-` |

Each release also carries `SHA256SUMS.txt` (the uploaded files are verifiable directly with `sha256sum -c`) and `frontend-dist.zip` (admin console static files; official binaries already embed them, so you normally do not need it).

On Windows, check Task Manager → Performance or System Information for x64 vs ARM. On Linux / macOS, run `uname -m` (`x86_64`, `aarch64` / `arm64`).

::: warning The three Linux libc flavours
`x86_64` and `arm64` are **musl** builds (they run on Alpine). `x86`, `armv7` and `riscv64` are **glibc cross builds** and require glibc ≥ 2.39 on the target (Ubuntu 24.04 generation); armv7 additionally needs hard-float. On older systems use [Docker](/en/guide/install/docker) or build from source.
:::

Next: [one-click dependencies](/en/guide/install/one-click) (Linux only) or [manual install](/en/guide/install/manual).
