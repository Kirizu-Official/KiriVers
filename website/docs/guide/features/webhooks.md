---
title: "Webhook"
description: "发布 / 版本线就绪 / 吊销时通知。投递记录在设置里查看。"
---

# Webhook

Webhook 在版本**发布**、版本线**就绪**、版本**吊销**时 POST 到你的 URL。

## 何时使用

要驱动 CI、聊天通知或外部审计时使用。不是客户端更新通道。

## 配置入口

项目 **设置 → Webhook**。填 **Webhook URL**，点 **查看投递记录**。步骤：[项目设置](/admin/projects/settings)。YAML 无单独 webhook 文件。

## 规则与错误码

投递是异步 Job，失败重试后进入投递表。密钥不要写进文档示例。HTTP 字段见 [API 参考](/api/reference/) 中的 `openapi.admin.json`（<a href="/api/scalar/admin" target="_blank" rel="noopener">新窗口打开 Scalar</a>）Webhooks 标签。

## 客户端职责

无。软件客户端不消费 Webhook。

相关：[版本发布](./versions)、[后台任务](/admin/jobs)。
