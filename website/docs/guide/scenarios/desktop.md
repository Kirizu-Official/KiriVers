---
title: "桌面应用"
description: "单文件可选二进制差量；多文件目录走 pack。商店更新器见 Electron / Sparkle / Tauri 专页。"
---

# 桌面应用

自制桌面客户端走原生 JSON：**POST** `/api/v1/projects/{project_ref}/update/check`。商店更新器（electron-updater、Sparkle、Tauri updater）走 listing URL，见对应专页。

## 产物形态

| 形态 | 平台矩阵 `package_type` | 增量 |
|------|-------------------------|------|
| 单个 exe / dmg / AppImage | 单文件 | 可申报 `binary_delta` 并 `POST /update/diff`。算法见 [增量更新](/guide/features/incremental) |
| 多文件目录 | 多文件 | `GET` integrity 对照本地，再 `POST /update/pack`。不对每个 dll/so 做二进制差量。过大返回 200 `full_package` |

控制台步骤：[平台矩阵](/admin/projects/matrix)、[版本发布](/admin/projects/release)。

## 客户端职责

- 方法必须是 **POST**。GET check 不是本产品的查询接口。
- 下载 `package_url` 后核对 SHA-256。未知 magic 回退整包。
- 可选 `POST .../telemetry/report`（202）；失败不得阻断安装。

相关：[检查更新](/guide/features/check)、[Electron](./electron)、[Sparkle](./sparkle)、[Tauri](./tauri)。
