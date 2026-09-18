import path from "node:path";
import {
  type ArchiveUnpacker,
  type FileStore,
  type Hasher,
  NodeFileStore,
  NodeHasher,
  NodeSignatureVerifier,
  FsRenameReplacer,
  type Patcher,
  type Replacer,
  type SignatureVerifier,
} from "./adapters.js";
import { deriveCapabilities } from "./capabilities.js";
import type { Client, PackPollOptions } from "./client.js";
import { DeltaMagicError, HashMismatchError } from "./errors.js";
import { guardedPatcher } from "./patcher.js";
import { uniqueNeededPaths } from "./pathutil.js";
import { payloadFromCheckLike } from "./signature.js";
import type { Integrity200, ManifestFile, UpdateCheck200, VerifyKey } from "./types.js";
import { YauzlUnpacker } from "./unpacker.js";

type ApplyMode = "full_package" | "patch_package" | "binary_delta" | "file_list";

export interface UpdaterOptions {
  client: Client;
  os: string;
  arch: string;
  currentVersion: string;
  channel?: string;
  deviceId?: string;
  osVersion?: string;
  hwRev?: string;
  custom?: Record<string, unknown>;
  reportDevice?: boolean;
  installDir?: string;
  stageDir?: string;
  targetPath?: string;
  localFile?: string;
  keys?: VerifyKey[];
  fileStore?: FileStore | null;
  hasher?: Hasher;
  unpacker?: ArchiveUnpacker | null;
  patcher?: Patcher | null;
  replacer?: Replacer | null;
  verifier?: SignatureVerifier | null;
  packPoll?: PackPollOptions;
}

export type UpdateOutcome =
  | { kind: "no_update" | "not_modified"; etag?: string }
  | {
      kind: "updated";
      check: UpdateCheck200;
      stagedPath: string;
      applied: boolean;
      mode: ApplyMode;
    };

function isIntegrityBody(
  res: { status: number; body?: Integrity200 },
): res is { status: 200; body: Integrity200 } {
  return res.status === 200 && res.body !== undefined;
}

/**
 * High-level check → download → verify → optional patch/unpack → optional replace.
 * Missing Replacer returns the staged verified path. Telemetry errors never fail the update.
 */
export class Updater {
  private readonly client: Client;
  private readonly os: string;
  private readonly arch: string;
  private readonly currentVersion: string;
  private readonly channel?: string;
  private readonly deviceId?: string;
  private readonly osVersion?: string;
  private readonly hwRev?: string;
  private readonly custom?: Record<string, unknown>;
  private readonly reportDevice: boolean;
  private readonly installDir?: string;
  private readonly stageDir: string;
  private readonly targetPath?: string;
  private readonly localFile?: string;
  private readonly keys: VerifyKey[];
  private readonly fileStore: FileStore | null;
  private readonly hasher: Hasher;
  private readonly unpacker: ArchiveUnpacker | null;
  private readonly patcher: Patcher | null;
  private readonly replacer: Replacer | null;
  private readonly verifier: SignatureVerifier | null;
  private readonly packPoll?: PackPollOptions;

  constructor(opts: UpdaterOptions) {
    this.client = opts.client;
    this.os = opts.os;
    this.arch = opts.arch;
    this.currentVersion = opts.currentVersion;
    this.channel = opts.channel;
    this.deviceId = opts.deviceId;
    this.osVersion = opts.osVersion;
    this.hwRev = opts.hwRev;
    this.custom = opts.custom;
    this.reportDevice = opts.reportDevice ?? false;
    this.installDir = opts.installDir;
    this.stageDir = opts.stageDir ?? path.join(opts.installDir ?? ".", ".kirivers-stage");
    this.targetPath = opts.targetPath;
    this.localFile = opts.localFile;
    this.keys = opts.keys ?? [];
    this.fileStore = opts.fileStore === undefined ? new NodeFileStore() : opts.fileStore;
    this.hasher = opts.hasher ?? new NodeHasher();
    this.unpacker = opts.unpacker === undefined ? new YauzlUnpacker() : opts.unpacker;
    this.patcher = opts.patcher ? guardedPatcher(opts.patcher) : null;
    this.replacer = opts.replacer === undefined ? new FsRenameReplacer() : opts.replacer;
    this.verifier =
      opts.verifier === undefined
        ? this.keys.length > 0
          ? new NodeSignatureVerifier()
          : null
        : opts.verifier;
    this.packPoll = opts.packPoll;
  }

  capabilities(): { capabilities: string[]; accepted_delta_algos?: string[] } {
    return deriveCapabilities({
      unpacker: this.unpacker,
      fileStore: this.fileStore,
      patcher: this.patcher,
    });
  }

