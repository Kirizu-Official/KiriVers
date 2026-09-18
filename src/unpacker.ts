import { createWriteStream } from "node:fs";
import { mkdir } from "node:fs/promises";
import { createRequire } from "node:module";
import path from "node:path";
import { pipeline } from "node:stream/promises";
import type { Entry, ZipFile } from "yauzl";
import type { ArchiveUnpacker } from "./adapters.js";
import { PathError } from "./errors.js";
import { normalizeRelPath } from "./pathutil.js";
import type { UnpackFile } from "./types.js";

const require = createRequire(import.meta.url);
const yauzl = require("yauzl") as typeof import("yauzl");

function memberHash(fileName: string): string {
  const base = fileName.replace(/\\/g, "/").split("/").pop() ?? fileName;
  return base.replace(/\.[A-Za-z0-9]+$/, "").toLowerCase();
}

/**
 * D18 zip unpacker. Patch-package members are named by content SHA-256;
 * `files[].path` is the install path (NFC, `/`).
 */
export class YauzlUnpacker implements ArchiveUnpacker {
  unpack(archive: Uint8Array, destDir: string, files: UnpackFile[]): Promise<void> {
    const byHash = new Map<string, UnpackFile>();
    for (const f of files) {
      byHash.set(f.sha256.toLowerCase(), f);
    }
    const seen = new Set<string>();
    return new Promise((resolve, reject) => {
      yauzl.fromBuffer(Buffer.from(archive), { lazyEntries: true }, (err, zip) => {
        if (err || !zip) {
          reject(err ?? new Error("yauzl failed to open zip"));
          return;
        }
        const fail = (e: unknown) => {
          try {
            zip.close();
          } catch {
            // ignore
          }
          reject(e);
        };
        zip.on("error", fail);
        zip.on("end", () => {
          try {
            zip.close();
          } catch {
            // ignore
          }
          if (files.length > 0) {
            for (const f of files) {
              if (!seen.has(f.sha256.toLowerCase())) {
                fail(new PathError(`zip missing member for ${f.path} (${f.sha256})`));
                return;
              }
            }
          }
          resolve();
        });
        zip.on("entry", (entry: Entry) => {
          void handleEntry(zip, entry, destDir, files, byHash, seen).then(
            () => zip.readEntry(),
            fail,
          );
        });
        zip.readEntry();
      });
    });
  }
}

async function handleEntry(
  zip: ZipFile,
  entry: Entry,
  destDir: string,
  files: UnpackFile[],
  byHash: Map<string, UnpackFile>,
  seen: Set<string>,
): Promise<void> {
  if (/\/$/.test(entry.fileName)) return;
  const hash = memberHash(entry.fileName);
  const meta = byHash.get(hash) ?? byHash.get(entry.fileName.replace(/\\/g, "/").toLowerCase());
  let rel: string;
  if (meta) {
    rel = normalizeRelPath(meta.path);
    seen.add(meta.sha256.toLowerCase());
  } else if (files.length === 0) {
    rel = normalizeRelPath(entry.fileName);
  } else {
    return;
  }
  const dest = path.resolve(destDir, ...rel.split("/"));
  const root = path.resolve(destDir);
  const relToRoot = path.relative(root, dest);
  if (
    relToRoot === "" ||
    relToRoot === ".." ||
    relToRoot.startsWith(`..${path.sep}`) ||
    path.isAbsolute(relToRoot)
  ) {
    throw new PathError(`zip member escapes destination: ${rel}`);
  }
  await mkdir(path.dirname(dest), { recursive: true });
  const stream = await new Promise<NodeJS.ReadableStream>((resolve, reject) => {
    zip.openReadStream(entry, (e, s) => {
      if (e || !s) reject(e ?? new Error("yauzl openReadStream failed"));
      else resolve(s);
    });
  });
  await pipeline(stream, createWriteStream(dest));
}
