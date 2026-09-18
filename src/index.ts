export { OPENAPI_REVISION } from "./types.js";
export {
  CAPABILITY_BINARY_DELTA,
  CAPABILITY_FILE_LIST,
  CAPABILITY_FULL_PACKAGE,
  CAPABILITY_PATCH_PACKAGE,
} from "./types.js";
export type * from "./types.js";
export { ApiError, DeltaMagicError, HashMismatchError, PathError, ReplaceBusyError } from "./errors.js";
export { PATHS, LEFTOVER_PATHS, expandPath } from "./paths.js";
export { normalizeRelPath, uniqueNeededPaths } from "./pathutil.js";
export { buildCheckPayload, payloadFromCheckLike } from "./signature.js";
export { inspectDeltaMagic, algoMatchesMagic, assertDeltaMagic } from "./delta.js";
export { deriveCapabilities } from "./capabilities.js";
export { FetchTransport } from "./transport.js";
export type { Transport, TransportRequest, TransportResponse } from "./transport.js";
export {
  NodeFileStore,
  NodeHasher,
  NodeSignatureVerifier,
  FsRenameReplacer,
} from "./adapters.js";
export type {
  FileStore,
  Hasher,
  SignatureVerifier,
  ArchiveUnpacker,
  Patcher,
  Replacer,
} from "./adapters.js";
export { YauzlUnpacker } from "./unpacker.js";
export { guardedPatcher } from "./patcher.js";
export { Client } from "./client.js";
export type { ClientOptions, PackPollOptions } from "./client.js";
export { Updater } from "./updater.js";
export type { UpdaterOptions, UpdateOutcome } from "./updater.js";
