---
title: "商店订阅"
description: "listing slug URL。缺 listing 纯文本 404。官方 SDK 不读取商店 feed。"
---

# 商店订阅

商店 listing 让 Electron、Sparkle、Tauri 等**现成更新器**拉取协议文档。listing 身份是 slug。URL：

`/api/v1/projects/{project_ref}/store/{protocol}/{listing_slug}`，可选 `/{doc}`。

## 何时使用

应用已经内置商店更新器时使用。自制客户端应走 [检查更新](./check) 原生 JSON。官方 SDK **不**读取商店 feed。

## 配置入口

项目 **设置 → 商店协议**（**新建上架**）。步骤：[项目设置](/admin/projects/settings)。YAML 无 protocol 表。

## 规则与错误码

钉死 os/arch/channel 后忽略冲突的 query。缺 listing、未知 protocol、旧路径无 slug → **纯文本 404**（无 JSON `code`）。Feed 鉴权失败 401 <ErrorCode code="UNAUTHORIZED" />。重复 `(protocol, slug)` 400 <ErrorCode code="INVALID_REQUEST" />。匿名 feed 只含 `GrayIsComplete()` 的版本。

九种 protocol：`sparkle`、`electron`、`tauri`、`squirrel`、`clickonce`、`appimage`、`winget`、`msix`、`fdroid`。

## 客户端 / 更新器职责

把控制台给出的商店 URL 填进更新器。不要给应用打源码级补丁去改 feed 路径。

相关：[Electron](/guide/scenarios/electron)、[Sparkle](/guide/scenarios/sparkle)、[Tauri](/guide/scenarios/tauri)、[API · 商店 feed](/api/client/store)。
