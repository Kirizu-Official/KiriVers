using System.Text;

namespace Kirizu.KiriVers.Client;

/// <summary>Caller-supplied configuration. The SDK never invents a device id.</summary>
public sealed class ClientOptions
{
    public required string BaseUrl { get; init; }
    public required string ProjectRef { get; init; }
    public string? ProjectToken { get; init; }
    public string? ChannelToken { get; init; }
    public ITransport? Transport { get; init; }
    public HttpClient? HttpClient { get; init; }
    public IFileStore? FileStore { get; init; }
    public IHasher? Hasher { get; init; }
    public ISignatureVerifier? SignatureVerifier { get; init; }
    public IArchiveUnpacker? ArchiveUnpacker { get; init; }
    public IPatcher? Patcher { get; init; }
    public IReplacer? Replacer { get; init; }
    public string? SigningPublicKeyPem { get; init; }
    public string SigningAlgo { get; init; } = BclSignatureVerifier.Ed25519;
    /// <summary>
    /// When true, fill Transport/Hasher/SignatureVerifier/Replacer with stdlib defaults.
    /// FileStore, ArchiveUnpacker, and Patcher stay off unless injected (D13 check capabilities).
    /// </summary>
    public bool UseDefaultAdapters { get; init; } = true;
    /// <summary>When set (and <see cref="FileStore"/> is omitted), installs a <see cref="LocalFileStore"/> at this root and may advertise <c>file_list</c>.</summary>
    public string? FileStoreRoot { get; init; }
}

/// <summary>Handwritten native JSON client for the KiriVers client plane.</summary>
public sealed class Client : IDisposable
{
    readonly ITransport _transport;
    readonly HttpClientTransport? _ownedTransport;
    readonly bool _ownsTransport;

    public Client(ClientOptions options)
    {
        ArgumentNullException.ThrowIfNull(options);
        if (string.IsNullOrWhiteSpace(options.BaseUrl))
        {
            throw new ArgumentException("BaseUrl is required", nameof(options));
        }

        if (string.IsNullOrWhiteSpace(options.ProjectRef))
        {
            throw new ArgumentException("ProjectRef is required", nameof(options));
        }

        Options = options;
        BaseUrl = options.BaseUrl.TrimEnd('/');
        ProjectRef = options.ProjectRef;

        if (options.Transport is not null)
        {
            _transport = options.Transport;
        }
        else
        {
            _ownedTransport = new HttpClientTransport(options.HttpClient);
            _transport = _ownedTransport;
            _ownsTransport = true;
        }

        Hasher = options.Hasher ?? (options.UseDefaultAdapters ? new BclHasher() : null);
        FileStore = options.FileStore ?? (options.FileStoreRoot is { Length: > 0 }
            ? new LocalFileStore(options.FileStoreRoot)
            : null);
        SignatureVerifier = options.SignatureVerifier ?? (options.UseDefaultAdapters ? new BclSignatureVerifier() : null);
        ArchiveUnpacker = options.ArchiveUnpacker;
        Patcher = options.Patcher;
        Replacer = options.Replacer ?? (options.UseDefaultAdapters ? new FileReplaceReplacer() : null);

        Capabilities = BuildCapabilities();
        AcceptedDeltaAlgos = Patcher is { SupportedAlgos.Count: > 0 }
            ? Patcher.SupportedAlgos.ToArray()
            : [];
    }

    public ClientOptions Options { get; }
    public string BaseUrl { get; }
    public string ProjectRef { get; }
    public ITransport Transport => _transport;
    public IFileStore? FileStore { get; }
    public IHasher? Hasher { get; }
    public ISignatureVerifier? SignatureVerifier { get; }
    public IArchiveUnpacker? ArchiveUnpacker { get; }
    public IPatcher? Patcher { get; }
    public IReplacer? Replacer { get; }
    public IReadOnlyList<string> Capabilities { get; }
    public IReadOnlyList<string> AcceptedDeltaAlgos { get; }

    IReadOnlyList<string> BuildCapabilities()
    {
        var caps = new List<string> { Capability.FullPackage };
        if (ArchiveUnpacker is not null)
        {
            caps.Add(Capability.PatchPackage);
        }

        if (FileStore is { CanWriteIndividualFiles: true })
        {
            caps.Add(Capability.FileList);
        }

        if (Patcher is { SupportedAlgos.Count: > 0 })
        {
            caps.Add(Capability.BinaryDelta);
        }

        return caps;
    }

