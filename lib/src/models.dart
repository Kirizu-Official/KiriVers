import 'json_util.dart';

class ProjectPublic {
  ProjectPublic({
    this.uuid = '',
    this.slug = '',
    this.compareEngine = '',
    this.defaultLocale = '',
    this.deviceIdPolicy = '',
    this.forceHttps = false,
    this.minimumSupportedVersion,
    this.requireClientToken = false,
    this.storageVisibility = '',
    this.createdAt,
    this.updatedAt,
  });

  final String uuid;
  final String slug;
  final String compareEngine;
  final String defaultLocale;
  final String deviceIdPolicy;
  final bool forceHttps;
  final String? minimumSupportedVersion;
  final bool requireClientToken;
  final String storageVisibility;
  final String? createdAt;
  final String? updatedAt;

  factory ProjectPublic.fromJson(Map<String, dynamic> json) {
    return ProjectPublic(
      uuid: asString(json['uuid']),
      slug: asString(json['slug']),
      compareEngine: asString(json['compare_engine']),
      defaultLocale: asString(json['default_locale']),
      deviceIdPolicy: asString(json['device_id_policy']),
      forceHttps: asBool(json['force_https']),
      minimumSupportedVersion:
          asStringOrNull(json['minimum_supported_version']),
      requireClientToken: asBool(json['require_client_token']),
      storageVisibility: asString(json['storage_visibility']),
      createdAt: asStringOrNull(json['created_at']),
      updatedAt: asStringOrNull(json['updated_at']),
    );
  }
}

class HealthStatus {
  HealthStatus({required this.ready, required this.status});

  final bool ready;
  final String status;

  factory HealthStatus.fromJson(Map<String, dynamic> json) {
    return HealthStatus(
      ready: asBool(json['ready']),
      status: asString(json['status']),
    );
  }
}

class DeviceReportInput {
  DeviceReportInput({
    required this.deviceId,
    this.os,
    this.arch,
    this.channel,
    this.version,
    this.custom,
  });

  final String deviceId;
  final String? os;
  final String? arch;
  final String? channel;
  final String? version;
  final Map<String, dynamic>? custom;

  Map<String, dynamic> toJson() {
    final out = <String, dynamic>{'device_id': deviceId};
    putIfNotNull(out, 'os', os);
    putIfNotNull(out, 'arch', arch);
    putIfNotNull(out, 'channel', channel);
    putIfNotNull(out, 'version', version);
    if (custom != null) {
      out['custom'] = custom;
    }
    return out;
  }
}

class DeviceReport {
  DeviceReport({
    required this.ip,
    required this.countryCode,
    required this.regionCode,
    required this.geoI18n,
  });

  final String ip;
  final String countryCode;
  final String regionCode;
  final Map<String, dynamic> geoI18n;

  factory DeviceReport.fromJson(Map<String, dynamic> json) {
    return DeviceReport(
      ip: asString(json['ip']),
      countryCode: asString(json['country_code']),
      regionCode: asString(json['region_code']),
      geoI18n: asMap(json['geo_i18n']),
    );
  }
}

class CheckRequest {
  CheckRequest({
    required this.currentVersion,
    required this.os,
    required this.arch,
    this.channel,
    this.hwRev,
    this.osVersion,
    this.deviceId,
    this.capabilities,
    this.acceptedDeltaAlgos,
  });

  final String currentVersion;
  final String os;
  final String arch;
  final String? channel;
  final String? hwRev;
  final String? osVersion;
  final String? deviceId;
  final List<String>? capabilities;
  final List<String>? acceptedDeltaAlgos;

  Map<String, dynamic> toJson({
    List<String>? defaultCapabilities,
    List<String>? defaultDeltaAlgos,
  }) {
    var caps = capabilities ?? defaultCapabilities;
    if (caps == null || caps.isEmpty) {
      caps = const ['full_package'];
    }
    final algos = [
      for (final algo
          in acceptedDeltaAlgos ?? defaultDeltaAlgos ?? const <String>[])
        if (algo.trim().isNotEmpty) algo.trim(),
    ];
    final out = <String, dynamic>{
      'current_version': currentVersion,
      'os': os,
      'arch': arch,
      'capabilities': caps,
    };
    putIfNotNull(out, 'channel', channel);
    putIfNotNull(out, 'hw_rev', hwRev);
    putIfNotNull(out, 'os_version', osVersion);
    putIfNotNull(out, 'device_id', deviceId);
    if (algos.isNotEmpty) {
      out['accepted_delta_algos'] = algos;
    }
    return out;
  }
}

