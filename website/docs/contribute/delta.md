---
title: "差量引擎"
description: "hdiffpatch CGO HDIFF13& vs 历史 KVDIFFHP1。magic 分流。构建要 C++ 工具链。"
---

# 差量引擎

单文件二进制差量在 `internal/delta`。服务端 **必须** `CGO_ENABLED=1` 并带 C++ 工具链（Windows MinGW `g++` / Linux `g++` 或 `clang++` / macOS Xcode `clang++`）。运行时不再从 PATH 调用 `hdiffz`/`hpatchz`。Go SDK 仍是 `CGO_ENABLED=0`，见 SDK 文档。

| 算法 | magic | 说明 |
|------|-------|------|
| hdiffpatch（CGO） | `HDIFF13&` | 新生成未压缩官方格式；`DescribeAvailable` 为 `cgo` |
| hdiffpatch 历史 | `KVDIFFHP1` | 仅 Patch，不是官方 .hdiff |
| bsdiff | `BSDIFF40` | `pure-go` |
| xdelta3 / VCDIFF | `D6 C3 C4` | RFC 3284 二进制 magic，不是 ASCII；`pure-go` |

`Patch` 按 magic 分流，禁止交叉解码。未知算法 HTTP `DELTA_ALGO_UNSUPPORTED`。管理 `POST .../artifacts/delta` 入队 Job。HTTP 路径不得 `archive/zip`。

官方 `darwin/*` 二进制必须在 macOS 宿主上 CGO 构建（`darwin/arm64` 原生、`darwin/amd64` 同机 `clang -arch x86_64`），不能从 Linux 无 Apple SDK 交叉。GitHub 发版工作流用钉版本的 macOS runner（`macos-15`）产出这两个目标；GitHub 是唯一 forge，十个目标里不存在「某个平台跳过」的情况，缺件时打包作业直接失败而不是伪造文件。
