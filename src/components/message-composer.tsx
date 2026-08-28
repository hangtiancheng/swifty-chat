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

import { useMutation } from "@tanstack/react-query";
import { EditorContent, useEditor, useEditorState } from "@tiptap/react";
import StarterKit from "@tiptap/starter-kit";
import { Paperclip, Send, Smile, Square } from "lucide-react";
import { AnimatePresence, motion } from "motion/react";
import { useEffect, useRef, useState } from "react";
import { useDropzone } from "react-dropzone";

import { Button } from "@/components/ui/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { Progress } from "@/components/ui/progress";
import { staticUrl } from "@/env";
import { cn } from "@/lib/utils";
import type { SlashCommand } from "@/service/agent-schemas";
import { MessageType } from "@/service/schemas";
import { uploadInChunks, type UploadProgress } from "@/service/upload";
import { getFileSize } from "@/utils/format";

/**
 * Tiptap runs with every mark and input rule disabled so that literal markdown
 * survives to the server, where the bubble renderer turns it back into rich text.
 */
const extensions = [
  StarterKit.configure({
    blockquote: false,
    bold: false,
    bulletList: false,
    code: false,
    codeBlock: false,
    heading: false,
    horizontalRule: false,
    italic: false,
    link: false,
    listItem: false,
    listKeymap: false,
    orderedList: false,
    strike: false,
    trailingNode: false,
    underline: false,
  }),
];

const EMOJIS = [
  "😀",
  "😂",
  "🥹",
  "😍",
  "🤔",
  "😴",
  "🥳",
  "😭",
  "👍",
  "👏",
  "🙏",
  "💪",
  "🤝",
  "👀",
  "🔥",
  "✨",
  "❤️",
  "💔",
  "🎉",
  "🎁",
  "☕",
  "🍺",
  "🌙",
  "⭐",
  "✅",
  "❌",
  "⚡",
  "🚀",
  "🐛",
  "📌",
  "💡",
  "🧠",
];

export interface ComposerPayload {
  type: number;
  content: string;
  url: string;
  file_name: string;
  file_size: string;
  file_type: string;
}

interface MessageComposerProps {
  disabled?: boolean;
  onSend: (payload: ComposerPayload) => void;
  /** Slash commands to offer while the text is a single "/word" token. */
  commands?: SlashCommand[];
  /** True while the assistant is mid-reply, which adds a stop control. */
  streaming?: boolean;
  onStop?: () => void;
  /** The assistant reads text only, so its thread hides uploads entirely. */
  allowAttachments?: boolean;
  placeholder?: string;
}

function messageTypeFor(mime: string): number {
  if (mime.startsWith("image/")) return MessageType.Image;
  if (mime.startsWith("video/")) return MessageType.Video;
  return MessageType.File;
}

