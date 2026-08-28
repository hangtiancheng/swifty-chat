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

import * as z from "zod";

export const telephoneSchema = z
  .string()
  .regex(/^1[3-9]\d{9}$/, "Enter a valid mobile number");

export const passwordSchema = z
  .string()
  .min(6, "Password must be at least 6 characters");

export const nicknameSchema = z
  .string()
  .min(3, "Nickname must be 3-10 characters")
  .max(10, "Nickname must be 3-10 characters");

export const emailSchema = z.email("Enter a valid email address");

export const loginSchema = z.object({
  telephone: telephoneSchema,
  password: passwordSchema,
});

const withMatchingConfirmation = <T extends z.ZodObject>(schema: T) =>
  schema.refine((value) => value.password === value.confirmPassword, {
    message: "Passwords do not match",
    path: ["confirmPassword"],
  });

export const registerSchema = withMatchingConfirmation(
  z.object({
    nickname: nicknameSchema,
    telephone: telephoneSchema,
    password: passwordSchema,
    confirmPassword: z.string(),
  }),
);

export const resetPasswordSchema = withMatchingConfirmation(
  z.object({
    telephone: telephoneSchema,
    password: passwordSchema,
    confirmPassword: z.string(),
  }),
);

/** Every profile field is optional: a blank input leaves the value untouched. */
const optionalText = <T extends z.ZodType>(schema: T) =>
  z.union([schema, z.literal("")]);

export const profileSchema = z.object({
  nickname: optionalText(nicknameSchema),
  email: optionalText(emailSchema),
  birthday: z.string(),
  signature: z.string(),
});

export const groupSettingsSchema = z.object({
  name: optionalText(
    z
      .string()
      .min(3, "Group name must be 3-10 characters")
      .max(10, "Group name must be 3-10 characters"),
  ),
  notice: z.string(),
});

export const createGroupSchema = z.object({
  name: z
    .string()
    .min(3, "Group name must be 3-10 characters")
    .max(10, "Group name must be 3-10 characters"),
  notice: z.string(),
  addMode: z.union([z.literal(0), z.literal(1)]),
});
