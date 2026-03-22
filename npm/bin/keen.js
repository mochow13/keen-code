#!/usr/bin/env node
// Shim: locates the downloaded keen binary and passes through all arguments.

const { execFileSync } = require("child_process");
const path = require("path");
const fs = require("fs");

const binaryName = process.platform === "win32" ? "keen.exe" : "keen";
const binaryPath = path.join(__dirname, binaryName);

if (!fs.existsSync(binaryPath)) {
  console.error(
    "[keen] Binary not found. Try reinstalling: npm install -g keen-code"
  );
  process.exit(1);
}

try {
  execFileSync(binaryPath, process.argv.slice(2), { stdio: "inherit" });
} catch (err) {
  process.exit(err.status ?? 1);
}
