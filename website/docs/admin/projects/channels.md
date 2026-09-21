---
title: "渠道"
description: "系统渠道不可删。填错渠道 Token 不是 403。slug 创建后不可改。"
---

# 渠道

导航 **渠道管理**。系统渠道 `alpha` / `beta` / `stable` 不可删（<ErrorCode code="SYSTEM_CHANNEL" />）。显示名在创建项目时按当时 UI 语言写入（中文界面为 **内测版** / **公测版** / **正式版**）。自定义渠道 slug 创建后不可改。未知渠道 <ErrorCode code="CHANNEL_NOT_FOUND" />。

1. 打开 **渠道管理**。
2. **新建渠道**：填 **渠道名**（slug，3–64 位小写字母数字连字符）、**显示名**、**稳定级**。
3. 可选勾选 **不公开列出**；填写或 **清除渠道令牌**。
4. 低稳渠道上的已发布版本可用版本页 **渠道晋升** 升到更稳渠道。
5. 需要覆盖安装规则时点 **安装策略**。

| 字段 | 界面 | 含义 |
|------|------|------|
| slug | **渠道名** | check 与 listing 使用；创建后不可改 |
| unlisted | **不公开列出** | 不进公开目录、不会被自动选为目标，除非客户端已在该渠道或请求指定 slug |
| token | **渠道令牌** | 客户端 `X-Channel-Token` 填错时 check **跳过该隐藏渠道**，不是 403。changelog 不匹配为 404 |

SemVer 预发布后缀须与渠道一致，否则 <ErrorCode code="CHANNEL_SUFFIX_MISMATCH" />。stable 不能用预发布后缀。概念：[功能指南 · 渠道](/guide/features/channels)。
