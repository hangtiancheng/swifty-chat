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

import { create } from "zustand";

import { wsUrl } from "@/env";
import {
  toolKey,
  type AgentCommand,
  type AgentEvent,
  type AgentItem,
  type AgentConnectionStatus,
  type PermissionResponse,
  type SlashCommand,
} from "@/service/agent-schemas";
import useAuthStore from "./auth";

/** Live progress for the Swiftx thread. Prompts and finished replies travel the
 * chat socket; this store holds only what the transcript cannot: the streaming
 * bubble, thinking, tool cards and the prompts a run is waiting on. */
export interface AgentState {
  status: AgentConnectionStatus;
  /** False while the agent warms up; a prompt sent now queues until it is true. */
  ready: boolean;
  items: AgentItem[];
  commands: SlashCommand[];
  usage: { inputTokens: number; outputTokens: number } | null;
  streaming: boolean;
  model: string;
  /** Chat message new items are placed after. */
  anchorId: string;
  currentStreamId: string | null;
  currentThinkingId: string | null;

  connect: () => void;
  disconnect: () => void;
  respondPermission: (id: string, response: PermissionResponse) => void;
  answerQuestions: (id: string, answers: Record<string, string>) => void;
  stop: () => void;
}

const INITIAL_RECONNECT_DELAY = 1000;
const MAX_RECONNECT_DELAY = 30_000;
const PING_INTERVAL = 10_000;

let socket: WebSocket | null = null;
let reconnectTimer: ReturnType<typeof setTimeout> | null = null;
let pingTimer: ReturnType<typeof setInterval> | null = null;
let reconnectDelay = INITIAL_RECONNECT_DELAY;
let intentionalClose = false;
let itemCounter = 0;

function nextId(prefix: string): string {
  itemCounter += 1;
  return `${prefix}_${itemCounter}`;
}

function send(command: AgentCommand) {
  if (socket?.readyState === WebSocket.OPEN) {
    socket.send(JSON.stringify(command));
  }
}

type Snapshot = Omit<
  AgentState,
  "connect" | "disconnect" | "respondPermission" | "answerQuestions" | "stop"
>;

const emptySnapshot: Snapshot = {
  status: "idle",
  ready: false,
  items: [],
  commands: [],
  usage: null,
  streaming: false,
  model: "",
  anchorId: "",
  currentStreamId: null,
  currentThinkingId: null,
};

function withItem(state: Snapshot, item: AgentItem): Snapshot {
  return { ...state, items: [...state.items, item] };
}

function notice(
  state: Snapshot,
  tone: "info" | "error" | "done",
  content: string,
): Snapshot {
  // A failing socket re-reports the same reason on every retry, so an
  // identical repeat of the previous line is dropped instead of stacking up.
  const last = state.items.at(-1);
  if (
    last?.kind === "notice" &&
    last.tone === tone &&
    last.content === content
  ) {
    return state;
  }
  return withItem(state, {
    kind: "notice",
    id: nextId("notice"),
    anchorId: state.anchorId,
    tone,
    content,
  });
}

function finalizeThinking(state: Snapshot): Snapshot {
  const id = state.currentThinkingId;
  if (id === null) return state;
  return {
    ...state,
    currentThinkingId: null,
    items: state.items.map((item) =>
      item.kind === "thinking" && item.id === id
        ? { ...item, done: true }
        : item,
    ),
  };
}

/** A run that ended can no longer accept answers — the server fails leftover
 * prompts (deny / empty answers), so their cards settle instead of staying
 * clickable forever. */
function settlePrompts(state: Snapshot): Snapshot {
  return {
    ...state,
    items: state.items.map((item) => {
      if (item.kind === "permission" && item.response === null) {
        return { ...item, response: "deny" as const };
      }
      if (item.kind === "question" && !item.answered) {
        return { ...item, answered: true };
      }
      return item;
    }),
  };
}

