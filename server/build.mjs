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
 * Release build script for the Swiftx CLI.
 *
 * Cross-compiles the `./cmd/swiftx` entrypoint for darwin, linux, and
 * windows (amd64 + arm64) and writes the resulting binaries to `./build`.
 *
 * Usage:
 *   node build.mjs
 */

import { spawn } from "node:child_process";
import { mkdirSync, statSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";

/**
 * A single cross-compilation target.
 *
 * @typedef {object} Target
 * @property {string} goos - Target operating system (`GOOS`).
 * @property {string} goarch - Target CPU architecture (`GOARCH`).
 * @property {string} arch - Architecture label used in output file names, e.g. `x64`.
 */

/**
 * The result of building one target.
 *
 * @typedef {object} BuildResult
 * @property {Target} target - The target that was built.
 * @property {string} output - Path of the produced binary, relative to the project root.
 * @property {boolean} ok - Whether the build succeeded.
 * @property {number} sizeBytes - Size of the produced binary in bytes (0 on failure).
 * @property {number} durationMs - Wall-clock build duration in milliseconds.
 * @property {string} stderr - Captured stderr output of the `go build` process.
 */

/** Name of the produced binary. */
const BINARY_NAME = "swiftx";

/** Go package to compile. */
const ENTRYPOINT = "./cmd/swiftx";

/** Output directory, relative to the project root. */
const OUTPUT_DIR = "build";

/** Absolute path of the project root (the directory containing this script). */
const ROOT = dirname(fileURLToPath(import.meta.url));

/** @type {Target[]} */
const TARGETS = [
  { goos: "darwin", goarch: "amd64", arch: "x64" },
  { goos: "darwin", goarch: "arm64", arch: "arm64" },
  { goos: "linux", goarch: "amd64", arch: "x64" },
  { goos: "linux", goarch: "arm64", arch: "arm64" },
  { goos: "windows", goarch: "amd64", arch: "x64" },
  { goos: "windows", goarch: "arm64", arch: "arm64" },
];

/**
 * Compute the output path for a target, relative to the project root.
 *
 * @param {Target} target - The compilation target.
 * @returns {string} Relative output path, e.g. `build/swiftx-linux-x64`.
 */
function outputPath(target) {
  const ext = target.goos === "windows" ? ".exe" : "";
  return join(OUTPUT_DIR, `${BINARY_NAME}-${target.goos}-${target.arch}${ext}`);
}

/**
 * Format a byte count as a human-readable string.
 *
 * @param {number} bytes - Number of bytes.
 * @returns {string} Human-readable size, e.g. `12.3 MB`.
 */
function formatSize(bytes) {
  if (bytes >= 1024 * 1024) return `${(bytes / (1024 * 1024)).toFixed(1)} MB`;
  if (bytes >= 1024) return `${(bytes / 1024).toFixed(1)} KB`;
  return `${bytes} B`;
}

/**
 * Cross-compile the CLI for a single target.
 *
 * Stderr is captured instead of inherited so that concurrent builds do not
 * interleave their output.
 *
 * @param {Target} target - The compilation target.
 * @returns {Promise<BuildResult>} The outcome of the build.
 */
function build(target) {
  const output = outputPath(target);
  const startedAt = performance.now();

  return new Promise((resolve) => {
    const child = spawn(
      "go",
      ["build", "-trimpath", "-ldflags", "-s -w", "-o", output, ENTRYPOINT],
      {
        cwd: ROOT,
        stdio: ["ignore", "ignore", "pipe"],
        env: {
          ...process.env,
          CGO_ENABLED: "0",
          GOOS: target.goos,
          GOARCH: target.goarch,
        },
      },
    );

    let stderr = "";
    child.stderr.on("data", (chunk) => {
      stderr += chunk;
    });

    child.on("close", (code) => {
      const durationMs = performance.now() - startedAt;
      const ok = code === 0;
      const sizeBytes = ok ? statSync(join(ROOT, output)).size : 0;
      resolve({ target, output, ok, sizeBytes, durationMs, stderr });
    });
  });
}

/**
 * Build every target concurrently (one `go build` process per target) and
 * print a summary.
 *
 * @returns {Promise<void>}
 */
async function main() {
  mkdirSync(join(ROOT, OUTPUT_DIR), { recursive: true });

  console.log(`Building ${TARGETS.length} targets concurrently...`);

  const results = await Promise.all(
    TARGETS.map(async (target) => {
      const result = await build(target);
      if (result.ok) {
        console.log(
          `ok ${target.goos}/${target.arch} (${formatSize(result.sizeBytes)}, ${(result.durationMs / 1000).toFixed(1)}s) -> ${result.output}`,
        );
      } else {
        console.error(`FAILED ${target.goos}/${target.arch}\n${result.stderr}`);
      }
      return result;
    }),
  );

  const failed = results.filter((r) => !r.ok);
  console.log(
    `\nDone: ${results.length - failed.length}/${results.length} targets built into ./${OUTPUT_DIR}`,
  );

  if (failed.length > 0) {
    console.error(
      `Failed targets: ${failed.map((r) => `${r.target.goos}/${r.target.arch}`).join(", ")}`,
    );
    process.exitCode = 1;
  }
}

await main();
