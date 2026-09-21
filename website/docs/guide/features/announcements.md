---
title: "公告"
description: "项目公告资源，不在 check 里。七种 scope。客户端独立 GET；空列表 200。"
---

# 公告

公告是**项目资源**：给软件客户端的有序通知。它不是版本 changelog，也不出现在 check 响应里。

## 何时使用

停机通知、强制文案、活动说明。版本发布说明应写在 [更新说明](./changelog)。

## 配置入口

项目 → **公告**。步骤：[管理 · 公告](/admin/projects/announcements)。YAML 无公告文件。公告 GET 有独立 `s-maxage`。

## 规则与错误码

客户端按请求里的 version / os / arch 匹配。**禁止**「仅 OS+Arch、无版本」（写入 400 <ErrorCode code="INVALID_REQUEST" />）。版本 scope 但 Version 不存在 → 404 <ErrorCode code="VERSION_NOT_FOUND" />。非法 `version` 查询 400 <ErrorCode code="INVALID_QUERY_PARAM" />。

一行一种语言。定时窗口：`starts_at` / `ends_at`。草稿与未到期 `scheduled` 只在管理台可见。

客户端 **GET 公告**（与 check 分开）。显式 `locale` **严格**匹配；省略 locale 才走语言链 + leftover。空匹配是 **200** `{ "announcements": [] }`，不是 204。配图 UUID GET 无需 Token。遵守 ETag / 304。产品没有全局固定拉取秒数。

相关：[语言](./languages)、[API · 公告](/api/client/announcements)。
