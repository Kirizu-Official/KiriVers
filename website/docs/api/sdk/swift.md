---
title: "Swift SDK"
description: "import KiriVersClient。SPM 独立仓。ZIPFoundation。不要把主仓加进 Xcode。iOS 不是 IPA 安装器。"
---

# Swift

SPM URL：`https://github.com/Kirizu-Official/KiriVers-SDK-Swift.git`。**不要**把 `Kirizu-Official/KiriVers` 加进 Xcode。运行时第三方仅 ZIPFoundation。CryptoKit 故 Linux `swift` 镜像编不过。

## 1. 安装

```swift
.package(url: "https://github.com/Kirizu-Official/KiriVers-SDK-Swift.git", from: "0.1.0")
```

::: warning 独立仓尚未公开
clone `-b sdk/swift-src` 后 path 包。不要 SPM 本仓 `main` 或 `sdk/swift` 跳转分支。
:::

## 2. Quick start

`import KiriVersClient`

```swift
let client = Client(configuration: ClientConfiguration(
    baseURL: URL(string: "http://127.0.0.1:8080")!,
    projectRef: "my-app"
))
let outcome = try await client.check(CheckRequest(
    currentVersion: "1.0.0",
    os: "macos",
    arch: "arm64",
    channel: "stable",
    deviceId: callerOwnedDeviceId
))
switch outcome {
case .update(let check, _):
    print(check.packageURL)
case .noUpdate, .notModified:
    break // HTTP 204 / 304
}
```

传入调用方保存的 `deviceId`。不要把本服务器仓库加进 Xcode。

## 3. 下载并核对

`client.download(url:)` + `CryptoKitHasher().sha256`。

## 4. Updater

`Updater(client:).run`。无 Replacer 时 `result.outcome == .staged`。

## 5. Client 方法

Project、DeviceReport、Check、Changelog、Integrity、Diff、Pack 同 POST 轮询、Download GET+HEAD、catalogs、Announcements、Telemetry、Media、Health。不实现：`/store/`、GET check、`clients/login`、`GET /ready`、manifest、pack/status、artifact filename。

## 6. 配置与鉴权

`ClientConfiguration(baseURL, projectRef, projectToken, channelToken, signingPublicKeyPEM)`。

## 7. 适配器

Transport URLSession。FileStore FileManager。Hasher CryptoKit。JSON Foundation。ArchiveUnpacker ZIPFoundation。SignatureVerifier CryptoKit+Security。Patcher **仅协议**。Replacer：macOS `moveItem`；**iOS 无默认 IPA 安装**。

## 8. 能力位

默认 FileStore+ZIP 会报 `full_package`/`patch_package`/`file_list`。无 Patcher 无 `binary_delta`。

## 9. Patcher

注入 `supportedAlgos` + `apply`。magic 不交叉解码。

## 10. Replacer

macOS 可替换你拥有的文件树。iOS **不能**当 IPA 安装器，须自备 Replacer。不是 App Store 私有协议。

## 11. 错误

抛出的错误携带 `code`。`.noUpdate` 不是失败。

## 12. Transport / 测试

注入 Transport。`swift test` 需要 Apple 工具链。不依赖 Docker。

## 13. 语言特有

SPM 独立仓；ZIPFoundation；CryptoKit 故 Linux swift 镜像编不过；iOS 无 IPA 安装。
