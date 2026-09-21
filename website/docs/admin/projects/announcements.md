---
title: "公告"
description: "七种 scope。一行一种语言。空匹配 200 空数组。配图走 media。"
---

# 公告

导航 **公告**。公告是项目资源，不是版本 changelog，也不出现在 check 响应里。

1. 点 **新建公告**。
2. 选 **作用域**：整个项目、仅版本、仅操作系统、仅架构、版本 + 操作系统、版本 + 架构、版本 + 平台线。**禁止**「仅 OS+Arch、无版本」（400 <ErrorCode code="INVALID_REQUEST" />）。版本 scope 但版本不存在 → 404 <ErrorCode code="VERSION_NOT_FOUND" />。
3. 一行一种 **语言**。填 **标题**、可选 **副标题**、**Markdown**。配图走 media（客户端 UUID GET，无 Token / 无 urlsign）。`${site_url}` 按 Referer 展开。
4. 可选 **开始时间（UTC 窗口）** / **结束时间**。**草稿** 与未到期 **定时开始** 只在管理台可见。
5. **上移** / **下移** 调整顺序。**测试客户端 API** 预览匹配。**删除公告** 后客户端立即收不到。

客户端空匹配是 **200** `{ announcements: [] }`，不是 204。显式 `locale` 严格匹配。概念：[功能指南 · 公告](/guide/features/announcements)。HTTP：[API · 公告](/api/client/announcements)。
