import 'dart:io';

import 'adapters.dart';
import 'capabilities.dart';
import 'client.dart';
import 'delta.dart';
import 'errors.dart';
import 'models.dart';
import 'pathutil.dart';

/// Caller identity and apply options. The SDK does not invent [deviceId].
class UpdatePlan {
  UpdatePlan({
    required this.currentVersion,
    required this.os,
    required this.arch,
    this.channel,
    this.deviceId,
    this.osVersion,
    this.hwRev,
    this.ifNoneMatch,
    this.reportDevice = false,
    this.apply = true,
    this.sendTelemetry = true,
    this.localFilePath,
    this.localRelativePath,
    this.localSha256,
    this.stageDir,
    this.installPath,
  });

  final String currentVersion;
  final String os;
  final String arch;
  final String? channel;
  final String? deviceId;
  final String? osVersion;
  final String? hwRev;
  final String? ifNoneMatch;
  final bool reportDevice;
  final bool apply;
  final bool sendTelemetry;
  final String? localFilePath;
  final String? localRelativePath;
  final String? localSha256;
  final String? stageDir;
  final String? installPath;
}

class UpdateResult {
  UpdateResult({
    required this.check,
    this.stagedPath,
    this.stagedBytes,
    this.applied = false,
    this.diffMode = capabilityFullPackage,
  });

  final CheckResult check;
  final String? stagedPath;
  final List<int>? stagedBytes;
  final bool applied;
  final String diffMode;

  bool get hasUpdate => check.hasUpdate;
  bool get noUpdate => check.isNoUpdate;
  bool get notModified => check.isNotModified;
}

/// High-level check → download → verify → optional patch/unpack → optional replace.
///
/// Missing [Replacer] is not a failure: verified bytes/path are returned.
/// Telemetry errors never fail the result. Unknown delta magic falls back to
/// the full package (never cross-decoded).
class Updater {
  Updater({required this.client});

  final Client client;

  Future<UpdateResult> run(UpdatePlan plan) async {
    final channel = plan.channel ?? 'stable';
    if (plan.reportDevice &&
        plan.deviceId != null &&
        plan.deviceId!.isNotEmpty) {
      await client.deviceReport(
        DeviceReportInput(
          deviceId: plan.deviceId!,
          os: plan.os,
          arch: plan.arch,
          channel: channel,
          version: plan.currentVersion,
        ),
      );
    }

    final check = await client.check(
      CheckRequest(
        currentVersion: plan.currentVersion,
        os: plan.os,
        arch: plan.arch,
        channel: plan.channel,
        hwRev: plan.hwRev,
        osVersion: plan.osVersion,
        deviceId: plan.deviceId,
      ),
      ifNoneMatch: plan.ifNoneMatch,
    );

    if (check.isNotModified || check.isNoUpdate || check.body == null) {
      return UpdateResult(check: check);
    }

    final body = check.body!;
    var mode = capabilityFullPackage;
    List<int>? bytes;

    try {
      await _verifyCheckSignature(body);
      await _telemetry(plan, body, telemetryDownloading, mode);

      bytes = await _obtainBytes(plan, body);
      mode = _lastMode;
      if (mode != capabilityPatchPackage) {
        _verifyBytes(bytes, body.sha256);
      }

      final staged = await _stage(plan, body, bytes);
      var applied = false;
      if (plan.apply &&
          client.replacer != null &&
          plan.installPath != null &&
          staged != null) {
        await _telemetry(plan, body, telemetryApplying, mode);
        await client.replacer!.replace(
          stagedPath: staged,
          installPath: plan.installPath!,
        );
        applied = true;
      }
      await _telemetry(plan, body, telemetryInstalled, mode);
      return UpdateResult(
        check: check,
        stagedPath: staged,
        stagedBytes: bytes,
        applied: applied,
        diffMode: mode,
      );
    } catch (e) {
      await _telemetry(
        plan,
        body,
        telemetryFailed,
        mode,
        errorCode: e is ApiException ? e.code : e.runtimeType.toString(),
        errorMessage: e.toString(),
      );
      rethrow;
    }
  }

  String _lastMode = capabilityFullPackage;

