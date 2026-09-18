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
      includeDefaultFileStore: false,
      includeDefaultArchiveUnpacker: false,
      includeDefaultReplacer: false,
    );
    addTearDown(client.close);

    try {
      await client.health();
    } on SocketException catch (e) {
      fail('client plane not reachable at $baseUrl: $e');
    } on HttpException catch (e) {
      fail('client plane not reachable at $baseUrl: $e');
    }

    final check = await client.check(
      CheckRequest(
        currentVersion: current,
        os: os,
        arch: arch,
        channel: channel,
        deviceId: deviceId,
      ),
    );

    expect(check.isNoUpdate, isFalse,
        reason: 'expected an update from $current to $target');
    expect(check.body, isNotNull);
    expect(check.body!.versionSemver, target);
    expect(check.body!.sha256, expectedSha);

    final download = await client.downloadUrl(check.body!.packageUrl);
    final actual = const CryptoHasher().sha256Hex(download.bytes);
    expect(actual, expectedSha);
    expect(download.bytes, isNotEmpty);
  }, timeout: const Timeout(Duration(minutes: 2)));
}
