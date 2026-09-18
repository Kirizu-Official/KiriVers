import 'models.dart';

/// HTTP request the [Transport] must perform. Query strings are already on [url].
class TransportRequest {
  TransportRequest({
    required this.method,
    required this.url,
    Map<String, String>? headers,
    this.body,
  }) : headers = Map<String, String>.unmodifiable(headers ?? const {});

  final String method;
  final Uri url;
  final Map<String, String> headers;
  final List<int>? body;
}

class TransportResponse {
  TransportResponse({
    required this.statusCode,
    Map<String, String>? headers,
    List<int>? body,
  })  : headers = Map<String, String>.unmodifiable(headers ?? const {}),
        body = List<int>.unmodifiable(body ?? const <int>[]);

  final int statusCode;
  final Map<String, String> headers;
  final List<int> body;
}

/// HTTP GET/HEAD/POST with headers, body bytes, Range, and preserved query.
abstract class Transport {
  Future<TransportResponse> send(TransportRequest request);
}

/// SHA-256 (and MD5 when integrity `hash_algo` asks). Hex, lowercase.
abstract class Hasher {
  String sha256Hex(List<int> bytes);
  String md5Hex(List<int> bytes);
}

/// Verify `signature` over [buildCheckPayload] (Ed25519 or RSA-SHA256).
abstract class SignatureVerifier {
  Future<void> verify({
    required String algo,
    required String publicKeyPem,
    required String payload,
    required String signatureBase64,
  });
}

/// Read/write/list local files and hash them for integrity compare.
abstract class FileStore {
  /// Relative paths use `/` + NFC (see [normalizePath]).
  Future<List<int>?> read(String relativePath);

  Future<void> write(String relativePath, List<int> bytes);

  Future<bool> exists(String relativePath);

  Future<List<String>> list({String prefix = ''});

  Future<String?> sha256Hex(String relativePath);

  /// When true the updater may advertise `file_list`.
  bool get canWriteFiles;
}

/// Apply one binary delta. Must reject unknown magic; never cross-decode.
abstract class Patcher {
  List<String> get supportedAlgos;

  Future<List<int>> apply({
    required List<int> oldBytes,
    required List<int> delta,
    required String algo,
  });
}

/// Replace a staged install (desktop rename, APK sideload, …).
abstract class Replacer {
  Future<void> replace({
    required String stagedPath,
    required String installPath,
  });
}

/// Unpack a native zip. Members are typically content-hash names.
abstract class ArchiveUnpacker {
  Future<Map<String, List<int>>> unpackZip({
    required List<int> zipBytes,
    required List<IntegrityFile> files,
  });
}
