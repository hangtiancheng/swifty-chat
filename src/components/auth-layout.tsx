import { motion } from "motion/react";
import type { ReactNode } from "react";

/** Shared chrome for the sign-in and register screens. */
export function AuthLayout({ children }: { children: ReactNode }) {
  return (
    <div className="bg-background relative flex min-h-screen items-center justify-center overflow-hidden p-4">
      <motion.div
        aria-hidden
        className="bg-primary/10 pointer-events-none absolute -top-32 -left-32 size-96 rounded-full blur-3xl"
        animate={{ scale: [1, 1.15, 1], opacity: [0.6, 0.9, 0.6] }}
        transition={{ duration: 7, repeat: Infinity, ease: "easeInOut" }}
      />
      <motion.div
        aria-hidden
        className="bg-primary/5 pointer-events-none absolute -right-24 -bottom-40 size-[28rem] rounded-full blur-3xl"
        animate={{ scale: [1, 1.1, 1], opacity: [0.5, 0.8, 0.5] }}
        transition={{
          duration: 9,
          repeat: Infinity,
          ease: "easeInOut",
          delay: 1.5,
        }}
      />
      <div
        aria-hidden
        className="bg-primary/[0.07] pointer-events-none absolute top-1/4 right-1/3 size-64 rounded-full blur-3xl"
      />

      <motion.div
        className="w-full max-w-md"
        initial={{ opacity: 0, scale: 0.95, y: 8 }}
        animate={{ opacity: 1, scale: 1, y: 0 }}
        transition={{ duration: 0.3, ease: "easeOut" }}
      >
        {children}
      </motion.div>
    </div>
  );
}
