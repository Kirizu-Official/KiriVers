import 'dart:convert';

import 'package:kirivers_client/kirivers_client.dart';
import 'package:test/test.dart';

import 'support.dart';

void main() {
  test('default Client.check sends only full_package', () async {
    final rec = RecordingTransport((req) => jsonOk(check200()));
    final client = testClient(transport: rec);
    expect(client.derivedCapabilities(), [capabilityFullPackage]);
    expect(client.derivedDeltaAlgos(), isEmpty);
    await client.check(
      CheckRequest(currentVersion: '1.0.0', os: 'windows', arch: 'x86_64'),
    );
    final body = jsonBody(rec.requests.single);
    expect(body['capabilities'], [capabilityFullPackage]);
    expect(body.containsKey('accepted_delta_algos'), isFalse);
    expect(body.containsKey('local_sha256'), isFalse);
    expect(body.containsKey('dirty_paths'), isFalse);
  });

  test('Client.check ignores attached adapters unless capabilities are passed',
      () async {
    final rec = RecordingTransport((req) => jsonOk(check200()));
    final client = testClient(
      transport: rec,
      fileStore: MemoryFileStore(),
      unpacker: const ZipArchiveUnpacker(),
      patcher: FakePatcher(algos: ['bsdiff']),
    );
    expect(client.derivedCapabilities(), [
      capabilityFullPackage,
      capabilityPatchPackage,
      capabilityFileList,
      capabilityBinaryDelta,
    ]);
    await client.check(
      CheckRequest(currentVersion: '1.0.0', os: 'windows', arch: 'x86_64'),
    );
    expect(jsonBody(rec.requests.single)['capabilities'], ['full_package']);
    expect(
      jsonBody(rec.requests.single).containsKey('accepted_delta_algos'),
      isFalse,
    );
  });

  test('empty capabilities list still sends full_package', () async {
    final rec = RecordingTransport((req) => jsonOk(check200()));
    final client = testClient(transport: rec);
    await client.check(
      CheckRequest(
        currentVersion: '1.0.0',
        os: 'windows',
        arch: 'x86_64',
        capabilities: const [],
      ),
    );
    expect(jsonBody(rec.requests.single)['capabilities'], ['full_package']);
  });

  test('FileStore plus unpacker justify patch_package and file_list', () {
    expect(
      deriveCapabilities(
        unpacker: const ZipArchiveUnpacker(),
        fileStore: MemoryFileStore(),
      ),
      [
        capabilityFullPackage,
        capabilityPatchPackage,
        capabilityFileList,
      ],
    );
    expect(
      deriveCapabilities(unpacker: const ZipArchiveUnpacker()),
      [capabilityFullPackage, capabilityPatchPackage],
    );
  });

  test('Updater zip default advertises patch_package, not binary_delta',
      () async {
    final rec = RecordingTransport((req) => empty(204));
    final updater = Updater(client: testClient(transport: rec));
    expect(updater.derivedCapabilities(), [
      capabilityFullPackage,
      capabilityPatchPackage,
    ]);
    expect(updater.derivedDeltaAlgos(), isEmpty);
    await updater.run(
      UpdatePlan(currentVersion: '1.0.0', os: 'windows', arch: 'x86_64'),
    );
    final body = jsonBody(rec.requests.single);
    expect(body['capabilities'], contains(capabilityFullPackage));
    expect(body['capabilities'], contains(capabilityPatchPackage));
    expect(body['capabilities'], isNot(contains(capabilityBinaryDelta)));
    expect(body['capabilities'], isNot(contains(capabilityFileList)));
    expect(body.containsKey('accepted_delta_algos'), isFalse);
  });

  test('injected Patcher advertises binary_delta and algos', () async {
    final rec = RecordingTransport((req) => empty(204));
    final updater = Updater(
      client: testClient(transport: rec),
      patcher: FakePatcher(algos: ['bsdiff', 'xdelta3']),
      includeDefaultArchiveUnpacker: false,
    );
    expect(updater.derivedCapabilities(), [
      capabilityFullPackage,
      capabilityBinaryDelta,
    ]);
    await updater.run(
      UpdatePlan(currentVersion: '1.0.0', os: 'windows', arch: 'x86_64'),
    );
    final body = jsonBody(rec.requests.single);
    expect(body['capabilities'], contains(capabilityBinaryDelta));
    expect(body['accepted_delta_algos'], ['bsdiff', 'xdelta3']);
  });

  test('blank Patcher algos do not advertise binary_delta', () {
    expect(
      deriveCapabilities(patcher: FakePatcher(algos: ['', '  '])),
      [capabilityFullPackage],
    );
    expect(deriveDeltaAlgos(FakePatcher(algos: ['', '  '])), isEmpty);
  });

  test('writable FileStore on Updater advertises file_list', () async {
    final rec = RecordingTransport((req) => empty(204));
    final updater = Updater(
      client: testClient(transport: rec),
      fileStore: MemoryFileStore(),
      includeDefaultArchiveUnpacker: false,
    );
    await updater.run(
      UpdatePlan(currentVersion: '1.0.0', os: 'linux', arch: 'arm64'),
    );
    expect(
      jsonBody(rec.requests.single)['capabilities'],
      [capabilityFullPackage, capabilityFileList],
    );
  });

  test('packUntilReady repeats identical JSON', () async {
    var n = 0;
    final rec = RecordingTransport((req) {
      n++;
      if (n == 1) {
        return jsonOk({'status': 'pending'}, status: 202);
      }
      return jsonOk({
        'status': 'ready',
        'package_url': '/api/v1/projects/sdk-fixture/packages/aa',
        'sha256': 'ab' * 32,
      });
    });
    final client = testClient(transport: rec);
    final req = PackRequest(
      sourceVersion: '1.0.0',
      targetVersion: '1.1.0',
      os: 'windows',
      arch: 'x86_64',
      neededPaths: ['bin/a.dll', 'bin/b.dll'],
    );
    final result = await client.packUntilReady(req);
    expect(result.isReady, isTrue);
    expect(rec.requests, hasLength(2));
    expect(
        utf8.decode(rec.requests[0].body!), utf8.decode(rec.requests[1].body!));
    expect(
        jsonBody(rec.requests[0])['needed_paths'], ['bin/a.dll', 'bin/b.dll']);
  });

  test('error envelope surfaces code and opaque unknown codes', () async {
    final rec = RecordingTransport((req) {
      return jsonOk({
        'error': {
          'code': 'WEIRD_CODE',
          'message': 'nope',
          'details': {'k': 1},
        },
      }, status: 400);
    });
    final client = testClient(transport: rec);
    try {
      await client.project();
      fail('expected ApiException');
    } on ApiException catch (e) {
      expect(e.statusCode, 400);
      expect(e.code, 'WEIRD_CODE');
      expect(e.message, 'nope');
    }
  });

  test('RATE_LIMITED reads Retry-After', () async {
    final rec = RecordingTransport((req) {
      return TransportResponse(
        statusCode: 429,
        headers: {
          'content-type': 'application/json',
          'retry-after': '7',
        },
        body: utf8.encode(jsonEncode({
          'error': {'code': 'RATE_LIMITED', 'message': 'slow down'},
        })),
      );
    });
    final client = testClient(transport: rec);
    try {
      await client.reportTelemetry(
        TelemetryReport(
          os: 'windows',
          arch: 'x86_64',
          channel: 'stable',
          fromVersion: '1.0.0',
          toVersion: '1.1.0',
          status: telemetryFailed,
        ),
      );
      fail('expected ApiException');
    } on ApiException catch (e) {
      expect(e.code, 'RATE_LIMITED');
      expect(e.retryAfterSeconds, 7);
    }
  });

  test('leftover routes are not in the SDK surface', () {
    for (final route in NativeRoutes.outOfSdk) {
      expect(NativeRoutes.inSdk, isNot(contains(route)));
    }
    expect(
      NativeRoutes.inSdk,
      contains('POST /api/v1/projects/{project_ref}/update/check'),
    );
    expect(
      NativeRoutes.inSdk,
      isNot(contains('GET /api/v1/projects/{project_ref}/update/check')),
    );
  });
}
