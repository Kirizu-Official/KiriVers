---
title: "硬件代号"
description: "rank、产物绑定、HW_REV_UNKNOWN / HW_REV_INCOMPATIBLE。不进入版本号。"
---

# 硬件代号

`hw_rev` 区分同一 Version Line 上的硬件变体，**不是**版本号的一部分。rank 定义兼容范围。

## 何时使用

同一固件镜像有多块硬件改版时使用。不要把 hw 写进 SemVer。MCU 场景见 [MCU](/guide/scenarios/mcu)。

## 配置入口

项目 → **硬件代号**；上传产物时绑定代号。步骤：[管理 · 硬件代号](/admin/projects/hw-revs)。

## 规则与错误码

未登记代号绑定产物 → <ErrorCode code="HW_REV_UNKNOWN" />。客户端携带不兼容代号 → <ErrorCode code="HW_REV_INCOMPATIBLE" />。按包哈希下载时忽略 `X-Hw-Rev`。

## 客户端职责

check 可选 `hw_rev`。商店 feed 只投影默认 hw 变体。

相关：[平台矩阵](./matrix)。