  async run(): Promise<UpdateOutcome> {
    if (this.reportDevice && this.deviceId) {
      await this.client.deviceReport({
        device_id: this.deviceId,
        version: this.currentVersion,
        os: this.os,
        arch: this.arch,
        channel: this.channel,
        custom: this.custom,
      });
    }

    const caps = this.capabilities();
    const check = await this.client.check({
      current_version: this.currentVersion,
      os: this.os,
      arch: this.arch,
      channel: this.channel,
      hw_rev: this.hwRev,
      os_version: this.osVersion,
      device_id: this.deviceId,
      capabilities: caps.capabilities,
      accepted_delta_algos: caps.accepted_delta_algos,
    });

    if (check.kind === "no_update") return { kind: "no_update", etag: check.etag };
    if (check.kind === "not_modified") return { kind: "not_modified", etag: check.etag };

    const target = check.body;
    let mode: ApplyMode = "full_package";
    let stagedPath: string | undefined;

    try {
      await this.safeTelemetry("downloading", target);
      const planned = await this.planDownload(target);
      mode = planned.mode;
      stagedPath = planned.stagedPath;
      this.verifySignature(target);
      let applied = false;
      if (this.replacer && this.targetPath) {
        await this.safeTelemetry("applying", target, mode);
        await this.replacer.replace(stagedPath, this.targetPath);
        applied = true;
      }
      await this.safeTelemetry("installed", target, mode);
      return { kind: "updated", check: target, stagedPath, applied, mode };
    } catch (err) {
      await this.safeTelemetry("failed", target, mode, err);
      throw err;
    }
  }

  private async planDownload(target: UpdateCheck200): Promise<{
    mode: ApplyMode;
    stagedPath: string;
    expectedSha256?: string;
    packageUrl?: string;
  }> {
    if (
      target.package_type === "single_file" &&
      this.patcher &&
      target.delta_available &&
      !target.is_downgrade
    ) {
      try {
        return await this.downloadDelta(target);
      } catch {
        // unknown magic, missing local file, or patch failure → full package
      }
    }

    if (target.package_type === "multi_file" && this.fileStore && this.installDir) {
      try {
        return await this.downloadPack(target);
      } catch {
        // fall through to full package
      }
    }

    return this.downloadFull(target);
  }

  private async downloadFull(target: UpdateCheck200): Promise<{
    mode: "full_package";
    stagedPath: string;
    expectedSha256: string;
    packageUrl: string;
  }> {
    const bytes = await this.client.downloadUrl(target.package_url);
    this.assertSha(bytes.body, target.sha256);
    const stagedPath = path.join(this.stageDir, target.file_name || "package.bin");
    await this.writeStaged(stagedPath, bytes.body);
    return {
      mode: "full_package",
      stagedPath,
      expectedSha256: target.sha256,
      packageUrl: target.package_url,
    };
  }

  private async downloadDelta(target: UpdateCheck200): Promise<{
    mode: "binary_delta";
    stagedPath: string;
    expectedSha256: string;
    packageUrl: string;
  }> {
    if (!this.patcher) throw new Error("no patcher");
    const localPath = this.localFile;
    if (!localPath || !this.fileStore) throw new Error("localFile required for binary_delta");
    const localBytes = await this.fileStore.read(localPath);
    const localSha = this.hasher.sha256(localBytes);
    const caps = this.capabilities();
    const diff = await this.client.diff({
      source_version: this.currentVersion,
      target_version: target.version_semver ?? this.currentVersion,
      os: this.os,
      arch: this.arch,
      channel: this.channel ?? target.target_channel,
      device_id: this.deviceId,
      hw_rev: this.hwRev,
      local_sha256: localSha,
      capabilities: caps.capabilities,
      accepted_delta_algos: caps.accepted_delta_algos,
    });
    if (diff.diff_mode !== "binary_delta" || !diff.package_url || !diff.delta_algo) {
      throw new Error("diff did not return binary_delta");
    }
    const delta = await this.client.downloadUrl(diff.package_url);
    if (diff.sha256) this.assertSha(delta.body, diff.sha256);
    let patched: Uint8Array;
    try {
      patched = await this.patcher.apply(diff.delta_algo, localBytes, delta.body);
    } catch (err) {
      if (err instanceof DeltaMagicError) throw err;
      throw err;
    }
    this.assertSha(patched, target.sha256);
    const stagedPath = path.join(this.stageDir, target.file_name || "patched.bin");
    await this.writeStaged(stagedPath, patched);
    return {
      mode: "binary_delta",
      stagedPath,
      expectedSha256: target.sha256,
      packageUrl: diff.package_url,
    };
  }

