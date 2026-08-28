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

import { useForm } from "@tanstack/react-form";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { ImageUp } from "lucide-react";
import Linkify from "linkify-react";
import { useState } from "react";
import { useDropzone } from "react-dropzone";
import type * as z from "zod";

import { ContactSidebar } from "@/components/contact-sidebar";
import { FormField } from "@/components/form-field";
import { ResetPasswordDialog } from "@/components/reset-password-dialog";
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
import { cn } from "@/lib/utils";
import { profileSchema } from "@/lib/validation";
import { message, user } from "@/service/api";
import { keys, profileQuery } from "@/service/queries";
import useAuthStore from "@/store/auth";
import { showToast } from "@/utils/toast";

type ProfileValues = z.infer<typeof profileSchema>;

function InfoLine({ label, value }: { label: string; value: string }) {
  return (
    <p>
      <span className="text-muted-foreground">{label}:</span>{" "}
      <span className="text-foreground">{value || "—"}</span>
    </p>
  );
}

export default function OwnInfo() {
  const queryClient = useQueryClient();
  const storedUser = useAuthStore((state) => state.userInfo);
  const userId = storedUser.uuid;

  const [editOpen, setEditOpen] = useState(false);
  const [passwordOpen, setPasswordOpen] = useState(false);
  const [avatarFile, setAvatarFile] = useState<File | null>(null);

  const profile = useQuery(profileQuery(userId));
  const me = profile.data ?? storedUser;

  const save = useMutation({
    mutationFn: async (values: ProfileValues) => {
      const patch: Parameters<typeof user.update>[0] = { uuid: userId };
      if (values.nickname) patch.nickname = values.nickname;
      if (values.email) patch.email = values.email;
      if (values.birthday) patch.birthday = values.birthday;
      if (values.signature) patch.signature = values.signature;
      if (avatarFile) {
        patch.avatar = (await message.uploadAvatar(avatarFile)).url;
      }
      await user.update(patch);
    },
    onSuccess: async () => {
      showToast("Profile updated", "success");
      // Re-read from the server so the nav rail avatar matches what was stored.
      await queryClient.invalidateQueries({
        queryKey: keys.users.profile(userId),
      });
      const fresh = queryClient.getQueryData(profileQuery(userId).queryKey);
      if (fresh) useAuthStore.getState().setUserInfo(fresh);
      closeEdit();
    },
  });

  const form = useForm({
    defaultValues: {
      nickname: "",
      email: "",
      birthday: "",
      signature: "",
    } as ProfileValues,
    validators: { onSubmit: profileSchema },
    onSubmit: ({ value }) => {
      const untouched =
        !value.nickname &&
        !value.email &&
        !value.birthday &&
        !value.signature &&
        !avatarFile;
      if (untouched) {
        showToast("Please modify at least one field", "warning");
        return;
      }
      save.mutate(value);
    },
  });

  const closeEdit = () => {
    setEditOpen(false);
    form.reset();
    setAvatarFile(null);
  };

  const { getRootProps, getInputProps, isDragActive } = useDropzone({
    accept: { "image/*": [".png", ".jpg", ".jpeg", ".gif", ".webp"] },
    multiple: false,
    maxSize: 5 * 1024 * 1024,
    onDrop: (accepted) => setAvatarFile(accepted[0] ?? null),
  });

  return (
    <>
      <div className="border-border w-55 shrink-0 border-r">
        <ContactSidebar />
      </div>

      <div className="relative flex flex-1 flex-col items-center justify-center p-8">
        <Avatar className="ring-primary/30 ring-offset-card mb-4 size-20 ring-2 ring-offset-2">
          <AvatarImage src={me.avatar || undefined} alt={me.nickname} />
          <AvatarFallback className="text-2xl">
            {me.nickname.charAt(0).toUpperCase() || "?"}
          </AvatarFallback>
        </Avatar>
        <h2 className="mb-6 text-xl font-semibold">{me.nickname}</h2>

        <div className="text-foreground flex flex-col gap-2 text-sm">
          <InfoLine label="User ID" value={me.uuid} />
          <InfoLine label="Phone" value={me.telephone} />
          <InfoLine label="Email" value={me.email} />
          <InfoLine
            label="Gender"
            value={me.gender === 0 ? "Male" : "Female"}
          />
          <InfoLine label="Birthday" value={me.birthday} />
          <InfoLine label="Joined" value={me.created_at} />
          <div>
            <span className="text-muted-foreground">Signature:</span>{" "}
            <Linkify
              as="span"
              options={{
                target: "_blank",
                rel: "noreferrer",
                className: "text-primary underline underline-offset-2",
              }}
            >
              {me.signature || "—"}
            </Linkify>
          </div>
        </div>

        <div className="absolute right-6 bottom-6 flex gap-2">
          <Button
            size="sm"
            variant="outline"
            onClick={() => setPasswordOpen(true)}
          >
            Change Password
          </Button>
          <Button size="sm" onClick={() => setEditOpen(true)}>
            Edit
          </Button>
        </div>

        <Dialog
          open={editOpen}
          onOpenChange={(next) => {
            if (!next) closeEdit();
          }}
        >
          <DialogContent className="sm:max-w-md">
            <DialogHeader>
              <DialogTitle>Edit Profile</DialogTitle>
            </DialogHeader>

            <form
              className="flex flex-col gap-3"
              onSubmit={(event) => {
                event.preventDefault();
                void form.handleSubmit();
              }}
            >
              <form.Field name="nickname">
                {(field) => (
                  <FormField
                    label="Nickname"
                    htmlFor="edit-nickname"
                    errors={field.state.meta.errors}
                  >
                    <Input
                      id="edit-nickname"
                      placeholder={me.nickname || "3-10 characters"}
                      value={field.state.value}
                      onBlur={field.handleBlur}
                      onChange={(event) =>
                        field.handleChange(event.target.value)
                      }
                    />
                  </FormField>
                )}
              </form.Field>

              <form.Field name="email">
                {(field) => (
                  <FormField
                    label="Email"
                    htmlFor="edit-email"
                    errors={field.state.meta.errors}
                  >
                    <Input
                      id="edit-email"
                      placeholder={me.email || "Optional"}
                      value={field.state.value}
                      onBlur={field.handleBlur}
                      onChange={(event) =>
                        field.handleChange(event.target.value)
                      }
                    />
                  </FormField>
                )}
              </form.Field>

              <form.Field name="birthday">
                {(field) => (
                  <FormField
                    label="Birthday"
                    htmlFor="edit-birthday"
                    errors={field.state.meta.errors}
                  >
                    <Input
                      id="edit-birthday"
                      type="date"
                      value={field.state.value}
                      onBlur={field.handleBlur}
                      onChange={(event) =>
                        field.handleChange(event.target.value)
                      }
                    />
                  </FormField>
                )}
              </form.Field>

              <form.Field name="signature">
                {(field) => (
                  <FormField
                    label="Signature"
                    htmlFor="edit-signature"
                    errors={field.state.meta.errors}
                  >
                    <Input
                      id="edit-signature"
                      placeholder={me.signature || "Optional"}
                      value={field.state.value}
                      onBlur={field.handleBlur}
                      onChange={(event) =>
                        field.handleChange(event.target.value)
                      }
                    />
                  </FormField>
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
                  variant="ghost"
                  size="sm"
                  onClick={closeEdit}
                >
                  Cancel
                </Button>
              </DialogFooter>
            </form>
          </DialogContent>
        </Dialog>

        <ResetPasswordDialog
          open={passwordOpen}
          onOpenChange={setPasswordOpen}
        />
      </div>
    </>
  );
}
