import 'dart:io';

import 'package:crypto/crypto.dart' as crypto;

import 'adapters.dart';
import 'pathutil.dart';

/// dart:io [FileStore]. Paths are stored relative to [root] using `/` + NFC.
class IoFileStore implements FileStore {
  IoFileStore({required this.root}) : _root = Directory(root);

  final String root;
  final Directory _root;

  @override
  bool get canWriteFiles => true;

  File _file(String relativePath) {
    final norm = normalizePath(relativePath);
    var path = _root.path;
    for (final seg in norm.split('/')) {
      path = '$path${Platform.pathSeparator}$seg';
    }
    return File(path);
  }

  @override
  Future<List<int>?> read(String relativePath) async {
    final file = _file(relativePath);
    if (!await file.exists()) {
      return null;
    }
    return file.readAsBytes();
  }

  @override
  Future<void> write(String relativePath, List<int> bytes) async {
    final file = _file(relativePath);
    await file.parent.create(recursive: true);
    await file.writeAsBytes(bytes, flush: true);
  }

  @override
  Future<bool> exists(String relativePath) => _file(relativePath).exists();

  @override
  Future<List<String>> list({String prefix = ''}) async {
    if (!await _root.exists()) {
      return const [];
    }
    final rootAbs = _root.absolute.path;
    final out = <String>[];
    await for (final entity
        in _root.list(recursive: true, followLinks: false)) {
      if (entity is! File) {
        continue;
      }
      var rel = entity.absolute.path;
      if (rel.length >= rootAbs.length) {
        rel = rel.substring(rootAbs.length);
      }
      while (rel.startsWith(r'\') || rel.startsWith('/')) {
        rel = rel.substring(1);
      }
      rel = rel.replaceAll(r'\', '/');
      if (rel.isEmpty) {
        continue;
      }
      try {
        rel = normalizePath(rel);
      } catch (_) {
        continue;
      }
      if (prefix.isEmpty || rel == prefix || rel.startsWith('$prefix/')) {
        out.add(rel);
      }
    }
    return out;
  }

  @override
  Future<String?> sha256Hex(String relativePath) async {
    final file = _file(relativePath);
    if (!await file.exists()) {
      return null;
    }
    final sink = _DigestSink();
    final input = crypto.sha256.startChunkedConversion(sink);
    await for (final chunk in file.openRead()) {
      input.add(chunk);
    }
    input.close();
    return sink.digest!.toString();
  }
}

/// In-memory [FileStore] for tests (still NFC-normalizes keys).
class MemoryFileStore implements FileStore {
  MemoryFileStore({Map<String, List<int>>? files})
      : _files = {
          if (files != null)
            for (final e in files.entries) normalizePath(e.key): e.value,
        };

  final Map<String, List<int>> _files;

  @override
  bool get canWriteFiles => true;

  @override
  Future<List<int>?> read(String relativePath) async {
    final n = normalizePath(relativePath);
    final v = _files[n];
    return v == null ? null : List<int>.from(v);
  }

  @override
  Future<void> write(String relativePath, List<int> bytes) async {
    _files[normalizePath(relativePath)] = List<int>.from(bytes);
  }

  @override
  Future<bool> exists(String relativePath) async =>
      _files.containsKey(normalizePath(relativePath));

  @override
  Future<List<String>> list({String prefix = ''}) async {
    final keys = _files.keys.toList()..sort();
    if (prefix.isEmpty) {
      return keys;
    }
    final n = normalizePath(prefix);
    return keys.where((k) => k == n || k.startsWith('$n/')).toList();
  }

  @override
  Future<String?> sha256Hex(String relativePath) async {
    final bytes = await read(relativePath);
    if (bytes == null) {
      return null;
    }
    return crypto.sha256.convert(bytes).toString();
  }
}

class _DigestSink implements Sink<crypto.Digest> {
  crypto.Digest? digest;

  @override
  void add(crypto.Digest data) => digest = data;

  @override
  void close() {}
}

/// Join [root] with an NFC relative path using the host separator.
String joinRoot(String root, String relativePath) {
  final norm = normalizePath(relativePath);
  var path = root;
  for (final seg in norm.split('/')) {
    path = '$path${Platform.pathSeparator}$seg';
  }
  return path;
}
