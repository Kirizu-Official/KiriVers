using System.Globalization;
using System.Text;

namespace Kirizu.KiriVers.Client;

/// <summary>
/// Aligns client fileset paths with the server: backslash to slash, Unicode NFC,
/// reject <c>..</c>, leading slashes, drive letters, and ASCII control characters.
/// </summary>
public static class PathUtil
{
    public static string Normalize(string relativePath)
    {
        var trimmed = (relativePath ?? "").Trim();
        if (trimmed.Length == 0)
        {
            throw new PathException("path must be non-empty");
        }

        foreach (var c in trimmed)
        {
            if (c < 0x20)
            {
                throw new PathException("path contains a control character");
            }
        }

        if (trimmed.StartsWith('/') || trimmed.StartsWith('\\'))
        {
            throw new PathException("path cannot start with a leading slash");
        }

        if (HasDrivePrefix(trimmed))
        {
            throw new PathException("path cannot contain a drive letter");
        }

        var unified = trimmed.Replace('\\', '/');
        foreach (var part in unified.Split('/'))
        {
            if (HasDrivePrefix(part))
            {
                throw new PathException("path segment cannot contain a drive letter");
            }

            if (part is "." or "..")
            {
                throw new PathException("path must not contain '.' or '..' segments");
            }
        }

        unified = unified.Normalize(NormalizationForm.FormC);
        while (unified.Contains("//", StringComparison.Ordinal))
        {
            unified = unified.Replace("//", "/", StringComparison.Ordinal);
        }

        unified = unified.Trim('/');
        if (unified.Length == 0)
        {
            throw new PathException("path must be non-empty");
        }

        foreach (var part in unified.Split('/'))
        {
            if (part is "." or ".." or "")
            {
                throw new PathException("path must not contain '.' or '..' segments");
            }
        }

        return unified;
    }

    static bool HasDrivePrefix(string value) =>
        value.Length >= 2 && char.IsAsciiLetter(value[0]) && value[1] == ':';

    public static string HexLower(ReadOnlySpan<byte> hash)
    {
        var sb = new StringBuilder(hash.Length * 2);
        foreach (var b in hash)
        {
            sb.Append(b.ToString("x2", CultureInfo.InvariantCulture));
        }

        return sb.ToString();
    }
}

/// <summary>Delta container magics. Unknown magic must not be cross-decoded.</summary>
public static class DeltaMagic
{
    public static readonly byte[] Kvdiffhp1 = "KVDIFFHP1\n"u8.ToArray();
    public static readonly byte[] Hdiff13 = "HDIFF13&"u8.ToArray();
    public static readonly byte[] Bsdiff40 = "BSDIFF40"u8.ToArray();
    public static readonly byte[] Vcdiff = [0xD6, 0xC3, 0xC4];

    public static string? Identify(ReadOnlySpan<byte> delta)
    {
        if (StartsWith(delta, Kvdiffhp1) || StartsWith(delta, Hdiff13))
        {
            return Capability.Hdiffpatch;
        }

        if (StartsWith(delta, Bsdiff40))
        {
            return Capability.Bsdiff;
        }

        if (StartsWith(delta, Vcdiff))
        {
            return Capability.Xdelta3;
        }

        return null;
    }

    public static bool IsKnown(ReadOnlySpan<byte> delta) => Identify(delta) is not null;

    static bool StartsWith(ReadOnlySpan<byte> data, ReadOnlySpan<byte> prefix) =>
        data.Length >= prefix.Length && data[..prefix.Length].SequenceEqual(prefix);
}

/// <summary>Check capability and delta algorithm names.</summary>
public static class Capability
{
    public const string FullPackage = "full_package";
    public const string PatchPackage = "patch_package";
    public const string BinaryDelta = "binary_delta";
    public const string FileList = "file_list";
    public const string Hdiffpatch = "hdiffpatch";
    public const string Bsdiff = "bsdiff";
    public const string Xdelta3 = "xdelta3";
}
