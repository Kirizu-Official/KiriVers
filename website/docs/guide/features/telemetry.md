---
title: "遥测与设备"
description: "可选 report；202 失败不得阻断更新。删名册 ≠ 隐私擦除。灰度依赖在册设备。"
---

# 遥测与设备

遥测是可选的安装结果上报。运营名册来自 check / report，供灰度放号与概览统计。

## 何时使用

需要国家分布、安装成败或灰度白名单时启用。`device_id_policy=none` 时无法按设备放号。

## 配置入口

- **客户端** 名册：[设备列表](/admin/projects/clients)
- **设置 → 隐私**：策略、留存、按哈希删除
- [GeoIP](/admin/geoip) 影响国家图

YAML 无遥测开关。

## 规则与错误码

客户端可选 `POST .../telemetry/report` → **202**。失败不得阻断下载/安装。`device_id_policy=none` 时带 device 可能 400 <ErrorCode code="INVALID_REQUEST" />。删除名册行 404 <ErrorCode code="CLIENT_NOT_FOUND" />。

删除「客户端」名册行 **不是** 隐私擦除。按哈希擦除在设置「隐私」，返回 `telemetry_deleted` / `allowlist_deleted`。遗留 `POST .../clients/login` 为 404。

## 客户端职责

report 失败忽略。设备身份由应用生成。geo 来自上报时的解析，不是实时查库。

相关：[API · 遥测](/api/client/telemetry)、[灰度发布](./gray)。
