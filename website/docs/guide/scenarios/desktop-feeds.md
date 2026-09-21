---
title: "其他桌面更新器"
description: "Squirrel RELEASES、ClickOnce .application、AppImage .zsync、WinGet REST、MSIX .appinstaller。官方 SDK 不读这些 feed。"
---

# 其他桌面更新器

这些协议与原生 JSON check **并行**：更新器拉 listing 文档；官方 SDK 不读取它们。缺 listing、未知文档名 → **纯文本 404**。钉死 os/arch/channel 的规则见 [商店订阅](/guide/features/store)。

控制台统一入口：项目 **设置 → 商店协议** → **新建上架**。

| 更新器 | protocol | 填进更新器的地址 |
|--------|----------|------------------|
| Squirrel | `squirrel` | `{listing}/RELEASES`（路径大小写敏感，必须是 `RELEASES`） |
| ClickOnce | `clickonce` | `{listing}/MyApp.application`（必须以 `.application` 结尾） |
| AppImageUpdate | `appimage` | `{listing}/latest.zsync`（必须以 `.zsync` 结尾） |
| WinGet REST 源 | `winget` | listing 根；文档 `information`、`manifestSearch`、`packageManifests` |
| MSIX / App Installer | `msix` | `{listing}/app.appinstaller`（必须以 `.appinstaller` 结尾） |

把 `https://updates.example.com` 与 `myapp` / listing slug 换成控制台给出的值。

::: code-group

```text [Squirrel]
https://updates.example.com/api/v1/projects/myapp/store/squirrel/stable/RELEASES
```

```text [ClickOnce]
https://updates.example.com/api/v1/projects/myapp/store/clickonce/stable/MyApp.application
```

```text [AppImage]
https://updates.example.com/api/v1/projects/myapp/store/appimage/stable/latest.zsync
```

```text [WinGet]
https://updates.example.com/api/v1/projects/myapp/store/winget/stable
```

```text [MSIX]
https://updates.example.com/api/v1/projects/myapp/store/msix/stable/app.appinstaller
```

:::

WinGet 的 PackageIdentifier 放在 listing identifiers。MSIX 可用 identifiers `appinstaller_uri` 覆盖根 Uri。Squirrel 的 RELEASES 用相对文件名；私有签名 URL **不**写入 RELEASES 行。

## 故障

纯文本 404：listing 不存在、协议关闭、文档名不符合上表。鉴权失败 401 <ErrorCode code="UNAUTHORIZED" />。

相关：[Electron](./electron)、[Sparkle](./sparkle)、[Tauri](./tauri)。
