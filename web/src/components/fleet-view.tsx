"use client";

import { useMemo, useState } from "react";
import { ActivityFeed } from "@/components/dashboard/activity-feed";
import type { NodeStatus } from "@/components/group-ring";
import { GroupDialog } from "@/components/group-dialog";
import { SolarSystem } from "@/components/solar-system";
import { Button } from "@/components/ui/button";
import { summarizeRings } from "@/lib/app-health";
import { appLatencyMs } from "@/lib/solar-layout";
import {
  useAddTopologyEdge,
  useApps,
  useDeployingApps,
  useGroupRings,
  useGroups,
  useRemoveTopologyEdge,
  useTopology,
  type GroupAppRings,
} from "@/lib/queries";
import { usePrefsStore } from "@/lib/stores";
import { useUiStore } from "@/lib/ui-store";
import type { AppGroup, TopologyEdge } from "@/lib/types";
import { cn } from "@/lib/utils";

const UNGROUPED_ID = "__ungrouped__";

type ViewMode = "apps" | "rings";

function baseStatus(r: GroupAppRings): NodeStatus {
  if (r.isPending || !r.rings) return "loading";
  const { active, healthy } = summarizeRings(r.rings);
  if (active.length === 0) return "empty";
  if (healthy === active.length) return "healthy";
  return healthy === 0 ? "failed" : "degraded";
}

const AGGREGATE_PRIORITY: NodeStatus[] = [
  "failed",
  "degraded",
  "deploying",
  "loading",
  "healthy",
  "empty",
];

function aggregateStatuses(statuses: NodeStatus[]): NodeStatus {
  return AGGREGATE_PRIORITY.find((s) => statuses.includes(s)) ?? "empty";
}

function groupMemberApps(group: AppGroup, known: string[]): string[] {
  return group.apps.filter((a) => known.includes(a));
}

/** Derive group↔group edges from app dependency topology. */
function groupEdges(
  groups: AppGroup[],
  appEdges: TopologyEdge[],
  known: string[],
): TopologyEdge[] {
  const appToGroup = new Map<string, string>();
  for (const g of groups) {
    for (const a of groupMemberApps(g, known)) {
      if (!appToGroup.has(a)) appToGroup.set(a, g.id);
    }
  }
  const seen = new Set<string>();
  const out: TopologyEdge[] = [];
  for (const e of appEdges) {
    const fromG = appToGroup.get(e.from);
    const toG = appToGroup.get(e.to);
    if (!fromG || !toG || fromG === toG) continue;
    const key = `${fromG}\0${toG}`;
    if (seen.has(key)) continue;
    seen.add(key);
    out.push({ from: fromG, to: toG, source: e.source });
  }
  return out;
}

/**
 * Solar System home with two views:
 * - All apps: every application is a planet (no grouping required)
 * - Rings: each group is a planet
 */
