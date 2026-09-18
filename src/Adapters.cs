namespace Kirizu.KiriVers.Client;

/// <summary>HTTP request seen by <see cref="ITransport"/>.</summary>
public sealed class TransportRequest
{
    public required string Method { get; init; }
    public required string Url { get; init; }
    public IReadOnlyDictionary<string, string>? Headers { get; init; }
    public byte[]? Body { get; init; }
}

/// <summary>HTTP response returned by <see cref="ITransport"/>.</summary>
public sealed class TransportResponse
{
    public required int StatusCode { get; init; }
    public byte[] Body { get; init; } = [];
    public IReadOnlyDictionary<string, string> Headers { get; init; } =
        new Dictionary<string, string>(StringComparer.OrdinalIgnoreCase);
}

/// <summary>Injectable HTTP. Must preserve query strings and honor Range.</summary>
public interface ITransport
{
    Task<TransportResponse> SendAsync(TransportRequest request, CancellationToken cancellationToken = default);
}

/// <summary>SHA-256 and MD5 used for package and integrity compare.</summary>
public interface IHasher
{
    string Sha256(ReadOnlySpan<byte> data);
    string Sha256(Stream data);
    string Md5(ReadOnlySpan<byte> data);
    string Md5(Stream data);
}

/// <summary>Local tree used for staging downloads and multi-file compare.</summary>
public interface IFileStore
{
    /// <summary>When true, check may declare <c>file_list</c>.</summary>
    bool CanWriteIndividualFiles { get; }

    bool Exists(string path);
    Stream OpenRead(string path);
    void Write(string path, byte[] data);
    IEnumerable<string> ListFiles(string directory);
    string NormalizeRelative(string relativePath);
}

/// <summary>Replace a staged file onto the running install path.</summary>
public interface IReplacer
{
    void Replace(string stagedPath, string installPath);
}

/// <summary>
/// Apply one binary delta. Implementations must reject unknown magics.
/// Official C# SDK ships no default Patcher.
/// </summary>
public interface IPatcher
{
    IReadOnlyList<string> SupportedAlgos { get; }

    byte[] Apply(ReadOnlySpan<byte> oldFile, ReadOnlySpan<byte> delta, string algo);
}

/// <summary>Verify check/integrity <c>signature</c> over BuildCheckPayload.</summary>
public interface ISignatureVerifier
{
    void Verify(string algo, string publicKeyPem, string payload, string signatureBase64);
}

/// <summary>Unpack a native zip whose members are content SHA-256 hex names.</summary>
public interface IArchiveUnpacker
{
    void Unpack(Stream zip, IReadOnlyList<IntegrityFile> files, string destinationDirectory);
}
