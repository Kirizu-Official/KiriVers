---
title: "网站 / Node"
description: "分发的是安装包对象，不能用来刷新网页升级站点。浏览器网页不能用 TypeScript SDK。"
---

# 网站 / Node

KiriVers 分发的是**安装包对象**（SHA-256 文件），不能把「刷新网页」当成站点升级通道。

## 何时使用

- 站点静态资源、SPA 构建产物：用自己的 CDN / 流水线，不要走 KiriVers check。
- 同仓里的 **Node 服务进程**（守护进程、CLI）：走原生 JSON **POST** check。
- 桌面壳（Electron）包住网站：安装包走 [Electron](./electron) listing 或壳内自制客户端。

官方 TypeScript SDK **没有** browser export，不能在浏览器里调用 check。

## 配置入口

Node 进程：项目 **设置** 的速率限制 / Token，以及 [检查更新](/guide/features/check)。Electron 壳：项目 **设置 → 商店协议** → **新建上架**。

## 客户端职责

| 形态 | 做法 |
|------|------|
| 浏览器里的网页应用 | 不要使用 TypeScript SDK。站点静态资源用自己的 CDN / 构建流水线 |
| Node 服务端程序 | 按 [服务与命令行](./server-cli) **POST** check |
| Electron 壳 | [Electron](./electron) 商店 listing，或壳内自制客户端走原生 JSON |

## 故障

浏览器里 `import` 官方 SDK 会失败（无 browser 构建）。把站点 HTML 当「安装包」上传后，刷新网页也不会走 check。Node 进程必须 **POST**；GET check 是 404。

相关：[检查更新](/guide/features/check)、[SDK](/api/sdk/)。
