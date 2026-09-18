import { assertDeltaMagic } from "./delta.js";
import type { Patcher } from "./adapters.js";

/** Reject unknown magic and refuse to cross-decode. */
export function guardedPatcher(inner: Patcher): Patcher {
  return {
    supportedAlgos: () => inner.supportedAlgos(),
    async apply(algo, oldBytes, delta) {
      assertDeltaMagic(algo, delta);
      return inner.apply(algo, oldBytes, delta);
    },
  };
}
