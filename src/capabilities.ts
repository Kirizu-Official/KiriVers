import type { ArchiveUnpacker, FileStore, Patcher } from "./adapters.js";
import {
  CAPABILITY_BINARY_DELTA,
  CAPABILITY_FILE_LIST,
  CAPABILITY_FULL_PACKAGE,
  CAPABILITY_PATCH_PACKAGE,
} from "./types.js";

export interface CapabilityAdapters {
  unpacker?: ArchiveUnpacker | null;
  fileStore?: FileStore | null;
  patcher?: Patcher | null;
}

/**
 * D13: never advertise a capability the live adapters cannot perform.
 * Default check (Client without extras) stays `full_package` only.
 */
export function deriveCapabilities(adapters: CapabilityAdapters): {
  capabilities: string[];
  accepted_delta_algos?: string[];
} {
  const capabilities = [CAPABILITY_FULL_PACKAGE];
  if (adapters.unpacker) {
    capabilities.push(CAPABILITY_PATCH_PACKAGE);
  }
  if (adapters.fileStore?.canWriteFiles()) {
    capabilities.push(CAPABILITY_FILE_LIST);
  }
  const algos = (adapters.patcher?.supportedAlgos() ?? []).filter((a) => a.length > 0);
  if (algos.length > 0) {
    capabilities.push(CAPABILITY_BINARY_DELTA);
    return { capabilities, accepted_delta_algos: algos };
  }
  return { capabilities };
}