    public Task<HealthStatus> GetHealthAsync(CancellationToken cancellationToken = default) =>
        GetJsonAsync<HealthStatus>(HttpMethod.Get, "/api/v1/health", null, cancellationToken);

    public Task<ProjectPublic> GetProjectAsync(CancellationToken cancellationToken = default) =>
        GetJsonAsync<ProjectPublic>(HttpMethod.Get, ProjectPath(""), null, cancellationToken);

    public Task<DeviceReportResponse> ReportDeviceAsync(DeviceReportRequest request, CancellationToken cancellationToken = default)
    {
        ArgumentNullException.ThrowIfNull(request);
        if (string.IsNullOrEmpty(request.DeviceId))
        {
            throw new ArgumentException("device_id is required", nameof(request));
        }

        return SendJsonAsync<DeviceReportResponse>(HttpMethod.Post, ProjectPath("/clients/report"), request, expected: 200, cancellationToken);
    }

    public async Task<CheckResult> CheckAsync(CheckRequest request, CancellationToken cancellationToken = default)
    {
        ArgumentNullException.ThrowIfNull(request);
        var caps = request.Capabilities is { Count: > 0 } requestedCaps ? requestedCaps : Capabilities;
        var algos = request.AcceptedDeltaAlgos ?? AcceptedDeltaAlgos;
        var body = new CheckBody
        {
            CurrentVersion = request.CurrentVersion,
            Os = request.Os,
            Arch = request.Arch,
            Channel = request.Channel,
            HwRev = request.HwRev,
            OsVersion = request.OsVersion,
            DeviceId = request.DeviceId,
            Capabilities = caps,
            AcceptedDeltaAlgos = algos.Count == 0 ? null : algos,
        };
        var headers = Headers(ifNoneMatch: request.IfNoneMatch);
        var resp = await SendAsync(HttpMethod.Post, ProjectPath("/update/check"), JsonUtil.Serialize(body), headers, cancellationToken).ConfigureAwait(false);
        var etag = Header(resp, "ETag");
        if (resp.StatusCode == 304)
        {
            return new CheckResult { StatusCode = 304, ETag = etag };
        }

        if (resp.StatusCode == 204)
        {
            return new CheckResult { StatusCode = 204, ETag = etag };
        }

        EnsureSuccess(resp, 200);
        return new CheckResult
        {
            StatusCode = 200,
            ETag = etag,
            Body = JsonUtil.Deserialize<UpdateCheckResponse>(resp.Body),
        };
    }

    public async Task<ChangelogResponse> GetChangelogAsync(
        string channel,
        string os,
        string arch,
        ChangelogQuery? query = null,
        CancellationToken cancellationToken = default)
    {
        var path = ProjectPath($"/changelog/{Enc(channel)}/{Enc(os)}/{Enc(arch)}");
        path = AddQuery(path,
            ("from_version", query?.FromVersion),
            ("to_version", query?.ToVersion),
            ("changelog_scope", query?.ChangelogScope),
            ("changelog_layout", query?.ChangelogLayout),
            ("changelog_include_revoked", Bool(query?.ChangelogIncludeRevoked)),
            ("changelog_include_platform_notes", Bool(query?.ChangelogIncludePlatformNotes)),
            ("changelog_locale", query?.ChangelogLocale),
            ("locale", query?.Locale));
        var resp = await SendAsync(HttpMethod.Get, path, null, Headers(ifNoneMatch: query?.IfNoneMatch), cancellationToken).ConfigureAwait(false);
        if (resp.StatusCode == 304)
        {
            return new ChangelogResponse { NotModified = true, ETag = Header(resp, "ETag") };
        }

        EnsureSuccess(resp, 200);
        var body = JsonUtil.Deserialize<ChangelogResponse>(resp.Body);
        body.ETag = Header(resp, "ETag");
        return body;
    }