  private async downloadPack(target: UpdateCheck200): Promise<{
    mode: ApplyMode;
    stagedPath: string;
    expectedSha256?: string;
    packageUrl?: string;
  }> {
    if (!this.fileStore || !this.installDir) throw new Error("installDir required");
    const integ = await this.client.integrity(target.version_semver ?? this.currentVersion, {
      os: this.os,
      arch: this.arch,
      channel: this.channel ?? target.target_channel,
      hw_rev: this.hwRev,
    });
    if (!isIntegrityBody(integ)) throw new Error("integrity not modified / empty");
    const needed = await this.neededPaths(integ.body.files);
    const pack = await this.client.packUntilReady(
      {
        source_version: this.currentVersion,
        target_version: target.version_semver ?? this.currentVersion,
        os: this.os,
        arch: this.arch,
        channel: this.channel ?? target.target_channel,
        device_id: this.deviceId,
        hw_rev: this.hwRev,
        needed_paths: needed,
      },
      this.packPoll,
    );
    if (pack.status === "full_package") {
      return this.downloadFull(target);
    }
    if (pack.status !== "ready" || !pack.package_url) {
      throw new Error(`unexpected pack status ${pack.status}`);
    }
    const bytes = await this.client.downloadUrl(pack.package_url);
    if (pack.sha256) this.assertSha(bytes.body, pack.sha256);
    const stagedPath = path.join(this.stageDir, pack.file_name || "patch.zip");
    await this.writeStaged(stagedPath, bytes.body);
    if (this.unpacker && pack.files) {
      const files = pack.files
        .filter((f): f is { path: string; sha256: string } => !!f.path && !!f.sha256)
        .map((f) => ({ path: f.path, sha256: f.sha256 }));
      await this.unpacker.unpack(bytes.body, this.installDir, files);
    }
    return {
      mode: pack.diff_mode === "patch_package" ? "patch_package" : "full_package",
      stagedPath,
      expectedSha256: pack.sha256,
      packageUrl: pack.package_url,
    };
  }

  private async neededPaths(files: ManifestFile[]): Promise<string[]> {
    const store = this.fileStore!;
    const needed: string[] = [];
    for (const file of files) {
      const rel = store.normalizePath(file.path);
      const full = path.join(this.installDir!, ...rel.split("/"));
      const exists = await store.exists(full);
      if (file.install_policy === "KEEP_IF_EXISTS" && exists) {
        continue;
      }
      if (exists && file.integrity_check && file.sha256) {
        const local = await store.hashFile(full, "sha256");
        if (local.toLowerCase() === file.sha256.toLowerCase()) continue;
      }
      needed.push(rel);
    }
    return uniqueNeededPaths(needed);
  }

  private async writeStaged(stagedPath: string, data: Uint8Array): Promise<void> {
    if (this.fileStore) {
      await this.fileStore.write(stagedPath, data);
      return;
    }
    const fs = new NodeFileStore();
    await fs.write(stagedPath, data);
  }

  private assertSha(data: Uint8Array, expected: string): void {
    const actual = this.hasher.sha256(data);
    if (actual.toLowerCase() !== expected.toLowerCase()) {
      throw new HashMismatchError(expected.toLowerCase(), actual.toLowerCase());
    }
  }

  private verifySignature(target: UpdateCheck200): void {
    if (!target.signature || !this.verifier || this.keys.length === 0) return;
    const payload = payloadFromCheckLike({
      version_integer: target.version_integer,
      version_semver: target.version_semver,
      root_hash: target.root_hash,
      package_url: target.package_url,
      size: target.size,
      sha256: target.sha256,
    });
    const ok = this.keys.some((k) => this.verifier!.verify(k.algo, k.pem, payload, target.signature!));
    if (!ok) throw new Error("response signature mismatch");
  }

  private async safeTelemetry(
    status: "downloading" | "applying" | "installed" | "failed",
    target: UpdateCheck200,
    mode?: ApplyMode,
    err?: unknown,
  ): Promise<void> {
    try {
      await this.client.reportTelemetry({
        os: this.os,
        arch: this.arch,
        channel: this.channel ?? target.target_channel,
        from_version: this.currentVersion,
        to_version: target.version_semver ?? "",
        status,
        device_id: this.deviceId,
        diff_mode: mode,
        error_code: err && status === "failed" ? (err as { name?: string }).name : undefined,
        error_message: err && status === "failed" ? String((err as Error).message ?? err) : undefined,
      });
    } catch {
      // telemetry must never block apply
    }
  }
}
