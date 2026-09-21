---
title: "Dart SDK"
description: "pub.dev kirivers_client. check.isNoUpdate. Flutter apps yes; Web / dart:html no."
---

# Dart

Dart 3. Flutter **apps** (VM/Android/iOS/desktop) work. **Web does not**: default file adapters use `dart:io`. The Flutter SDK is not a package dependency.

## 1. Install

```text
dart pub add kirivers_client
```

::: warning Not on pub.dev yet
Clone `-b sdk/dart` and path-depend.
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

Flutter apps are supported; Web / `dart:html` are not.

## 3. Download and verify

`client.downloadUrl(check.body!.packageUrl)` + `CryptoHasher().sha256Hex`.

## 4. Updater

`Updater(client: client).run(UpdatePlan(..., apply: true, installPath: ...))`. Default `File.rename` is not an APK installer.

## 5. Client methods

check, download, integrity, pack, telemetry, catalogs, announcements, health (see the README table). Not implemented: `/store/`, GET check, `clients/login`, `GET /ready`, manifest, pack/status, artifact filename.

## 6. Config and auth

`ClientConfig`: `baseUrl`, `projectRef`, `projectToken`, `channelToken`, `signingPublicKeyPem`.

## 7. Adapters

HTTP `http` package. Files `dart:io`. Hasher/crypto. archive zip. Patcher interface only. Replacer `FileRenameReplacer`.

## 8. Capabilities

From live adapters. No Patcher → no `binary_delta`.

## 9. Patcher

Advertise only after inject. Do not cross-decode magics.

## 10. Replacer

Android/HarmonyOS APK and busy Windows files are caller-owned. Web unsupported.

## 11. Errors

Read `error.code`. `isNoUpdate` is not a failure.

## 12. Transport / tests

Inject Transport. No Docker.

## 13. Language notes

`dart:io` file adapters; no Web; Flutter SDK is not a package dependency.
