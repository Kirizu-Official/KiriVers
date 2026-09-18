import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { createHash } from "node:crypto";
import path from "node:path";
import { describe, it } from "node:test";
import { fileURLToPath } from "node:url";
import { Client } from "../src/client.js";
import { ApiError } from "../src/errors.js";
import { LEFTOVER_PATHS } from "../src/paths.js";
import { bytesResponse, jsonResponse, MockTransport, pathnameOf } from "./helpers.js";

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "../..");
const spec = JSON.parse(readFileSync(path.join(root, "openapi.client.json"), "utf8")) as {
  info: { version: string };
  paths: Record<string, Record<string, unknown>>;
};

const SKIP_PATH_SUBSTR = ["/store/", "/openapi.json"];

function nativeOps(): Array<{ method: string; path: string }> {
  const out: Array<{ method: string; path: string }> = [];
  for (const [p, item] of Object.entries(spec.paths)) {
    if (SKIP_PATH_SUBSTR.some((s) => p.includes(s))) continue;
    for (const method of Object.keys(item)) {
      if (method === "parameters" || method === "options") continue;
      out.push({ method: method.toUpperCase(), path: p });
    }
  }
  return out;
}

function canned(req: { method: string; url: string }) {
  const u = new URL(req.url);
  const p = u.pathname;
  if (req.method === "POST" && p.endsWith("/update/check")) {
    return jsonResponse(204, "", { etag: '"abc"' });
  }
  if (req.method === "POST" && p.endsWith("/update/pack")) {
    return jsonResponse(200, { status: "full_package" });
  }
  if (req.method === "POST" && p.endsWith("/telemetry/report")) {
    return jsonResponse(202, { status: "accepted" });
  }
  if ((req.method === "GET" || req.method === "HEAD") && p.includes("/packages/")) {
    return bytesResponse(200, new Uint8Array([0x01, 0x02]));
  }
  if ((req.method === "GET" || req.method === "HEAD") && p.includes("/media/")) {
    return bytesResponse(200, new Uint8Array([0x03]));
  }
  if (p.endsWith("/health")) {
    return jsonResponse(200, { status: "ok", ready: true });
  }
  return jsonResponse(200, {
    channels: [],
    matrix: [],
    languages: [],
    announcements: [],
    files: [],
    version_integer: null,
    version_semver: "1.0.0",
    channel: "stable",
    package_type: "single_file",
    root_hash: "",
    full_package_url: "/pkg",
    file_name: "a.bin",
    size: 1,
    sha256: "ab",
    diff_mode: "full_package",
    compare_engine: "semver",
    ip: "127.0.0.1",
    country_code: "",
    region_code: "",
    geo_i18n: {},
  });
}

