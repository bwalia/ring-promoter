"use client";

import { useState } from "react";
import type { NodeStatus } from "@/components/group-ring";
import { GroupDialog } from "@/components/group-dialog";
import { QaStatusBar } from "@/components/qa-status-bar";
import { DescentStage } from "@/components/descent-stage";
import { RingsAppFilter } from "@/components/rings-app-filter";
import { Button } from "@/components/ui/button";
import { matchesAppQuery, parseAppQuery } from "@/lib/app-filter";
import { summarizeRings } from "@/lib/app-health";
import {
  useAddTopologyEdge,
  useAppLocations,
  useApps,
  useAppTitle,
  useDeployingApps,
  useGroupRings,
  useGroups,
  useRemoveTopologyEdge,
  useTopology,
  type GroupAppRings,
} from "@/lib/queries";
import { usePrefsStore } from "@/lib/stores";
import { useUiStore } from "@/lib/ui-store";

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

/**
 * "Rings of Applications" in the nav; the stage itself is titled "Rings of
 * Apps". One Descent stage: every app is a spoke through the promotion
 * rings, and groups ("rings" in the nav) are arcs around the outside.
 */
export function FleetView() {
  const { data } = useApps();
  const known = data?.apps ?? [];
  const appLocations = useAppLocations();
  const groups = useGroups().data ?? [];
  const { data: appEdges = [] } = useTopology();
  const addEdge = useAddTopologyEdge();
  const removeEdge = useRemoveTopologyEdge();
  const selectApp = usePrefsStore((s) => s.selectApp);
  const selectGroup = usePrefsStore((s) => s.selectGroup);
  const setPendingAction = useUiStore((s) => s.setPendingAction);
  const [createOpen, setCreateOpen] = useState(false);

  const appResults = useGroupRings(known);
  const deploying = useDeployingApps(known);
  const appStatuses: NodeStatus[] = appResults.map((r) =>
    deploying.has(r.app) ? "deploying" : baseStatus(r),
  );

  // Keyword / wildcard filter. Health is still fetched for every app (the
  // queries are shared with the sidebar and app pages); only what the stage
  // draws is narrowed, so clearing the filter is instant.
  const query = useUiStore((s) => s.fleetQuery);
  const setQuery = useUiStore((s) => s.setFleetQuery);
  const title = useAppTitle();
  const parsed = parseAppQuery(query);
  const groupNames = (app: string) =>
    groups.filter((g) => g.apps.includes(app)).map((g) => g.name);
  const shownIdx = known.flatMap((app, i) =>
    matchesAppQuery(parsed, [app, title(app), ...groupNames(app)]) ? [i] : [],
  );
  const shownApps = shownIdx.map((i) => known[i]);
  const shownResults = shownIdx.map((i) => appResults[i]);
  const shownStatuses = shownIdx.map((i) => appStatuses[i]);

  const seedApp = (app: string) => {
    setPendingAction({ type: "seed", app });
    selectApp(app);
  };

  return (
    <div className="relative flex h-full min-h-0 flex-col overflow-hidden">
      {known.length === 0 ? (
        <div className="flex flex-1 flex-col items-center justify-center gap-3 p-10 text-center">
          <p className="text-sm font-medium">No apps configured</p>
          <p className="text-sm text-muted-foreground">
            Add apps under <code>apps:</code> in the server config.
          </p>
        </div>
      ) : (
        <DescentStage
          className="min-h-0 flex-1"
          apps={shownApps}
          totalApps={known.length}
          results={shownResults}
          statuses={shownStatuses}
          aggregate={aggregateStatuses(shownStatuses)}
          deploying={deploying}
          edges={appEdges}
          groups={groups}
          locations={appLocations}
          editable
          onAddEdge={(from, to) => addEdge.mutate({ from, to })}
          onRemoveEdge={(from, to) => removeEdge.mutate({ from, to })}
          onOpen={selectApp}
          onSeed={seedApp}
          onOpenGroup={selectGroup}
          filter={
            <RingsAppFilter
              value={query}
              onChange={setQuery}
              shown={shownApps.length}
              total={known.length}
            />
          }
          onClearFilter={() => setQuery("")}
          actions={
            <>
              <QaStatusBar compact />
              <Button
                variant="outline"
                size="sm"
                className="h-8 border-white/15 bg-[#07070a]/70 text-xs text-neutral-100 backdrop-blur-md hover:bg-white/10"
                onClick={() => setCreateOpen(true)}
              >
                New ring
              </Button>
            </>
          }
        />
      )}

      {createOpen && (
        <GroupDialog open apps={known} onOpenChange={setCreateOpen} />
      )}
    </div>
  );
}
