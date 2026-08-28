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
/**
 * Deferred Loading Token Savings Benchmark.
 * Compares token consumption between full loading and deferred loading
 * using real API usage.prompt_tokens values.
 *
 * Ported from benchmark_token_count.py. Requires Node 18+ (global fetch).
 */

/**
 * Minimal JSON Schema shape used by tool parameter definitions.
 * @typedef {Object} JsonSchema
 * @property {string} [type]
 * @property {string[]} [required]
 * @property {Record<string, JsonSchema>} [properties]
 * @property {JsonSchema} [items]
 * @property {string[]} [enum]
 * @property {string} [description]
 */

/**
 * @typedef {Object} FunctionDef
 * @property {string} name
 * @property {string} description
 * @property {JsonSchema} parameters
 */

/**
 * @typedef {Object} Tool
 * @property {"function"} type
 * @property {FunctionDef} function
 */

/**
 * @typedef {Object} McpTemplate
 * @property {string} name
 * @property {string} desc
 * @property {Record<string, JsonSchema>} params
 */

/**
 * @typedef {Object} Message
 * @property {string} role
 * @property {string} content
 */

/**
 * @typedef {Object} Usage
 * @property {number} prompt_tokens
 * @property {number} [completion_tokens]
 * @property {number} [total_tokens]
 */

/**
 * @typedef {Object} ApiResponse
 * @property {Usage} [usage]
 * @property {string} [error]
 * @property {Object} [choices]
 */

const API_KEY = "sk-";
const BASE_URL = "https://api.minimaxi.com/v1";
const MODEL = "MiniMax-M3";

/** @type {Tool[]} */
const BUILTIN_TOOLS = [
  {
    type: "function",
    function: {
      name: "ReadFile",
      description: "Read a file from the local filesystem.",
      parameters: {
        type: "object",
        required: ["file_path"],
        properties: {
          file_path: {
            type: "string",
            description: "The absolute path to the file to read",
          },
          offset: {
            type: "integer",
            description: "Line number to start reading from",
          },
          limit: { type: "integer", description: "Number of lines to read" },
        },
      },
    },
  },
  {
    type: "function",
    function: {
      name: "WriteFile",
      description:
        "Write content to a file, creating it if needed or overwriting existing content.",
      parameters: {
        type: "object",
        required: ["file_path", "content"],
        properties: {
          file_path: {
            type: "string",
            description: "The absolute path to the file to write",
          },
          content: {
            type: "string",
            description: "The content to write to the file",
          },
        },
      },
    },
  },
  {
    type: "function",
    function: {
      name: "EditFile",
      description:
        "Make targeted edits to a file by replacing specific text with new text.",
      parameters: {
        type: "object",
        required: ["file_path", "old_string", "new_string"],
        properties: {
          file_path: {
            type: "string",
            description: "The absolute path to the file to modify",
          },
          old_string: { type: "string", description: "The text to replace" },
          new_string: { type: "string", description: "The replacement text" },
          replace_all: {
            type: "boolean",
            description: "Replace all occurrences",
          },
        },
      },
    },
  },
  {
    type: "function",
    function: {
      name: "Bash",
      description: "Execute a bash command and return its output.",
      parameters: {
        type: "object",
        required: ["command"],
        properties: {
          command: { type: "string", description: "The command to execute" },
          timeout: { type: "integer", description: "Timeout in milliseconds" },
        },
      },
    },
  },
  {
    type: "function",
    function: {
      name: "Glob",
      description:
        "Find files matching a glob pattern in the project directory.",
      parameters: {
        type: "object",
        required: ["pattern"],
        properties: {
          pattern: {
            type: "string",
            description: "Glob pattern to match files",
          },
          path: {
            type: "string",
            description: "Base directory to search from",
          },
        },
      },
    },
  },
  {
    type: "function",
    function: {
      name: "Grep",
      description: "Search file contents using regular expressions.",
      parameters: {
        type: "object",
        required: ["pattern"],
        properties: {
          pattern: {
            type: "string",
            description: "Regular expression pattern to search for",
          },
          path: {
            type: "string",
            description: "Directory or file to search in",
          },
          include: { type: "string", description: "File pattern to include" },
        },
      },
    },
  },
];

