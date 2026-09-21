---
title: "批量上传发版"
description: "把 zip 解到已有版本。会话内跟踪 Job。direct_s3 恒 false。"
---

# 批量上传发版

导航 **批量发版**。页说明：「为已有版本上传 zip 产物包，可选在解包后发布。进度在任务中心查看。」

1. 打开 **批量发版**。选 **单文件** 或 **目录压缩包**。
2. **目录压缩包**：选 **Bundle zip**，填 **版本号**（须已存在），可选勾选 **解压成功后立即发布**，可填 **auto_publish_when（全部就绪后自动发布）**（每行 `os/arch`）。
3. **单文件**：勾选 **上传成功后立即发布** 时会创建缺失版本与该 OS/Arch 线再上传。
4. 点 **提交发版任务**。`direct_s3` 恒 false。目录无法解析 → <ErrorCode code="ZIP_LAYOUT_INVALID" />。上传未完成 → <ErrorCode code="UPLOAD_INCOMPLETE" />。auto_publish 未齐 → <ErrorCode code="AUTO_PUBLISH_PENDING" />。
5. 本页会话跟踪 Job，同时看顶栏 **任务中心**。`GET /api/v1/admin/jobs/{job_id}`。缺失 <ErrorCode code="JOB_NOT_FOUND" />；失败 <ErrorCode code="JOB_FAILED" />。

概念：[版本发布](/guide/features/versions)。
