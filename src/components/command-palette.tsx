import { useQuery } from "@tanstack/react-query";
import { Command } from "cmdk";
import { MessageSquare, Settings, User, Users } from "lucide-react";
import { useState } from "react";
import { useHotkeys } from "react-hotkeys-hook";
import { useNavigate } from "react-router-dom";

import { Avatar, AvatarFallback, AvatarImage } from "@/components/ui/avatar";
import {
  friendsQuery,
  groupSessionsQuery,
  userSessionsQuery,
} from "@/service/queries";
import useAuthStore from "@/store/auth";

const itemClass =
  "data-[selected=true]:bg-accent data-[selected=true]:text-accent-foreground flex cursor-pointer items-center gap-2.5 rounded-md px-2 py-2 text-sm";

const groupClass =
  "[&_[cmdk-group-heading]]:text-muted-foreground [&_[cmdk-group-heading]]:px-2 [&_[cmdk-group-heading]]:py-1.5 [&_[cmdk-group-heading]]:text-xs [&_[cmdk-group-heading]]:font-medium";

export function CommandPalette() {
  const [open, setOpen] = useState(false);
  const navigate = useNavigate();
  const userId = useAuthStore((state) => state.userInfo.uuid);
  const isAdmin = useAuthStore((state) => state.userInfo.is_admin === 1);

  useHotkeys("mod+k", () => setOpen((previous) => !previous), {
    preventDefault: true,
    enableOnFormTags: true,
  });

  // Nothing is fetched until the palette is actually opened.
  const enabled = open && Boolean(userId);
  const { data: userSessions = [] } = useQuery({
    ...userSessionsQuery(userId),
    enabled,
  });
  const { data: groupSessions = [] } = useQuery({
    ...groupSessionsQuery(userId),
    enabled,
  });
  const { data: friends = [] } = useQuery({
    ...friendsQuery(userId),
    enabled,
  });

  const go = (path: string) => {
    setOpen(false);
    navigate(path);
  };

  return (
    <Command.Dialog
      open={open}
      onOpenChange={setOpen}
      label="Search conversations and contacts"
      loop
      overlayClassName="fixed inset-0 z-50 bg-black/40 backdrop-blur-sm data-[state=open]:animate-in data-[state=open]:fade-in-0"
      contentClassName="bg-popover text-popover-foreground border-border data-[state=open]:animate-in data-[state=open]:fade-in-0 data-[state=open]:zoom-in-95 fixed top-1/4 left-1/2 z-50 w-full max-w-lg -translate-x-1/2 overflow-hidden rounded-xl border shadow-2xl"
    >
      <Command.Input
        placeholder="Jump to a conversation, contact or page…"
        className="border-border placeholder:text-muted-foreground/60 w-full border-b bg-transparent px-4 py-3 text-sm outline-none"
      />
      <Command.List className="max-h-80 overflow-y-auto p-2">
        <Command.Empty className="text-muted-foreground py-6 text-center text-sm">
          No results found
        </Command.Empty>

        <Command.Group heading="Go to" className={groupClass}>
          <Command.Item
            className={itemClass}
            onSelect={() => go("/chat/sessions")}
          >
            <MessageSquare className="size-4" />
            Sessions
          </Command.Item>
          <Command.Item
            className={itemClass}
            onSelect={() => go("/chat/contacts")}
          >
            <Users className="size-4" />
            Contacts
          </Command.Item>
          <Command.Item
            className={itemClass}
            onSelect={() => go("/chat/profile")}
          >
            <User className="size-4" />
            My Profile
          </Command.Item>
          {isAdmin && (
            <Command.Item className={itemClass} onSelect={() => go("/manager")}>
              <Settings className="size-4" />
              Admin
            </Command.Item>
          )}
        </Command.Group>

        {[
          { heading: "Conversations", rows: userSessions },
          { heading: "Groups", rows: groupSessions },
        ].map(({ heading, rows }) =>
          rows.length === 0 ? null : (
            <Command.Group
              key={heading}
              heading={heading}
              className={groupClass}
            >
              {rows.map((row) => (
                <Command.Item
                  key={row.id}
                  value={`${row.name} ${row.id}`}
                  className={itemClass}
                  onSelect={() => go(`/chat/${row.id}`)}
                >
                  <Avatar className="size-6">
                    <AvatarImage src={row.avatar} alt={row.name} />
                    <AvatarFallback className="text-[10px]">
                      {row.name.charAt(0).toUpperCase()}
                    </AvatarFallback>
                  </Avatar>
                  <span className="truncate">{row.name}</span>
                  {row.unreadCount > 0 && (
                    <span className="text-muted-foreground ml-auto text-xs">
                      {row.unreadCount} unread
                    </span>
                  )}
                </Command.Item>
              ))}
            </Command.Group>
          ),
        )}

        {friends.length > 0 && (
          <Command.Group heading="Contacts" className={groupClass}>
            {friends.map((friend) => (
              <Command.Item
                key={friend.user_id}
                value={`${friend.note_name || friend.nickname} ${friend.user_id}`}
                className={itemClass}
                onSelect={() => go(`/chat/${friend.user_id}`)}
              >
                <Avatar className="size-6">
                  <AvatarImage src={friend.avatar} alt={friend.nickname} />
                  <AvatarFallback className="text-[10px]">
                    {friend.nickname.charAt(0).toUpperCase()}
                  </AvatarFallback>
                </Avatar>
                <span className="truncate">
                  {friend.note_name || friend.nickname}
                </span>
                {friend.online && (
                  <span className="ml-auto size-2 rounded-full bg-emerald-500" />
                )}
              </Command.Item>
            ))}
          </Command.Group>
        )}
      </Command.List>
    </Command.Dialog>
  );
}
