using System.Globalization;

namespace Kirizu.KiriVers.Client;

public sealed class UpdateRequest
{
    public required string CurrentVersion { get; init; }
    public required string Os { get; init; }
    public required string Arch { get; init; }
    public string Channel { get; init; } = "stable";
    public string? DeviceId { get; init; }
    public string? HwRev { get; init; }
    public string? OsVersion { get; init; }
    public string? LocalInstallPath { get; init; }
    public string? StageDirectory { get; init; }
    public string? ApplyPath { get; init; }
    public bool ReportDevice { get; init; }
    public bool FetchChangelog { get; init; }
    public PackPollOptions? PackPoll { get; init; }
}

public sealed class UpdateResult
{
    public CheckResult Check { get; init; } = new();
    public ChangelogResponse? Changelog { get; init; }
    public string? DiffMode { get; init; }
    public string? StagedPath { get; init; }
    public byte[]? StagedBytes { get; init; }
    public bool Applied { get; init; }
    public bool NoUpdate { get; init; }
    public string? Sha256 { get; init; }
}

/// <summary>
/// Check → download/diff/pack → verify → optional Patcher/unpack → optional Replacer → telemetry.
/// Missing Replacer is not a failure: verified bytes/path are returned.
/// </summary>
public sealed class Updater
{
    readonly Client _client;

    public Updater(Client client) => _client = client ?? throw new ArgumentNullException(nameof(client));

    public async Task<UpdateResult> RunAsync(UpdateRequest request, CancellationToken cancellationToken = default)
    {
        ArgumentNullException.ThrowIfNull(request);
        var hasher = _client.Hasher ?? new BclHasher();
        var stageDir = request.StageDirectory ?? Path.Combine(Path.GetTempPath(), "kirivers-stage-" + Guid.NewGuid().ToString("N"));
        Directory.CreateDirectory(stageDir);

        if (request.ReportDevice && !string.IsNullOrEmpty(request.DeviceId))
        {
            await _client.ReportDeviceAsync(new DeviceReportRequest
            {
                DeviceId = request.DeviceId,
                Version = request.CurrentVersion,
                Os = request.Os,
                Arch = request.Arch,
                Channel = request.Channel,
            }, cancellationToken).ConfigureAwait(false);
        }

        var check = await _client.CheckAsync(new CheckRequest
        {
            CurrentVersion = request.CurrentVersion,
            Os = request.Os,
            Arch = request.Arch,
            Channel = request.Channel,
            DeviceId = request.DeviceId,
            HwRev = request.HwRev,
            OsVersion = request.OsVersion,
        }, cancellationToken).ConfigureAwait(false);

        if (check.NoUpdate || check.NotModified || check.Body is null || !check.HasUpdate)
        {
            return new UpdateResult { Check = check, NoUpdate = true };
        }

        var body = check.Body;
        ChangelogResponse? changelog = null;
        if (request.FetchChangelog)
        {
            var ch = body.TargetChannel ?? request.Channel;
            changelog = await _client.GetChangelogAsync(ch, request.Os, request.Arch, new ChangelogQuery
            {
                FromVersion = request.CurrentVersion,
                ToVersion = body.VersionSemver,
            }, cancellationToken).ConfigureAwait(false);
        }

        try
        {
            var (bytes, mode, sha) = await ObtainVerifiedAsync(request, body, hasher, stageDir, cancellationToken).ConfigureAwait(false);
            if (!string.IsNullOrEmpty(body.Signature) &&
                _client.SignatureVerifier is not null &&
                !string.IsNullOrEmpty(_client.Options.SigningPublicKeyPem))
            {
                _client.VerifySignature(CheckPayload.FromCheck(body), body.Signature);
            }

            var stagedPath = Path.Combine(stageDir, string.IsNullOrEmpty(body.FileName) ? sha + ".bin" : body.FileName);
            await File.WriteAllBytesAsync(stagedPath, bytes, cancellationToken).ConfigureAwait(false);

            var applied = false;
            if (_client.Replacer is not null && !string.IsNullOrEmpty(request.ApplyPath))
            {
                _client.Replacer.Replace(stagedPath, request.ApplyPath);
                applied = true;
            }

            await TryTelemetryAsync(request, body, "installed", mode, null, null, cancellationToken).ConfigureAwait(false);
            return new UpdateResult
            {
                Check = check,
                Changelog = changelog,
                DiffMode = mode,
                StagedPath = stagedPath,
                StagedBytes = bytes,
                Applied = applied,
                Sha256 = sha,
            };
        }
        catch (Exception ex) when (ex is not OperationCanceledException)
        {
            await TryTelemetryAsync(request, body, "failed", Capability.FullPackage, "UPDATE_FAILED", ex.Message, cancellationToken).ConfigureAwait(false);
            throw;
        }
    }

