import { useMutation, useQueryClient } from "@tanstack/react-query";
import { AnimatePresence, motion } from "motion/react";

import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { contact } from "@/service/api";
import { keys } from "@/service/queries";
import type { Apply } from "@/service/schemas";
import { showToast } from "@/utils/toast";

/** Friend requests and group join requests share these three endpoints. */
type Verdict = "pass" | "refuse" | "block";

const RESOLVE: Record<
  Verdict,
  { run: (id: string) => Promise<void>; done: string }
> = {
  pass: { run: contact.passApply, done: "Approved" },
  refuse: { run: contact.refuseApply, done: "Refused" },
  block: { run: contact.blackApply, done: "Blocked" },
};

interface ApplyListDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  title: string;
  emptyLabel: string;
  applies: Apply[];
  isPending: boolean;
  allowBlock?: boolean;
}

export function ApplyListDialog({
  open,
  onOpenChange,
  title,
  emptyLabel,
  applies,
  isPending,
  allowBlock,
}: ApplyListDialogProps) {
  const queryClient = useQueryClient();

  const resolve = useMutation({
    mutationFn: ({ verdict, applyId }: { verdict: Verdict; applyId: string }) =>
      RESOLVE[verdict].run(applyId),
    onSuccess: (_result, { verdict }) => {
      showToast(RESOLVE[verdict].done, "success");
      void queryClient.invalidateQueries({ queryKey: keys.contacts.all });
      void queryClient.invalidateQueries({ queryKey: keys.groups.all });
      void queryClient.invalidateQueries({ queryKey: keys.sessions.all });
    },
  });

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle>{title}</DialogTitle>
        </DialogHeader>

        {applies.length === 0 ? (
          <p className="text-muted-foreground py-4 text-center text-sm">
            {isPending ? "Loading…" : emptyLabel}
          </p>
        ) : (
          <div className="flex max-h-60 flex-col gap-2 overflow-y-auto">
            <AnimatePresence initial={false}>
              {applies.map((apply) => (
                <motion.div
                  key={apply.apply_id}
                  layout="position"
                  initial={{ opacity: 0, y: -4 }}
                  animate={{ opacity: 1, y: 0 }}
                  exit={{ opacity: 0, height: 0 }}
                  transition={{ duration: 0.18 }}
                  className="border-border flex items-center justify-between gap-2 border-b py-2"
                >
                  <div className="flex min-w-0 items-center gap-2">
                    <span className="text-foreground truncate text-sm">
                      {apply.contact_name || apply.user_id}
                    </span>
                    {apply.message && (
                      <span className="text-muted-foreground truncate text-xs">
                        ({apply.message})
                      </span>
                    )}
                  </div>
                  <div className="flex shrink-0 gap-1">
                    <Button
                      size="sm"
                      className="px-2 text-xs"
                      disabled={resolve.isPending}
                      onClick={() =>
                        resolve.mutate({
                          verdict: "pass",
                          applyId: apply.apply_id,
                        })
                      }
                    >
                      Approve
                    </Button>
                    <Button
                      size="sm"
                      variant="ghost"
                      className="text-muted-foreground px-2 text-xs"
                      disabled={resolve.isPending}
                      onClick={() =>
                        resolve.mutate({
                          verdict: "refuse",
                          applyId: apply.apply_id,
                        })
                      }
                    >
                      Refuse
                    </Button>
                    {allowBlock && (
                      <Button
                        size="sm"
                        variant="ghost"
                        className="text-destructive px-2 text-xs"
                        disabled={resolve.isPending}
                        onClick={() =>
                          resolve.mutate({
                            verdict: "block",
                            applyId: apply.apply_id,
                          })
                        }
                      >
                        Block
                      </Button>
                    )}
                  </div>
                </motion.div>
              ))}
            </AnimatePresence>
          </div>
        )}

        <DialogFooter>
          <Button variant="ghost" size="sm" onClick={() => onOpenChange(false)}>
            Close
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
