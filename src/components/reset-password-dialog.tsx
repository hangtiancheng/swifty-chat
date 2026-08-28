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
import { useMutation } from "@tanstack/react-query";
import type * as z from "zod";

import { FormField } from "@/components/form-field";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { resetPasswordSchema } from "@/lib/validation";
import { auth } from "@/service/api";
import { showToast } from "@/utils/toast";

interface ResetPasswordDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
}

/** `/user/update-password` is public and keyed on the phone number, so this
 * doubles as both "forgot password" and "change password". */
export function ResetPasswordDialog({
  open,
  onOpenChange,
}: ResetPasswordDialogProps) {
  const reset = useMutation({
    mutationFn: (values: z.infer<typeof resetPasswordSchema>) =>
      auth.updatePassword({
        telephone: values.telephone,
        password: values.password,
      }),
    onSuccess: () => {
      showToast("Password updated, please sign in", "success");
      onOpenChange(false);
    },
  });

  const form = useForm({
    defaultValues: { telephone: "", password: "", confirmPassword: "" },
    validators: { onSubmit: resetPasswordSchema },
    onSubmit: ({ value }) => {
      reset.mutate(value);
    },
  });

  return (
    <Dialog
      open={open}
      onOpenChange={(next) => {
        if (!next) form.reset();
        onOpenChange(next);
      }}
    >
      <DialogContent className="sm:max-w-sm">
        <DialogHeader>
          <DialogTitle>Reset Password</DialogTitle>
        </DialogHeader>
        <form
          className="flex flex-col gap-3"
          onSubmit={(event) => {
            event.preventDefault();
            void form.handleSubmit();
          }}
        >
          <form.Field name="telephone">
            {(field) => (
              <FormField
                label="Phone"
                htmlFor="reset-phone"
                errors={field.state.meta.errors}
              >
                <Input
                  id="reset-phone"
                  inputMode="numeric"
                  autoComplete="tel"
                  placeholder="Registered phone number"
                  value={field.state.value}
                  onBlur={field.handleBlur}
                  onChange={(event) => field.handleChange(event.target.value)}
                />
              </FormField>
            )}
          </form.Field>

          <form.Field name="password">
            {(field) => (
              <FormField
                label="New Password"
                htmlFor="reset-password"
                errors={field.state.meta.errors}
              >
                <Input
                  id="reset-password"
                  type="password"
                  autoComplete="new-password"
                  placeholder="At least 6 characters"
                  value={field.state.value}
                  onBlur={field.handleBlur}
                  onChange={(event) => field.handleChange(event.target.value)}
                />
              </FormField>
            )}
          </form.Field>

          <form.Field name="confirmPassword">
            {(field) => (
              <FormField
                label="Confirm Password"
                htmlFor="reset-confirm"
                errors={field.state.meta.errors}
              >
                <Input
                  id="reset-confirm"
                  type="password"
                  autoComplete="new-password"
                  placeholder="Repeat the new password"
                  value={field.state.value}
                  onBlur={field.handleBlur}
                  onChange={(event) => field.handleChange(event.target.value)}
                />
              </FormField>
            )}
          </form.Field>

          <DialogFooter>
            <Button type="submit" size="sm" disabled={reset.isPending}>
              {reset.isPending ? "Saving…" : "Save"}
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
