using System.Text.Json;
using System.Text.Json.Serialization;

namespace Kirizu.KiriVers.Client;

public sealed class CheckRequest
{
    public required string CurrentVersion { get; init; }
    public required string Os { get; init; }
    public required string Arch { get; init; }
    public string? Channel { get; init; }
    public string? HwRev { get; init; }
    public string? OsVersion { get; init; }
    public string? DeviceId { get; init; }
    public IReadOnlyList<string>? Capabilities { get; init; }
    public IReadOnlyList<string>? AcceptedDeltaAlgos { get; init; }
    [JsonIgnore]
    public string? IfNoneMatch { get; init; }
}

public sealed class CheckResult
{
    public int StatusCode { get; init; }
    public UpdateCheckResponse? Body { get; init; }
    public string? ETag { get; init; }
    public bool HasUpdate => StatusCode == 200 && Body is { HasUpdate: true };
    public bool NoUpdate => StatusCode is 204;
    public bool NotModified => StatusCode is 304;
}

public sealed class UpdateCheckResponse
{
    public bool HasUpdate { get; set; }
    public bool IsMandatory { get; set; }
    public bool IsDowngrade { get; set; }
    public string? Reason { get; set; }
    public string? CompareEngine { get; set; }
    public long? VersionInteger { get; set; }
    public string? VersionSemver { get; set; }
    public string? TargetChannel { get; set; }
    public string? TargetHwRev { get; set; }
    public string? PackageType { get; set; }
    public string? RootHash { get; set; }
    public string? PackageUrl { get; set; }
    public string? FileName { get; set; }
    public long? Size { get; set; }
    public string? Sha256 { get; set; }
    public bool DeltaAvailable { get; set; }
    public string? DeltaAlgo { get; set; }
    public string? Signature { get; set; }
    public string? ArtifactSignature { get; set; }
    public string? PlatformNotes { get; set; }
    public DateTimeOffset? PublishTime { get; set; }
}

public sealed class DeviceReportRequest
{
    public required string DeviceId { get; init; }
    public string? Version { get; init; }
    public string? Os { get; init; }
    public string? Arch { get; init; }
    public string? Channel { get; init; }
    public JsonElement? Custom { get; init; }
}

public sealed class DeviceReportResponse
{
    public string? Ip { get; set; }
    public string? CountryCode { get; set; }
    public string? RegionCode { get; set; }
    public JsonElement GeoI18n { get; set; }
}

public sealed class ChangelogQuery
{
    public string? FromVersion { get; init; }
    public string? ToVersion { get; init; }
    public string? ChangelogScope { get; init; }
    public string? ChangelogLayout { get; init; }
    public bool? ChangelogIncludeRevoked { get; init; }
    public bool? ChangelogIncludePlatformNotes { get; init; }
    public string? ChangelogLocale { get; init; }
    public string? Locale { get; init; }
    public string? IfNoneMatch { get; init; }
}

public sealed class ChangelogResponse
{
    public string? Changelog { get; set; }
    public List<ChangelogVersion>? ChangelogVersions { get; set; }
    public string? ETag { get; set; }
    public bool NotModified { get; set; }
}

public sealed class ChangelogVersion
{
    public string? Channel { get; set; }
    public string? Status { get; set; }
    public string? Changelog { get; set; }
    public bool HadArtifactForRequestPlatform { get; set; }
    public string? Title { get; set; }
    public string? PlatformNotes { get; set; }
    public long? VersionInteger { get; set; }
    public string? VersionSemver { get; set; }
}

public sealed class IntegrityQuery
{
    public required string Version { get; init; }
    public required string Os { get; init; }
    public required string Arch { get; init; }
    public string? HashAlgo { get; init; }
    public bool? Compact { get; init; }
    public bool? IncludeFileUrls { get; init; }
    public string? HwRev { get; init; }
    public string? Channel { get; init; }
    public string? IfNoneMatch { get; init; }
}

public sealed class IntegrityManifest
{
    public long? VersionInteger { get; set; }
    public string? VersionSemver { get; set; }
    public string? Channel { get; set; }
    public string? PackageType { get; set; }
    public string? RootHash { get; set; }
    public string? FullPackageUrl { get; set; }
    public string? FileName { get; set; }
    public long? Size { get; set; }
    public string? Sha256 { get; set; }
    public string? Signature { get; set; }
    public List<IntegrityFile> Files { get; set; } = [];
    public List<IntegrityVolume>? Volumes { get; set; }
    public string? ETag { get; set; }
    public bool NotModified { get; set; }
}

public sealed class IntegrityFile
{
    public string Path { get; set; } = "";
    public long Size { get; set; }
    public string? Sha256 { get; set; }
    public string? Md5 { get; set; }
    public string? InstallPolicy { get; set; }
    public bool IntegrityCheck { get; set; }
    public string? Url { get; set; }
}

public sealed class IntegrityVolume
{
    public string? Sha256 { get; set; }
    public long Size { get; set; }
    public string? Url { get; set; }
}

public sealed class DiffRequest
{
    public required string SourceVersion { get; init; }
    public required string TargetVersion { get; init; }
    public required string Os { get; init; }
    public required string Arch { get; init; }
    public string? Channel { get; init; }
    public string? DeviceId { get; init; }
    public string? HwRev { get; init; }
    public string? LocalSha256 { get; init; }
    public bool? PreferFull { get; init; }
    public IReadOnlyList<string>? Capabilities { get; init; }
    public IReadOnlyList<string>? AcceptedDeltaAlgos { get; init; }
}

