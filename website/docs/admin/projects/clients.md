---
title: "设备列表"
description: "名册、统计。删除名册不等于隐私擦除。"
---

# 设备列表

导航 **客户端**。这是运营名册（check / report 写入），用于灰度放号与概览统计。

1. 打开 **客户端**，阅读 KPI：**设备总数**、**24 小时活跃**、**7 日活跃**。
2. 列含 **设备哈希**、**最近 IP**、**最近检查**、**国家 / 地区**。点 **完整 JSON** 看 custom。
3. **删除客户端** 只去掉名册行，并会从本项目灰度白名单移除该设备。**不会**擦除隐私哈希。

隐私擦除在 [项目设置](./settings) **隐私**：点 **按哈希删除设备**，响应 `telemetry_deleted` / `allowlist_deleted`。`device_id_policy=none` 时 check 不插入名册，灰度无法按设备放号。未知行 404 <ErrorCode code="CLIENT_NOT_FOUND" />。概念：[遥测与设备](/guide/features/telemetry)。