function apply(state: Snapshot, event: AgentEvent): Snapshot {
  switch (event.type) {
    case "connected":
      return {
        ...state,
        model: event.data.model,
        streaming: event.data.streaming,
        ready: event.data.ready,
        anchorId: event.data.anchorId || state.anchorId,
        usage:
          event.data.inputTokens || event.data.outputTokens
            ? {
                inputTokens: event.data.inputTokens,
                outputTokens: event.data.outputTokens,
              }
            : state.usage,
      };

    case "ready":
      return { ...state, ready: true };

    case "commands":
      return { ...state, commands: event.data ?? [] };

    case "run_start":
      return { ...state, streaming: true, anchorId: event.data.userMessageId };

    case "thinking_text": {
      const id = state.currentThinkingId;
      if (id === null) {
        const created = nextId("think");
        return withItem(
          { ...state, currentThinkingId: created },
          {
            kind: "thinking",
            id: created,
            anchorId: state.anchorId,
            content: event.data.text,
            done: false,
          },
        );
      }
      return {
        ...state,
        items: state.items.map((item) =>
          item.kind === "thinking" && item.id === id
            ? { ...item, content: item.content + event.data.text }
            : item,
        ),
      };
    }

    case "stream_text": {
      const next = finalizeThinking(state);
      const id = next.currentStreamId;
      if (id === null) {
        const created = nextId("stream");
        return withItem(
          { ...next, currentStreamId: created },
          {
            kind: "stream",
            id: created,
            anchorId: next.anchorId,
            content: event.data.text,
            streaming: true,
            messageId: "",
          },
        );
      }
      return {
        ...next,
        items: next.items.map((item) =>
          item.kind === "stream" && item.id === id
            ? { ...item, content: item.content + event.data.text }
            : item,
        ),
      };
    }

    case "stream_end": {
      const { messageId, text } = event.data;
      // The stored message takes over from here, so later items belong after
      // it rather than after the prompt.
      const anchorId = messageId || state.anchorId;
      const id = state.currentStreamId;
      if (id === null) {
        // Nothing was streamed into a bubble — only worth showing when the
        // text never made it into the transcript.
        if (messageId) return { ...state, anchorId };
        return withItem(
          { ...state, anchorId },
          {
            kind: "stream",
            id: nextId("stream"),
            anchorId: state.anchorId,
            content: text,
            streaming: false,
            messageId: "",
          },
        );
      }
      return {
        ...state,
        anchorId,
        currentStreamId: null,
        items: state.items.map((item) =>
          item.kind === "stream" && item.id === id
            ? { ...item, streaming: false, messageId }
            : item,
        ),
      };
    }

    case "tool_use": {
      const next = finalizeThinking(state);
      const key = toolKey(event.data.toolName, event.data.toolId);
      const known = next.items.some(
        (item) =>
          item.kind === "tool" && toolKey(item.toolName, item.toolId) === key,
      );
      if (known) {
        // A call is announced twice: once when the model starts emitting it,
        // and again once its arguments have been parsed. That second
        // announcement is the only place the args ever arrive.
        if (!event.data.args) return next;
        return {
          ...next,
          items: next.items.map((item) =>
            item.kind === "tool" && toolKey(item.toolName, item.toolId) === key
              ? { ...item, args: event.data.args }
              : item,
          ),
        };
      }
      return withItem(next, {
        kind: "tool",
        id: nextId("tool"),
        anchorId: next.anchorId,
        toolId: event.data.toolId,
        toolName: event.data.toolName,
        args: event.data.args,
        status: "running",
        output: "",
        elapsed: 0,
      });
    }

    case "tool_result": {
      const key = toolKey(event.data.toolName, event.data.toolId);
      let matched = false;
      const items = state.items.map((item) => {
        if (
          item.kind !== "tool" ||
          toolKey(item.toolName, item.toolId) !== key
        ) {
          return item;
        }
        matched = true;
        return {
          ...item,
          status: event.data.isError ? ("error" as const) : ("ok" as const),
          output: event.data.output,
          elapsed: event.data.elapsed,
        };
      });
      if (matched) return { ...state, items };
      return withItem(state, {
        kind: "tool",
        id: nextId("tool"),
        anchorId: state.anchorId,
        toolId: event.data.toolId,
        toolName: event.data.toolName,
        args: null,
        status: event.data.isError ? "error" : "ok",
        output: event.data.output,
        elapsed: event.data.elapsed,
      });
    }

    case "permission_request":
      return withItem(state, {
        kind: "permission",
        id: event.data.id,
        anchorId: state.anchorId,
        toolName: event.data.toolName,
        description: event.data.description,
        response: null,
      });

    case "ask_user":
      return withItem(state, {
        kind: "question",
        id: event.data.id,
        anchorId: state.anchorId,
        questions: event.data.questions ?? [],
        answered: false,
      });

    case "loop_complete": {
      const next = settlePrompts(finalizeThinking(state));
      return notice(
        { ...next, streaming: false },
        "done",
        `Done in ${event.data.elapsed.toFixed(1)}s`,
      );
    }

    case "usage":
      return { ...state, usage: event.data };

    case "system":
      return notice(state, "info", event.data.message);

    case "error":
      return notice(
        { ...settlePrompts(state), streaming: false },
        "error",
        event.data.message,
      );

    case "compact":
      return notice(state, "info", `⟳ ${event.data.message}`);

    case "retry":
      return notice(state, "info", `↻ Retrying: ${event.data.reason}`);

    case "context_cleared":
      return notice(
        state,
        "info",
        "Context cleared — Swiftx starts fresh from here. Your history above is untouched.",
      );

    case "command_done":
      return { ...settlePrompts(state), streaming: false };

    case "turn_complete":
    case "pong":
      return state;
  }
}

