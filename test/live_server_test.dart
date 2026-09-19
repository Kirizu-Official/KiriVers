import 'dart:convert';
import 'dart:io';
import 'dart:math';

import 'package:kirivers_client/kirivers_client.dart';
import 'package:test/test.dart';

void main() {
  test('live client plane: check 1.0.0 then download 1.1.0 sha256', () async {
    final fixturePath = Platform.environment['KIRIVERS_SDK_FIXTURE'] ??
        r'D:\KiriVers\configs\sdk-fixture.json';
    final fixtureFile = File(fixturePath);
    if (!fixtureFile.existsSync()) {
      fail('sdk-fixture.json not found at $fixturePath');
    }
    final fixture =
        jsonDecode(fixtureFile.readAsStringSync()) as Map<String, dynamic>;
    final baseUrl = fixture['client_base_url'] as String;
    final projectRef = fixture['project_ref'] as String;
    final os = fixture['os'] as String;
    final arch = fixture['arch'] as String;
    final channel = fixture['channel'] as String;
    final current = fixture['current_version'] as String;
    final target = fixture['target_version'] as String;
    final shaMap = Map<String, dynamic>.from(fixture['sha256'] as Map);
    final expectedSha = (shaMap[target] as String).toLowerCase();

    final deviceId =
        'sdk-dart-${DateTime.now().microsecondsSinceEpoch}-${Random.secure().nextInt(0x7fffffff).toRadixString(16)}';

    final client = Client(
      config: ClientConfig(baseUrl: baseUrl, projectRef: projectRef),
    );
    addTearDown(client.close);

    try {
      await client.health();
    } on SocketException catch (e) {
      _writeBackendIssue(
        repro: 'GET $baseUrl/api/v1/health',
        expected: 'HTTP 200 with JSON status/ready',
        actual: e.toString(),
        suggested:
            'Keep the host client plane listening on :8080; Docker Compose is not the client API.',
      );
      fail('client plane not reachable at $baseUrl: $e');
    } on HttpException catch (e) {
      _writeBackendIssue(
        repro: 'GET $baseUrl/api/v1/health',
        expected: 'HTTP 200 with JSON status/ready',
        actual: e.toString(),
        suggested:
            'Keep the host client plane listening on :8080; Docker Compose is not the client API.',
      );
      fail('client plane not reachable at $baseUrl: $e');
    }

    late final CheckResult check;
    try {
      check = await client.check(
        CheckRequest(
          currentVersion: current,
          os: os,
          arch: arch,
          channel: channel,
          deviceId: deviceId,
        ),
      );
    } on ApiException catch (e) {
      _writeBackendIssue(
        repro:
            'GET $baseUrl/api/v1/projects/$projectRef and POST $baseUrl/api/v1/projects/$projectRef/update/check current_version=$current os=$os arch=$arch channel=$channel device_id=$deviceId',
        expected:
            'Project $projectRef exists; HTTP 200 has_update toward $target sha256=$expectedSha',
        actual: 'HTTP ${e.statusCode} ${e.code}: ${e.message}',
        suggested:
            'Re-seed the local fixture with .trellis/tasks/09-17-client-sdk/scripts/seed_local_fixture.py so slug sdk-fixture has published 1.0.0 and 1.1.0 windows/x86_64 stable artifacts.',
      );
      fail('client plane check failed: ${e.code}: ${e.message}');
    }

    if (check.isNoUpdate || check.isNotModified || check.body == null) {
      _writeBackendIssue(
        repro:
            'POST /api/v1/projects/$projectRef/update/check current_version=$current os=$os arch=$arch',
        expected: 'HTTP 200 has_update toward $target sha256=$expectedSha',
        actual:
            'status=${check.statusCode} no_update=${check.isNoUpdate} not_modified=${check.isNotModified}',
        suggested:
            'Re-seed configs/sdk-fixture.json so 1.0.0 → 1.1.0 is a published single_file line.',
      );
      fail('client plane returned no update for sdk-fixture 1.0.0');
    }

    expect(check.body!.versionSemver, target);
    expect(check.body!.sha256, expectedSha);

    final download = await client.downloadUrl(check.body!.packageUrl);
    final actual = const CryptoHasher().sha256Hex(download.bytes);
    expect(actual, expectedSha);
    expect(download.bytes, isNotEmpty);
  }, timeout: const Timeout(Duration(minutes: 2)));
}

void _writeBackendIssue({
  required String repro,
  required String expected,
  required String actual,
  required String suggested,
}) {
  File('BACKEND_ISSUE.md').writeAsStringSync('''
# Backend issue (Dart SDK integration)

Live client plane: `http://127.0.0.1:8080`
Fixture: `D:\\KiriVers\\configs\\sdk-fixture.json`
Language worktree did **not** edit server code.

## Repro

$repro

## Expected

$expected

## Actual

$actual

## Suggested fix

$suggested
''');
}
