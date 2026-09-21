---
title: "C# SDK"
description: "Kirizu.KiriVers.Client。CheckAsync。无运行时 PackageReference。MoveFileEx 说明。"
---

# C#

`net8.0`。NuGet `Kirizu.KiriVers.Client`。无运行时 PackageReference。JSON：`System.Text.Json`（非适配器）。

## 1. 安装

```bash
dotnet add package Kirizu.KiriVers.Client
```

::: warning 尚未上架 NuGet
对该 `sdk/csharp` 分支做 project reference。
:::

## 2. Quick start

```csharp
using var client = new Client(new ClientOptions {
    BaseUrl = "http://127.0.0.1:8080",
    ProjectRef = "my-app",
});
var check = await client.CheckAsync(new CheckRequest {
    CurrentVersion = "1.0.0",
    Os = "windows",
    Arch = "x86_64",
    Channel = "stable",
    DeviceId = "your-stable-device-id",
});
if (!check.HasUpdate) {
    return; // HTTP 204 / 304
}
```

## 3. 下载并核对

`DownloadAsync(check.Body.PackageUrl)` + `BclHasher().Sha256`。

## 4. Updater

`Updater.RunAsync`。省略 `ApplyPath` 即只暂存。

## 5. Client 方法

`GetProjectAsync`、`ReportDeviceAsync`、`CheckAsync`、`GetChangelogAsync`、`GetIntegrityAsync`、`DiffAsync`、`PackAsync` / `PackUntilReadyAsync`、Download/Head、catalogs、announcements、telemetry、media、`GetHealthAsync`。不实现：`/store/`、GET check、`clients/login`、`GET /ready`、manifest、pack/status、artifact filename。

## 6. 配置与鉴权

`ClientOptions.BaseUrl`、`ProjectRef`、`ProjectToken`、`ChannelToken`、验签 PEM。

## 7. 适配器

`HttpClient`、`LocalFileStore`、SHA256/MD5、`ZipArchive`、RSA+Ed25519。Patcher **无默认**。Replacer 默认 `File.Replace`；占用文件 P/Invoke `MoveFileEx`。

## 8. 能力位

默认 `full_package` + 有 zip/文件时的 `patch_package`/`file_list`。无 IPatcher 则无 `binary_delta`。

## 9. Patcher

实现 `IPatcher`。magic 不交叉解码。

## 10. Replacer

`MoveFileEx` 可延后到重启。仍不是 MSI/APK 安装器。省略 `ApplyPath` 只暂存。

## 11. 错误

`ApiException.Code`。204/304 不是失败。

## 12. Transport / 测试

注入 `ITransport`。`dotnet test`，不依赖 Docker。

## 13. 语言特有

无运行时 NuGet 依赖。占用文件可用 `MoveFileEx`。仍不是 MSI/APK 安装器。
