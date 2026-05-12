"use strict";

// Downloads the wuphf binary that matches the current package version
// from the corresponding GitHub release and extracts it into bin/.
// GoReleaser archive name: wuphf_<version>_<os>_<arch>.tar.gz
// where <version> is the tag without the leading 'v'.

const fs = require("node:fs");
const fsp = require("node:fs/promises");
const path = require("node:path");
const os = require("node:os");
const crypto = require("node:crypto");
const { execFileSync } = require("node:child_process");

const REPO = process.env.WUPHF_RELEASE_REPO || "luiztrilha/dunderia";

function detectPlatform() {
  const platform = process.platform;
  const arch = process.arch;

  const osMap = { darwin: "darwin", linux: "linux" };
  const archMap = { x64: "amd64", arm64: "arm64" };

  if (!osMap[platform]) {
    throw new Error(
      `Unsupported platform: ${platform}. wuphf supports darwin and linux.`,
    );
  }
  if (!archMap[arch]) {
    throw new Error(
      `Unsupported architecture: ${arch}. wuphf supports x64 (amd64) and arm64.`,
    );
  }
  return { os: osMap[platform], arch: archMap[arch] };
}

function packageVersion() {
  const pkg = JSON.parse(
    fs.readFileSync(path.join(__dirname, "..", "package.json"), "utf8"),
  );
  return process.env.WUPHF_RELEASE_VERSION || pkg.version;
}

function assertDownloadableVersion(version) {
  if (!version || version === "0.0.0") {
    throw new Error(
      "No downloadable wuphf release is configured. The npm package version is 0.0.0, " +
        "which is a development placeholder and has no GitHub release asset. " +
        "Publish with a real package version or set WUPHF_RELEASE_VERSION to an existing release tag without the leading 'v'.",
    );
  }
}

function archiveUrl(version) {
  const { os: goOs, arch: goArch } = detectPlatform();
  const archive = `wuphf_${version}_${goOs}_${goArch}.tar.gz`;
  return {
    archive,
    url: `https://github.com/${REPO}/releases/download/v${version}/${archive}`,
  };
}

function checksumUrl(version) {
  return (
    process.env.WUPHF_RELEASE_CHECKSUMS_URL ||
    `https://github.com/${REPO}/releases/download/v${version}/checksums.txt`
  );
}

async function fetchToFile(url, dest) {
  const res = await fetch(url, { redirect: "follow" });
  if (!res.ok) {
    throw new Error(`Download failed: ${res.status} ${res.statusText} (${url})`);
  }
  const buf = Buffer.from(await res.arrayBuffer());
  await fsp.writeFile(dest, buf);
  return buf;
}

async function fetchText(url) {
  const res = await fetch(url, { redirect: "follow" });
  if (!res.ok) {
    throw new Error(`Checksum download failed: ${res.status} ${res.statusText} (${url})`);
  }
  return await res.text();
}

function checksumFromListing(listing, archive) {
  for (const line of listing.split(/\r?\n/)) {
    const trimmed = line.trim();
    if (!trimmed || !trimmed.includes(archive)) continue;
    const match = trimmed.match(/^([a-fA-F0-9]{64})\s+\*?(.+)$/);
    if (match && path.basename(match[2].trim()) === archive) {
      return match[1].toLowerCase();
    }
  }
  return "";
}

async function expectedChecksum(version, archive) {
  if (process.env.WUPHF_RELEASE_SHA256) {
    return process.env.WUPHF_RELEASE_SHA256.trim().toLowerCase();
  }
  const listing = await fetchText(checksumUrl(version));
  const checksum = checksumFromListing(listing, archive);
  if (!checksum) {
    throw new Error(`No SHA-256 checksum found for ${archive}`);
  }
  return checksum;
}

function verifyChecksum(buffer, expected, archive) {
  const actual = crypto.createHash("sha256").update(buffer).digest("hex");
  if (actual !== expected) {
    throw new Error(
      `Checksum mismatch for ${archive}: expected ${expected}, got ${actual}`,
    );
  }
}

async function downloadBinary({ silent = false } = {}) {
  const version = packageVersion();
  assertDownloadableVersion(version);
  const { archive, url } = archiveUrl(version);
  const binDir = path.join(__dirname, "..", "bin");
  const binaryPath = path.join(binDir, "wuphf");

  await fsp.mkdir(binDir, { recursive: true });

  const tmpDir = await fsp.mkdtemp(path.join(os.tmpdir(), "wuphf-"));
  const archivePath = path.join(tmpDir, "wuphf.tar.gz");

  try {
    if (!silent) {
      process.stderr.write(`wuphf: downloading ${url}\n`);
    }
    const archiveBytes = await fetchToFile(url, archivePath);
    const checksum = await expectedChecksum(version, archive);
    verifyChecksum(archiveBytes, checksum, archive);

    // Extract using system tar (available on darwin + linux).
    execFileSync("tar", ["-xzf", archivePath, "-C", tmpDir], {
      stdio: silent ? "ignore" : "inherit",
    });

    const extractedBinary = path.join(tmpDir, "wuphf");
    await fsp.copyFile(extractedBinary, binaryPath);
    await fsp.chmod(binaryPath, 0o755);

    // macOS 15+ invalidates GoReleaser's embedded ad-hoc signature after
    // copy+chmod. Re-sign so the kernel does not SIGKILL on exec.
    if (process.platform === "darwin") {
      try {
        execFileSync("codesign", ["--force", "--sign", "-", binaryPath], {
          stdio: "ignore",
        });
      } catch {
        // codesign is optional — binary may still run.
      }
    }

    return binaryPath;
  } finally {
    await fsp.rm(tmpDir, { recursive: true, force: true });
  }
}

module.exports = { downloadBinary, packageVersion };
