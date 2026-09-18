import 'dart:convert';
import 'dart:io';

import 'adapters.dart';
import 'capabilities.dart';
import 'config.dart';
import 'errors.dart';
import 'file_store.dart';
import 'hasher.dart';
import 'json_util.dart';
import 'models.dart';
import 'paths.dart';
import 'replacer.dart';
import 'signature.dart';
import 'transport.dart';
import 'unpacker.dart';

typedef SleepFn = Future<void> Function(Duration duration);

/// Handwritten native JSON client. No OpenAPI codegen.
///
/// Default check capabilities follow attached adapters (D13). HTTP 204 is
/// "no update", not an error. 304 is an ETag hit.
class Client {
  Client({
    required this.config,
    Transport? transport,
    FileStore? fileStore,
    Hasher? hasher,
    SignatureVerifier? signatureVerifier,
    ArchiveUnpacker? archiveUnpacker,
    this.patcher,
    Replacer? replacer,
    this.includeDefaultFileStore = true,
    this.includeDefaultArchiveUnpacker = true,
    this.includeDefaultReplacer = true,
    SleepFn? sleep,
  })  : transport = transport ?? HttpTransport(timeout: config.requestTimeout),
        hasher = hasher ?? const CryptoHasher(),
        signatureVerifier =
            signatureVerifier ?? CryptographySignatureVerifier(),
        _sleep = sleep ?? ((Duration d) => Future<void>.delayed(d)) {
    this.fileStore = fileStore ??
        (includeDefaultFileStore
            ? IoFileStore(root: config.fileRoot ?? Directory.systemTemp.path)
            : null);
    this.archiveUnpacker = archiveUnpacker ??
        (includeDefaultArchiveUnpacker ? const ZipArchiveUnpacker() : null);
    this.replacer = replacer ??
        (includeDefaultReplacer ? const FileRenameReplacer() : null);
  }

  final ClientConfig config;
  final Transport transport;
  late final FileStore? fileStore;
  final Hasher hasher;
  final SignatureVerifier signatureVerifier;
  late final ArchiveUnpacker? archiveUnpacker;
  final Patcher? patcher;
  late final Replacer? replacer;
  final bool includeDefaultFileStore;
  final bool includeDefaultArchiveUnpacker;
  final bool includeDefaultReplacer;
  final SleepFn _sleep;

  /// D13: `full_package` plus patch/file_list/delta only when adapters exist.
  List<String> derivedCapabilities() {
    final caps = <String>[capabilityFullPackage];
    if (archiveUnpacker != null) {
      caps.add(capabilityPatchPackage);
    }
    if (fileStore != null && fileStore!.canWriteFiles) {
      caps.add(capabilityFileList);
    }
    if (patcher != null && patcher!.supportedAlgos.isNotEmpty) {
      caps.add(capabilityBinaryDelta);
    }
    return caps;
  }

  List<String> derivedDeltaAlgos() {
    final p = patcher;
    if (p == null || p.supportedAlgos.isEmpty) {
      return const [];
    }
    return List<String>.from(p.supportedAlgos);
  }

  Future<HealthStatus> health() async {
    final res = await _send(
      method: 'GET',
      uri: _uri(NativeRoutes.health),
    );
    _ensureOk(res, const {200});
    return HealthStatus.fromJson(_mustMap(res.body));
  }

  Future<ProjectPublic> project() async {
    final res = await _send(
      method: 'GET',
      uri: _uri(_project('')),
    );
    _ensureOk(res, const {200});
    return ProjectPublic.fromJson(_mustMap(res.body));
  }

  Future<DeviceReport> deviceReport(DeviceReportInput input) async {
    final res = await _send(
      method: 'POST',
      uri: _uri(_project('/clients/report')),
      jsonBody: input.toJson(),
    );
    _ensureOk(res, const {200});
    return DeviceReport.fromJson(_mustMap(res.body));
  }

