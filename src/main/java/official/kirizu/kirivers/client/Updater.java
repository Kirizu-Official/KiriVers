package official.kirizu.kirivers.client;

import official.kirizu.kirivers.client.adapter.DeltaMagic;
import official.kirizu.kirivers.client.adapter.JdkSignatureVerifier;
import official.kirizu.kirivers.client.adapter.SignatureVerifier;
import official.kirizu.kirivers.client.model.ClientLoginInput;
import official.kirizu.kirivers.client.model.Diff200;
import official.kirizu.kirivers.client.model.DiffRequest;
import official.kirizu.kirivers.client.model.FileEntry;
import official.kirizu.kirivers.client.model.Integrity200;
import official.kirizu.kirivers.client.model.IntegrityQuery;
import official.kirizu.kirivers.client.model.Pack200;
import official.kirizu.kirivers.client.model.PackRequest;
import official.kirizu.kirivers.client.model.TelemetryRequest;
import official.kirizu.kirivers.client.model.UpdateCheck200;
import official.kirizu.kirivers.client.model.UpdateCheckRequest;

import java.io.IOException;
import java.nio.file.Files;
import java.nio.file.Path;
import java.util.ArrayList;
import java.util.LinkedHashMap;
import java.util.LinkedHashSet;
import java.util.List;
import java.util.Map;
import java.util.Objects;
import java.util.Set;

/**
 * Optional high-level Update: check → download/diff/pack → verify → optional patch/unpack/replace.
 * Telemetry failures never fail the result. Missing Replacer returns a staged path.
 */
public final class Updater {

  private final Client client;

  public Updater(Client client) {
    this.client = Objects.requireNonNull(client, "client");
  }

  public UpdateResult update(UpdateRequest request) throws IOException {
    Objects.requireNonNull(request, "request");
    if (request.currentVersion == null || request.os == null || request.arch == null) {
      throw new IllegalArgumentException("currentVersion, os and arch are required");
    }
    Config cfg = client.config();
    if (request.reportDevice && request.deviceId != null && !request.deviceId.isBlank()) {
      ClientLoginInput login = new ClientLoginInput();
      login.deviceId = request.deviceId;
      login.version = request.currentVersion;
      login.os = request.os;
      login.arch = request.arch;
      login.channel = request.channel;
      login.custom = request.custom;
      try {
        client.deviceReport(login);
      } catch (ApiException ignored) {
        // report is optional; check still proceeds
      }
    }

    UpdateCheckRequest checkReq = new UpdateCheckRequest();
    checkReq.currentVersion = request.currentVersion;
    checkReq.os = request.os;
    checkReq.arch = request.arch;
    checkReq.channel = request.channel;
    checkReq.hwRev = request.hwRev;
    checkReq.osVersion = request.osVersion;
    checkReq.deviceId = request.deviceId;
    CheckOutcome check = client.check(checkReq, request.ifNoneMatch);
    if (check.status() == CheckOutcome.Status.NOT_MODIFIED) {
      return new UpdateResult(UpdateResult.Status.NOT_MODIFIED, check, null, null, null, false);
    }
    if (check.status() == CheckOutcome.Status.NO_UPDATE || !check.hasUpdate()) {
      return new UpdateResult(UpdateResult.Status.UP_TO_DATE, check, null, null, null, false);
    }

    UpdateCheck200 body = check.body();
    String target = body.versionSemver != null ? body.versionSemver : String.valueOf(body.versionInteger);
    String diffMode = "full_package";
    Path staged;
    String sha;
    try {
      report(request, target, "downloading", diffMode, null, null);
      Downloaded downloaded = downloadAndVerify(request, body);
      diffMode = downloaded.diffMode;
      staged = downloaded.path;
      sha = downloaded.sha256;
      if (cfg.replacer() != null && request.targetPath != null) {
        report(request, target, "applying", diffMode, null, null);
        cfg.replacer().replace(staged, request.targetPath);
        report(request, target, "installed", diffMode, null, null);
        return new UpdateResult(UpdateResult.Status.REPLACED, check, request.targetPath, sha, diffMode, true);
      }
      report(request, target, "installed", diffMode, null, null);
      return new UpdateResult(UpdateResult.Status.STAGED, check, staged, sha, diffMode, false);
    } catch (Exception e) {
      report(request, target, "failed", diffMode, "UPDATE_FAILED", e.getMessage());
      if (e instanceof IOException io) {
        throw io;
      }
      if (e instanceof RuntimeException re) {
        throw re;
      }
      throw new IOException(e);
    }
  }