  Future<List<int>> _obtainBytes(UpdatePlan plan, UpdateCheck body) async {
    _lastMode = capabilityFullPackage;

    if (body.packageType == 'single_file' &&
        client.patcher != null &&
        body.deltaAvailable &&
        !body.isDowngrade) {
      try {
        final patched = await _tryDelta(plan, body);
        if (patched != null) {
          _lastMode = capabilityBinaryDelta;
          return patched;
        }
      } catch (_) {
        // Unknown magic, hash mismatch, or patcher failure → full package.
      }
    }

    if (body.packageType == 'multi_file' && client.fileStore != null) {
      try {
        final packed = await _tryPack(plan, body);
        if (packed != null) {
          return packed;
        }
      } catch (_) {
        // Fall back to the check full package.
      }
    }

    _lastMode = capabilityFullPackage;
    return _downloadVerified(body.packageUrl, body.sha256);
  }

  Future<List<int>?> _tryDelta(UpdatePlan plan, UpdateCheck body) async {
    final local = await _localSha256(plan);
    if (local == null) {
      return null;
    }
    final oldBytes = await _oldBytes(plan);
    if (oldBytes == null) {
      return null;
    }
    final diff = await client.diff(
      DiffRequest(
        sourceVersion: plan.currentVersion,
        targetVersion: body.targetVersion,
        os: plan.os,
        arch: plan.arch,
        channel: plan.channel ?? body.targetChannel,
        deviceId: plan.deviceId,
        hwRev: plan.hwRev,
        localSha256: local,
      ),
    );
    if (diff.diffMode != capabilityBinaryDelta ||
        diff.packageUrl == null ||
        diff.packageUrl!.isEmpty) {
      return null;
    }
    final delta = await client.downloadUrl(diff.packageUrl!);
    if (diff.sha256 != null && diff.sha256!.isNotEmpty) {
      final hex = client.hasher.sha256Hex(delta.bytes);
      if (hex != diff.sha256) {
        throw HashMismatchException(expected: diff.sha256!, actual: hex);
      }
    }
    final algo = (diff.deltaAlgo ?? body.deltaAlgo ?? '').trim();
    if (algo.isEmpty) {
      throw PatchException('missing delta_algo');
    }
    DeltaMagic.rejectUnknownOrMismatch(delta.bytes, algo);
    return client.patcher!.apply(
      oldBytes: oldBytes,
      delta: delta.bytes,
      algo: algo,
    );
  }

  Future<List<int>?> _tryPack(UpdatePlan plan, UpdateCheck body) async {
    final manifest = await client.integrity(
      version: body.targetVersion,
      query: IntegrityQuery(
        os: plan.os,
        arch: plan.arch,
        channel: plan.channel ?? body.targetChannel,
        hwRev: plan.hwRev,
      ),
    );
    if (manifest == null) {
      return null;
    }
    final needed = await _neededPaths(manifest);
    final pack = await client.packUntilReady(
      PackRequest(
        sourceVersion: plan.currentVersion,
        targetVersion: body.targetVersion,
        os: plan.os,
        arch: plan.arch,
        channel: plan.channel ?? body.targetChannel,
        deviceId: plan.deviceId,
        hwRev: plan.hwRev,
        neededPaths: needed,
      ),
    );
    if (pack.isFullPackage) {
      _lastMode = capabilityFullPackage;
      return _downloadVerified(body.packageUrl, body.sha256);
    }
    if (!pack.isReady || pack.packageUrl == null || pack.packageUrl!.isEmpty) {
      return null;
    }
    final zip = await client.downloadUrl(pack.packageUrl!);
    if (pack.sha256 != null && pack.sha256!.isNotEmpty) {
      final hex = client.hasher.sha256Hex(zip.bytes);
      if (hex != pack.sha256) {
        throw HashMismatchException(expected: pack.sha256!, actual: hex);
      }
    }
    final unpacker = client.archiveUnpacker;
    final store = client.fileStore;
    if (unpacker != null && store != null) {
      final files = pack.files.isNotEmpty ? pack.files : manifest.files;
      final unpacked = await unpacker.unpackZip(
        zipBytes: zip.bytes,
        files: files,
      );
      for (final entry in unpacked.entries) {
        await store.write(entry.key, entry.value);
      }
      _lastMode = capabilityPatchPackage;
      return zip.bytes;
    }
    _lastMode = capabilityPatchPackage;
    return zip.bytes;
  }

