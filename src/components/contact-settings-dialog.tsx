import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
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
import { cn } from "@/lib/utils";
import { contact } from "@/service/api";
import { keys, tagsQuery } from "@/service/queries";
import type { Friend } from "@/service/schemas";
import useAuthStore from "@/store/auth";
import { showToast } from "@/utils/toast";

interface ContactSettingsDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  friend: Friend | null;
}

/** Per-contact remark plus tag assignment (`/contact/update-contact`). */
export function ContactSettingsDialog({
  open,
  onOpenChange,
  friend,
}: ContactSettingsDialogProps) {
  const queryClient = useQueryClient();
  const userId = useAuthStore((state) => state.userInfo.uuid);

  const [noteName, setNoteName] = useState(friend?.note_name ?? "");
  const [tagId, setTagId] = useState(friend?.tag_id ?? "");
  const [newTag, setNewTag] = useState("");

  const tags = useQuery({ ...tagsQuery(userId), enabled: open });

  const refreshContacts = () =>
    queryClient.invalidateQueries({ queryKey: keys.contacts.all });

  const createTag = useMutation({
    mutationFn: (name: string) => contact.addTag(userId, name),
    onSuccess: (tag) => {
      setTagId(tag.tag_id);
      setNewTag("");
      void refreshContacts();
    },
  });

  const save = useMutation({
    mutationFn: () =>
      contact.update({
        user_id: userId,
        contact_id: friend?.user_id ?? "",
        note_name: noteName,
        tag_id: tagId,
      }),
    onSuccess: () => {
      showToast("Contact updated", "success");
      void refreshContacts();
      onOpenChange(false);
    },
  });

  return (
    <Dialog
      open={open}
      onOpenChange={(next) => {
        if (next && friend) {
          setNoteName(friend.note_name);
          setTagId(friend.tag_id);
        }
        onOpenChange(next);
      }}
    >
      <DialogContent className="sm:max-w-sm">
        <DialogHeader>
          <DialogTitle>{friend?.nickname ?? "Contact"} settings</DialogTitle>
        </DialogHeader>

        <div className="flex flex-col gap-3">
          <div className="flex flex-col gap-1.5">
            <Label htmlFor="contact-note">Remark</Label>
            <Input
              id="contact-note"
              placeholder={friend?.nickname || "Display name for this contact"}
              value={noteName}
              onChange={(event) => setNoteName(event.target.value)}
            />
          </div>

          <div className="flex flex-col gap-1.5">
            <Label>Tag</Label>
            <div className="flex flex-wrap gap-1.5">
              <button
                type="button"
                onClick={() => setTagId("")}
                className={cn(
                  "border-border cursor-pointer rounded-full border px-2.5 py-1 text-xs transition-colors",
                  tagId === ""
                    ? "bg-primary text-primary-foreground"
                    : "hover:bg-accent",
                )}
              >
                None
              </button>
              {(tags.data ?? []).map((tag) => (
                <button
                  key={tag.tag_id}
                  type="button"
                  onClick={() => setTagId(tag.tag_id)}
                  className={cn(
                    "border-border cursor-pointer rounded-full border px-2.5 py-1 text-xs transition-colors",
                    tagId === tag.tag_id
                      ? "bg-primary text-primary-foreground"
                      : "hover:bg-accent",
                  )}
                >
                  {tag.name}
                </button>
              ))}
            </div>
          </div>

          <div className="flex items-end gap-2">
            <div className="flex flex-1 flex-col gap-1.5">
              <Label htmlFor="contact-new-tag">New tag</Label>
              <Input
                id="contact-new-tag"
                placeholder="e.g. Work"
                value={newTag}
                onChange={(event) => setNewTag(event.target.value)}
              />
            </div>
            <Button
              size="sm"
              variant="outline"
              disabled={!newTag.trim() || createTag.isPending}
              onClick={() => createTag.mutate(newTag.trim())}
            >
              Add
            </Button>
          </div>
        </div>

        <DialogFooter>
          <Button
            size="sm"
            disabled={!friend || save.isPending}
            onClick={() => save.mutate()}
          >
            {save.isPending ? "Saving…" : "Save"}
          </Button>
          <Button size="sm" variant="ghost" onClick={() => onOpenChange(false)}>
            Cancel
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