class UpdateCheck {
  UpdateCheck({
    required this.hasUpdate,
    required this.isMandatory,
    required this.isDowngrade,
    required this.reason,
    required this.compareEngine,
    this.versionInteger,
    this.versionSemver,
    required this.targetChannel,
    this.targetHwRev,
    required this.packageType,
    required this.rootHash,
    required this.packageUrl,
    required this.fileName,
    required this.size,
    required this.sha256,
    required this.deltaAvailable,
    this.deltaAlgo,
    this.platformNotes,
    this.publishTime,
    this.signature,
    this.artifactSignature,
  });

  final bool hasUpdate;
  final bool isMandatory;
  final bool isDowngrade;
  final String reason;
  final String compareEngine;
  final int? versionInteger;
  final String? versionSemver;
  final String targetChannel;
  final String? targetHwRev;
  final String packageType;
  final String rootHash;
  final String packageUrl;
  final String fileName;
  final int size;
  final String sha256;
  final bool deltaAvailable;
  final String? deltaAlgo;
  final String? platformNotes;
  final String? publishTime;
  final String? signature;
  final String? artifactSignature;

  String get targetVersion =>
      (versionSemver != null && versionSemver!.isNotEmpty)
          ? versionSemver!
          : (versionInteger?.toString() ?? '');

  factory UpdateCheck.fromJson(Map<String, dynamic> json) {
    return UpdateCheck(
      hasUpdate: asBool(json['has_update']),
      isMandatory: asBool(json['is_mandatory']),
      isDowngrade: asBool(json['is_downgrade']),
      reason: asString(json['reason']),
      compareEngine: asString(json['compare_engine']),
      versionInteger: asInt(json['version_integer']),
      versionSemver: asStringOrNull(json['version_semver']),
      targetChannel: asString(json['target_channel']),
      targetHwRev: asStringOrNull(json['target_hw_rev']),
      packageType: asString(json['package_type']),
      rootHash: asString(json['root_hash']),
      packageUrl: asString(json['package_url']),
      fileName: asString(json['file_name']),
      size: asInt(json['size']) ?? 0,
      sha256: asString(json['sha256']).toLowerCase(),
      deltaAvailable: asBool(json['delta_available']),
      deltaAlgo: asStringOrNull(json['delta_algo']),
      platformNotes: asStringOrNull(json['platform_notes']),
      publishTime: asStringOrNull(json['publish_time']),
      signature: asStringOrNull(json['signature']),
      artifactSignature: asStringOrNull(json['artifact_signature']),
    );
  }
}

class CheckResult {
  CheckResult({
    required this.statusCode,
    this.body,
    this.etag,
    Map<String, String>? headers,
  }) : headers = Map<String, String>.unmodifiable(headers ?? const {});

  final int statusCode;
  final UpdateCheck? body;
  final String? etag;
  final Map<String, String> headers;

  bool get isNotModified => statusCode == 304;
  bool get isNoUpdate => statusCode == 204;
  bool get hasUpdate => statusCode == 200 && (body?.hasUpdate ?? false);
}

class ChangelogQuery {
  ChangelogQuery({
    this.fromVersion,
    this.toVersion,
    this.changelogScope,
    this.changelogLayout,
    this.changelogIncludeRevoked,
    this.changelogIncludePlatformNotes,
    this.changelogLocale,
    this.locale,
    this.ifNoneMatch,
  });

  final String? fromVersion;
  final String? toVersion;
  final String? changelogScope;
  final String? changelogLayout;
  final bool? changelogIncludeRevoked;
  final bool? changelogIncludePlatformNotes;
  final String? changelogLocale;
  final String? locale;
  final String? ifNoneMatch;

