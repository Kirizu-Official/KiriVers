---
title: "平台矩阵"
description: "(os,arch)、package_type、最低系统。决定增量路径。已发布后形态锁定。"
---

# 平台矩阵

平台矩阵登记每个 `(os, arch)` 的 **产物形态**、默认差量算法与硬件策略。

## 何时使用

发第一个包之前至少登记一行。形态决定增量：单文件 → diff；多文件 → pack。场景页按形态分流，见 [场景总览](/guide/scenarios/)。

## 配置入口

项目 → **平台矩阵**。步骤：[管理 · 平台矩阵](/admin/projects/matrix)。OS/Arch 来自 `platforms/catalog`。YAML 无矩阵表。

## 规则与错误码

该平台已有已发布版本线后形态锁定（<ErrorCode code="PACKAGE_TYPE_IMMUTABLE" />）。最低系统 / API 在**版本线**上（`min_os` / `min_api_level`），不在已删除的矩阵列。客户端当前线低于下限 → <ErrorCode code="MIN_OS_NOT_MET" />。回退架构按矩阵登记。

## 客户端职责

check 的 `os` / `arch` 必须能规范化到已登记行。未知组合不会匹配产物。

相关：[增量更新](./incremental)、[硬件代号](./hw-revs)。