/** @type {Tool} */
const TOOL_SEARCH = {
  type: "function",
  function: {
    name: "ToolSearch",
    description:
      'Search for and load additional tools that are not immediately available. Some tools are deferred (not loaded by default) to save context space. Use this tool to discover and load them.\n\nQuery forms:\n- "select:ToolName,AnotherTool" — fetch exact tools by name\n- "keyword search" — keyword search, returns up to max_results matches',
    parameters: {
      type: "object",
      required: ["query"],
      properties: {
        query: {
          type: "string",
          description:
            'Query to find deferred tools. Use "select:Name1,Name2" for direct selection, or keywords to search.',
        },
        max_results: {
          type: "integer",
          description: "Maximum results to return (default: 5)",
        },
      },
    },
  },
};

// 6 real MCP tool templates, cycled to generate 58 tools.
/** @type {McpTemplate[]} */
const MCP_TEMPLATES = [
  {
    name: "mcp__grafana__query_prometheus_{i:03d}",
    desc: "Execute a PromQL query against the specified Prometheus datasource and return time-series or instant results. Supports range queries with configurable step and time window.",
    params: {
      expr: {
        type: "string",
        description:
          "PromQL query expression to evaluate against the datasource",
      },
      datasource: {
        type: "string",
        description: "Name or UID of the Prometheus datasource to query",
      },
      start: {
        type: "string",
        description:
          "Start of the time range in RFC3339 format or relative (e.g. 'now-1h')",
      },
      end: {
        type: "string",
        description:
          "End of the time range in RFC3339 format or relative (e.g. 'now')",
      },
      step: {
        type: "string",
        description:
          "Query resolution step width in Prometheus duration format (e.g. '15s', '1m')",
      },
      format: {
        type: "string",
        description: "Output format for results",
        enum: ["table", "timeseries", "json"],
      },
      max_results: {
        type: "integer",
        description: "Maximum number of time series to return",
      },
      legend: {
        type: "string",
        description: "Legend format template for result series names",
      },
    },
  },
  {
    name: "mcp__grafana__search_dashboards_{i:03d}",
    desc: "Search for Grafana dashboards by title, tag, or folder. Returns matching dashboards with metadata including UID, title, URL, folder, and tags.",
    params: {
      query: {
        type: "string",
        description: "Search query string to match against dashboard titles",
      },
      tag: {
        type: "array",
        items: { type: "string" },
        description: "Filter dashboards by tags (AND logic)",
      },
      folder: {
        type: "string",
        description: "Folder title or UID to restrict search scope",
      },
      starred: {
        type: "boolean",
        description: "If true, only return starred dashboards",
      },
      limit: {
        type: "integer",
        description: "Maximum number of dashboards to return",
      },
      sort: {
        type: "string",
        description: "Sort order for results",
        enum: ["alpha-asc", "alpha-desc", "created-asc", "created-desc"],
      },
    },
  },
  {
    name: "mcp__playwright__browser_click_{i:03d}",
    desc: "Click an element on the page identified by a CSS selector or accessible role. Supports options for button type, click count, position offset, force click, and timeout.",
    params: {
      selector: {
        type: "string",
        description:
          "CSS selector, XPath, or text selector to identify the target element",
      },
      button: {
        type: "string",
        description: "Mouse button to click",
        enum: ["left", "right", "middle"],
      },
      clickCount: {
        type: "integer",
        description: "Number of clicks (1 for single, 2 for double)",
      },
      force: {
        type: "boolean",
        description:
          "Whether to bypass actionability checks and force the click",
      },
      timeout: {
        type: "integer",
        description: "Maximum time in milliseconds to wait for the element",
      },
      position: {
        type: "object",
        description: "Offset position relative to element's top-left corner",
        properties: { x: { type: "number" }, y: { type: "number" } },
      },
      modifiers: {
        type: "array",
        items: { type: "string", enum: ["Alt", "Control", "Meta", "Shift"] },
        description: "Keyboard modifiers to press during click",
      },
    },
  },
  {
    name: "mcp__grafana__query_loki_{i:03d}",
    desc: "Run a LogQL query against a Loki datasource and return matching log lines or metric results. Supports log queries, metric queries, and pattern-based aggregation.",
    params: {
      query: {
        type: "string",
        description: "LogQL query expression to execute",
      },
      datasource: {
        type: "string",
        description: "Name or UID of the Loki datasource",
      },
      start: {
        type: "string",
        description: "Start timestamp in RFC3339 or relative format",
      },
      end: {
        type: "string",
        description: "End timestamp in RFC3339 or relative format",
      },
      limit: {
        type: "integer",
        description: "Maximum number of log entries to return",
      },
      direction: {
        type: "string",
        description: "Log ordering direction",
        enum: ["forward", "backward"],
      },
      step: { type: "string", description: "Step interval for metric queries" },
      dedup: {
        type: "boolean",
        description: "Whether to deduplicate log lines with same content",
      },
    },
  },
  {
    name: "mcp__playwright__browser_fill_{i:03d}",
    desc: "Fill an input field with text. Clears existing content before typing. Works with input, textarea, and content editable elements. Dispatches input and change events.",
    params: {
      selector: {
        type: "string",
        description:
          "CSS selector or text selector for the input element to fill",
      },
      value: {
        type: "string",
        description: "Text value to fill into the input field",
      },
      force: {
        type: "boolean",
        description: "Whether to bypass actionability checks",
      },
      timeout: {
        type: "integer",
        description: "Maximum time in milliseconds to wait for element",
      },
      noWaitAfter: {
        type: "boolean",
        description: "If true, do not wait for navigation events after filling",
      },
    },
  },
  {
    name: "mcp__grafana__create_annotation_{i:03d}",
    desc: "Create an annotation on a Grafana dashboard panel or at the global level. Annotations mark important events on time-series graphs with optional tags and rich text descriptions.",
    params: {
      dashboardUID: {
        type: "string",
        description:
          "UID of the dashboard to annotate (omit for global annotation)",
      },
      panelId: {
        type: "integer",
        description: "Panel ID within the dashboard to annotate",
      },
      time: {
        type: "integer",
        description: "Unix timestamp in milliseconds for annotation start",
      },
      timeEnd: {
        type: "integer",
        description:
          "Unix timestamp in milliseconds for annotation end (for range annotations)",
      },
      text: {
        type: "string",
        description:
          "Annotation description text, supports basic HTML formatting",
      },
      tags: {
        type: "array",
        items: { type: "string" },
        description: "Tags to associate with the annotation for filtering",
      },
    },
  },
];

