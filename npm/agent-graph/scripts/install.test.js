const assert = require("node:assert/strict");
const test = require("node:test");

const { expectedChecksum, platformAsset } = require("./install.js");

test("platformAsset maps supported release platforms", () => {
  assert.equal(platformAsset("darwin", "arm64"), "agent-graph-darwin-arm64.gz");
  assert.equal(platformAsset("darwin", "x64"), "agent-graph-darwin-x64.gz");
  assert.equal(platformAsset("linux", "x64"), "agent-graph-linux-x64.gz");
  assert.equal(platformAsset("win32", "x64"), "agent-graph-win32-x64.gz");
});

test("platformAsset rejects unsupported release platforms", () => {
  assert.throws(() => platformAsset("linux", "arm64"), /Unsupported platform/);
  assert.throws(() => platformAsset("win32", "arm64"), /Unsupported platform/);
});

test("expectedChecksum finds GNU and BSD checksum entries", () => {
  const checksums = Buffer.from(
    "abc123\tother-file\n" +
      "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef  agent-graph-linux-x64.gz\n"
  );
  assert.equal(
    expectedChecksum(checksums, "agent-graph-linux-x64.gz"),
    "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
  );
});