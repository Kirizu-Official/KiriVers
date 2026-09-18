using System.Text;
using System.Text.Json;
using Kirizu.KiriVers.Client;
using Xunit;

namespace Kirizu.KiriVers.Client.Tests;

public class ContractTests
{
    static readonly string OpenApiPath = Path.Combine(AppContext.BaseDirectory, "openapi.client.json");

    static readonly HashSet<string> Implemented =
    [
        "GET /api/v1/health",
        "GET /api/v1/projects/{project_ref}",
        "POST /api/v1/projects/{project_ref}/clients/report",
        "POST /api/v1/projects/{project_ref}/update/check",
        "GET /api/v1/projects/{project_ref}/changelog/{channel}/{os}/{arch}",
        "GET /api/v1/projects/{project_ref}/versions/{version}/integrity",
        "POST /api/v1/projects/{project_ref}/update/diff",
        "POST /api/v1/projects/{project_ref}/update/pack",
        "GET /api/v1/projects/{project_ref}/packages/{ref}",
        "HEAD /api/v1/projects/{project_ref}/packages/{ref}",
        "GET /api/v1/projects/{project_ref}/channels",
        "GET /api/v1/projects/{project_ref}/matrix",
        "GET /api/v1/projects/{project_ref}/languages",
        "GET /api/v1/projects/{project_ref}/announcements",
        "POST /api/v1/projects/{project_ref}/telemetry/report",
        "GET /api/v1/projects/{project_ref}/media/{id}",
        "HEAD /api/v1/projects/{project_ref}/media/{id}",
    ];

    [Fact]
    public void OpenApiNativePathsAreImplemented()
    {
        using var doc = JsonDocument.Parse(File.ReadAllText(OpenApiPath));
        var missing = new List<string>();
        foreach (var path in doc.RootElement.GetProperty("paths").EnumerateObject())
        {
            if (path.Name.Contains("/store/", StringComparison.Ordinal) ||
                path.Name.EndsWith("/openapi.json", StringComparison.Ordinal))
            {
                continue;
            }

            foreach (var method in path.Value.EnumerateObject())
            {
                var m = method.Name.ToUpperInvariant();
                if (m is "OPTIONS" or "PARAMETERS")
                {
                    continue;
                }

                var key = m + " " + path.Name;
                if (!Implemented.Contains(key))
                {
                    missing.Add(key);
                }
            }
        }

        Assert.True(missing.Count == 0, "unimplemented OpenAPI operations: " + string.Join(", ", missing));
    }

