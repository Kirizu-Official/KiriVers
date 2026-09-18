import 'package:kirivers_client/kirivers_client.dart';
import 'package:test/test.dart';

void main() {
  group('normalizePath', () {
    test('converts backslashes and NFC', () {
      expect(normalizePath(r'bin\resources\config.json'),
          'bin/resources/config.json');
      const nfd = 'caf\u0065\u0301.txt';
      const nfc = 'caf\u00e9.txt';
      expect(normalizePath(nfd), normalizePath(nfc));
      expect(
          normalizePath('bin//sub\\\\dir///file.txt'), 'bin/sub/dir/file.txt');
    });

    test('rejects traversal and absolute paths', () {
      expect(() => normalizePath('../x'), throwsA(isA<PathException>()));
      expect(() => normalizePath('/etc/passwd'), throwsA(isA<PathException>()));
      expect(() => normalizePath(r'C:\Windows'), throwsA(isA<PathException>()));
      expect(
          () => normalizePath('dir/./file.txt'), throwsA(isA<PathException>()));
      expect(() => normalizePath(''), throwsA(isA<PathException>()));
    });

    test('uniqueNormalizedPaths drops duplicates after NFC', () {
      final got = uniqueNormalizedPaths([
        r'a\b.txt',
        'a/b.txt',
        'caf\u0065\u0301.txt',
        'caf\u00e9.txt',
      ]);
      expect(got, ['a/b.txt', 'caf\u00e9.txt']);
    });
  });
}
