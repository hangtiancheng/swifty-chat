import { useMutation } from "@tanstack/react-query";
import { EditorContent, useEditor, useEditorState } from "@tiptap/react";
import StarterKit from "@tiptap/starter-kit";
import { Paperclip, Send, Smile } from "lucide-react";
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
}

function messageTypeFor(mime: string): number {
  if (mime.startsWith("image/")) return MessageType.Image;
  if (mime.startsWith("video/")) return MessageType.Video;
  return MessageType.File;
}

export function MessageComposer({ disabled, onSend }: MessageComposerProps) {
  const [progress, setProgress] = useState<UploadProgress | null>(null);
  // Enter must not send while an editor popup owns the key.
  const sendRef = useRef<() => void>(() => {});

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

  const sendText = () => {
    if (!editor) return;
    const text = editor.getText({ blockSeparator: "\n\n" }).trim();
    if (!text) return;
    onSend({
      type: MessageType.Text,
      content: text,
      url: "",
      file_name: "",
      file_size: "",
      file_type: "",
    });
    editor.commands.clearContent(true);
  };

  // The editor's keydown handler is installed once, so it reads the newest
  // closure through the ref rather than capturing a stale one.
  useEffect(() => {
    sendRef.current = sendText;
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

  const insertEmoji = (emoji: string) =>
    editor?.chain().focus().insertContent(emoji).run();

  return (
    <div
      {...getRootProps()}
      className={cn(
        "border-border bg-card relative flex flex-col border-t",
        isDragActive && "ring-primary/50 ring-2 ring-inset",
      )}
    >
      <input {...getInputProps()} />

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

      <div className="relative">
        <EditorContent editor={editor} />
        {isEmpty && (
          <span className="text-muted-foreground/50 pointer-events-none absolute top-2.5 left-3 text-sm">
            Type a message — markdown supported
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
        </div>

        <Button size="sm" disabled={disabled || isEmpty} onClick={sendText}>
          <Send className="size-3.5" />
          Send
        </Button>
      </div>
    </div>
  );
}