  Map<String, String> toQuery() {
    final q = <String, String>{};
    void put(String k, Object? v) {
      if (v == null) {
        return;
      }
      q[k] = v is bool ? (v ? 'true' : 'false') : v.toString();
    }

    put('from_version', fromVersion);
    put('to_version', toVersion);
    put('changelog_scope', changelogScope);
    put('changelog_layout', changelogLayout);
    put('changelog_include_revoked', changelogIncludeRevoked);
    put('changelog_include_platform_notes', changelogIncludePlatformNotes);
    put('changelog_locale', changelogLocale);
    put('locale', locale);
    return q;
  }
}

class ChangelogVersion {
  ChangelogVersion({
    required this.channel,
    required this.status,
    required this.changelog,
    required this.hadArtifactForRequestPlatform,
    this.versionInteger,
    this.versionSemver,
    this.title,
    this.platformNotes,
  });

  final String channel;
  final String status;
  final String changelog;
  final bool hadArtifactForRequestPlatform;
  final int? versionInteger;
  final String? versionSemver;
  final String? title;
  final String? platformNotes;

  factory ChangelogVersion.fromJson(Map<String, dynamic> json) {
    return ChangelogVersion(
      channel: asString(json['channel']),
      status: asString(json['status']),
      changelog: asString(json['changelog']),
      hadArtifactForRequestPlatform:
          asBool(json['had_artifact_for_request_platform']),
      versionInteger: asInt(json['version_integer']),
      versionSemver: asStringOrNull(json['version_semver']),
      title: asStringOrNull(json['title']),
      platformNotes: asStringOrNull(json['platform_notes']),
    );
  }
}

class ChangelogResult {
  ChangelogResult({
    required this.statusCode,
    this.changelog,
    List<ChangelogVersion>? versions,
    this.etag,
  }) : versions = List<ChangelogVersion>.unmodifiable(versions ?? const []);

  final int statusCode;
  final String? changelog;
  final List<ChangelogVersion> versions;
  final String? etag;

  bool get isNotModified => statusCode == 304;

  factory ChangelogResult.fromJson(
    Map<String, dynamic> json, {
    required int statusCode,
    String? etag,
  }) {
    return ChangelogResult(
      statusCode: statusCode,
      changelog: asStringOrNull(json['changelog']),
      versions: asList(json['changelog_versions'])
          .map(asMap)
          .map(ChangelogVersion.fromJson)
          .toList(),
      etag: etag,
    );
  }
}

class IntegrityFile {
  IntegrityFile({
    required this.path,
    required this.size,
    required this.installPolicy,
    required this.integrityCheck,
    this.sha256,
    this.md5,
    this.url,
  });

  final String path;
  final int size;
  final String installPolicy;
  final bool integrityCheck;
  final String? sha256;
  final String? md5;
  final String? url;

  factory IntegrityFile.fromJson(Map<String, dynamic> json) {
    return IntegrityFile(
      path: asString(json['path']),
      size: asInt(json['size']) ?? 0,
      installPolicy: asString(json['install_policy'], 'OVERWRITE'),
      integrityCheck: asBool(json['integrity_check'], true),
      sha256: asStringOrNull(json['sha256']),
      md5: asStringOrNull(json['md5']),
      url: asStringOrNull(json['url']),
    );
  }
}

class IntegrityManifest {
  IntegrityManifest({
    this.versionInteger,
    this.versionSemver,
    required this.channel,
    required this.packageType,
    required this.rootHash,
    required this.fullPackageUrl,
    required this.fileName,
    required this.size,
    required this.sha256,
    required this.files,
    this.signature,
    this.etag,
  });

  final int? versionInteger;
  final String? versionSemver;
  final String channel;
  final String packageType;
  final String rootHash;
  final String fullPackageUrl;
  final String fileName;
  final int size;
  final String sha256;
  final List<IntegrityFile> files;
  final String? signature;
  final String? etag;

  factory IntegrityManifest.fromJson(
    Map<String, dynamic> json, {
    String? etag,
  }) {
    return IntegrityManifest(
      versionInteger: asInt(json['version_integer']),
      versionSemver: asStringOrNull(json['version_semver']),
      channel: asString(json['channel']),
      packageType: asString(json['package_type']),
      rootHash: asString(json['root_hash']),
      fullPackageUrl: asString(json['full_package_url']),
      fileName: asString(json['file_name']),
      size: asInt(json['size']) ?? 0,
      sha256: asString(json['sha256']).toLowerCase(),
      files:
          asList(json['files']).map(asMap).map(IntegrityFile.fromJson).toList(),
      signature: asStringOrNull(json['signature']),
      etag: etag,
    );
  }
}

