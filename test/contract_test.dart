import 'dart:convert';
import 'dart:io';

import 'package:kirivers_client/kirivers_client.dart';
import 'package:test/test.dart';

import 'support.dart';

late Map<String, dynamic> openApiSpec;

void main() {
  setUpAll(() {
    openApiSpec = jsonDecode(File('openapi.client.json').readAsStringSync())
        as Map<String, dynamic>;
  });

  test('handwritten client covers every native OpenAPI path+method', () async {
    final rec = RecordingTransport(_handler);
    final client = testClient(transport: rec);

    await client.health();
    await client.project();
    await client.deviceReport(DeviceReportInput(deviceId: 'dev-1'));
    await client.check(
      CheckRequest(
        currentVersion: '1.0.0',
        os: 'windows',
        arch: 'x86_64',
        channel: 'stable',
        deviceId: 'dev-1',
      ),
      ifNoneMatch: '"etag-1"',
    );
    await client.changelog(
      channel: 'stable',
      os: 'windows',
      arch: 'x86_64',
      query: ChangelogQuery(fromVersion: '1.0.0', changelogScope: 'range_all'),
    );
    await client.integrity(
      version: '1.1.0',
      query: IntegrityQuery(os: 'windows', arch: 'x86_64', hashAlgo: 'sha256'),
    );
    await client.diff(
      DiffRequest(
        sourceVersion: '1.0.0',
        targetVersion: '1.1.0',
        os: 'windows',
        arch: 'x86_64',
        localSha256: 'aa' * 32,
      ),
    );
    await client.pack(
      PackRequest(
        sourceVersion: '1.0.0',
        targetVersion: '1.1.0',
        os: 'windows',
        arch: 'x86_64',
        neededPaths: ['app.bin'],
      ),
    );
    await client.downloadPackage(ref: 'ab' * 32, range: 'bytes=0-10');
    await client.headPackage(ref: 'ab' * 32);
    await client.channels();
    await client.matrix();
    await client.languages();
    await client.announcements(
      query: AnnouncementQuery(version: '1.0.0', os: 'windows', arch: 'x86_64'),
    );
    await client.reportTelemetry(
      TelemetryReport(
        os: 'windows',
        arch: 'x86_64',
        channel: 'stable',
        fromVersion: '1.0.0',
        toVersion: '1.1.0',
        status: telemetryInstalled,
        deviceId: 'dev-1',
      ),
    );
    await client.media('11111111-1111-1111-1111-111111111111');
    await client.headMedia('11111111-1111-1111-1111-111111111111');
    await client.downloadUrl(
      '/api/v1/projects/sdk-fixture/packages/${'cd' * 32}?exp=99&sig=sigvalue',
    );

    final seen = rec.requests
        .map((r) => '${r.method.toUpperCase()} ${r.url.path}')
        .toList();

    for (final route in NativeRoutes.inSdk) {
      final space = route.indexOf(' ');
      final method = route.substring(0, space);
      final template = route.substring(space + 1);
      final hit = rec.requests.any(
        (r) =>
            r.method.toUpperCase() == method &&
            NativeRoutes.matchesTemplate(r.url.path, template),
      );
      expect(hit, isTrue, reason: 'missing $route; captured $seen');
    }

    final paths = openApiSpec['paths'] as Map<String, dynamic>;
    for (final entry in paths.entries) {
      final path = entry.key;
      if (path.contains('/store/') || path.endsWith('/openapi.json')) {
        continue;
      }
      final item = entry.value as Map<String, dynamic>;
      for (final method in const [
        'get',
        'post',
        'head',
        'put',
        'patch',
        'delete'
      ]) {
        if (!item.containsKey(method)) {
          continue;
        }
        if (method == 'options') {
          continue;
        }
        final key = '${method.toUpperCase()} $path';
        if (NativeRoutes.outOfSdk.contains(key)) {
          continue;
        }
        if (path.contains('{listing_slug}') ||
            path.contains('/artifacts/') ||
            path.endsWith('/clients/login') ||
            path.endsWith('/pack/status') ||
            path.contains('/manifest')) {
          continue;
        }
        if (key == 'GET /api/v1/projects/{project_ref}/update/check') {
          continue;
        }
        expect(
          NativeRoutes.inSdk.contains(key),
          isTrue,
          reason: 'OpenAPI native path $key is not implemented',
        );
      }
    }

    for (final req in rec.requests) {
      expect(req.url.path.contains('/store/'), isFalse);
      expect(req.url.path.contains('/clients/login'), isFalse);
      expect(req.url.path.contains('/pack/status'), isFalse);
      expect(req.url.path.contains('/manifest'), isFalse);
      expect(req.url.path.contains('/artifacts/'), isFalse);
      if (req.method.toUpperCase() == 'GET') {
        expect(req.url.path.endsWith('/update/check'), isFalse);
      }
    }

    final checkReq = rec.requests.firstWhere(
      (r) => r.url.path.endsWith('/update/check'),
    );
    expect(checkReq.method, 'POST');
    expect(checkReq.headers['If-None-Match'], '"etag-1"');
    final checkBody = jsonBody(checkReq);
    for (final name in schemaRequired('UpdateCheckRequest')) {
      expect(checkBody.containsKey(name), isTrue, reason: name);
    }

    final reportReq = rec.requests.firstWhere(
      (r) => r.url.path.endsWith('/clients/report'),
    );
    for (final name in schemaRequired('ClientLoginInput')) {
      expect(jsonBody(reportReq).containsKey(name), isTrue, reason: name);
    }

    final diffReq = rec.requests.firstWhere(
      (r) => r.url.path.endsWith('/update/diff'),
    );
    final diffBody = jsonBody(diffReq);
    for (final name in schemaRequired('DiffRequest')) {
      expect(diffBody.containsKey(name), isTrue, reason: name);
    }
    expect(diffBody.containsKey('local_sha256'), isTrue);

    final packReq = rec.requests.firstWhere(
      (r) => r.url.path.endsWith('/update/pack'),
    );
    for (final name in schemaRequired('PackRequest')) {
      expect(jsonBody(packReq).containsKey(name), isTrue, reason: name);
    }

    final telReq = rec.requests.firstWhere(
      (r) => r.url.path.endsWith('/telemetry/report'),
    );
    final telBody = jsonBody(telReq);
    expect(telBody['os'], 'windows');
    expect(telBody['arch'], 'x86_64');
    expect(telBody['channel'], 'stable');
    expect(telBody['from_version'], '1.0.0');
    expect(telBody['to_version'], '1.1.0');
    expect(telBody['status'], telemetryInstalled);

    final integrityReq = rec.requests.firstWhere(
      (r) => r.url.path.endsWith('/integrity'),
    );
    expect(integrityReq.url.queryParameters['os'], 'windows');
    expect(integrityReq.url.queryParameters['arch'], 'x86_64');

    final ranged = rec.requests.firstWhere(
      (r) =>
          r.method == 'GET' &&
          r.url.path.contains('/packages/') &&
          r.headers['Range'] == 'bytes=0-10',
    );
    expect(ranged.headers['Range'], 'bytes=0-10');

    final signed = rec.requests.firstWhere(
      (r) => r.url.queryParameters['sig'] == 'sigvalue',
    );
    expect(signed.url.queryParameters['exp'], '99');
    expect(signed.url.queryParameters['sig'], 'sigvalue');
  });

  test('error envelope from OpenAPI Error schema', () async {
    final rec = RecordingTransport((req) {
      return jsonOk({
        'error': {
          'code': 'INVALID_REQUEST',
          'message': 'missing field',
          'details': null,
        },
      }, status: 400);
    });
    final client = testClient(transport: rec);
    try {
      await client.check(
        CheckRequest(currentVersion: '1.0.0', os: 'windows', arch: 'x86_64'),
      );
      fail('expected ApiException');
    } on ApiException catch (e) {
      expect(e.code, 'INVALID_REQUEST');
      expect(e.message, 'missing field');
    }
  });
}

