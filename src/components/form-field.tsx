import { AnimatePresence, motion } from "motion/react";
import type { ReactNode } from "react";

import { Label } from "@/components/ui/label";

/** react-form yields Standard Schema issue objects, but a plain function
 * validator yields whatever it returned, so both shapes must be handled. */
function firstMessage(errors: readonly unknown[]): string | undefined {
  for (const error of errors) {
    if (typeof error === "string") return error;
    if (error && typeof error === "object" && "message" in error) {
      const { message } = error as { message?: unknown };
      if (typeof message === "string") return message;
    }
  }
  return undefined;
}

interface FormFieldProps {
  label: string;
  htmlFor: string;
  errors: readonly unknown[];
  children: ReactNode;
}

export function FormField({
  label,
  htmlFor,
  errors,
  children,
}: FormFieldProps) {
  const message = firstMessage(errors);
  return (
    <div className="flex flex-col gap-2">
      <Label htmlFor={htmlFor}>{label}</Label>
      {children}
      <AnimatePresence initial={false}>
        {message && (
          <motion.p
            key={message}
            initial={{ opacity: 0, y: -4 }}
            animate={{ opacity: 1, y: 0 }}
            exit={{ opacity: 0, y: -4 }}
            transition={{ duration: 0.15 }}
            className="text-destructive text-xs"
          >
            {message}
          </motion.p>
        )}
      </AnimatePresence>
    </div>
  );
}
