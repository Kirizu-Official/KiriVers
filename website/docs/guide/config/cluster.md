---
title: "多节点"
description: "cluster.download 仅在 storage.driver=s3 且 cache.driver=redis 时生效。s3 直链 vs local 代拉。"
---

# 多节点

| 参数 | 类型 | 默认 | 说明 | 何时改 |
|------|------|------|------|--------|
| `cluster.download` | `s3` \| `local` | `s3` | 仅 `storage.driver=s3` **且** `cache.driver=redis` 时生效 | 选公共直链或节点代拉 |
| `node.display_name` | string | `""` | 管理台节点列表显示名；空则显示 UUID | 便于辨认机器 |

`cluster.download` 决定多节点时客户端拿到公共直链还是节点代拉；`node.display_name` 只影响管理台节点列表。拓扑与何时启用见 [功能指南 · 多节点](/guide/features/cluster)。

<<< @/../../configs/config-example.yaml{54-57}
