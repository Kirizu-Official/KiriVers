import 'package:kirivers_client/kirivers_client.dart';

Future<void> main() async {
  final client = Client(
    config: ClientConfig(
      baseUrl: 'http://127.0.0.1:8080',
      projectRef: 'sdk-fixture',
    ),
  );
  try {
    final check = await client.check(
      CheckRequest(
        currentVersion: '1.0.0',
        os: 'windows',
        arch: 'x86_64',
        channel: 'stable',
        deviceId: 'sdk-dart-example',
      ),
    );
    if (check.isNoUpdate) {
      print('already up to date');
      return;
    }
    if (check.isNotModified) {
      print('etag hit');
      return;
    }
    final body = check.body!;
    print('update ${body.versionSemver} sha256=${body.sha256}');
    final bytes = await client.downloadUrl(body.packageUrl);
    print('downloaded ${bytes.bytes.length} bytes');
  } finally {
    client.close();
  }
}
