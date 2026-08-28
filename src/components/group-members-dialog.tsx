import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useVirtualizer } from "@tanstack/react-virtual";
import { xor } from "es-toolkit";
import { useRef, useState } from "react";

import { Avatar, AvatarFallback, AvatarImage } from "@/components/ui/avatar";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import {
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { group } from "@/service/api";
import { friendsQuery, groupMembersQuery, keys } from "@/service/queries";
import useAuthStore from "@/store/auth";
import { showToast } from "@/utils/toast";

const ROW_HEIGHT = 48;

interface PickerRow {
  id: string;
  name: string;
  avatar: string;
  note?: string;
}

interface VirtualPickerProps {
  rows: PickerRow[];
  selected: string[];
  emptyLabel: string;
  onToggle: (id: string) => void;
}

/** Member lists can run to hundreds of rows, so only the visible slice mounts. */
function VirtualPicker({
  rows,
  selected,
  emptyLabel,
  onToggle,
}: VirtualPickerProps) {
  const scrollRef = useRef<HTMLDivElement>(null);
  const virtualizer = useVirtualizer({
    count: rows.length,
    getScrollElement: () => scrollRef.current,
    estimateSize: () => ROW_HEIGHT,
    overscan: 6,
  });

  if (rows.length === 0) {
    return (
      <p className="text-muted-foreground py-6 text-center text-sm">
        {emptyLabel}
      </p>
    );
  }

  return (
    <div ref={scrollRef} className="h-60 overflow-y-auto">
      <div
        className="relative w-full"
        style={{ height: virtualizer.getTotalSize() }}
      >
        {virtualizer.getVirtualItems().map((item) => {
          const row = rows[item.index];
          const checked = selected.includes(row.id);
          return (
            <div
              key={row.id}
              className="border-border hover:bg-accent/50 absolute top-0 left-0 flex w-full cursor-pointer items-center justify-between gap-2 border-b px-2 transition-colors"
              style={{
                height: item.size,
                transform: `translateY(${item.start}px)`,
              }}
              onClick={() => onToggle(row.id)}
            >
              <div className="flex min-w-0 items-center gap-2">
                <Avatar className="size-8">
                  <AvatarImage src={row.avatar} alt={row.name} />
                  <AvatarFallback className="text-xs">
                    {row.name.charAt(0).toUpperCase() || "?"}
                  </AvatarFallback>
                </Avatar>
                <span className="text-foreground truncate text-sm">
                  {row.name}
                </span>
                {row.note && (
                  <span className="text-muted-foreground shrink-0 text-xs">
                    {row.note}
                  </span>
                )}
              </div>
              <Checkbox
                checked={checked}
                onCheckedChange={() => onToggle(row.id)}
              />
            </div>
          );
        })}
      </div>
    </div>
  );
}

interface GroupMembersDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  groupId: string;
  canManage: boolean;
}

export function GroupMembersDialog({
  open,
  onOpenChange,
  groupId,
  canManage,
}: GroupMembersDialogProps) {
  const queryClient = useQueryClient();
  const userId = useAuthStore((state) => state.userInfo.uuid);
  const [selectedMembers, setSelectedMembers] = useState<string[]>([]);
  const [selectedInvitees, setSelectedInvitees] = useState<string[]>([]);

  const members = useQuery({ ...groupMembersQuery(groupId), enabled: open });
  const friends = useQuery({
    ...friendsQuery(userId),
    enabled: open && canManage,
  });

  const refreshMembers = () => {
    void queryClient.invalidateQueries({ queryKey: keys.groups.all });
    void queryClient.invalidateQueries({ queryKey: keys.contacts.all });
  };

  const removeMembers = useMutation({
    mutationFn: () => group.removeMembers(groupId, selectedMembers),
    onSuccess: () => {
      showToast("Members removed", "success");
      setSelectedMembers([]);
      refreshMembers();
    },
  });

  const inviteMembers = useMutation({
    mutationFn: () => group.inviteMembers(groupId, selectedInvitees),
    onSuccess: () => {
      showToast("Invitations sent", "success");
      setSelectedInvitees([]);
      refreshMembers();
    },
  });

  const memberRows: PickerRow[] = (members.data ?? []).map((member) => ({
    id: member.user_id,
    name: member.nickname || member.user_id,
    avatar: member.avatar,
    note: member.is_owner ? "owner" : undefined,
  }));

  const memberIds = new Set(memberRows.map((row) => row.id));
  const inviteRows: PickerRow[] = (friends.data ?? [])
    .filter((friend) => !memberIds.has(friend.user_id))
    .map((friend) => ({
      id: friend.user_id,
      name: friend.note_name || friend.nickname || friend.user_id,
      avatar: friend.avatar,
    }));

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle>Group Members</DialogTitle>
        </DialogHeader>

        {canManage ? (
          <Tabs defaultValue="members">
            <TabsList>
              <TabsTrigger value="members">
                Members ({memberRows.length})
              </TabsTrigger>
              <TabsTrigger value="invite">
                Invite ({inviteRows.length})
              </TabsTrigger>
            </TabsList>

            <TabsContent value="members">
              <VirtualPicker
                rows={memberRows}
                selected={selectedMembers}
                emptyLabel={
                  members.isPending ? "Loading members…" : "No members found"
                }
                onToggle={(id) =>
                  setSelectedMembers((previous) => xor(previous, [id]))
                }
              />
              <Button
                variant="destructive"
                size="sm"
                className="mt-3"
                disabled={
                  selectedMembers.length === 0 || removeMembers.isPending
                }
                onClick={() => removeMembers.mutate()}
              >
                Remove selected ({selectedMembers.length})
              </Button>
            </TabsContent>

            <TabsContent value="invite">
              <VirtualPicker
                rows={inviteRows}
                selected={selectedInvitees}
                emptyLabel={
                  friends.isPending
                    ? "Loading contacts…"
                    : "Every contact is already a member"
                }
                onToggle={(id) =>
                  setSelectedInvitees((previous) => xor(previous, [id]))
                }
              />
              <Button
                size="sm"
                className="mt-3"
                disabled={
                  selectedInvitees.length === 0 || inviteMembers.isPending
                }
                onClick={() => inviteMembers.mutate()}
              >
                Invite selected ({selectedInvitees.length})
              </Button>
            </TabsContent>
          </Tabs>
        ) : (
          <VirtualPicker
            rows={memberRows}
            selected={[]}
            emptyLabel={
              members.isPending ? "Loading members…" : "No members found"
            }
            onToggle={() => undefined}
          />
        )}

        <DialogFooter>
          <Button variant="ghost" size="sm" onClick={() => onOpenChange(false)}>
            Close
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