/**
 * Build a single MCP tool from a template, filling the index into the name.
 * @param {McpTemplate} template
 * @param {number} i
 * @returns {Tool}
 */
function makeMcpTool(template, i) {
  return {
    type: "function",
    function: {
      name: template.name.replace("{i:03d}", String(i).padStart(3, "0")),
      description: template.desc,
      parameters: {
        type: "object",
        required: ["query", "datasource"],
        properties: template.params,
      },
    },
  };
}

/**
 * Generate `n` MCP tools by cycling through the templates.
 * @param {number} [n=58]
 * @returns {Tool[]}
 */
function makeAllMcpTools(n = 58) {
  return Array.from({ length: n }, (_, i) =>
    makeMcpTool(MCP_TEMPLATES[i % MCP_TEMPLATES.length], i),
  );
}

/**
 * Build the system-reminder listing deferred tool names.
 * @param {string[]} toolNames
 * @returns {string}
 */
function makeDeferredReminder(toolNames) {
  return (
    "The following deferred tools are available via ToolSearch. " +
    "Their schemas are NOT loaded - use ToolSearch with query " +
    '"select:<name>[,<name>...]" to load tool schemas before calling them:\n' +
    toolNames.join("\n")
  );
}

/**
 * Call the chat completions API and return the usage object, or null on error.
 * @param {Message[]} messages
 * @param {Tool[]} tools
 * @param {string} [system]
 * @returns {Promise<Usage | null>}
 */
async function callApi(messages, tools, system) {
  /** @type {Message[]} */
  const msgs = system
    ? [{ role: "system", content: system }, ...messages]
    : messages;
  const body = { model: MODEL, messages: msgs, tools, max_tokens: 1 };
  const resp = await fetch(`${BASE_URL}/chat/completions`, {
    method: "POST",
    headers: {
      Authorization: `Bearer ${API_KEY}`,
      "Content-Type": "application/json",
    },
    body: JSON.stringify(body),
    signal: AbortSignal.timeout(30000),
  });
  const data = /** @type {ApiResponse} */ (await resp.json());
  if (!data.usage) {
    console.error(`ERROR: ${JSON.stringify(data, null, 2)}`);
    return null;
  }
  return data.usage;
}

