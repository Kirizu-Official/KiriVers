import assert from "node:assert/strict";
import { randomBytes } from "node:crypto";
import { readFileSync } from "node:fs";
import { describe, it } from "node:test";
import { Client } from "../src/client.js";
import { NodeHasher } from "../src/adapters.js";

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
      baseUrl: fixture.client_base_url,
      projectRef: fixture.project_ref,
    });

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
      await writeBackendIssue("check request failed", err);
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

async function writeBackendIssue(summary: string, detail: unknown): Promise<void> {
  const { writeFile } = await import("node:fs/promises");
  const { fileURLToPath } = await import("node:url");
  const path = await import("node:path");
  const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "../..");
  const text = `# Backend issue (TypeScript SDK integration)

Do not treat this as an SDK bug until the live client plane is checked.

## Summary

${summary}

## Expected

- Client plane at \`http://127.0.0.1:8080\`
- Project \`sdk-fixture\`, channel \`stable\`, os \`windows\`, arch \`x86_64\`
- Check from \`1.0.0\` yields target \`1.1.0\`
- Package SHA-256 \`7f063a6901ebfd0aa7b1adb8af58f4e792f13310f42213767ced623eb0669969\`

## Actual

\`\`\`
${typeof detail === "string" ? detail : JSON.stringify(detail, null, 2)}
\`\`\`

Language agent did not modify \`D:\\\\KiriVers\` server code.
`;
  await writeFile(path.join(root, "BACKEND_ISSUE.md"), text, "utf8");
}
