using System.IO.Compression;
using System.Net.Http.Headers;
using System.Security.Cryptography;

namespace Kirizu.KiriVers.Client;

/// <summary>Default <see cref="ITransport"/> using <see cref="HttpClient"/>.</summary>
public sealed class HttpClientTransport : ITransport, IDisposable
{
    readonly HttpClient _http;
    readonly bool _owns;

    public HttpClientTransport(HttpClient? httpClient = null)
    {
        if (httpClient is null)
        {
            _http = new HttpClient();
            _owns = true;
        }
        else
        {
            _http = httpClient;
            _owns = false;
        }
    }

    public async Task<TransportResponse> SendAsync(TransportRequest request, CancellationToken cancellationToken = default)
    {
        using var msg = new HttpRequestMessage(new HttpMethod(request.Method), request.Url);
        if (request.Body is { Length: > 0 } || string.Equals(request.Method, "POST", StringComparison.OrdinalIgnoreCase))
        {
            var content = new ByteArrayContent(request.Body ?? []);
            if (request.Headers is not null && TryPop(request.Headers, "Content-Type", out var ct))
            {
                content.Headers.ContentType = MediaTypeHeaderValue.Parse(ct);
            }
            else if (request.Body is { Length: > 0 })
            {
                content.Headers.ContentType = new MediaTypeHeaderValue("application/json");
            }

            msg.Content = content;
        }

        if (request.Headers is not null)
        {
            foreach (var (key, value) in request.Headers)
            {
                if (key.Equals("Content-Type", StringComparison.OrdinalIgnoreCase))
                {
                    continue;
                }

                if (!msg.Headers.TryAddWithoutValidation(key, value) && msg.Content is not null)
                {
                    msg.Content.Headers.TryAddWithoutValidation(key, value);
                }
            }
        }

        using var resp = await _http.SendAsync(msg, HttpCompletionOption.ResponseHeadersRead, cancellationToken).ConfigureAwait(false);
        var bytes = await resp.Content.ReadAsByteArrayAsync(cancellationToken).ConfigureAwait(false);
        var headers = new Dictionary<string, string>(StringComparer.OrdinalIgnoreCase);
        foreach (var h in resp.Headers)
        {
            headers[h.Key] = string.Join(",", h.Value);
        }

        foreach (var h in resp.Content.Headers)
        {
            headers[h.Key] = string.Join(",", h.Value);
        }

        return new TransportResponse
        {
            StatusCode = (int)resp.StatusCode,
            Body = bytes,
            Headers = headers,
        };
    }

    static bool TryPop(IReadOnlyDictionary<string, string> headers, string name, out string value)
    {
        foreach (var (k, v) in headers)
        {
            if (k.Equals(name, StringComparison.OrdinalIgnoreCase))
            {
                value = v;
                return true;
            }
        }

        value = "";
        return false;
    }

    public void Dispose()
    {
        if (_owns)
        {
            _http.Dispose();
        }
    }
}

/// <summary>SHA-256 / MD5 via <c>System.Security.Cryptography</c>.</summary>
public sealed class BclHasher : IHasher
{
    public string Sha256(ReadOnlySpan<byte> data) => PathUtil.HexLower(SHA256.HashData(data));

    public string Sha256(Stream data)
    {
        using var sha = SHA256.Create();
        return PathUtil.HexLower(sha.ComputeHash(data));
    }

    public string Md5(ReadOnlySpan<byte> data) => PathUtil.HexLower(MD5.HashData(data));

    public string Md5(Stream data)
    {
        using var md5 = MD5.Create();
        return PathUtil.HexLower(md5.ComputeHash(data));
    }
}

/// <summary>File tree rooted at <see cref="Root"/> using <c>System.IO</c>.</summary>
public sealed class LocalFileStore : IFileStore
{
    public LocalFileStore(string root)
    {
        Root = Path.GetFullPath(root);
        Directory.CreateDirectory(Root);
    }

    public string Root { get; }
    public bool CanWriteIndividualFiles => true;

    public bool Exists(string path) => File.Exists(Map(path));

    public Stream OpenRead(string path) => File.OpenRead(Map(path));

    public void Write(string path, byte[] data)
    {
        var full = Map(path);
        var dir = Path.GetDirectoryName(full);
        if (!string.IsNullOrEmpty(dir))
        {
            Directory.CreateDirectory(dir);
        }

        File.WriteAllBytes(full, data);
    }

    public IEnumerable<string> ListFiles(string directory)
    {
        var full = string.IsNullOrEmpty(directory) ? Root : Map(directory);
        if (!Directory.Exists(full))
        {
            yield break;
        }

        foreach (var file in Directory.EnumerateFiles(full, "*", SearchOption.AllDirectories))
        {
            yield return Path.GetRelativePath(Root, file).Replace('\\', '/');
        }
    }

    public string NormalizeRelative(string relativePath) => PathUtil.Normalize(relativePath);

    string Map(string relative)
    {
        var norm = PathUtil.Normalize(relative);
        var combined = Path.GetFullPath(Path.Combine(Root, norm.Replace('/', Path.DirectorySeparatorChar)));
        var root = Root.TrimEnd(Path.DirectorySeparatorChar, Path.AltDirectorySeparatorChar)
                   + Path.DirectorySeparatorChar;
        if (!combined.StartsWith(root, StringComparison.OrdinalIgnoreCase) &&
            !string.Equals(combined, Root, StringComparison.OrdinalIgnoreCase))
        {
            throw new PathException("path escapes file-store root");
        }

        return combined;
    }
}

/// <summary>Native zip whose members are SHA-256 hex; output names come from integrity <c>path</c>.</summary>
public sealed class ZipArchiveUnpacker : IArchiveUnpacker
{
    public void Unpack(Stream zip, IReadOnlyList<IntegrityFile> files, string destinationDirectory)
    {
        Directory.CreateDirectory(destinationDirectory);
        using var archive = new ZipArchive(zip, ZipArchiveMode.Read, leaveOpen: true);
        var byName = archive.Entries.ToDictionary(e => e.FullName.Replace('\\', '/'), e => e, StringComparer.OrdinalIgnoreCase);
        foreach (var file in files)
        {
            if (string.IsNullOrEmpty(file.Sha256))
            {
                continue;
            }

            var key = file.Sha256.ToLowerInvariant();
            if (!byName.TryGetValue(key, out var entry) && !byName.TryGetValue(key + ".bin", out entry))
            {
                var match = archive.Entries.FirstOrDefault(e =>
                    Path.GetFileName(e.FullName).StartsWith(key, StringComparison.OrdinalIgnoreCase));
                entry = match ?? throw new VerifyException($"zip is missing member {key} for {file.Path}");
            }

            var destRel = PathUtil.Normalize(file.Path);
            var destRoot = Path.GetFullPath(destinationDirectory);
            var destRootPrefix = destRoot.TrimEnd(Path.DirectorySeparatorChar, Path.AltDirectorySeparatorChar)
                                 + Path.DirectorySeparatorChar;
            var dest = Path.GetFullPath(Path.Combine(destRoot, destRel.Replace('/', Path.DirectorySeparatorChar)));
            if (!dest.StartsWith(destRootPrefix, StringComparison.OrdinalIgnoreCase) &&
                !string.Equals(dest, destRoot, StringComparison.OrdinalIgnoreCase))
            {
                throw new PathException("unpack path escapes destination directory");
            }

            var destDir = Path.GetDirectoryName(dest);
            if (!string.IsNullOrEmpty(destDir))
            {
                Directory.CreateDirectory(destDir);
            }

            entry.ExtractToFile(dest, overwrite: true);
        }
    }
}
