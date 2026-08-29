#!/usr/bin/env node

const { spawnSync } = require("node:child_process");
const path = require("node:path");

const packageDirectory = path.dirname(require.resolve("@arcmantle/agent-graph/package.json"));
const command = path.join(packageDirectory, "bin", "agent-graph.js");
const result = spawnSync(process.execPath, [command, ...process.argv.slice(2)], { stdio: "inherit" });

if (result.error) {
	console.error(`agent-graph could not start: ${result.error.message}`);
	process.exitCode = 1;
} else {
	process.exitCode = result.status ?? 1;
}