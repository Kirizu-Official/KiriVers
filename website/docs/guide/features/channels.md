---
title: "渠道"
description: "系统渠道 alpha/beta/stable 不可删。slug 不可改。填错渠道 Token 不是 403。"
---

# 渠道

每个版本归属**一个**渠道。渠道把内测 / 公测 / 正式分流，并决定商店 listing 能否钉死某一轨。

## 何时使用

需要多轨发布、隐藏渠道或渠道 Token 时使用。单轨产品可只用系统渠道 `stable`。

## 配置入口

项目 → **渠道管理**。步骤：[管理 · 渠道](/admin/projects/channels)。YAML 无渠道表。

## 规则与错误码

| 概念 | 行为 |
|------|------|
| 系统渠道 `alpha` / `beta` / `stable` | 创建项目时写入；**不可删除**（<ErrorCode code="SYSTEM_CHANNEL" />） |
| 自定义 slug | 创建后不可改 |
| `stability_rank` | 数值越大越稳。低稳渠道上的版本可 `promote` |
| `unlisted` | 不进公开目录、不会被自动选为目标，除非客户端已在该渠道或请求指定 slug |
| 渠道 Token / `X-Channel-Token` | check 填错只**跳过该隐藏渠道**，不是 403。changelog 不匹配为 404 |

SemVer 预发布后缀须与渠道一致，否则 <ErrorCode code="CHANNEL_SUFFIX_MISMATCH" />。stable 不能用预发布后缀。未知渠道 <ErrorCode code="CHANNEL_NOT_FOUND" />。

## 客户端职责

公开目录 `GET /channels` 省略 unlisted 与 token-protected 行。隐藏渠道升级用 check `channel=` + `X-Channel-Token`。

相关：[版本发布](./versions)、[安装策略](./install-policy)、[商店订阅](./store)。
