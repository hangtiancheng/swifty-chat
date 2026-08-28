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

import { ShieldQuestion } from "lucide-react";

import { Button } from "@/components/ui/button";
import type { AgentPermissionItem } from "@/service/agent-schemas";
import useAgentStore from "@/store/agent";

const RESULT_LABEL = {
  allow: "Allowed once",
  allowAlways: "Always allowed",
  deny: "Denied",
} as const;

/** Swiftx blocks on this card: the tool call does not proceed until an answer
 * goes back, so the choices stay visible until one is picked. */
export function AgentPermissionCard({ item }: { item: AgentPermissionItem }) {
  const respondPermission = useAgentStore((state) => state.respondPermission);

  if (item.response) {
    return (
      <p className="text-muted-foreground text-xs">
        {RESULT_LABEL[item.response]} · {item.toolName}
      </p>
    );
  }

  return (
    <section
      aria-label={`Permission request for ${item.toolName}`}
      className="border-primary/30 bg-card max-w-[85%] rounded-lg border px-3 py-2.5 shadow-sm"
    >
      <div className="text-foreground flex items-center gap-1.5 text-xs font-semibold">
        <ShieldQuestion className="text-primary size-3.5" />
        {item.toolName} needs permission
      </div>
      <p className="text-muted-foreground mt-1 text-xs wrap-break-word whitespace-pre-wrap">
        {item.description}
      </p>
      <div className="mt-2.5 flex flex-wrap gap-1.5">
        <Button size="sm" onClick={() => respondPermission(item.id, "allow")}>
          Allow
        </Button>
        <Button
          size="sm"
          variant="outline"
          onClick={() => respondPermission(item.id, "allowAlways")}
        >
          Always allow
        </Button>
        <Button
          size="sm"
          variant="ghost"
          className="text-destructive"
          onClick={() => respondPermission(item.id, "deny")}
        >
          Deny
        </Button>
      </div>
    </section>
  );
}