    [Fact]
    public async Task HandwrittenClientEmitsNativePathsAndRequiredFields()
    {
        var t = new RecordingTransport { Handler = CheckLike };
        var client = new Client(new ClientOptions
        {
            BaseUrl = TestClient.Base,
            ProjectRef = TestClient.Project,
            Transport = t,
            ProjectToken = "proj-token",
            ChannelToken = "chan-token",
            FileStoreRoot = Path.Combine(Path.GetTempPath(), "kv-" + Guid.NewGuid().ToString("N")),
        });

        await client.GetHealthAsync();
        AssertHit(t, "GET", "/api/v1/health");

        await client.GetProjectAsync();
        AssertHit(t, "GET", "/api/v1/projects/sdk-fixture");

        await client.ReportDeviceAsync(new DeviceReportRequest { DeviceId = "dev-1", Os = "windows", Arch = "x86_64" });
        AssertHit(t, "POST", "/clients/report");
        AssertJsonHas(t.LastJson, "device_id");

        t.Handler = Check200;
        var check = await client.CheckAsync(new CheckRequest
        {
            CurrentVersion = "1.0.0",
            Os = "windows",
            Arch = "x86_64",
            Channel = "stable",
            DeviceId = "dev-1",
            IfNoneMatch = "\"abc\"",
        });
        Assert.True(check.HasUpdate);
        AssertHit(t, "POST", "/update/check");
        using (var body = JsonDocument.Parse(t.LastJson))
        {
            var root = body.RootElement;
            Assert.Equal("1.0.0", root.GetProperty("current_version").GetString());
            Assert.Equal("windows", root.GetProperty("os").GetString());
            Assert.Equal("x86_64", root.GetProperty("arch").GetString());
            Assert.False(root.TryGetProperty("local_sha256", out _));
            Assert.False(root.TryGetProperty("dirty_paths", out _));
            var caps = root.GetProperty("capabilities").EnumerateArray().Select(x => x.GetString()).ToArray();
            Assert.Contains(Capability.FullPackage, caps);
            Assert.Contains(Capability.PatchPackage, caps);
            Assert.Contains(Capability.FileList, caps);
            Assert.DoesNotContain(Capability.BinaryDelta, caps);
            Assert.False(root.TryGetProperty("accepted_delta_algos", out _));
        }

        Assert.Equal("Bearer proj-token", t.Last.Headers!["Authorization"]);
        Assert.Equal("proj-token", t.Last.Headers["X-Project-Token"]);
        Assert.Equal("chan-token", t.Last.Headers["X-Channel-Token"]);
        Assert.Equal("\"abc\"", t.Last.Headers["If-None-Match"]);

        t.Handler = Json200("""{"changelog":"hi"}""");
        await client.GetChangelogAsync("stable", "windows", "x86_64", new ChangelogQuery
        {
            FromVersion = "1.0.0",
            ToVersion = "1.1.0",
            ChangelogScope = "range_all",
            ChangelogLayout = "both",
        });
        AssertHit(t, "GET", "/changelog/stable/windows/x86_64");
        Assert.Contains("from_version=1.0.0", t.Last.Url);
        Assert.Contains("changelog_scope=range_all", t.Last.Url);

        t.Handler = Json200("""{"files":[],"package_type":"single_file","root_hash":"","full_package_url":"/p","file_name":"a","size":1,"sha256":"ab","version_integer":null,"version_semver":"1.1.0","channel":"stable"}""");
        await client.GetIntegrityAsync(new IntegrityQuery { Version = "1.1.0", Os = "windows", Arch = "x86_64" });
        AssertHit(t, "GET", "/versions/1.1.0/integrity");
        Assert.Contains("os=windows", t.Last.Url);
        Assert.Contains("arch=x86_64", t.Last.Url);

        t.Handler = Json200("""{"diff_mode":"full_package","root_hash":"","version_integer":null,"version_semver":"1.1.0","channel":"stable","compare_engine":"semver"}""");
        await client.DiffAsync(new DiffRequest
        {
            SourceVersion = "1.0.0",
            TargetVersion = "1.1.0",
            Os = "windows",
            Arch = "x86_64",
            LocalSha256 = "abc",
        });
        AssertHit(t, "POST", "/update/diff");
        using (var diff = JsonDocument.Parse(t.LastJson))
        {
            Assert.Equal("1.0.0", diff.RootElement.GetProperty("source_version").GetString());
            Assert.Equal("1.1.0", diff.RootElement.GetProperty("target_version").GetString());
            Assert.Equal("abc", diff.RootElement.GetProperty("local_sha256").GetString());
        }

        t.Handler = JsonStatus(202, """{"status":"pending"}""");
        var packOnce = await client.PackAsync(new PackRequest
        {
            SourceVersion = "1.0.0",
            TargetVersion = "1.1.0",
            Os = "windows",
            Arch = "x86_64",
            NeededPaths = ["foo/bar"],
        });
        Assert.Equal("pending", packOnce.Status);
        AssertHit(t, "POST", "/update/pack");
        using (var pack = JsonDocument.Parse(t.LastJson))
        {
            Assert.Equal("1.0.0", pack.RootElement.GetProperty("source_version").GetString());
            Assert.Equal("1.1.0", pack.RootElement.GetProperty("target_version").GetString());
            Assert.True(pack.RootElement.TryGetProperty("needed_paths", out _));
        }

        t.Handler = Bytes200("pkg"u8.ToArray());
        await client.DownloadPackageAsync("7f063a6901ebfd0aa7b1adb8af58f4e792f13310f42213767ced623eb0669969", range: "bytes=0-10");
        AssertHit(t, "GET", "/packages/7f063a6901ebfd0aa7b1adb8af58f4e792f13310f42213767ced623eb0669969");
        Assert.Equal("bytes=0-10", t.Last.Headers!["Range"]);

        await client.HeadPackageAsync("7f063a6901ebfd0aa7b1adb8af58f4e792f13310f42213767ced623eb0669969");
        AssertHit(t, "HEAD", "/packages/");

        t.Handler = Json200("""{"channels":[]}""");
        await client.GetChannelsAsync();
        AssertHit(t, "GET", "/channels");

        t.Handler = Json200("""{"matrix":[]}""");
        await client.GetMatrixAsync();
        AssertHit(t, "GET", "/matrix");

        t.Handler = Json200("""{"languages":[]}""");
        await client.GetLanguagesAsync();
        AssertHit(t, "GET", "/languages");

        t.Handler = Json200("""{"announcements":[]}""");
        await client.GetAnnouncementsAsync(new AnnouncementQuery { Version = "1.0.0", Os = "windows", Arch = "x86_64", Locale = "en" });
        AssertHit(t, "GET", "/announcements");
        Assert.Contains("version=1.0.0", t.Last.Url);

        t.Handler = JsonStatus(202, """{"status":"accepted"}""");
        await client.ReportTelemetryAsync(new TelemetryRequest
        {
            Os = "windows",
            Arch = "x86_64",
            Channel = "stable",
            FromVersion = "1.0.0",
            ToVersion = "1.1.0",
            Status = "installed",
            DeviceId = "dev-1",
        });
        AssertHit(t, "POST", "/telemetry/report");
        using (var tel = JsonDocument.Parse(t.LastJson))
        {
            Assert.Equal("installed", tel.RootElement.GetProperty("status").GetString());
            Assert.Equal("windows", tel.RootElement.GetProperty("os").GetString());
            Assert.Equal("1.0.0", tel.RootElement.GetProperty("from_version").GetString());
        }

        t.Handler = Bytes200("img"u8.ToArray());
        await client.GetMediaAsync("aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee");
        AssertHit(t, "GET", "/media/aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee");
        await client.HeadMediaAsync("aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee");
        AssertHit(t, "HEAD", "/media/");

        Assert.DoesNotContain(t.Requests, r =>
            r.Method.Equals("GET", StringComparison.OrdinalIgnoreCase) && r.Url.Contains("/update/check"));
        Assert.DoesNotContain(t.Requests, r => r.Url.Contains("/clients/login"));
        Assert.DoesNotContain(t.Requests, r => r.Url.Contains("/update/pack/status"));
        Assert.DoesNotContain(t.Requests, r => r.Url.Contains("/store/"));
        Assert.DoesNotContain(t.Requests, r => r.Url.Contains("/manifest"));
    }

