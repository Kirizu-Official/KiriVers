import 'package:crypto/crypto.dart' as crypto;

import 'adapters.dart';

/// SHA-256 / MD5 via `package:crypto` (D18).
class CryptoHasher implements Hasher {
  const CryptoHasher();

  @override
  String sha256Hex(List<int> bytes) => crypto.sha256.convert(bytes).toString();

  @override
  String md5Hex(List<int> bytes) => crypto.md5.convert(bytes).toString();
}
