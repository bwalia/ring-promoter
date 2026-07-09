"use client";

import { Rss } from "lucide-react";
import { RelativeTime } from "@/components/relative-time";
import { ACTION_META } from "@/components/status";
import { Skeleton } from "@/components/ui/skeleton";
import { useActivityFeed, useApps } from "@/lib/queries";
import { usePrefsStore } from "@/lib/stores";
import type { HistoryEntry } from "@/lib/types";
import { cn } from "@/lib/utils";

function describe(h: HistoryEntry): string {
  switch (h.action) {
    case "seed":
      return `seeded ${h.to_version} into ${h.ring}`;
    case "promote":
      return `promoted ${h.to_version} to ${h.ring}`;
    case "rollback":
      return `rolled ${h.ring} back to ${h.to_version}`;
    default:
      return `${h.action} ${h.to_version} (${h.ring})`;
  }
}

/** Recent seeds, promotions and rollbacks across every application. */
export function ActivityFeed() {
  const { data } = useApps();
  const apps = data?.apps ?? [];
  const { items, isPending } = useActivityFeed(apps);
  const selectApp = usePrefsStore((s) => s.selectApp);

  return (
    <section className="rounded-xl border bg-card shadow-xs">
      <div className="border-b p-3">
        <h2 className="flex items-center gap-2 text-sm font-semibold">
          <Rss aria-hidden className="size-4 text-muted-foreground" />
          Activity
        </h2>
        <p className="text-xs text-muted-foreground">All applications</p>
      </div>

      {isPending ? (
        <div className="space-y-2 p-3">
          {Array.from({ length: 6 }).map((_, i) => (
            <Skeleton key={i} className="h-9 w-full" />
          ))}
        </div>
      ) : items.length === 0 ? (
        <p className="p-6 text-center text-sm text-muted-foreground">
          No activity anywhere yet.
        </p>
      ) : (
        <ol className="max-h-[28rem] divide-y overflow-y-auto">
          {items.slice(0, 30).map((h) => {
            const meta = ACTION_META[h.action];
            const failed = h.result === "failure";
            return (
              <li key={`${h.app}-${h.id}`}>
                <button
                  type="button"
                  onClick={() => selectApp(h.app)}
                  className="flex w-full items-start gap-2.5 px-3 py-2.5 text-left hover:bg-muted/40"
                >
                  {meta && (
                    <meta.Icon
                      aria-hidden
                      className={cn(
                        "mt-0.5 size-4 shrink-0",
                        failed ? "text-status-critical" : "text-status-good",
                      )}
                    />
                  )}
                  <span className="min-w-0 flex-1">
                    <span className="block truncate text-sm">
                      <span className="font-medium">{h.app}</span>{" "}
                      <span className="text-muted-foreground">
                        {describe(h)}
                      </span>
                    </span>
                    <span
                      className={cn(
                        "text-xs",
                        failed
                          ? "text-status-critical"
                          : "text-muted-foreground",
                      )}
                    >
                      {failed ? "failed · " : ""}
                      <RelativeTime iso={h.created_at} />
                    </span>
                  </span>
                </button>
              </li>
            );
          })}
        </ol>
      )}
    </section>
  );
}