  Future<List<String>> _neededPaths(IntegrityManifest manifest) async {
    final store = client.fileStore!;
    final needed = <String>[];
    for (final file in manifest.files) {
      late final String path;
      try {
        path = normalizePath(file.path);
      } on PathException {
        continue;
      }
      final exists = await store.exists(path);
      if (file.installPolicy.toUpperCase() == 'KEEP_IF_EXISTS' && exists) {
        continue;
      }
      if (file.sha256 != null && file.sha256!.isNotEmpty) {
        final local = await store.sha256Hex(path);
        if (local != null && local == file.sha256!.toLowerCase()) {
          continue;
        }
      }
      needed.add(path);
    }
    return uniqueNormalizedPaths(needed);
  }

  Future<String?> _localSha256(UpdatePlan plan) async {
    if (plan.localSha256 != null && plan.localSha256!.isNotEmpty) {
      return plan.localSha256!.toLowerCase();
    }
    if (plan.localRelativePath != null && client.fileStore != null) {
      return client.fileStore!.sha256Hex(plan.localRelativePath!);
    }
    final bytes = await _oldBytes(plan);
    if (bytes == null) {
      return null;
    }
    return client.hasher.sha256Hex(bytes);
  }

  Future<List<int>?> _oldBytes(UpdatePlan plan) async {
    if (plan.localRelativePath != null && client.fileStore != null) {
      return client.fileStore!.read(plan.localRelativePath!);
    }
    if (plan.localFilePath != null) {
      final file = File(plan.localFilePath!);
      if (await file.exists()) {
        return file.readAsBytes();
      }
    }
    return null;
  }

  Future<List<int>> _downloadVerified(String url, String expected) async {
    final dl = await client.downloadUrl(url);
    _verifyBytes(dl.bytes, expected);
    return dl.bytes;
  }

  void _verifyBytes(List<int> bytes, String expected) {
    if (expected.isEmpty) {
      return;
    }
    final actual = client.hasher.sha256Hex(bytes);
    if (actual != expected.toLowerCase()) {
      throw HashMismatchException(
          expected: expected.toLowerCase(), actual: actual);
    }
  }

  Future<void> _verifyCheckSignature(UpdateCheck body) async {
    final pem = client.config.signingPublicKeyPem;
    final sig = body.signature;
    if (pem == null || pem.isEmpty || sig == null || sig.isEmpty) {
      return;
    }
    final payload = payloadFromCheckFields(
      versionInteger: body.versionInteger,
      versionSemver: body.versionSemver,
      rootHash: body.rootHash,
      packageUrl: body.packageUrl,
      size: body.size,
      sha256Hex: body.sha256,
    );
    final algo = client.config.signingAlgo;
    if (algo != null && algo.isNotEmpty) {
      await client.signatureVerifier.verify(
        algo: algo,
        publicKeyPem: pem,
        payload: payload,
        signatureBase64: sig,
      );
      return;
    }
    try {
      await client.signatureVerifier.verify(
        algo: 'ed25519',
        publicKeyPem: pem,
        payload: payload,
        signatureBase64: sig,
      );
    } on SignatureException {
      await client.signatureVerifier.verify(
        algo: 'rsa-sha256',
        publicKeyPem: pem,
        payload: payload,
        signatureBase64: sig,
      );
    }
  }

  Future<String?> _stage(
    UpdatePlan plan,
    UpdateCheck body,
    List<int> bytes,
  ) async {
    final dir =
        plan.stageDir ?? client.config.fileRoot ?? Directory.systemTemp.path;
    final name = body.fileName.isNotEmpty ? body.fileName : 'package.bin';
    final safeName = name.replaceAll(r'\', '/').split('/').last;
    if (safeName.isEmpty || safeName == '.' || safeName == '..') {
      return null;
    }
    final path = '$dir${Platform.pathSeparator}$safeName';
    final file = File(path);
    await file.parent.create(recursive: true);
    await file.writeAsBytes(bytes, flush: true);
    return path;
  }

  Future<void> _telemetry(
    UpdatePlan plan,
    UpdateCheck body,
    String status,
    String mode, {
    String? errorCode,
    String? errorMessage,
  }) async {
    if (!plan.sendTelemetry) {
      return;
    }
    try {
      await client.reportTelemetry(
        TelemetryReport(
          os: plan.os,
          arch: plan.arch,
          channel: plan.channel ?? body.targetChannel,
          fromVersion: plan.currentVersion,
          toVersion: body.targetVersion,
          status: status,
          deviceId: plan.deviceId,
          diffMode: mode,
          errorCode: errorCode,
          errorMessage: errorMessage,
        ),
      );
    } catch (_) {
      // Telemetry must never block apply.
    }
  }
}
