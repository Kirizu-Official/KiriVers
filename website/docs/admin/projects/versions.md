---
title: "版本"
description: "draft → ready → publish。列表 latest=true 语义。TUS。禁止未知 S3 键裸 PUT。"
---

# 版本

导航 **版本管理**。生命周期：**草稿** → **标记就绪** → **发布** / **弃用** / **吊销**。版本线 **撤下（yank）**、**停用**。**渠道晋升** 到更稳渠道。

## 创建与发布

1. 点 **创建版本**。填 **构建号** 和/或 **SemVer**、**渠道**。stable 不能用预发布后缀（<ErrorCode code="CHANNEL_SUFFIX_MISMATCH" />）。同号占用 <ErrorCode code="VERSION_ALREADY_EXISTS" />。引擎与标识不符 <ErrorCode code="ENGINE_MISMATCH" />。
2. 可选 **长期支持（LTS）**、**关键版本（强制全员）**、**最低直升来源**。关键版本不能走灰度（<ErrorCode code="GRAY_NOT_ALLOWED_ON_CRITICAL" />）。
3. 打开 **产物管理**。点 **上传产物**，通道可选 **一次性直传** 或 **TUS 分片（可续传）**。`direct_s3` 恒为 **false**：禁止对未知 S3 键裸 PUT。声明哈希不符 <ErrorCode code="CHECKSUM_MISMATCH" />。未登记 hw → <ErrorCode code="HW_REV_UNKNOWN" />。已发布产物不可覆盖（<ErrorCode code="ARTIFACT_IMMUTABLE" />，先 **撤下（yank）**）。
4. 对该平台线点 **标记就绪**。上传未完成 → <ErrorCode code="UPLOAD_INCOMPLETE" />。
5. 点 **发布**。没有任何就绪版本线 → <ErrorCode code="ARTIFACT_REQUIRED" />。`auto_publish_when` 未齐 → <ErrorCode code="AUTO_PUBLISH_PENDING" />。界面「发布后更新日志不可修改」。
6. 差量 **生成差量**、产物 **复用已有产物**、多文件打归档为异步 Job（顶栏 **任务中心**）。失败 <ErrorCode code="JOB_FAILED" />。

## 列表 `latest=true`

管理 `GET versions?latest=true&os=&arch=` 返回 `{ versions, latest }`。`latest` 是已发布且线 ready、用 `CompareVersions` 算出的，**不是**客户端 `SelectTarget`。无 `latest` 查询时响应不含 `latest`。

概念：[功能指南 · 版本发布](/guide/features/versions)。CI：[自动发版](/api/ci)。
