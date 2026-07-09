"use client";

import { useEffect, useState } from "react";
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from "@/components/ui/tooltip";
import { absoluteTime, isZeroTime, timeAgo } from "@/lib/time";
import { cn } from "@/lib/utils";

/** Relative timestamp that re-renders every 30s, with the absolute time on hover. */
export function RelativeTime({
  iso,
  className,
}: {
  iso: string;
  className?: string;
}) {
  const [, setTick] = useState(0);

  useEffect(() => {
    const t = setInterval(() => setTick((n) => n + 1), 30_000);
    return () => clearInterval(t);
  }, []);

  if (isZeroTime(iso)) {
    return <span className={cn("text-muted-foreground", className)}>—</span>;
  }
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <span className={cn("whitespace-nowrap", className)}>
          {timeAgo(iso)}
        </span>
      </TooltipTrigger>
      <TooltipContent>{absoluteTime(iso)}</TooltipContent>
    </Tooltip>
  );
}
