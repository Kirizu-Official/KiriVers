import 'dart:convert';
import 'dart:typed_data';

import 'package:crypto/crypto.dart' as hash;
import 'package:cryptography/cryptography.dart';

import 'adapters.dart';
import 'delta.dart';
import 'errors.dart';

/// Ed25519 via `package:cryptography` (D18). RSA-SHA256 PKCS#1 v1.5 is verified
/// with Dart `BigInt` because cryptography's VM backend leaves RSA unimplemented.
///
/// Public keys are PKIX (`PUBLIC KEY`) or PKCS#1 (`RSA PUBLIC KEY`) PEM.
class CryptographySignatureVerifier implements SignatureVerifier {
  CryptographySignatureVerifier({Ed25519? ed25519})
      : _ed25519 = ed25519 ?? Ed25519();

  final Ed25519 _ed25519;

  @override
  Future<void> verify({
    required String algo,
    required String publicKeyPem,
    required String payload,
    required String signatureBase64,
  }) async {
    final sig = _decodeSig(signatureBase64);
    final message = utf8.encode(payload);
    final normalized = algo.trim().toLowerCase();
    late final bool ok;
    try {
      if (normalized == 'ed25519') {
        final key = parseEd25519PublicKey(publicKeyPem);
        ok = await _ed25519.verify(
          message,
          signature: Signature(sig, publicKey: key),
        );
      } else if (normalized == 'rsa-sha256' || normalized == 'rsasha256') {
        final key = parseRsaPublicKey(publicKeyPem);
        ok = _verifyRsaSha256Pkcs1(key, message, sig);
      } else {
        throw SignatureException('unsupported signature algorithm: $algo');
      }
    } on SignatureException {
      rethrow;
    } on FormatException catch (e) {
      throw SignatureException('invalid key or signature: $e');
    }
    if (!ok) {
      throw SignatureException('signature mismatch');
    }
  }

  static List<int> _decodeSig(String b64) {
    try {
      return base64.decode(b64.trim());
    } on FormatException catch (e) {
      throw SignatureException('invalid signature base64: $e');
    }
  }
}

/// RFC 8017 RSASSA-PKCS1-v1_5 with SHA-256.
bool _verifyRsaSha256Pkcs1(
  RsaPublicKey key,
  List<int> message,
  List<int> signature,
) {
  final n = _bytesToBigInt(key.n);
  final e = _bytesToBigInt(key.e);
  if (n <= BigInt.one || e <= BigInt.one) {
    return false;
  }
  final k = (n.bitLength + 7) >> 3;
  if (signature.length > k || signature.isEmpty) {
    return false;
  }
  final s = _bytesToBigInt(signature);
  if (s >= n) {
    return false;
  }
  final em = _i2osp(s.modPow(e, n), k);
  final digest = hash.sha256.convert(message).bytes;
  const prefix = <int>[
    0x30,
    0x31,
    0x30,
    0x0d,
    0x06,
    0x09,
    0x60,
    0x86,
    0x48,
    0x01,
    0x65,
    0x03,
    0x04,
    0x02,
    0x01,
    0x05,
    0x00,
    0x04,
    0x20,
  ];
  final t = <int>[...prefix, ...digest];
  final psLen = k - t.length - 3;
  if (psLen < 8) {
    return false;
  }
  if (em.length != k || em[0] != 0x00 || em[1] != 0x01) {
    return false;
  }
  for (var i = 0; i < psLen; i++) {
    if (em[2 + i] != 0xff) {
      return false;
    }
  }
  if (em[2 + psLen] != 0x00) {
    return false;
  }
  final tOff = 3 + psLen;
  if (tOff + t.length != k) {
    return false;
  }
  for (var i = 0; i < t.length; i++) {
    if (em[tOff + i] != t[i]) {
      return false;
    }
  }
  return true;
}

BigInt _bytesToBigInt(List<int> bytes) {
  var v = BigInt.zero;
  for (final b in bytes) {
    v = (v << 8) | BigInt.from(b);
  }
  return v;
}

List<int> _i2osp(BigInt value, int length) {
  final out = List<int>.filled(length, 0);
  var x = value;
  for (var i = length - 1; i >= 0; i--) {
    out[i] = (x & BigInt.from(0xff)).toInt();
    x = x >> 8;
  }
  return out;
}

