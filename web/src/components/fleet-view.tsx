"use client";

import { useMemo } from "react";
import { ActivityFeed } from "@/components/dashboard/activity-feed";
import type { NodeStatus } from "@/components/group-ring";
import { GroupDialog } from "@/components/group-dialog";
import { SolarSystem } from "@/components/solar-system";
import { Button } from "@/components/ui/button";
import { summarizeRings } from "@/lib/app-health";
import { appLatencyMs } from "@/lib/solar-layout";
import {
  useApps,
  useDeployingApps,
  useGroupRings,
  useGroups,
  useTopology,
  type GroupAppRings,
} from "@/lib/queries";
import { usePrefsStore } from "@/lib/stores";
import type { AppGroup, TopologyEdge } from "@/lib/types";
import { useState } from "react";

const UNGROUPED_ID = "__ungrouped__";

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
 * Solar System home: each planet is a ring/group of apps.
 * Click a planet to open that group; high latency pushes planets apart.
 */
export function FleetView() {
  const { data } = useApps();
  const known = data?.apps ?? [];
  const groups = useGroups().data ?? [];
  const { data: appEdges = [] } = useTopology();
  const selectGroup = usePrefsStore((s) => s.selectGroup);
  const [createOpen, setCreateOpen] = useState(false);

  const planets = useMemo(() => {
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

  const allMemberApps = useMemo(
    () => [...new Set(planets.flatMap((p) => p.apps))],
    [planets],
  );
  const appResults = useGroupRings(allMemberApps);
  const deploying = useDeployingApps(allMemberApps);
  const appResultByName = useMemo(() => {
    const m = new Map<string, GroupAppRings>();
    for (const r of appResults) m.set(r.app, r);
    return m;
  }, [appResults]);

  const planetIds = planets.map((p) => p.id);
  const results: GroupAppRings[] = planets.map((p) => {
    const rings = p.apps.flatMap((a) => appResultByName.get(a)?.rings ?? []);
    const pending = p.apps.some((a) => appResultByName.get(a)?.isPending);
    return {
      app: p.id,
      rings,
      isPending: pending,
      error: null,
    };
  });

  const statuses: NodeStatus[] = planets.map((p) => {
    if (p.apps.some((a) => deploying.has(a))) return "deploying";
    const memberStatuses = p.apps.map((a) => {
      const r = appResultByName.get(a);
      return r ? baseStatus(r) : ("loading" as NodeStatus);
    });
    return aggregateStatuses(memberStatuses);
  });

  const latencyById: Record<string, number | null> = {};
  const subtitles: Record<string, string> = {};
  for (const p of planets) {
    const lats = p.apps
      .map((a) => appLatencyMs(appResultByName.get(a)?.rings))
      .filter((n): n is number => n != null);
    latencyById[p.id] = lats.length ? Math.max(...lats) : null;
    subtitles[p.id] =
      p.apps.length === 1 ? "1 app" : `${p.apps.length} apps`;
  }

  const edges =
    groups.length > 0 ? groupEdges(groups, appEdges, known) : [];
  const aggregate = aggregateStatuses(statuses);

  const titles = useMemo(() => {
    const m = new Map(planets.map((p) => [p.id, p.name]));
    return (id: string) => m.get(id) ?? id;
  }, [planets]);

  return (
    <div className="mx-auto max-w-6xl space-y-6 p-4 md:p-6">
      <div className="flex flex-wrap items-end justify-between gap-3">
        <div>
          <h2 className="text-lg font-semibold">Solar System</h2>
          <p className="text-sm text-muted-foreground">
            Each planet is a ring (group of apps). Tight dependencies pull rings
            together; high health latency pushes them apart.
          </p>
        </div>
        <Button variant="outline" size="sm" onClick={() => setCreateOpen(true)}>
          New ring
        </Button>
      </div>

      {known.length === 0 ? (
        <div className="flex flex-col items-center gap-3 rounded-xl border border-dashed p-10 text-center">
          <p className="text-sm font-medium">No applications configured</p>
          <p className="text-sm text-muted-foreground">
            Add apps under <code>apps:</code> in the server config.
          </p>
        </div>
      ) : planets.length === 0 ? (
        <div className="flex flex-col items-center gap-3 rounded-xl border border-dashed p-10 text-center">
          <p className="text-sm font-medium">No rings yet</p>
          <p className="text-sm text-muted-foreground">
            Create a group of apps — each group appears as a planet here.
          </p>
          <Button size="sm" onClick={() => setCreateOpen(true)}>
            Create a ring
          </Button>
        </div>
      ) : (
        <>
          <SolarSystem
            sunLabel="Rings"
            members={planetIds}
            results={results}
            statuses={statuses}
            aggregate={aggregate}
            edges={edges}
            mode="groups"
            resolveTitle={titles}
            subtitles={subtitles}
            latencyById={latencyById}
            onOpen={(id) => {
              if (id === UNGROUPED_ID) return;
              selectGroup(id);
            }}
          />
          <ActivityFeed apps={allMemberApps} />
        </>
      )}

      {createOpen && (
        <GroupDialog open apps={known} onOpenChange={setCreateOpen} />
      )}
    </div>
  );
}
