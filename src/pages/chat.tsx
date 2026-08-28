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

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { MoreVertical, Phone, Video } from "lucide-react";
import { useEffect, useState } from "react";
import { useNavigate, useParams } from "react-router-dom";
import { useStickToBottom } from "use-stick-to-bottom";

import { ContactDetailDialog } from "@/components/contact-detail-dialog";
import { GroupMembersDialog } from "@/components/group-members-dialog";
import { GroupRequestsDialog } from "@/components/group-requests-dialog";
import { GroupSettingsDialog } from "@/components/group-settings-dialog";
import { MessageBubble } from "@/components/message-bubble";
import {
  MessageComposer,
  type ComposerPayload,
} from "@/components/message-composer";
import { SessionSidebar } from "@/components/session-sidebar";
import { Avatar, AvatarFallback, AvatarImage } from "@/components/ui/avatar";
import { Button } from "@/components/ui/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from "@/components/ui/tooltip";
import { contact, group, session } from "@/service/api";
import { errorMessage } from "@/service/http";
import {
  contactInfoQuery,
  keys,
  messagesQuery,
  openSessionQuery,
} from "@/service/queries";
import { isGroupId } from "@/service/schemas";
import useAuthStore from "@/store/auth";
import useCallStore from "@/store/call";
import useWsStore from "@/store/ws";
import type { CallMedia } from "@/utils/rtc";
import { showToast } from "@/utils/toast";

type OpenDialog = "user" | "group" | "settings" | "members" | "requests";

/** These all end the same way: toast, refresh a list, leave the conversation. */
interface ConversationExit {
  run: () => Promise<void>;
  message: string;
  staleKey: readonly unknown[];
}

