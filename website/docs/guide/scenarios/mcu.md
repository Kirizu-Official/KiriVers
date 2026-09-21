---
title: "MCU / 固件"
description: "常用整数版本。硬件变体用 hw_rev，不写进版本号。客户端仍是 POST check。"
---

# MCU / 固件

## 产物形态

固件镜像按**单文件**发布。比较引擎常用**整数构建号**（项目创建后、首次发布前选定；已发布则 <ErrorCode code="COMPARE_ENGINE_IMMUTABLE" />）。

同一镜像的硬件改版用 [硬件代号](/guide/features/hw-revs) 区分，**不要**写进版本号。未知代号 <ErrorCode code="HW_REV_UNKNOWN" />；不兼容 <ErrorCode code="HW_REV_INCOMPATIBLE" />。

MCU 场景没有商店 feed：更新器是自制客户端，走原生 JSON。

## 客户端职责

1. 启动或按产品节奏 **POST** check（产品不规定固定秒数；遵守 `Retry-After`）。
2. 带上 `hw_rev`、整数 `current_version`、os/arch。
3. 下载 SHA-256 对象后写入闪存；官方 SDK 无默认 Replacer / 总线驱动。

控制台：[硬件代号](/admin/projects/hw-revs)、[版本发布](/admin/projects/release)。