SimplePublicKey parseEd25519PublicKey(String pem) {
  final der = decodePem(pem);
  final spki = _parseSpki(der);
  if (!_oidEquals(spki.algorithmOid, _oidEd25519)) {
    throw SignatureException('PEM is not an Ed25519 public key');
  }
  if (spki.subjectPublicKey.length != 32) {
    throw SignatureException('Ed25519 public key must be 32 bytes');
  }
  return SimplePublicKey(spki.subjectPublicKey, type: KeyPairType.ed25519);
}

RsaPublicKey parseRsaPublicKey(String pem) {
  final der = decodePem(pem);
  List<int> body = der;
  try {
    final spki = _parseSpki(der);
    if (_oidEquals(spki.algorithmOid, _oidRsaEncryption)) {
      body = spki.subjectPublicKey;
    }
  } on SignatureException {
    // PKCS#1 RSAPublicKey is a bare SEQUENCE of n, e.
  }
  final cursor = _DerCursor(body);
  final seq = cursor.read();
  if (seq.tag != 0x30) {
    throw SignatureException('RSA public key is not a SEQUENCE');
  }
  final inner = _DerCursor(seq.content);
  final n = _unsigned(inner.readInteger());
  final e = _unsigned(inner.readInteger());
  return RsaPublicKey(n: n, e: e);
}

const _oidEd25519 = [0x2b, 0x65, 0x70];
const _oidRsaEncryption = [
  0x2a,
  0x86,
  0x48,
  0x86,
  0xf7,
  0x0d,
  0x01,
  0x01,
  0x01,
];

class _Spki {
  _Spki(this.algorithmOid, this.subjectPublicKey);
  final List<int> algorithmOid;
  final List<int> subjectPublicKey;
}

_Spki _parseSpki(List<int> der) {
  final cursor = _DerCursor(der);
  final seq = cursor.read();
  if (seq.tag != 0x30) {
    throw SignatureException('SPKI is not a SEQUENCE');
  }
  final inner = _DerCursor(seq.content);
  final algSeq = inner.read();
  if (algSeq.tag != 0x30) {
    throw SignatureException('AlgorithmIdentifier is not a SEQUENCE');
  }
  final algInner = _DerCursor(algSeq.content);
  final oid = algInner.read();
  if (oid.tag != 0x06) {
    throw SignatureException('missing algorithm OID');
  }
  final bit = inner.read();
  if (bit.tag != 0x03) {
    throw SignatureException('missing subjectPublicKey BIT STRING');
  }
  if (bit.content.isEmpty) {
    throw SignatureException('empty BIT STRING');
  }
  // First byte is unused-bits count (0 for keys).
  final key = bit.content.sublist(1);
  return _Spki(oid.content, key);
}

bool _oidEquals(List<int> a, List<int> b) {
  if (a.length != b.length) {
    return false;
  }
  for (var i = 0; i < a.length; i++) {
    if (a[i] != b[i]) {
      return false;
    }
  }
  return true;
}

List<int> _unsigned(List<int> integer) {
  var v = integer;
  while (v.length > 1 && v[0] == 0) {
    v = v.sublist(1);
  }
  return v;
}

class _Der {
  _Der(this.tag, this.content);
  final int tag;
  final List<int> content;
}

class _DerCursor {
  _DerCursor(List<int> bytes)
      : _bytes = bytes is Uint8List ? bytes : Uint8List.fromList(bytes);

  final Uint8List _bytes;
  int _offset = 0;

  _Der read() {
    if (_offset >= _bytes.length) {
      throw SignatureException('truncated DER');
    }
    final tag = _bytes[_offset++];
    if (_offset >= _bytes.length) {
      throw SignatureException('truncated DER length');
    }
    var len = _bytes[_offset++];
    if (len & 0x80 != 0) {
      final n = len & 0x7f;
      if (n == 0 || n > 4 || _offset + n > _bytes.length) {
        throw SignatureException('invalid DER length');
      }
      len = 0;
      for (var i = 0; i < n; i++) {
        len = (len << 8) | _bytes[_offset++];
      }
    }
    if (_offset + len > _bytes.length) {
      throw SignatureException('truncated DER content');
    }
    final content = _bytes.sublist(_offset, _offset + len);
    _offset += len;
    return _Der(tag, content);
  }

  List<int> readInteger() {
    final der = read();
    if (der.tag != 0x02) {
      throw SignatureException('expected INTEGER');
    }
    return der.content;
  }
}
