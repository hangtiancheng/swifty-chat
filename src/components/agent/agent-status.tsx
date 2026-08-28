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

import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from "@/components/ui/tooltip";
import { cn } from "@/lib/utils";
import useAgentStore from "@/store/agent";
import { formatTokens } from "@/utils/format";

const DOT_CLASS = {
  idle: "bg-muted-foreground",
  connecting: "bg-amber-500 animate-pulse",
  connected: "bg-emerald-500",
  reconnecting: "bg-amber-500 animate-pulse",
} as const;

const DOT_LABEL = {
  idle: "Assistant offline",
  connecting: "Connecting to Swiftx…",
  connected: "Swiftx connected",
  reconnecting: "Reconnecting to Swiftx…",
} as const;

/** Header strip for the assistant thread: link health, model and token spend. */
export function AgentStatus() {
  const status = useAgentStore((state) => state.status);
  const ready = useAgentStore((state) => state.ready);
  const model = useAgentStore((state) => state.model);
  const usage = useAgentStore((state) => state.usage);

  // Warming up can take a while when MCP servers are configured, and a prompt
  // sent meanwhile just waits its turn — so say so rather than showing a
  // healthy dot the agent cannot live up to yet.
  const warming = status === "connected" && !ready;
  const dotClass = warming ? "bg-amber-500 animate-pulse" : DOT_CLASS[status];
  const label = warming ? "Swiftx is starting up…" : DOT_LABEL[status];

  return (
    <div className="text-muted-foreground flex items-center gap-2 text-xs">
      {usage && (
        <span className="tabular-nums" title="Tokens in / out">
          {formatTokens(usage.inputTokens)}↑ {formatTokens(usage.outputTokens)}↓
        </span>
      )}
      {model && <span className="hidden font-mono sm:inline">{model}</span>}
      <Tooltip>
        <TooltipTrigger
          render={
            <span
              aria-label={label}
              className={cn("size-2 rounded-full", dotClass)}
            />
          }
        />
        <TooltipContent side="bottom">{label}</TooltipContent>
      </Tooltip>
    </div>
  );
}
