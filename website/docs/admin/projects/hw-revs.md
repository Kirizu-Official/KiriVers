---
title: "硬件代号"
description: "未登记代号绑定产物 → HW_REV_UNKNOWN。不进入版本号。"
---

# 硬件代号

导航 **硬件代号**。`hw_rev` 区分同一固件的硬件变体，**不进入版本号**。**rank** 越大越新。

1. 打开 **硬件代号**。
2. 点 **新增代号**。
3. 填 **代号**（规范化后须匹配 `^[a-z0-9_-]{1,64}$`）、**rank**、可选 **备注**。

未登记代号绑定产物 → <ErrorCode code="HW_REV_UNKNOWN" />。客户端携带不兼容代号 → <ErrorCode code="HW_REV_INCOMPATIBLE" />。按内容 SHA-256 下载时忽略 `X-Hw-Rev`。概念：[功能指南 · 硬件代号](/guide/features/hw-revs)。