class IntegrityQuery {
  IntegrityQuery({
    required this.os,
    required this.arch,
    this.hashAlgo,
    this.compact,
    this.includeFileUrls,
    this.hwRev,
    this.channel,
    this.ifNoneMatch,
  });

  final String os;
  final String arch;
  final String? hashAlgo;
  final bool? compact;
  final bool? includeFileUrls;
  final String? hwRev;
  final String? channel;
  final String? ifNoneMatch;

  Map<String, String> toQuery() {
    final q = <String, String>{'os': os, 'arch': arch};
    void put(String k, Object? v) {
      if (v == null) {
        return;
      }
      q[k] = v is bool ? (v ? 'true' : 'false') : v.toString();
    }

    put('hash_algo', hashAlgo);
    put('compact', compact);
    put('include_file_urls', includeFileUrls);
    put('hw_rev', hwRev);
    put('channel', channel);
    return q;
  }
}

class DiffRequest {
  DiffRequest({
    required this.sourceVersion,
    required this.targetVersion,
    required this.os,
    required this.arch,
    this.channel,
    this.deviceId,
    this.hwRev,
    this.localSha256,
    this.capabilities,
    this.acceptedDeltaAlgos,
    this.preferFull,
  });

  final String sourceVersion;
  final String targetVersion;
  final String os;
  final String arch;
  final String? channel;
  final String? deviceId;
  final String? hwRev;
  final String? localSha256;
  final List<String>? capabilities;
  final List<String>? acceptedDeltaAlgos;
  final bool? preferFull;

  Map<String, dynamic> toJson({
    List<String>? defaultCapabilities,
    List<String>? defaultDeltaAlgos,
  }) {
    final out = <String, dynamic>{
      'source_version': sourceVersion,
      'target_version': targetVersion,
      'os': os,
      'arch': arch,
    };
    putIfNotNull(out, 'channel', channel);
    putIfNotNull(out, 'device_id', deviceId);
    putIfNotNull(out, 'hw_rev', hwRev);
    putIfNotNull(out, 'local_sha256', localSha256);
    final caps = capabilities ?? defaultCapabilities;
    if (caps != null && caps.isNotEmpty) {
      out['capabilities'] = caps;
    }
    final algos = [
      for (final algo
          in acceptedDeltaAlgos ?? defaultDeltaAlgos ?? const <String>[])
        if (algo.trim().isNotEmpty) algo.trim(),
    ];
    if (algos.isNotEmpty) {
      out['accepted_delta_algos'] = algos;
    }
    if (preferFull != null) {
      out['prefer_full'] = preferFull;
    }
    return out;
  }
}

class DiffFile {
  DiffFile({
    required this.path,
    this.sha256,
    this.md5,
    this.size,
    this.url,
  });

  final String path;
  final String? sha256;
  final String? md5;
  final int? size;
  final String? url;

  factory DiffFile.fromJson(Map<String, dynamic> json) {
    return DiffFile(
      path: asString(json['path']),
      sha256: asStringOrNull(json['sha256']),
      md5: asStringOrNull(json['md5']),
      size: asInt(json['size']),
      url: asStringOrNull(json['url']),
    );
  }
}

class DiffResult {
  DiffResult({
    required this.diffMode,
    required this.rootHash,
    this.versionInteger,
    this.versionSemver,
    required this.channel,
    required this.compareEngine,
    this.packageUrl,
    this.sha256,
    this.size,
    this.fileName,
    this.deltaAlgo,
    this.signature,
    List<DiffFile>? files,
    List<String>? deletedPaths,
    List<String>? invalidPaths,
  })  : files = List<DiffFile>.unmodifiable(files ?? const []),
        deletedPaths = List<String>.unmodifiable(deletedPaths ?? const []),
        invalidPaths = List<String>.unmodifiable(invalidPaths ?? const []);

  final String diffMode;
  final String rootHash;
  final int? versionInteger;
  final String? versionSemver;
  final String channel;
  final String compareEngine;
  final String? packageUrl;
  final String? sha256;
  final int? size;
  final String? fileName;
  final String? deltaAlgo;
  final String? signature;
  final List<DiffFile> files;
  final List<String> deletedPaths;
  final List<String> invalidPaths;

