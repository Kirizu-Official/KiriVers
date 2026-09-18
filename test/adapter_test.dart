import 'dart:convert';

import 'package:kirivers_client/kirivers_client.dart';
import 'package:test/test.dart';

import 'support.dart';

void main() {
  test('default capabilities omit binary_delta without Patcher', () async {
    final rec = RecordingTransport((req) {
      return jsonOk(check200());
    });
    final client = testClient(
      transport: rec,
      defaultFileStore: true,
      defaultUnpacker: true,
      fileStore: MemoryFileStore(),
      unpacker: const ZipArchiveUnpacker(),
    );
    expect(client.derivedCapabilities(), [
      capabilityFullPackage,
      capabilityPatchPackage,
      capabilityFileList,
    ]);
    expect(client.derivedDeltaAlgos(), isEmpty);
    await client.check(
      CheckRequest(currentVersion: '1.0.0', os: 'windows', arch: 'x86_64'),
    );
    final body = jsonBody(rec.requests.single);
    expect(body['capabilities'], contains(capabilityFullPackage));
    expect(body['capabilities'], contains(capabilityPatchPackage));
    expect(body['capabilities'], contains(capabilityFileList));
    expect(body['capabilities'], isNot(contains(capabilityBinaryDelta)));
    expect(body.containsKey('accepted_delta_algos'), isFalse);
    expect(body.containsKey('local_sha256'), isFalse);
    expect(body.containsKey('dirty_paths'), isFalse);
  });

  test('injected Patcher advertises binary_delta and algos', () async {
    final rec = RecordingTransport((req) => jsonOk(check200()));
    final client = testClient(
      transport: rec,
      patcher: FakePatcher(algos: ['bsdiff', 'xdelta3']),
    );
    expect(client.derivedCapabilities(), [
      capabilityFullPackage,
      capabilityBinaryDelta,
    ]);
    await client.check(
      CheckRequest(currentVersion: '1.0.0', os: 'windows', arch: 'x86_64'),
    );
    final body = jsonBody(rec.requests.single);
    expect(body['capabilities'], contains(capabilityBinaryDelta));
    expect(body['accepted_delta_algos'], ['bsdiff', 'xdelta3']);
  });

  test('without FileStore or unpacker only full_package is sent', () async {
    final rec = RecordingTransport((req) => jsonOk(check200()));
    final client = testClient(transport: rec);
    expect(client.derivedCapabilities(), [capabilityFullPackage]);
    await client.check(
      CheckRequest(currentVersion: '1.0.0', os: 'linux', arch: 'arm64'),
    );
    expect(jsonBody(rec.requests.single)['capabilities'], ['full_package']);
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
