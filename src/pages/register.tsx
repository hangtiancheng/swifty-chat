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
import { useNavigate } from "react-router-dom";
import type * as z from "zod";

import { AuthLayout } from "@/components/auth-layout";
import { FormField } from "@/components/form-field";
import { Button } from "@/components/ui/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardFooter,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { registerSchema } from "@/lib/validation";
import { auth } from "@/service/api";
import useAuthStore from "@/store/auth";
import { showToast } from "@/utils/toast";

export default function Register() {
  const navigate = useNavigate();

  const signUp = useMutation({
    mutationFn: (values: z.infer<typeof registerSchema>) =>
      auth.register({
        nickname: values.nickname,
        telephone: values.telephone,
        password: values.password,
      }),
    onSuccess: (result) => {
      showToast("Welcome to Swifty Chat", "success");
      useAuthStore.getState().setAuth(result);
      navigate("/chat/sessions", { replace: true });
    },
  });

  const form = useForm({
    defaultValues: {
      nickname: "",
      telephone: "",
      password: "",
      confirmPassword: "",
    },
    validators: { onSubmit: registerSchema },
    onSubmit: ({ value }) => {
      signUp.mutate(value);
    },
  });

  return (
    <AuthLayout>
      <Card className="shadow-primary/5 shadow-xl">
        <CardHeader>
          <CardTitle className="text-2xl font-semibold tracking-tight">
            Create Account
          </CardTitle>
          <CardDescription>Join Swifty Chat in a few seconds</CardDescription>
        </CardHeader>
        <form
          onSubmit={(event) => {
            event.preventDefault();
            void form.handleSubmit();
          }}
        >
          <CardContent className="flex flex-col gap-4">
            <form.Field name="nickname">
              {(field) => (
                <FormField
                  label="Nickname"
                  htmlFor="register-nickname"
                  errors={field.state.meta.errors}
                >
                  <Input
                    id="register-nickname"
                    autoComplete="nickname"
                    placeholder="3-10 characters"
                    value={field.state.value}
                    onBlur={field.handleBlur}
                    onChange={(event) => field.handleChange(event.target.value)}
                  />
                </FormField>
              )}
            </form.Field>

            <form.Field name="telephone">
              {(field) => (
                <FormField
                  label="Phone"
                  htmlFor="register-phone"
                  errors={field.state.meta.errors}
                >
                  <Input
                    id="register-phone"
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
                  htmlFor="register-password"
                  errors={field.state.meta.errors}
                >
                  <Input
                    id="register-password"
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
                  htmlFor="register-confirm"
                  errors={field.state.meta.errors}
                >
                  <Input
                    id="register-confirm"
                    type="password"
                    autoComplete="new-password"
                    placeholder="Repeat your password"
                    value={field.state.value}
                    onBlur={field.handleBlur}
                    onChange={(event) => field.handleChange(event.target.value)}
                  />
                </FormField>
              )}
            </form.Field>
          </CardContent>

          <CardFooter className="mt-4 flex-col gap-3">
            <Button
              type="submit"
              className="w-full"
              disabled={signUp.isPending}
            >
              {signUp.isPending ? "Creating account…" : "Create Account"}
            </Button>
            <div className="flex w-full justify-end">
              <button
                type="button"
                className="text-primary cursor-pointer text-sm hover:underline"
                onClick={() => navigate("/login")}
              >
                Back to sign in
              </button>
            </div>
          </CardFooter>
        </form>
      </Card>
    </AuthLayout>
  );
}
