using System.IO.Compression;
using System.Security.Cryptography;
using System.Text;
using Kirizu.KiriVers.Client;
using Xunit;

namespace Kirizu.KiriVers.Client.Tests;

public class AdapterTests
{
    [Fact]
    public async Task CapabilitiesFollowInjectedAdapters()
    {
        var t = new RecordingTransport
        {
            Handler = _ => new TransportResponse
            {
                StatusCode = 200,
                Body = """{"has_update":false,"is_mandatory":false,"is_downgrade":false,"reason":"normal","compare_engine":"semver","version_integer":null,"version_semver":"1.0.0","target_channel":"stable","target_hw_rev":null,"package_type":"single_file","root_hash":"","package_url":"","file_name":"","size":0,"sha256":"","delta_available":false}"""u8.ToArray(),
            },
        };

        var bare = TestClient.Create(t, patcher: null, unpacker: null, store: null);
        Assert.Equal(new[] { Capability.FullPackage }, bare.Capabilities);
        await bare.CheckAsync(new CheckRequest { CurrentVersion = "1.0.0", Os = "windows", Arch = "x86_64" });
        using (var json = System.Text.Json.JsonDocument.Parse(t.LastJson))
        {
            var caps = json.RootElement.GetProperty("capabilities").EnumerateArray().Select(x => x.GetString()).ToArray();
            Assert.Equal(new[] { Capability.FullPackage }, caps);
            Assert.False(json.RootElement.TryGetProperty("accepted_delta_algos", out _));
        }

        var withPatch = TestClient.Create(t, new FakePatcher(Capability.Bsdiff, Capability.Xdelta3), new ZipArchiveUnpacker(), new LocalFileStore(Path.Combine(Path.GetTempPath(), "kv-fs-" + Guid.NewGuid().ToString("N"))));
        Assert.Contains(Capability.BinaryDelta, withPatch.Capabilities);
        Assert.Contains(Capability.PatchPackage, withPatch.Capabilities);
        Assert.Contains(Capability.FileList, withPatch.Capabilities);
        Assert.Contains(Capability.Bsdiff, withPatch.AcceptedDeltaAlgos);
        await withPatch.CheckAsync(new CheckRequest { CurrentVersion = "1.0.0", Os = "windows", Arch = "x86_64" });
        using (var json = System.Text.Json.JsonDocument.Parse(t.LastJson))
        {
            var caps = json.RootElement.GetProperty("capabilities").EnumerateArray().Select(x => x.GetString()).ToArray();
            Assert.Contains(Capability.BinaryDelta, caps);
            var algos = json.RootElement.GetProperty("accepted_delta_algos").EnumerateArray().Select(x => x.GetString()).ToArray();
            Assert.Contains(Capability.Bsdiff, algos);
        }
    }

