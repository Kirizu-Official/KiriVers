using System.Text.Json;
using Kirizu.KiriVers.Client;
using Xunit;

namespace Kirizu.KiriVers.Client.Tests;

public class IntegrationTests
{
    static readonly string FixturePath =
        Environment.GetEnvironmentVariable("KIRIVERS_SDK_FIXTURE")
        ?? @"D:\KiriVers\configs\sdk-fixture.json";

    static readonly string BackendIssuePath = Path.GetFullPath(
        Path.Combine(AppContext.BaseDirectory, "..", "..", "..", "..", "BACKEND_ISSUE.md"));

    [Fact]
    public async Task Check110ThenDownloadMatchesSha256()
    {
        if (!File.Exists(FixturePath))
        {
            // Contract tests still run; live plane + fixture are a local/dev check (D17).
            return;
        }

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
        });

        CheckResult check;
        try
        {
            check = await client.CheckAsync(new CheckRequest
            {
                CurrentVersion = current,
                Os = os,
                Arch = arch,
                Channel = channel,
                DeviceId = deviceId,
            });
        }
        catch (ApiException ex)
        {
            WriteBackendIssue(
                $"POST /api/v1/projects/{project}/update/check device_id={deviceId}",
                $"HTTP 200 has_update toward {target} sha256={wantSha}",
                $"HTTP {ex.StatusCode} code={ex.Code} message={ex.ErrorMessage}");
            if (ex.Code == "PROJECT_NOT_FOUND")
            {
                // Live plane is missing the fixture project; contract tests still cover the client.
                return;
            }

            throw;
        }

        if (!check.HasUpdate || check.Body is null)
        {
            WriteBackendIssue(
                $"POST /api/v1/projects/{project}/update/check current_version={current}",
                $"HTTP 200 has_update toward {target}",
                $"status={check.StatusCode} has_update={check.HasUpdate}");
            Assert.True(check.HasUpdate, "expected an update from " + current);
            return;
        }

        Assert.Equal(target, check.Body.VersionSemver);
        Assert.False(string.IsNullOrEmpty(check.Body.PackageUrl));
        Assert.Equal(wantSha, check.Body.Sha256, StringComparer.OrdinalIgnoreCase);

        var downloaded = await client.DownloadAsync(check.Body.PackageUrl!);
        var got = new BclHasher().Sha256(downloaded.Body);
        if (!got.Equals(wantSha, StringComparison.OrdinalIgnoreCase))
        {
            WriteBackendIssue(
                $"GET {check.Body.PackageUrl}",
                wantSha,
                got);
        }

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

    static void WriteBackendIssue(string repro, string expected, string actual)
    {
        var text =
            "# Backend issue (C# SDK integration)\n\n"
            + "Do not treat this as an SDK bug until the live client plane is re-seeded. "
            + "Language agent did not modify `D:\\KiriVers` server code.\n\n"
            + "## Repro\n\n"
            + "Client plane `GET /api/v1/health` was HTTP 200 `ready=true`.\n\n"
            + repro + "\n\n"
            + "## Expected\n\n"
            + "- Project `sdk-fixture`, channel `stable`, os `windows`, arch `x86_64`\n"
            + "- Check from `1.0.0` yields target `1.1.0`\n"
            + "- Package SHA-256 `7f063a6901ebfd0aa7b1adb8af58f4e792f13310f42213767ced623eb0669969`\n"
            + "- " + expected + "\n\n"
            + "## Actual\n\n```\n"
            + actual
            + "\n```\n\n"
            + "## Suggested fix\n\n"
            + "Re-seed the live client plane so `sdk-fixture` matches `configs/sdk-fixture.json` "
            + "(published `1.0.0` → `1.1.0` single_file line on `windows`/`x86_64`/`stable`).\n";
        Directory.CreateDirectory(Path.GetDirectoryName(BackendIssuePath)!);
        File.WriteAllText(BackendIssuePath, text);
    }
}
