#!/usr/bin/env node
/**
 * Copyright (c) 2026 hangtiancheng
 *
 * Permission is hereby granted, free of charge, to any person obtaining a copy
 * of this software and associated documentation files (the "Software"), to deal
 * in the Software without restriction, including without limitation the rights
 * to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
 * copies of the Software, and to permit persons to whom the Software is
 * furnished to do so, subject to the following conditions:
 *
 * The above copyright notice and this permission notice shall be included in
 * all copies or substantial portions of the Software.
 *
 * THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
 * IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
 * FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
 * AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
 * LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
 * OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
 * SOFTWARE.
 */

/**
 * GitHub Release publish script for the Swiftx CLI.
 *
 * Builds all targets (via build.mjs), then creates the release `swiftx` on
 * github.com/hangtiancheng/swifty.go and uploads the binaries from ./build as
 * release assets. If the release already exists, assets are uploaded with
 * --clobber.
 *
 * Requires the GitHub CLI (`gh`) to be installed and authenticated.
 *
 * Usage:
 *   node release.mjs [--skip-build]
 */

import { spawnSync } from "node:child_process";
import { existsSync, readdirSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";

/** GitHub repository that hosts the releases. */
const REPO = "hangtiancheng/swifty.go";

/** Output directory produced by build.mjs, relative to the project root. */
const OUTPUT_DIR = "build";

/** Expected release asset file names. */
const EXPECTED_ASSETS = [
  "swiftx-darwin-arm64",
  "swiftx-darwin-x64",
  "swiftx-linux-arm64",
  "swiftx-linux-x64",
  "swiftx-windows-arm64.exe",
  "swiftx-windows-x64.exe",
];

/** Absolute path of the project root (the directory containing this script). */
const ROOT = dirname(fileURLToPath(import.meta.url));

/**
 * Print an error message and exit with a non-zero status.
 *
 * @param {string} message - The error message.
 * @returns {never}
 */
function fail(message) {
  console.error(`[release] ${message}`);
  process.exit(1);
}

/**
 * Run a command, inheriting stdio, and fail the script if it exits non-zero.
 *
 * @param {string} command - The executable to run.
 * @param {string[]} args - Command arguments.
 * @returns {void}
 */
function run(command, args) {
  console.log(`[release] $ ${command} ${args.join(" ")}`);
  const result = spawnSync(command, args, { cwd: ROOT, stdio: "inherit" });
  if (result.error) {
    fail(`failed to run "${command}": ${result.error.message}`);
  }
  if (result.status !== 0) {
    fail(`"${command}" exited with code ${result.status}`);
  }
}

/**
 * Run a command silently and report whether it exited with code 0.
 *
 * @param {string} command - The executable to run.
 * @param {string[]} args - Command arguments.
 * @returns {boolean} True if the command succeeded.
 */
function check(command, args) {
  const result = spawnSync(command, args, { cwd: ROOT, stdio: "ignore" });
  return result.status === 0;
}

const args = process.argv.slice(2);
const skipBuild = args.includes("--skip-build");

if (!check("gh", ["--version"])) {
  fail(
    "GitHub CLI (gh) not found. Install it, then run `gh auth login`:\n" +
      "  macOS:          brew install gh\n" +
      "  Ubuntu/Debian:  sudo apt update && sudo apt install gh\n" +
      "                  (older releases: see https://github.com/cli/cli/blob/trunk/docs/install_linux.md)\n" +
      "  Windows:        winget install --id GitHub.cli\n" +
      "                  (or: choco install gh / scoop install gh)\n" +
      "  Other:          https://cli.github.com",
  );
}
if (!check("gh", ["auth", "status"])) {
  fail('gh is not authenticated, run "gh auth login" first');
}

if (!skipBuild) {
  run("node", ["./build.mjs"]);
}

const buildDir = join(ROOT, OUTPUT_DIR);
if (!existsSync(buildDir)) {
  fail(`build output not found: ${buildDir}`);
}

const missing = EXPECTED_ASSETS.filter(
  (name) => !existsSync(join(buildDir, name)),
);
if (missing.length > 0) {
  fail(
    `missing binaries in ./${OUTPUT_DIR}: ${missing.join(", ")} (run without --skip-build)`,
  );
}

const extra = readdirSync(buildDir).filter(
  (name) => !EXPECTED_ASSETS.includes(name),
);
if (extra.length > 0) {
  console.log(
    `[release] ignoring extra files in ./${OUTPUT_DIR}: ${extra.join(", ")}`,
  );
}

const tag = "swiftx";
const assets = EXPECTED_ASSETS.map((name) => join(buildDir, name));

if (check("gh", ["release", "view", tag, "--repo", REPO])) {
  console.log(
    `[release] release ${tag} already exists, uploading assets with --clobber`,
  );
  run("gh", ["release", "upload", tag, ...assets, "--repo", REPO, "--clobber"]);
} else {
  run("gh", [
    "release",
    "create",
    tag,
    ...assets,
    "--repo",
    REPO,
    "--title",
    tag,
    "--notes",
    `Swiftx CLI native binaries for ${tag}.`,
  ]);
}

console.log(`[release] done: https://github.com/${REPO}/releases/tag/${tag}`);
