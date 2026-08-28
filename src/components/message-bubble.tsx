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

import { Download, FileText, MessageCircle } from "lucide-react";
import { motion } from "motion/react";
import { lazy, Suspense } from "react";

import { Avatar, AvatarFallback, AvatarImage } from "@/components/ui/avatar";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { staticUrl } from "@/env";
import { cn } from "@/lib/utils";
import { MessageType, type Message } from "@/service/schemas";
import {
  formatMessageDay,
  formatMessageTime,
  messageDayKey,
} from "@/utils/format";

// Markdown brings shiki, katex and mermaid with it, so it ships as its own chunk.
const MessageContent = lazy(() =>
  import("@/components/message-content").then((module) => ({
    default: module.MessageContent,
  })),
);

interface MessageBubbleProps {
  messageList: Message[];
  currentUserId: string;
  currentUserAvatar: string;
  currentUserName: string;
}

async function downloadFile(url: string, name: string) {
  const fileUrl = url ? staticUrl(url) : staticUrl(`/static/files/${name}`);
  try {
    const response = await fetch(fileUrl);
    if (!response.ok) return;
    const objectUrl = URL.createObjectURL(await response.blob());
    const link = document.createElement("a");
    link.href = objectUrl;
    link.download = name || "download";
    document.body.appendChild(link);
    link.click();
    link.remove();
    URL.revokeObjectURL(objectUrl);
  } catch {
    // A failed download needs no recovery beyond leaving the bubble untouched.
  }
}

function initialOf(name: string): string {
  return name.trim().charAt(0).toUpperCase() || "?";
}

function FileAttachment({
  message,
  isSelf,
}: {
  message: Message;
  isSelf: boolean;
}) {
  const fileName = message.file_name || "file";

  return (
    <div className="flex items-center gap-3">
      <span
        className={cn(
          "flex size-10 shrink-0 items-center justify-center rounded-xl",
          isSelf
            ? "bg-primary-foreground/15 text-primary-foreground"
            : "bg-muted text-primary",
        )}
      >
        <FileText className="size-5" />
      </span>
      <div className="flex min-w-0 flex-col items-start gap-1.5">
        <div className="flex min-w-0 items-center gap-2">
          <span
            className={cn(
              "truncate text-sm font-medium",
              isSelf ? "text-primary-foreground" : "text-foreground",
            )}
          >
            {fileName}
          </span>
          {message.file_size && (
            <Badge variant="secondary" className="px-1.5 py-0 text-[10px]">
              {message.file_size}
            </Badge>
          )}
        </div>
        <Button
          variant="outline"
          size="sm"
          onClick={() => void downloadFile(message.url, fileName)}
        >
          <Download />
          Download
        </Button>
      </div>
    </div>
  );
}

function Attachment({
  message,
  isSelf,
}: {
  message: Message;
  isSelf: boolean;
}) {
  if (message.type === MessageType.Image) {
    return (
      <a href={staticUrl(message.url)} target="_blank" rel="noreferrer">
        <img
          src={staticUrl(message.url)}
          alt={message.file_name || "image"}
          loading="lazy"
          className="max-h-60 rounded-lg object-cover"
        />
      </a>
    );
  }
  if (message.type === MessageType.Video) {
    return (
      <video
        controls
        preload="metadata"
        src={staticUrl(message.url)}
        className="max-h-60 rounded-lg"
      />
    );
  }
  return <FileAttachment message={message} isSelf={isSelf} />;
}

export function MessageBubble({
  messageList,
  currentUserId,
  currentUserAvatar,
  currentUserName,
}: MessageBubbleProps) {
  // Call signalling is persisted as type 3 but carries no renderable payload.
  const conversation = messageList.filter(
    (message) => message.type !== MessageType.AvSignal,
  );

  if (conversation.length === 0) {
    return (
      <motion.div
        initial={{ opacity: 0 }}
        animate={{ opacity: 1 }}
        transition={{ duration: 0.4 }}
        className="flex flex-1 flex-col items-center justify-center gap-3 py-20"
      >
        <span className="bg-muted/70 flex size-12 items-center justify-center rounded-full">
          <MessageCircle className="text-muted-foreground/60 size-5" />
        </span>
        <p className="text-muted-foreground/60 text-sm">No messages yet</p>
      </motion.div>
    );
  }

  return (
    <div className="flex flex-col gap-4">
      {conversation.map((message, index) => {
        const isSelf = message.send_id === currentUserId;
        const name = isSelf ? currentUserName : message.send_name;
        const avatar = isSelf ? currentUserAvatar : message.send_avatar;
        const isText = message.type === MessageType.Text;
        const dayKey = messageDayKey(message.created_at);
        const showDay =
          index === 0 ||
          messageDayKey(conversation[index - 1].created_at) !== dayKey;

        return (
          <div key={message.uuid || `${message.send_id}-${index}`}>
            {showDay && (
              <div className="text-muted-foreground/70 mb-4 text-center text-[11px]">
                {formatMessageDay(message.created_at)}
              </div>
            )}
            <motion.div
              initial={{ opacity: 0, y: 8 }}
              animate={{ opacity: 1, y: 0 }}
              transition={{ duration: 0.2, ease: "easeOut" }}
              className={cn(
                "flex items-start gap-2.5",
                isSelf && "flex-row-reverse",
              )}
            >
              <Avatar
                className={cn(
                  "size-9 transition-transform duration-300 hover:scale-105",
                  isSelf && "ring-primary/30 ring-2",
                )}
              >
                <AvatarImage src={avatar} alt={name} />
                <AvatarFallback>{initialOf(name)}</AvatarFallback>
              </Avatar>

              <div
                className={cn(
                  "flex min-w-0 flex-col gap-1",
                  isSelf ? "items-end" : "items-start",
                )}
              >
                <span className="text-muted-foreground flex items-baseline gap-2 px-1 text-xs">
                  <span className="font-medium">{name}</span>
                  <span className="tabular-nums opacity-70">
                    {formatMessageTime(message.created_at)}
                  </span>
                </span>

                <div
                  className={cn(
                    "max-w-[70%] rounded-2xl px-3.5 py-2.5 text-sm leading-relaxed break-words transition-shadow duration-300",
                    isSelf
                      ? "bg-primary text-primary-foreground hover:shadow-primary/20 rounded-br-md hover:shadow-md"
                      : "border-border bg-card text-foreground rounded-bl-md border shadow-sm hover:shadow-md",
                  )}
                >
                  {isText ? (
                    <Suspense
                      fallback={
                        <p className="whitespace-pre-wrap">{message.content}</p>
                      }
                    >
                      <MessageContent content={message.content} />
                    </Suspense>
                  ) : (
                    <Attachment message={message} isSelf={isSelf} />
                  )}
                </div>
              </div>
            </motion.div>
          </div>
        );
      })}
    </div>
  );
}
