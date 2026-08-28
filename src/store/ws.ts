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

import { uniqBy } from "es-toolkit";
import { create } from "zustand";

import { wsUrl } from "@/env";
import { queryClient } from "@/lib/query-client";
import { keys } from "@/service/queries";
import {
  MessageType,
  SYSTEM_SENDER,
  SystemTopic,
  isGroupId,
  messageSchema,
  type Message,
  type OutgoingFrame,
} from "@/service/schemas";
import { showToast } from "@/utils/toast";
import useAuthStore from "./auth";

export type ConnectionStatus = "disconnected" | "connecting" | "connected";

/** `type: 3` frames carry WebRTC signalling; the call UI subscribes to them. */
export type SignalListener = (frame: Message) => void;

export interface WsState {
  status: ConnectionStatus;
  connect: (userId: string) => void;
  disconnect: () => void;
  send: (frame: OutgoingFrame) => void;
  subscribeToSignals: (listener: SignalListener) => () => void;
}

/** The server rejects a frame when its outbound queue is full. */
const OVERFLOW_TYPE = -1;
const INITIAL_RECONNECT_DELAY = 1000;
const MAX_RECONNECT_DELAY = 30_000;

let socket: WebSocket | null = null;
let reconnectTimer: ReturnType<typeof setTimeout> | null = null;
let reconnectDelay = INITIAL_RECONNECT_DELAY;
let intentionalClose = false;
let reconnecting = false;
const signalListeners = new Set<SignalListener>();

/** A system frame names the list that went stale rather than carrying new data. */
const staleKeysByTopic: Record<string, ReadonlyArray<readonly unknown[]>> = {
  [SystemTopic.Session]: [keys.sessions.all],
  [SystemTopic.Contact]: [keys.contacts.all],
  [SystemTopic.Apply]: [keys.contacts.all],
  [SystemTopic.Group]: [keys.groups.all, keys.contacts.all],
  [SystemTopic.Online]: [keys.chatroom.online, keys.contacts.all],
};

function invalidate(queryKey: readonly unknown[]) {
  void queryClient.invalidateQueries({ queryKey });
}

function dispatchSignal(frame: Message) {
  for (const listener of signalListeners) {
    listener(frame);
  }
}

/** Direct messages are keyed by the peer, group messages by the group. */
function conversationIdOf(frame: Message, selfId: string): string {
  if (isGroupId(frame.receive_id)) return frame.receive_id;
  return frame.send_id === selfId ? frame.receive_id : frame.send_id;
}

function appendToConversation(frame: Message) {
  const selfId = useAuthStore.getState().userInfo.uuid;
  if (!selfId) return;

  const queryKey = keys.messages.with(selfId, conversationIdOf(frame, selfId));
  // Writing to a conversation that was never opened would mark it fresh and
  // suppress the real fetch, so only patch transcripts already in the cache.
  if (queryClient.getQueryState(queryKey)) {
    queryClient.setQueryData<Message[]>(queryKey, (current) =>
      uniqBy([...(current ?? []), frame], (message) => message.uuid),
    );
  }
  invalidate(keys.sessions.all);
}

function handleFrame(raw: unknown) {
  // The server greets new clients with a plain-text line.
  if (typeof raw !== "string" || !raw.startsWith("{")) return;

  let payload: unknown;
  try {
    payload = JSON.parse(raw);
  } catch {
    return;
  }

  const parsed = messageSchema.safeParse(payload);
  if (!parsed.success) return;
  const frame = parsed.data;

  if (frame.type === OVERFLOW_TYPE) {
    showToast(frame.content || "Message send failed, please retry", "warning");
    return;
  }
  // `call_failed` arrives as SYSTEM with type 3, so signalling has to be
  // dispatched before the system-topic branch would swallow it.
  if (frame.type === MessageType.AvSignal) {
    dispatchSignal(frame);
    return;
  }
  if (frame.send_id === SYSTEM_SENDER) {
    for (const queryKey of staleKeysByTopic[frame.content] ?? []) {
      invalidate(queryKey);
    }
    return;
  }
  appendToConversation(frame);
}

function scheduleReconnect(userId: string) {
  if (intentionalClose) return;
  reconnectTimer = setTimeout(() => {
    reconnectDelay = Math.min(reconnectDelay * 2, MAX_RECONNECT_DELAY);
    openSocket(userId);
  }, reconnectDelay);
}

function openSocket(userId: string) {
  if (socket) {
    socket.onclose = null;
    socket.close();
  }

  // The server derives the identity from this token; a browser cannot set
  // headers on a handshake, so it travels in the query string.
  const { token } = useAuthStore.getState();
  if (!token) {
    useWsStore.setState({ status: "disconnected" });
    return;
  }

  useWsStore.setState({ status: "connecting" });

  const next = new WebSocket(
    `${wsUrl}/wss?client_id=${encodeURIComponent(userId)}&token=${encodeURIComponent(token)}`,
  );
  next.onopen = () => {
    reconnectDelay = INITIAL_RECONNECT_DELAY;
    useWsStore.setState({ status: "connected" });
    // Anything could have changed while the socket was down.
    if (reconnecting) {
      reconnecting = false;
      void queryClient.invalidateQueries();
    }
  };
  next.onmessage = (event: MessageEvent) => handleFrame(event.data);
  next.onclose = () => {
    socket = null;
    reconnecting = true;
    useWsStore.setState({ status: "disconnected" });
    scheduleReconnect(userId);
  };
  next.onerror = () => next.close();
  socket = next;
}

function clearReconnectTimer() {
  if (reconnectTimer) {
    clearTimeout(reconnectTimer);
    reconnectTimer = null;
  }
}

const useWsStore = create<WsState>(() => ({
  status: "disconnected",

  connect(userId: string) {
    if (!userId) return;
    intentionalClose = false;
    reconnecting = false;
    reconnectDelay = INITIAL_RECONNECT_DELAY;
    clearReconnectTimer();
    openSocket(userId);
  },

  disconnect() {
    intentionalClose = true;
    clearReconnectTimer();
    if (socket) {
      socket.onclose = null;
      socket.close();
      socket = null;
    }
    useWsStore.setState({ status: "disconnected" });
  },

  send(frame: OutgoingFrame) {
    if (socket?.readyState === WebSocket.OPEN) {
      socket.send(JSON.stringify(frame));
    } else {
      showToast("Not connected, message not sent", "error");
    }
  },

  subscribeToSignals(listener: SignalListener) {
    signalListeners.add(listener);
    return () => signalListeners.delete(listener);
  },
}));

export default useWsStore;
