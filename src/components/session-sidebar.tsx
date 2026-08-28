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

import NumberFlow from "@number-flow/react";
import { useQuery } from "@tanstack/react-query";
import { ChevronDown } from "lucide-react";
import { AnimatePresence, motion } from "motion/react";
import { useState } from "react";
import { useNavigate, useParams } from "react-router-dom";

import { Avatar, AvatarFallback, AvatarImage } from "@/components/ui/avatar";
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from "@/components/ui/collapsible";
import { Input } from "@/components/ui/input";
import { cn } from "@/lib/utils";
import { groupSessionsQuery, userSessionsQuery } from "@/service/queries";
import { MessageType, type ChatSession } from "@/service/schemas";
import useAuthStore from "@/store/auth";
import { formatSessionTime } from "@/utils/format";

const PREVIEW_BY_TYPE: Record<number, string> = {
  [MessageType.Image]: "[Image]",
  [MessageType.File]: "[File]",
  [MessageType.AvSignal]: "[Call]",
  [MessageType.Video]: "[Video]",
};

function previewOf(session: ChatSession): string {
  return PREVIEW_BY_TYPE[session.lastMessageType] ?? session.lastMessage;
}

interface SessionRowProps {
  session: ChatSession;
  active: boolean;
  onSelect: () => void;
}

function SessionRow({ session, active, onSelect }: SessionRowProps) {
  const preview = previewOf(session);
  return (
    <motion.button
      type="button"
      layout="position"
      initial={{ opacity: 0, y: -4 }}
      animate={{ opacity: 1, y: 0 }}
      exit={{ opacity: 0 }}
      transition={{ duration: 0.18 }}
      onClick={onSelect}
      className={cn(
        "hover:bg-accent/60 active:bg-accent flex w-full cursor-pointer items-center gap-2.5 px-3 py-2 text-left transition-colors",
        active && "bg-accent/80",
      )}
    >
      <Avatar className="size-9 shrink-0">
        <AvatarImage src={session.avatar} alt={session.name} />
        <AvatarFallback className="text-xs">
          {session.name.charAt(0).toUpperCase() || "?"}
        </AvatarFallback>
      </Avatar>

      <div className="flex min-w-0 flex-1 flex-col">
        <div className="flex items-baseline justify-between gap-2">
          <span className="text-foreground truncate text-sm font-medium">
            {session.name}
          </span>
          <span className="text-muted-foreground shrink-0 text-[10px] tabular-nums">
            {formatSessionTime(session.lastMessageAtMs)}
          </span>
        </div>
        <div className="flex items-center justify-between gap-2">
          <span className="text-muted-foreground truncate text-xs">
            {preview || "No messages yet"}
          </span>
          {session.unreadCount > 0 && (
            <span className="bg-primary text-primary-foreground flex h-4 min-w-4 shrink-0 items-center justify-center rounded-full px-1 text-[10px] font-medium tabular-nums">
              <NumberFlow value={session.unreadCount} />
            </span>
          )}
        </div>
      </div>
    </motion.button>
  );
}

interface SessionSectionProps {
  title: string;
  open: boolean;
  isLoading: boolean;
  filtered: boolean;
  sessions: ChatSession[];
  activeId: string | undefined;
  onOpenChange: (open: boolean) => void;
  onSelect: (id: string) => void;
}

function SessionSection({
  title,
  open,
  isLoading,
  filtered,
  sessions,
  activeId,
  onOpenChange,
  onSelect,
}: SessionSectionProps) {
  return (
    <Collapsible open={open} onOpenChange={onOpenChange}>
      <CollapsibleTrigger
        className="border-border bg-muted/30 hover:bg-accent/50 flex w-full cursor-pointer items-center justify-between border-b px-3 py-2.5 transition-colors"
        aria-label={`${title} (${sessions.length})`}
      >
        <span className="text-foreground text-sm font-medium">
          {title}
          <span className="text-muted-foreground ml-2 text-xs font-normal tabular-nums">
            <NumberFlow value={sessions.length} />
          </span>
        </span>
        <ChevronDown
          className={cn(
            "text-muted-foreground size-4 transition-transform duration-200",
            open && "rotate-180",
          )}
        />
      </CollapsibleTrigger>

      <CollapsibleContent className="overflow-hidden">
        {isLoading ? (
          <p className="text-muted-foreground animate-pulse px-3 py-3 text-xs">
            Loading sessions…
          </p>
        ) : sessions.length === 0 ? (
          <p className="text-muted-foreground px-3 py-3 text-xs">
            {filtered ? "No matches found" : "No sessions yet"}
          </p>
        ) : (
          <AnimatePresence initial={false}>
            {sessions.map((session) => (
              <SessionRow
                key={session.id}
                session={session}
                active={session.id === activeId}
                onSelect={() => onSelect(session.id)}
              />
            ))}
          </AnimatePresence>
        )}
      </CollapsibleContent>
    </Collapsible>
  );
}

export function SessionSidebar() {
  const navigate = useNavigate();
  const { id: activeId } = useParams<{ id: string }>();
  const userId = useAuthStore((state) => state.userInfo.uuid);

  const [query, setQuery] = useState("");
  const [usersOpen, setUsersOpen] = useState(true);
  const [groupsOpen, setGroupsOpen] = useState(false);

  const users = useQuery(userSessionsQuery(userId));
  // Group sessions stay unfetched until the section is expanded.
  const groups = useQuery({
    ...groupSessionsQuery(userId),
    enabled: groupsOpen && Boolean(userId),
  });

  const needle = query.trim().toLowerCase();
  const matching = (sessions: ChatSession[] | undefined) => {
    const rows = sessions ?? [];
    return needle
      ? rows.filter((row) => row.name.toLowerCase().includes(needle))
      : rows;
  };

  const openChat = (id: string) => navigate(`/chat/${id}`);

  return (
    <div className="flex h-full w-full flex-col">
      <div className="p-2">
        <Input
          value={query}
          onChange={(event) => setQuery(event.target.value)}
          placeholder="Search sessions"
          aria-label="Search sessions"
        />
      </div>

      <div className="flex-1 overflow-y-auto">
        <SessionSection
          title="Users"
          open={usersOpen}
          isLoading={users.isPending}
          filtered={Boolean(needle)}
          sessions={matching(users.data)}
          activeId={activeId}
          onOpenChange={setUsersOpen}
          onSelect={openChat}
        />
        <SessionSection
          title="Groups"
          open={groupsOpen}
          isLoading={groupsOpen && groups.isPending}
          filtered={Boolean(needle)}
          sessions={matching(groups.data)}
          activeId={activeId}
          onOpenChange={setGroupsOpen}
          onSelect={openChat}
        />
      </div>
    </div>
  );
}
