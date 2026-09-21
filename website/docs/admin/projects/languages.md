---
title: "语言"
description: "信封 { languages: [] }。创建要 default_locale。上限 16。客户端 GET /languages 独立。"
---

# 语言

导航 **语言**。管理信封 `{ languages: [] }`。创建需要项目默认语言。上限 16。不能删除最后一门语言或当前 **默认**（400 <ErrorCode code="INVALID_REQUEST" />）。重复代码 <ErrorCode code="LANGUAGE_TAKEN" />。未知 <ErrorCode code="LANGUAGE_NOT_FOUND" />。

1. 打开 **语言**。
2. 点 **添加语言**。
3. 填 **语言代码**（2–32 位，大小写按输入保留，例如 `zh-CN`）、可选 **显示名称**、**排序**。需要时勾选 **默认**。
4. changelog / 公告编辑器切换语言会保留其它语言文案；保存时不得丢掉未选中语言的已有键。

客户端 `GET /languages` 独立于公告。概念：[功能指南 · 语言](/guide/features/languages)。