export function MessageComposer({
  disabled,
  onSend,
  commands = [],
  streaming = false,
  onStop,
  allowAttachments = true,
  placeholder = "Type a message — markdown supported",
}: MessageComposerProps) {
  const [progress, setProgress] = useState<UploadProgress | null>(null);
  // Enter must not send while an editor popup owns the key.
  const sendRef = useRef<() => void>(() => {});
  const menuKeyRef = useRef<(event: KeyboardEvent) => boolean>(() => false);

  const upload = useMutation({
    mutationFn: (source: File) => uploadInChunks(source, setProgress),
    onSuccess: (result, source) => {
      onSend({
        type: messageTypeFor(source.type),
        content: "",
        url: staticUrl(result.url),
        file_name: result.file_name || source.name,
        file_size: getFileSize(Number(result.file_size) || source.size),
        file_type: source.type,
      });
    },
    onSettled: () => setProgress(null),
  });

  const editor = useEditor({
    extensions,
    editorProps: {
      attributes: {
        class:
          "min-h-24 max-h-32 overflow-y-auto px-3 py-2.5 text-sm leading-relaxed outline-none",
      },
      handleKeyDown: (_view, event) => {
        if (menuKeyRef.current(event)) return true;
        if (event.key === "Enter" && !event.shiftKey && !event.isComposing) {
          event.preventDefault();
          sendRef.current();
          return true;
        }
        return false;
      },
    },
  });

  const isEmpty = useEditorState({
    editor,
    selector: ({ editor: instance }) => instance?.isEmpty ?? true,
  });

  const text = useEditorState({
    editor,
    selector: ({ editor: instance }) => instance?.getText() ?? "",
  });

  const [dismissedQuery, setDismissedQuery] = useState<string | null>(null);
  const [pick, setPick] = useState<{ query: string | null; index: number }>({
    query: null,
    index: 0,
  });

  // The menu belongs on a bare "/word" only. The text is matched untrimmed on
  // purpose: the space that follows a chosen command is what dismisses the
  // list, so trimming it away would leave the menu stuck open.
  const query = /^\/\S*$/.test(text) ? text.slice(1) : null;
  const matches =
    query === null
      ? []
      : commands.filter((command) =>
          command.name.toLowerCase().startsWith(query.toLowerCase()),
        );
  const menuOpen = matches.length > 0 && dismissedQuery !== query;
  // Both the highlight and the dismissal are tied to the query that produced
  // them, so editing the text resets them without an effect.
  const active =
    pick.query === query ? Math.min(pick.index, matches.length - 1) : 0;

  const applyCommand = (name: string) => {
    editor?.chain().focus().clearContent().insertContent(`/${name} `).run();
  };

  const sendText = () => {
    if (!editor) return;
    const value = editor.getText({ blockSeparator: "\n\n" }).trim();
    if (!value) return;
    onSend({
      type: MessageType.Text,
      content: value,
      url: "",
      file_name: "",
      file_size: "",
      file_type: "",
    });
    editor.commands.clearContent(true);
  };

  // The editor's keydown handler is installed once, so it reads the newest
  // closures through refs rather than capturing stale ones.
  useEffect(() => {
    sendRef.current = sendText;
    menuKeyRef.current = (event) => {
      if (!menuOpen) return false;
      switch (event.key) {
        case "ArrowDown":
          event.preventDefault();
          setPick({ query, index: (active + 1) % matches.length });
          return true;
        case "ArrowUp":
          event.preventDefault();
          setPick({
            query,
            index: (active - 1 + matches.length) % matches.length,
          });
          return true;
        case "Tab":
        case "Enter":
          event.preventDefault();
          applyCommand(matches[active].name);
          return true;
        case "Escape":
          event.preventDefault();
          setDismissedQuery(query);
          return true;
        default:
          return false;
      }
    };
  });

  const { getRootProps, getInputProps, open, isDragActive } = useDropzone({
    noClick: true,
    noKeyboard: true,
    multiple: false,
    disabled: disabled || upload.isPending,
    onDrop: (accepted) => {
      const [source] = accepted;
      if (source) upload.mutate(source);
    },
  });

  // A thread that takes no attachments simply does not wire the dropzone up.
  // Disabling it instead would mark the whole composer aria-disabled, which
  // makes every control inside it — the stop button included — read as dead.
  const dropProps = allowAttachments ? getRootProps() : {};

  const insertEmoji = (emoji: string) =>
    editor?.chain().focus().insertContent(emoji).run();

  return (
    <div
      {...dropProps}
      className={cn(
        "border-border bg-card relative flex flex-col border-t",
        isDragActive && "ring-primary/50 ring-2 ring-inset",
      )}
    >
      {allowAttachments && <input {...getInputProps()} />}

      <AnimatePresence>
        {isDragActive && (
          <motion.div
            initial={{ opacity: 0 }}
            animate={{ opacity: 1 }}
            exit={{ opacity: 0 }}
            className="bg-primary/5 text-primary pointer-events-none absolute inset-0 z-10 flex items-center justify-center text-sm font-medium"
          >
            Drop a file to send
          </motion.div>
        )}
      </AnimatePresence>

      {menuOpen && (
        <ul
          aria-label="Slash commands"
          className="border-border bg-popover absolute bottom-full left-2 z-20 mb-1 max-h-60 w-80 overflow-y-auto rounded-lg border p-1 shadow-md"
        >
          {matches.map((command, index) => (
            <li key={command.name}>
              <button
                type="button"
                onMouseDown={(event) => {
                  // Keeps the editor focused so the insert lands in the doc.
                  event.preventDefault();
                  applyCommand(command.name);
                }}
                onMouseEnter={() => setPick({ query, index })}
                className={cn(
                  "flex w-full cursor-pointer flex-col items-start gap-0.5 rounded-md px-2 py-1.5 text-left",
                  index === active && "bg-accent",
                )}
              >
                <span className="font-mono text-xs font-medium">
                  /{command.name}
                </span>
                {command.description && (
                  <span className="text-muted-foreground line-clamp-2 text-xs">
                    {command.description}
                  </span>
                )}
              </button>
            </li>
          ))}
        </ul>
      )}

      <div className="relative">
        <EditorContent editor={editor} />
        {isEmpty && (
          <span className="text-muted-foreground/50 pointer-events-none absolute top-2.5 left-3 text-sm">
            {placeholder}
          </span>
        )}
      </div>

      {progress && (
        <div className="flex items-center gap-2 px-3 pb-1">
          <Progress
            value={(progress.completed / progress.total) * 100}
            className="flex-1"
          />
          <span className="text-muted-foreground text-[10px] tabular-nums">
            {progress.phase === "hashing" ? "Hashing" : "Uploading"}{" "}
            {progress.completed}/{progress.total}
          </span>
        </div>
      )}

      <div className="flex items-center justify-between px-2 pb-2">
        <div className="flex items-center gap-1">
          <DropdownMenu>
            <DropdownMenuTrigger
              render={
                <Button
                  variant="ghost"
                  size="icon"
                  className="text-muted-foreground"
                  aria-label="Insert emoji"
                />
              }
            >
              <Smile className="size-4" />
            </DropdownMenuTrigger>
            <DropdownMenuContent align="start" className="w-64 p-2">
              <div className="grid grid-cols-8 gap-1">
                {EMOJIS.map((emoji) => (
                  <button
                    key={emoji}
                    type="button"
                    className="hover:bg-accent cursor-pointer rounded-md p-1 text-lg leading-none"
                    onClick={() => insertEmoji(emoji)}
                  >
                    {emoji}
                  </button>
                ))}
              </div>
            </DropdownMenuContent>
          </DropdownMenu>

          {allowAttachments && (
            <Button
              variant="ghost"
              size="icon"
              className="text-muted-foreground"
              aria-label="Attach a file"
              disabled={disabled || upload.isPending}
              onClick={open}
            >
              <Paperclip className="size-4" />
            </Button>
          )}
        </div>

        <div className="flex items-center gap-1.5">
          {streaming && onStop && (
            <Button size="sm" variant="outline" onClick={onStop}>
              <Square className="size-3.5" />
              Stop
            </Button>
          )}
          <Button size="sm" disabled={disabled || isEmpty} onClick={sendText}>
            <Send className="size-3.5" />
            Send
          </Button>
        </div>
      </div>
    </div>
  );
}
