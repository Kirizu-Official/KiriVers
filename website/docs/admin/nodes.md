---
title: "节点管理"
description: "集群节点在线状态。单节点可跳过。node-sync 仅平台管理员。"
---

# 节点管理

抽屉 **节点** 仅平台管理员可见。页标题 **集群节点**。单进程部署可跳过本页。项目成员 403 <ErrorCode code="FORBIDDEN" />。

1. 打开抽屉 **节点**。
2. 查看 **状态**：**在线** / **离线**（超过 30s 无心跳视为离线）、**当前** 标记、**显示名**（YAML `node.display_name`，空则显示 UUID）、**最后在线**。
3. 模式芯片：**单例** 或 **集群**。多节点下载要同时满足 `storage.driver=s3` 与 `cache.driver=redis`。
4. **节点产物同步**（`node-sync`）仅平台管理员可操作。

CPU 百分比当前恒为 0；内存为 Go HeapAlloc / Sys，不是主机 RSS。概念：[功能指南 · 多节点](/guide/features/cluster)。YAML：[配置 · 多节点](/guide/config/cluster)。
