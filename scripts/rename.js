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

// @ts-check
"use strict";

/**
 * Bulk-rename files and directories under a root.
 *
 * Features:
 * - Case-sensitive by default.
 * - Optional regex mode: each rule's source is compiled as a regex pattern,
 *   and target may use backreferences (e.g. "$1").
 * - Skips paths ignored by git by default; falls back to a built-in deny-list
 *   when the root is not inside a git repository.
 * - Processes deepest paths first to avoid renaming a parent before its children.
 *
 * Library usage:
 *     import { renamePaths } from './rename.js';
 *     renamePaths([['foo', 'bar']], { root: '.', regex: false, dryRun: true });
 *
 * CLI usage:
 *     node scripts/rename.js "foo=>bar" "v1=>v2" --apply
 *     node scripts/rename.js "(?i)readme=>README" --regex --apply
 */

import fs from "node:fs";
import path from "node:path";
import { spawnSync } from "node:child_process";
import { fileURLToPath } from "node:url";

/** @type {Set<string>} Fallback deny-list when the root is not a git repo. */
const FALLBACK_IGNORED_DIRS = new Set([
  ".git",
  ".hg",
  ".svn",
  "node_modules",
  "env",
  "dist",
  "build",
  ".next",
  ".turbo",
  ".cache",
]);

/**
 * @typedef {object} RenamePlan
 * @property {string} src - Absolute source path.
 * @property {string} dst - Absolute destination path.
 */

/**
 * @typedef {object} RenameOptions
 * @property {string} root - Directory to scan recursively.
 * @property {boolean} dryRun - If true, print without touching the filesystem.
 * @property {boolean} regex - If true, treat each rule as regex substitution.
 * @property {boolean} respectGitignore - If true, skip git-ignored paths.
 */

// ---------- name transformation ---------------------------------------------

/**
 * Apply rules as literal, case-sensitive substring replacements.
 * @param {string} name
 * @param {Array<[string, string]>} rules
 * @returns {string}
 */
function applyLiteral(name, rules) {
  let newName = name;
  for (const [source, target] of rules) {
    if (!source) continue;
    newName = newName.replaceAll(source, target);
  }
  return newName;
}

/**
 * Apply rules as regex substitutions; target may use backreferences.
 * @param {string} name
 * @param {Array<[RegExp, string]>} compiled
 * @returns {string}
 */
function applyRegex(name, compiled) {
  let newName = name;
  for (const [pattern, target] of compiled) {
    newName = newName.replace(pattern, target);
  }
  return newName;
}

// ---------- ignore filtering -------------------------------------------------

/**
 * Return true if `root` lives inside a git working tree.
 * @param {string} root
 * @returns {boolean}
 */
function isGitRepo(root) {
  const result = spawnSync(
    "git",
    ["-C", root, "rev-parse", "--is-inside-work-tree"],
    {
      stdio: ["pipe", "pipe", "pipe"],
    },
  );
  return (
    result.status === 0 &&
    (result.stdout ?? Buffer.alloc(0)).toString().trim() === "true"
  );
}

/**
 * Ask git which of `candidates` are ignored. Empty set on failure.
 * @param {string} root
 * @param {string[]} candidates
 * @returns {Set<string>}
 */
function gitIgnored(root, candidates) {
  if (candidates.length === 0) return new Set();

  const stdin = candidates.join("\x00");
  const result = spawnSync(
    "git",
    ["-C", root, "check-ignore", "--stdin", "-z"],
    {
      input: stdin,
      stdio: ["pipe", "pipe", "pipe"],
    },
  );

  if (result.status !== 0 && result.status !== 1) return new Set();
  const output = (result.stdout ?? Buffer.alloc(0)).toString();
  /** @type {Set<string>} */
  const ignored = new Set(output.split("\x00").filter(Boolean));
  return ignored;
}