  Future<CheckResult> check(
    CheckRequest request, {
    String? ifNoneMatch,
  }) async {
    final headers = <String, String>{};
    if (ifNoneMatch != null && ifNoneMatch.isNotEmpty) {
      headers['If-None-Match'] = ifNoneMatch;
    }
    final res = await _send(
      method: 'POST',
      uri: _uri(_project('/update/check')),
      jsonBody: request.toJson(
        defaultCapabilities: derivedCapabilities(),
        defaultDeltaAlgos: derivedDeltaAlgos(),
      ),
      extraHeaders: headers,
    );
    _ensureOk(res, const {200, 204, 304});
    final etag = headerValue(res.headers, 'etag');
    if (res.statusCode == 304 || res.statusCode == 204) {
      return CheckResult(
        statusCode: res.statusCode,
        etag: etag.isEmpty ? null : etag,
        headers: res.headers,
      );
    }
    return CheckResult(
      statusCode: res.statusCode,
      body: UpdateCheck.fromJson(_mustMap(res.body)),
      etag: etag.isEmpty ? null : etag,
      headers: res.headers,
    );
  }

  Future<ChangelogResult> changelog({
    required String channel,
    required String os,
    required String arch,
    ChangelogQuery? query,
  }) async {
    final q = query ?? ChangelogQuery();
    final headers = <String, String>{};
    if (q.ifNoneMatch != null && q.ifNoneMatch!.isNotEmpty) {
      headers['If-None-Match'] = q.ifNoneMatch!;
    }
    final path =
        '${_project('/changelog/')}${_enc(channel)}/${_enc(os)}/${_enc(arch)}';
    final res = await _send(
      method: 'GET',
      uri: _uri(path, q.toQuery()),
      extraHeaders: headers,
    );
    _ensureOk(res, const {200, 304});
    final etag = headerValue(res.headers, 'etag');
    if (res.statusCode == 304) {
      return ChangelogResult(statusCode: 304, etag: etag.isEmpty ? null : etag);
    }
    return ChangelogResult.fromJson(
      _mustMap(res.body),
      statusCode: res.statusCode,
      etag: etag.isEmpty ? null : etag,
    );
  }

  Future<IntegrityManifest?> integrity({
    required String version,
    required IntegrityQuery query,
  }) async {
    final headers = <String, String>{};
    if (query.ifNoneMatch != null && query.ifNoneMatch!.isNotEmpty) {
      headers['If-None-Match'] = query.ifNoneMatch!;
    }
    final path = '${_project('/versions/')}${_enc(version)}/integrity';
    final res = await _send(
      method: 'GET',
      uri: _uri(path, query.toQuery()),
      extraHeaders: headers,
    );
    _ensureOk(res, const {200, 304});
    if (res.statusCode == 304) {
      return null;
    }
    final etag = headerValue(res.headers, 'etag');
    return IntegrityManifest.fromJson(
      _mustMap(res.body),
      etag: etag.isEmpty ? null : etag,
    );
  }

  Future<DiffResult> diff(DiffRequest request) async {
    final res = await _send(
      method: 'POST',
      uri: _uri(_project('/update/diff')),
      jsonBody: request.toJson(
        defaultCapabilities: derivedCapabilities(),
        defaultDeltaAlgos: derivedDeltaAlgos(),
      ),
    );
    _ensureOk(res, const {200});
    return DiffResult.fromJson(_mustMap(res.body));
  }

  Future<PackResult> pack(PackRequest request) async {
    final res = await _send(
      method: 'POST',
      uri: _uri(_project('/update/pack')),
      jsonBody: request.toJson(),
    );
    _ensureOk(res, const {200, 202});
    return PackResult.fromJson(_mustMap(res.body), statusCode: res.statusCode);
  }

  /// Poll the same POST `/update/pack` JSON until ready / full_package / error.
  /// Never calls admin jobs. Default backoff 1s, cap 15s.
  Future<PackResult> packUntilReady(
    PackRequest request, {
    Duration? deadline,
    Duration? initialDelay,
    Duration? maxDelay,
  }) async {
    final limit = DateTime.now().add(deadline ?? config.packPollDeadline);
    var delay = initialDelay ?? config.packPollInitialDelay;
    final cap = maxDelay ?? config.packPollMaxDelay;
    while (true) {
      final result = await pack(request);
      if (!result.isPending) {
        return result;
      }
      final now = DateTime.now();
      if (!now.isBefore(limit)) {
        return result;
      }
      var wait = delay;
      final remaining = limit.difference(now);
      if (wait > remaining) {
        wait = remaining;
      }
      if (wait > Duration.zero) {
        await _sleep(wait);
      }
      final doubled = delay * 2;
      delay = doubled > cap ? cap : doubled;
    }
  }

