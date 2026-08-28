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
import { useState } from "react";
import { useNavigate } from "react-router-dom";
import type * as z from "zod";

import { AuthLayout } from "@/components/auth-layout";
import { FormField } from "@/components/form-field";
import { ResetPasswordDialog } from "@/components/reset-password-dialog";
import { Button } from "@/components/ui/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardFooter,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { Checkbox } from "@/components/ui/checkbox";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { loginSchema } from "@/lib/validation";
import { auth } from "@/service/api";
import useAuthStore from "@/store/auth";
import usePreferencesStore from "@/store/preferences";
import { showToast } from "@/utils/toast";

const BANNED = 1;

export default function Login() {
  const navigate = useNavigate();
  const rememberedPhone = usePreferencesStore((state) => state.rememberedPhone);
  const setRememberedPhone = usePreferencesStore(
    (state) => state.setRememberedPhone,
  );
  const [remember, setRemember] = useState(Boolean(rememberedPhone));
  const [resetOpen, setResetOpen] = useState(false);

  const signIn = useMutation({
    mutationFn: (values: z.infer<typeof loginSchema>) => auth.login(values),
    onSuccess: (result, values) => {
      if (result.user_info.status === BANNED) {
        showToast("This account has been banned", "error");
        return;
      }
      setRememberedPhone(remember ? values.telephone : "");
      // The socket connects from App once a uuid lands in the store.
      useAuthStore.getState().setAuth(result);
      navigate("/chat/sessions", { replace: true });
    },
  });

  const form = useForm({
    defaultValues: { telephone: rememberedPhone, password: "" },
    validators: { onSubmit: loginSchema },
    onSubmit: ({ value }) => {
      signIn.mutate(value);
    },
  });

  return (
    <AuthLayout>
      <Card className="shadow-primary/5 shadow-xl">
        <CardHeader>
          <CardTitle className="text-2xl font-semibold tracking-tight">
            Sign In
          </CardTitle>
          <CardDescription>Welcome back to Swifty Chat</CardDescription>
        </CardHeader>
        <form
          onSubmit={(event) => {
            event.preventDefault();
            void form.handleSubmit();
          }}
        >
          <CardContent className="flex flex-col gap-4">
            <form.Field name="telephone">
              {(field) => (
                <FormField
                  label="Phone"
                  htmlFor="login-phone"
                  errors={field.state.meta.errors}
                >
                  <Input
                    id="login-phone"
                    inputMode="numeric"
                    autoComplete="tel"
                    placeholder="Enter your phone number"
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
                  label="Password"
                  htmlFor="login-password"
                  errors={field.state.meta.errors}
                >
                  <Input
                    id="login-password"
                    type="password"
                    autoComplete="current-password"
                    placeholder="Enter your password"
                    value={field.state.value}
                    onBlur={field.handleBlur}
                    onChange={(event) => field.handleChange(event.target.value)}
                  />
                </FormField>
              )}
            </form.Field>

            <div className="flex items-center gap-2">
              <Checkbox
                id="login-remember"
                checked={remember}
                onCheckedChange={(checked) => setRemember(checked === true)}
              />
              <Label htmlFor="login-remember" className="font-normal">
                Remember my phone number
              </Label>
            </div>
          </CardContent>

          <CardFooter className="mt-4 flex-col gap-3">
            <Button
              type="submit"
              className="w-full"
              disabled={signIn.isPending}
            >
              {signIn.isPending ? "Signing in…" : "Sign In"}
            </Button>
            <div className="flex w-full justify-between">
              <button
                type="button"
                className="text-muted-foreground hover:text-foreground cursor-pointer text-sm"
                onClick={() => setResetOpen(true)}
              >
                Forgot password?
              </button>
              <button
                type="button"
                className="text-primary cursor-pointer text-sm hover:underline"
                onClick={() => navigate("/register")}
              >
                Register
              </button>
            </div>
          </CardFooter>
        </form>
      </Card>

      <ResetPasswordDialog open={resetOpen} onOpenChange={setResetOpen} />
    </AuthLayout>
  );
}
