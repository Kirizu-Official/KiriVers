using System.Formats.Asn1;
using System.Globalization;
using System.Security.Cryptography;
using System.Text;

namespace Kirizu.KiriVers.Client;

/// <summary>
/// Payload for check/integrity signatures: integer, semver, root_hash, package_url, size, sha256 (newline joined).
/// </summary>
public static class CheckPayload
{
    public static string Build(
        string? versionInteger,
        string? versionSemver,
        string? rootHash,
        string? packageUrl,
        string? size,
        string? sha256Hex) =>
        string.Join('\n', new[]
        {
            versionInteger ?? "",
            versionSemver ?? "",
            rootHash ?? "",
            packageUrl ?? "",
            size ?? "",
            sha256Hex ?? "",
        });

    public static string FromCheck(UpdateCheckResponse body) =>
        Build(
            body.VersionInteger?.ToString(CultureInfo.InvariantCulture),
            body.VersionSemver,
            body.RootHash,
            body.PackageUrl,
            body.Size?.ToString(CultureInfo.InvariantCulture),
            body.Sha256);

    public static string FromIntegrity(IntegrityManifest body) =>
        Build(
            body.VersionInteger?.ToString(CultureInfo.InvariantCulture),
            body.VersionSemver,
            body.RootHash,
            body.FullPackageUrl,
            body.Size?.ToString(CultureInfo.InvariantCulture),
            body.Sha256);
}

/// <summary>BCL RSA-SHA256 plus RFC 8032 Ed25519 (no extra packages on net8.0).</summary>
public sealed class BclSignatureVerifier : ISignatureVerifier
{
    public const string Ed25519 = "ed25519";
    public const string RsaSha256 = "rsa-sha256";

    public void Verify(string algo, string publicKeyPem, string payload, string signatureBase64)
    {
        ArgumentException.ThrowIfNullOrEmpty(algo);
        ArgumentException.ThrowIfNullOrEmpty(publicKeyPem);
        ArgumentNullException.ThrowIfNull(payload);
        ArgumentException.ThrowIfNullOrEmpty(signatureBase64);

        byte[] sig;
        try
        {
            sig = Convert.FromBase64String(signatureBase64);
        }
        catch (FormatException ex)
        {
            throw new VerifyException("signature is not standard base64: " + ex.Message);
        }

        var payloadBytes = Encoding.UTF8.GetBytes(payload);
        switch (algo.Trim().ToLowerInvariant())
        {
            case Ed25519:
                VerifyEd25519(publicKeyPem, payloadBytes, sig);
                return;
            case RsaSha256:
                VerifyRsa(publicKeyPem, payloadBytes, sig);
                return;
            default:
                throw new VerifyException("unsupported signature algorithm: " + algo);
        }
    }

    static void VerifyRsa(string publicKeyPem, byte[] payload, byte[] signature)
    {
        using var rsa = RSA.Create();
        rsa.ImportFromPem(publicKeyPem);
        if (!rsa.VerifyData(payload, signature, HashAlgorithmName.SHA256, RSASignaturePadding.Pkcs1))
        {
            throw new VerifyException("RSA-SHA256 signature mismatch");
        }
    }

    static void VerifyEd25519(string publicKeyPem, byte[] payload, byte[] signature)
    {
        var key = ParseEd25519PublicKey(publicKeyPem);
        if (!Ed25519Core.Verify(key, payload, signature))
        {
            throw new VerifyException("Ed25519 signature mismatch");
        }
    }

    internal static byte[] ParseEd25519PublicKey(string pemOrRaw)
    {
        var trimmed = pemOrRaw.Trim();
        if (trimmed.Contains("BEGIN", StringComparison.Ordinal))
        {
            var der = DecodePem(trimmed, "PUBLIC KEY");
            return ParseEd25519Spki(der);
        }

        if (trimmed.Length == 64 && trimmed.All(Uri.IsHexDigit))
        {
            return Convert.FromHexString(trimmed);
        }

        throw new VerifyException("Ed25519 public key must be SPKI PEM or 32-byte hex");
    }

    static byte[] DecodePem(string pem, string label)
    {
        var begin = "-----BEGIN " + label + "-----";
        var end = "-----END " + label + "-----";
        var i = pem.IndexOf(begin, StringComparison.Ordinal);
        var j = pem.IndexOf(end, StringComparison.Ordinal);
        if (i < 0 || j < 0)
        {
            throw new VerifyException("PEM is missing " + label);
        }

        var b64 = pem[(i + begin.Length)..j].Replace("\r", "").Replace("\n", "").Trim();
        return Convert.FromBase64String(b64);
    }

    static byte[] ParseEd25519Spki(byte[] der)
    {
        var reader = new AsnReader(der, AsnEncodingRules.DER);
        var seq = reader.ReadSequence();
        var alg = seq.ReadSequence();
        var oid = alg.ReadObjectIdentifier();
        if (oid != "1.3.101.112")
        {
            throw new VerifyException("SPKI OID is not id-Ed25519");
        }

        var bits = seq.ReadBitString(out var unused);
        if (unused != 0 || bits.Length != 32)
        {
            throw new VerifyException("Ed25519 SPKI must contain a 32-byte key");
        }

        return bits;
    }
}
