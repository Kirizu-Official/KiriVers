import { createHash, createPublicKey, verify } from "node:crypto";
import { createReadStream } from "node:fs";
import { mkdir, readdir, readFile, rename, stat, writeFile } from "node:fs/promises";
import path from "node:path";
import { Writable } from "node:stream";
import { pipeline } from "node:stream/promises";
import { PathError, ReplaceBusyError } from "./errors.js";
import { normalizeRelPath } from "./pathutil.js";
import type { UnpackFile } from "./types.js";

export interface FileStore {
  read(filePath: string): Promise<Uint8Array>;
  write(filePath: string, data: Uint8Array): Promise<void>;
  exists(filePath: string): Promise<boolean>;
  list(dir: string): Promise<string[]>;
  mkdirp(dir: string): Promise<void>;
  hashFile(filePath: string, algo: "sha256" | "md5"): Promise<string>;
  canWriteFiles(): boolean;
  normalizePath(rel: string): string;
}

export class NodeFileStore implements FileStore {
  canWriteFiles(): boolean {
    return true;
  }

  normalizePath(rel: string): string {
    return normalizeRelPath(rel);
  }

  async read(filePath: string): Promise<Uint8Array> {
    return new Uint8Array(await readFile(filePath));
  }

  async write(filePath: string, data: Uint8Array): Promise<void> {
    await mkdir(path.dirname(filePath), { recursive: true });
    await writeFile(filePath, data);
  }

  async exists(filePath: string): Promise<boolean> {
    try {
      await stat(filePath);
      return true;
    } catch (err) {
      if ((err as NodeJS.ErrnoException).code === "ENOENT") return false;
      throw err;
    }
  }

  async list(dir: string): Promise<string[]> {
    const out: string[] = [];
    await walk(dir, dir, out);
    return out;
  }

  async mkdirp(dir: string): Promise<void> {
    await mkdir(dir, { recursive: true });
  }

  async hashFile(filePath: string, algo: "sha256" | "md5"): Promise<string> {
    const hash = createHash(algo);
    await pipeline(
      createReadStream(filePath),
      new Writable({
        write(chunk, _enc, cb) {
          hash.update(chunk as Buffer);
          cb();
        },
      }),
    );
    return hash.digest("hex");
  }
}

async function walk(root: string, current: string, out: string[]): Promise<void> {
  let entries;
  try {
    entries = await readdir(current, { withFileTypes: true });
  } catch (err) {
    if ((err as NodeJS.ErrnoException).code === "ENOENT") return;
    throw err;
  }
  for (const entry of entries) {
    const full = path.join(current, entry.name);
    if (entry.isDirectory()) {
      await walk(root, full, out);
    } else if (entry.isFile()) {
      out.push(normalizeRelPath(path.relative(root, full)));
    }
  }
}

export interface Hasher {
  sha256(data: Uint8Array): string;
  md5(data: Uint8Array): string;
}

export class NodeHasher implements Hasher {
  sha256(data: Uint8Array): string {
    return createHash("sha256").update(data).digest("hex");
  }

  md5(data: Uint8Array): string {
    return createHash("md5").update(data).digest("hex");
  }
}

export interface SignatureVerifier {
  verify(
    algo: "ed25519" | "rsa-sha256",
    publicKeyPem: string,
    payload: string,
    sigBase64: string,
  ): boolean;
}

export class NodeSignatureVerifier implements SignatureVerifier {
  verify(
    algo: "ed25519" | "rsa-sha256",
    publicKeyPem: string,
    payload: string,
    sigBase64: string,
  ): boolean {
    const key = createPublicKey(publicKeyPem);
    const sig = Buffer.from(sigBase64, "base64");
    const data = Buffer.from(payload, "utf8");
    if (algo === "ed25519") {
      return verify(null, data, key, sig);
    }
    return verify("sha256", data, key, sig);
  }
}

export interface ArchiveUnpacker {
  unpack(archive: Uint8Array, destDir: string, files: UnpackFile[]): Promise<void>;
}

export interface Patcher {
  supportedAlgos(): string[];
  apply(algo: string, oldBytes: Uint8Array, delta: Uint8Array): Promise<Uint8Array> | Uint8Array;
}

export interface Replacer {
  replace(stagedPath: string, targetPath: string): Promise<void>;
}

/**
 * Default Node/Electron app-level replacer (`fs.rename`).
 * Occupied Windows files need a caller Replacer. This is not a one-click GUI installer.
 */
export class FsRenameReplacer implements Replacer {
  async replace(stagedPath: string, targetPath: string): Promise<void> {
    await mkdir(path.dirname(targetPath), { recursive: true });
    try {
      await rename(stagedPath, targetPath);
    } catch (err) {
      const code = (err as NodeJS.ErrnoException).code;
      if (code === "EXDEV") {
        await writeFile(targetPath, await readFile(stagedPath));
        return;
      }
      if (code === "EBUSY" || code === "EPERM" || code === "EACCES") {
        throw new ReplaceBusyError(
          "target file is in use; inject a Replacer (MoveFileEx / a helper BAT you own)",
        );
      }
      throw err;
    }
  }
}

export function assertSafeRel(rel: string): string {
  const n = normalizeRelPath(rel);
  if (n === "") throw new PathError("empty path");
  return n;
}