public sealed class DiffResponse
{
    public string? DiffMode { get; set; }
    public string? RootHash { get; set; }
    public long? VersionInteger { get; set; }
    public string? VersionSemver { get; set; }
    public string? Channel { get; set; }
    public string? CompareEngine { get; set; }
    public string? PackageUrl { get; set; }
    public string? FileName { get; set; }
    public long? Size { get; set; }
    public string? Sha256 { get; set; }
    public string? Signature { get; set; }
    public string? DeltaAlgo { get; set; }
    public List<string>? DeletedPaths { get; set; }
    public List<string>? InvalidPaths { get; set; }
    public List<IntegrityFile>? Files { get; set; }
}

public sealed class PackRequest
{
    public required string SourceVersion { get; init; }
    public required string TargetVersion { get; init; }
    public required string Os { get; init; }
    public required string Arch { get; init; }
    public string? Channel { get; init; }
    public string? DeviceId { get; init; }
    public string? HwRev { get; init; }
    public IReadOnlyList<string>? NeededPaths { get; init; }
}

public sealed class PackPollOptions
{
    public TimeSpan InitialDelay { get; init; } = TimeSpan.FromSeconds(1);
    public TimeSpan MaxDelay { get; init; } = TimeSpan.FromSeconds(15);
    public TimeSpan Deadline { get; init; } = TimeSpan.FromMinutes(5);
}

public sealed class PackResponse
{
    public string? Status { get; set; }
    public string? DiffMode { get; set; }
    public string? Compression { get; set; }
    public string? PackageUrl { get; set; }
    public string? FileName { get; set; }
    public long? Size { get; set; }
    public string? Sha256 { get; set; }
    public string? Signature { get; set; }
    public string? RootHash { get; set; }
    public long? VersionInteger { get; set; }
    public string? VersionSemver { get; set; }
    public string? Channel { get; set; }
    public string? CompareEngine { get; set; }
    public List<IntegrityFile>? Files { get; set; }
    public List<string>? DeletedPaths { get; set; }
    public List<string>? InvalidPaths { get; set; }
}

public sealed class ProjectPublic
{
    public string? Uuid { get; set; }
    public string? Slug { get; set; }
    public string? CompareEngine { get; set; }
    public string? DefaultLocale { get; set; }
    public string? DeviceIdPolicy { get; set; }
    public bool ForceHttps { get; set; }
    public string? MinimumSupportedVersion { get; set; }
    public bool RequireClientToken { get; set; }
    public string? StorageVisibility { get; set; }
    public DateTimeOffset? CreatedAt { get; set; }
    public DateTimeOffset? UpdatedAt { get; set; }
}

public sealed class ChannelList
{
    public List<ChannelInfo> Channels { get; set; } = [];
}

public sealed class ChannelInfo
{
    public string? Name { get; set; }
    public string? Slug { get; set; }
    public int StabilityRank { get; set; }
}

public sealed class MatrixList
{
    public List<MatrixRow> Matrix { get; set; } = [];
}

public sealed class MatrixRow
{
    public string? Os { get; set; }
    public string? Arch { get; set; }
    public string? PackageType { get; set; }
}

public sealed class LanguageList
{
    public List<LanguageInfo> Languages { get; set; } = [];
}

public sealed class LanguageInfo
{
    public string? Code { get; set; }
    public string? DisplayName { get; set; }
    public bool IsDefault { get; set; }
    public int SortOrder { get; set; }
}

public sealed class AnnouncementQuery
{
    public string? Version { get; init; }
    public string? Os { get; init; }
    public string? Arch { get; init; }
    public string? Locale { get; init; }
    public string? AcceptLanguage { get; init; }
    public string? IfNoneMatch { get; init; }
}

public sealed class AnnouncementList
{
    public List<Announcement> Announcements { get; set; } = [];
    public string? ETag { get; set; }
    public bool NotModified { get; set; }
}

public sealed class Announcement
{
    public string? Id { get; set; }
    public string? Title { get; set; }
    public string? Subtitle { get; set; }
    public string? Markdown { get; set; }
    public string? Locale { get; set; }
    public DateTimeOffset? StartsAt { get; set; }
    public DateTimeOffset? EndsAt { get; set; }
}

public sealed class TelemetryRequest
{
    public required string Os { get; init; }
    public required string Arch { get; init; }
    public required string Channel { get; init; }
    public required string FromVersion { get; init; }
    public required string ToVersion { get; init; }
    public required string Status { get; init; }
    public string? DeviceId { get; init; }
    public string? DiffMode { get; init; }
    public string? ErrorCode { get; init; }
    public string? ErrorMessage { get; init; }
}

public sealed class HealthStatus
{
    public string? Status { get; set; }
    public bool Ready { get; set; }
}

public sealed class BinaryResponse
{
    public int StatusCode { get; init; }
    public byte[] Body { get; init; } = [];
    public IReadOnlyDictionary<string, string> Headers { get; init; } =
        new Dictionary<string, string>(StringComparer.OrdinalIgnoreCase);
}

internal sealed class CheckBody
{
    public required string CurrentVersion { get; init; }
    public required string Os { get; init; }
    public required string Arch { get; init; }
    public string? Channel { get; init; }
    public string? HwRev { get; init; }
    public string? OsVersion { get; init; }
    public string? DeviceId { get; init; }
    public IReadOnlyList<string>? Capabilities { get; init; }
    public IReadOnlyList<string>? AcceptedDeltaAlgos { get; init; }
}