export function FleetView() {
  const { data } = useApps();
  const known = data?.apps ?? [];
  const groups = useGroups().data ?? [];
  const { data: appEdges = [] } = useTopology();
  const addEdge = useAddTopologyEdge();
  const removeEdge = useRemoveTopologyEdge();
  const selectApp = usePrefsStore((s) => s.selectApp);
  const selectGroup = usePrefsStore((s) => s.selectGroup);
  const setPendingAction = useUiStore((s) => s.setPendingAction);
  const [createOpen, setCreateOpen] = useState(false);
  const [view, setView] = useState<ViewMode>("apps");

  const ringPlanets = useMemo(() => {
    const listed = groups.map((g) => ({
      id: g.id,
      name: g.name,
      apps: groupMemberApps(g, known),
    }));
    const grouped = new Set(listed.flatMap((g) => g.apps));
    const ungrouped = known.filter((a) => !grouped.has(a));
    if (ungrouped.length > 0) {
      listed.push({
        id: UNGROUPED_ID,
        name: "Ungrouped",
        apps: ungrouped,
      });
    }
    return listed;
  }, [groups, known]);

  const appResults = useGroupRings(known);
  const deploying = useDeployingApps(known);
  const appResultByName = useMemo(() => {
    const m = new Map<string, GroupAppRings>();
    for (const r of appResults) m.set(r.app, r);
    return m;
  }, [appResults]);

  const appStatuses: NodeStatus[] = appResults.map((r) =>
    deploying.has(r.app) ? "deploying" : baseStatus(r),
  );
  const appAggregate = aggregateStatuses(appStatuses);

  const ringIds = ringPlanets.map((p) => p.id);
  const ringResults: GroupAppRings[] = ringPlanets.map((p) => {
    const rings = p.apps.flatMap((a) => appResultByName.get(a)?.rings ?? []);
    const pending = p.apps.some((a) => appResultByName.get(a)?.isPending);
    return { app: p.id, rings, isPending: pending, error: null };
  });
  const ringStatuses: NodeStatus[] = ringPlanets.map((p) => {
    if (p.apps.some((a) => deploying.has(a))) return "deploying";
    const memberStatuses = p.apps.map((a) => {
      const r = appResultByName.get(a);
      return r ? baseStatus(r) : ("loading" as NodeStatus);
    });
    return aggregateStatuses(memberStatuses);
  });
  const ringLatency: Record<string, number | null> = {};
  const ringSubtitles: Record<string, string> = {};
  for (const p of ringPlanets) {
    const lats = p.apps
      .map((a) => appLatencyMs(appResultByName.get(a)?.rings))
      .filter((n): n is number => n != null);
    ringLatency[p.id] = lats.length ? Math.max(...lats) : null;
    ringSubtitles[p.id] =
      p.apps.length === 1 ? "1 app" : `${p.apps.length} apps`;
  }
  const ringTitles = useMemo(() => {
    const m = new Map(ringPlanets.map((p) => [p.id, p.name]));
    return (id: string) => m.get(id) ?? id;
  }, [ringPlanets]);

  const openApp = (app: string) => selectApp(app);
  const seedApp = (app: string) => {
    setPendingAction({ type: "seed", app });
    selectApp(app);
  };

  return (
    <div className="mx-auto max-w-6xl space-y-6 p-4 md:p-6">
      <div className="flex flex-wrap items-end justify-between gap-3">
        <div>
          <h2 className="text-lg font-semibold">Solar System</h2>
          <p className="text-sm text-muted-foreground">
            {view === "apps"
              ? "Every application is a planet. Dependencies pull apps together; high health latency pushes them apart."
              : "Each planet is a ring (group of apps). Switch to All apps to see every application."}
          </p>
        </div>
        <div className="flex flex-wrap items-center gap-2">
          <div
            className="inline-flex rounded-md border border-border bg-muted/40 p-0.5"
            role="group"
            aria-label="Solar System view"
          >
            <button
              type="button"
              onClick={() => setView("apps")}
              className={cn(
                "rounded-sm px-3 py-1.5 text-xs font-medium transition-colors",
                view === "apps"
                  ? "bg-background text-foreground shadow-sm"
                  : "text-muted-foreground hover:text-foreground",
              )}
              data-testid="solar-view-apps"
            >
              All apps
            </button>
            <button
              type="button"
              onClick={() => setView("rings")}
              className={cn(
                "rounded-sm px-3 py-1.5 text-xs font-medium transition-colors",
                view === "rings"
                  ? "bg-background text-foreground shadow-sm"
                  : "text-muted-foreground hover:text-foreground",
              )}
              data-testid="solar-view-rings"
            >
              Rings
            </button>
          </div>
          {view === "rings" && (
            <Button
              variant="outline"
              size="sm"
              onClick={() => setCreateOpen(true)}
            >
              New ring
            </Button>
          )}
        </div>
      </div>

      {known.length === 0 ? (
        <div className="flex flex-col items-center gap-3 rounded-xl border border-dashed p-10 text-center">
          <p className="text-sm font-medium">No applications configured</p>
          <p className="text-sm text-muted-foreground">
            Add apps under <code>apps:</code> in the server config.
          </p>
        </div>
      ) : view === "apps" ? (
        <>
          <SolarSystem
            sunLabel="Sun"
            members={known}
            results={appResults}
            statuses={appStatuses}
            aggregate={appAggregate}
            edges={appEdges}
            groups={groups}
            mode="apps"
            editable
            onAddEdge={(from, to) => addEdge.mutate({ from, to })}
            onRemoveEdge={(from, to) => removeEdge.mutate({ from, to })}
            onOpen={openApp}
            onSeed={seedApp}
          />
          <ActivityFeed apps={known} />
        </>
      ) : ringPlanets.length === 0 ? (
        <div className="flex flex-col items-center gap-3 rounded-xl border border-dashed p-10 text-center">
          <p className="text-sm font-medium">No rings yet</p>
          <p className="text-sm text-muted-foreground">
            Create a group of apps — or switch to All apps to see every
            application as a planet.
          </p>
          <div className="flex gap-2">
            <Button size="sm" variant="outline" onClick={() => setView("apps")}>
              All apps
            </Button>
            <Button size="sm" onClick={() => setCreateOpen(true)}>
              Create a ring
            </Button>
          </div>
        </div>
      ) : (
        <>
          <SolarSystem
            sunLabel="Rings"
            members={ringIds}
            results={ringResults}
            statuses={ringStatuses}
            aggregate={aggregateStatuses(ringStatuses)}
            edges={groupEdges(groups, appEdges, known)}
            mode="groups"
            resolveTitle={ringTitles}
            subtitles={ringSubtitles}
            latencyById={ringLatency}
            onOpen={(id) => {
              if (id === UNGROUPED_ID) {
                setView("apps");
                return;
              }
              selectGroup(id);
            }}
          />
          <ActivityFeed apps={known} />
        </>
      )}

      {createOpen && (
        <GroupDialog open apps={known} onOpenChange={setCreateOpen} />
      )}
    </div>
  );
}
