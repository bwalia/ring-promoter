"use client";

import { RelativeTime } from "@/components/relative-time";
import { ACTION_META } from "@/components/status";
import { Skeleton } from "@/components/ui/skeleton";
import { VersionLabel } from "@/components/version-label";
import { useAppTitle } from "@/lib/queries";
import { usePrefsStore } from "@/lib/stores";
import { useNow } from "@/lib/use-now";
import type { HistoryEntry, RingView } from "@/lib/types";
import { cn } from "@/lib/utils";

/** Quiet stat strip: production version, ring health, last activity. */
export function OverviewCards({
  app,
  rings,
  history,
  updatedAt,
}: {
  app: string;
  rings: RingView[] | undefined;
  history: HistoryEntry[] | undefined;
  updatedAt: number;
}) {
  const title = useAppTitle();
  const autoRefresh = usePrefsStore((s) => s.autoRefresh);
  const now = useNow(5_000);

  if (!rings) {
    return <Skeleton className="h-12 w-full max-w-2xl" />;
  }

  const prod = rings.find((r) => r.ring.name === "prod");
  const active = rings.filter((r) => r.configured && r.current_version);
  const healthy = active.filter((r) => r.live_healthy);
  const last = history?.[0];
  const allHealthy = active.length > 0 && healthy.length === active.length;
  const updatedAgo =
    updatedAt && now ? Math.max(0, Math.round((now - updatedAt) / 1000)) : null;

  return (
    <div className="flex flex-wrap items-baseline gap-x-10 gap-y-3">
      <Stat label="Production">
        {prod?.current_version ? (
          <VersionLabel version={prod.current_version} className="text-sm" />
        ) : (
          <span className="text-muted-foreground">—</span>
        )}
      </Stat>

      <Stat label="Ring health">
        {active.length === 0 ? (
          <span className="text-muted-foreground">—</span>
        ) : (
          <span
            className={cn(
              "inline-flex items-center gap-1.5",
              allHealthy ? "text-status-good" : "text-status-critical",
            )}
          >
            <span
              className={cn(
                "size-1.5 rounded-full",
                allHealthy ? "bg-status-good" : "bg-status-critical",
              )}
            />
            {healthy.length}/{active.length} healthy
          </span>
        )}
      </Stat>

      <Stat label="Last activity">
        {last ? (
          <span
            className={cn(
              last.result === "failure" && "text-status-critical",
            )}
          >
            {ACTION_META[last.action]?.label ?? last.action} → {last.ring}
            {last.result === "failure" && " (failed)"}{" "}
            <span className="text-muted-foreground">
              <RelativeTime iso={last.created_at} />
            </span>
          </span>
        ) : (
          <span className="text-muted-foreground">none for {title(app)}</span>
        )}
      </Stat>

      <span className="ml-auto self-center text-xs text-muted-foreground">
        {autoRefresh
          ? updatedAgo === null || updatedAgo <= 1
            ? "live · updated just now"
            : `live · updated ${updatedAgo}s ago`
          : "live updates paused"}
      </span>
    </div>
  );
}

function Stat({
  label,
  children,
}: {
  label: string;
  children: React.ReactNode;
}) {
  return (
    <div className="min-w-0">
      <p className="text-xs text-muted-foreground">{label}</p>
      <div className="mt-0.5 text-sm font-medium">{children}</div>
    </div>
  );
}