  factory DiffResult.fromJson(Map<String, dynamic> json) {
    return DiffResult(
      diffMode: asString(json['diff_mode']),
      rootHash: asString(json['root_hash']),
      versionInteger: asInt(json['version_integer']),
      versionSemver: asStringOrNull(json['version_semver']),
      channel: asString(json['channel']),
      compareEngine: asString(json['compare_engine']),
      packageUrl: asStringOrNull(json['package_url']),
      sha256: asStringOrNull(json['sha256'])?.toLowerCase(),
      size: asInt(json['size']),
      fileName: asStringOrNull(json['file_name']),
      deltaAlgo: asStringOrNull(json['delta_algo']),
      signature: asStringOrNull(json['signature']),
      files: asList(json['files']).map(asMap).map(DiffFile.fromJson).toList(),
      deletedPaths:
          asList(json['deleted_paths']).map((e) => e.toString()).toList(),
      invalidPaths:
          asList(json['invalid_paths']).map((e) => e.toString()).toList(),
    );
  }
}

class PackRequest {
  PackRequest({
    required this.sourceVersion,
    required this.targetVersion,
    required this.os,
    required this.arch,
    this.channel,
    this.deviceId,
    this.hwRev,
    List<String>? neededPaths,
  }) : neededPaths = List<String>.unmodifiable(neededPaths ?? const []);

  final String sourceVersion;
  final String targetVersion;
  final String os;
  final String arch;
  final String? channel;
  final String? deviceId;
  final String? hwRev;
  final List<String> neededPaths;

  Map<String, dynamic> toJson() {
    final out = <String, dynamic>{
      'source_version': sourceVersion,
      'target_version': targetVersion,
      'os': os,
      'arch': arch,
      'needed_paths': neededPaths,
    };
    putIfNotNull(out, 'channel', channel);
    putIfNotNull(out, 'device_id', deviceId);
    putIfNotNull(out, 'hw_rev', hwRev);
    return out;
  }
}

class PackResult {
  PackResult({
    required this.status,
    required this.statusCode,
    this.channel,
    this.compareEngine,
    this.compression,
    this.diffMode,
    this.packageUrl,
    this.sha256,
    this.size,
    this.fileName,
    this.rootHash,
    this.signature,
    this.versionInteger,
    this.versionSemver,
    List<IntegrityFile>? files,
    List<String>? deletedPaths,
    List<String>? invalidPaths,
  })  : files = List<IntegrityFile>.unmodifiable(files ?? const []),
        deletedPaths = List<String>.unmodifiable(deletedPaths ?? const []),
        invalidPaths = List<String>.unmodifiable(invalidPaths ?? const []);

  final String status;
  final int statusCode;
  final String? channel;
  final String? compareEngine;
  final String? compression;
  final String? diffMode;
  final String? packageUrl;
  final String? sha256;
  final int? size;
  final String? fileName;
  final String? rootHash;
  final String? signature;
  final int? versionInteger;
  final String? versionSemver;
  final List<IntegrityFile> files;
  final List<String> deletedPaths;
  final List<String> invalidPaths;

  bool get isPending => status == 'pending' || statusCode == 202;
  bool get isReady => status == 'ready';
  bool get isFullPackage => status == 'full_package';

  factory PackResult.fromJson(Map<String, dynamic> json,
      {required int statusCode}) {
    return PackResult(
      status: asString(json['status']),
      statusCode: statusCode,
      channel: asStringOrNull(json['channel']),
      compareEngine: asStringOrNull(json['compare_engine']),
      compression: asStringOrNull(json['compression']),
      diffMode: asStringOrNull(json['diff_mode']),
      packageUrl: asStringOrNull(json['package_url']),
      sha256: asStringOrNull(json['sha256'])?.toLowerCase(),
      size: asInt(json['size']),
      fileName: asStringOrNull(json['file_name']),
      rootHash: asStringOrNull(json['root_hash']),
      signature: asStringOrNull(json['signature']),
      versionInteger: asInt(json['version_integer']),
      versionSemver: asStringOrNull(json['version_semver']),
      files:
          asList(json['files']).map(asMap).map(IntegrityFile.fromJson).toList(),
      deletedPaths:
          asList(json['deleted_paths']).map((e) => e.toString()).toList(),
      invalidPaths:
          asList(json['invalid_paths']).map((e) => e.toString()).toList(),
    );
  }
}

