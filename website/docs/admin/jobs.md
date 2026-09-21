---
title: "后台任务"
description: "顶栏铃铛与批量发版会话跟踪。GET /api/v1/admin/jobs/{job_id}。客户端 pack 不返回 job_id。"
---

# 后台任务

界面：顶栏铃铛（**任务中心**）+ **批量发版** 页会话内跟踪。**无**独立 Jobs 菜单或 `/jobs` 页面路由。

1. 上传、差量、复用、打归档、清理会入队。
2. 点顶栏铃铛打开 **任务中心**，看 **排队中 / 执行中 / 成功 / 失败**。
3. HTTP 查询：`GET /api/v1/admin/jobs/{job_id}`。缺失 <ErrorCode code="JOB_NOT_FOUND" />；失败 <ErrorCode code="JOB_FAILED" />。

客户端 `POST /update/pack` **不**返回管理 `job_id`。对同一 pack URL 轮询直到 `ready` 或 `full_package`。见 [增量更新](/guide/features/incremental) 与 [批量发版](/admin/projects/release)。
