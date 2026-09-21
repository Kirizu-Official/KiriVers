---
title: "用户端没有更新"
description: "未发布、os/arch、灰度、GET check、渠道 Token 非 403、MIN_OS_NOT_MET、NO_SAFE_TARGET、VERSION_REVOKED。"
---

# 用户端没有更新

| 检查 | 说明 |
|------|------|
| 版本状态 | 必须「已发布」，线 ready（`packs_ready_at` 已打戳） |
| os/arch | 必须有对应版本线 |
| 灰度未完成 | 未入名单（含匿名）仍见旧版 |
| 方法 | 必须 **POST** check；GET 同路径是 404 |
| 渠道 Token | 填错**不是** 403，只跳过隐藏渠道 |
| `MIN_OS_NOT_MET` | 系统版本低于线下限 |
| `NO_SAFE_TARGET` / `VERSION_REVOKED` | 吊销/yank 后无安全目标 |

204 表示已是最新，不是失败。304 是 ETag 命中。
