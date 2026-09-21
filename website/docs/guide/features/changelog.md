---
title: "更新说明"
description: "正文只在 GET changelog/{channel}/{os}/{arch}。check 不含正文。"
---

# 更新说明

更新说明正文只在 `GET .../changelog/{channel}/{os}/{arch}`。check 响应不含 changelog。

## 何时使用

客户端要展示版本区间说明时使用。商店协议会把 Version changelog 投影进自己的文档（例如 Sparkle `<description>`）。

## 配置入口

项目 **设置 → 更新日志默认值**（范围、排版、条数）。点击路径：[项目设置](/admin/projects/settings)。实例天花板：`changelog.default_entries` / `max_entries`，见 [config.yaml](/guide/config/config.yaml)。YAML `default > max` 拒绝启动。

## 规则与错误码

| 条件 | HTTP | 码 |
|------|------|-----|
| 非法 scope/layout 或非法 `from_version` | 400 | <ErrorCode code="CHANGELOG_QUERY_INVALID" /> |
| 未知路径渠道 | 400 | <ErrorCode code="INVALID_QUERY_PARAM" /> |
| 渠道 Token 不匹配 | 404 | <ErrorCode code="NOT_FOUND" /> |
| 未知 from/to 版本 | 404 | <ErrorCode code="VERSION_NOT_FOUND" /> |
| 项目条数超过实例天花板 | 400 | <ErrorCode code="INVALID_REQUEST" /> |

无 `from_version` 时返回最新 `default_entries` 条（产品默认 5）。有 from 时截断到 `max_entries`（默认 50），溢出仍 200。Markdown `${site_url}` 从 RequestOrigin 展开。

## 客户端职责

check 之后如需正文再 GET changelog。不要把 changelog 查询参数打到 check 上。

相关：[API · 更新说明](/api/client/changelog)。
