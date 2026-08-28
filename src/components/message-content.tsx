import { cjk } from "@streamdown/cjk";
import { code } from "@streamdown/code";
import { math } from "@streamdown/math";
import { mermaid } from "@streamdown/mermaid";
import { Streamdown } from "streamdown";

import { cn } from "@/lib/utils";

/** Configured once at module scope: chat text is static markdown, never streamed. */
const plugins = { cjk, code, math, mermaid };

interface MessageContentProps {
  content: string;
  className?: string;
}

export function MessageContent({ content, className }: MessageContentProps) {
  return (
    <Streamdown
      mode="static"
      animated={false}
      parseIncompleteMarkdown={false}
      plugins={plugins}
      className={cn("space-y-2 text-sm leading-relaxed", className)}
    >
      {content}
    </Streamdown>
  );
}
