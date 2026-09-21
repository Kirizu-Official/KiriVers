---
title: "移动应用"
description: "Android 侧载按单文件发布。F-Droid 是 listing。不能代替 App Store / Play 上架审核。"
---

# 移动应用

## 产物形态

Android **侧载**把 APK 当**单个文件**发布（平台矩阵单文件）。官方 SDK 不是 APK 安装器：须自备 Replacer。比较与下载仍是 **POST** check + SHA-256 对象。

F-Droid 订阅是另一条商店 listing（`fdroid`），与原生 JSON **不是**同一 URL。索引文档：

::: code-group

```text [index-v1]
https://updates.example.com/api/v1/projects/myapp/store/fdroid/stable/index-v1.json
```

```text [index-v2]
https://updates.example.com/api/v1/projects/myapp/store/fdroid/stable/index-v2.json
```

:::

控制台：**设置 → 商店协议** → **新建上架**，协议 `fdroid`。未知路径（例如 `index.xml`）纯文本 404。官方 SDK **不**读 F-Droid 索引。

iPhone 企业签 / 侧载可以走原生 JSON 或自定义分发，**不能**代替 App Store 审核。Play / App Store 私有协议不在本产品范围。

## 故障

缺 listing → 纯文本 404。原生 check 失败码见 [FAQ · 失败代码](/faq/errors)。

相关：[商店订阅](/guide/features/store)、[桌面应用](./desktop)。