List<String> schemaRequired(String name) {
  final components = openApiSpec['components'] as Map<String, dynamic>;
  final schemas = components['schemas'] as Map<String, dynamic>;
  final schema = schemas[name] as Map<String, dynamic>;
  return List<String>.from(schema['required'] as List);
}

TransportResponse _handler(TransportRequest req) {
  final path = req.url.path;
  if (req.method == 'HEAD') {
    return empty(200);
  }
  if (path == '/api/v1/health') {
    return jsonOk({'ready': true, 'status': 'ok'});
  }
  if (path.endsWith('/clients/report')) {
    return jsonOk({
      'ip': '127.0.0.1',
      'country_code': '',
      'region_code': '',
      'geo_i18n': <String, dynamic>{},
    });
  }
  if (path.endsWith('/update/check')) {
    return jsonOk(check200(), headers: {'etag': '"abc"'});
  }
  if (path.contains('/changelog/')) {
    return jsonOk({'changelog': 'notes', 'changelog_versions': <dynamic>[]});
  }
  if (path.endsWith('/integrity')) {
    return jsonOk({
      'version_integer': null,
      'version_semver': '1.1.0',
      'channel': 'stable',
      'package_type': 'single_file',
      'root_hash': '',
      'full_package_url': '/api/v1/projects/sdk-fixture/packages/${'ab' * 32}',
      'file_name': 'x.bin',
      'size': 1,
      'sha256': 'ab' * 32,
      'files': [
        {
          'path': 'app.bin',
          'size': 1,
          'install_policy': 'OVERWRITE',
          'integrity_check': true,
          'sha256': 'ab' * 32,
        }
      ],
    });
  }
  if (path.endsWith('/update/diff')) {
    return jsonOk({
      'diff_mode': 'full_package',
      'root_hash': '',
      'version_integer': null,
      'version_semver': '1.1.0',
      'channel': 'stable',
      'compare_engine': 'semver',
      'package_url': '/api/v1/projects/sdk-fixture/packages/${'ab' * 32}',
    });
  }
  if (path.endsWith('/update/pack')) {
    return jsonOk({'status': 'full_package'});
  }
  if (path.contains('/packages/') || path.contains('/media/')) {
    return TransportResponse(statusCode: 200, body: [1, 2, 3]);
  }
  if (path.endsWith('/channels')) {
    return jsonOk({'channels': <dynamic>[]});
  }
  if (path.endsWith('/matrix')) {
    return jsonOk({'matrix': <dynamic>[]});
  }
  if (path.endsWith('/languages')) {
    return jsonOk({'languages': <dynamic>[]});
  }
  if (path.endsWith('/announcements')) {
    return jsonOk({'announcements': <dynamic>[]});
  }
  if (path.endsWith('/telemetry/report')) {
    return jsonOk({'status': 'accepted'}, status: 202);
  }
  if (path.startsWith('/api/v1/projects/') &&
      path.split('/').where((s) => s.isNotEmpty).length == 4) {
    return jsonOk({
      'uuid': '00000000-0000-0000-0000-000000000000',
      'slug': 'sdk-fixture',
      'compare_engine': 'semver',
      'force_https': false,
      'require_client_token': false,
    });
  }
  return jsonOk({
    'error': {'code': 'NOT_FOUND', 'message': path},
  }, status: 404);
}
