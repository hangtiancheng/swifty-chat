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
 * Wire shapes for the Swiftx control socket, plus the timeline model built from
 * them.
 *
 * The assistant's finished text arrives as ordinary chat messages, so this
 * socket only carries what a transcript cannot hold: token deltas, thinking,
 * tool calls and the prompts a run blocks on. Each of those is anchored to the
 * chat message it followed, which is what lets tool cards sit between two
 * replies instead of piling up at the end.
 */

/* Server → client */

export interface AgentConnected {
  model: string;
  streaming: boolean;
  /** False while the agent is still warming up: MCP servers, tools, context. */
  ready: boolean;
  anchorId: string;
  inputTokens: number;
  outputTokens: number;
  permissionMode: string;
}

export interface SlashCommand {
  name: string;
  description: string;
}

/** Tool arguments are opaque agent JSON; only a few known keys are previewed. */
export type ToolArgs = Record<string, unknown> | null;

export interface QuestionOption {
  label: string;
  description: string;
}

export interface Question {
  question: string;
  header: string;
  options: QuestionOption[];
  multiSelect: boolean;
}

export type AgentEvent =
  | { type: "connected"; data: AgentConnected }
  | { type: "ready"; data: null }
  | { type: "commands"; data: SlashCommand[] | null }
  | { type: "run_start"; data: { userMessageId: string } }
  | { type: "stream_text"; data: { text: string } }
  | { type: "stream_end"; data: { text: string; messageId: string } }
  | { type: "thinking_text"; data: { text: string } }
  | {
      type: "tool_use";
      data: { toolId: string; toolName: string; args: ToolArgs };
    }
  | {
      type: "tool_result";
      data: {
        toolId: string;
        toolName: string;
        output: string;
        isError: boolean;
        elapsed: number;
      };
    }
  | {
      type: "permission_request";
      data: { id: string; toolName: string; description: string };
    }
  | { type: "ask_user"; data: { id: string; questions: Question[] } }
  | { type: "turn_complete"; data: { turn: number } }
  | { type: "loop_complete"; data: { totalTurns: number; elapsed: number } }
  | { type: "usage"; data: { inputTokens: number; outputTokens: number } }
  | { type: "system"; data: { message: string } }
  | { type: "error"; data: { message: string } }
  | { type: "compact"; data: { message: string } }
  | { type: "retry"; data: { reason: string; waitMs: number } }
  | { type: "context_cleared"; data: null }
  | { type: "command_done"; data: null }
  | { type: "pong"; data: null };

/* Client → server */

export type PermissionResponse = "allow" | "deny" | "allowAlways";

export type AgentCommand =
  | {
      type: "permission_response";
      data: { id: string; response: PermissionResponse };
    }
  | {
      type: "ask_user_response";
      data: { id: string; answers: Record<string, string> };
    }
  | { type: "cancel"; data: null }
  | { type: "ping"; data: null };

/* Timeline model */

export type AgentConnectionStatus =
  "idle" | "connecting" | "connected" | "reconnecting";

export type ToolStatus = "running" | "ok" | "error";

/** Every item remembers the chat message it came after, so the transcript and
 * this overlay stay interleaved in the order things actually happened. */
interface Anchored {
  id: string;
  anchorId: string;
}

export interface AgentStreamItem extends Anchored {
  kind: "stream";
  content: string;
  streaming: boolean;
  /** Once set, the chat message with this uuid replaces the live bubble. */
  messageId: string;
}

export interface AgentThinkingItem extends Anchored {
  kind: "thinking";
  content: string;
  done: boolean;
}

export interface AgentToolItem extends Anchored {
  kind: "tool";
  toolId: string;
  toolName: string;
  args: ToolArgs;
  status: ToolStatus;
  output: string;
  elapsed: number;
}

export interface AgentPermissionItem extends Anchored {
  kind: "permission";
  toolName: string;
  description: string;
  response: PermissionResponse | null;
}

export interface AgentQuestionItem extends Anchored {
  kind: "question";
  questions: Question[];
  answered: boolean;
}

export interface AgentNoticeItem extends Anchored {
  kind: "notice";
  tone: "info" | "error" | "done";
  content: string;
}

export type AgentItem =
  | AgentStreamItem
  | AgentThinkingItem
  | AgentToolItem
  | AgentPermissionItem
  | AgentQuestionItem
  | AgentNoticeItem;

/** tool_use and tool_result are matched on this pair, not on arrival order. */
export const toolKey = (toolName: string, toolId: string) =>
  `${toolName}_${toolId}`;
