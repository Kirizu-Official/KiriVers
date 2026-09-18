import 'package:kirivers_client/kirivers_client.dart';
import 'package:test/test.dart';

import 'support.dart';

void main() {
  test('204 check is not an error and skips download', () async {
    final rec =
        RecordingTransport((req) => empty(204, headers: {'etag': '"abc"'}));
    final updater = Updater(client: testClient(transport: rec));
    final result = await updater.run(
      UpdatePlan(currentVersion: '1.1.0', os: 'windows', arch: 'x86_64'),
    );
    expect(result.noUpdate, isTrue);
    expect(result.stagedBytes, isNull);
    expect(rec.requests.single.method, 'POST');
  });

  test('304 is ETag hit', () async {
    final rec =
        RecordingTransport((req) => empty(304, headers: {'etag': '"x"'}));
    final updater = Updater(client: testClient(transport: rec));
    final result = await updater.run(
      UpdatePlan(
        currentVersion: '1.0.0',
        os: 'windows',
        arch: 'x86_64',
        ifNoneMatch: '"x"',
      ),
    );
    expect(result.notModified, isTrue);
  });

  test('unknown delta magic falls back to full package', () async {
    final payload = List<int>.generate(51, (i) => i);
    final hasher = const CryptoHasher();
    final realSha = hasher.sha256Hex(payload);

    final rec = RecordingTransport((req) {
      final path = req.url.path;
      if (path.endsWith('/update/check')) {
        return jsonOk(check200(
          sha256: realSha,
          deltaAvailable: true,
          packageUrl: '/api/v1/projects/sdk-fixture/packages/$realSha',
        )..['delta_algo'] = 'bsdiff');
      }
      if (path.endsWith('/update/diff')) {
        return jsonOk({
          'diff_mode': 'binary_delta',
          'root_hash': '',
          'version_integer': null,
          'version_semver': '1.1.0',
          'channel': 'stable',
          'compare_engine': 'semver',
          'package_url': '/api/v1/projects/sdk-fixture/packages/delta',
          'sha256': hasher.sha256Hex([1, 2, 3, 4]),
          'delta_algo': 'bsdiff',
        });
      }
      if (path.endsWith('/packages/delta')) {
        return TransportResponse(statusCode: 200, body: [1, 2, 3, 4]);
      }
      if (path.contains('/packages/')) {
        return TransportResponse(statusCode: 200, body: payload);
      }
      if (path.endsWith('/telemetry/report')) {
        return jsonOk({'status': 'accepted'}, status: 202);
      }
      return jsonOk({
        'error': {'code': 'NOT_FOUND', 'message': path}
      }, status: 404);
    });

    final store = MemoryFileStore(files: {
      'app.bin': [9, 9, 9]
    });
    final client = testClient(
      transport: rec,
      fileStore: store,
      patcher: FakePatcher(),
    );
    final updater = Updater(client: client);
    final result = await updater.run(
      UpdatePlan(
        currentVersion: '1.0.0',
        os: 'windows',
        arch: 'x86_64',
        localRelativePath: 'app.bin',
        apply: false,
        sendTelemetry: false,
      ),
    );
    expect(result.hasUpdate, isTrue);
    expect(result.diffMode, capabilityFullPackage);
    expect(result.stagedBytes, payload);
  });

  test('telemetry failure does not fail a verified download', () async {
    final payload = [10, 20, 30];
    final sha = const CryptoHasher().sha256Hex(payload);
    final rec = RecordingTransport((req) {
      final path = req.url.path;
      if (path.endsWith('/update/check')) {
        return jsonOk(check200(
          sha256: sha,
          packageUrl: '/api/v1/projects/sdk-fixture/packages/$sha',
        ));
      }
      if (path.contains('/packages/')) {
        return TransportResponse(statusCode: 200, body: payload);
      }
      if (path.endsWith('/telemetry/report')) {
        return jsonOk({
          'error': {'code': 'RATE_LIMITED', 'message': 'later'},
        }, status: 429);
      }
      return empty(404);
    });
    final updater = Updater(client: testClient(transport: rec));
    final result = await updater.run(
      UpdatePlan(
        currentVersion: '1.0.0',
        os: 'windows',
        arch: 'x86_64',
        apply: false,
      ),
    );
    expect(result.stagedBytes, payload);
  });
}
