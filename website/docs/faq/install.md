---
title: "安装不上 / 打不开网页"
description: "DSN、端口、Redis 启动 Ping、static_dir 空字符串、自编译未 yarn build。官方 Releases 不必拷 dist。"
---

# 安装不上 / 打不开网页

| 现象 | 检查 |
|------|------|
| 不知道该下哪个包 | Release 里按前缀 `KiriVers-<系统>-<架构>-` 选，见 [如何选 Releases 文件](/guide/install/#releases-files)。解压出来的可执行文件叫 `kirivers-<os>-<arch>` |
| 上次的下载链接今天 404 | 正常：包名末尾六位是二进制内容哈希，每次发版都变。收藏 Release 页面，不要收藏单个文件链接 |
| Linux 报 `GLIBC_x.y not found` | `x86` / `armv7` / `riscv64` 三个包是 glibc ≥ 2.39 的交叉构建；老系统改用 [Docker](/guide/install/docker)（musl，仅 amd64/arm64）或自编译 |
| 校验和怎么验 | `sha256sum -c SHA256SUMS.txt`（第一段是 11 个上传件）。第二段以 `#` 注释记录包内二进制，需解压后自行比对 |
| 进程立刻退出 | `postgres.dsn`；五路日志是否双 sink 全关；`changelog.default_entries > max_entries` |
| `cache.driver=redis` 起不来 | 启动 Ping 失败。先起 Redis 或改回 `memory` |
| 连错端口 | 客户端 `:8080`，管理 `:8081` |
| JSON 而不是登录页 | `admin.enabled`；`static_dir: ""` 关闭了 UI；自编译二进制未先构建管理台（没有 `index.html`） |
| 官方 Releases 打不开 UI | 官方二进制已嵌入管理台，不必再拷 `frontend/dist`。检查 `static_dir` 是否被设成空 |

Windows 使用 [手动安装](/guide/install/manual)，一键脚本只支持带 apt/yum 的 Linux。`static_dir` 指向管理台；文档站 VitePress `dist` 不是管理台。
