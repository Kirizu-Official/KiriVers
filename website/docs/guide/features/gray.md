---
title: "灰度发布"
description: "白名单加 start/step/interval 旋钮与 gray_completed_at。不是 HMAC 或 rollout_percent。"
---

# 灰度发布

灰度用**设备白名单**加上覆盖率旋钮（`gray_start_percent` / `gray_step_percent` / `gray_interval_seconds`）和完成标记 `gray_completed_at`。**不是** HMAC，也不是 `rollout_percent` 哈希放号。

## 何时使用

要把新版本先给一部分在册设备、再全量给匿名客户端和商店 feed 时使用。空名册或关键版本不要走增量灰度。

## 配置入口

版本 → **灰度**。步骤：[管理 · 灰度](/admin/projects/gray)。项目 **设置 → 更新策略** 控制放号权重。YAML 无灰度百分比键。

## 规则与错误码

| 状态 | 未入名单（含匿名） | 白名单设备 |
|------|--------------------|------------|
| 灰度未完成 | 仍见旧版 | 可见该版本 |
| `gray_completed_at` 已设置 | 与商店匿名 feed 可见 | 可见 |

未完成时 check / feed 为 `Cache-Control: private`。空设备名册上开始灰度会**立即完成**。`device_id_policy=none` 无法按设备放号。PATCH 旋钮**不**重置 `gray_started_at`。关键版本 <ErrorCode code="GRAY_NOT_ALLOWED_ON_CRITICAL" />。

## 客户端职责

check 必须带稳定 `device_id`（策略允许时）。匿名客户端在完成前看不到该版本。商店 feed 使用同一 `GrayIsComplete()` 门。

相关：[检查更新](./check)、[遥测与设备](./telemetry)、[CDN](./cdn)。
