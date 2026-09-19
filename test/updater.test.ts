import assert from "node:assert/strict";
import { mkdtemp, readFile, rm, writeFile } from "node:fs/promises";
import os from "node:os";
import path from "node:path";
import { describe, it } from "node:test";
import { NodeHasher } from "../src/adapters.js";
import { Client } from "../src/client.js";
import { HashMismatchError } from "../src/errors.js";
import { Updater } from "../src/updater.js";
import { bytesResponse, jsonResponse, MockTransport, pathnameOf } from "./helpers.js";

const check200 = {
  has_update: true,
  is_mandatory: false,
  is_downgrade: false,
  reason: "normal",
  compare_engine: "semver" as const,
  version_integer: null,
  version_semver: "1.1.0",
  target_channel: "stable",
  target_hw_rev: null,
  package_type: "single_file" as const,
  root_hash: "",
  package_url: "/api/v1/projects/p/packages/aa",
  file_name: "app.bin",
  size: 4,
  sha256: new NodeHasher().sha256(Buffer.from("full")),
  delta_available: false,
};

describe("Updater", () => {
  it("downloads and verifies full package without Patcher; telemetry errors do not fail", async () => {
    const payload = Buffer.from("full");
    const transport = new MockTransport((req) => {
      const p = pathnameOf(req);
      if (p.endsWith("/update/check")) return jsonResponse(200, check200);
      if (p.includes("/packages/")) return bytesResponse(200, payload);
      if (p.endsWith("/telemetry/report")) {
        return jsonResponse(500, { error: { code: "INTERNAL_ERROR", message: "nope" } });
      }
      return jsonResponse(404, { error: { code: "NOT_FOUND", message: "no" } });
    });
    const dir = await mkdtemp(path.join(os.tmpdir(), "kv-up-"));
    try {
      const client = new Client({ baseUrl: "http://example.invalid", projectRef: "p", transport });
      const updater = new Updater({
        client,
        os: "windows",
        arch: "x86_64",
        currentVersion: "1.0.0",
        channel: "stable",
        stageDir: dir,
        unpacker: null,
        fileStore: null,
        replacer: null,
      });
      const caps = updater.capabilities();
      assert.deepEqual(caps.capabilities, ["full_package"]);
      assert.equal(caps.accepted_delta_algos, undefined);
      const out = await updater.run();
      assert.equal(out.kind, "updated");
      if (out.kind === "updated") {
        assert.equal(out.applied, false);
        assert.equal(out.mode, "full_package");
        const got = await readFile(out.stagedPath);
        assert.equal(got.toString(), "full");
      }
      const checkBody = JSON.parse(
        new TextDecoder().decode(transport.requests.find((r) => pathnameOf(r).endsWith("/update/check"))!.body!),
      ) as { capabilities: string[]; accepted_delta_algos?: string[] };
      assert.deepEqual(checkBody.capabilities, ["full_package"]);
      assert.equal(checkBody.accepted_delta_algos, undefined);
      const downloading = JSON.parse(
        new TextDecoder().decode(
          transport.requests.find((r) => pathnameOf(r).endsWith("/telemetry/report"))!.body!,
        ),
      ) as { status: string; diff_mode?: string };
      assert.equal(downloading.status, "downloading");
      assert.equal(downloading.diff_mode, undefined);
    } finally {
      await rm(dir, { recursive: true, force: true });
    }
  });

  it("falls back to full package when Patcher magic is unknown", async () => {
    const full = Buffer.from("FULLPKG");
    const sha = new NodeHasher().sha256(full);
    const body = { ...check200, sha256: sha, delta_available: true, size: full.length };
    const transport = new MockTransport((req) => {
      const p = pathnameOf(req);
      if (p.endsWith("/update/check")) {
        return jsonResponse(200, body);
      }
      if (p.endsWith("/update/diff")) {
        return jsonResponse(200, {
          diff_mode: "binary_delta",
          root_hash: "",
          version_integer: null,
          version_semver: "1.1.0",
          channel: "stable",
          compare_engine: "semver",
          package_url: "/api/v1/projects/p/packages/delta",
          sha256: new NodeHasher().sha256(Buffer.from("not-a-delta")),
          delta_algo: "bsdiff",
        });
      }
      if (p.endsWith("/delta") || req.url.endsWith("/delta")) {
        return bytesResponse(200, Buffer.from("not-a-delta"));
      }
      if (p.includes("/packages/")) return bytesResponse(200, full);
      if (p.endsWith("/telemetry/report")) return jsonResponse(202, { status: "ok" });
      return jsonResponse(404, { error: { code: "NOT_FOUND", message: "no" } });
    });
    const dir = await mkdtemp(path.join(os.tmpdir(), "kv-up-"));
    try {
      await writeFile(path.join(dir, "old.bin"), Buffer.from("OLD"));
      const client = new Client({ baseUrl: "http://example.invalid", projectRef: "p", transport });
      const updater = new Updater({
        client,
        os: "windows",
        arch: "x86_64",
        currentVersion: "1.0.0",
        stageDir: path.join(dir, "stage"),
        localFile: path.join(dir, "old.bin"),
        unpacker: null,
        replacer: null,
        patcher: {
          supportedAlgos: () => ["bsdiff"],
          apply: async () => {
            throw new Error("should not apply unknown magic");
          },
        },
      });
      const out = await updater.run();
      assert.equal(out.kind, "updated");
      if (out.kind === "updated") assert.equal(out.mode, "full_package");
    } finally {
      await rm(dir, { recursive: true, force: true });
    }
  });

  it("verifies check signature over check package_url, not the delta URL", async () => {
    const full = Buffer.from("FULLPKG");
    const sha = new NodeHasher().sha256(full);
    const deltaBytes = Buffer.from("BSDIFF40" + "xxxx");
    const body = {
      ...check200,
      sha256: sha,
      size: full.length,
      signature: "dGVzdA==",
      delta_available: true,
      package_url: "/api/v1/projects/p/packages/full",
    };
    let verifiedPayload = "";
    const transport = new MockTransport((req) => {
      const p = pathnameOf(req);
      if (p.endsWith("/update/check")) return jsonResponse(200, body);
      if (p.endsWith("/update/diff")) {
        return jsonResponse(200, {
          diff_mode: "binary_delta",
          root_hash: "",
          version_integer: null,
          version_semver: "1.1.0",
          channel: "stable",
          compare_engine: "semver",
          package_url: "/api/v1/projects/p/packages/delta",
          sha256: new NodeHasher().sha256(deltaBytes),
          delta_algo: "bsdiff",
        });
      }
      if (p.endsWith("/delta")) return bytesResponse(200, deltaBytes);
      if (p.includes("/packages/")) return bytesResponse(200, full);
      if (p.endsWith("/telemetry/report")) return jsonResponse(202, { status: "ok" });
      return jsonResponse(404, { error: { code: "NOT_FOUND", message: "no" } });
    });
    const dir = await mkdtemp(path.join(os.tmpdir(), "kv-up-"));
    try {
      await writeFile(path.join(dir, "old.bin"), Buffer.from("OLD"));
      const client = new Client({ baseUrl: "http://example.invalid", projectRef: "p", transport });
      const updater = new Updater({
        client,
        os: "windows",
        arch: "x86_64",
        currentVersion: "1.0.0",
        stageDir: path.join(dir, "stage"),
        localFile: path.join(dir, "old.bin"),
        unpacker: null,
        replacer: null,
        keys: [
          {
            algo: "ed25519",
            pem: "-----BEGIN PUBLIC KEY-----\nMCowBQYDK2VwAyEA\n-----END PUBLIC KEY-----\n",
          },
        ],
        verifier: {
          verify(_algo, _pem, payload) {
            verifiedPayload = payload;
            return true;
          },
        },
        patcher: {
          supportedAlgos: () => ["bsdiff"],
          apply: async () => full,
        },
      });
      const out = await updater.run();
      assert.equal(out.kind, "updated");
      if (out.kind === "updated") assert.equal(out.mode, "binary_delta");
      assert.match(verifiedPayload, /\/api\/v1\/projects\/p\/packages\/full/);
      assert.equal(verifiedPayload.includes("/packages/delta"), false);
    } finally {
      await rm(dir, { recursive: true, force: true });
    }
  });

  it("rejects sha256 mismatch", async () => {
    const transport = new MockTransport((req) => {
      const p = pathnameOf(req);
      if (p.endsWith("/update/check")) return jsonResponse(200, check200);
      if (p.includes("/packages/")) return bytesResponse(200, Buffer.from("nope"));
      if (p.endsWith("/telemetry/report")) return jsonResponse(202, {});
      return jsonResponse(404, { error: { code: "NOT_FOUND", message: "no" } });
    });
    const dir = await mkdtemp(path.join(os.tmpdir(), "kv-up-"));
    try {
      const client = new Client({ baseUrl: "http://example.invalid", projectRef: "p", transport });
      const updater = new Updater({
        client,
        os: "windows",
        arch: "x86_64",
        currentVersion: "1.0.0",
        stageDir: dir,
        unpacker: null,
        fileStore: null,
        replacer: null,
      });
      await assert.rejects(() => updater.run(), HashMismatchError);
    } finally {
      await rm(dir, { recursive: true, force: true });
    }
  });

  it("default Updater advertises patch_package (yauzl) and not binary_delta", () => {
    const transport = new MockTransport(() => jsonResponse(204, ""));
    const client = new Client({ baseUrl: "http://example.invalid", projectRef: "p", transport });
    const updater = new Updater({
      client,
      os: "windows",
      arch: "x86_64",
      currentVersion: "1.0.0",
      replacer: null,
    });
    const caps = updater.capabilities();
    assert.ok(caps.capabilities.includes("full_package"));
    assert.ok(caps.capabilities.includes("patch_package"));
    assert.ok(caps.capabilities.includes("file_list"));
    assert.equal(caps.capabilities.includes("binary_delta"), false);
    assert.equal(caps.accepted_delta_algos, undefined);
  });

  it("integer-engine target uses version_integer for integrity, pack, and telemetry", async () => {
    const payload = Buffer.from("full");
    const sha = new NodeHasher().sha256(payload);
    const body = {
      has_update: true,
      is_mandatory: false,
      is_downgrade: false,
      reason: "normal",
      compare_engine: "integer" as const,
      version_integer: 2,
      version_semver: null,
      target_channel: "stable",
      target_hw_rev: null,
      package_type: "multi_file" as const,
      root_hash: "",
      package_url: "/api/v1/projects/p/packages/aa",
      file_name: "app.zip",
      size: payload.length,
      sha256: sha,
      delta_available: false,
    };
    const transport = new MockTransport((req) => {
      const p = pathnameOf(req);
      if (p.endsWith("/update/check")) return jsonResponse(200, body);
      if (p.includes("/integrity")) {
        assert.ok(p.includes("/versions/2/integrity"), p);
        return jsonResponse(200, {
          version_integer: 2,
          version_semver: null,
          channel: "stable",
          package_type: "multi_file",
          root_hash: "",
          full_package_url: body.package_url,
          file_name: "app.zip",
          size: payload.length,
          sha256: sha,
          files: [
            {
              path: "app.bin",
              size: 1,
              install_policy: "OVERWRITE",
              integrity_check: true,
              sha256: "ab",
            },
          ],
        });
      }
      if (p.endsWith("/update/pack")) {
        const packBody = JSON.parse(new TextDecoder().decode(req.body!)) as { target_version: string };
        assert.equal(packBody.target_version, "2");
        return jsonResponse(200, { status: "full_package" });
      }
      if (p.includes("/packages/")) return bytesResponse(200, payload);
      if (p.endsWith("/telemetry/report")) return jsonResponse(202, { status: "ok" });
      return jsonResponse(404, { error: { code: "NOT_FOUND", message: "no" } });
    });
    const dir = await mkdtemp(path.join(os.tmpdir(), "kv-int-"));
    try {
      const client = new Client({ baseUrl: "http://example.invalid", projectRef: "p", transport });
      const updater = new Updater({
        client,
        os: "windows",
        arch: "x86_64",
        currentVersion: "1",
        installDir: path.join(dir, "install"),
        stageDir: path.join(dir, "stage"),
        unpacker: null,
        replacer: null,
      });
      const out = await updater.run();
      assert.equal(out.kind, "updated");
      const tels = transport.requests
        .filter((r) => pathnameOf(r).endsWith("/telemetry/report"))
        .map((r) => JSON.parse(new TextDecoder().decode(r.body!)) as { status: string; to_version: string });
      assert.ok(tels.some((t) => t.to_version === "2"));
    } finally {
      await rm(dir, { recursive: true, force: true });
    }
  });
});
