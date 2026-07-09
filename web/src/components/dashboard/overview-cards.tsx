"use client";

import { Activity, HeartPulse, Rocket, Radio } from "lucide-react";
import { RelativeTime } from "@/components/relative-time";
import { ACTION_META } from "@/components/status";
import { Skeleton } from "@/components/ui/skeleton";
import { VersionLabel } from "@/components/version-label";
import { usePrefsStore } from "@/lib/stores";
import { useNow } from "@/lib/use-now";
import type { HistoryEntry, RingView } from "@/lib/types";
import { cn } from "@/lib/utils";

/** KPI row: stat tiles — value + one line of context each. */
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
  const autoRefresh = usePrefsStore((s) => s.autoRefresh);

  // Quantized clock so "updated Xs ago" stays live.
  const now = useNow(5_000);

  if (!rings) {
    return (
      <div className="grid gap-3 sm:grid-cols-2 xl:grid-cols-4">
        {Array.from({ length: 4 }).map((_, i) => (
          <Skeleton key={i} className="h-24 rounded-xl" />
        ))}
      </div>
    );
  }

  const prod = rings.find((r) => r.ring.name === "prod");
  const active = rings.filter((r) => r.configured && r.current_version);
  const healthy = active.filter((r) => r.live_healthy);
  const last = history?.[0];
  const allHealthy = active.length > 0 && healthy.length === active.length;
  const updatedAgo =
    updatedAt && now ? Math.max(0, Math.round((now - updatedAt) / 1000)) : null;

  return (
    <div className="grid gap-3 sm:grid-cols-2 xl:grid-cols-4">
      <Tile
        icon={Rocket}
        label="Production"
        value={
          prod?.current_version ? (
            <VersionLabel version={prod.current_version} className="text-lg" />
          ) : (
            <span className="text-lg text-muted-foreground">—</span>
          )
        }
        sub={
          prod?.current_version
            ? prod.live_healthy
              ? "live and healthy"
              : "health check failing"
            : prod?.configured
              ? "nothing deployed yet"
              : "ring not configured"
        }
        subClass={
          prod?.current_version && !prod.live_healthy
            ? "text-status-critical"
            : undefined
        }
      />
      <Tile
        icon={HeartPulse}
        label="Ring health"
        value={
          <span
            className={cn(
              "text-lg font-semibold",
              active.length === 0
                ? "text-muted-foreground"
                : allHealthy
                  ? "text-status-good"
                  : "text-status-critical",
            )}
          >
            {active.length === 0 ? "—" : `${healthy.length}/${active.length}`}
          </span>
        }
        sub={
          active.length === 0
            ? "no versions deployed"
            : allHealthy
              ? "all live checks passing"
              : `${active.length - healthy.length} ring${active.length - healthy.length === 1 ? "" : "s"} failing`
        }
        subClass={
          active.length > 0 && !allHealthy ? "text-status-critical" : undefined
        }
      />
      <Tile
        icon={Activity}
        label="Last activity"
        value={
          last ? (
            <span className="text-lg font-semibold">
              {ACTION_META[last.action]?.label ?? last.action}
              <span className="text-muted-foreground"> → {last.ring}</span>
            </span>
          ) : (
            <span className="text-lg text-muted-foreground">none yet</span>
          )
        }
        sub={
          last ? (
            <>
              {last.result === "success" ? "succeeded" : "failed"}{" "}
              <RelativeTime iso={last.created_at} />
            </>
          ) : (
            `no history for ${app}`
          )
        }
        subClass={
          last?.result === "failure" ? "text-status-critical" : undefined
        }
      />
      <Tile
        icon={Radio}
        label="Live updates"
        value={
          <span className="inline-flex items-center gap-2 text-lg font-semibold">
            <span className="relative flex size-2">
              {autoRefresh && (
                <span className="absolute inline-flex h-full w-full animate-ping rounded-full bg-status-good opacity-60" />
              )}
              <span
                className={cn(
                  "relative inline-flex size-2 rounded-full",
                  autoRefresh ? "bg-status-good" : "bg-status-warning",
                )}
              />
            </span>
            {autoRefresh ? "On" : "Paused"}
          </span>
        }
        sub={
          updatedAgo === null
            ? "waiting for first refresh"
            : updatedAgo <= 1
              ? "updated just now"
              : `updated ${updatedAgo}s ago`
        }
      />
    </div>
  );
}

function Tile({
  icon: Icon,
  label,
  value,
  sub,
  subClass,
}: {
  icon: React.ComponentType<{ className?: string }>;
  label: string;
  value: React.ReactNode;
  sub: React.ReactNode;
  subClass?: string;
}) {
  return (
    <div className="rounded-xl border bg-card p-4 shadow-xs">
      <div className="flex items-center gap-1.5 text-xs font-medium text-muted-foreground">
        <Icon aria-hidden className="size-3.5" />
        {label}
      </div>
      <div className="mt-2 min-w-0">{value}</div>
      <p className={cn("mt-1 truncate text-xs text-muted-foreground", subClass)}>
        {sub}
      </p>
    </div>
  );
}
