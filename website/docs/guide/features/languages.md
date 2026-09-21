---
title: "语言"
description: "默认语言、changelog 与公告文案。上限 16。客户端 GET /languages 独立。"
---

# 语言

项目语言目录给 changelog 与公告提供行。上限 16。每个项目恰好一个默认语言。

## 何时使用

需要多语言更新说明或公告时使用。比较引擎与版本号不受语言影响。

## 配置入口

项目 → **语言**。步骤：[管理 · 语言](/admin/projects/languages)。创建项目时的默认语言见 [项目创建](/admin/projects/create)。

## 规则与错误码

创建需要 `default_locale`。不能删除最后一门或当前默认（400 <ErrorCode code="INVALID_REQUEST" />）。重复代码 <ErrorCode code="LANGUAGE_TAKEN" />。未知 <ErrorCode code="LANGUAGE_NOT_FOUND" />。客户端 `GET /languages` 独立于公告。公告显式 `locale` 严格匹配。编辑器切换语言时不得丢掉其它语言已有键。

相关：[公告](./announcements)、[更新说明](./changelog)。
