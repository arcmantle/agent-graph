const assert = require("node:assert/strict");
const test = require("node:test");

const packageInfo = require("./package.json");

test("unscoped package uses the matching canonical package version", () => {
	assert.equal(packageInfo.dependencies["@arcmantle/agent-graph"], packageInfo.version);
});