  Future<ByteDownload> downloadPackage({
    required String ref,
    String? range,
    Map<String, String>? query,
  }) {
    return _download(
      method: 'GET',
      uri: _uri(_project('/packages/${_enc(ref)}'), query),
      range: range,
    );
  }

  Future<ByteDownload> headPackage({
    required String ref,
    String? range,
    Map<String, String>? query,
  }) {
    return _download(
      method: 'HEAD',
      uri: _uri(_project('/packages/${_enc(ref)}'), query),
      range: range,
    );
  }

  /// Download [packageUrl] (relative or absolute). Preserves `exp`/`sig` query.
  Future<ByteDownload> downloadUrl(String packageUrl, {String? range}) {
    return _download(method: 'GET', uri: resolveUrl(packageUrl), range: range);
  }

  Future<ByteDownload> headUrl(String packageUrl, {String? range}) {
    return _download(method: 'HEAD', uri: resolveUrl(packageUrl), range: range);
  }

  Future<List<ChannelInfo>> channels() async {
    final res = await _send(method: 'GET', uri: _uri(_project('/channels')));
    _ensureOk(res, const {200});
    return asList(_mustMap(res.body)['channels'])
        .map(asMap)
        .map(ChannelInfo.fromJson)
        .toList();
  }

  Future<List<MatrixRow>> matrix() async {
    final res = await _send(method: 'GET', uri: _uri(_project('/matrix')));
    _ensureOk(res, const {200});
    return asList(_mustMap(res.body)['matrix'])
        .map(asMap)
        .map(MatrixRow.fromJson)
        .toList();
  }

  Future<List<LanguageInfo>> languages() async {
    final res = await _send(method: 'GET', uri: _uri(_project('/languages')));
    _ensureOk(res, const {200});
    return asList(_mustMap(res.body)['languages'])
        .map(asMap)
        .map(LanguageInfo.fromJson)
        .toList();
  }

  Future<AnnouncementList> announcements({AnnouncementQuery? query}) async {
    final q = query ?? AnnouncementQuery();
    final headers = <String, String>{};
    if (q.ifNoneMatch != null && q.ifNoneMatch!.isNotEmpty) {
      headers['If-None-Match'] = q.ifNoneMatch!;
    }
    if (q.acceptLanguage != null && q.acceptLanguage!.isNotEmpty) {
      headers['Accept-Language'] = q.acceptLanguage!;
    }
    final res = await _send(
      method: 'GET',
      uri: _uri(_project('/announcements'), q.toQuery()),
      extraHeaders: headers,
    );
    _ensureOk(res, const {200, 304});
    final etag = headerValue(res.headers, 'etag');
    if (res.statusCode == 304) {
      return AnnouncementList(
        statusCode: 304,
        announcements: const [],
        etag: etag.isEmpty ? null : etag,
      );
    }
    final items = asList(_mustMap(res.body)['announcements'])
        .map(asMap)
        .map(Announcement.fromJson)
        .toList();
    return AnnouncementList(
      statusCode: 200,
      announcements: items,
      etag: etag.isEmpty ? null : etag,
    );
  }

  /// Success is HTTP 202. Failures must not block apply (caller decides).
  Future<void> reportTelemetry(TelemetryReport report) async {
    final res = await _send(
      method: 'POST',
      uri: _uri(_project('/telemetry/report')),
      jsonBody: report.toJson(),
    );
    _ensureOk(res, const {202});
  }

  Future<ByteDownload> media(String id, {String? range}) {
    return _download(
      method: 'GET',
      uri: _uri(_project('/media/${_enc(id)}')),
      range: range,
    );
  }

  Future<ByteDownload> headMedia(String id, {String? range}) {
    return _download(
      method: 'HEAD',
      uri: _uri(_project('/media/${_enc(id)}')),
      range: range,
    );
  }

  Uri resolveUrl(String packageUrl) {
    final trimmed = packageUrl.trim();
    if (trimmed.startsWith('http://') || trimmed.startsWith('https://')) {
      return Uri.parse(trimmed);
    }
    if (trimmed.startsWith('/')) {
      return Uri.parse('${config.baseUrl}$trimmed');
    }
    return Uri.parse('${config.baseUrl}/$trimmed');
  }

  void close() {
    final t = transport;
    if (t is HttpTransport) {
      t.close();
    }
  }

