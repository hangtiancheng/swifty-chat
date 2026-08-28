// Copyright (c) 2026 hangtiancheng
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in
// all copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
// SOFTWARE.

package tools

const BashDescription = `Execute a shell command and return stdout and stderr.

IMPORTANT: Avoid using this tool to run cat, head, tail, sed, awk, or echo commands. Instead use the dedicated ReadFile, EditFile, or WriteFile tools which provide a better experience.

Usage notes:
- The working directory persists between commands, but shell state does not.
- Always quote file paths containing spaces with double quotes.
- Try to maintain your current working directory using absolute paths; avoid cd unless the user explicitly requests it.
- Optional timeout in seconds (max 600). Default is 120s.
- When issuing multiple independent commands, make separate parallel tool calls instead of chaining with &&.
- Use && to chain sequential dependent commands. Use ; only when you don't care if earlier commands fail.
- DO NOT use newlines to separate commands.

Git Safety Protocol:
- NEVER run destructive git commands (push --force, reset --hard, checkout ., clean -f, branch -D) unless the user explicitly requests it.
- NEVER skip hooks (--no-verify) unless the user explicitly requests it.
- Prefer creating a new commit rather than amending an existing one.
- Before running destructive operations, consider safer alternatives.

Avoid unnecessary sleep commands. Do not retry failing commands in a sleep loop — diagnose the root cause instead.
When using find, search from "." or a specific path, not "/" — scanning the full filesystem is too expensive.`

const ReadFileDescription = `Read a file and return its contents with line numbers.

Usage notes:
- The file_path parameter should be an absolute path when possible.
- By default reads up to 2000 lines from the beginning of the file.
- Use offset and limit to read specific parts of large files. Only read what you need.
- Results are returned with line numbers (1-based) for easy reference.
- This tool can only read files, not directories. Use Glob to list directory contents.
- Do NOT re-read a file you just edited to verify — EditFile would have errored if the change failed.`

const EditFileDescription = `Replace an exact string in a file. The old_string must appear exactly once in the file.

Usage notes:
- You MUST read the file with ReadFile before editing. This tool will fail otherwise.
- When editing text from ReadFile output, preserve the exact indentation (tabs/spaces) as shown.
- ALWAYS prefer editing existing files over creating new ones.
- The edit will FAIL if old_string is not unique in the file. Provide more surrounding context to make it unique.
- Use the smallest old_string that is clearly unique — 2-4 adjacent lines is usually sufficient.
- The new_string must be different from old_string.`

const WriteFileDescription = `Write content to a file, creating parent directories if needed. Overwrites existing files.

Usage notes:
- If modifying an existing file, prefer EditFile over WriteFile — it only sends the diff.
- Use this tool only to create new files or for complete rewrites.
- You MUST read existing files with ReadFile before overwriting them.
- NEVER create documentation files (*.md) or README files unless explicitly requested.`

const GlobDescription = `Find files matching a glob pattern, returning relative paths sorted alphabetically.

Usage notes:
- Supports patterns like "**/*.py", "src/**/*.ts", "*.go".
- Search from "." or a specific path, never from "/".
- Automatically skips .git, node_modules, __pycache__, and similar directories.
- Use this instead of find or ls commands via Bash.`

const GrepDescription = `Search file contents using a regex pattern, returning file:line:content matches.

Usage notes:
- Supports full regex syntax (e.g., "log.*Error", "func\s+\w+").
- Filter files with the include parameter (e.g., "*.py", "*.go").
- Search from "." or a specific path, never from "/".
- Use this instead of grep or rg commands via Bash.
- Automatically skips .git, node_modules, __pycache__, and similar directories.`
