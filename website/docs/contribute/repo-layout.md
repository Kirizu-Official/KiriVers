---
title: "目录结构"
description: "以当前仓库为准。含 frontend/embed.go 与 internal/delta。默认分支没有语言 SDK 源码。"
---

# 目录结构

以默认分支磁盘为准。语言 SDK **不**在本树。`.trellis/` 是维护者/AI 工作流，不是贡献必读主路径。GitHub 是唯一 forge：检查与发版都只有 `.github/workflows/` 一套，仓库里没有第二套流水线定义。

```text
main.go                 # 委托 cmd.Run()
CHANGELOG.md            # 发版作业插回头部的更新日志（首次发版后出现），贡献者不要手写
cmd/                    # server / admin / listen（listen 不是 CLI 子命令）
configs/                # *-example.yaml；本地 yaml gitignore
internal/
  cache/
  config/
  controller/           # admin/ + client/ + openapi.*.json
  database/
  delta/                # 差量引擎
  logger/
  middleware/
  model/
  platform/
  repository/
  service/              # 含 update/、store/
  storage/              # local + s3
pkg/
  hashutil/ pathutil/ response/ semver/
  signature/ urlsign/ webhook/ grayutil/
frontend/               # Vue 管理台；embed.go 嵌入 dist；日常 yarn dev
frontend/embed.go
frontend/dist/.gitkeep
docs/                   # 内部说明（非 VitePress 根）
dev/docker/             # 开发用 Postgres/Redis
dev/build/              # 运行镜像 Dockerfile 与发版 CLI（无 shell 脚本、无 Compose）
  Dockerfile            # Alpine 运行镜像，构建上下文为仓库根
  changelog-types.conf  # 章节 emoji/文案表——只是显示层，不参与版本推导
  kirivers.py           # 唯一入口：python3 dev/build/kirivers.py <子命令>（Windows 用 python）
  kirivers_build/       # 纯标准库包，按职责分模块；子命令名沿用旧脚本名
    cli.py              # 子命令注册表与分发（usage 与退出码约定）
    common.py           # 平台矩阵、内部二进制名、归档名、file 机器模式、sha256 的唯一定义（文档表格由它派生）
    build.py            # build-cgo / build-linux-musl / assert-linux-musl / install-cross-toolchains / install-llvm-mingw / frontend-build
    version.py          # next-version：由 Conventional Commits 推导下一版本
    notes.py            # release-notes（Release 正文与 CHANGELOG 段落）与 changelog 写回
    package.py          # asset-names 与 package-assets（zip + SHA256SUMS.txt）
    images.py           # docker-image（每架构原生构建并推 <semver>_amd64 / _arm64）与 docker-manifest
    guards.py           # commitlint / issue-link（进 main 的 PR 是否绑定 Issue）/ guard（守卫判定矩阵）
    selfcheck.py        # check-workflows：流水线静态门（触发方式、矩阵、零 QEMU）
.github/workflows/      # ci.yml（PR 与 main push 的检查）、pr-guard.yml、gosec-scan.yml + security-gate.yml（Gosec 沙箱扫描与 ready-merge 状态机）、release.yml（只手动 workflow_dispatch）、sdk-automerge.yml（只手动；批量合并 sdk/* 并发版推 registry）
website/                # 官方文档站
deploy/                 # 使用者一键 Compose（.env 统一口令 + 自带 YAML）
scripts/install-deps.sh
```
