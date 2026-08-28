import { useForm } from "@tanstack/react-form";
import { useMutation, useQueryClient } from "@tanstack/react-query";
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
import { createGroupSchema } from "@/lib/validation";
import { group, message } from "@/service/api";
import { keys } from "@/service/queries";
import useAuthStore from "@/store/auth";
import { showToast } from "@/utils/toast";

type CreateGroupValues = z.infer<typeof createGroupSchema>;

interface CreateGroupDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
}

export function CreateGroupDialog({
  open,
  onOpenChange,
}: CreateGroupDialogProps) {
  const queryClient = useQueryClient();
  const userId = useAuthStore((state) => state.userInfo.uuid);
  const [avatarFile, setAvatarFile] = useState<File | null>(null);

  const create = useMutation({
    mutationFn: async (values: CreateGroupValues) => {
      const avatar = avatarFile
        ? (await message.uploadAvatar(avatarFile)).url
        : "";
      await group.create({
        name: values.name,
        owner_id: userId,
        avatar,
        notice: values.notice,
        add_mode: values.addMode,
      });
    },
    onSuccess: () => {
      showToast("Group created — invite members from Group Members", "success");
      void queryClient.invalidateQueries({ queryKey: keys.groups.all });
      void queryClient.invalidateQueries({ queryKey: keys.sessions.all });
      onOpenChange(false);
    },
  });

  const form = useForm({
    defaultValues: { name: "", notice: "", addMode: 0 } as CreateGroupValues,
    validators: { onSubmit: createGroupSchema },
    onSubmit: ({ value }) => {
      create.mutate(value);
    },
  });

  const { getRootProps, getInputProps, isDragActive } = useDropzone({
    accept: { "image/*": [".png", ".jpg", ".jpeg", ".gif", ".webp"] },
    multiple: false,
    maxSize: 5 * 1024 * 1024,
    onDrop: (accepted) => setAvatarFile(accepted[0] ?? null),
  });

  return (
    <Dialog
      open={open}
      onOpenChange={(next) => {
        if (!next) {
          form.reset();
          setAvatarFile(null);
        }
        onOpenChange(next);
      }}
    >
      <DialogContent className="sm:max-w-sm">
        <DialogHeader>
          <DialogTitle>Create Group</DialogTitle>
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
                htmlFor="create-group-name"
                errors={field.state.meta.errors}
              >
                <Input
                  id="create-group-name"
                  placeholder="3-10 characters"
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
                htmlFor="create-group-notice"
                errors={field.state.meta.errors}
              >
                <Textarea
                  id="create-group-notice"
                  rows={2}
                  maxLength={500}
                  placeholder="Optional"
                  value={field.state.value}
                  onBlur={field.handleBlur}
                  onChange={(event) => field.handleChange(event.target.value)}
                />
              </FormField>
            )}
          </form.Field>

          <form.Field name="addMode">
            {(field) => (
              <div className="flex flex-col gap-1.5">
                <Label>Join Mode</Label>
                <RadioGroup
                  value={String(field.state.value)}
                  onValueChange={(value) =>
                    field.handleChange(Number(value) === 1 ? 1 : 0)
                  }
                  className="flex gap-4"
                >
                  <div className="flex items-center gap-2">
                    <RadioGroupItem value="0" id="create-addmode-0" />
                    <Label htmlFor="create-addmode-0" className="font-normal">
                      Direct Join
                    </Label>
                  </div>
                  <div className="flex items-center gap-2">
                    <RadioGroupItem value="1" id="create-addmode-1" />
                    <Label htmlFor="create-addmode-1" className="font-normal">
                      Owner Approval
                    </Label>
                  </div>
                </RadioGroup>
              </div>
            )}
          </form.Field>

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
                  : "Optional — drop an image or click to choose"}
              </span>
            </div>
          </div>

          <DialogFooter>
            <Button type="submit" size="sm" disabled={create.isPending}>
              {create.isPending ? "Creating…" : "Create"}
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
