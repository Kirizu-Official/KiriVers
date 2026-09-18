import assert from "node:assert/strict";
import { generateKeyPairSync, sign as nodeSign } from "node:crypto";
import { describe, it } from "node:test";
import { NodeSignatureVerifier } from "../src/adapters.js";
import { buildCheckPayload, payloadFromCheckLike } from "../src/signature.js";

describe("signature payload", () => {
  it("matches pkg/signature.BuildCheckPayload", () => {
    assert.equal(
      buildCheckPayload("102", "1.2.3", "roothash", "/pkg", "123456", "abcd"),
      "102\n1.2.3\nroothash\n/pkg\n123456\nabcd",
    );
    assert.equal(buildCheckPayload("", "1.2.3", "", "/pkg", "", "abcd"), "\n1.2.3\n\n/pkg\n\nabcd");
    assert.equal(
      payloadFromCheckLike({
        version_integer: 102,
        version_semver: "1.2.3",
        root_hash: "roothash",
        package_url: "/pkg",
        size: 123456,
        sha256: "abcd",
      }),
      "102\n1.2.3\nroothash\n/pkg\n123456\nabcd",
    );
  });

  it("verifies Ed25519 and RSA-SHA256", () => {
    const v = new NodeSignatureVerifier();
    const payload = buildCheckPayload("102", "1.2.3", "root", "/u", "1", "aa");

    const ed = generateKeyPairSync("ed25519");
    const edSig = nodeSign(null, Buffer.from(payload), ed.privateKey).toString("base64");
    assert.equal(v.verify("ed25519", ed.publicKey.export({ type: "spki", format: "pem" }).toString(), payload, edSig), true);
    assert.equal(
      v.verify("ed25519", ed.publicKey.export({ type: "spki", format: "pem" }).toString(), payload + "x", edSig),
      false,
    );

    const rsa = generateKeyPairSync("rsa", { modulusLength: 2048 });
    const rsaSig = nodeSign("sha256", Buffer.from(payload), rsa.privateKey).toString("base64");
    const pem = rsa.publicKey.export({ type: "spki", format: "pem" }).toString();
    assert.equal(v.verify("rsa-sha256", pem, payload, rsaSig), true);
    assert.equal(v.verify("rsa-sha256", pem, payload + "x", rsaSig), false);
  });
});
