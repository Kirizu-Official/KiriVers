---
title: "C# SDK"
description: "Kirizu.KiriVers.Client. CheckAsync. No runtime PackageReference. MoveFileEx notes."
---

# C#

`net8.0`. NuGet `Kirizu.KiriVers.Client`. No runtime PackageReference. JSON: `System.Text.Json` (not an adapter).

## 1. Install

```bash
dotnet add package Kirizu.KiriVers.Client
```

::: warning Not on NuGet yet
Project-reference the `sdk/csharp` branch.
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

## 3. Download and verify

`DownloadAsync(check.Body.PackageUrl)` + `BclHasher().Sha256`.

## 4. Updater

`Updater.RunAsync`. Omit `ApplyPath` to stage only.

## 5. Client methods

`GetProjectAsync`, `ReportDeviceAsync`, `CheckAsync`, changelog/integrity/diff/pack, download/head, catalogs, announcements, telemetry, media, `GetHealthAsync`. Not implemented: `/store/`, GET check, `clients/login`, `GET /ready`, manifest, pack/status, artifact filename.

## 6. Config and auth

`ClientOptions.BaseUrl`, `ProjectRef`, `ProjectToken`, `ChannelToken`, verify PEM.

## 7. Adapters

`HttpClient`, `LocalFileStore`, SHA256/MD5, `ZipArchive`, RSA+Ed25519. No default Patcher. Replacer: `File.Replace`; busy files use `MoveFileEx`.

## 8. Capabilities

`full_package` plus `patch_package`/`file_list` when zip/files exist. No `binary_delta` without `IPatcher`.

## 9. Patcher

Implement `IPatcher`. Do not cross-decode magics.

## 10. Replacer

`MoveFileEx` may delay until reboot. Still not an MSI/APK installer. Omit `ApplyPath` to stage.

## 11. Errors

`ApiException.Code`. 204/304 are success.

## 12. Transport / tests

Inject `ITransport`. `dotnet test` without Docker.

## 13. Language notes

No runtime NuGet deps. Busy files may use `MoveFileEx`. Not an MSI/APK installer.
