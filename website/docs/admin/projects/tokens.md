---
title: "访问密钥"
description: "项目 Token 与 CI Token 分开。明文只在创建响应出现。"
---

# 访问密钥

导航 **Token 管理**。页内 **项目 Token** 与 **CI Token** 分 Tab。商店 Token 在 [项目设置](./settings) **安全**。渠道 Token 在 [渠道](./channels)。

1. 打开 **Token 管理**，选 **项目 Token** 或 **CI Token**。
2. 点 **创建 Token**。填 **名称**、**Scopes**、可选 **过期时间（留空永久）**。
3. 对话框 **请立即保存 Token**：明文只在本次展示，关闭后只显示 **指纹**。
4. **吊销 Token** 立即让调用方失效。缺失会话 401 <ErrorCode code="UNAUTHORIZED" />。

CI Token 走管理平面 `ci/releases`，见 [自动发版](/api/ci)。概念：[功能指南 · 访问密钥](/guide/features/tokens)。
