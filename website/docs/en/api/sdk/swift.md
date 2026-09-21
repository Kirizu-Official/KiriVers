---
title: "Swift SDK"
description: "import KiriVersClient. Dedicated SPM repo. ZIPFoundation. Do not add the server repo in Xcode. iOS is not an IPA installer."
---

# Swift

SPM URL: `https://github.com/Kirizu-Official/KiriVers-SDK-Swift.git`. Do **not** add `Kirizu-Official/KiriVers` in Xcode. Third-party runtime: ZIPFoundation only. CryptoKit means Linux `swift` images will not compile this package.

## 1. Install

```swift
.package(url: "https://github.com/Kirizu-Official/KiriVers-SDK-Swift.git", from: "0.1.0")
```

::: warning Dedicated GitHub repository not public yet
Clone `-b sdk/swift-src` and use a path package. Do not SPM this server repo `main` or pointer branch `sdk/swift`.
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

Pass a caller-owned `deviceId`. Do not add the server repository in Xcode.

## 3. Download and verify

`client.download(url:)` + `CryptoKitHasher().sha256`.

## 4. Updater

`Updater(client:).run`. Without a Replacer, `result.outcome == .staged`.

## 5. Client methods

Project, DeviceReport, Check, Changelog, Integrity, Diff, Pack (same POST poll), Download GET+HEAD, catalogs, Announcements, Telemetry, Media, Health. Not implemented: `/store/`, GET check, `clients/login`, `GET /ready`, manifest, pack/status, artifact filename.

## 6. Config and auth

`ClientConfiguration(baseURL, projectRef, projectToken, channelToken, signingPublicKeyPEM)`.

## 7. Adapters

Transport URLSession. FileStore FileManager. Hasher CryptoKit. JSON Foundation. ArchiveUnpacker ZIPFoundation. SignatureVerifier CryptoKit+Security. Patcher **protocol only**. Replacer: macOS `moveItem`; **no default IPA install on iOS**.

## 8. Capabilities

Default FileStore+ZIP send `full_package`/`patch_package`/`file_list`. No Patcher → no `binary_delta`.

## 9. Patcher

Inject `supportedAlgos` + `apply`. Do not cross-decode magics.

## 10. Replacer

macOS can replace file trees you own. iOS is **not** an IPA installer; inject a Replacer. Not App Store private APIs.

## 11. Errors

Thrown errors carry `code`. `.noUpdate` is not a failure.

## 12. Transport / tests

Inject Transport. `swift test` needs an Apple toolchain. No Docker.

## 13. Language notes

Dedicated SPM repo; ZIPFoundation; CryptoKit blocks Linux swift images; iOS has no IPA installer.
