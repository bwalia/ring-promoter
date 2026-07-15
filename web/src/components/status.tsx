"use client";

import {
  ArrowUpRight,
  CircleCheck,
  CircleDashed,
  CircleX,
  Download,
  Undo2,
} from "lucide-react";
import { Badge } from "@/components/ui/badge";
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from "@/components/ui/tooltip";
import type { Action, HistoryResult, RingView } from "@/lib/types";
import { cn } from "@/lib/utils";

// Status colors are always paired with an icon and/or a label — never color
// alone (see the dataviz status-palette rule).

export type RingHealth = "healthy" | "unhealthy" | "empty" | "unconfigured";

export function ringHealth(view: RingView): RingHealth {
  if (!view.configured) return "unconfigured";
  if (!view.current_version) return "empty";
  return view.live_healthy ? "healthy" : "unhealthy";
}

const HEALTH: Record<
  RingHealth,
  { label: string; dot: string; text: string; bg: string }
> = {
  healthy: {
    label: "Healthy",
    dot: "bg-status-good",
    text: "text-status-good",
    bg: "bg-status-good/10",
  },
  unhealthy: {
    label: "Unhealthy",
    dot: "bg-status-critical",
    text: "text-status-critical",
    bg: "bg-status-critical/10",
  },
  empty: {
    label: "No version",
    dot: "bg-muted-foreground/40",
    text: "text-muted-foreground",
    bg: "bg-muted",
  },
  unconfigured: {
    label: "Not configured",
    dot: "bg-muted-foreground/40",
    text: "text-muted-foreground",
    bg: "bg-muted",
  },
};

export function HealthBadge({
  health,
  detail,
  pulse,
  className,
}: {
  health: RingHealth;
  detail?: string;
  pulse?: boolean;
  className?: string;
}) {
  const spec = HEALTH[health];
  const badge = (
    <span
      className={cn(
        "inline-flex items-center gap-1.5 rounded-full px-2 py-0.5 text-xs font-medium",
        spec.text,
        spec.bg,
        className,
      )}
    >
      <span className="relative flex size-2">
        {pulse && health === "healthy" && (
          <span
            className={cn(
              "absolute inline-flex h-full w-full animate-ping rounded-full opacity-60",
              spec.dot,
            )}
          />
        )}
        <span
          className={cn("relative inline-flex size-2 rounded-full", spec.dot)}
        />
      </span>
      {spec.label}
    </span>
  );
  if (!detail) return badge;
  return (
    <Tooltip>
      <TooltipTrigger asChild>{badge}</TooltipTrigger>
      <TooltipContent className="max-w-96 break-words">{detail}</TooltipContent>
    </Tooltip>
  );
}

export const ACTION_META: Record<
  Action,
  { label: string; Icon: typeof Download }
> = {
  seed: { label: "Seed", Icon: Download },
  promote: { label: "Promote", Icon: ArrowUpRight },
  rollback: { label: "Rollback", Icon: Undo2 },
};

export function ActionBadge({ action }: { action: Action }) {
  const meta = ACTION_META[action] ?? {
    label: action,
    Icon: CircleDashed,
  };
  return (
    <Badge variant="secondary" className="gap-1 font-normal">
      <meta.Icon aria-hidden className="size-3" />
      {meta.label}
    </Badge>
  );
}

export function ResultIcon({
  result,
  message,
}: {
  result: HistoryResult;
  message?: string;
}) {
  const icon =
    result === "success" ? (
      <CircleCheck aria-hidden className="size-4 text-status-good" />
    ) : (
      <CircleX aria-hidden className="size-4 text-status-critical" />
    );
  const label = result === "success" ? "Success" : "Failed";
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <span className="inline-flex items-center" aria-label={label}>
          {icon}
        </span>
      </TooltipTrigger>
      <TooltipContent className="max-w-96 break-words">
        {label}
        {message ? ` — ${message}` : ""}
      </TooltipContent>
    </Tooltip>
  );
}
