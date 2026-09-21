---
title: "SDK"
description: "官方 SDK 只封装原生 JSON。每种语言一页：安装、Quick start、完整参考。"
---

# SDK

官方 SDK 只封装**原生 JSON**（检查更新、下载、校验、可选差量/打包、可选替换）。**不做** Sparkle / electron-updater / WinGet 等商店协议。

阅读顺序：安装 → Quick start 完成一次 `check`；需要自定义 Transport / 差量 / 占用文件替换 / 嵌入式网络时阅读该语言页后半与 [概念](./concepts)。商店协议见指南中的 Electron/Sparkle 等页。

文档示例版本号（如 `0.1.0`）会过时；安装时以 registry 当前版本为准。

## 安装对照

| 语言 | 怎么装进应用 | 最低环境 | 源码 git 分支 |
|------|--------------|----------|---------------|
| Python | `pip install kirivers-client` | Python 3.10+ | `sdk/python` |
| Java | Maven `official.kirizu:kirivers-client` | JDK 17 | `sdk/java` |
| Kotlin | `official.kirizu:kirivers-client-kotlin`（独立包，不是 Java 包装） | JDK 17 | `sdk/kotlin` |
| C# | `dotnet add package Kirizu.KiriVers.Client` | .NET 8 | `sdk/csharp` |
| TypeScript | `npm install @kirizu/kirivers-client` | Node 20+ / Electron 主进程；**不能用于网页** | `sdk/typescript` |
| Rust | crates.io `kirivers-client` | 阻塞 API，不必 tokio | `sdk/rust` |
| Dart | `dart pub add kirivers_client` | Dart 3；Flutter 应用可用、**Web 不行** | `sdk/dart` |
| C | `git clone -b sdk/c https://github.com/Kirizu-Official/KiriVers.git` | C11；CMake hosted 或 embedded | `sdk/c` |
| C++ | `git clone -b sdk/cpp …` | C++17 | `sdk/cpp` |
| Go | `go get github.com/Kirizu-Official/KiriVers-SDK-Go`（**独立仓**） | Go 模块 | 发布仓；本仓源码 `sdk/go-src`；`sdk/go` 仅跳转 |
| PHP | `composer require kirizu/kirivers-client`（仓 `KiriVers-SDK-PHP`） | PHP 8.2+，不要装 Guzzle | `sdk/php-src`；`sdk/php` 仅跳转 |
| Swift | SPM `https://github.com/Kirizu-Official/KiriVers-SDK-Swift.git` | Apple 工具链；不要把主仓加进 Xcode | `sdk/swift-src`；`sdk/swift` 仅跳转 |

::: warning 包尚未上架或独立仓尚未公开
同一语言页给出 clone 对应分支的退路。**禁止** `go get` / Packagist / SPM 指向本仓 `main` 或跳转分支 `sdk/go`、`sdk/php`、`sdk/swift`。
:::

## 所有语言共通

1. 调用方提供 `base_url`、`project_ref`、当前版本、os/arch、渠道，以及由应用生成并稳定保存的 `device_id`（SDK 不生成设备身份）。
2. HTTP 204 / `no_update` 表示已是最新，不是错误；304 为 ETag 命中。
3. 未注入 Replacer 时不得描述为一键安装。
4. 未注入 Patcher 时不得申报 `binary_delta`。
