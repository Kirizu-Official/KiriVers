import 'dart:convert';

import 'package:cryptography/cryptography.dart';
import 'package:kirivers_client/kirivers_client.dart';
import 'package:test/test.dart';

void main() {
  test('buildCheckPayload matches server concatenation', () {
    expect(
      buildCheckPayload(
        versionInteger: '102',
        versionSemver: '1.2.3',
        rootHash: 'roothash',
        packageUrl: '/pkg',
        size: '123456',
        sha256Hex: 'abcd',
      ),
      '102\n1.2.3\nroothash\n/pkg\n123456\nabcd',
    );
    expect(
      buildCheckPayload(
        versionSemver: '1.2.3',
        packageUrl: '/pkg',
        sha256Hex: 'abcd',
      ),
      '\n1.2.3\n\n/pkg\n\nabcd',
    );
  });

  test('Ed25519 PEM verify roundtrip', () async {
    final algo = Ed25519();
    final pair = await algo.newKeyPair();
    final public = await pair.extractPublicKey();
    final payload = buildCheckPayload(
      versionInteger: '102',
      versionSemver: '1.2.3',
      rootHash: 'root',
      packageUrl: '/u',
      size: '1',
      sha256Hex: 'aa',
    );
    final sig = await algo.sign(utf8.encode(payload), keyPair: pair);
    final pem = _ed25519Pem(public.bytes);
    final verifier = CryptographySignatureVerifier();
    await verifier.verify(
      algo: 'ed25519',
      publicKeyPem: pem,
      payload: payload,
      signatureBase64: base64.encode(sig.bytes),
    );
    await expectLater(
      verifier.verify(
        algo: 'ed25519',
        publicKeyPem: pem,
        payload: '${payload}x',
        signatureBase64: base64.encode(sig.bytes),
      ),
      throwsA(isA<SignatureException>()),
    );
  });

  test('RSA-SHA256 PEM verify uses a PKCS1v15 SHA-256 vector', () async {
    // package:cryptography's Dart RSA backend cannot generate key pairs.
    const pem = '''
-----BEGIN PUBLIC KEY-----
MIIBIjANBgkqhkiG9w0BAQEFAAOCAQ8AMIIBCgKCAQEAqb2rVraNf6gqwJXu6xFd
vw6DaT+9V7A+EEWLkPBMb2fXFGSZYOSlvgtnGC5bdI2oyzx5efsuCJubTj+DPx4X
uupJsXJS7p4tj3ufE3wIMlm0dP5VnCl/lF+h+yIxOAZeIUF7vHdRSTnuQJOsQGYm
kL0inZ7l15aAq59f2aMwrlQ9cYVFSOnVdD18wTO8mN5UucsAjSjpc6Lk7is2bvvC
V7pRl8TK+2+2QDYuDZEbPi6QMEv2Om9VoMSQoTiM3nNScm8ESVfaMIg5yLo828Z+
TqjTFLEb87Sa/wZzP6aidda0y8QD87P5tpLFgp3CG129MCqZeK8xfdUR0ZndDy5a
TwIDAQAB
-----END PUBLIC KEY-----
''';
    const sig =
        'U1AP2Bc7aT+zD1W56Qe33imHPPDo5cRPMjbkjVxxA263zkhmB/8GU8KcksL/9LQivACdutbEY++hnk4HOiE1Sc493AM7nguzbfIN5NihG4nHJh1bg4u02UA5VMgYCnJDXbi8/RORXBrFkasOhZEqVuhvmn4MRwRo3SLXIjl1vbQjWzi3Te0xNCtmcIJ5UCoI78/WXj3eUL88yhFfG2A2QYQI1QD2LqkNiA8x2qnndcy1m9qPBwYMdVs8pdNsYPyaPnjFiLWnFItqUEFvc74kQGrZuk8AUlcnlRhYdfNwQ41bbKas2SFL64q/3rNF5i2ngP1C4J3jRo572Lz6EbnENQ==';
    final payload = buildCheckPayload(
      versionInteger: '7',
      versionSemver: '',
      rootHash: 'root',
      packageUrl: '/u',
      size: '9',
      sha256Hex: 'bb',
    );
    final verifier = CryptographySignatureVerifier();
    await verifier.verify(
      algo: 'rsa-sha256',
      publicKeyPem: pem,
      payload: payload,
      signatureBase64: sig,
    );
    await expectLater(
      verifier.verify(
        algo: 'rsa-sha256',
        publicKeyPem: pem,
        payload: '${payload}x',
        signatureBase64: sig,
      ),
      throwsA(isA<SignatureException>()),
    );
  });
}

String _ed25519Pem(List<int> raw32) {
  final der = <int>[
    0x30,
    0x2a,
    0x30,
    0x05,
    0x06,
    0x03,
    0x2b,
    0x65,
    0x70,
    0x03,
    0x21,
    0x00,
    ...raw32,
  ];
  return _pem(der);
}

String _pem(List<int> der) {
  final b64 = base64.encode(der);
  final chunks = <String>[];
  for (var i = 0; i < b64.length; i += 64) {
    final end = i + 64 > b64.length ? b64.length : i + 64;
    chunks.add(b64.substring(i, end));
  }
  return '-----BEGIN PUBLIC KEY-----\n${chunks.join('\n')}\n-----END PUBLIC KEY-----\n';
}