/**
 * True if `p` is equal to or nested inside any ancestor.
 * @param {string} p - Absolute path.
 * @param {string[]} ancestors - Absolute paths.
 * @returns {boolean}
 */
function isUnder(p, ancestors) {
  return ancestors.some((a) => p === a || p.startsWith(a + path.sep));
}

// ---------- path collection --------------------------------------------------

/**
 * Walk `root` recursively, applying ignore rules, deepest-first.
 * @param {string} root - Absolute root path.
 * @param {boolean} respectGitignore
 * @returns {string[]}
 */
function collectPaths(root, respectGitignore) {
  /** @type {string[]} */
  const allPaths = walkRecursive(root);

  if (respectGitignore && isGitRepo(root)) {
    const ignored = gitIgnored(root, allPaths);
    // Always keep .git itself out of the rename set.
    for (const p of allPaths) {
      if (path.basename(p) === ".git" && fs.statSync(p).isDirectory()) {
        ignored.add(p);
      }
    }
    /** @type {string[]} */
    const ignoredDirs = [...ignored].filter((p) => {
      try {
        return fs.statSync(p).isDirectory();
      } catch {
        return false;
      }
    });
    /** @type {string[]} */
    const kept = allPaths.filter(
      (p) => !ignored.has(p) && !isUnder(p, ignoredDirs),
    );
    return kept.sort((a, b) => b.length - a.length);
  }

  /** @type {string[]} */
  const kept = allPaths.filter((p) => {
    const relative = path.relative(root, p);
    const parts = relative.split(path.sep);
    return !parts.some((part) => FALLBACK_IGNORED_DIRS.has(part));
  });
  return kept.sort((a, b) => b.length - a.length);
}

/**
 * Recursively walk a directory and return all file/dir paths.
 * @param {string} dir
 * @returns {string[]}
 */
function walkRecursive(dir) {
  /** @type {string[]} */
  const results = [];
  const entries = fs.readdirSync(dir, { withFileTypes: true });
  for (const entry of entries) {
    const fullPath = path.join(dir, entry.name);
    results.push(fullPath);
    if (entry.isDirectory()) {
      results.push(...walkRecursive(fullPath));
    }
  }
  return results;
}

// ---------- planning & execution --------------------------------------------

/**
 * Compute the rename plan for paths whose basename actually changes.
 * @param {string[]} paths
 * @param {Array<[string, string]>} rules
 * @param {boolean} regex
 * @returns {RenamePlan[]}
 */
function buildPlans(paths, rules, regex) {
  /** @type {Array<[RegExp, string]>} */
  const compiled = regex
    ? rules.map(([src, dst]) => [new RegExp(src), dst])
    : [];

  /** @type {RenamePlan[]} */
  const plans = [];
  for (const src of paths) {
    const baseName = path.basename(src);
    const newName = regex
      ? applyRegex(baseName, compiled)
      : applyLiteral(baseName, rules);
    if (newName === baseName || !newName) continue;
    plans.push({ src, dst: path.join(path.dirname(src), newName) });
  }
  return plans;
}

/**
 * Execute rename plans; return [succeeded, skipped] counts.
 * @param {RenamePlan[]} plans
 * @param {boolean} dryRun
 * @returns {[number, number]}
 */
function execute(plans, dryRun) {
  let succeeded = 0;
  let skipped = 0;
  for (const plan of plans) {
    if (fs.existsSync(plan.dst)) {
      console.error(`[SKIP] target exists: ${plan.src} -> ${plan.dst}`);
      skipped++;
      continue;
    }
    console.log(`[${dryRun ? "DRY" : "OK "}] ${plan.src} -> ${plan.dst}`);
    if (!dryRun) fs.renameSync(plan.src, plan.dst);
    succeeded++;
  }
  return [succeeded, skipped];
}

// ---------- public entry point -----------------------------------------------

/**
 * Rename every file/directory under `root` whose name matches a rule.
 * @param {Array<[string, string]>} rules - Ordered list of [source, target] pairs.
 * @param {Partial<RenameOptions>} options
 * @returns {[number, number]} [renamedCount, skippedCount]
 */
