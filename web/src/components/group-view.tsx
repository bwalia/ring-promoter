"use client";

import { useState } from "react";
import { ChevronRight, Pencil } from "lucide-react";
import { ActivityFeed } from "@/components/dashboard/activity-feed";
import { GroupDialog } from "@/components/group-dialog";
import { GroupRing, type NodeStatus } from "@/components/group-ring";
import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from "@/components/ui/tooltip";
import { ringHealth } from "@/components/status";
import { summarizeRings } from "@/lib/app-health";
import {
  useApps,
  useDeployingApps,
  useGroupRings,
  type GroupAppRings,
} from "@/lib/queries";
import { usePrefsStore } from "@/lib/stores";
import { useUiStore } from "@/lib/ui-store";
import type { AppGroup, RingView } from "@/lib/types";
import { cn } from "@/lib/utils";

// Aggregate health of one app across its active (configured + deployed) rings.
function baseStatus(r: GroupAppRings): NodeStatus {
  if (r.isPending || !r.rings) return "loading";
  const { active, healthy } = summarizeRings(r.rings);
  if (active.length === 0) return "empty";
  if (healthy === active.length) return "healthy";
  return healthy === 0 ? "failed" : "degraded";
}

const STATUS_DOT: Record<NodeStatus, string> = {
  healthy: "bg-status-good",
  degraded: "bg-status-warning",
  failed: "bg-status-critical",
  deploying: "bg-[#3b82f6]",
  empty: "bg-muted-foreground/40",
  loading: "bg-muted-foreground/30 animate-pulse",
};

// Most-urgent-first: the first present status colors the whole ring.
const AGGREGATE_PRIORITY: NodeStatus[] = [
  "failed",
  "degraded",
  "deploying",
  "loading",
  "healthy",
];

function healthSummary(r: GroupAppRings): string {
  if (r.isPending || !r.rings) return "checking…";
  const { active, healthy } = summarizeRings(r.rings);
  if (active.length === 0) return "nothing deployed";
  return `${healthy}/${active.length} rings healthy`;
}

/** Group page: the deployment ring stage plus member list and activity. */
export function GroupView({ group }: { group: AppGroup }) {
  const { data } = useApps();
  const known = data?.apps ?? [];
  const members = group.apps.filter((a) => known.includes(a));
  const results = useGroupRings(members);
  const deploying = useDeployingApps(members);
  const selectApp = usePrefsStore((s) => s.selectApp);
  const setPendingAction = useUiStore((s) => s.setPendingAction);
  const [editOpen, setEditOpen] = useState(false);

  const statuses: NodeStatus[] = results.map((r) =>
    deploying.has(r.app) ? "deploying" : baseStatus(r),
  );

  // The ring wears the group's most urgent state.
  const aggregate: NodeStatus =
    AGGREGATE_PRIORITY.find((s) => statuses.includes(s)) ?? "empty";

  const openApp = (app: string) => selectApp(app);
  const seedApp = (app: string) => {
    setPendingAction({ type: "seed", app });
    selectApp(app);
  };

  return (
    <div className="mx-auto max-w-6xl space-y-6 p-4 md:p-6">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <div>
          <h2 className="text-lg font-semibold">{group.name}</h2>
          <p className="text-sm text-muted-foreground">
            {members.length} application{members.length === 1 ? "" : "s"}
          </p>
        </div>
        <Button variant="outline" size="sm" onClick={() => setEditOpen(true)}>
          <Pencil aria-hidden className="size-3.5" /> Edit group
        </Button>
      </div>

      {members.length === 0 ? (
        <div className="flex flex-col items-center gap-3 rounded-xl border border-dashed p-10 text-center">
          <p className="text-sm font-medium">This group is empty</p>
          <p className="text-sm text-muted-foreground">
            Add applications to see them on the group ring.
          </p>
          <Button size="sm" onClick={() => setEditOpen(true)}>
            Add applications
          </Button>
        </div>
      ) : (
        <>
          <GroupRing
            group={group}
            members={members}
            results={results}
            statuses={statuses}
            aggregate={aggregate}
            onOpen={openApp}
            onSeed={seedApp}
          />

          <div className="grid items-start gap-6 xl:grid-cols-3">
            <section className="rounded-xl border bg-card xl:col-span-2">
              <div className="border-b p-3">
                <h3 className="text-sm font-semibold">Applications</h3>
              </div>
              <ol className="divide-y">
                {results.map((r, i) => (
                  <li key={r.app}>
                    <button
                      type="button"
                      onClick={() => selectApp(r.app)}
                      className="flex w-full items-center gap-3 px-3 py-2.5 text-left hover:bg-muted/40"
                    >
                      <span
                        className={cn(
                          "size-2 shrink-0 rounded-full",
                          STATUS_DOT[statuses[i]],
                        )}
                      />
                      <span className="min-w-0 flex-1 truncate text-sm font-medium">
                        {r.app}
                      </span>
                      {r.isPending ? (
                        <Skeleton className="h-4 w-24" />
                      ) : (
                        <span className="flex items-center gap-1.5">
                          {(r.rings ?? [])
                            .filter((v) => v.configured)
                            .map((v) => (
                              <RingDot key={v.ring.name} view={v} />
                            ))}
                        </span>
                      )}
                      <span className="hidden text-xs text-muted-foreground sm:inline">
                        {deploying.has(r.app) ? "deploying…" : healthSummary(r)}
                      </span>
                      <ChevronRight
                        aria-hidden
                        className="size-4 shrink-0 text-muted-foreground"
                      />
                    </button>
                  </li>
                ))}
              </ol>
            </section>

            <ActivityFeed apps={members} />
          </div>
        </>
      )}

      {editOpen && (
        <GroupDialog
          open
          group={group}
          apps={known}
          onOpenChange={setEditOpen}
        />
      )}
    </div>
  );
}

/** One small dot per configured ring of an app, with details on hover. */
function RingDot({ view }: { view: RingView }) {
  // Classified by the shared ringHealth helper so this page can never
  // disagree with the pipeline cards about the same ring.
  const health = ringHealth(view);
  const color =
    health === "healthy"
      ? "bg-status-good"
      : health === "unhealthy"
        ? "bg-status-critical"
        : "bg-muted-foreground/30";
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <span className={cn("size-1.5 rounded-full", color)} />
      </TooltipTrigger>
      <TooltipContent>
        {view.ring.name}:{" "}
        {view.current_version
          ? `${view.current_version} · ${view.live_healthy ? "healthy" : "unhealthy"}`
          : "nothing deployed"}
      </TooltipContent>
    </Tooltip>
  );
}
