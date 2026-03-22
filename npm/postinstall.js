#!/usr/bin/env node
// Downloads the keen binary for the current platform during npm install.

const https = require("https");
const fs = require("fs");
const path = require("path");
const crypto = require("crypto");
const { execFileSync } = require("child_process");

const REPO = "mochow13/keen-code";
const pkg = require("./package.json");
const VERSION = pkg.version;

const PLATFORM_MAP = {
  darwin: "darwin",
  linux: "linux",
  win32: "windows",
};

const ARCH_MAP = {
  x64: "amd64",
  arm64: "arm64",
};

function fail(msg) {
  console.error(`[keen] ${msg}`);
  process.exit(1);
}

const platform = PLATFORM_MAP[process.platform];
if (!platform) fail(`Unsupported platform: ${process.platform}`);

const arch = ARCH_MAP[process.arch];
if (!arch) fail(`Unsupported architecture: ${process.arch}`);

const ext = platform === "windows" ? "zip" : "tar.gz";
const archiveName = `keen_${VERSION}_${platform}_${arch}.${ext}`;
const baseURL = `https://github.com/${REPO}/releases/download/v${VERSION}`;
const archiveURL = `${baseURL}/${archiveName}`;
const checksumsURL = `${baseURL}/checksums.txt`;

const binDir = path.join(__dirname, "bin");
const binaryName = platform === "windows" ? "keen.exe" : "keen";
const binaryPath = path.join(binDir, binaryName);

if (!fs.existsSync(binDir)) fs.mkdirSync(binDir, { recursive: true });

function download(url) {
  return new Promise((resolve, reject) => {
    const chunks = [];
    const get = (u) =>
      https.get(u, { headers: { "User-Agent": "keen-code-npm-installer" } }, (res) => {
        if (res.statusCode === 301 || res.statusCode === 302) {
          get(res.headers.location);
          return;
        }
        if (res.statusCode !== 200) {
          reject(new Error(`HTTP ${res.statusCode} for ${u}`));
          return;
        }
        res.on("data", (c) => chunks.push(c));
        res.on("end", () => resolve(Buffer.concat(chunks)));
        res.on("error", reject);
      });
    get(url);
  });
}

function verifySHA256(buf, expected) {
  const actual = crypto.createHash("sha256").update(buf).digest("hex");
  if (actual !== expected) {
    fail(`Checksum mismatch: expected ${expected}, got ${actual}`);
  }
}

function extractBinary(archiveBuf) {
  const tmpArchive = path.join(binDir, archiveName);
  fs.writeFileSync(tmpArchive, archiveBuf);

  if (ext === "tar.gz") {
    execFileSync("tar", ["-xzf", tmpArchive, "-C", binDir, binaryName]);
  } else {
    // windows zip
    execFileSync("powershell", [
      "-Command",
      `Expand-Archive -Path '${tmpArchive}' -DestinationPath '${binDir}' -Force`,
    ]);
  }

  fs.unlinkSync(tmpArchive);
  fs.chmodSync(binaryPath, 0o755);
}

async function main() {
  console.log(`[keen] Downloading keen v${VERSION} for ${platform}/${arch}...`);

  const [archiveBuf, checksumsBuf] = await Promise.all([
    download(archiveURL),
    download(checksumsURL),
  ]);

  const checksums = checksumsBuf.toString("utf8");
  const match = checksums.split("\n").find((line) => line.includes(archiveName));
  if (!match) fail(`No checksum entry found for ${archiveName}`);

  const expectedHash = match.trim().split(/\s+/)[0];
  verifySHA256(archiveBuf, expectedHash);
  console.log("[keen] Checksum verified.");

  extractBinary(archiveBuf);
  console.log(`[keen] Installed to ${binaryPath}`);
}

main().catch((err) => fail(err.message));