    async Task<(byte[] Bytes, string Mode, string Sha256)> ObtainVerifiedAsync(
        UpdateRequest request,
        UpdateCheckResponse body,
        IHasher hasher,
        string stageDir,
        CancellationToken cancellationToken)
    {
        var targetSha = body.Sha256 ?? throw new VerifyException("check response missing sha256");
        var single = string.Equals(body.PackageType, "single_file", StringComparison.OrdinalIgnoreCase);

        if (single &&
            _client.Patcher is not null &&
            body.DeltaAvailable &&
            !body.IsDowngrade &&
            !string.IsNullOrEmpty(request.LocalInstallPath) &&
            File.Exists(request.LocalInstallPath))
        {
            try
            {
                return await TryDeltaAsync(request, body, hasher, targetSha, cancellationToken).ConfigureAwait(false);
            }
            catch (Exception ex) when (ex is not OperationCanceledException)
            {
                // Unknown magic, patch failure, or hash mismatch → full package.
            }
        }

        if (!single &&
            string.Equals(body.PackageType, "multi_file", StringComparison.OrdinalIgnoreCase) &&
            _client.FileStore is not null &&
            _client.Hasher is not null)
        {
            try
            {
                return await TryPackAsync(request, body, hasher, targetSha, stageDir, cancellationToken).ConfigureAwait(false);
            }
            catch (Exception ex) when (ex is not OperationCanceledException)
            {
                // Fall back to the check full package.
            }
        }

        return await DownloadFullAsync(body, hasher, targetSha, cancellationToken).ConfigureAwait(false);
    }

    async Task<(byte[] Bytes, string Mode, string Sha256)> TryDeltaAsync(
        UpdateRequest request,
        UpdateCheckResponse body,
        IHasher hasher,
        string targetSha,
        CancellationToken cancellationToken)
    {
        await using var local = File.OpenRead(request.LocalInstallPath!);
        var localSha = hasher.Sha256(local);
        var diff = await _client.DiffAsync(new DiffRequest
        {
            SourceVersion = request.CurrentVersion,
            TargetVersion = body.VersionSemver ?? body.VersionInteger?.ToString(CultureInfo.InvariantCulture) ?? request.CurrentVersion,
            Os = request.Os,
            Arch = request.Arch,
            Channel = body.TargetChannel ?? request.Channel,
            DeviceId = request.DeviceId,
            HwRev = request.HwRev,
            LocalSha256 = localSha,
        }, cancellationToken).ConfigureAwait(false);

        if (!string.Equals(diff.DiffMode, Capability.BinaryDelta, StringComparison.OrdinalIgnoreCase) ||
            string.IsNullOrEmpty(diff.PackageUrl))
        {
            return await DownloadFullAsync(body, hasher, targetSha, cancellationToken).ConfigureAwait(false);
        }

        var delta = await _client.DownloadAsync(diff.PackageUrl, cancellationToken: cancellationToken).ConfigureAwait(false);
        var magicAlgo = DeltaMagic.Identify(delta.Body);
        if (magicAlgo is null)
        {
            return await DownloadFullAsync(body, hasher, targetSha, cancellationToken).ConfigureAwait(false);
        }

        if (!string.IsNullOrEmpty(diff.DeltaAlgo) &&
            !string.Equals(diff.DeltaAlgo, magicAlgo, StringComparison.OrdinalIgnoreCase))
        {
            return await DownloadFullAsync(body, hasher, targetSha, cancellationToken).ConfigureAwait(false);
        }

        var oldBytes = await File.ReadAllBytesAsync(request.LocalInstallPath!, cancellationToken).ConfigureAwait(false);
        var patched = _client.Patcher!.Apply(oldBytes, delta.Body, diff.DeltaAlgo ?? magicAlgo);
        var got = hasher.Sha256(patched);
        if (!got.Equals(targetSha, StringComparison.OrdinalIgnoreCase))
        {
            throw new VerifyException("patched bytes sha256 mismatch");
        }

        return (patched, Capability.BinaryDelta, got);
    }