    [Fact]
    public async Task PackPollRepeatsIdenticalBody()
    {
        var bodies = new List<byte[]>();
        var calls = 0;
        var t = new RecordingTransport
        {
            Handler = req =>
            {
                calls++;
                bodies.Add(req.Body ?? []);
                if (calls == 1)
                {
                    return new TransportResponse { StatusCode = 202, Body = """{"status":"pending"}"""u8.ToArray() };
                }

                return new TransportResponse { StatusCode = 200, Body = """{"status":"ready","package_url":"/p","sha256":"ab"}"""u8.ToArray() };
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
        var result = await client.PackUntilReadyAsync(new PackRequest
        {
            SourceVersion = "1.0.0",
            TargetVersion = "1.1.0",
            Os = "windows",
            Arch = "x86_64",
            NeededPaths = ["b", "a"],
        }, new PackPollOptions
        {
            InitialDelay = TimeSpan.FromMilliseconds(1),
            MaxDelay = TimeSpan.FromMilliseconds(1),
            Deadline = TimeSpan.FromSeconds(5),
        });
        Assert.Equal("ready", result.Status);
        Assert.Equal(2, t.Requests.Count);
        Assert.Equal(t.Requests[0].Url, t.Requests[1].Url);
        Assert.Equal("POST", t.Requests[0].Method);
        Assert.True(bodies[0].AsSpan().SequenceEqual(bodies[1]));
    }

    [Fact]
    public void PathUtilNfcAndRejectsDotDot()
    {
        Assert.Equal("foo/bar", PathUtil.Normalize(@"foo\bar"));
        var nfd = "e\u0301.txt";
        var nfc = "é.txt";
        Assert.Equal(PathUtil.Normalize(nfc), PathUtil.Normalize(nfd));
        Assert.Throws<PathException>(() => PathUtil.Normalize("../etc/passwd"));
        Assert.Throws<PathException>(() => PathUtil.Normalize("foo/../bar"));
        Assert.Throws<PathException>(() => PathUtil.Normalize("/etc/passwd"));
        Assert.Throws<PathException>(() => PathUtil.Normalize(@"\windows\system32"));
        Assert.Throws<PathException>(() => PathUtil.Normalize(@"C:\Windows\System32"));
        Assert.Throws<PathException>(() => PathUtil.Normalize("foo/C:/bar"));
        Assert.Throws<PathException>(() => PathUtil.Normalize("a\nb"));
    }

    [Fact]
    public void DeltaMagicSplitsKnownContainers()
    {
        Assert.Equal(Capability.Hdiffpatch, DeltaMagic.Identify("KVDIFFHP1\nxxxx"u8.ToArray()));
        Assert.Equal(Capability.Hdiffpatch, DeltaMagic.Identify("HDIFF13&xxxx"u8.ToArray()));
        Assert.Equal(Capability.Bsdiff, DeltaMagic.Identify("BSDIFF40xxxx"u8.ToArray()));
        Assert.Equal(Capability.Xdelta3, DeltaMagic.Identify([0xD6, 0xC3, 0xC4, 0x00]));
        Assert.Null(DeltaMagic.Identify("NOTAMAGIC"u8.ToArray()));
    }

    [Fact]
    public async Task UpdaterFallsBackWhenMagicUnknown()
    {
        var full = "full-package-bytes"u8.ToArray();
        var sha = new BclHasher().Sha256(full);
        var t = new RecordingTransport
        {
            Handler = req =>
            {
                if (req.Url.Contains("/update/check"))
                {
                    return new TransportResponse
                    {
                        StatusCode = 200,
                        Body = Encoding.UTF8.GetBytes(
                            $$"""{"has_update":true,"is_mandatory":false,"is_downgrade":false,"reason":"normal","compare_engine":"semver","version_integer":null,"version_semver":"1.1.0","target_channel":"stable","target_hw_rev":null,"package_type":"single_file","root_hash":"","package_url":"http://127.0.0.1:8080/full.bin","file_name":"app.bin","size":{{full.Length}},"sha256":"{{sha}}","delta_available":true}"""),
                    };
                }

                if (req.Url.Contains("/update/diff"))
                {
                    return new TransportResponse
                    {
                        StatusCode = 200,
                        Body = """{"diff_mode":"binary_delta","root_hash":"","version_integer":null,"version_semver":"1.1.0","channel":"stable","compare_engine":"semver","package_url":"http://127.0.0.1:8080/delta.bin","delta_algo":"bsdiff"}"""u8.ToArray(),
                    };
                }

                if (req.Url.Contains("delta.bin"))
                {
                    return new TransportResponse { StatusCode = 200, Body = "????not-a-delta????"u8.ToArray() };
                }

                if (req.Url.Contains("full.bin"))
                {
                    return new TransportResponse { StatusCode = 200, Body = full };
                }

                if (req.Url.Contains("/telemetry/report"))
                {
                    return new TransportResponse { StatusCode = 202, Body = """{"status":"ok"}"""u8.ToArray() };
                }

                return new TransportResponse { StatusCode = 200, Body = "{}"u8.ToArray() };
            },
        };

        var install = Path.Combine(Path.GetTempPath(), "kv-old-" + Guid.NewGuid().ToString("N") + ".bin");
        await File.WriteAllBytesAsync(install, "old"u8.ToArray());
        var client = new Client(new ClientOptions
        {
            BaseUrl = TestClient.Base,
            ProjectRef = TestClient.Project,
            Transport = t,
            UseDefaultAdapters = false,
            Hasher = new BclHasher(),
            Patcher = new FakePatcher(Capability.Bsdiff),
        });
        var updater = new Updater(client);
        var result = await updater.RunAsync(new UpdateRequest
        {
            CurrentVersion = "1.0.0",
            Os = "windows",
            Arch = "x86_64",
            LocalInstallPath = install,
        });
        Assert.Equal(Capability.FullPackage, result.DiffMode);
        Assert.Equal(sha, result.Sha256);
        Assert.False(result.Applied);
        File.Delete(install);
    }

    [Fact]
    public void ZipUnpackerMapsHashMembersToPaths()
    {
        var payload = "hello-zip"u8.ToArray();
        var name = new BclHasher().Sha256(payload);
        using var zipStream = new MemoryStream();
        using (var zip = new ZipArchive(zipStream, ZipArchiveMode.Create, leaveOpen: true))
        {
            var entry = zip.CreateEntry(name);
            using var s = entry.Open();
            s.Write(payload);
        }

        zipStream.Position = 0;
        var dest = Path.Combine(Path.GetTempPath(), "kv-unzip-" + Guid.NewGuid().ToString("N"));
        new ZipArchiveUnpacker().Unpack(zipStream, [new IntegrityFile { Path = "dir/app.bin", Sha256 = name }], dest);
        Assert.Equal(payload, File.ReadAllBytes(Path.Combine(dest, "dir", "app.bin")));
        Directory.Delete(dest, recursive: true);
    }

    [Fact]
    public void ZipUnpackerRejectsDriveLetterPaths()
    {
        var payload = "hello-zip"u8.ToArray();
        var name = new BclHasher().Sha256(payload);
        using var zipStream = new MemoryStream();
        using (var zip = new ZipArchive(zipStream, ZipArchiveMode.Create, leaveOpen: true))
        {
            var entry = zip.CreateEntry(name);
            using var s = entry.Open();
            s.Write(payload);
        }

        zipStream.Position = 0;
        var dest = Path.Combine(Path.GetTempPath(), "kv-unzip-" + Guid.NewGuid().ToString("N"));
        Assert.Throws<PathException>(() =>
            new ZipArchiveUnpacker().Unpack(zipStream, [new IntegrityFile { Path = @"C:\Windows\system32\evil.bin", Sha256 = name }], dest));
        if (Directory.Exists(dest))
        {
            Directory.Delete(dest, recursive: true);
        }
    }

    [Fact]
    public async Task PackUntilReadyTreatsHttp202EmptyBodyAsPending()
    {
        var calls = 0;
        var t = new RecordingTransport
        {
            Handler = _ =>
            {
                calls++;
                if (calls == 1)
                {
                    return new TransportResponse { StatusCode = 202, Body = [] };
                }

                return new TransportResponse { StatusCode = 200, Body = """{"status":"ready","package_url":"/p","sha256":"ab"}"""u8.ToArray() };
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
        var result = await client.PackUntilReadyAsync(new PackRequest
        {
            SourceVersion = "1.0.0",
            TargetVersion = "1.1.0",
            Os = "windows",
            Arch = "x86_64",
            NeededPaths = ["a"],
        }, new PackPollOptions
        {
            InitialDelay = TimeSpan.FromMilliseconds(1),
            MaxDelay = TimeSpan.FromMilliseconds(1),
            Deadline = TimeSpan.FromSeconds(5),
        });
        Assert.Equal("ready", result.Status);
        Assert.Equal(2, t.Requests.Count);
        Assert.Equal(t.Requests[0].Url, t.Requests[1].Url);
        Assert.True((t.Requests[0].Body ?? []).AsSpan().SequenceEqual(t.Requests[1].Body ?? []));
    }

    [Fact]
    public void FileReplaceReplacerReplacesExistingFile()
    {
        var dir = Path.Combine(Path.GetTempPath(), "kv-repl-" + Guid.NewGuid().ToString("N"));
        Directory.CreateDirectory(dir);
        var dest = Path.Combine(dir, "app.bin");
        var staged = Path.Combine(dir, "staged.bin");
        File.WriteAllText(dest, "old");
        File.WriteAllText(staged, "new");
        new FileReplaceReplacer().Replace(staged, dest);
        Assert.Equal("new", File.ReadAllText(dest));
        Directory.Delete(dir, recursive: true);
    }

    [Fact]
    public void CheckPayloadMatchesServerJoinFormat()
    {
        var payload = CheckPayload.FromCheck(new UpdateCheckResponse
        {
            VersionInteger = 102,
            VersionSemver = "1.2.3",
            RootHash = "root",
            PackageUrl = "/u",
            Size = 1,
            Sha256 = "aa",
        });
        Assert.Equal("102\n1.2.3\nroot\n/u\n1\naa", payload);
    }

    [Fact]
    public void DefaultClientCheckCapabilitiesAreFullPackageOnly()
    {
        using var client = new Client(new ClientOptions
        {
            BaseUrl = TestClient.Base,
            ProjectRef = TestClient.Project,
            Transport = new RecordingTransport(),
        });
        Assert.Equal(new[] { Capability.FullPackage }, client.Capabilities);
        Assert.Empty(client.AcceptedDeltaAlgos);
        Assert.Null(client.FileStore);
        Assert.Null(client.ArchiveUnpacker);
        Assert.Null(client.Patcher);
    }

    [Fact]
    public async Task FileStoreRootAdvertisesFileListNotPatchPackage()
    {
        var t = new RecordingTransport
        {
            Handler = _ => new TransportResponse
            {
                StatusCode = 200,
                Body = """{"has_update":false,"is_mandatory":false,"is_downgrade":false,"reason":"normal","compare_engine":"semver","version_integer":null,"version_semver":"1.0.0","target_channel":"stable","target_hw_rev":null,"package_type":"single_file","root_hash":"","package_url":"","file_name":"","size":0,"sha256":"","delta_available":false}"""u8.ToArray(),
            },
        };
        var root = Path.Combine(Path.GetTempPath(), "kv-fs-root-" + Guid.NewGuid().ToString("N"));
        var client = new Client(new ClientOptions
        {
            BaseUrl = TestClient.Base,
            ProjectRef = TestClient.Project,
            Transport = t,
            FileStoreRoot = root,
        });
        Assert.Contains(Capability.FileList, client.Capabilities);
        Assert.DoesNotContain(Capability.PatchPackage, client.Capabilities);
        await client.CheckAsync(new CheckRequest { CurrentVersion = "1.0.0", Os = "windows", Arch = "x86_64" });
        using var json = System.Text.Json.JsonDocument.Parse(t.LastJson);
        var caps = json.RootElement.GetProperty("capabilities").EnumerateArray().Select(x => x.GetString()).ToArray();
        Assert.Equal(new[] { Capability.FullPackage, Capability.FileList }, caps);
        Directory.Delete(root, recursive: true);
    }

    [Fact]
    public async Task EmptyCapabilitiesOverrideStillSendsFullPackage()
    {
        var t = new RecordingTransport
        {
            Handler = _ => new TransportResponse
            {
                StatusCode = 200,
                Body = """{"has_update":false,"is_mandatory":false,"is_downgrade":false,"reason":"normal","compare_engine":"semver","version_integer":null,"version_semver":"1.0.0","target_channel":"stable","target_hw_rev":null,"package_type":"single_file","root_hash":"","package_url":"","file_name":"","size":0,"sha256":"","delta_available":false}"""u8.ToArray(),
            },
        };
        var client = TestClient.Create(t, patcher: null, unpacker: null, store: null);
        await client.CheckAsync(new CheckRequest
        {
            CurrentVersion = "1.0.0",
            Os = "windows",
            Arch = "x86_64",
            Capabilities = [],
        });
        using var json = System.Text.Json.JsonDocument.Parse(t.LastJson);
        var caps = json.RootElement.GetProperty("capabilities").EnumerateArray().Select(x => x.GetString()).ToArray();
        Assert.Equal(new[] { Capability.FullPackage }, caps);
    }

    [Fact]
    public async Task UpdaterFallsBackWhenMagicDoesNotMatchAlgo()
    {
        var full = "full-package-bytes"u8.ToArray();
        var sha = new BclHasher().Sha256(full);
        var bsdiff = "BSDIFF40xxxx"u8.ToArray();
        var t = new RecordingTransport
        {
            Handler = req =>
            {
                if (req.Url.Contains("/update/check"))
                {
                    return new TransportResponse
                    {
                        StatusCode = 200,
                        Body = Encoding.UTF8.GetBytes(
                            $$"""{"has_update":true,"is_mandatory":false,"is_downgrade":false,"reason":"normal","compare_engine":"semver","version_integer":null,"version_semver":"1.1.0","target_channel":"stable","target_hw_rev":null,"package_type":"single_file","root_hash":"","package_url":"http://127.0.0.1:8080/full.bin","file_name":"app.bin","size":{{full.Length}},"sha256":"{{sha}}","delta_available":true}"""),
                    };
                }

                if (req.Url.Contains("/update/diff"))
                {
                    return new TransportResponse
                    {
                        StatusCode = 200,
                        Body = """{"diff_mode":"binary_delta","root_hash":"","version_integer":null,"version_semver":"1.1.0","channel":"stable","compare_engine":"semver","package_url":"http://127.0.0.1:8080/delta.bin","delta_algo":"xdelta3"}"""u8.ToArray(),
                    };
                }

                if (req.Url.Contains("delta.bin"))
                {
                    return new TransportResponse { StatusCode = 200, Body = bsdiff };
                }

                if (req.Url.Contains("full.bin"))
                {
                    return new TransportResponse { StatusCode = 200, Body = full };
                }

                if (req.Url.Contains("/telemetry/report"))
                {
                    return new TransportResponse { StatusCode = 202, Body = """{"status":"ok"}"""u8.ToArray() };
                }

                return new TransportResponse { StatusCode = 200, Body = "{}"u8.ToArray() };
            },
        };

        var install = Path.Combine(Path.GetTempPath(), "kv-old-" + Guid.NewGuid().ToString("N") + ".bin");
        await File.WriteAllBytesAsync(install, "old"u8.ToArray());
        var patcher = new FakePatcher(Capability.Bsdiff, Capability.Xdelta3);
        var client = new Client(new ClientOptions
        {
            BaseUrl = TestClient.Base,
            ProjectRef = TestClient.Project,
            Transport = t,
            UseDefaultAdapters = false,
            Hasher = new BclHasher(),
            Patcher = patcher,
        });
        var result = await new Updater(client).RunAsync(new UpdateRequest
        {
            CurrentVersion = "1.0.0",
            Os = "windows",
            Arch = "x86_64",
            LocalInstallPath = install,
        });
        Assert.Equal(Capability.FullPackage, result.DiffMode);
        Assert.Equal(0, patcher.ApplyCalls);
        File.Delete(install);
    }

    [Fact]
    public void RsaSha256RoundTrip()
    {
        using var rsa = RSA.Create(2048);
        var pub = rsa.ExportSubjectPublicKeyInfoPem();
        var payload = CheckPayload.Build("102", "1.2.3", "root", "/u", "1", "aa");
        var sig = Convert.ToBase64String(rsa.SignData(Encoding.UTF8.GetBytes(payload), HashAlgorithmName.SHA256, RSASignaturePadding.Pkcs1));
        new BclSignatureVerifier().Verify(BclSignatureVerifier.RsaSha256, pub, payload, sig);
        Assert.Throws<VerifyException>(() =>
            new BclSignatureVerifier().Verify(BclSignatureVerifier.RsaSha256, pub, payload + "x", sig));
    }
}

public class Ed25519Tests
{
    [Fact]
    public void Rfc8032Test1EmptyMessage()
    {
        var pub = Convert.FromHexString("d75a980182b10ab7d54bfed3c964073a0ee172f3daa62325af021a68f707511a");
        var sig = Convert.FromHexString("e5564300c360ac729086e2cc806e828a84877f1eb8e5d974d873e065224901555fb8821590a33bacc61e39701cf9b46bd25bf5f0595bbe24655141438e7a100b");
        Assert.True(Ed25519Core.Verify(pub, ReadOnlySpan<byte>.Empty, sig));
        Assert.False(Ed25519Core.Verify(pub, "x"u8, sig));
    }

    [Fact]
    public void Rfc8032Test2OneByte()
    {
        var pub = Convert.FromHexString("3d4017c3e843895a92b70aa74d1b7ebc9c982ccf2ec4968cc0cd55f12af4660c");
        var sig = Convert.FromHexString("92a009a9f0d4cab8720e820b5f642540a2b27b5416503f8fb3762223ebdb69da085ac1e43e15996e458f3613d0f11d8c387b2eaeb4302aeeb00d291612bb0c00");
        Assert.True(Ed25519Core.Verify(pub, [0x72], sig));
    }
}
