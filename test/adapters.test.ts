import assert from "node:assert/strict";
import { mkdtemp, readFile, rm } from "node:fs/promises";
import os from "node:os";
import path from "node:path";
import { describe, it } from "node:test";
import { NodeFileStore, NodeHasher } from "../src/adapters.js";
import { deriveCapabilities } from "../src/capabilities.js";
import { Client } from "../src/client.js";
import { assertDeltaMagic, inspectDeltaMagic } from "../src/delta.js";
import { DeltaMagicError } from "../src/errors.js";
import { guardedPatcher } from "../src/patcher.js";
import { YauzlUnpacker } from "../src/unpacker.js";
import { jsonResponse, MockTransport, pathnameOf } from "./helpers.js";
import { makeStoredZip } from "./zip-fixture.js";

describe("capabilities D13", () => {
  it("default Client check is full_package only", async () => {
    const transport = new MockTransport(() => jsonResponse(204, ""));
    const client = new Client({ baseUrl: "http://example.invalid", projectRef: "p", transport });
    await client.check({ current_version: "1.0.0", os: "windows", arch: "x86_64" });
    const body = JSON.parse(new TextDecoder().decode(transport.requests[0]!.body!)) as {
      capabilities: string[];
      accepted_delta_algos?: string[];
    };
    assert.deepEqual(body.capabilities, ["full_package"]);
    assert.equal(body.accepted_delta_algos, undefined);

    await client.check({
      current_version: "1.0.0",
      os: "windows",
      arch: "x86_64",
      capabilities: [],
    });
    const empty = JSON.parse(new TextDecoder().decode(transport.requests[1]!.body!)) as {
      capabilities: string[];
    };
    assert.deepEqual(empty.capabilities, ["full_package"]);
  });

  it("yauzl unpacker advertises patch_package; no Patcher means no binary_delta", () => {
    const caps = deriveCapabilities({
      unpacker: new YauzlUnpacker(),
      fileStore: new NodeFileStore(),
      patcher: null,
    });
    assert.ok(caps.capabilities.includes("full_package"));
    assert.ok(caps.capabilities.includes("patch_package"));
    assert.ok(caps.capabilities.includes("file_list"));
    assert.equal(caps.capabilities.includes("binary_delta"), false);
    assert.equal(caps.accepted_delta_algos, undefined);
  });

  it("injected Patcher advertises binary_delta and algos", () => {
    const caps = deriveCapabilities({
      patcher: {
        supportedAlgos: () => ["bsdiff", "xdelta3"],
        apply: async () => new Uint8Array(),
      },
    });
    assert.ok(caps.capabilities.includes("binary_delta"));
    assert.deepEqual(caps.accepted_delta_algos, ["bsdiff", "xdelta3"]);
  });
});

describe("pack poll", () => {
  it("repeats the identical JSON until ready", async () => {
    let n = 0;
    const transport = new MockTransport(() => {
      n += 1;
      if (n === 1) return jsonResponse(202, { status: "pending" });
      return jsonResponse(200, { status: "ready", package_url: "/p", sha256: "aa" });
    });
    const client = new Client({ baseUrl: "http://example.invalid", projectRef: "p", transport });
    const body = {
      source_version: "1.0.0",
      target_version: "1.1.0",
      os: "windows",
      arch: "x86_64",
      needed_paths: ["b.bin", "a.bin"],
    };
    const out = await client.packUntilReady(body, { sleep: async () => undefined, deadlineMs: 10_000 });
    assert.equal(out.status, "ready");
    assert.equal(transport.requests.length, 2);
    assert.equal(pathnameOf(transport.requests[0]!), "/api/v1/projects/p/update/pack");
    assert.equal(transport.requests[0]!.method, "POST");
    const a = new TextDecoder().decode(transport.requests[0]!.body!);
    const b = new TextDecoder().decode(transport.requests[1]!.body!);
    assert.equal(a, b);
  });
});

describe("delta magic", () => {
  it("does not cross-decode", () => {
    assert.equal(inspectDeltaMagic(Buffer.from("BSDIFF40xxxx")), "bsdiff40");
    assert.equal(inspectDeltaMagic(Buffer.from("HDIFF13&xxxx")), "hdiff13");
    assert.equal(inspectDeltaMagic(Buffer.from("KVDIFFHP1\nxxxx")), "kvdiffhp1");
    assert.equal(inspectDeltaMagic(Buffer.from([0xd6, 0xc3, 0xc4, 1])), "vcdiff");
    assert.equal(inspectDeltaMagic(Buffer.from("nope")), "unknown");
    assert.throws(() => assertDeltaMagic("bsdiff", Buffer.from("HDIFF13&")), DeltaMagicError);
    assert.throws(() => assertDeltaMagic("hdiffpatch", Buffer.from("BSDIFF40")), DeltaMagicError);
  });

  it("guarded Patcher rejects unknown magic", async () => {
    const inner = guardedPatcher({
      supportedAlgos: () => ["bsdiff"],
      apply: async () => new Uint8Array([1]),
    });
    await assert.rejects(async () => {
      await inner.apply("bsdiff", new Uint8Array(), new Uint8Array([0, 1, 2]));
    }, DeltaMagicError);
  });
});

describe("hasher and unpacker", () => {
  it("NodeHasher sha256", () => {
    const h = new NodeHasher();
    assert.equal(
      h.sha256(Buffer.from("abc")),
      "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad",
    );
  });

  it("yauzl unpacks hash-named members to files[].path", async () => {
    const payload = Buffer.from("hello zip");
    const sha = new NodeHasher().sha256(payload);
    const zip = makeStoredZip([{ name: sha, data: payload }]);
    const dir = await mkdtemp(path.join(os.tmpdir(), "kv-zip-"));
    try {
      await new YauzlUnpacker().unpack(zip, dir, [{ path: "nested/app.bin", sha256: sha }]);
      const got = await readFile(path.join(dir, "nested", "app.bin"));
      assert.equal(got.toString(), "hello zip");
    } finally {
      await rm(dir, { recursive: true, force: true });
    }
  });
});
