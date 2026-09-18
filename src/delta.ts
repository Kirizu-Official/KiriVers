import { DeltaMagicError } from "./errors.js";

export type DeltaMagic = "kvdiffhp1" | "hdiff13" | "bsdiff40" | "vcdiff" | "unknown";

const HDIFF13 = Buffer.from("HDIFF13&", "ascii");
const KVDIFF = Buffer.from("KVDIFFHP1\n", "ascii");
const BSDIFF = Buffer.from("BSDIFF40", "ascii");
const VCDIFF = Buffer.from([0xd6, 0xc3, 0xc4]);

export function inspectDeltaMagic(bytes: Uint8Array): DeltaMagic {
  if (startsWith(bytes, KVDIFF)) return "kvdiffhp1";
  if (startsWith(bytes, HDIFF13)) return "hdiff13";
  if (startsWith(bytes, BSDIFF)) return "bsdiff40";
  if (startsWith(bytes, VCDIFF)) return "vcdiff";
  return "unknown";
}

export function algoMatchesMagic(algo: string, magic: DeltaMagic): boolean {
  const a = algo.toLowerCase();
  if (a === "hdiffpatch") return magic === "kvdiffhp1" || magic === "hdiff13";
  if (a === "bsdiff") return magic === "bsdiff40";
  if (a === "xdelta3") return magic === "vcdiff";
  return false;
}

export function assertDeltaMagic(algo: string, delta: Uint8Array): DeltaMagic {
  const magic = inspectDeltaMagic(delta);
  if (magic === "unknown") {
    throw new DeltaMagicError("unknown delta container magic");
  }
  if (!algoMatchesMagic(algo, magic)) {
    throw new DeltaMagicError(`delta magic ${magic} does not match algo ${algo}`);
  }
  return magic;
}

function startsWith(bytes: Uint8Array, prefix: Uint8Array): boolean {
  if (bytes.length < prefix.length) return false;
  for (let i = 0; i < prefix.length; i++) {
    if (bytes[i] !== prefix[i]) return false;
  }
  return true;
}
