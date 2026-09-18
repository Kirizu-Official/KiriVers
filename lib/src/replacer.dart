import 'dart:io';

import 'adapters.dart';

/// Default [Replacer]: `File.rename` (copy+delete if rename crosses volumes).
///
/// This is app-level file replace, not APK / HarmonyOS package install.
/// Those platforms must inject a [Replacer] that starts the system installer.
class FileRenameReplacer implements Replacer {
  const FileRenameReplacer();

  @override
  Future<void> replace({
    required String stagedPath,
    required String installPath,
  }) async {
    final staged = File(stagedPath);
    if (!await staged.exists()) {
      final dir = Directory(stagedPath);
      if (await dir.exists()) {
        await _replaceDir(dir, Directory(installPath));
        return;
      }
      throw FileSystemException('staged path does not exist', stagedPath);
    }
    await _replaceFile(staged, File(installPath));
  }

  Future<void> _replaceFile(File staged, File target) async {
    await target.parent.create(recursive: true);
    if (await target.exists()) {
      await target.delete();
    }
    try {
      await staged.rename(target.path);
    } on FileSystemException {
      await staged.copy(target.path);
      await staged.delete();
    }
  }

  Future<void> _replaceDir(Directory staged, Directory target) async {
    if (await target.exists()) {
      await target.delete(recursive: true);
    }
    await target.parent.create(recursive: true);
    try {
      await staged.rename(target.path);
    } on FileSystemException {
      await _copyDir(staged, target);
      await staged.delete(recursive: true);
    }
  }

  Future<void> _copyDir(Directory from, Directory to) async {
    await to.create(recursive: true);
    await for (final entity in from.list(recursive: true, followLinks: false)) {
      final rel = entity.path.substring(from.path.length);
      final dest = '${to.path}$rel';
      if (entity is Directory) {
        await Directory(dest).create(recursive: true);
      } else if (entity is File) {
        await File(dest).parent.create(recursive: true);
        await entity.copy(dest);
      }
    }
  }
}
