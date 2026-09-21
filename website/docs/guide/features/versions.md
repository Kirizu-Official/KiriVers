---
title: "版本发布"
description: "Version vs Version Line。draft → ready → publish。关键版本禁灰度。比较引擎发布后不可改。"
---

# 版本发布

**Version** 是渠道上的一个版本号。**Version Line** 是该版本在某一 `(os,arch)`（及 hw）上的产物线。

## 何时使用

每个要发给客户端的构建都要有一条 Version，并在矩阵登记过的平台上准备好 Version Line。商店 feed 与原生 check 共用这些已发布且就绪的线。

## 配置入口

- 控制台：**版本管理**、**产物管理**、**批量发版**。点击路径：[管理 · 版本](/admin/projects/versions)。
- YAML：无单独「发版」文件。比较引擎在创建项目时选定，见 [项目创建](/admin/projects/create)。

## 规则与错误码

生命周期：**草稿** → **标记就绪** → **发布**。还可 **弃用**、**吊销**；线可 **撤下（yank）** / **停用**。低稳渠道可 **渠道晋升**。

发布拒绝：

- <ErrorCode code="ARTIFACT_REQUIRED" />
- <ErrorCode code="AUTO_PUBLISH_PENDING" />
- <ErrorCode code="UPLOAD_INCOMPLETE" />

发布后比较引擎不可改（<ErrorCode code="COMPARE_ENGINE_IMMUTABLE" />）。关键版本不能走灰度（<ErrorCode code="GRAY_NOT_ALLOWED_ON_CRITICAL" />）。已发布产物不可覆盖（<ErrorCode code="ARTIFACT_IMMUTABLE" />）。

CI 走 `ci/releases`（[自动发版](/api/ci)）；浏览器走产物管理 / TUS。`direct_s3` 恒 false。产物可复用。

## 客户端职责

只消费 **已发布** 且线 **就绪**（`packs_ready_at` 已打戳）的目标。草稿与未预热线对 check/feed 不可见（<ErrorCode code="VERSION_NOT_VISIBLE" />）。

相关：[灰度发布](./gray)、[增量更新](./incremental)、[渠道](./channels)。
