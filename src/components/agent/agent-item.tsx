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
  AlertTriangle,
  Brain,
  Check,
  ChevronRight,
  CircleCheck,
  FileText,
  Info,
  Loader2,
  Search,
  Terminal,
  Wrench,
  X,
} from "lucide-react";
import { lazy, Suspense } from "react";

import { AgentPermissionCard } from "@/components/agent/agent-permission-card";
import { AgentQuestionCard } from "@/components/agent/agent-question-card";
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from "@/components/ui/collapsible";
import { cn } from "@/lib/utils";
import type {
  AgentItem,
  AgentNoticeItem,
  AgentStreamItem,
  AgentThinkingItem,
  AgentToolItem,
  ToolArgs,
} from "@/service/agent-schemas";

// Same lazy boundary the bubbles use, so markdown keeps its own chunk.
const MessageContent = lazy(() =>
  import("@/components/message-content").then((module) => ({
    default: module.MessageContent,
  })),
);

const OUTPUT_LIMIT = 5000;

/** A few well-known argument keys make a far better header than raw JSON. */
function argsPreview(args: ToolArgs): string {
  if (!args) return "";
  for (const key of ["command", "file_path", "pattern", "path", "url"]) {
    const value = args[key];
    if (typeof value === "string" && value.length > 0) return value;
  }
  return "";
}

function ToolIcon({ name, className }: { name: string; className?: string }) {
  if (name === "Bash") return <Terminal className={className} />;
  if (name === "Grep" || name === "Glob" || name === "WebSearch") {
    return <Search className={className} />;
  }
  if (
    name.startsWith("Read") ||
    name.startsWith("Write") ||
    name.startsWith("Edit")
  ) {
    return <FileText className={className} />;
  }
  return <Wrench className={className} />;
}

function StreamBlock({ item }: { item: AgentStreamItem }) {
  return (
    <div className="border-border bg-card text-foreground max-w-[70%] rounded-2xl rounded-bl-md border px-3.5 py-2.5 text-sm leading-relaxed wrap-break-word shadow-sm">
      <Suspense
        fallback={<p className="whitespace-pre-wrap">{item.content}</p>}
      >
        <MessageContent content={item.content} />
      </Suspense>
      {item.streaming && (
        <span className="bg-primary ml-0.5 inline-block h-4 w-1.5 animate-pulse align-text-bottom" />
      )}
    </div>
  );
}

function ThinkingBlock({ item }: { item: AgentThinkingItem }) {
  return (
    <Collapsible>
      <CollapsibleTrigger
        className={cn(
          "text-muted-foreground hover:text-foreground group flex w-full items-center gap-1.5 text-left text-xs",
          "cursor-pointer",
        )}
      >
        <ChevronRight className="size-3 transition-transform group-data-panel-open:rotate-90" />
        <Brain className="size-3.5" />
        <span className={cn(!item.done && "animate-pulse")}>
          {item.done ? "Thought" : "Thinking…"}
        </span>
      </CollapsibleTrigger>
      <CollapsibleContent>
        <p className="text-muted-foreground border-border mt-1.5 ml-2 border-l pl-3 text-xs whitespace-pre-wrap italic">
          {item.content}
        </p>
      </CollapsibleContent>
    </Collapsible>
  );
}

function ToolBlock({ item }: { item: AgentToolItem }) {
  const preview = argsPreview(item.args);
  const output =
    item.output.length > OUTPUT_LIMIT
      ? `${item.output.slice(0, OUTPUT_LIMIT)}\n… (truncated)`
      : item.output;

  return (
    <Collapsible className="border-border bg-card/60 max-w-[85%] rounded-lg border">
      <CollapsibleTrigger className="group flex w-full cursor-pointer items-center gap-2 px-2.5 py-1.5 text-left">
        <ChevronRight className="text-muted-foreground size-3 shrink-0 transition-transform group-data-panel-open:rotate-90" />
        <ToolIcon
          name={item.toolName}
          className="text-muted-foreground size-3.5 shrink-0"
        />
        <span className="text-foreground shrink-0 font-mono text-xs font-medium">
          {item.toolName}
        </span>
        {preview && (
          <span className="text-muted-foreground truncate font-mono text-xs">
            {preview}
          </span>
        )}
        <span className="ml-auto shrink-0">
          {item.status === "running" && (
            <Loader2 className="text-muted-foreground size-3.5 animate-spin" />
          )}
          {item.status === "ok" && (
            <span className="flex items-center gap-1 text-xs text-emerald-600 tabular-nums dark:text-emerald-400">
              <Check className="size-3.5" />
              {item.elapsed.toFixed(1)}s
            </span>
          )}
          {item.status === "error" && (
            <X className="text-destructive size-3.5" />
          )}
        </span>
      </CollapsibleTrigger>
      <CollapsibleContent>
        <div className="border-border space-y-2 border-t px-2.5 py-2">
          {item.args && (
            <pre className="text-muted-foreground max-h-40 overflow-auto font-mono text-[11px] whitespace-pre-wrap">
              {JSON.stringify(item.args, null, 2)}
            </pre>
          )}
          {output && (
            <pre
              className={cn(
                "max-h-60 overflow-auto font-mono text-[11px] whitespace-pre-wrap",
                item.status === "error"
                  ? "text-destructive"
                  : "text-foreground/80",
              )}
            >
              {output}
            </pre>
          )}
        </div>
      </CollapsibleContent>
    </Collapsible>
  );
}

function NoticeBlock({ item }: { item: AgentNoticeItem }) {
  const Icon =
    item.tone === "error"
      ? AlertTriangle
      : item.tone === "done"
        ? CircleCheck
        : Info;
  return (
    <div
      className={cn(
        "flex items-start gap-1.5 text-xs",
        item.tone === "error" ? "text-destructive" : "text-muted-foreground",
      )}
    >
      <Icon className="mt-0.5 size-3.5 shrink-0" />
      <span className="whitespace-pre-wrap">{item.content}</span>
    </div>
  );
}

/** Renders one entry of the live overlay that sits between chat messages. */
export function AgentItemView({ item }: { item: AgentItem }) {
  switch (item.kind) {
    case "stream":
      return <StreamBlock item={item} />;
    case "thinking":
      return <ThinkingBlock item={item} />;
    case "tool":
      return <ToolBlock item={item} />;
    case "notice":
      return <NoticeBlock item={item} />;
    case "permission":
      return <AgentPermissionCard item={item} />;
    case "question":
      return <AgentQuestionCard item={item} />;
  }
}

/** The overlay is indented to line up with the assistant's bubbles. */
export function AgentItemList({ items }: { items: AgentItem[] }) {
  if (items.length === 0) return null;
  return (
    <div className="mt-4 flex flex-col gap-2 pl-11">
      {items.map((item) => (
        <AgentItemView key={item.id} item={item} />
      ))}
    </div>
  );
}