    public async Task<IntegrityManifest> GetIntegrityAsync(IntegrityQuery query, CancellationToken cancellationToken = default)
    {
        ArgumentNullException.ThrowIfNull(query);
        var path = ProjectPath($"/versions/{Enc(query.Version)}/integrity");
        path = AddQuery(path,
            ("os", query.Os),
            ("arch", query.Arch),
            ("hash_algo", query.HashAlgo),
            ("compact", Bool(query.Compact)),
            ("include_file_urls", Bool(query.IncludeFileUrls)),
            ("hw_rev", query.HwRev),
            ("channel", query.Channel));
        var resp = await SendAsync(HttpMethod.Get, path, null, Headers(ifNoneMatch: query.IfNoneMatch), cancellationToken).ConfigureAwait(false);
        if (resp.StatusCode == 304)
        {
            return new IntegrityManifest { NotModified = true, ETag = Header(resp, "ETag") };
        }

        EnsureSuccess(resp, 200);
        var body = JsonUtil.Deserialize<IntegrityManifest>(resp.Body);
        body.ETag = Header(resp, "ETag");
        return body;
    }

    public Task<DiffResponse> DiffAsync(DiffRequest request, CancellationToken cancellationToken = default)
    {
        ArgumentNullException.ThrowIfNull(request);
        var body = new
        {
            request.SourceVersion,
            request.TargetVersion,
            request.Os,
            request.Arch,
            request.Channel,
            request.DeviceId,
            request.HwRev,
            request.LocalSha256,
            request.PreferFull,
            Capabilities = request.Capabilities is { Count: > 0 } requestedCaps ? requestedCaps : Capabilities,
            AcceptedDeltaAlgos = (request.AcceptedDeltaAlgos ?? AcceptedDeltaAlgos) is { Count: > 0 } a ? a : null,
        };
        return SendJsonAsync<DiffResponse>(HttpMethod.Post, ProjectPath("/update/diff"), body, 200, cancellationToken);
    }

    public Task<PackResponse> PackAsync(PackRequest request, CancellationToken cancellationToken = default)
    {
        ArgumentNullException.ThrowIfNull(request);
        return PackWithBodyAsync(JsonUtil.Serialize(request), cancellationToken);
    }

    /// <summary>POST pack and poll the same URL with identical JSON until ready, full_package, or error.</summary>
    public async Task<PackResponse> PackUntilReadyAsync(
        PackRequest request,
        PackPollOptions? poll = null,
        CancellationToken cancellationToken = default)
    {
        ArgumentNullException.ThrowIfNull(request);
        poll ??= new PackPollOptions();
        var body = JsonUtil.Serialize(request);
        var delay = poll.InitialDelay;
        var deadline = DateTime.UtcNow + poll.Deadline;
        while (true)
        {
            cancellationToken.ThrowIfCancellationRequested();
            var result = await PackWithBodyAsync(body, cancellationToken).ConfigureAwait(false);
            if (!string.Equals(result.Status, "pending", StringComparison.OrdinalIgnoreCase))
            {
                return result;
            }

            if (DateTime.UtcNow + delay > deadline)
            {
                throw new TimeoutException("pack poll exceeded deadline");
            }

            await Task.Delay(delay, cancellationToken).ConfigureAwait(false);
            var next = delay + delay;
            delay = next > poll.MaxDelay ? poll.MaxDelay : next;
        }
    }

    async Task<PackResponse> PackWithBodyAsync(byte[] body, CancellationToken cancellationToken)
    {
        var resp = await SendAsync(HttpMethod.Post, ProjectPath("/update/pack"), body, Headers(contentJson: true), cancellationToken).ConfigureAwait(false);
        if (resp.StatusCode is not (200 or 202))
        {
            throw ApiException.FromResponse(resp.StatusCode, resp.Body, resp.Headers);
        }

        // HTTP 202 with an empty/unspecified body is still "pending" (design §3.5).
        if (resp.StatusCode == 202 && resp.Body.Length == 0)
        {
            return new PackResponse { Status = "pending" };
        }

        var result = JsonUtil.Deserialize<PackResponse>(resp.Body);
        if (resp.StatusCode == 202 && string.IsNullOrEmpty(result.Status))
        {
            result.Status = "pending";
        }

        return result;
    }

    public Task<BinaryResponse> DownloadAsync(string urlOrPath, string? range = null, CancellationToken cancellationToken = default) =>
        SendBinaryAsync(HttpMethod.Get, Resolve(urlOrPath), range, cancellationToken);

    public Task<BinaryResponse> HeadAsync(string urlOrPath, CancellationToken cancellationToken = default) =>
        SendBinaryAsync(HttpMethod.Head, Resolve(urlOrPath), null, cancellationToken);