  String _enc(String value) => Uri.encodeComponent(value);

  String _project(String rest) =>
      '/api/v1/projects/${_enc(config.projectRef)}$rest';

  Uri _uri(String path, [Map<String, String>? query]) {
    var uri = Uri.parse('${config.baseUrl}$path');
    if (query != null && query.isNotEmpty) {
      uri = uri.replace(
        queryParameters: {
          ...uri.queryParameters,
          ...query,
        },
      );
    }
    return uri;
  }

  Map<String, String> _headers({
    Map<String, String>? extra,
    bool json = false,
  }) {
    final headers = <String, String>{};
    if (json) {
      headers['Content-Type'] = 'application/json';
      headers['Accept'] = 'application/json';
    }
    final token = config.projectToken;
    if (token != null && token.isNotEmpty) {
      headers['Authorization'] = 'Bearer $token';
      headers['X-Project-Token'] = token;
    }
    final channel = config.channelToken;
    if (channel != null && channel.isNotEmpty) {
      headers['X-Channel-Token'] = channel;
    }
    if (extra != null) {
      headers.addAll(extra);
    }
    return headers;
  }

  Future<TransportResponse> _send({
    required String method,
    required Uri uri,
    Object? jsonBody,
    Map<String, String>? extraHeaders,
  }) {
    List<int>? body;
    final json = jsonBody != null;
    if (jsonBody != null) {
      body = utf8.encode(jsonEncode(jsonBody));
    }
    return transport.send(
      TransportRequest(
        method: method,
        url: uri,
        headers: _headers(extra: extraHeaders, json: json),
        body: body,
      ),
    );
  }

  Future<ByteDownload> _download({
    required String method,
    required Uri uri,
    String? range,
  }) async {
    final extra = <String, String>{};
    if (range != null && range.isNotEmpty) {
      extra['Range'] = range;
    }
    final res = await transport.send(
      TransportRequest(
        method: method,
        url: uri,
        headers: _headers(extra: extra),
      ),
    );
    _ensureOk(res, const {200, 206});
    return ByteDownload(
      statusCode: res.statusCode,
      bytes: res.body,
      headers: res.headers,
    );
  }

  void _ensureOk(TransportResponse res, Set<int> ok) {
    if (ok.contains(res.statusCode)) {
      return;
    }
    throw _apiError(res);
  }

  ApiException _apiError(TransportResponse res) {
    var code = 'HTTP_${res.statusCode}';
    var message = 'HTTP ${res.statusCode}';
    Object? details;
    final map = _tryMap(res.body);
    final err = map == null ? null : asMap(map['error']);
    if (err != null && err.isNotEmpty) {
      final c = asStringOrNull(err['code']);
      if (c != null && c.isNotEmpty) {
        code = c;
      }
      final m = asStringOrNull(err['message']);
      if (m != null && m.isNotEmpty) {
        message = m;
      }
      details = err['details'];
    }
    final retryRaw = headerValue(res.headers, 'retry-after');
    int? retry;
    if (retryRaw.isNotEmpty) {
      retry = int.tryParse(retryRaw.trim());
    }
    return ApiException(
      statusCode: res.statusCode,
      code: code,
      message: message,
      details: details,
      retryAfterSeconds: retry,
    );
  }

  Map<String, dynamic> _mustMap(List<int> body) {
    final map = _tryMap(body);
    if (map == null) {
      throw ApiException(
        statusCode: 200,
        code: 'INVALID_REQUEST',
        message: 'expected JSON object',
      );
    }
    return map;
  }

  Map<String, dynamic>? _tryMap(List<int> body) {
    if (body.isEmpty) {
      return null;
    }
    final text = utf8.decode(body);
    if (text.trim().isEmpty) {
      return null;
    }
    try {
      final decoded = jsonDecode(text);
      if (decoded is Map<String, dynamic>) {
        return decoded;
      }
      if (decoded is Map) {
        return Map<String, dynamic>.from(decoded);
      }
    } on FormatException {
      return null;
    }
    return null;
  }
}

class AnnouncementList {
  AnnouncementList({
    required this.statusCode,
    required this.announcements,
    this.etag,
  });

  final int statusCode;
  final List<Announcement> announcements;
  final String? etag;

  bool get isNotModified => statusCode == 304;
}
