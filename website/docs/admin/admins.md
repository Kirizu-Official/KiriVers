---
title: "用户管理"
description: "平台管理员与项目成员。LAST_ADMIN / LAST_OWNER。项目成员看不到 /admins /geoip /nodes。"
---

# 用户管理

抽屉 **管理员用户** 仅**平台管理员**可见。项目成员请求 `/admins`、`/geoip`、`/nodes` 得到 403 <ErrorCode code="FORBIDDEN" />。

## 平台管理员

实例级账号。可用 [kirivers admin add](/guide/config/bootstrap-admin) 或本页创建。不能删除最后一名平台管理员（<ErrorCode code="LAST_ADMIN" />）。用户名占用：<ErrorCode code="USERNAME_TAKEN" />。目标不存在：<ErrorCode code="ADMIN_NOT_FOUND" />。

1. 抽屉打开 **管理员用户**。
2. 点 **新建管理员**，填写 **用户名** 与 **密码**（至少 8 位）。
3. **改名 / 改密**：密码留空表示不修改。
4. 列表 **从未登录** 对应 JSON `last_login_at` 为 `null`。列还有 **上次登录时间** / **上次登录 IP**、类型芯片 **平台管理员** / **项目账号**。
5. **删除管理员** 前确认不是最后一名。文案：「实例至少需要保留一名管理员。」

## 项目成员

在项目 **设置** 底部 **项目成员** 卡片添加。角色：**拥有者** / **管理员**。不能移除最后一名拥有者（<ErrorCode code="LAST_OWNER" />）。未知成员：<ErrorCode code="MEMBER_NOT_FOUND" />。项目成员看不到全局抽屉里的管理员 / GeoIP / 节点。

概念：[访问密钥](/guide/features/tokens)。