export default function Chat() {
  const { id = "" } = useParams<{ id: string }>();
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const userInfo = useAuthStore((state) => state.userInfo);
  const userId = userInfo.uuid;

  const [dialog, setDialog] = useState<OpenDialog | null>(null);

  const contactInfo = useQuery(contactInfoQuery(userId, id));
  const openSession = useQuery(openSessionQuery(userId, id));
  const messages = useQuery(messagesQuery(userId, id));

  const isGroup = isGroupId(id);
  const isOwner = Boolean(
    contactInfo.data && contactInfo.data.contact_owner_id === userId,
  );

  const { scrollRef, contentRef } = useStickToBottom({
    initial: "instant",
    resize: "smooth",
  });

  const { mutate: markAsRead } = useMutation({
    mutationFn: () => session.markRead(userId, id),
    onSuccess: () =>
      queryClient.invalidateQueries({ queryKey: keys.sessions.all }),
  });

  // Clearing the badge waits for the transcript, so it happens once per open.
  useEffect(() => {
    if (id && userId && messages.isSuccess) markAsRead();
  }, [id, userId, messages.isSuccess, markAsRead]);

  const leaveConversation = useMutation({
    mutationFn: (exit: ConversationExit) => exit.run(),
    onSuccess: (_result, exit) => {
      showToast(exit.message, "success");
      void queryClient.invalidateQueries({ queryKey: exit.staleKey });
      navigate("/chat/sessions");
    },
  });

  const sendMessage = (payload: ComposerPayload) => {
    if (!contactInfo.data) return;
    useWsStore.getState().send({
      ...payload,
      session_id: openSession.data ?? "",
      send_id: userInfo.uuid,
      send_name: userInfo.nickname,
      send_avatar: userInfo.avatar,
      receive_id: contactInfo.data.contact_id,
    });
  };

  const name = contactInfo.data?.contact_name ?? "";
  const avatar = contactInfo.data?.contact_avatar ?? "";

  const startCall = (media: CallMedia) => {
    if (!contactInfo.data) return;
    useCallStore.getState().dial({
      conversationId: contactInfo.data.contact_id,
      sessionId: openSession.data ?? "",
      title: name,
      media,
    });
  };

  return (
    <>
      <div className="border-border w-55 shrink-0 border-r">
        <SessionSidebar />
      </div>

      <div className="flex min-w-0 flex-1 flex-col">
        <div className="border-border bg-muted/30 flex h-14 shrink-0 items-center justify-between border-b px-4">
          <div className="flex min-w-0 items-center gap-3">
            {avatar && (
              <Avatar className="ring-border ring-offset-card size-10 ring-2 ring-offset-2">
                <AvatarImage src={avatar} alt={name} />
                <AvatarFallback>{name.charAt(0) || "?"}</AvatarFallback>
              </Avatar>
            )}
            <h2 className="text-foreground truncate text-base font-semibold">
              {name || (contactInfo.isPending ? "Loading…" : "Unknown")}
            </h2>
          </div>

          <div className="flex items-center gap-1">
            <Tooltip>
              <TooltipTrigger
                render={
                  <Button
                    variant="ghost"
                    size="icon"
                    className="text-muted-foreground"
                    aria-label="Audio call"
                    disabled={!contactInfo.data}
                    onClick={() => startCall("audio")}
                  />
                }
              >
                <Phone className="size-4" />
              </TooltipTrigger>
              <TooltipContent side="bottom">Audio Call</TooltipContent>
            </Tooltip>

            <Tooltip>
              <TooltipTrigger
                render={
                  <Button
                    variant="ghost"
                    size="icon"
                    className="text-muted-foreground"
                    aria-label="Video call"
                    disabled={!contactInfo.data}
                    onClick={() => startCall("video")}
                  />
                }
              >
                <Video className="size-4" />
              </TooltipTrigger>
              <TooltipContent side="bottom">Video Call</TooltipContent>
            </Tooltip>

            <DropdownMenu>
              <DropdownMenuTrigger
                render={
                  <Button
                    variant="ghost"
                    size="icon"
                    className="text-muted-foreground"
                    aria-label="Chat options"
                  />
                }
              >
                <MoreVertical className="size-4" />
              </DropdownMenuTrigger>
              <DropdownMenuContent align="end" className="w-44">
                <DropdownMenuItem
                  className="text-sm"
                  onClick={() => setDialog(isGroup ? "group" : "user")}
                >
                  {isGroup ? "Group Info" : "User Info"}
                </DropdownMenuItem>

                {isGroup && (
                  <DropdownMenuItem
                    className="text-sm"
                    onClick={() => setDialog("members")}
                  >
                    Members
                  </DropdownMenuItem>
                )}
                {isGroup && isOwner && (
                  <>
                    <DropdownMenuItem
                      className="text-sm"
                      onClick={() => setDialog("settings")}
                    >
                      Edit Group
                    </DropdownMenuItem>
                    <DropdownMenuItem
                      className="text-sm"
                      onClick={() => setDialog("requests")}
                    >
                      Join Requests
                    </DropdownMenuItem>
                  </>
                )}

                <DropdownMenuItem
                  className="text-sm"
                  onClick={() =>
                    leaveConversation.mutate({
                      run: () => session.remove(userId, openSession.data ?? ""),
                      message: "Session deleted",
                      staleKey: keys.sessions.all,
                    })
                  }
                >
                  Delete Session
                </DropdownMenuItem>

                {!isGroup && (
                  <>
                    <DropdownMenuItem
                      className="text-sm"
                      onClick={() =>
                        leaveConversation.mutate({
                          run: () => contact.remove(userId, id),
                          message: "Contact removed",
                          staleKey: keys.contacts.all,
                        })
                      }
                    >
                      Remove Contact
                    </DropdownMenuItem>
                    <DropdownMenuItem
                      className="text-destructive focus:text-destructive text-sm"
                      onClick={() =>
                        leaveConversation.mutate({
                          run: () => contact.block(userId, id),
                          message: "Contact blocked",
                          staleKey: keys.contacts.all,
                        })
                      }
                    >
                      Block Contact
                    </DropdownMenuItem>
                  </>
                )}

                {isGroup &&
                  (isOwner ? (
                    <DropdownMenuItem
                      className="text-destructive focus:text-destructive text-sm"
                      onClick={() =>
                        leaveConversation.mutate({
                          run: () => group.dismiss(id),
                          message: "Group disbanded",
                          staleKey: keys.groups.all,
                        })
                      }
                    >
                      Disband Group
                    </DropdownMenuItem>
                  ) : (
                    <DropdownMenuItem
                      className="text-sm"
                      onClick={() =>
                        leaveConversation.mutate({
                          run: () => group.leave(userId, id),
                          message: "Left group",
                          staleKey: keys.groups.all,
                        })
                      }
                    >
                      Leave Group
                    </DropdownMenuItem>
                  ))}
              </DropdownMenuContent>
            </DropdownMenu>
          </div>
        </div>

        <div ref={scrollRef} className="bg-muted/20 flex-1 overflow-y-auto p-4">
          <div ref={contentRef}>
            {messages.isError ? (
              <p className="text-destructive py-10 text-center text-sm">
                {errorMessage(messages.error)}
              </p>
            ) : messages.isPending ? (
              <p className="text-muted-foreground animate-pulse py-10 text-center text-sm">
                Loading messages…
              </p>
            ) : (
              <MessageBubble
                messageList={messages.data}
                currentUserId={userInfo.uuid}
                currentUserAvatar={userInfo.avatar}
                currentUserName={userInfo.nickname}
              />
            )}
          </div>
        </div>

        <MessageComposer disabled={!contactInfo.data} onSend={sendMessage} />
      </div>

      <ContactDetailDialog
        open={dialog === "user" || dialog === "group"}
        onOpenChange={(next) => setDialog(next ? dialog : null)}
        contact={contactInfo.data}
        isGroup={isGroup}
      />
      <GroupSettingsDialog
        open={dialog === "settings"}
        onOpenChange={(next) => setDialog(next ? "settings" : null)}
        groupId={id}
      />
      <GroupMembersDialog
        open={dialog === "members"}
        onOpenChange={(next) => setDialog(next ? "members" : null)}
        groupId={id}
        canManage={isOwner}
      />
      <GroupRequestsDialog
        open={dialog === "requests"}
        onOpenChange={(next) => setDialog(next ? "requests" : null)}
      />
    </>
  );
}