describe("OpenAPI contract (mock Transport)", () => {
  it("snapshot revision matches info.version and OPENAPI_REVISION file", async () => {
    const { OPENAPI_REVISION } = await import("../src/index.js");
    const file = readFileSync(path.join(root, "OPENAPI_REVISION"), "utf8").trim();
    assert.equal(spec.info.version, "1.0.0");
    assert.equal(file, "1.0.0 B443DEA6");
    assert.equal(OPENAPI_REVISION, file);
    const digest = createHash("sha256")
      .update(readFileSync(path.join(root, "openapi.client.json")))
      .digest("hex")
      .slice(0, 8)
      .toUpperCase();
    assert.equal(digest, "B443DEA6");
  });

  it("package.json dependencies are only yauzl and there is no browser export", () => {
    const pkg = JSON.parse(readFileSync(path.join(root, "package.json"), "utf8")) as {
      dependencies: Record<string, string>;
      exports?: unknown;
      browser?: unknown;
    };
    assert.deepEqual(Object.keys(pkg.dependencies), ["yauzl"]);
    assert.equal(pkg.browser, undefined);
    const exports = JSON.stringify(pkg.exports ?? {});
    assert.equal(exports.includes("browser"), false);
  });

  it("handwritten client emits every native path+method and required field names", async () => {
    const transport = new MockTransport((req) => canned(req));
    const client = new Client({
      baseUrl: "http://127.0.0.1:8080",
      projectRef: "demo",
      projectToken: "ptok",
      channelToken: "ctok",
      transport,
    });

    await client.health();
    await client.project();
    await client.deviceReport({ device_id: "dev-1", os: "windows", arch: "x86_64" });
    const check = await client.check({
      current_version: "1.0.0",
      os: "windows",
      arch: "x86_64",
      channel: "stable",
      device_id: "dev-1",
    });
    assert.equal(check.kind, "no_update");
    await client.changelog("stable", "windows", "x86_64", { from_version: "1.0.0" });
    await client.integrity("1.1.0", { os: "windows", arch: "x86_64" });
    await client.diff({
      source_version: "1.0.0",
      target_version: "1.1.0",
      os: "windows",
      arch: "x86_64",
      local_sha256: "abc",
    });
    await client.pack({
      source_version: "1.0.0",
      target_version: "1.1.0",
      os: "windows",
      arch: "x86_64",
      needed_paths: ["a.bin"],
    });
    await client.downloadPackage("7f063a6901ebfd0aa7b1adb8af58f4e792f13310f42213767ced623eb0669969", {
      exp: "1",
      sig: "2",
      range: "bytes=0-10",
    });
    await client.headPackage("7f063a6901ebfd0aa7b1adb8af58f4e792f13310f42213767ced623eb0669969", {
      exp: "1",
      sig: "2",
    });
    await client.reportTelemetry({
      os: "windows",
      arch: "x86_64",
      channel: "stable",
      from_version: "1.0.0",
      to_version: "1.1.0",
      status: "installed",
    });
    await client.channels();
    await client.matrix();
    await client.languages();
    await client.announcements({ version: "1.0.0", os: "windows", arch: "x86_64" });
    await client.getMedia("11111111-1111-1111-1111-111111111111");
    await client.headMedia("11111111-1111-1111-1111-111111111111");

    const seen = new Set(transport.requests.map((r) => `${r.method} ${pathnameOf(r)}`));
    for (const op of nativeOps()) {
      const expectedPath = op.path
        .replace("{project_ref}", "demo")
        .replace("{channel}", "stable")
        .replace("{os}", "windows")
        .replace("{arch}", "x86_64")
        .replace("{version}", "1.1.0")
        .replace("{ref}", "7f063a6901ebfd0aa7b1adb8af58f4e792f13310f42213767ced623eb0669969")
        .replace("{id}", "11111111-1111-1111-1111-111111111111");
      const key = `${op.method} ${expectedPath}`;
      assert.ok(seen.has(key), `missing ${key}; got ${[...seen].join(", ")}`);
    }

    for (const req of transport.requests) {
      const p = pathnameOf(req);
      assert.equal(req.method === "GET" && p.endsWith("/update/check"), false);
      assert.equal(p.endsWith("/clients/login"), false);
      assert.equal(p.includes("/store/"), false);
      assert.equal(p.includes("/pack/status"), false);
      assert.equal(p.includes("/manifest"), false);
      assert.equal(p.includes("/artifacts/"), false);
      assert.equal(p === "/api/v1/ready", false);
    }
    assert.ok(LEFTOVER_PATHS.includes("/clients/login"));
    assert.ok(LEFTOVER_PATHS.includes("/store/"));

    const checkReq = transport.requests.find((r) => pathnameOf(r).endsWith("/update/check"))!;
    const checkBody = JSON.parse(new TextDecoder().decode(checkReq.body!)) as Record<string, unknown>;
    for (const field of ["current_version", "os", "arch"]) {
      assert.ok(field in checkBody, `check missing ${field}`);
    }
    assert.equal("local_sha256" in checkBody, false);
    assert.equal("dirty_paths" in checkBody, false);
    assert.deepEqual(checkBody.capabilities, ["full_package"]);
    assert.equal(checkReq.headers["X-Channel-Token"], "ctok");
    assert.equal(checkReq.headers.Authorization, "Bearer ptok");

    const integ = transport.requests.find((r) => pathnameOf(r).includes("/integrity"))!;
    const iu = new URL(integ.url);
    assert.equal(iu.searchParams.get("os"), "windows");
    assert.equal(iu.searchParams.get("arch"), "x86_64");

    const diffReq = transport.requests.find((r) => pathnameOf(r).endsWith("/update/diff"))!;
    const diffBody = JSON.parse(new TextDecoder().decode(diffReq.body!)) as Record<string, unknown>;
    for (const field of ["source_version", "target_version", "os", "arch"]) {
      assert.ok(field in diffBody);
    }
    assert.equal(diffBody.local_sha256, "abc");

    const packReq = transport.requests.find((r) => pathnameOf(r).endsWith("/update/pack"))!;
    const packBody = JSON.parse(new TextDecoder().decode(packReq.body!)) as Record<string, unknown>;
    for (const field of ["source_version", "target_version", "os", "arch"]) {
      assert.ok(field in packBody);
    }

    const tel = transport.requests.find((r) => pathnameOf(r).endsWith("/telemetry/report"))!;
    const telBody = JSON.parse(new TextDecoder().decode(tel.body!)) as Record<string, unknown>;
    for (const field of ["os", "arch", "channel", "from_version", "to_version", "status"]) {
      assert.ok(field in telBody);
    }

    const report = transport.requests.find((r) => pathnameOf(r).endsWith("/clients/report"))!;
    const reportBody = JSON.parse(new TextDecoder().decode(report.body!)) as Record<string, unknown>;
    assert.equal(reportBody.device_id, "dev-1");

    const dl = transport.requests.find(
      (r) => r.method === "GET" && pathnameOf(r).includes("/packages/"),
    )!;
    const du = new URL(dl.url);
    assert.equal(du.searchParams.get("exp"), "1");
    assert.equal(du.searchParams.get("sig"), "2");
    assert.equal(dl.headers.Range, "bytes=0-10");
  });

  it("parses the error envelope and keeps unknown codes", async () => {
    const transport = new MockTransport(() =>
      jsonResponse(404, { error: { code: "CUSTOM_NEW_CODE", message: "nope", details: { k: 1 } } }),
    );
    const client = new Client({ baseUrl: "http://127.0.0.1:8080", projectRef: "demo", transport });
    await assert.rejects(
      () => client.project(),
      (err: unknown) => {
        assert.ok(err instanceof ApiError);
        assert.equal(err.code, "CUSTOM_NEW_CODE");
        assert.equal(err.message, "nope");
        assert.deepEqual(err.details, { k: 1 });
        assert.equal(err.status, 404);
        return true;
      },
    );
  });

  it("check 200 / 304 and RATE_LIMITED Retry-After", async () => {
    let n = 0;
    const transport = new MockTransport(() => {
      n += 1;
      if (n === 1) {
        return jsonResponse(
          200,
          {
            has_update: true,
            is_mandatory: false,
            is_downgrade: false,
            reason: "normal",
            compare_engine: "semver",
            version_integer: null,
            version_semver: "1.1.0",
            target_channel: "stable",
            target_hw_rev: null,
            package_type: "single_file",
            root_hash: "",
            package_url: "/api/v1/projects/demo/packages/aa",
            file_name: "a.bin",
            size: 1,
            sha256: "aa",
            delta_available: false,
          },
          { etag: '"e1"' },
        );
      }
      if (n === 2) {
        return jsonResponse(304, "", { etag: '"e1"' });
      }
      return jsonResponse(429, { error: { code: "RATE_LIMITED", message: "slow" } }, { "retry-after": "7" });
    });
    const client = new Client({ baseUrl: "http://127.0.0.1:8080", projectRef: "demo", transport });
    const a = await client.check({ current_version: "1.0.0", os: "windows", arch: "x86_64" });
    assert.equal(a.kind, "update");
    if (a.kind === "update") assert.equal(a.body.version_semver, "1.1.0");
    const b = await client.check(
      { current_version: "1.0.0", os: "windows", arch: "x86_64" },
      { ifNoneMatch: '"e1"' },
    );
    assert.equal(b.kind, "not_modified");
    await assert.rejects(
      () => client.project(),
      (err: unknown) => {
        assert.ok(err instanceof ApiError);
        assert.equal(err.code, "RATE_LIMITED");
        assert.equal(err.retryAfter, 7);
        return true;
      },
    );
  });

  it("does not call leftover GET /update/check or POST /clients/login", () => {
    const names = Object.getOwnPropertyNames(Client.prototype);
    assert.equal(names.includes("login"), false);
  });
});
