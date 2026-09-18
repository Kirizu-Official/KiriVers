import 'dart:convert';
import 'dart:typed_data';

import 'errors.dart';

/// Magic split for binary deltas. Unknown magic must not be cross-decoded.
class DeltaMagic {
  DeltaMagic._();

  static const algoHdiffpatch = 'hdiffpatch';
  static const algoBsdiff = 'bsdiff';
  static const algoXdelta3 = 'xdelta3';

  static const kvdiffhp1 = 'KVDIFFHP1\n';
  static const hdiff13 = 'HDIFF13&';
  static const bsdiff40 = 'BSDIFF40';
  static const vcdiffPrefix = [0xD6, 0xC3, 0xC4];

  /// Returns `hdiffpatch`, `bsdiff`, `xdelta3`, or null if unknown.
  static String? detectAlgo(List<int> delta) {
    if (delta.length >= 3 &&
        delta[0] == vcdiffPrefix[0] &&
        delta[1] == vcdiffPrefix[1] &&
        delta[2] == vcdiffPrefix[2]) {
      return algoXdelta3;
    }
    final n = delta.length < 16 ? delta.length : 16;
    if (n == 0) {
      return null;
    }
    final head = utf8.decode(delta.sublist(0, n), allowMalformed: true);
    if (head.startsWith('KVDIFFHP1')) {
      return algoHdiffpatch;
    }
    if (head.startsWith(hdiff13)) {
      return algoHdiffpatch;
    }
    if (head.startsWith(bsdiff40)) {
      return algoBsdiff;
    }
    return null;
  }

  static void rejectUnknownOrMismatch(List<int> delta, String algo) {
    final detected = detectAlgo(delta);
    if (detected == null) {
      throw PatchException('unknown delta magic');
    }
    if (detected != algo) {
      throw PatchException(
        'delta magic is $detected but request algo is $algo',
      );
    }
  }
}

/// Check / integrity / diff signature payload (`pkg/signature.BuildCheckPayload`).
///
/// Empty fields stay as empty strings so positions remain distinguishable:
/// `version_integer \n version_semver \n root_hash \n package_url \n size \n sha256`.
String buildCheckPayload({
  String versionInteger = '',
  String versionSemver = '',
  String rootHash = '',
  String packageUrl = '',
  String size = '',
  String sha256Hex = '',
}) {
  return [
    versionInteger,
    versionSemver,
    rootHash,
    packageUrl,
    size,
    sha256Hex,
  ].join('\n');
}

String payloadFromCheckFields({
  int? versionInteger,
  String? versionSemver,
  String? rootHash,
  String? packageUrl,
  int? size,
  String? sha256Hex,
}) {
  return buildCheckPayload(
    versionInteger: versionInteger?.toString() ?? '',
    versionSemver: versionSemver ?? '',
    rootHash: rootHash ?? '',
    packageUrl: packageUrl ?? '',
    size: size?.toString() ?? '',
    sha256Hex: sha256Hex ?? '',
  );
}

/// Decode PEM (CERTIFICATE / PUBLIC KEY / RSA PUBLIC KEY) to DER bytes.
Uint8List decodePem(String pem) {
  final lines = pem
      .split(RegExp(r'\r?\n'))
      .where((l) => l.isNotEmpty && !l.startsWith('-----'))
      .join();
  if (lines.isEmpty) {
    throw SignatureException('empty PEM');
  }
  try {
    return Uint8List.fromList(base64.decode(lines));
  } on FormatException catch (e) {
    throw SignatureException('invalid PEM: $e');
  }
}