function handleFrame(raw: unknown) {
  if (typeof raw !== "string") return;
  let payload: unknown;
  try {
    payload = JSON.parse(raw);
  } catch {
    return;
  }
  if (
    typeof payload !== "object" ||
    payload === null ||
    typeof (payload as { type?: unknown }).type !== "string"
  ) {
    return;
  }
  useAgentStore.setState((state) => apply(state, payload as AgentEvent));
}

function clearTimers() {
  if (reconnectTimer) {
    clearTimeout(reconnectTimer);
    reconnectTimer = null;
  }
  if (pingTimer) {
    clearInterval(pingTimer);
    pingTimer = null;
  }
}

function openSocket() {
  const { token } = useAuthStore.getState();
  if (!token) {
    useAgentStore.setState({ status: "idle" });
    return;
  }

  if (socket) {
    socket.onclose = null;
    socket.close();
  }

  const next = new WebSocket(
    `${wsUrl}/agent/ws?token=${encodeURIComponent(token)}`,
  );
  next.onopen = () => {
    reconnectDelay = INITIAL_RECONNECT_DELAY;
    useAgentStore.setState({ status: "connected" });
    // The server answers with pong; the round trip keeps proxies from idling
    // the socket out during a long tool call.
    pingTimer = setInterval(
      () => send({ type: "ping", data: null }),
      PING_INTERVAL,
    );
  };
  next.onmessage = (event: MessageEvent) => handleFrame(event.data);
  next.onclose = () => {
    socket = null;
    if (pingTimer) {
      clearInterval(pingTimer);
      pingTimer = null;
    }
    if (intentionalClose) {
      useAgentStore.setState({ status: "idle" });
      return;
    }
    useAgentStore.setState({ status: "reconnecting" });
    reconnectTimer = setTimeout(() => {
      reconnectDelay = Math.min(reconnectDelay * 2, MAX_RECONNECT_DELAY);
      openSocket();
    }, reconnectDelay);
  };
  next.onerror = () => next.close();
  socket = next;
}

const useAgentStore = create<AgentState>(() => ({
  ...emptySnapshot,

  connect() {
    if (socket) return;
    intentionalClose = false;
    reconnectDelay = INITIAL_RECONNECT_DELAY;
    clearTimers();
    // Progress is only meaningful next to the transcript it belongs to, so a
    // fresh visit starts from an empty overlay.
    useAgentStore.setState({ ...emptySnapshot, status: "connecting" });
    openSocket();
  },

  disconnect() {
    intentionalClose = true;
    clearTimers();
    if (socket) {
      socket.onclose = null;
      socket.close();
      socket = null;
    }
    useAgentStore.setState({ ...emptySnapshot });
  },

  respondPermission(id, response) {
    send({ type: "permission_response", data: { id, response } });
    useAgentStore.setState((state) => ({
      items: state.items.map((item) =>
        item.kind === "permission" && item.id === id
          ? { ...item, response }
          : item,
      ),
    }));
  },

  answerQuestions(id, answers) {
    send({ type: "ask_user_response", data: { id, answers } });
    useAgentStore.setState((state) => ({
      items: state.items.map((item) =>
        item.kind === "question" && item.id === id
          ? { ...item, answered: true }
          : item,
      ),
    }));
  },

  stop() {
    send({ type: "cancel", data: null });
  },
}));

export default useAgentStore;