    [Fact]
    public async Task Check204And304AreNotErrors()
    {
        var t = new RecordingTransport { Handler = _ => new TransportResponse { StatusCode = 204, Body = [] } };
        var client = new Client(new ClientOptions
        {
            BaseUrl = TestClient.Base,
            ProjectRef = TestClient.Project,
            Transport = t,
            UseDefaultAdapters = false,
            Hasher = new BclHasher(),
        });
        var none = await client.CheckAsync(new CheckRequest { CurrentVersion = "1.1.0", Os = "windows", Arch = "x86_64" });
        Assert.True(none.NoUpdate);
        Assert.Equal(204, none.StatusCode);

        t.Handler = r => new TransportResponse
        {
            StatusCode = 304,
            Body = [],
            Headers = new Dictionary<string, string> { ["ETag"] = "\"x\"" },
        };
        var nm = await client.CheckAsync(new CheckRequest { CurrentVersion = "1.1.0", Os = "windows", Arch = "x86_64", IfNoneMatch = "\"x\"" });
        Assert.True(nm.NotModified);
    }

    [Fact]
    public async Task ErrorEnvelopeSurfacesOpaqueCode()
    {
        var t = new RecordingTransport
        {
            Handler = _ => new TransportResponse
            {
                StatusCode = 404,
                Body = """{"error":{"code":"VERSION_NOT_FOUND","message":"no such version","details":{"ref":"9.9.9"}}}"""u8.ToArray(),
            },
        };
        var client = new Client(new ClientOptions
        {
            BaseUrl = TestClient.Base,
            ProjectRef = TestClient.Project,
            Transport = t,
            UseDefaultAdapters = false,
            Hasher = new BclHasher(),
        });
        var ex = await Assert.ThrowsAsync<ApiException>(() =>
            client.CheckAsync(new CheckRequest { CurrentVersion = "9.9.9", Os = "windows", Arch = "x86_64" }));
        Assert.Equal("VERSION_NOT_FOUND", ex.Code);
        Assert.Equal("no such version", ex.ErrorMessage);
        Assert.Equal(404, ex.StatusCode);
        Assert.Equal(JsonValueKind.Object, ex.Details!.Value.ValueKind);

        t.Handler = _ => new TransportResponse
        {
            StatusCode = 418,
            Body = """{"error":{"code":"IM_A_TEAPOT","message":"short and stout"}}"""u8.ToArray(),
        };
        var opaque = await Assert.ThrowsAsync<ApiException>(() => client.GetProjectAsync());
        Assert.Equal("IM_A_TEAPOT", opaque.Code);
    }

