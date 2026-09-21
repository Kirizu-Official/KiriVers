---
title: "项目创建"
description: "显示名 name 不是 project_ref。slug、compare_engine、默认语言。发布后比较引擎不可改。"
---

# 项目创建

谁能打开：能看见 **项目管理** 的账号。创建项目按钮仅平台管理员（项目账号 403 <ErrorCode code="FORBIDDEN" />）。

1. **项目管理** → **新建项目**。
2. 填写下表，点 **创建**。
3. 点 **进入项目** 打开概览。

| 字段 | 界面 | 含义 | 失败 |
|------|------|------|------|
| 项目名称 `name` | **项目名称** | 控制台标题，**不是** `project_ref` | 过长 400 <ErrorCode code="INVALID_REQUEST" /> |
| Slug | **Slug** | 公开 URL 标识；改名后旧 slug 作为别名直到过期 | 重复活 slug 被拒绝 |
| 比较引擎 | **比较引擎** | **语义化版本** 或 **整数构建号**。**首次发布后不可更改** | <ErrorCode code="COMPARE_ENGINE_IMMUTABLE" /> |
| 默认语言 | **默认语言** | 客户端缺文案时的回退 | 非法代码 400 |
| 拥有者用户名 | **项目拥有者用户名** | 可选。已有账号则绑定 | — |
| 拥有者密码 | **拥有者密码** | 仅创建新账号时需要，至少 8 位 | — |

客户端 `project_ref` 可以是 UUID、活 slug 或未过期别名。过期别名 404 <ErrorCode code="PROJECT_NOT_FOUND" />。概念：[版本发布](/guide/features/versions)。
