"use client";

import type { ReactNode } from "react";
import { GitCommitHorizontal, Hammer, Rocket, Tag } from "lucide-react";
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from "@/components/ui/tooltip";
import { VersionLabel } from "@/components/version-label";
import { useBuildInfo } from "@/lib/queries";
import { absoluteTime, isZeroTime, timeAgo } from "@/lib/time";
import { useNow } from "@/lib/use-now";
import { cn } from "@/lib/utils";

/**
 * Slim build-info bar pinned under the dashboard: which build of Ring Promoter
 * this tab is talking to, and when it was built and rolled out. Fed by
 * /version, which the deploy pipeline stamps into the binary at build time.
 */
export function AppFooter() {
  const { data, isError } = useBuildInfo();
  const now = useNow(30_000);

  return (
    <footer className="flex h-9 shrink-0 items-center gap-3 border-t px-3 text-xs text-muted-foreground md:gap-5 md:px-4">
      <span className="shrink-0 font-medium text-foreground/70">
        Ring Promoter
      </span>

      {!data ? (
        <span className="truncate">
          {isError ? "build info unavailable" : "loading build info…"}
        </span>
      ) : (
        <>
          <Item icon={<Tag aria-hidden className="size-3 shrink-0" />}>
            <VersionLabel version={data.version} />
          </Item>

          <Item
            className="hidden sm:flex"
            icon={
              <GitCommitHorizontal aria-hidden className="size-3 shrink-0" />
            }
          >
            <VersionLabel
              version={stamped(data.commit) ? data.commit : ""}
              className="max-w-[13ch]"
            />
          </Item>

          <Stamp
            className="hidden md:flex"
            icon={<Hammer aria-hidden className="size-3 shrink-0" />}
            label="built"
            title="Compiled"
            iso={data.built_at}
          />

          <Stamp
            icon={<Rocket aria-hidden className="size-3 shrink-0" />}
            label="deployed"
            title="This build started serving"
            iso={data.started_at}
            extra={now ? `up ${uptime(data.started_at, now)}` : undefined}
          />
        </>
      )}
    </footer>
  );
}

/** Build metadata is a placeholder ("none", "unknown") outside a CI build. */
function stamped(value: string): boolean {
  return !!value && value !== "none" && value !== "unknown";
}

function Item({
  icon,
  children,
  className,
}: {
  icon: ReactNode;
  children: ReactNode;
  className?: string;
}) {
  return (
    <span className={cn("flex min-w-0 items-center gap-1.5", className)}>
      {icon}
      {children}
    </span>
  );
}

/** A timestamp shown relative ("2h ago"), with the absolute time on hover. */
function Stamp({
  icon,
  label,
  title,
  iso,
  extra,
  className,
}: {
  icon: ReactNode;
  label: string;
  title: string;
  iso: string;
  extra?: string;
  className?: string;
}) {
  const missing = isZeroTime(iso);
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <span
          className={cn(
            "flex min-w-0 cursor-default items-center gap-1.5",
            className,
          )}
        >
          {icon}
          <span className="truncate">
            {label} {missing ? "—" : timeAgo(iso)}
          </span>
        </span>
      </TooltipTrigger>
      <TooltipContent>
        {missing
          ? `${title}: not recorded in this build`
          : [`${title} ${absoluteTime(iso)}`, extra].filter(Boolean).join(" · ")}
      </TooltipContent>
    </Tooltip>
  );
}

function uptime(startedAt: string, now: number): string {
  const secs = Math.max(
    0,
    Math.round((now - new Date(startedAt).getTime()) / 1000),
  );
  const days = Math.floor(secs / 86_400);
  const hours = Math.floor((secs % 86_400) / 3_600);
  const mins = Math.floor((secs % 3_600) / 60);
  if (days) return `${days}d ${hours}h`;
  if (hours) return `${hours}h ${mins}m`;
  return `${mins}m`;
}
