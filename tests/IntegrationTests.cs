using System.Text.Json;
using Kirizu.KiriVers.Client;
using Xunit;

namespace Kirizu.KiriVers.Client.Tests;

public class IntegrationTests
{
    static readonly string FixturePath =
        Environment.GetEnvironmentVariable("KIRIVERS_SDK_FIXTURE")
        ?? @"D:\KiriVers\configs\sdk-fixture.json";

    [Fact]
    public async Task Check110ThenDownloadMatchesSha256()
    {
        Assert.True(File.Exists(FixturePath), "sdk-fixture.json is missing at " + FixturePath);
        using var doc = JsonDocument.Parse(await File.ReadAllTextAsync(FixturePath));
        var root = doc.RootElement;
        var baseUrl = root.GetProperty("client_base_url").GetString()!;
        var project = root.GetProperty("project_ref").GetString()!;
        var channel = root.GetProperty("channel").GetString()!;
        var os = root.GetProperty("os").GetString()!;
        var arch = root.GetProperty("arch").GetString()!;
        var current = root.GetProperty("current_version").GetString()!;
        var target = root.GetProperty("target_version").GetString()!;
        var wantSha = root.GetProperty("sha256").GetProperty("1.1.0").GetString()!;
        var deviceId = "sdk-csharp-" + Guid.NewGuid().ToString("N");

        using var client = new Client(new ClientOptions
        {
            BaseUrl = baseUrl,
            ProjectRef = project,
            FileStoreRoot = Path.Combine(Path.GetTempPath(), "kv-int-" + Guid.NewGuid().ToString("N")),
        });

        var check = await client.CheckAsync(new CheckRequest
        {
            CurrentVersion = current,
            Os = os,
            Arch = arch,
            Channel = channel,
            DeviceId = deviceId,
        });

        Assert.True(check.HasUpdate, "expected an update from " + current);
        Assert.NotNull(check.Body);
        Assert.Equal(target, check.Body!.VersionSemver);
        Assert.False(string.IsNullOrEmpty(check.Body.PackageUrl));
        Assert.Equal(wantSha, check.Body.Sha256, StringComparer.OrdinalIgnoreCase);

        var downloaded = await client.DownloadAsync(check.Body.PackageUrl!);
        var got = new BclHasher().Sha256(downloaded.Body);
        Assert.Equal(wantSha, got, StringComparer.OrdinalIgnoreCase);

        var updater = new Updater(client);
        var staged = Path.Combine(Path.GetTempPath(), "kv-int-stage-" + Guid.NewGuid().ToString("N"));
        var result = await updater.RunAsync(new UpdateRequest
        {
            CurrentVersion = current,
            Os = os,
            Arch = arch,
            Channel = channel,
            DeviceId = deviceId,
            StageDirectory = staged,
        });
        Assert.Equal(wantSha, result.Sha256, StringComparer.OrdinalIgnoreCase);
        Assert.False(result.Applied);
        Assert.True(File.Exists(result.StagedPath));
    }
}
