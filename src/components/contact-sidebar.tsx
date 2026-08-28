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
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { groupBy, sortBy } from "es-toolkit";
import { ChevronDown, MoreHorizontal, Plus } from "lucide-react";
import { AnimatePresence, motion } from "motion/react";
import { useState } from "react";
import { useNavigate } from "react-router-dom";

import { AddContactDialog } from "@/components/add-contact-dialog";
import { ApplyListDialog } from "@/components/apply-list-dialog";
import { ContactSettingsDialog } from "@/components/contact-settings-dialog";
import { CreateGroupDialog } from "@/components/create-group-dialog";
import { Avatar, AvatarFallback, AvatarImage } from "@/components/ui/avatar";
import { Button } from "@/components/ui/button";
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from "@/components/ui/collapsible";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { Input } from "@/components/ui/input";
import { useDebouncedValue } from "@/lib/use-debounced-value";
import { cn } from "@/lib/utils";
import { contact, session } from "@/service/api";
import {
  contactAppliesQuery,
  friendsQuery,
  groupSearchQuery,
  joinedGroupsQuery,
  keys,
  myGroupsQuery,
  tagsQuery,
  userSearchQuery,
} from "@/service/queries";
import type { Friend } from "@/service/schemas";
import useAuthStore from "@/store/auth";

const BLOCKED_BY_ME = 1;
const UNGROUPED = "Ungrouped";

interface SectionProps {
  title: string;
  count: number;
  open: boolean;
  onOpenChange: (open: boolean) => void;
  children: React.ReactNode;
}

function Section({ title, count, open, onOpenChange, children }: SectionProps) {
  return (
    <Collapsible open={open} onOpenChange={onOpenChange}>
      <CollapsibleTrigger className="border-border bg-muted/30 hover:bg-accent/50 flex w-full cursor-pointer items-center justify-between border-b px-3 py-2.5 text-sm font-medium transition-colors">
        <span>
          {title}
          <span className="text-muted-foreground ml-2 text-xs font-normal tabular-nums">
            <NumberFlow value={count} />
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
        {children}
      </CollapsibleContent>
    </Collapsible>
  );
}

interface RowProps {
  name: string;
  avatar: string;
  online?: boolean;
  trailing?: React.ReactNode;
  onClick: () => void;
}

function Row({ name, avatar, online, trailing, onClick }: RowProps) {
  return (
    <motion.div
      layout="position"
      initial={{ opacity: 0 }}
      animate={{ opacity: 1 }}
      exit={{ opacity: 0 }}
      transition={{ duration: 0.15 }}
      className="group hover:bg-accent/60 flex items-center justify-between gap-2 px-3 py-2 transition-colors"
    >
      <button
        type="button"
        onClick={onClick}
        className="flex min-w-0 flex-1 cursor-pointer items-center gap-2 text-left"
      >
        <span className="relative shrink-0">
          <Avatar className="size-7">
            <AvatarImage src={avatar} alt={name} />
            <AvatarFallback className="text-[10px]">
              {name.charAt(0).toUpperCase() || "?"}
            </AvatarFallback>
          </Avatar>
          {online && (
            <span className="ring-card absolute -right-0.5 -bottom-0.5 size-2.5 rounded-full bg-emerald-500 ring-2" />
          )}
        </span>
        <span className="truncate text-sm">{name}</span>
      </button>
      {trailing}
    </motion.div>
  );
}

