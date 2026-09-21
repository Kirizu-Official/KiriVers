---
title: "Kotlin SDK"
description: "official.kirizu:kirivers-client-kotlin。CheckResult 密封类。无默认 Replacer。不是 Java 包装。"
---

# Kotlin

独立工件 `official.kirizu:kirivers-client-kotlin`，**不是** Java 包包装。JSON：kotlinx.serialization。不要 Jackson/Gson/OkHttp/Ktor Client。

## 1. 安装

Gradle `implementation("official.kirizu:kirivers-client-kotlin:REPLACE_WITH_RELEASE")`（JDK 17）。

::: warning 尚未上架
```text
git clone -b sdk/kotlin https://github.com/Kirizu-Official/KiriVers.git kirivers-client-kotlin
./gradlew publishToMavenLocal
```
:::

## 2. Quick start

```kotlin
val client = Client(
    Config(baseUrl = "http://127.0.0.1:8080", projectRef = "my-app"),
)
when (val result = client.check(CheckRequest(
    currentVersion = "1.0.0", os = "windows", arch = "x86_64",
    channel = "stable", deviceId = appDeviceId,
))) {
    is CheckResult.UpToDate -> { }      // 204
    is CheckResult.NotModified -> { }   // 304
    is CheckResult.Update -> { }
}
```

## 3. 下载并核对

`client.download(result.update.packageUrl)` + `JdkHasher().sha256Hex`。

## 4. Updater

`Updater(client, fileStore = JdkFileStore(stagingDir)).run(...)`。无默认 Replacer：缺省只暂存。

## 5. Client 方法

`project()`、`reportDevice()`、`check()`、`changelog()`、`integrity()`、`diff()`、`pack()` / `packUntilReady()`、download/head、`channels()`/`matrix()`/`languages()`、`announcements()`、`reportTelemetry()`、media、`health()`。不实现：`/store/`、GET check、`clients/login`、`GET /ready`、manifest、pack/status、artifact filename URLs。

## 6. 配置与鉴权

`ClientConfig(baseUrl, projectRef, projectToken, channelToken)`。渠道 Token 填错不会在 check 上 403。

## 7. 适配器

Transport：`JdkHttpTransport`。JSON：kotlinx.serialization（非适配器）。Hasher/FileStore/JdkZipUnpacker/JdkSignatureVerifier。Patcher 与 Replacer **仅接口**。

## 8. 能力位

默认 `full_package`。FileStore 可写 → `file_list`；ArchiveUnpacker → `patch_package`；Patcher → `binary_delta`。

## 9. Patcher

无 JNI。magic 规则同概念页。

## 10. Replacer

无默认实现。Windows `MoveFileEx`、APK 安装均须自备。

## 11. 错误

读取信封 `code`。204/304 映射为密封类，不是失败。

## 12. Transport / 测试

注入 Transport。不依赖 Docker。

## 13. 语言特有

不是 Java 包装。kotlinx.serialization。无默认 Replacer。
