---
title: "Java SDK"
description: "official.kirizu:kirivers-client。Config.builder()。可选 FilesMoveReplacer。不要加 OkHttp/Gson。"
---

# Java

## 1. 安装

Maven 坐标 `official.kirizu:kirivers-client`（JDK 17）。运行时仅 Jackson databind。

```xml
<dependency>
  <groupId>official.kirizu</groupId>
  <artifactId>kirivers-client</artifactId>
  <version>REPLACE_WITH_RELEASE</version>
</dependency>
```

::: warning 尚未上架 Maven Central
```text
git clone -b sdk/java --single-branch https://github.com/Kirizu-Official/KiriVers.git kirivers-client-java
cd kirivers-client-java && ./mvnw install
```
:::

## 2. Quick start

```java
var client = new Client(Config.builder()
    .baseUrl("http://127.0.0.1:8080")
    .projectRef("my-app")
    .build());
var result = client.check(UpdateCheckRequest.builder()
    .currentVersion("1.0.0")
    .os("windows")
    .arch("x86_64")
    .channel("stable")
    .deviceId("your-stable-device-id")
    .build());
if (result.isNoUpdate() || result.isNotModified()) {
    return; // HTTP 204 / 304
}
```

`official.kirizu.kirivers.client.Client` + `Config.builder()`。传入应用自己的 `deviceId`。

## 3. 下载并核对

按 `packageUrl` 下载，比对 SHA-256。保留 `exp`/`sig`。

## 4. Updater

`new Updater(client).update(upd)`。未设 Replacer 且未传 `targetPath` 时只暂存到 `stageDir`。

## 5. Client 方法

project、reportDevice、check、changelog、integrity、diff、pack / packUntilReady、download/head、catalogs、announcements、reportTelemetry、media、health。不实现：`/store/`、GET check、`clients/login`、`GET /ready`、manifest、pack/status、artifact filename URLs。

## 6. 配置与鉴权

`baseUrl`、`projectRef`、`projectToken`、渠道令牌、验签 PEM。不要记录令牌。

## 7. 适配器

Transport 默认 JDK `HttpClient`（Android 缺 `java.net.http` 时注入；**不要加 OkHttp**）。JSON 为 Jackson（不是适配器；不要 Gson）。FileStore nio+NFC。Hasher MessageDigest。SignatureVerifier JDK Ed25519+RSA。ArchiveUnpacker `java.util.zip`。Patcher **仅接口**。Replacer **仅接口**，可选 `FilesMoveReplacer`。

## 8. 能力位

Transport → `full_package`；zip → `patch_package`；可写 FileStore → `file_list`；Patcher.supportedAlgos → `binary_delta`。

## 9. Patcher

无 JNI / 无 hpatchz。注入后才申报算法。magic 不得交叉解码。

## 10. Replacer

桌面可用 `FilesMoveReplacer`。占用 Windows exe 仍可能需要应用自己的助手。Android/HarmonyOS 用 `PackageInstaller`。iOS 不适用本包安装 IPA。

## 11. 错误

`ApiException.code()` 读取 `error.code`。204/304 不是失败。

## 12. Transport / 测试

注入 Transport。契约测试读包内 `openapi.client.json`，不启动 Docker。

## 13. 语言特有

Jackson 写死。Android 注入 Transport。`FilesMoveReplacer` 可选。不要加 OkHttp/Gson。
