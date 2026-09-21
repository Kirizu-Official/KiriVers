---
title: "场景总览"
description: "按产物形态分流：单文件 vs 多文件目录，再选原生 JSON 或商店 feed。"
---

# 场景总览

先确定安装包形态，再选协议。

- **原生 JSON**：应用 **POST** [检查更新](/guide/features/check)。官方 SDK 只封装这条路径。
- **商店 feed**：应用内已有 Electron / Sparkle / Tauri 等更新器时，创建商店 listing，把 URL 交给更新器。官方 SDK **不**读取商店 feed。

| 形态 | 去向 |
|------|------|
| 单个 exe / dmg / AppImage | [桌面应用](./desktop) 或对应商店页 |
| 多文件目录 | [桌面应用](./desktop) 多文件节；不对每个 dll 做二进制差量 |
| Electron | [Electron](./electron)（electron-updater generic） |
| Tauri | [Tauri](./tauri) |
| Sparkle / WinSparkle | [macOS / Sparkle](./sparkle) |
| Squirrel / ClickOnce / WinGet / MSIX | [其他桌面更新器](./desktop-feeds) |
| Android 侧载 / F-Droid | [移动应用](./mobile) |
| 固件 | [MCU](./mcu) |
| 无 UI 服务 | [服务与命令行](./server-cli) |
| 网站本身 | [网站 / Node](./web-node) |

相关：[商店订阅](/guide/features/store)。
