# KiriVers Swift Client SDK

本分支是**跳转**，不含 SwiftPM 包源码。SPM 索引的是独立仓库根上的 `Package.swift` 与 SemVer tag，不能指向本仓 `main` 或本分支。

- 发布仓：https://github.com/Kirizu-Official/KiriVers-SDK-Swift.git
- 本仓完整源码（开发与测试）：`sdk/swift-src`

```swift
.package(url: "https://github.com/Kirizu-Official/KiriVers-SDK-Swift.git", from: "0.1.0")
```

独立仓尚未创建时，从 `sdk/swift-src` 拷到新仓根目录再打 `1.0.0` / `v1.0.0`。不要在本仓 KiriVers 上打 Swift 版本 tag。步骤见默认分支 [`docs/sdk-publish.md`](https://github.com/Kirizu-Official/KiriVers/blob/main/docs/sdk-publish.md)。