    async Task<(byte[] Bytes, string Mode, string Sha256)> TryPackAsync(
        UpdateRequest request,
        UpdateCheckResponse body,
        IHasher hasher,
        string targetSha,
        string stageDir,
        CancellationToken cancellationToken)
    {
        var version = body.VersionSemver ?? body.VersionInteger?.ToString(CultureInfo.InvariantCulture) ?? throw new VerifyException("missing target version");
        var integrity = await _client.GetIntegrityAsync(new IntegrityQuery
        {
            Version = version,
            Os = request.Os,
            Arch = request.Arch,
            Channel = body.TargetChannel ?? request.Channel,
            HwRev = request.HwRev,
            HashAlgo = "both",
        }, cancellationToken).ConfigureAwait(false);

        var needed = new List<string>();
        foreach (var file in integrity.Files)
        {
            var rel = PathUtil.Normalize(file.Path);
            var keep = string.Equals(file.InstallPolicy, "KEEP_IF_EXISTS", StringComparison.OrdinalIgnoreCase);
            if (keep && _client.FileStore!.Exists(rel))
            {
                if (!file.IntegrityCheck)
                {
                    continue;
                }

                if (!string.IsNullOrEmpty(file.Sha256))
                {
                    await using var stream = _client.FileStore.OpenRead(rel);
                    if (hasher.Sha256(stream).Equals(file.Sha256, StringComparison.OrdinalIgnoreCase))
                    {
                        continue;
                    }
                }
            }

            needed.Add(rel);
        }

        needed = needed.Distinct(StringComparer.Ordinal).ToList();

        var pack = await _client.PackUntilReadyAsync(new PackRequest
        {
            SourceVersion = request.CurrentVersion,
            TargetVersion = version,
            Os = request.Os,
            Arch = request.Arch,
            Channel = body.TargetChannel ?? request.Channel,
            DeviceId = request.DeviceId,
            HwRev = request.HwRev,
            NeededPaths = needed,
        }, request.PackPoll, cancellationToken).ConfigureAwait(false);

        if (string.Equals(pack.Status, "full_package", StringComparison.OrdinalIgnoreCase) ||
            string.IsNullOrEmpty(pack.PackageUrl))
        {
            return await DownloadFullAsync(body, hasher, targetSha, cancellationToken).ConfigureAwait(false);
        }

        var zip = await _client.DownloadAsync(pack.PackageUrl, cancellationToken: cancellationToken).ConfigureAwait(false);
        if (!string.IsNullOrEmpty(pack.Sha256))
        {
            var got = hasher.Sha256(zip.Body);
            if (!got.Equals(pack.Sha256, StringComparison.OrdinalIgnoreCase))
            {
                throw new VerifyException("pack sha256 mismatch");
            }
        }

        if (_client.ArchiveUnpacker is not null && pack.Files is { Count: > 0 })
        {
            using var ms = new MemoryStream(zip.Body, writable: false);
            _client.ArchiveUnpacker.Unpack(ms, pack.Files, stageDir);
        }

        return (zip.Body, pack.DiffMode ?? Capability.PatchPackage, pack.Sha256 ?? hasher.Sha256(zip.Body));
    }

    async Task<(byte[] Bytes, string Mode, string Sha256)> DownloadFullAsync(
        UpdateCheckResponse body,
        IHasher hasher,
        string targetSha,
        CancellationToken cancellationToken)
    {
        if (string.IsNullOrEmpty(body.PackageUrl))
        {
            throw new VerifyException("check response missing package_url");
        }

        var downloaded = await _client.DownloadAsync(body.PackageUrl, cancellationToken: cancellationToken).ConfigureAwait(false);
        var got = hasher.Sha256(downloaded.Body);
        if (!got.Equals(targetSha, StringComparison.OrdinalIgnoreCase))
        {
            throw new VerifyException($"sha256 mismatch: got {got}, want {targetSha}");
        }

        return (downloaded.Body, Capability.FullPackage, got);
    }

    async Task TryTelemetryAsync(
        UpdateRequest request,
        UpdateCheckResponse body,
        string status,
        string? diffMode,
        string? errorCode,
        string? errorMessage,
        CancellationToken cancellationToken)
    {
        try
        {
            await _client.ReportTelemetryAsync(new TelemetryRequest
            {
                Os = request.Os,
                Arch = request.Arch,
                Channel = body.TargetChannel ?? request.Channel,
                FromVersion = request.CurrentVersion,
                ToVersion = body.VersionSemver ?? body.VersionInteger?.ToString(CultureInfo.InvariantCulture) ?? "",
                Status = status,
                DeviceId = request.DeviceId,
                DiffMode = diffMode,
                ErrorCode = errorCode,
                ErrorMessage = errorMessage,
            }, cancellationToken).ConfigureAwait(false);
        }
        catch (Exception) when (status is "installed" or "failed")
        {
            // Telemetry must never block apply.
        }
    }
}
