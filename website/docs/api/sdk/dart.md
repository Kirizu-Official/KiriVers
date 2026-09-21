---
title: "Dart SDK"
description: "pub.dev kirivers_client。check.isNoUpdate。Flutter 应用可用，Web / dart:html 不行。"
---

# Dart

Dart 3。Flutter **应用**（VM/Android/iOS/桌面）可用。**Web 不行**：默认文件适配器使用 `dart:io`。Flutter SDK 不是包依赖。

## 1. 安装

```text
dart pub add kirivers_client
```

::: warning 尚未上架 pub.dev
clone `-b sdk/dart` 后 path 依赖。
:::

## 2. Quick start

```dart
final client = Client(
  config: ClientConfig(
    baseUrl: 'http://127.0.0.1:8080',
    projectRef: 'my-app',
  ),
);
final check = await client.check(CheckRequest(
  currentVersion: '1.0.0',
  os: 'android',
  arch: 'arm64',
  channel: 'stable',
  deviceId: appDeviceId,
));
if (check.isNoUpdate || check.isNotModified) {
  return; // HTTP 204 / 304
}
```

Flutter 应用可用；Web / `dart:html` 不行。

## 3. 下载并核对

`client.downloadUrl(check.body!.packageUrl)` + `CryptoHasher().sha256Hex`。

## 4. Updater

`Updater(client: client).run(UpdatePlan(..., apply: true, installPath: ...))`。默认 `File.rename` 不是 APK 安装器。

## 5. Client 方法

与 README Native JSON 表一致：check、download、integrity、pack、telemetry、catalogs、announcements、health。不实现：`/store/`、GET check、`clients/login`、`GET /ready`、manifest、pack/status、artifact filename。

## 6. 配置与鉴权

`ClientConfig`：`baseUrl`、`projectRef`、`projectToken`、`channelToken`、`signingPublicKeyPem`。

## 7. 适配器

HTTP `http` 包。文件 `dart:io`。Hasher/crypto。archive zip。Patcher 仅接口。Replacer `FileRenameReplacer`。

## 8. 能力位

按活适配器。无 Patcher 无 `binary_delta`。

## 9. Patcher

注入后才申报。magic 不交叉解码。

## 10. Replacer

Android/HarmonyOS APK、占用 Windows 文件须自备。Web 不支持。

## 11. 错误

读取 `error.code`。`isNoUpdate` 不是失败。

## 12. Transport / 测试

注入 Transport。不依赖 Docker。

## 13. 语言特有

`dart:io` 文件适配器；Web 不行；Flutter SDK 不是包依赖。
