import 'package:unorm_dart/unorm_dart.dart' as unorm;

import 'errors.dart';

final _driveLetter = RegExp(r'^[a-zA-Z]:');

/// Normalize a relative install path the same way the server `pathutil` does:
/// trim, reject controls / absolute / drive / `.` / `..`, `\` → `/`, NFC,
/// collapse duplicate slashes.
String normalizePath(String raw) {
  final trimmed = raw.trim();
  if (trimmed.isEmpty) {
    throw PathException('path is empty');
  }
  for (var i = 0; i < trimmed.length; i++) {
    if (trimmed.codeUnitAt(i) < 0x20) {
      throw PathException('contains control character');
    }
  }
  if (trimmed.startsWith('/') || trimmed.startsWith(r'\')) {
    throw PathException('path cannot start with leading slash');
  }
  if (_driveLetter.hasMatch(trimmed)) {
    throw PathException('path cannot contain drive letter');
  }
  final slashUnified = trimmed.replaceAll(r'\', '/');
  for (final seg in slashUnified.split('/')) {
    if (_driveLetter.hasMatch(seg)) {
      throw PathException('segment cannot contain drive letter');
    }
    if (seg == '.' || seg == '..') {
      throw PathException('path traversal segment "$seg" is forbidden');
    }
  }
  final nfcNormalized = unorm.nfc(slashUnified);
  var cleaned = nfcNormalized.replaceAll(RegExp(r'/+'), '/');
  cleaned = cleaned.replaceAll(RegExp(r'^/+|/+$'), '');
  if (cleaned.isEmpty) {
    throw PathException('path normalized to empty');
  }
  for (final seg in cleaned.split('/')) {
    if (seg == '.' || seg == '..' || seg.isEmpty) {
      throw PathException('invalid segment in normalized path');
    }
  }
  return cleaned;
}

/// Unique NFC paths for pack `needed_paths`. Order is not hashed by the server.
List<String> uniqueNormalizedPaths(Iterable<String> raw) {
  final seen = <String>{};
  final out = <String>[];
  for (final item in raw) {
    final n = normalizePath(item);
    if (seen.add(n)) {
      out.add(n);
    }
  }
  return out;
}