class ByteDownload {
  ByteDownload({
    required this.statusCode,
    required this.bytes,
    Map<String, String>? headers,
  }) : headers = Map<String, String>.unmodifiable(headers ?? const {});

  final int statusCode;
  final List<int> bytes;
  final Map<String, String> headers;

  bool get isPartial => statusCode == 206;
}

class ChannelInfo {
  ChannelInfo({
    required this.slug,
    required this.name,
    required this.stabilityRank,
  });

  final String slug;
  final String name;
  final int stabilityRank;

  factory ChannelInfo.fromJson(Map<String, dynamic> json) {
    return ChannelInfo(
      slug: asString(json['slug']),
      name: asString(json['name']),
      stabilityRank: asInt(json['stability_rank']) ?? 0,
    );
  }
}

class MatrixRow {
  MatrixRow({
    required this.os,
    required this.arch,
    required this.packageType,
  });

  final String os;
  final String arch;
  final String packageType;

  factory MatrixRow.fromJson(Map<String, dynamic> json) {
    return MatrixRow(
      os: asString(json['os']),
      arch: asString(json['arch']),
      packageType: asString(json['package_type']),
    );
  }
}

class LanguageInfo {
  LanguageInfo({
    required this.code,
    required this.displayName,
    required this.isDefault,
    required this.sortOrder,
  });

  final String code;
  final String displayName;
  final bool isDefault;
  final int sortOrder;

  factory LanguageInfo.fromJson(Map<String, dynamic> json) {
    return LanguageInfo(
      code: asString(json['code']),
      displayName: asString(json['display_name']),
      isDefault: asBool(json['is_default']),
      sortOrder: asInt(json['sort_order']) ?? 0,
    );
  }
}

class Announcement {
  Announcement({
    required this.id,
    required this.title,
    required this.subtitle,
    required this.markdown,
    required this.locale,
    this.startsAt,
    this.endsAt,
  });

  final String id;
  final String title;
  final String subtitle;
  final String markdown;
  final String locale;
  final String? startsAt;
  final String? endsAt;

  factory Announcement.fromJson(Map<String, dynamic> json) {
    return Announcement(
      id: asString(json['id']),
      title: asString(json['title']),
      subtitle: asString(json['subtitle']),
      markdown: asString(json['markdown']),
      locale: asString(json['locale']),
      startsAt: asStringOrNull(json['starts_at']),
      endsAt: asStringOrNull(json['ends_at']),
    );
  }
}

class AnnouncementQuery {
  AnnouncementQuery({
    this.version,
    this.os,
    this.arch,
    this.locale,
    this.acceptLanguage,
    this.ifNoneMatch,
  });

  final String? version;
  final String? os;
  final String? arch;
  final String? locale;
  final String? acceptLanguage;
  final String? ifNoneMatch;

  Map<String, String> toQuery() {
    final q = <String, String>{};
    void put(String k, String? v) {
      if (v != null && v.isNotEmpty) {
        q[k] = v;
      }
    }

    put('version', version);
    put('os', os);
    put('arch', arch);
    put('locale', locale);
    return q;
  }
}

class TelemetryReport {
  TelemetryReport({
    required this.os,
    required this.arch,
    required this.channel,
    required this.fromVersion,
    required this.toVersion,
    required this.status,
    this.deviceId,
    this.diffMode,
    this.errorCode,
    this.errorMessage,
  });

  final String os;
  final String arch;
  final String channel;
  final String fromVersion;
  final String toVersion;
  final String status;
  final String? deviceId;
  final String? diffMode;
  final String? errorCode;
  final String? errorMessage;

  Map<String, dynamic> toJson() {
    final out = <String, dynamic>{
      'os': os,
      'arch': arch,
      'channel': channel,
      'from_version': fromVersion,
      'to_version': toVersion,
      'status': status,
    };
    putIfNotNull(out, 'device_id', deviceId);
    putIfNotNull(out, 'diff_mode', diffMode);
    putIfNotNull(out, 'error_code', errorCode);
    putIfNotNull(out, 'error_message', errorMessage);
    return out;
  }
}