    public Task<BinaryResponse> DownloadPackageAsync(string packageRef, string? range = null, string? extraQuery = null, CancellationToken cancellationToken = default)
    {
        var path = ProjectPath($"/packages/{Enc(packageRef)}");
        if (!string.IsNullOrEmpty(extraQuery))
        {
            path += extraQuery.StartsWith('?') ? extraQuery : "?" + extraQuery;
        }

        return SendBinaryAsync(HttpMethod.Get, Resolve(path), range, cancellationToken);
    }

    public Task<BinaryResponse> HeadPackageAsync(string packageRef, string? extraQuery = null, CancellationToken cancellationToken = default)
    {
        var path = ProjectPath($"/packages/{Enc(packageRef)}");
        if (!string.IsNullOrEmpty(extraQuery))
        {
            path += extraQuery.StartsWith('?') ? extraQuery : "?" + extraQuery;
        }

        return SendBinaryAsync(HttpMethod.Head, Resolve(path), null, cancellationToken);
    }

    public Task<ChannelList> GetChannelsAsync(CancellationToken cancellationToken = default) =>
        GetJsonAsync<ChannelList>(HttpMethod.Get, ProjectPath("/channels"), null, cancellationToken);

    public Task<MatrixList> GetMatrixAsync(CancellationToken cancellationToken = default) =>
        GetJsonAsync<MatrixList>(HttpMethod.Get, ProjectPath("/matrix"), null, cancellationToken);

    public Task<LanguageList> GetLanguagesAsync(CancellationToken cancellationToken = default) =>
        GetJsonAsync<LanguageList>(HttpMethod.Get, ProjectPath("/languages"), null, cancellationToken);

    public async Task<AnnouncementList> GetAnnouncementsAsync(AnnouncementQuery? query = null, CancellationToken cancellationToken = default)
    {
        var path = AddQuery(ProjectPath("/announcements"),
            ("version", query?.Version),
            ("os", query?.Os),
            ("arch", query?.Arch),
            ("locale", query?.Locale));
        var headers = Headers(ifNoneMatch: query?.IfNoneMatch);
        if (!string.IsNullOrEmpty(query?.AcceptLanguage))
        {
            headers["Accept-Language"] = query.AcceptLanguage;
        }

        var resp = await SendAsync(HttpMethod.Get, path, null, headers, cancellationToken).ConfigureAwait(false);
        if (resp.StatusCode == 304)
        {
            return new AnnouncementList { NotModified = true, ETag = Header(resp, "ETag") };
        }

        EnsureSuccess(resp, 200);
        var body = JsonUtil.Deserialize<AnnouncementList>(resp.Body);
        body.ETag = Header(resp, "ETag");
        return body;
    }

    public async Task ReportTelemetryAsync(TelemetryRequest request, CancellationToken cancellationToken = default)
    {
        ArgumentNullException.ThrowIfNull(request);
        var resp = await SendAsync(HttpMethod.Post, ProjectPath("/telemetry/report"), JsonUtil.Serialize(request), Headers(contentJson: true), cancellationToken).ConfigureAwait(false);
        if (resp.StatusCode != 202)
        {
            throw ApiException.FromResponse(resp.StatusCode, resp.Body, resp.Headers);
        }
    }

    public Task<BinaryResponse> GetMediaAsync(string id, string? range = null, CancellationToken cancellationToken = default) =>
        SendBinaryAsync(HttpMethod.Get, Resolve(ProjectPath($"/media/{Enc(id)}")), range, cancellationToken);

    public Task<BinaryResponse> HeadMediaAsync(string id, CancellationToken cancellationToken = default) =>
        SendBinaryAsync(HttpMethod.Head, Resolve(ProjectPath($"/media/{Enc(id)}")), null, cancellationToken);

    public void VerifySignature(string payload, string signatureBase64)
    {
        if (SignatureVerifier is null || string.IsNullOrEmpty(Options.SigningPublicKeyPem))
        {
            throw new VerifyException("no SignatureVerifier or signing public key configured");
        }

        SignatureVerifier.Verify(Options.SigningAlgo, Options.SigningPublicKeyPem, payload, signatureBase64);
    }

    async Task<T> GetJsonAsync<T>(HttpMethod method, string path, byte[]? body, CancellationToken cancellationToken)
    {
        var resp = await SendAsync(method, path.StartsWith("http", StringComparison.OrdinalIgnoreCase) ? path : Resolve(path), body, Headers(), cancellationToken).ConfigureAwait(false);
        EnsureSuccess(resp, 200);
        return JsonUtil.Deserialize<T>(resp.Body);
    }

