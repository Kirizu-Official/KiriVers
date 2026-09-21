---
title: "macOS / Sparkle"
description: "protocol=sparkle。appcast.xml。sparkle:version 是构建号，shortVersionString 是营销号。官方 SDK 不读 appcast。"
---

# macOS / Sparkle

Sparkle、WinSparkle、NetSparkle 共用 protocol `sparkle`。文档名为 `appcast.xml`。官方 SDK 只封装原生 JSON check，**不**解析 appcast。

## 产物形态

macOS（或 Windows 上的 WinSparkle）**单文件**安装包。appcast 每条 item 一个 enclosure，只投影默认硬件变体。`sparkle:edSignature` 签的是 enclosure 文件字节（与原生 check 签名格式隔离）。RSA 项目不发 `sparkle:dsaSignature`（本服务不支持 DSA）。

双号映射**不**跟随项目 `compare_engine`：

| 元素 | 来源 |
|------|------|
| `sparkle:version` | 内部构建号 `version_integer`（空则省略） |
| `sparkle:shortVersionString` | 用户可见 SemVer `version_semver`（空则省略） |

`stable` 渠道不标注 `sparkle:channel`（无该元素即默认渠道）。

## 控制台

1. **设置 → 商店协议** → **新建上架**，协议 `sparkle`。
2. 商店 URL 指向 appcast：

```text
https://updates.example.com/api/v1/projects/myapp/store/sparkle/stable/appcast.xml
```

## 更新器配置

::: code-group

```xml [Info.plist]
<key>SUFeedURL</key>
<string>https://updates.example.com/api/v1/projects/myapp/store/sparkle/stable/appcast.xml</string>
```

```ini [WinSparkle]
FeedURL=https://updates.example.com/api/v1/projects/myapp/store/sparkle/stable/appcast.xml
```

:::

## 故障

| 现象 | 原因 |
|------|------|
| 纯文本 **404** | listing 缺失、未知路径、协议关闭 |
| HTTP 401 <ErrorCode code="UNAUTHORIZED" /> | feed 鉴权失败 |
| 验签失败 | 客户端公钥与 listing / 产物 Ed25519 不匹配；RSA 项目无 DSA 签名 |

相关：[商店订阅](/guide/features/store)、[桌面应用](./desktop)。