  private Downloaded downloadAndVerify(UpdateRequest request, UpdateCheck200 check) throws IOException {
    Config cfg = client.config();
    boolean downgrade = Boolean.TRUE.equals(check.isDowngrade);
    boolean multi = "multi_file".equalsIgnoreCase(check.packageType);

    if (!multi
        && !downgrade
        && Boolean.TRUE.equals(check.deltaAvailable)
        && cfg.patcher() != null
        && localSha(request) != null) {
      try {
        return applyDelta(request, check);
      } catch (Exception ignored) {
        // unknown magic / patch failure → full package
      }
    }

    if (multi && cfg.fileStore() != null && request.installDir != null) {
      try {
        return applyPack(request, check);
      } catch (Exception ignored) {
        // fall back to full zip
      }
    }

    return downloadFull(request, check, "full_package");
  }

  private Downloaded applyDelta(UpdateRequest request, UpdateCheck200 check) throws IOException {
    Config cfg = client.config();
    DiffRequest diffReq = new DiffRequest();
    diffReq.sourceVersion = request.currentVersion;
    diffReq.targetVersion = check.versionSemver != null ? check.versionSemver : String.valueOf(check.versionInteger);
    diffReq.os = request.os;
    diffReq.arch = request.arch;
    diffReq.channel = request.channel != null ? request.channel : check.targetChannel;
    diffReq.hwRev = request.hwRev;
    diffReq.deviceId = request.deviceId;
    diffReq.localSha256 = localSha(request);
    diffReq.capabilities = new ArrayList<>(cfg.capabilities());
    diffReq.acceptedDeltaAlgos = new ArrayList<>(cfg.acceptedDeltaAlgos());
    Diff200 diff = client.diff(diffReq);
    if (diff.diffMode == null || !"binary_delta".equals(diff.diffMode) || diff.packageUrl == null) {
      return downloadFull(request, check, "full_package");
    }
    byte[] delta = client.downloadUrl(diff.packageUrl, null).body();
    DeltaMagic.requireSupported(cfg.patcher().supportedAlgos(), delta);
    byte[] source = request.localFile;
    if (source == null) {
      throw new IOException("localFile is required to apply a binary delta");
    }
    byte[] patched = cfg.patcher().apply(source, delta);
    String expect = check.sha256;
    verifySha(patched, expect);
    verifySignature(
        check.versionInteger,
        check.versionSemver,
        check.rootHash,
        check.packageUrl,
        check.size,
        check.sha256,
        check.signature);
    Path dest = stageFile(request, check.fileName, patched);
    return new Downloaded(dest, expect, "binary_delta");
  }

  private Downloaded applyPack(UpdateRequest request, UpdateCheck200 check) throws IOException {
    Config cfg = client.config();
    String target = check.versionSemver != null ? check.versionSemver : String.valueOf(check.versionInteger);
    IntegrityQuery iq = new IntegrityQuery();
    iq.os = request.os;
    iq.arch = request.arch;
    iq.channel = request.channel != null ? request.channel : check.targetChannel;
    iq.hwRev = request.hwRev;
    iq.hashAlgo = "sha256";
    Integrity200 integrity = client.integrity(target, iq).body();
    if (integrity == null) {
      return downloadFull(request, check, "full_package");
    }
    List<String> needed = neededPaths(request.installDir, integrity.files);
    PackRequest packReq = new PackRequest();
    packReq.sourceVersion = request.currentVersion;
    packReq.targetVersion = target;
    packReq.os = request.os;
    packReq.arch = request.arch;
    packReq.channel = request.channel != null ? request.channel : check.targetChannel;
    packReq.hwRev = request.hwRev;
    packReq.deviceId = request.deviceId;
    packReq.neededPaths = needed;
    PackOutcome pack = client.packUntilReady(packReq);
    Pack200 body = pack.body();
    if (pack.fullPackage() || body == null || body.packageUrl == null) {
      return downloadFull(request, check, "full_package");
    }
    byte[] zip = client.downloadUrl(body.packageUrl, null).body();
    verifySha(zip, body.sha256);
    verifySignature(
        body.versionInteger != null ? body.versionInteger : check.versionInteger,
        body.versionSemver != null ? body.versionSemver : check.versionSemver,
        body.rootHash != null ? body.rootHash : check.rootHash,
        body.packageUrl,
        body.size,
        body.sha256,
        body.signature);
    Path destDir = request.stageDir;
    if (destDir == null) {
      throw new IOException("stageDir is required");
    }
    Files.createDirectories(destDir);
    if (cfg.archiveUnpacker() != null && body.files != null) {
      Map<String, String> map = new LinkedHashMap<>();
      for (FileEntry f : body.files) {
        if (f.sha256 != null && f.path != null) {
          map.put(f.sha256, PathUtil.normalize(f.path));
        }
      }
      cfg.archiveUnpacker().unpack(zip, map, destDir);
      return new Downloaded(destDir, body.sha256, "patch_package");
    }
    Path file = stageFile(request, body.fileName, zip);
    return new Downloaded(file, body.sha256, "patch_package");
  }