export function ContactSidebar() {
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const userId = useAuthStore((state) => state.userInfo.uuid);

  const [keyword, setKeyword] = useState("");
  const search = useDebouncedValue(keyword.trim(), 300);

  const [friendsOpen, setFriendsOpen] = useState(true);
  const [myGroupsOpen, setMyGroupsOpen] = useState(false);
  const [joinedGroupsOpen, setJoinedGroupsOpen] = useState(false);

  const [addOpen, setAddOpen] = useState(false);
  const [createOpen, setCreateOpen] = useState(false);
  const [requestsOpen, setRequestsOpen] = useState(false);
  const [editing, setEditing] = useState<Friend | null>(null);

  const friends = useQuery(friendsQuery(userId));
  const tags = useQuery(tagsQuery(userId));
  const myGroups = useQuery({
    ...myGroupsQuery(userId),
    enabled: myGroupsOpen && Boolean(userId),
  });
  const joinedGroups = useQuery({
    ...joinedGroupsQuery(userId),
    enabled: joinedGroupsOpen && Boolean(userId),
  });
  const applies = useQuery({
    ...contactAppliesQuery(userId),
    enabled: requestsOpen && Boolean(userId),
  });

  const searching = search.length > 0;
  const userResults = useQuery({
    ...userSearchQuery(userId, search),
    enabled: searching && Boolean(userId),
  });
  const groupResults = useQuery({
    ...groupSearchQuery(userId, search),
    enabled: searching && Boolean(userId),
  });

  const openChat = useMutation({
    mutationFn: async (contactId: string) => {
      if (!(await session.isOpenAllowed(userId, contactId))) {
        throw new Error("This conversation cannot be opened");
      }
      return contactId;
    },
    onSuccess: (contactId) => navigate(`/chat/${contactId}`),
  });

  const unblock = useMutation({
    mutationFn: (contactId: string) => contact.unblock(userId, contactId),
    onSuccess: () =>
      queryClient.invalidateQueries({ queryKey: keys.contacts.all }),
  });

  const tagNames = new Map(
    (tags.data ?? []).map((tag) => [tag.tag_id, tag.name]),
  );
  const friendSections = sortBy(
    Object.entries(
      groupBy(friends.data ?? [], (friend) => friend.tag_id || ""),
    ).map(([tagId, rows]) => ({
      label: tagId ? (tagNames.get(tagId) ?? "Tagged") : UNGROUPED,
      rows,
    })),
    // Ungrouped contacts sink to the bottom of the list.
    [
      (section) => (section.label === UNGROUPED ? 1 : 0),
      (section) => section.label,
    ],
  );

  return (
    <div className="flex h-full w-full flex-col">
      <div className="flex items-center gap-1 p-2">
        <Input
          className="flex-1 text-sm"
          placeholder="Search users and groups"
          aria-label="Search users and groups"
          value={keyword}
          onChange={(event) => setKeyword(event.target.value)}
        />
        <DropdownMenu>
          <DropdownMenuTrigger
            render={
              <Button
                variant="outline"
                size="icon"
                className="rounded-md"
                aria-label="Add contact or group"
              />
            }
          >
            <Plus className="size-4" />
          </DropdownMenuTrigger>
          <DropdownMenuContent align="end" className="w-44">
            <DropdownMenuItem
              className="text-sm"
              onClick={() => setAddOpen(true)}
            >
              Add Contact / Group
            </DropdownMenuItem>
            <DropdownMenuItem
              className="text-sm"
              onClick={() => setCreateOpen(true)}
            >
              Create Group
            </DropdownMenuItem>
            <DropdownMenuItem
              className="text-sm"
              onClick={() => setRequestsOpen(true)}
            >
              Friend Requests
            </DropdownMenuItem>
          </DropdownMenuContent>
        </DropdownMenu>
      </div>

      <div className="flex-1 overflow-y-auto">
        {searching ? (
          <div className="flex flex-col">
            <p className="text-muted-foreground border-border bg-muted/30 border-b px-3 py-2 text-xs">
              {userResults.isPending || groupResults.isPending
                ? "Searching…"
                : `${userResults.data?.length ?? 0} users · ${groupResults.data?.length ?? 0} groups`}
            </p>
            <AnimatePresence initial={false}>
              {(userResults.data ?? []).map((result) => (
                <Row
                  key={result.uuid}
                  name={result.nickname || result.uuid}
                  avatar={result.avatar}
                  onClick={() =>
                    result.is_friend
                      ? openChat.mutate(result.uuid)
                      : setAddOpen(true)
                  }
                  trailing={
                    <span className="text-muted-foreground shrink-0 text-[10px]">
                      {result.is_friend ? "friend" : "add"}
                    </span>
                  }
                />
              ))}
              {(groupResults.data ?? []).map((result) => (
                <Row
                  key={result.group_id}
                  name={result.name || result.group_id}
                  avatar={result.avatar}
                  onClick={() =>
                    result.is_joined
                      ? openChat.mutate(result.group_id)
                      : setAddOpen(true)
                  }
                  trailing={
                    <span className="text-muted-foreground shrink-0 text-[10px]">
                      {result.is_joined ? "joined" : `${result.member_cnt}`}
                    </span>
                  }
                />
              ))}
            </AnimatePresence>
          </div>
        ) : (
          <>
            <Section
              title="Friends"
              count={friends.data?.length ?? 0}
              open={friendsOpen}
              onOpenChange={setFriendsOpen}
            >
              {friends.isPending ? (
                <p className="text-muted-foreground animate-pulse px-3 py-3 text-xs">
                  Loading contacts…
                </p>
              ) : friendSections.length === 0 ? (
                <p className="text-muted-foreground px-3 py-3 text-xs">
                  No contacts yet
                </p>
              ) : (
                friendSections.map((group) => (
                  <div key={group.label}>
                    {friendSections.length > 1 && (
                      <p className="text-muted-foreground/70 px-3 pt-2 text-[10px] uppercase">
                        {group.label}
                      </p>
                    )}
                    <AnimatePresence initial={false}>
                      {group.rows.map((friend) => (
                        <Row
                          key={friend.user_id}
                          name={friend.note_name || friend.nickname}
                          avatar={friend.avatar}
                          online={friend.online}
                          onClick={() => openChat.mutate(friend.user_id)}
                          trailing={
                            <div className="flex shrink-0 items-center gap-1">
                              {friend.status === BLOCKED_BY_ME && (
                                <Button
                                  variant="ghost"
                                  size="sm"
                                  className="text-muted-foreground h-6 px-2 text-xs"
                                  onClick={() => unblock.mutate(friend.user_id)}
                                >
                                  Unblock
                                </Button>
                              )}
                              <DropdownMenu>
                                <DropdownMenuTrigger
                                  render={
                                    <Button
                                      variant="ghost"
                                      size="icon"
                                      aria-label={`Options for ${friend.nickname}`}
                                      className="text-muted-foreground size-6 opacity-0 transition-opacity group-hover:opacity-100"
                                    />
                                  }
                                >
                                  <MoreHorizontal className="size-3.5" />
                                </DropdownMenuTrigger>
                                <DropdownMenuContent
                                  align="end"
                                  className="w-40"
                                >
                                  <DropdownMenuItem
                                    className="text-sm"
                                    onClick={() => setEditing(friend)}
                                  >
                                    Remark &amp; Tag
                                  </DropdownMenuItem>
                                </DropdownMenuContent>
                              </DropdownMenu>
                            </div>
                          }
                        />
                      ))}
                    </AnimatePresence>
                  </div>
                ))
              )}
            </Section>

            <Section
              title="My Groups"
              count={myGroups.data?.length ?? 0}
              open={myGroupsOpen}
              onOpenChange={setMyGroupsOpen}
            >
              {(myGroups.data ?? []).map((group) => (
                <Row
                  key={group.group_id}
                  name={group.name}
                  avatar={group.avatar}
                  onClick={() => openChat.mutate(group.group_id)}
                  trailing={
                    <span className="text-muted-foreground shrink-0 text-[10px] tabular-nums">
                      {group.member_cnt}
                    </span>
                  }
                />
              ))}
            </Section>

            <Section
              title="Joined Groups"
              count={joinedGroups.data?.length ?? 0}
              open={joinedGroupsOpen}
              onOpenChange={setJoinedGroupsOpen}
            >
              {(joinedGroups.data ?? []).map((group) => (
                <Row
                  key={group.group_id}
                  name={group.name}
                  avatar={group.avatar}
                  onClick={() => openChat.mutate(group.group_id)}
                  trailing={
                    <span className="text-muted-foreground shrink-0 text-[10px] tabular-nums">
                      {group.member_cnt}
                    </span>
                  }
                />
              ))}
            </Section>
          </>
        )}
      </div>

      <AddContactDialog open={addOpen} onOpenChange={setAddOpen} />
      <CreateGroupDialog open={createOpen} onOpenChange={setCreateOpen} />
      <ApplyListDialog
        open={requestsOpen}
        onOpenChange={setRequestsOpen}
        title="Friend Requests"
        emptyLabel="No pending friend requests"
        applies={applies.data ?? []}
        isPending={applies.isPending}
        allowBlock
      />
      <ContactSettingsDialog
        open={editing !== null}
        onOpenChange={(next) => {
          if (!next) setEditing(null);
        }}
        friend={editing}
      />
    </div>
  );
}
