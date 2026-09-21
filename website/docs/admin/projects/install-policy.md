---
title: "安装策略"
description: "独立资源。同平台渠道覆盖项目。只盖章新 Manifest。参考版本用渠道内 CompareVersions。"
---

# 安装策略

面板与渠道宽对话框标题 **安装策略模板** / **安装策略**。这是独立资源 `install-policy-rules`，不是项目 PATCH JSON。无矩阵时界面：「请先登记平台矩阵，再编辑安装策略。」

1. **设置** 打开 **安装策略模板**，或从 **渠道管理** 点 **安装策略**。
2. 选择 **平台 (os/arch)**。未知矩阵对 → 400 <ErrorCode code="INVALID_REQUEST" />。
3. 项目级规则 `channel_id` 为空 UUID；渠道级按路径覆盖同一 `(os, arch)`。
4. **添加路径** 手填 **路径（NFC，正斜杠）**，或从 **参考渠道最新版本中的文件** 勾选。参考 Manifest 用该渠道内 `CompareVersions` 的最新就绪版本，**不是** `GET versions?latest=true`。
5. 策略 **存在则跳过**（`KEEP_IF_EXISTS`）只是元数据，不会写入 zip。完整性校验仍按 Manifest SHA。忽略 `_keep.json` / `keep_if_exists.txt`。只盖章**之后新建**的 Manifest。

非法路径 <ErrorCode code="INVALID_PATH" />。未知渠道 404 <ErrorCode code="CHANNEL_NOT_FOUND" />。概念：[功能指南 · 安装策略](/guide/features/install-policy)。
