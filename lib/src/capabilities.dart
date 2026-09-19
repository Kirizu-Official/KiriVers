import 'adapters.dart';

/// Check `capabilities` / `accepted_delta_algos` follow attached adapters (D13).
///
/// `Client.check` defaults to `full_package` only. Extra bits are sent when a
/// live `ArchiveUnpacker` / writable `FileStore` / `Patcher` is attached
/// (typically via `Updater`). Never advertise `binary_delta` without a Patcher.
const capabilityFullPackage = 'full_package';
const capabilityPatchPackage = 'patch_package';
const capabilityFileList = 'file_list';
const capabilityBinaryDelta = 'binary_delta';

const deltaAlgoHdiffpatch = 'hdiffpatch';
const deltaAlgoBsdiff = 'bsdiff';
const deltaAlgoXdelta3 = 'xdelta3';

const telemetryDownloading = 'downloading';
const telemetryApplying = 'applying';
const telemetryInstalled = 'installed';
const telemetryFailed = 'failed';
const telemetryRolledBack = 'rolled_back';

/// D13: never advertise a capability the live adapters cannot perform.
List<String> deriveCapabilities({
  ArchiveUnpacker? unpacker,
  FileStore? fileStore,
  Patcher? patcher,
}) {
  final caps = <String>[capabilityFullPackage];
  if (unpacker != null) {
    caps.add(capabilityPatchPackage);
  }
  if (fileStore != null && fileStore.canWriteFiles) {
    caps.add(capabilityFileList);
  }
  if (deriveDeltaAlgos(patcher).isNotEmpty) {
    caps.add(capabilityBinaryDelta);
  }
  return caps;
}

List<String> deriveDeltaAlgos(Patcher? patcher) {
  if (patcher == null) {
    return const [];
  }
  return [
    for (final algo in patcher.supportedAlgos)
      if (algo.trim().isNotEmpty) algo.trim(),
  ];
}
