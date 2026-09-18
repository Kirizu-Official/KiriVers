import assert from "node:assert/strict";
import { describe, it } from "node:test";
import { PathError } from "../src/errors.js";
import { normalizeRelPath, uniqueNeededPaths } from "../src/pathutil.js";

describe("pathutil", () => {
  it("converts backslashes, NFC, and matches server fileset rules", () => {
    const decomposed = "cafe\u0301/file.bin";
    const composed = "café/file.bin";
    assert.equal(normalizeRelPath(decomposed), normalizeRelPath(composed));
    assert.equal(normalizeRelPath("a\\b\\c"), "a/b/c");
    assert.equal(normalizeRelPath("bin//sub\\\\dir///file.txt"), "bin/sub/dir/file.txt");
    assert.equal(normalizeRelPath("folder/subfolder/"), "folder/subfolder");
    assert.throws(() => normalizeRelPath("../secret"), PathError);
    assert.throws(() => normalizeRelPath("ok/../../x"), PathError);
    assert.throws(() => normalizeRelPath("./file.txt"), PathError);
    assert.throws(() => normalizeRelPath("dir/./file.txt"), PathError);
    assert.throws(() => normalizeRelPath("/etc/passwd"), PathError);
    assert.throws(() => normalizeRelPath("C:/Windows/app.exe"), PathError);
    assert.throws(() => normalizeRelPath("foo/C:/bar"), PathError);
    assert.throws(() => normalizeRelPath("dir/file\x00.txt"), PathError);
    assert.throws(() => normalizeRelPath(""), PathError);
  });

  it("uniqueNeededPaths is order-stable unique NFC", () => {
    const a = uniqueNeededPaths(["b.bin", "a.bin", "a.bin", "b.bin"]);
    const b = uniqueNeededPaths(["a.bin", "b.bin"]);
    assert.deepEqual(a, b);
  });
});