function renamePaths(rules, options = {}) {
  const rootPath = path.resolve(options.root ?? ".");
  if (!fs.existsSync(rootPath) || !fs.statSync(rootPath).isDirectory()) {
    throw new Error(`root is not a directory: ${rootPath}`);
  }

  const paths = collectPaths(rootPath, options.respectGitignore ?? true);
  const plans = buildPlans(paths, rules, options.regex ?? false);
  return execute(plans, options.dryRun ?? true);
}

// ---------- CLI --------------------------------------------------------------

/**
 * Parse CLI rule strings of the form 'SOURCE=>TARGET'.
 * @param {string[]} raw
 * @returns {Array<[string, string]>}
 */
function parseCliRules(raw) {
  /** @type {Array<[string, string]>} */
  const rules = [];
  for (const item of raw) {
    if (!item.includes("=>")) {
      throw new Error(`invalid rule (expected SOURCE=>TARGET): '${item}'`);
    }
    const [source, target] = item.split("=>", 2);
    rules.push([source, target]);
  }
  return rules;
}

/**
 * @typedef {object} CliResult
 * @property {Array<[string, string]>} rules
 * @property {string} root
 * @property {boolean} apply
 * @property {boolean} regex
 * @property {boolean} noGitignore
 */

/**
 * Parse CLI arguments.
 * @param {string[]} argv
 * @returns {CliResult}
 */
function parseCliArgs(argv) {
  const args = argv.slice(2);
  /** @type {string[]} */
  const ruleArgs = [];
  /** @type {CliResult} */
  const result = {
    rules: [],
    root: ".",
    apply: false,
    regex: false,
    noGitignore: false,
  };

  for (let i = 0; i < args.length; i++) {
    if (args[i] === "--root") {
      result.root = args[++i] ?? ".";
    } else if (args[i] === "--apply") {
      result.apply = true;
    } else if (args[i] === "--regex") {
      result.regex = true;
    } else if (args[i] === "--no-gitignore") {
      result.noGitignore = true;
    } else if (!args[i].startsWith("--")) {
      ruleArgs.push(args[i]);
    }
  }

  result.rules = parseCliRules(ruleArgs);
  return result;
}

/**
 * Main CLI entry point.
 * @param {string[]} argv
 * @returns {number}
 */
function main(argv) {
  const cli = parseCliArgs(argv);
  if (cli.rules.length === 0) {
    console.log(
      'Usage: node scripts/rename.js "SOURCE=>TARGET" [...] [options]\n\n' +
        "Options:\n" +
        "  --root <dir>      Directory to scan (default: .)\n" +
        "  --apply           Actually rename (default: dry-run)\n" +
        "  --regex           Treat rules as regex substitutions\n" +
        "  --no-gitignore    Do not skip git-ignored paths\n\n" +
        "Examples:\n" +
        '  node scripts/rename.js "foo=>bar" --apply\n' +
        '  node scripts/rename.js "old_name=>new_name" --root ./src --apply',
    );
    return 1;
  }
  const [renamed, skipped] = renamePaths(cli.rules, {
    root: cli.root,
    dryRun: !cli.apply,
    regex: cli.regex,
    respectGitignore: !cli.noGitignore,
  });
  console.log(
    `\nDone. renamed=${renamed}, skipped=${skipped}, dry_run=${!cli.apply}`,
  );
  return 0;
}

if (
  process.argv[1] &&
  fileURLToPath(import.meta.url) === path.resolve(process.argv[1])
) {
  process.exit(main(process.argv));
}

export {
  applyLiteral,
  applyRegex,
  isGitRepo,
  gitIgnored,
  isUnder,
  collectPaths,
  buildPlans,
  execute,
  renamePaths,
  parseCliRules,
  parseCliArgs,
  main,
};