    [Fact]
    public async Task DownloadKeepsSignedQuery()
    {
        var t = new RecordingTransport { Handler = Bytes200("x"u8.ToArray()) };
        var client = new Client(new ClientOptions
        {
            BaseUrl = TestClient.Base,
            ProjectRef = TestClient.Project,
            Transport = t,
            UseDefaultAdapters = false,
            Hasher = new BclHasher(),
        });
        var url = TestClient.Base + "/api/v1/projects/sdk-fixture/packages/abcd?exp=1&sig=deadbeef";
        await client.DownloadAsync(url);
        Assert.Contains("exp=1", t.Last.Url);
        Assert.Contains("sig=deadbeef", t.Last.Url);
    }

    static void AssertHit(RecordingTransport t, string method, string pathFragment)
    {
        Assert.Equal(method, t.Last.Method, StringComparer.OrdinalIgnoreCase);
        Assert.Contains(pathFragment, t.Last.Url);
    }

    static void AssertJsonHas(string json, string name)
    {
        using var doc = JsonDocument.Parse(json);
        Assert.True(doc.RootElement.TryGetProperty(name, out _));
    }

    static TransportResponse CheckLike(TransportRequest req) => Json200("{}")(req);

    static Func<TransportRequest, TransportResponse> Check200 => Json200(
        """{"has_update":true,"is_mandatory":false,"is_downgrade":false,"reason":"normal","compare_engine":"semver","version_integer":null,"version_semver":"1.1.0","target_channel":"stable","target_hw_rev":null,"package_type":"single_file","root_hash":"","package_url":"/api/v1/projects/sdk-fixture/packages/ab","file_name":"a.bin","size":51,"sha256":"ab","delta_available":false}""");

    static Func<TransportRequest, TransportResponse> Json200(string json) =>
        _ => new TransportResponse { StatusCode = 200, Body = Encoding.UTF8.GetBytes(json) };

    static Func<TransportRequest, TransportResponse> JsonStatus(int status, string json) =>
        _ => new TransportResponse { StatusCode = status, Body = Encoding.UTF8.GetBytes(json) };

    static Func<TransportRequest, TransportResponse> Bytes200(byte[] bytes) =>
        _ => new TransportResponse { StatusCode = 200, Body = bytes };
}
