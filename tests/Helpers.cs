using System.Text;
using Kirizu.KiriVers.Client;

namespace Kirizu.KiriVers.Client.Tests;

sealed class RecordingTransport : ITransport
{
    public List<TransportRequest> Requests { get; } = [];

    public Func<TransportRequest, TransportResponse> Handler { get; set; } = _ =>
        new TransportResponse { StatusCode = 200, Body = "{}"u8.ToArray() };

    public Task<TransportResponse> SendAsync(TransportRequest request, CancellationToken cancellationToken = default)
    {
        Requests.Add(request);
        return Task.FromResult(Handler(request));
    }

    public TransportRequest Last => Requests[^1];

    public string LastJson => Last.Body is null ? "" : Encoding.UTF8.GetString(Last.Body);
}

static class TestClient
{
    public const string Base = "http://127.0.0.1:8080";
    public const string Project = "sdk-fixture";

    public static Client Create(RecordingTransport transport, IPatcher? patcher, IArchiveUnpacker? unpacker, IFileStore? store) =>
        new(new ClientOptions
        {
            BaseUrl = Base,
            ProjectRef = Project,
            Transport = transport,
            UseDefaultAdapters = false,
            Hasher = new BclHasher(),
            SignatureVerifier = new BclSignatureVerifier(),
            FileStore = store,
            ArchiveUnpacker = unpacker,
            Patcher = patcher,
            Replacer = null,
        });
}

sealed class FakePatcher : IPatcher
{
    public FakePatcher(params string[] algos) => SupportedAlgos = algos;

    public IReadOnlyList<string> SupportedAlgos { get; }

    public int ApplyCalls { get; private set; }

    public byte[] Apply(ReadOnlySpan<byte> oldFile, ReadOnlySpan<byte> delta, string algo)
    {
        ApplyCalls++;
        if (!DeltaMagic.IsKnown(delta))
        {
            throw new VerifyException("unknown magic");
        }

        return oldFile.ToArray();
    }
}