  private Downloaded downloadFull(UpdateRequest request, UpdateCheck200 check, String mode) throws IOException {
    if (check.packageUrl == null || check.packageUrl.isBlank()) {
      throw new IOException("check response missing package_url");
    }
    byte[] bytes = client.downloadUrl(check.packageUrl, null).body();
    verifySha(bytes, check.sha256);
    verifySignature(
        check.versionInteger,
        check.versionSemver,
        check.rootHash,
        check.packageUrl,
        check.size,
        check.sha256,
        check.signature);
    Path dest = stageFile(request, check.fileName, bytes);
    return new Downloaded(dest, check.sha256, mode);
  }

  private Path stageFile(UpdateRequest request, String fileName, byte[] bytes) throws IOException {
    Path dir = request.stageDir;
    if (dir == null) {
      throw new IOException("stageDir is required");
    }
    Files.createDirectories(dir);
    String name = (fileName == null || fileName.isBlank()) ? "package.bin" : PathUtil.normalize(fileName);
    if (name.contains("/")) {
      name = name.substring(name.lastIndexOf('/') + 1);
    }
    Path dest = dir.resolve(name);
    if (client.config().fileStore() != null) {
      client.config().fileStore().write(dest, bytes);
    } else {
      Files.write(dest, bytes);
    }
    return dest;
  }

  private void verifySha(byte[] bytes, String expected) throws IOException {
    if (expected == null || expected.isBlank() || client.config().hasher() == null) {
      return;
    }
    String got = client.config().hasher().sha256(bytes);
    if (!PathUtil.hashesEqual(got, expected)) {
      throw new IOException("sha256 mismatch");
    }
  }

  private void verifySignature(
      Long versionInteger,
      String versionSemver,
      String rootHash,
      String packageUrl,
      Long size,
      String sha,
      String signature) {
    Config cfg = client.config();
    if (signature == null || signature.isBlank() || cfg.signingKeys().isEmpty() || cfg.signatureVerifier() == null) {
      return;
    }
    byte[] payload =
        JdkSignatureVerifier.buildCheckPayload(
            JdkSignatureVerifier.decimalOrEmpty(versionInteger),
            versionSemver,
            rootHash,
            packageUrl,
            JdkSignatureVerifier.decimalOrEmpty(size),
            sha);
    SignatureVerifier.SignatureException last = null;
    for (var key : cfg.signingKeys()) {
      try {
        cfg.signatureVerifier().verify(key.algo(), key.publicKeyPem(), payload, signature);
        return;
      } catch (SignatureVerifier.SignatureException e) {
        last = e;
      }
    }
    throw new ApiException(0, "SIGNATURE_MISMATCH", last == null ? "signature mismatch" : last.getMessage(), null, null);
  }

  private List<String> neededPaths(Path installDir, List<FileEntry> files) throws IOException {
    Set<String> needed = new LinkedHashSet<>();
    if (files == null) {
      return List.of();
    }
    for (FileEntry f : files) {
      if (f.path == null) {
        continue;
      }
      String rel = PathUtil.normalize(f.path);
      Path local = installDir;
      for (String part : rel.split("/")) {
        if (!part.isEmpty()) {
          local = local.resolve(part);
        }
      }
      boolean keep = PathUtil.isKeepIfExists(f.installPolicy);
      boolean exists = client.config().fileStore() != null && client.config().fileStore().exists(local);
      if (keep && exists) {
        boolean checkHash = Boolean.TRUE.equals(f.integrityCheck);
        if (!checkHash) {
          continue;
        }
        if (f.sha256 != null && PathUtil.hashesEqual(client.config().fileStore().sha256(local), f.sha256)) {
          continue;
        }
      } else if (exists && f.sha256 != null && PathUtil.hashesEqual(client.config().fileStore().sha256(local), f.sha256)) {
        continue;
      }
      needed.add(rel);
    }
    return new ArrayList<>(needed);
  }

  private String localSha(UpdateRequest request) {
    if (request.localSha256 != null && !request.localSha256.isBlank()) {
      return request.localSha256;
    }
    if (request.localFile != null && client.config().hasher() != null) {
      return client.config().hasher().sha256(request.localFile);
    }
    return null;
  }

  private void report(
      UpdateRequest request, String toVersion, String status, String diffMode, String errorCode, String errorMessage) {
    String channel = request.channel == null || request.channel.isBlank() ? "stable" : request.channel;
    TelemetryRequest tel = new TelemetryRequest();
    tel.os = request.os;
    tel.arch = request.arch;
    tel.channel = channel;
    tel.fromVersion = request.currentVersion;
    tel.toVersion = toVersion;
    tel.status = status;
    tel.deviceId = request.deviceId;
    tel.diffMode = diffMode;
    tel.errorCode = errorCode;
    tel.errorMessage = errorMessage;
    try {
      client.reportTelemetry(tel);
    } catch (RuntimeException ignored) {
      // never block apply
    }
  }

  private static final class Downloaded {
    final Path path;
    final String sha256;
    final String diffMode;

    Downloaded(Path path, String sha256, String diffMode) {
      this.path = path;
      this.sha256 = sha256;
      this.diffMode = diffMode;
    }
  }
}
