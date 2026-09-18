# Kirizu.KiriVers.Client

Official **KiriVers** native JSON client SDK for .NET 8 (`net8.0`). It talks to the client plane (`openapi.client.json`), not the admin plane and not store feeds (Sparkle / WinGet / …).

Install from a packed `.nupkg` or a project reference to this tree (the `sdk/csharp` branch of [Kirizu-Official/KiriVers](https://github.com/Kirizu-Official/KiriVers)):

```bash
dotnet add package Kirizu.KiriVers.Client
```

This package is **not** published from the server default branch. NuGet identity is `Kirizu.KiriVers.Client`; tags on this repo for SDK releases are `sdk-csharp-v*`.

## Quick start

`device_id` is always caller-supplied. The SDK never generates a device identity and never writes a raw `device_id` to logs.

```csharp
using Kirizu.KiriVers.Client;

var client = new Client(new ClientOptions
{
    BaseUrl = "http://127.0.0.1:8080",
    ProjectRef = "my-app",
    // ProjectToken = "...",   // when the project sets require_client_token
    // ChannelToken = "...",   // X-Channel-Token for hidden channels
});

var check = await client.CheckAsync(new CheckRequest
{
    CurrentVersion = "1.0.0",
    Os = "windows",
    Arch = "x86_64",
    Channel = "stable",
    DeviceId = myDeviceId,
});

if (check.HasUpdate)
{
    var pkg = await client.DownloadAsync(check.Body!.PackageUrl!);
    var sha = new BclHasher().Sha256(pkg.Body);
    // sha should equal check.Body.Sha256
}

var updater = new Updater(client);
var result = await updater.RunAsync(new UpdateRequest
{
    CurrentVersion = "1.0.0",
    Os = "windows",
    Arch = "x86_64",
    Channel = "stable",
    DeviceId = myDeviceId,
    StageDirectory = @"C:\App\updates",
    ApplyPath = @"C:\App\app.bin", // omit to stage only; see Replacer below
});
```

HTTP 204 on check is “already up to date”, not an error. HTTP 304 is an ETag hit.

## Native JSON API

| Method | Client call |
|--------|-------------|
| `GET /api/v1/projects/{ref}` | `GetProjectAsync` |
| `POST .../clients/report` | `ReportDeviceAsync` |
| `POST .../update/check` | `CheckAsync` |
| `GET .../changelog/{channel}/{os}/{arch}` | `GetChangelogAsync` |
| `GET .../versions/{version}/integrity` | `GetIntegrityAsync` |
| `POST .../update/diff` | `DiffAsync` |
| `POST .../update/pack` (enqueue **and** poll) | `PackAsync` / `PackUntilReadyAsync` |
| `GET`/`HEAD` `.../packages/{sha256}` | `DownloadAsync` / `HeadAsync` / `DownloadPackageAsync` |
| `POST .../telemetry/report` | `ReportTelemetryAsync` (202; updater never fails on it) |
| `GET .../channels` `matrix` `languages` | `GetChannelsAsync` / `GetMatrixAsync` / `GetLanguagesAsync` |
| `GET .../announcements` | `GetAnnouncementsAsync` |
| `GET`/`HEAD` `.../media/{id}` | `GetMediaAsync` / `HeadMediaAsync` |
| `GET /api/v1/health` | `GetHealthAsync` |

Not implemented (out of SDK): `/store/...`, `GET /update/check`, `POST /clients/login`, `POST /update/pack/status`, `GET .../manifest`, admin plane.

Private downloads keep `?exp=&sig=` on the URL. `Range` is passed through.

Errors parse `{ "error": { "code", "message", "details" } }` into `ApiException.Code` (unknown codes stay opaque strings).

## Adapters (D18 / stdlib)

No runtime `PackageReference`. JSON is `System.Text.Json` (not an adapter).

| Adapter | Default | Caller must inject to enable |
|---------|---------|------------------------------|
| `ITransport` | `HttpClient` | — |
| `IFileStore` | `System.IO` (`LocalFileStore`) | Disable default to omit `file_list` |
| `IHasher` | `SHA256` / `MD5` | — |
| `IArchiveUnpacker` | `System.IO.Compression.ZipArchive` | Disable default to omit `patch_package` |
| `ISignatureVerifier` | RSA-SHA256 (`RSA`) + Ed25519 (RFC 8032, BCL math) | Configure `SigningPublicKeyPem` to enforce `signature` |
| `IPatcher` | **none** | Inject to send `binary_delta` + `accepted_delta_algos` |
| `IReplacer` | `File.Replace`; Windows in-use files: P/Invoke `MoveFileEx` (replace now, or delay until reboot) | Inject to replace APK / custom layouts |

### Capabilities (D13)

Default check body includes `full_package`. With the stock adapters it also sends `patch_package` (zip unpacker) and `file_list` (file store can write individual files). It does **not** send `binary_delta` or `accepted_delta_algos` unless a live `IPatcher` advertises algorithms.

`local_sha256` is never sent on check; it is only used on `POST /update/diff`.

Inject a Patcher:

```csharp
sealed class MyPatcher : IPatcher
{
    public IReadOnlyList<string> SupportedAlgos { get; } = [Capability.Bsdiff];
    public byte[] Apply(ReadOnlySpan<byte> oldFile, ReadOnlySpan<byte> delta, string algo)
    {
        if (DeltaMagic.Identify(delta) is not { } magic || magic != algo)
            throw new VerifyException("unknown or mismatched delta magic");
        // call your existing bsdiff apply here
        throw new NotImplementedException();
    }
}
```

Unknown magics (`KVDIFFHP1\n`, `HDIFF13&`, `BSDIFF40`, VCDIFF `D6 C3 C4`) must not be cross-decoded. The updater falls back to the full package.

### Replacer / “install”

The default `FileReplaceReplacer` replaces **one on-disk path**. Occupied Windows executables may be swapped with `MoveFileEx`; if the file stays locked, replacement is scheduled for the next reboot.

That is **not** one-click product install, not APK sideload, and not an MSI. Android / HarmonyOS installers stay caller-owned `IReplacer` implementations. Omit `UpdateRequest.ApplyPath` to download-and-verify only.

## OpenAPI snapshot

`openapi.client.json` plus `OPENAPI_REVISION` (`info.version` and a short content hash) live at the package root. Contract tests load that snapshot; they do not start Docker.

## Tests

```bash
dotnet test
```

Integration tests expect a live client plane at `http://127.0.0.1:8080` and `D:\KiriVers\configs\sdk-fixture.json` (or `KIRIVERS_SDK_FIXTURE`). Docker Compose is only for the server’s Postgres/Redis, not an SDK dependency.
