---
title: "商店订阅"
description: "listing slug、纯文本 404、ETag、store_full vs hash-root。"
---

# 商店订阅

缺 listing、未知 protocol、或旧路径无 slug → **纯文本 404**（无 JSON `code`）。URL 必须含 `{listing_slug}`。

未完成灰度不会出现在匿名 feed。`line_full` 使用 `store_full`，不是 hash-root `full`。遵守 ETag。官方 SDK 不读 feed。