/**
 * Run the three-scenario benchmark and print the full-session savings.
 * @returns {Promise<void>}
 */
async function main() {
  const mcpTools = makeAllMcpTools(58);
  const mcpNames = mcpTools.map((t) => t.function.name);

  /** @type {Message[]} */
  const userMsg = [
    {
      role: "user",
      content:
        "Check which services in Grafana have had CPU usage above 80% in the last hour.",
    },
  ];

  console.log("=".repeat(70));
  console.log(
    "Deferred Loading Token Savings Benchmark (Real API token counts)",
  );
  console.log(`Model: ${MODEL} | Built-in tools: 6 | MCP tools: 58`);
  console.log("=".repeat(70));

  // --- Scenario 1: Full loading (all 64 tools passed in) ---
  const toolsFull = [...BUILTIN_TOOLS, ...mcpTools];
  console.log(
    `\n[Scenario 1] Full loading: all ${toolsFull.length} tools passed to the tools parameter`,
  );
  const usageFull = await callApi(userMsg, toolsFull);
  if (!usageFull) return;
  const tokensFull = usageFull.prompt_tokens;
  console.log(`  prompt_tokens = ${tokensFull}`);

  // --- Scenario 2: Deferred loading (7 tools + system-reminder listing names) ---
  const toolsDeferred = [...BUILTIN_TOOLS, TOOL_SEARCH];
  const reminder = makeDeferredReminder(mcpNames);
  console.log(
    `\n[Scenario 2] Deferred loading: ${toolsDeferred.length} tools + system-reminder listing 58 deferred tool names`,
  );
  const usageDeferred = await callApi(userMsg, toolsDeferred, reminder);
  if (!usageDeferred) return;
  const tokensDeferred = usageDeferred.prompt_tokens;
  console.log(`  prompt_tokens = ${tokensDeferred}`);

  // --- Scenario 3: Deferred loading + 2 tools already activated ---
  const activated = [mcpTools[5], mcpTools[20]];
  const activatedNames = activated.map((t) => t.function.name);
  const remainingNames = mcpNames.filter((n) => !activatedNames.includes(n));
  const toolsPartial = [...BUILTIN_TOOLS, TOOL_SEARCH, ...activated];
  const reminderPartial = makeDeferredReminder(remainingNames);
  console.log(
    `\n[Scenario 3] Deferred loading + 2 activated: ${toolsPartial.length} tools + system-reminder listing 56 deferred tool names`,
  );
  const usagePartial = await callApi(userMsg, toolsPartial, reminderPartial);
  if (!usagePartial) return;
  const tokensPartial = usagePartial.prompt_tokens;
  console.log(`  prompt_tokens = ${tokensPartial}`);

  // --- Full session comparison (10 turns) ---
  // Full loading: each turn costs tokensFull.
  // Deferred: turns 1-2 cost tokensDeferred, turns 3-10 cost tokensPartial.
  const totalFull = tokensFull * 10;
  const totalDeferred = tokensDeferred * 2 + tokensPartial * 8;
  const savings = 1 - totalDeferred / totalFull;

  console.log(`\n${"=".repeat(70)}`);
  console.log("Full Session Statistics (10 turns)");
  console.log("=".repeat(70));
  console.log(
    `  Full loading:     ${String(tokensFull).padStart(8)} tokens/turn x 10 = ${String(totalFull).padStart(8)} tokens`,
  );
  console.log(
    `  Deferred loading: ${String(tokensDeferred).padStart(8)} tokens x 2 + ${String(tokensPartial).padStart(8)} tokens x 8 = ${String(totalDeferred).padStart(8)} tokens`,
  );
  console.log(
    `  Savings:          ${(savings * 100).toFixed(1)}% (${totalFull - totalDeferred} tokens saved)`,
  );
  console.log("\nPer-turn comparison:");
  console.log(
    `  Full vs deactivated:    ${tokensFull} vs ${tokensDeferred}  (savings ${((1 - tokensDeferred / tokensFull) * 100).toFixed(1)}%)`,
  );
  console.log(
    `  Full vs 2 activated:    ${tokensFull} vs ${tokensPartial}  (savings ${((1 - tokensPartial / tokensFull) * 100).toFixed(1)}%)`,
  );
}

main();