    async Task<T> SendJsonAsync<T>(HttpMethod method, string path, object body, int expected, CancellationToken cancellationToken)
    {
        var resp = await SendAsync(method, Resolve(path), JsonUtil.Serialize(body), Headers(contentJson: true), cancellationToken).ConfigureAwait(false);
        EnsureSuccess(resp, expected);
        return JsonUtil.Deserialize<T>(resp.Body);
    }

    async Task<BinaryResponse> SendBinaryAsync(HttpMethod method, string url, string? range, CancellationToken cancellationToken)
    {
        var headers = Headers();
        if (!string.IsNullOrEmpty(range))
        {
            headers["Range"] = range;
        }

        var resp = await SendAsync(method, url, null, headers, cancellationToken).ConfigureAwait(false);
        if (resp.StatusCode is not (200 or 206))
        {
            throw ApiException.FromResponse(resp.StatusCode, resp.Body, resp.Headers);
        }

        return new BinaryResponse { StatusCode = resp.StatusCode, Body = resp.Body, Headers = resp.Headers };
    }

    async Task<TransportResponse> SendAsync(
        HttpMethod method,
        string url,
        byte[]? body,
        Dictionary<string, string> headers,
        CancellationToken cancellationToken)
    {
        url = Resolve(url);
        if (body is { Length: > 0 } || method == HttpMethod.Post)
        {
            headers.TryAdd("Content-Type", "application/json");
        }

        return await _transport.SendAsync(new TransportRequest
        {
            Method = method.Method,
            Url = url,
            Headers = headers,
            Body = body,
        }, cancellationToken).ConfigureAwait(false);
    }

    Dictionary<string, string> Headers(string? ifNoneMatch = null, bool contentJson = false)
    {
        var h = new Dictionary<string, string>(StringComparer.OrdinalIgnoreCase)
        {
            ["Accept"] = "application/json",
        };
        if (!string.IsNullOrEmpty(Options.ProjectToken))
        {
            h["Authorization"] = "Bearer " + Options.ProjectToken;
            h["X-Project-Token"] = Options.ProjectToken;
        }

        if (!string.IsNullOrEmpty(Options.ChannelToken))
        {
            h["X-Channel-Token"] = Options.ChannelToken;
        }

        if (!string.IsNullOrEmpty(ifNoneMatch))
        {
            h["If-None-Match"] = ifNoneMatch;
        }

        if (contentJson)
        {
            h["Content-Type"] = "application/json";
        }

        return h;
    }

    string ProjectPath(string suffix) =>
        "/api/v1/projects/" + Enc(ProjectRef) + suffix;

    string Resolve(string urlOrPath)
    {
        if (Uri.TryCreate(urlOrPath, UriKind.Absolute, out var abs) &&
            (abs.Scheme == Uri.UriSchemeHttp || abs.Scheme == Uri.UriSchemeHttps))
        {
            return abs.AbsoluteUri;
        }

        if (urlOrPath.StartsWith('/'))
        {
            return BaseUrl + urlOrPath;
        }

        return BaseUrl + "/" + urlOrPath.TrimStart('/');
    }

    static string Enc(string value) => Uri.EscapeDataString(value);

    static string? Bool(bool? v) => v is null ? null : v.Value ? "true" : "false";

    static string AddQuery(string path, params (string Key, string? Value)[] pairs)
    {
        var sb = new StringBuilder();
        foreach (var (key, value) in pairs)
        {
            if (string.IsNullOrEmpty(value))
            {
                continue;
            }

            sb.Append(sb.Length == 0 ? '?' : '&');
            sb.Append(Uri.EscapeDataString(key));
            sb.Append('=');
            sb.Append(Uri.EscapeDataString(value));
        }

        return path + sb;
    }

    static string? Header(TransportResponse resp, string name) =>
        resp.Headers.TryGetValue(name, out var v) ? v : null;

    static void EnsureSuccess(TransportResponse resp, int expected)
    {
        if (resp.StatusCode != expected)
        {
            throw ApiException.FromResponse(resp.StatusCode, resp.Body, resp.Headers);
        }
    }

    public void Dispose()
    {
        if (_ownsTransport)
        {
            _ownedTransport?.Dispose();
        }
    }
}
