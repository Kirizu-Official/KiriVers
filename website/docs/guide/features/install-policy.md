---
title: "安装策略"
description: "独立资源。只盖章新 Manifest。同平台渠道覆盖项目。"
---

# 安装策略

安装策略是独立资源，不是项目 PATCH JSON。它告诉多文件客户端哪些路径 **存在则跳过**（`KEEP_IF_EXISTS`）。

## 何时使用

多文件 `package_type` 且希望保留用户本地文件时使用。单文件线没有 Manifest 路径策略。

## 配置入口

**设置 → 安装策略模板** 或渠道上的 **安装策略**。步骤：[管理 · 安装策略](/admin/projects/install-policy)。

## 规则与错误码

同一 `(os, arch)` 上渠道规则按路径覆盖项目。只盖章**之后新建**的 Manifest。忽略 `_keep.json` / `keep_if_exists.txt`。参考 Manifest 用渠道内 `CompareVersions`，不是 `versions?latest=true`。未知矩阵对 400 <ErrorCode code="INVALID_REQUEST" />。非法路径 <ErrorCode code="INVALID_PATH" />。

`KEEP_IF_EXISTS` 只是元数据；完整性校验仍按 Manifest SHA。官方 SDK 可在文件已存在时从 `needed_paths` 省略该路径。

相关：[增量更新](./incremental)、[平台矩阵](./matrix)。
