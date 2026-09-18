import 'dart:convert';

import 'package:kirivers_client/kirivers_client.dart';

class RecordingTransport implements Transport {
  RecordingTransport(this.handler);

  TransportResponse Function(TransportRequest request) handler;
  final requests = <TransportRequest>[];

  @override
  Future<TransportResponse> send(TransportRequest request) async {
    requests.add(request);
    return handler(request);
  }
}

TransportResponse jsonOk(
  Object body, {
  int status = 200,
  Map<String, String>? headers,
}) {
  return TransportResponse(
    statusCode: status,
    headers: {'content-type': 'application/json', ...?headers},
    body: utf8.encode(jsonEncode(body)),
  );
}

TransportResponse empty(int status, {Map<String, String>? headers}) {
  return TransportResponse(
      statusCode: status, headers: headers, body: const []);
}

Map<String, dynamic> jsonBody(TransportRequest request) {
  if (request.body == null || request.body!.isEmpty) {
    return <String, dynamic>{};
  }
  return Map<String, dynamic>.from(
    jsonDecode(utf8.decode(request.body!)) as Map,
  );
}

Client testClient({
  required Transport transport,
  FileStore? fileStore,
  Patcher? patcher,
  ArchiveUnpacker? unpacker,
  Replacer? replacer,
  SleepFn? sleep,
  bool defaultFileStore = false,
  bool defaultUnpacker = false,
  bool defaultReplacer = false,
  ClientConfig? config,
}) {
  return Client(
    config: config ??
        ClientConfig(
          baseUrl: 'http://127.0.0.1:8080',
          projectRef: 'sdk-fixture',
        ),
    transport: transport,
    fileStore: fileStore,
    patcher: patcher,
    archiveUnpacker: unpacker,
    replacer: replacer,
    includeDefaultFileStore: defaultFileStore,
    includeDefaultArchiveUnpacker: defaultUnpacker,
    includeDefaultReplacer: defaultReplacer,
    sleep: sleep ?? (_) async {},
  );
}

class FakePatcher implements Patcher {
  FakePatcher({
    List<String>? algos,
    this.onApply,
    this.rejectUnknown = true,
  }) : supportedAlgos = algos ?? const ['bsdiff'];

  @override
  final List<String> supportedAlgos;

  final List<int> Function(List<int> oldBytes, List<int> delta, String algo)?
      onApply;
  final bool rejectUnknown;

  @override
  Future<List<int>> apply({
    required List<int> oldBytes,
    required List<int> delta,
    required String algo,
  }) async {
    if (rejectUnknown) {
      DeltaMagic.rejectUnknownOrMismatch(delta, algo);
    }
    if (onApply != null) {
      return onApply!(oldBytes, delta, algo);
    }
    return oldBytes;
  }
}

Map<String, dynamic> check200({
  String version = '1.1.0',
  String sha256 =
      '7f063a6901ebfd0aa7b1adb8af58f4e792f13310f42213767ced623eb0669969',
  String packageType = 'single_file',
  bool deltaAvailable = false,
  String packageUrl =
      '/api/v1/projects/sdk-fixture/packages/7f063a6901ebfd0aa7b1adb8af58f4e792f13310f42213767ced623eb0669969',
}) {
  return {
    'has_update': true,
    'is_mandatory': false,
    'is_downgrade': false,
    'reason': 'normal',
    'compare_engine': 'semver',
    'version_integer': null,
    'version_semver': version,
    'target_channel': 'stable',
    'target_hw_rev': null,
    'package_type': packageType,
    'root_hash': '',
    'package_url': packageUrl,
    'file_name': 'sdk-fixture-$version-windows-x86_64.bin',
    'size': 51,
    'sha256': sha256,
    'delta_available': deltaAvailable,
  };
}
