#!/usr/bin/env node

"use strict";

const crypto = require("node:crypto");
const fs = require("node:fs");
const https = require("node:https");
const os = require("node:os");
const path = require("node:path");
const { spawnSync } = require("node:child_process");
const packageJSON = require("../package.json");

const repository = "semyonfox/seol";
const version = packageJSON.version;
const platformNames = {
  darwin: "darwin",
  freebsd: "freebsd",
  linux: "linux",
  win32: "windows",
};
const architectureNames = {
  arm: "armv7",
  arm64: "arm64",
  x64: "x64",
};
const maxRedirects = 5;
const maxDownloadBytes = 64 * 1024 * 1024;

function fail(message) {
  process.stderr.write(`seol: ${message}\n`);
  process.exit(1);
}

function get(url, client = https, redirects = 0) {
  return new Promise((resolve, reject) => {
    const request = client.get(url, { headers: { "User-Agent": "seol-npm-cli" } }, (response) => {
      if (response.statusCode >= 300 && response.statusCode < 400 && response.headers.location) {
        response.resume();
        if (redirects >= maxRedirects) {
          reject(new Error("download exceeded redirect limit"));
          return;
        }
        resolve(get(new URL(response.headers.location, url).toString(), client, redirects + 1));
        return;
      }
      if (response.statusCode !== 200) {
        response.resume();
        reject(new Error(`download returned HTTP ${response.statusCode}`));
        return;
      }
      const chunks = [];
      let size = 0;
      let complete = false;
      response.on("data", (chunk) => {
        if (complete) {
          return;
        }
        size += chunk.length;
        if (size > maxDownloadBytes) {
          complete = true;
          const error = new Error("download exceeds 64 MiB limit");
          request.destroy(error);
          reject(error);
          return;
        }
        chunks.push(chunk);
      });
      response.on("end", () => {
        if (!complete) {
          resolve(Buffer.concat(chunks));
        }
      });
      response.on("error", reject);
    });
    request.setTimeout(30_000, () => request.destroy(new Error("download timed out")));
    request.on("error", reject);
  });
}

async function installBinary(binaryPath, assetName) {
  const releaseBase = `https://github.com/${repository}/releases/download/v${version}`;
  const [binary, checksums] = await Promise.all([
    get(`${releaseBase}/${assetName}`),
    get(`${releaseBase}/checksums.txt`),
  ]);
  const expectedLine = checksums
    .toString("utf8")
    .split(/\r?\n/)
    .find((line) => line.trim().endsWith(`  ${assetName}`));
  if (!expectedLine) {
    throw new Error(`release checksum is missing for ${assetName}`);
  }
  const expected = expectedLine.trim().split(/\s+/)[0];
  const actual = crypto.createHash("sha256").update(binary).digest("hex");
  if (!crypto.timingSafeEqual(Buffer.from(actual), Buffer.from(expected))) {
    throw new Error(`checksum verification failed for ${assetName}`);
  }
  fs.mkdirSync(path.dirname(binaryPath), { recursive: true, mode: 0o700 });
  const temporary = `${binaryPath}.${process.pid}.tmp`;
  fs.writeFileSync(temporary, binary, { mode: 0o755 });
  fs.renameSync(temporary, binaryPath);
}

async function main() {
  const platform = platformNames[process.platform];
  const architecture = architectureNames[process.arch];
  if (!platform || !architecture || (architecture === "armv7" && platform !== "linux")) {
    fail(`unsupported platform ${process.platform}/${process.arch}; use a native release or go install github.com/semyonfox/seol/cmd/seol@latest`);
  }
  const extension = platform === "windows" ? ".exe" : "";
  const assetName = `seol_${platform}_${architecture}${extension}`;
  const platformCache = process.platform === "win32"
    ? (process.env.LOCALAPPDATA || path.join(os.homedir(), "AppData", "Local"))
    : (process.env.XDG_CACHE_HOME || path.join(os.homedir(), ".cache"));
  const cacheRoot = process.env.SEOL_CLI_CACHE_DIR ||
    path.join(platformCache, "seol", "cli", version);
  const binaryPath = path.join(cacheRoot, assetName);
  if (!fs.existsSync(binaryPath)) {
    await installBinary(binaryPath, assetName);
  }
  const result = spawnSync(binaryPath, process.argv.slice(2), { stdio: "inherit" });
  if (result.error) {
    throw result.error;
  }
  process.exit(result.status === null ? 1 : result.status);
}

if (require.main === module) {
  main().catch((error) => fail(error.message));
}

module.exports = { get };
