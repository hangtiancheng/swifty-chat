import { Progress as ProgressPrimitive } from "@base-ui/react/progress";

import { cn } from "@/lib/utils";

function Progress({
  className,
  ...props
}: ProgressPrimitive.Root.Props & { className?: string }) {
  return (
    <ProgressPrimitive.Root className={cn("w-full", className)} {...props}>
      <ProgressPrimitive.Track className="bg-muted relative h-1.5 w-full overflow-hidden rounded-full">
        <ProgressPrimitive.Indicator className="bg-primary h-full transition-all duration-200" />
      </ProgressPrimitive.Track>
    </ProgressPrimitive.Root>
  );
}

export { Progress };
