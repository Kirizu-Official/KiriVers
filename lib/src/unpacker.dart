import 'package:archive/archive.dart';
import 'package:crypto/crypto.dart' as crypto;

import 'adapters.dart';
import 'models.dart';
import 'pathutil.dart';

/// Zip unpacker via `package:archive` (D18).
///
/// Native patch zips name members by content SHA-256; install paths come from
/// integrity / pack `files[].path`.
class ZipArchiveUnpacker implements ArchiveUnpacker {
  const ZipArchiveUnpacker();

  @override
  Future<Map<String, List<int>>> unpackZip({
    required List<int> zipBytes,
    required List<IntegrityFile> files,
  }) async {
    final archive = ZipDecoder().decodeBytes(zipBytes);
    final byName = <String, List<int>>{};
    final byHash = <String, List<int>>{};
    for (final file in archive) {
      if (!file.isFile) {
        continue;
      }
      final bytes = List<int>.from(file.content);
      final name = file.name.replaceAll(r'\', '/');
      byName[name] = bytes;
      final hash = crypto.sha256.convert(bytes).toString();
      byHash[hash] = bytes;
      final base = name.split('/').last.toLowerCase();
      byHash.putIfAbsent(base, () => bytes);
    }

    if (files.isEmpty) {
      return byName;
    }

    final out = <String, List<int>>{};
    for (final meta in files) {
      final dest = normalizePath(meta.path);
      List<int>? bytes;
      final sha = meta.sha256?.toLowerCase();
      if (sha != null && sha.isNotEmpty) {
        bytes = byHash[sha] ?? byName[sha] ?? byName['$sha.bin'];
      }
      bytes ??= byName[meta.path] ?? byName[dest];
      if (bytes == null) {
        continue;
      }
      out[dest] = bytes;
    }
    return out;
  }
}
