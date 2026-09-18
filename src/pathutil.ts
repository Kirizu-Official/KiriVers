import { PathError } from "./errors.js";

const DRIVE_LETTER = /^[a-zA-Z]:/;

/**
 * Align relative paths with server `pkg/pathutil`: `/` separators, Unicode NFC,
 * reject `..` / `.` segments, leading slashes, drive letters, and control chars.
 */
export function normalizeRelPath(input: string): string {
  const trimmed = input.trim();
  if (trimmed === "") {
    throw new PathError("path is empty");
  }
  for (let i = 0; i < trimmed.length; i++) {
    const code = trimmed.charCodeAt(i);
    if (code < 0x20) {
      throw new PathError(`path contains control character 0x${code.toString(16)}`);
    }
  }
  if (trimmed.startsWith("/") || trimmed.startsWith("\\")) {
    throw new PathError(`path cannot start with leading slash: ${input}`);
  }
  if (DRIVE_LETTER.test(trimmed)) {
    throw new PathError(`path cannot contain drive letter: ${input}`);
  }

  const slashUnified = trimmed.replace(/\\/g, "/");
  for (const seg of slashUnified.split("/")) {
    if (seg === "") continue;
    if (DRIVE_LETTER.test(seg)) {
      throw new PathError(`segment cannot contain drive letter: ${input}`);
    }
    if (seg === "." || seg === "..") {
      throw new PathError(`path traversal segment ${JSON.stringify(seg)} is forbidden`);
    }
  }

  const cleaned = slashUnified.normalize("NFC").replace(/\/+/g, "/").replace(/^\/+|\/+$/g, "");
  if (cleaned === "") {
    throw new PathError("path normalized to empty");
  }
  for (const seg of cleaned.split("/")) {
    if (seg === "" || seg === "." || seg === "..") {
      throw new PathError(`invalid segment in normalized path ${JSON.stringify(seg)}`);
    }
  }
  return cleaned;
}

/** Unique NFC paths. Order is sorted so a pack poll body stays stable. */
export function uniqueNeededPaths(paths: string[]): string[] {
  const set = new Set<string>();
  for (const p of paths) {
    set.add(normalizeRelPath(p));
  }
  return [...set].sort();
}

export function joinPosix(...parts: string[]): string {
  return parts
    .filter((p) => p !== "")
    .map((p, i) => (i === 0 ? p.replace(/\/+$/, "") : p.replace(/^\/+|\/+$/g, "")))
    .join("/");
}
