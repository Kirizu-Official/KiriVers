---
title: "Electron"
description: "protocol=electron。generic provider 读取 latest.yml / latest-mac.yml / latest-linux.yml。官方 SDK 不读该 feed。"
---

# Electron

Electron 应用使用 **electron-updater generic provider** 拉取 YAML feed。后台 protocol 为 `electron`。

官方 SDK 只封装原生 JSON check，**不**读取 `latest.yml`。自制非 electron-updater 客户端应走 [检查更新](/guide/features/check)。

## 产物形态

按平台发布 **单文件** 安装包（NSIS / DMG / AppImage 等）。yml 只投影该 os 的默认硬件变体 `kind=full` 产物。未知文档名（不是下面三份 YAML）为纯文本 404。

| 文档 | 对应 os |
|------|---------|
| `latest.yml` | windows |
| `latest-mac.yml` | macos |
| `latest-linux.yml` | linux |

generic provider 需要 **electron-updater ≥ 6**（读取 `files[]`；`path` 为 `{sha256}{ext}`，下载走 `/packages/` 哈希 URL，不在商店文档目录下提供安装包）。

## 控制台

1. 项目 **设置 → 商店协议**，点 **新建上架**。
2. 协议选 `electron`，记下 **商店 URL**（含 listing slug）。
3. 钉死的 os / arch / channel 覆盖冲突的 query。

步骤细节：[项目设置](/admin/projects/settings)。

Listing 根形如：

```text
https://updates.example.com/api/v1/projects/myapp/store/electron/stable
```

更新器会再请求 `{根}/latest.yml`（Windows）、`latest-mac.yml`、`latest-linux.yml`。

## 更新器配置

把控制台给出的商店 URL 填进 generic `url`。不要改 electron-updater 源码去拼接路径。

::: code-group

```yaml [electron-builder]
publish:
  provider: generic
  url: https://updates.example.com/api/v1/projects/myapp/store/electron/stable
```

```js [setFeedURL]
autoUpdater.setFeedURL({
  provider: 'generic',
  url: 'https://updates.example.com/api/v1/projects/myapp/store/electron/stable',
})
```

:::

## 故障

| 现象 | 原因 |
|------|------|
| 纯文本 **404**（无 JSON `code`） | listing 不存在、protocol 关闭、未知文件名、旧路径无 slug |
| HTTP 401 <ErrorCode code="UNAUTHORIZED" /> | feed 鉴权失败 |
| 无更新 / 空渠道 | 匿名 feed 只含 `GrayIsComplete()` 的版本；空可见集不发空 yml |

相关：[商店订阅](/guide/features/store)、[API · 商店 feed](/api/client/store)。
