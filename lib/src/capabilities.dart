/// Check `capabilities` / `accepted_delta_algos` follow attached adapters (D13).
///
/// Default is `full_package` plus whatever default adapters justify.
/// Never advertise `binary_delta` without a live Patcher.
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
