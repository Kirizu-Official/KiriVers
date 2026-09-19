import assert from "node:assert/strict";
import { randomBytes } from "node:crypto";
import { readFileSync } from "node:fs";
import { describe, it } from "node:test";
import { Client } from "../src/client.js";
import { NodeHasher } from "../src/adapters.js";
import { ApiError } from "../src/errors.js";

const FIXTURE_PATH = process.env.KIRIVERS_SDK_FIXTURE ?? "D:\\KiriVers\\configs\\sdk-fixture.json";

interface Fixture {
  client_base_url: string;
  project_ref: string;
  channel: string;
  os: string;
  arch: string;
  current_version: string;
  target_version: string;
  sha256: Record<string, string>;
}

describe("integration vs local client plane", () => {
  it("check 1.0.0 → download 1.1.0 and verify sha256", async () => {
    let fixture: Fixture;
    try {
      fixture = JSON.parse(readFileSync(FIXTURE_PATH, "utf8")) as Fixture;
    } catch (err) {
      throw new Error(`cannot read sdk-fixture.json at ${FIXTURE_PATH}: ${String(err)}`);
    }

    const deviceId = `sdk-typescript-${randomBytes(8).toString("hex")}`;
    const client = new Client({
      baseUrl: process.env.KIRIVERS_CLIENT_BASE_URL ?? fixture.client_base_url,
      projectRef: fixture.project_ref,
    });

    try {
      const health = await client.health();
      assert.equal(health.status, "ok");
    } catch (err) {
      await writeBackendIssue("GET /api/v1/health failed", err);
      throw err;
    }

    let check;
    try {
      check = await client.check({
        current_version: fixture.current_version,
        os: fixture.os,
        arch: fixture.arch,
        channel: fixture.channel,
        device_id: deviceId,
      });
    } catch (err) {
      await writeBackendIssue(
        `POST /api/v1/projects/${fixture.project_ref}/update/check returned an error (curl GET project is also 404 PROJECT_NOT_FOUND; client plane /health is ready)`,
        err,
      );
      throw err;
    }

    if (check.kind !== "update") {
      await writeBackendIssue(`expected update from ${fixture.current_version}`, check);
      assert.equal(check.kind, "update");
      return;
    }
    if (check.body.version_semver !== fixture.target_version) {
      await writeBackendIssue(`expected target ${fixture.target_version}`, check.body);
    }
    assert.equal(check.body.version_semver, fixture.target_version);

    const expectedSha = fixture.sha256[fixture.target_version];
    assert.equal(expectedSha, "7f063a6901ebfd0aa7b1adb8af58f4e792f13310f42213767ced623eb0669969");

    const dl = await client.downloadUrl(check.body.package_url);
    const actual = new NodeHasher().sha256(dl.body);
    if (actual !== expectedSha) {
      await writeBackendIssue(`sha256 mismatch for ${fixture.target_version}`, {
        expected: expectedSha,
        actual,
        size: dl.body.length,
        status: dl.status,
      });
    }
    assert.equal(actual, expectedSha);
    assert.equal(check.body.sha256, expectedSha);
  });
});

function serializeDetail(detail: unknown): string {
  if (detail instanceof ApiError) {
    return JSON.stringify(
      {
        name: detail.name,
        code: detail.code,
        message: detail.message,
        status: detail.status,
        details: detail.details,
      },
      null,
      2,
    );
  }
  if (typeof detail === "string") return detail;
  try {
    return JSON.stringify(detail, null, 2);
  } catch {
    return String(detail);
  }
}

async function writeBackendIssue(summary: string, detail: unknown): Promise<void> {
  const { writeFile } = await import("node:fs/promises");
  const { fileURLToPath } = await import("node:url");
  const path = await import("node:path");
  const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "../..");
  const text = `# Backend issue (TypeScript SDK integration)

Do not treat this as an SDK bug until the live client plane is checked.
Language agent did not modify \`D:\\\\KiriVers\` server code.

## Repro

1. \`GET http://127.0.0.1:8080/api/v1/health\` → HTTP 200 \`{"ready":true,"status":"ok"}\`
2. \`GET http://127.0.0.1:8080/api/v1/projects/sdk-fixture\`
3. \`POST /api/v1/projects/sdk-fixture/update/check\` with \`current_version=1.0.0\`, \`os=windows\`, \`arch=x86_64\`, unique \`device_id=sdk-typescript-<random>\`

## Expected

- Project \`sdk-fixture\` exists on the client plane
- Channel \`stable\`, os \`windows\`, arch \`x86_64\`
- Check from \`1.0.0\` yields target \`1.1.0\`
- Package SHA-256 \`7f063a6901ebfd0aa7b1adb8af58f4e792f13310f42213767ced623eb0669969\`
- Matches \`D:\\\\KiriVers\\\\configs\\\\sdk-fixture.json\`

## Actual

${summary}

\`\`\`
${serializeDetail(detail)}
\`\`\`

## Suggested fix

Recreate or restore the \`sdk-fixture\` project (published 1.0.0 and 1.1.0, windows/x86_64 single-file artifacts) on the host client plane at \`:8080\`. Re-seed with \`.trellis/tasks/09-17-client-sdk/scripts/seed_local_fixture.py\`. Do not change the TypeScript SDK for this 404.
`;
  await writeFile(path.join(root, "BACKEND_ISSUE.md"), text, "utf8");
}
