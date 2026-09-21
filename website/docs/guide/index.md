---
title: "简介"
description: "KiriVers 是自托管软件更新服务：你发布产物，应用来检查并下载更新。"
---

# 简介

KiriVers 是**自托管软件更新服务**。你发布安装包或固件；用户设备上的应用向本服务查询并下载更新。它不是 git，也不能替代 App Store、Google Play 或 Microsoft Store 的上架审核。

## 运行形态

单进程提供两个平面（plane）：

| 平面 | 默认地址 | 职责 |
|------|----------|------|
| 客户端平面 | `:8080` | 检查更新、下载、商店 feed、遥测、公告 |
| 管理平面 | `:8081` | 管理台 SPA 与 CI Agent |

官方 Releases 可执行文件已 `go:embed` 管理台。使用者不必再部署前端静态文件。PostgreSQL 必需；Redis 可选（见[安装](/guide/install/)与[缓存](/guide/config/cache)）。

CLI 仅有 `kirivers` / `kirivers server`，以及本机 `kirivers admin add|delete|list|reset-password|clear-2fa`。没有发版子命令，也没有 `kirivers listen`。

## 接入两条路

1. **原生 JSON**：应用直接调用客户端平面（`POST .../update/check`）。适合自制桌面、服务、固件客户端。官方 [SDK](/api/sdk/) 只封装这条路径。
2. **商店订阅**：在管理台创建 listing，把 URL 填进 Electron / Sparkle / Tauri 等现成更新器。原生 SDK **不**读 feed。

## 做不到的事

- 代替各商店的上架与审核。
- 充当 Linux 系统软件源（APT / RPM / Flatpak）。
- 用管理平面 `GET /` 托管本站文档。

## 阅读顺序

1. [选择安装方式](/guide/install/) → 配置 YAML → [创建第一个管理员](/guide/config/bootstrap-admin)
2. 登录后按 [管理 → 快速上手](/admin/quick-start) 发出第一个版本
3. 用 [功能指南](/guide/features/) 理解渠道、灰度、增量、公告等
4. 再按软件类型阅读 [使用场景](/guide/scenarios/)
