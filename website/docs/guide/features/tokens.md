---
title: "访问密钥"
description: "项目 Token、CI Token、商店 listing Token、渠道 Token、管理会话。明文只在创建响应出现。"
---

# 访问密钥

密钥按平面与作用域分开。明文只在**创建响应**出现，之后 GET 不回显。

## 何时使用

公开项目可不要求项目 Token。私有检查、隐藏渠道、CI 发版、商店 feed 鉴权时再开对应密钥。

## 配置入口

- **Token 管理**：项目 Token、CI Token。[管理 · Token](/admin/projects/tokens)
- **设置 → 安全**：商店 Token、**客户端检查需要 Token**
- **渠道管理**：渠道令牌
- 管理会话：登录 Cookie/Bearer，TTL 见 [安全相关配置](/guide/config/security)

## 规则与错误码

| 种类 | 作用域 | 失败形态 |
|------|--------|----------|
| 项目 Token | `require_client_token` 时客户端平面；`X-Project-Token` 或 `Authorization: Bearer` | 401 <ErrorCode code="UNAUTHORIZED" /> |
| CI Token | 管理平面 CI 上传/发版 | 与 CI 路由一致；无权限 403 <ErrorCode code="FORBIDDEN" /> |
| 商店 listing Token | 商店 feed 鉴权（StoreAuth） | 401；缺 listing 为纯文本 404 |
| 渠道 Token | `X-Channel-Token`；隐藏渠道 | check **不是** 403，跳过该渠道 |
| 管理会话 | Cookie/Bearer；空闲滑动 TTL | 401；项目成员碰 `/admins` 等为 403 <ErrorCode code="FORBIDDEN" /> |

## 客户端职责

令牌不要写入日志。丢失只能作废再创建。

相关：[自动发版](/api/ci)、[商店订阅](./store)。
