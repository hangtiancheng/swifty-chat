import { useForm } from "@tanstack/react-form";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { ImageUp } from "lucide-react";
import { useState } from "react";
import { useDropzone } from "react-dropzone";
import type * as z from "zod";

import { FormField } from "@/components/form-field";
import { Avatar, AvatarFallback, AvatarImage } from "@/components/ui/avatar";
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
import { RadioGroup, RadioGroupItem } from "@/components/ui/radio-group";
import { Textarea } from "@/components/ui/textarea";
import { cn } from "@/lib/utils";
import { groupSettingsSchema } from "@/lib/validation";
import { group, message } from "@/service/api";
import { groupInfoQuery, keys } from "@/service/queries";
import { showToast } from "@/utils/toast";

type GroupSettings = z.infer<typeof groupSettingsSchema>;

interface GroupSettingsDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  groupId: string;
}

export function GroupSettingsDialog({
  open,
  onOpenChange,
  groupId,
}: GroupSettingsDialogProps) {
  const queryClient = useQueryClient();
  const [addMode, setAddMode] = useState<number>(-1);
  const [avatarFile, setAvatarFile] = useState<File | null>(null);

  const info = useQuery({ ...groupInfoQuery(groupId), enabled: open });

  const save = useMutation({
    mutationFn: async (values: GroupSettings) => {
      const patch: Parameters<typeof group.update>[0] = { uuid: groupId };
      if (values.name) patch.name = values.name;
      if (values.notice) patch.notice = values.notice;
      if (addMode !== -1) patch.add_mode = addMode;
      if (avatarFile) {
        patch.avatar = (await message.uploadAvatar(avatarFile)).url;
      }
      await group.update(patch);
    },
    onSuccess: () => {
      showToast("Group updated", "success");
      void queryClient.invalidateQueries({ queryKey: keys.groups.all });
      void queryClient.invalidateQueries({ queryKey: keys.contacts.all });
      onOpenChange(false);
    },
  });

  const form = useForm({
    defaultValues: { name: "", notice: "" } as GroupSettings,
    validators: { onSubmit: groupSettingsSchema },
    onSubmit: ({ value }) => {
      if (!value.name && !value.notice && addMode === -1 && !avatarFile) {
        showToast("Please modify at least one field", "warning");
        return;
      }
      save.mutate(value);
    },
  });

  const { getRootProps, getInputProps, isDragActive } = useDropzone({
    accept: { "image/*": [".png", ".jpg", ".jpeg", ".gif", ".webp"] },
    multiple: false,
    maxSize: 5 * 1024 * 1024,
    onDrop: (accepted) => setAvatarFile(accepted[0] ?? null),
  });

  const reset = () => {
    form.reset();
    setAddMode(-1);
    setAvatarFile(null);
  };

  return (
    <Dialog
      open={open}
      onOpenChange={(next) => {
        if (!next) reset();
        onOpenChange(next);
      }}
    >
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle>Edit Group</DialogTitle>
        </DialogHeader>

        <form
          className="flex flex-col gap-3"
          onSubmit={(event) => {
            event.preventDefault();
            void form.handleSubmit();
          }}
        >
          <form.Field name="name">
            {(field) => (
              <FormField
                label="Group Name"
                htmlFor="group-name"
                errors={field.state.meta.errors}
              >
                <Input
                  id="group-name"
                  placeholder={info.data?.name || "3-10 characters"}
                  value={field.state.value}
                  onBlur={field.handleBlur}
                  onChange={(event) => field.handleChange(event.target.value)}
                />
              </FormField>
            )}
          </form.Field>

          <form.Field name="notice">
            {(field) => (
              <FormField
                label="Notice"
                htmlFor="group-notice"
                errors={field.state.meta.errors}
              >
                <Textarea
                  id="group-notice"
                  rows={3}
                  maxLength={500}
                  placeholder={info.data?.notice || "Optional"}
                  value={field.state.value}
                  onBlur={field.handleBlur}
                  onChange={(event) => field.handleChange(event.target.value)}
                />
              </FormField>
            )}
          </form.Field>

          <div className="flex flex-col gap-1.5">
            <Label>Join Mode</Label>
            <RadioGroup
              value={addMode === -1 ? undefined : String(addMode)}
              onValueChange={(value) => setAddMode(Number(value))}
              className="flex gap-4"
            >
              <div className="flex items-center gap-2">
                <RadioGroupItem value="0" id="group-addmode-0" />
                <Label htmlFor="group-addmode-0" className="font-normal">
                  Direct Join
                </Label>
              </div>
              <div className="flex items-center gap-2">
                <RadioGroupItem value="1" id="group-addmode-1" />
                <Label htmlFor="group-addmode-1" className="font-normal">
                  Owner Approval
                </Label>
              </div>
            </RadioGroup>
          </div>

          <div className="flex flex-col gap-1.5">
            <Label>Avatar</Label>
            <div
              {...getRootProps()}
              className={cn(
                "border-border hover:bg-accent/40 flex cursor-pointer items-center gap-3 rounded-md border border-dashed px-3 py-3 transition-colors",
                isDragActive && "border-primary bg-primary/5",
              )}
            >
              <input {...getInputProps()} />
              {avatarFile ? (
                <Avatar className="size-9">
                  <AvatarImage
                    src={URL.createObjectURL(avatarFile)}
                    alt={avatarFile.name}
                  />
                  <AvatarFallback>?</AvatarFallback>
                </Avatar>
              ) : (
                <ImageUp className="text-muted-foreground size-5" />
              )}
              <span className="text-muted-foreground truncate text-xs">
                {avatarFile
                  ? avatarFile.name
                  : "Drop an image here, or click to choose one"}
              </span>
            </div>
          </div>

          <DialogFooter>
            <Button type="submit" size="sm" disabled={save.isPending}>
              {save.isPending ? "Saving…" : "Save"}
            </Button>
            <Button
              type="button"
              size="sm"
              variant="ghost"
              onClick={() => onOpenChange(false)}
            >
              Cancel
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}
