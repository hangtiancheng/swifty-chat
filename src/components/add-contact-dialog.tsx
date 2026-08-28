import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";

import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Textarea } from "@/components/ui/textarea";
import { contact, group } from "@/service/api";
import { keys } from "@/service/queries";
import { isGroupId } from "@/service/schemas";
import useAuthStore from "@/store/auth";
import { showToast } from "@/utils/toast";

const CONTACT_APPLY = 0;
const GROUP_APPLY = 1;
const DIRECT_JOIN = 0;

interface AddContactDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  /** Prefilled when the dialog is opened from a search result. */
  initialId?: string;
}

export function AddContactDialog({
  open,
  onOpenChange,
  initialId = "",
}: AddContactDialogProps) {
  const queryClient = useQueryClient();
  const userId = useAuthStore((state) => state.userInfo.uuid);
  const [targetId, setTargetId] = useState(initialId);
  const [note, setNote] = useState("");

  const submit = useMutation({
    mutationFn: async () => {
      const id = targetId.trim();
      if (!isGroupId(id)) {
        await contact.apply({
          user_id: userId,
          contact_id: id,
          contact_type: CONTACT_APPLY,
          message: note,
        });
        return "Application sent";
      }
      // An open group lets anyone in without the owner approving.
      if ((await group.addMode(id)) === DIRECT_JOIN) {
        await group.enterDirectly(userId, id);
        return "Joined group";
      }
      await contact.apply({
        user_id: userId,
        contact_id: id,
        contact_type: GROUP_APPLY,
        message: note,
      });
      return "Application sent";
    },
    onSuccess: (message) => {
      showToast(message, "success");
      void queryClient.invalidateQueries({ queryKey: keys.contacts.all });
      void queryClient.invalidateQueries({ queryKey: keys.groups.all });
      onOpenChange(false);
    },
  });

  return (
    <Dialog
      open={open}
      onOpenChange={(next) => {
        if (!next) {
          setTargetId(initialId);
          setNote("");
        }
        onOpenChange(next);
      }}
    >
      <DialogContent className="sm:max-w-sm">
        <DialogHeader>
          <DialogTitle>Add Contact / Group</DialogTitle>
        </DialogHeader>
        <div className="flex flex-col gap-3">
          <div className="flex flex-col gap-1.5">
            <Label htmlFor="apply-id">User / Group ID</Label>
            <Input
              id="apply-id"
              placeholder="Starts with U or G"
              value={targetId}
              onChange={(event) => setTargetId(event.target.value)}
            />
          </div>
          <div className="flex flex-col gap-1.5">
            <Label htmlFor="apply-message">Message</Label>
            <Textarea
              id="apply-message"
              rows={2}
              maxLength={100}
              placeholder="Optional"
              value={note}
              onChange={(event) => setNote(event.target.value)}
            />
          </div>
        </div>
        <DialogFooter>
          <Button
            size="sm"
            disabled={!targetId.trim() || submit.isPending}
            onClick={() => submit.mutate()}
          >
            {submit.isPending ? "Sending…" : "Submit"}
          </Button>
          <Button size="sm" variant="ghost" onClick={() => onOpenChange(false)}>
            Cancel
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
