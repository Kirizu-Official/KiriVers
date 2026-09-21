---
title: "选哪种安装方式"
description: "Linux 可先用一键脚本装 PostgreSQL；Windows 只走手动安装。官方二进制已内嵌管理台。"
---

# 选哪种安装方式

KiriVers 的主安装路径是 **GitHub Releases 可执行文件** 或 Docker 镜像 `kirizuofficial/kirivers`。不必安装 Node.js 或 Go，也不必编译或拷贝管理台网页。官方可执行文件已内嵌管理 UI。

以当前 [GitHub Releases](https://github.com/Kirizu-Official/KiriVers/releases) 与 Docker Hub 标签为准，不要使用会 404 的猜测下载链接。

## 对照表

| 环境 | 建议 |
|------|------|
| Linux（Debian/Ubuntu：apt；RHEL/CentOS/Fedora：yum 或 dnf） | 可先跑 [一键脚本](/guide/install/one-click) 装 PostgreSQL（可选 Redis），再按 [手动安装](/guide/install/manual) 下载程序 |
| Windows | **不要**跑一键脚本。按手动安装：官方安装包装 PostgreSQL，可选 Redis 或使用内存缓存，再下载 `.exe` |
| macOS 及其他没有 apt/yum/dnf 的 Linux | 一键脚本会退出。按手动安装，或使用 [Docker](/guide/install/docker) |
| 已有 Docker | [Docker](/guide/install/docker) 或 [Compose](/guide/install/compose) 把程序与数据库一起跑起来 |

## 如何选 Releases 文件 {#releases-files}

打开 <https://github.com/Kirizu-Official/KiriVers/releases>，按**前缀**挑包：`KiriVers-<系统>-<架构>-<哈希>.zip`。末尾六位数是包内二进制的 SHA-256 前缀，所以文件名每次发版都会变——认准前缀，不要收藏具体链接。解压后里面的可执行文件叫 `kirivers-<os>-<arch>`（Windows 为 `.exe`）。

| 你的系统 | 常见 CPU | 下载前缀 |
|----------|----------|----------|
| Windows | x64 | `KiriVers-Windows-x86_64-` |
| Windows | 32 位 x86 | `KiriVers-Windows-x86-` |
| Windows | Arm | `KiriVers-Windows-arm64-` |
| Linux | x86_64 | `KiriVers-Linux-x86_64-` |
| Linux | aarch64 | `KiriVers-Linux-arm64-` |
| Linux | 32 位 x86 | `KiriVers-Linux-x86-` |
| Linux | ARMv7（树莓派 2/3 等） | `KiriVers-Linux-armv7-` |
| Linux | RISC-V 64 | `KiriVers-Linux-riscv64-` |
| macOS | Intel | `KiriVers-macOS-x86_64-` |
| macOS | Apple Silicon | `KiriVers-macOS-arm64-` |

另附 `SHA256SUMS.txt`（可直接 `sha256sum -c` 校验下载件）与 `frontend-dist.zip`（管理台静态文件，官方二进制已内嵌，通常不需要）。

在 Windows 任务管理器「性能」或「系统信息」中查看是 x64 还是 ARM。在 Linux / macOS 上运行 `uname -m`（`x86_64` 或 `aarch64` / `arm64`）。

::: warning 三种 Linux 包的 libc 差别
`x86_64` 与 `arm64` 是 **musl** 构建（能跑在 Alpine 上）；`x86`、`armv7`、`riscv64` 是 **glibc 交叉构建**，要求目标机 glibc ≥ 2.39（Ubuntu 24.04 一代），armv7 还需 hard-float，riscv64 需 RVA20+ 用户态。老系统请用 [Docker](/guide/install/docker)（仅 amd64/arm64）或自编译。
:::

下一步：[一键安装](/guide/install/one-click)（仅 Linux 依赖）或 [手动安装](/guide/install/manual)。
